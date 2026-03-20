pub mod codex_setup;
pub mod cursor_setup;
pub mod gh_cli_setup;
pub mod images;
pub mod pr;
pub mod util;
pub mod workspace_summary;

use std::{
    collections::{HashMap, HashSet},
    path::{Component, Path, PathBuf},
};

use axum::{
    Extension, Json, Router,
    extract::{
        Query, State,
        ws::{Message, WebSocket, WebSocketUpgrade},
    },
    http::StatusCode,
    middleware::from_fn_with_state,
    response::{IntoResponse, Json as ResponseJson},
    routing::{get, post, put},
};
use db::models::{
    coding_agent_turn::CodingAgentTurn,
    execution_process::{ExecutionProcess, ExecutionProcessRunReason, ExecutionProcessStatus},
    merge::{Merge, MergeStatus, PrMerge, PullRequestInfo},
    project::SearchResult,
    repo::{Repo, RepoError},
    session::{CreateSession, Session},
    task::{Task, TaskRelationships, TaskStatus},
    workspace::{CreateWorkspace, Workspace, WorkspaceError},
    workspace_repo::{CreateWorkspaceRepo, RepoWithTargetBranch, WorkspaceRepo},
};
use deployment::Deployment;
use executors::{
    actions::{
        ExecutorAction, ExecutorActionType,
        script::{ScriptContext, ScriptRequest, ScriptRequestLanguage},
    },
    executors::{CodingAgent, ExecutorError},
    profile::{ExecutorConfigs, ExecutorProfileId},
};
use git2::BranchType;
use serde::{Deserialize, Serialize};
use services::services::{
    container::ContainerService,
    file_search::SearchQuery,
    git::{ConflictOp, GitCliError, GitServiceError},
    workspace_manager::WorkspaceManager,
};
use sqlx::Error as SqlxError;
use ts_rs::TS;
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{
    DeploymentImpl, error::ApiError, middleware::load_workspace_middleware,
    routes::task_attempts::gh_cli_setup::GhCliSetupError,
};

#[derive(Debug, Deserialize, Serialize, TS)]
pub struct RebaseTaskAttemptRequest {
    pub repo_id: Uuid,
    pub old_base_branch: Option<String>,
    pub new_base_branch: Option<String>,
}

#[derive(Debug, Deserialize, Serialize, TS)]
pub struct AbortConflictsRequest {
    pub repo_id: Uuid,
}

#[derive(Debug, Serialize, Deserialize, TS)]
#[serde(tag = "type", rename_all = "snake_case")]
#[ts(tag = "type", rename_all = "snake_case")]
pub enum GitOperationError {
    MergeConflicts { message: String, op: ConflictOp },
    RebaseInProgress,
}

#[derive(Debug, Deserialize)]
pub struct TaskAttemptQuery {
    pub task_id: Option<Uuid>,
}

#[derive(Debug, Deserialize)]
pub struct DiffStreamQuery {
    #[serde(default)]
    pub stats_only: bool,
}

#[derive(Debug, Deserialize)]
pub struct WorkspaceStreamQuery {
    pub archived: Option<bool>,
    pub limit: Option<i64>,
}

const WS_HEARTBEAT_INTERVAL_SECS: u64 = 30;

#[derive(Debug, Deserialize, TS)]
pub struct UpdateWorkspace {
    pub archived: Option<bool>,
    pub pinned: Option<bool>,
    pub name: Option<String>,
}

pub async fn get_task_attempts(
    State(deployment): State<DeploymentImpl>,
    Query(query): Query<TaskAttemptQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<Workspace>>>, ApiError> {
    let pool = &deployment.db().pool;
    let workspaces = Workspace::fetch_all(pool, query.task_id).await?;
    Ok(ResponseJson(ApiResponse::success(workspaces)))
}

pub async fn get_workspace_count(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<i64>>, ApiError> {
    let pool = &deployment.db().pool;
    let count = Workspace::count_all(pool).await?;
    Ok(ResponseJson(ApiResponse::success(count)))
}

pub async fn get_task_attempt(
    Extension(workspace): Extension<Workspace>,
) -> Result<ResponseJson<ApiResponse<Workspace>>, ApiError> {
    Ok(ResponseJson(ApiResponse::success(workspace)))
}

pub async fn update_workspace(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(request): Json<UpdateWorkspace>,
) -> Result<ResponseJson<ApiResponse<Workspace>>, ApiError> {
    let pool = &deployment.db().pool;
    Workspace::update(
        pool,
        workspace.id,
        request.archived,
        request.pinned,
        request.name.as_deref(),
    )
    .await?;
    let updated = Workspace::find_by_id(pool, workspace.id)
        .await?
        .ok_or(WorkspaceError::TaskNotFound)?;
    Ok(ResponseJson(ApiResponse::success(updated)))
}

#[derive(Debug, Serialize, Deserialize, ts_rs::TS)]
pub struct CreateTaskAttemptBody {
    pub task_id: Uuid,
    pub executor_profile_id: ExecutorProfileId,
    pub repos: Vec<WorkspaceRepoInput>,
}

#[derive(Debug, Serialize, Deserialize, ts_rs::TS)]
pub struct WorkspaceRepoInput {
    pub repo_id: Uuid,
    pub target_branch: String,
}

#[derive(Debug, Deserialize, Serialize, TS)]
pub struct RunAgentSetupRequest {
    pub executor_profile_id: ExecutorProfileId,
}

#[derive(Debug, Serialize, TS)]
pub struct RunAgentSetupResponse {}

#[axum::debug_handler]
pub async fn create_task_attempt(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateTaskAttemptBody>,
) -> Result<ResponseJson<ApiResponse<Workspace>>, ApiError> {
    let executor_profile_id = payload.executor_profile_id.clone();

    if payload.repos.is_empty() {
        return Err(ApiError::BadRequest(
            "At least one repository is required".to_string(),
        ));
    }

    let pool = &deployment.db().pool;
    let task = Task::find_by_id(&deployment.db().pool, payload.task_id)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    // Compute agent_working_dir based on repo count:
    // - Single repo: use repo name as working dir (agent runs in repo directory)
    // - Multiple repos: use None (agent runs in workspace root)
    let agent_working_dir = if payload.repos.len() == 1 {
        let repo = Repo::find_by_id(pool, payload.repos[0].repo_id)
            .await?
            .ok_or(RepoError::NotFound)?;
        Some(repo.name)
    } else {
        None
    };

    let attempt_id = Uuid::new_v4();
    let git_branch_name = deployment
        .container()
        .git_branch_from_workspace(&attempt_id, &task.title)
        .await;

    // G34-004: Check for existing running attempt to prevent concurrent duplicates.
    // If a workspace already has a running execution process for this task, return 409.
    let existing_workspaces = Workspace::fetch_all(pool, Some(payload.task_id)).await?;
    for existing_ws in &existing_workspaces {
        if !existing_ws.archived {
            let has_running = ExecutionProcess::has_running_non_dev_server_processes_for_workspace(
                pool,
                existing_ws.id,
            )
            .await?;
            if has_running {
                return Err(ApiError::Conflict(
                    "A running attempt already exists for this task".to_string(),
                ));
            }
        }
    }

    let workspace = Workspace::create(
        pool,
        &CreateWorkspace {
            branch: git_branch_name.clone(),
            agent_working_dir,
        },
        attempt_id,
        payload.task_id,
    )
    .await?;

    let workspace_repos: Vec<CreateWorkspaceRepo> = payload
        .repos
        .iter()
        .map(|r| CreateWorkspaceRepo {
            repo_id: r.repo_id,
            target_branch: r.target_branch.clone(),
        })
        .collect();

    WorkspaceRepo::create_many(pool, workspace.id, &workspace_repos).await?;

    // G34-005: Roll back the created DB records if start_workspace fails,
    // to avoid leaving orphaned workspace rows in the database.
    if let Err(start_err) = deployment
        .container()
        .start_workspace(&workspace, executor_profile_id.clone())
        .await
    {
        tracing::error!(
            workspace_id = %workspace.id,
            task_id = %payload.task_id,
            "start_workspace failed; rolling back created workspace DB record: {}",
            start_err
        );
        if let Err(del_err) = Workspace::delete(pool, workspace.id).await {
            tracing::error!(
                workspace_id = %workspace.id,
                "Failed to roll back workspace record after start_workspace failure: {}",
                del_err
            );
        }
        return Err(ApiError::from(start_err));
    }

    deployment
        .track_if_analytics_allowed(
            "task_attempt_started",
            serde_json::json!({
                "task_id": workspace.task_id.to_string(),
                "variant": &executor_profile_id.variant,
                "executor": &executor_profile_id.executor,
                "workspace_id": workspace.id.to_string(),
                "repository_count": payload.repos.len(),
            }),
        )
        .await;

    tracing::info!("Created attempt for task {}", task.id);

    Ok(ResponseJson(ApiResponse::success(workspace)))
}

#[axum::debug_handler]
pub async fn run_agent_setup(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<RunAgentSetupRequest>,
) -> Result<ResponseJson<ApiResponse<RunAgentSetupResponse>>, ApiError> {
    let executor_profile_id = payload.executor_profile_id;
    let config = ExecutorConfigs::get_cached();
    let coding_agent = config.get_coding_agent_or_default(&executor_profile_id);
    match coding_agent {
        CodingAgent::CursorAgent(_) => {
            cursor_setup::run_cursor_setup(&deployment, &workspace).await?;
        }
        CodingAgent::Codex(codex) => {
            codex_setup::run_codex_setup(&deployment, &workspace, &codex).await?;
        }
        _ => return Err(ApiError::Executor(ExecutorError::SetupHelperNotSupported)),
    }

    deployment
        .track_if_analytics_allowed(
            "agent_setup_script_executed",
            serde_json::json!({
                "executor_profile_id": executor_profile_id.to_string(),
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(RunAgentSetupResponse {})))
}

#[axum::debug_handler]
pub async fn stream_task_attempt_diff_ws(
    ws: WebSocketUpgrade,
    Query(params): Query<DiffStreamQuery>,
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> impl IntoResponse {
    let _ = Workspace::touch(&deployment.db().pool, workspace.id).await;

    let stats_only = params.stats_only;
    ws.on_upgrade(move |socket| async move {
        if let Err(e) = handle_task_attempt_diff_ws(socket, deployment, workspace, stats_only).await
        {
            tracing::warn!("diff WS closed: {}", e);
        }
    })
}

async fn handle_task_attempt_diff_ws(
    socket: WebSocket,
    deployment: DeploymentImpl,
    workspace: Workspace,
    stats_only: bool,
) -> anyhow::Result<()> {
    use futures_util::{SinkExt, StreamExt, TryStreamExt};
    use utils::log_msg::LogMsg;

    let stream = deployment
        .container()
        .stream_diff(&workspace, stats_only)
        .await?;

    let mut stream = stream.map_ok(|msg: LogMsg| msg.to_ws_message_unchecked());

    let (mut sender, mut receiver) = socket.split();
    let mut heartbeat =
        tokio::time::interval(tokio::time::Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));
    let mut client_closed = false;

    loop {
        tokio::select! {
            _ = heartbeat.tick() => {
                if sender.send(Message::Ping(Vec::new().into())).await.is_err() {
                    tracing::debug!(workspace_id = %workspace.id, "diff WS heartbeat send failed; closing");
                    client_closed = true;
                    break;
                }
            }
            // Wait for next stream item
            item = stream.next() => {
                match item {
                    Some(Ok(msg)) => {
                        if sender.send(msg).await.is_err() {
                            tracing::debug!(workspace_id = %workspace.id, "diff WS send failed; client disconnected");
                            client_closed = true;
                            break;
                        }
                    }
                    Some(Err(e)) => {
                        tracing::error!("stream error: {}", e);
                        break;
                    }
                    None => break,
                }
            }
            // Detect client disconnection
            msg = receiver.next() => {
                match msg {
                    Some(Ok(Message::Close(_))) => {
                        tracing::debug!(workspace_id = %workspace.id, "diff WS client requested close");
                        client_closed = true;
                        break;
                    }
                    Some(Ok(Message::Ping(payload))) => {
                        if sender.send(Message::Pong(payload)).await.is_err() {
                            tracing::debug!(workspace_id = %workspace.id, "diff WS failed to respond pong");
                            break;
                        }
                    }
                    Some(Ok(Message::Pong(_))) => {}
                    Some(Ok(_)) => {}
                    Some(Err(e)) => {
                        tracing::debug!(workspace_id = %workspace.id, error = %e, "diff WS receive error");
                        client_closed = true;
                        break;
                    }
                    None => {
                        tracing::debug!(workspace_id = %workspace.id, "diff WS receiver closed");
                        client_closed = true;
                        break;
                    }
                }
            }
        }
    }

    if !client_closed {
        let _ = sender.send(Message::Close(None)).await;
    }
    let _ = sender.close().await;

    Ok(())
}

pub async fn stream_workspaces_ws(
    ws: WebSocketUpgrade,
    Query(query): Query<WorkspaceStreamQuery>,
    State(deployment): State<DeploymentImpl>,
) -> impl IntoResponse {
    ws.on_upgrade(move |socket| async move {
        if let Err(e) = handle_workspaces_ws(socket, deployment, query.archived, query.limit).await
        {
            tracing::warn!("workspaces WS closed: {}", e);
        }
    })
}

async fn handle_workspaces_ws(
    socket: WebSocket,
    deployment: DeploymentImpl,
    archived: Option<bool>,
    limit: Option<i64>,
) -> anyhow::Result<()> {
    use futures_util::{SinkExt, StreamExt, TryStreamExt};

    let mut stream = deployment
        .events()
        .stream_workspaces_raw(archived, limit)
        .await?
        .map_ok(|msg| msg.to_ws_message_unchecked());

    let (mut sender, mut receiver) = socket.split();
    let mut heartbeat =
        tokio::time::interval(tokio::time::Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));
    let mut client_closed = false;

    loop {
        tokio::select! {
            _ = heartbeat.tick() => {
                if sender.send(Message::Ping(Vec::new().into())).await.is_err() {
                    tracing::debug!("workspaces WS heartbeat send failed; closing");
                    client_closed = true;
                    break;
                }
            }
            item = stream.next() => {
                match item {
                    Some(Ok(msg)) => {
                        if sender.send(msg).await.is_err() {
                            tracing::debug!("workspaces WS send failed; client disconnected");
                            client_closed = true;
                            break;
                        }
                    }
                    Some(Err(e)) => {
                        tracing::error!("stream error: {}", e);
                        break;
                    }
                    None => break,
                }
            }
            msg = receiver.next() => {
                match msg {
                    Some(Ok(Message::Close(_))) => {
                        tracing::debug!("workspaces WS client requested close");
                        client_closed = true;
                        break;
                    }
                    Some(Ok(Message::Ping(payload))) => {
                        if sender.send(Message::Pong(payload)).await.is_err() {
                            tracing::debug!("workspaces WS failed to respond pong");
                            break;
                        }
                    }
                    Some(Ok(Message::Pong(_))) => {}
                    Some(Ok(_)) => {}
                    Some(Err(e)) => {
                        tracing::debug!("workspaces WS receive error: {}", e);
                        client_closed = true;
                        break;
                    }
                    None => {
                        tracing::debug!("workspaces WS receiver closed");
                        client_closed = true;
                        break;
                    }
                }
            }
        }
    }

    if !client_closed {
        let _ = sender.send(Message::Close(None)).await;
    }
    let _ = sender.close().await;

    Ok(())
}

#[derive(Debug, Deserialize, Serialize, TS)]
pub struct MergeTaskAttemptRequest {
    pub repo_id: Uuid,
}

#[derive(Debug, Deserialize, Serialize, TS)]
pub struct PushTaskAttemptRequest {
    pub repo_id: Uuid,
}

async fn finalize_workspace_if_all_repos_merged(
    pool: &sqlx::SqlitePool,
    workspace: &Workspace,
    task_id: Uuid,
) -> Result<bool, ApiError> {
    let workspace_repos = WorkspaceRepo::find_by_workspace_id(pool, workspace.id).await?;
    if workspace_repos.is_empty() {
        return Ok(false);
    }

    let merges = Merge::find_by_workspace_id(pool, workspace.id).await?;
    let mut merged_repo_ids = HashSet::new();
    for merge in merges {
        match merge {
            Merge::Direct(direct_merge) => {
                merged_repo_ids.insert(direct_merge.repo_id);
            }
            Merge::Pr(pr_merge) => {
                if matches!(pr_merge.pr_info.status, MergeStatus::Merged) {
                    merged_repo_ids.insert(pr_merge.repo_id);
                }
            }
        }
    }

    let all_repos_merged = workspace_repos
        .iter()
        .all(|workspace_repo| merged_repo_ids.contains(&workspace_repo.repo_id));

    if all_repos_merged {
        // G34-012: Wrap Task status update and Workspace archival in a logical sequence
        // with rollback: if set_archived fails, revert Task status to avoid inconsistency.
        Task::update_status(pool, task_id, TaskStatus::Done).await?;
        if !workspace.pinned {
            if let Err(archive_err) = Workspace::set_archived(pool, workspace.id, true).await {
                tracing::error!(
                    workspace_id = %workspace.id,
                    task_id = %task_id,
                    error = %archive_err,
                    "set_archived failed after marking task Done; \
                     rolling back Task status to avoid inconsistency"
                );
                // Best-effort rollback: revert task status back to In Progress
                if let Err(rollback_err) =
                    Task::update_status(pool, task_id, TaskStatus::InProgress).await
                {
                    tracing::error!(
                        task_id = %task_id,
                        error = %rollback_err,
                        "Failed to roll back task status after archive failure; \
                         task may be in inconsistent state"
                    );
                }
                return Err(ApiError::Database(archive_err));
            }
        }
    }

    Ok(all_repos_merged)
}

#[axum::debug_handler]
pub async fn merge_task_attempt(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(request): Json<MergeTaskAttemptRequest>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;

    let workspace_repo =
        WorkspaceRepo::find_by_workspace_and_repo_id(pool, workspace.id, request.repo_id)
            .await?
            .ok_or(RepoError::NotFound)?;

    let repo = Repo::find_by_id(pool, workspace_repo.repo_id)
        .await?
        .ok_or(RepoError::NotFound)?;

    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_path = Path::new(&container_ref);
    let worktree_path = workspace_path.join(repo.name);

    let task = workspace
        .parent_task(pool)
        .await?
        .ok_or(ApiError::Workspace(WorkspaceError::TaskNotFound))?;
    let task_uuid_str = task.id.to_string();
    let first_uuid_section = task_uuid_str.split('-').next().unwrap_or(&task_uuid_str);

    let mut commit_message = format!("{} (gitcortex {})", task.title, first_uuid_section);

    // Add description on next line if it exists
    if let Some(description) = &task.description
        && !description.trim().is_empty()
    {
        commit_message.push_str("\n\n");
        commit_message.push_str(description);
    }

    let merge_commit_id = deployment.git().merge_changes(
        &repo.path,
        &worktree_path,
        &workspace.branch,
        &workspace_repo.target_branch,
        &commit_message,
    )?;

    // G34-006: Git merge succeeded but DB write may fail — git merge cannot be rolled back.
    // Log to dead-letter style error log so the operator can reconcile manually if needed.
    if let Err(db_err) = Merge::create_direct(
        pool,
        workspace.id,
        workspace_repo.repo_id,
        &workspace_repo.target_branch,
        &merge_commit_id,
    )
    .await
    {
        tracing::error!(
            workspace_id = %workspace.id,
            repo_id = %workspace_repo.repo_id,
            merge_commit = %merge_commit_id,
            target_branch = %workspace_repo.target_branch,
            error = %db_err,
            "[DEAD-LETTER] Git merge succeeded but DB record creation failed. \
             The merge commit '{}' is in git history but not recorded in the database. \
             Manual reconciliation may be required.",
            merge_commit_id
        );
        return Err(ApiError::Database(db_err));
    }
    let all_repos_merged =
        finalize_workspace_if_all_repos_merged(pool, &workspace, task.id).await?;
    if !all_repos_merged {
        tracing::info!(
            workspace_id = %workspace.id,
            repo_id = %workspace_repo.repo_id,
            "Repository merged, waiting for remaining repositories before marking task done"
        );
    }

    // Stop any running dev servers for this workspace
    let dev_servers =
        ExecutionProcess::find_running_dev_servers_by_workspace(pool, workspace.id).await?;

    for dev_server in dev_servers {
        tracing::info!(
            "Stopping dev server {} for completed task attempt {}",
            dev_server.id,
            workspace.id
        );

        if let Err(e) = deployment
            .container()
            .stop_execution(&dev_server, ExecutionProcessStatus::Killed)
            .await
        {
            tracing::error!(
                "Failed to stop dev server {} for task attempt {}: {}",
                dev_server.id,
                workspace.id,
                e
            );
        }
    }

    if all_repos_merged {
        tracing::debug!("Task {} marked done after all repositories merged", task.id);
    }

    deployment
        .track_if_analytics_allowed(
            "task_attempt_merged",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(())))
}

pub async fn push_task_attempt_branch(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(request): Json<PushTaskAttemptRequest>,
) -> Result<(StatusCode, ResponseJson<ApiResponse<(), PushError>>), ApiError> {
    let pool = &deployment.db().pool;

    let workspace_repo =
        WorkspaceRepo::find_by_workspace_and_repo_id(pool, workspace.id, request.repo_id)
            .await?
            .ok_or(RepoError::NotFound)?;

    let repo = Repo::find_by_id(pool, workspace_repo.repo_id)
        .await?
        .ok_or(RepoError::NotFound)?;

    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_path = Path::new(&container_ref);
    let worktree_path = workspace_path.join(&repo.name);

    match deployment
        .git()
        .push_to_remote(&worktree_path, &workspace.branch, false)
    {
        Ok(()) => Ok((StatusCode::OK, ResponseJson(ApiResponse::success(())))),
        Err(GitServiceError::GitCLI(GitCliError::PushRejected(_))) => Ok(push_rejected_response()),
        Err(e) => Err(ApiError::GitService(e)),
    }
}

fn push_rejected_response() -> (StatusCode, ResponseJson<ApiResponse<(), PushError>>) {
    (
        StatusCode::CONFLICT,
        ResponseJson(ApiResponse::error_with_data(PushError::ForcePushRequired)),
    )
}

pub async fn force_push_task_attempt_branch(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(request): Json<PushTaskAttemptRequest>,
) -> Result<ResponseJson<ApiResponse<(), PushError>>, ApiError> {
    let pool = &deployment.db().pool;

    let workspace_repo =
        WorkspaceRepo::find_by_workspace_and_repo_id(pool, workspace.id, request.repo_id)
            .await?
            .ok_or(RepoError::NotFound)?;

    let repo = Repo::find_by_id(pool, workspace_repo.repo_id)
        .await?
        .ok_or(RepoError::NotFound)?;

    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_path = Path::new(&container_ref);
    let worktree_path = workspace_path.join(&repo.name);

    deployment
        .git()
        .push_to_remote(&worktree_path, &workspace.branch, true)?;
    Ok(ResponseJson(ApiResponse::success(())))
}

#[derive(Debug, Serialize, Deserialize, TS)]
#[serde(tag = "type", rename_all = "snake_case")]
#[ts(tag = "type", rename_all = "snake_case")]
pub enum PushError {
    ForcePushRequired,
}

#[derive(serde::Deserialize, TS)]
pub struct OpenEditorRequest {
    #[serde(default)]
    pub editor_type: Option<String>,
    #[serde(default)]
    pub file_path: Option<String>,
    #[serde(default)]
    #[ts(optional)]
    pub git_repo_path: Option<String>,
}

#[derive(Debug, Serialize, TS)]
pub struct OpenEditorResponse {
    pub url: Option<String>,
}

fn normalize_editor_repo_path(path: &str) -> String {
    path.replace('\\', "/").trim_end_matches('/').to_string()
}

fn resolve_workspace_repo_for_editor<'a>(
    repositories: &'a [Repo],
    requested_repo_path: Option<&str>,
) -> Result<Option<&'a Repo>, ApiError> {
    if let Some(requested_repo_path) = requested_repo_path {
        let requested_repo_path = requested_repo_path.trim();
        if !requested_repo_path.is_empty() {
            let requested_repo_path = normalize_editor_repo_path(requested_repo_path);
            return repositories
                .iter()
                .find(|repo| {
                    normalize_editor_repo_path(&repo.path.to_string_lossy()) == requested_repo_path
                        || repo.name == requested_repo_path
                })
                .map(Some)
                .ok_or_else(|| {
                    ApiError::BadRequest(
                        "Requested repository is not part of this task attempt".to_string(),
                    )
                });
        }
    }

    Ok(repositories.first().filter(|_| repositories.len() == 1))
}

fn resolve_workspace_file_open_root(
    workspace_path: &Path,
    selected_repo: Option<&Repo>,
) -> PathBuf {
    if let Some(selected_repo) = selected_repo {
        return workspace_path.join(&selected_repo.name);
    }

    workspace_path.to_path_buf()
}

fn resolve_workspace_file_path_for_editor(
    base_path: &Path,
    file_path: &str,
    selected_repo_name: Option<&str>,
) -> Result<PathBuf, ApiError> {
    let trimmed_file_path = file_path.trim();
    if trimmed_file_path.is_empty() {
        return Ok(base_path.to_path_buf());
    }

    let mut relative_path = PathBuf::from(trimmed_file_path);
    if relative_path.is_absolute() {
        return Err(ApiError::BadRequest(
            "file_path must be relative to the selected root".to_string(),
        ));
    }

    if relative_path.components().any(|component| {
        matches!(
            component,
            Component::ParentDir | Component::Prefix(_) | Component::RootDir
        )
    }) {
        return Err(ApiError::BadRequest(
            "file_path must stay within the selected root".to_string(),
        ));
    }

    if let Some(repo_name) = selected_repo_name
        && let Ok(stripped) = relative_path.strip_prefix(repo_name)
    {
        if stripped.as_os_str().is_empty() {
            return Ok(base_path.to_path_buf());
        }
        relative_path = stripped.to_path_buf();
    }

    Ok(base_path.join(relative_path))
}

#[cfg(test)]
mod open_editor_path_tests {
    use std::path::{Path, PathBuf};

    use super::{normalize_editor_repo_path, resolve_workspace_file_path_for_editor};

    #[test]
    fn strips_repo_prefix_for_single_repo_workspace_file_path() {
        let resolved = resolve_workspace_file_path_for_editor(
            Path::new("/workspace/repo-a"),
            "repo-a/src/main.rs",
            Some("repo-a"),
        )
        .expect("path should resolve");

        assert_eq!(
            resolved,
            Path::new("/workspace/repo-a").join("src").join("main.rs")
        );
    }

    #[test]
    fn rejects_parent_dir_traversal_in_file_path() {
        let result =
            resolve_workspace_file_path_for_editor(Path::new("/workspace"), "../outside.txt", None);

        assert!(result.is_err(), "path traversal must be rejected");
    }

    #[test]
    fn normalizes_repo_path_slashes_and_trailing_separator() {
        let normalized = normalize_editor_repo_path(r"C:\work\repo-a\");
        assert_eq!(normalized, "C:/work/repo-a");
    }

    #[test]
    fn file_open_root_prefers_selected_repo() {
        let selected_repo = db::models::repo::Repo {
            id: uuid::Uuid::nil(),
            path: "/workspace/repo-a".into(),
            name: "repo-a".to_string(),
            display_name: "repo-a".to_string(),
            setup_script: None,
            cleanup_script: None,
            copy_files: None,
            parallel_setup_script: false,
            dev_server_script: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
        };

        let root =
            super::resolve_workspace_file_open_root(Path::new("/workspace"), Some(&selected_repo));

        assert_eq!(root, PathBuf::from("/workspace").join("repo-a"));
    }
}

#[cfg(test)]
mod status_semantics_tests {
    use super::*;

    #[test]
    fn push_rejected_response_uses_conflict_status() {
        let (status, _payload) = push_rejected_response();
        assert_eq!(status, StatusCode::CONFLICT);
    }

    #[test]
    fn rebase_conflict_response_uses_conflict_status() {
        let (status, _payload) = rebase_conflict_response(GitOperationError::RebaseInProgress);
        assert_eq!(status, StatusCode::CONFLICT);
    }
}

pub async fn open_task_attempt_in_editor(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<OpenEditorRequest>,
) -> Result<ResponseJson<ApiResponse<OpenEditorResponse>>, ApiError> {
    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;

    Workspace::touch(&deployment.db().pool, workspace.id).await?;

    let workspace_path = Path::new(&container_ref);

    // Resolve repo context when explicitly selected or when single-repo.
    let workspace_repos =
        WorkspaceRepo::find_repos_for_workspace(&deployment.db().pool, workspace.id).await?;
    let selected_repo =
        resolve_workspace_repo_for_editor(&workspace_repos, payload.git_repo_path.as_deref())?;

    let file_path = payload
        .file_path
        .as_deref()
        .filter(|value| !value.trim().is_empty());

    let base_path = resolve_workspace_file_open_root(workspace_path, selected_repo);

    let path = if let Some(file_path) = file_path {
        resolve_workspace_file_path_for_editor(
            &base_path,
            file_path,
            selected_repo.map(|repo| repo.name.as_str()),
        )?
    } else {
        base_path
    };

    let editor_config = {
        let config = deployment.config().read().await;
        let editor_type_str = payload.editor_type.as_deref();
        config.editor.with_override(editor_type_str)
    };

    match editor_config
        .open_file_with_hint(path.as_path(), Some(file_path.is_some()))
        .await
    {
        Ok(url) => {
            tracing::info!(
                "Opened editor for task attempt {} at path: {}{}",
                workspace.id,
                path.display(),
                if url.is_some() { " (remote mode)" } else { "" }
            );

            deployment
                .track_if_analytics_allowed(
                    "task_attempt_editor_opened",
                    serde_json::json!({
                        "workspace_id": workspace.id.to_string(),
                        "editor_type": payload.editor_type.as_ref(),
                        "remote_mode": url.is_some(),
                    }),
                )
                .await;

            Ok(ResponseJson(ApiResponse::success(OpenEditorResponse {
                url,
            })))
        }
        Err(e) => {
            tracing::error!(
                "Failed to open editor for attempt {}: {:?}",
                workspace.id,
                e
            );
            Err(ApiError::EditorOpen(e))
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, TS)]
pub struct BranchStatus {
    pub commits_behind: Option<usize>,
    pub commits_ahead: Option<usize>,
    pub has_uncommitted_changes: Option<bool>,
    pub head_oid: Option<String>,
    pub uncommitted_count: Option<usize>,
    pub untracked_count: Option<usize>,
    pub target_branch_name: String,
    pub remote_commits_behind: Option<usize>,
    pub remote_commits_ahead: Option<usize>,
    pub merges: Vec<Merge>,
    /// True if a `git rebase` is currently in progress in this worktree
    pub is_rebase_in_progress: bool,
    /// Current conflict operation if any
    pub conflict_op: Option<ConflictOp>,
    /// List of files currently in conflicted (unmerged) state
    pub conflicted_files: Vec<String>,
}

#[derive(Debug, Clone, Serialize, TS)]
pub struct RepoBranchStatus {
    pub repo_id: Uuid,
    pub repo_name: String,
    #[serde(flatten)]
    pub status: BranchStatus,
}

pub async fn get_task_attempt_branch_status(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<RepoBranchStatus>>>, ApiError> {
    let pool = &deployment.db().pool;

    let repositories = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;
    let workspace_repos = WorkspaceRepo::find_by_workspace_id(pool, workspace.id).await?;
    let target_branches: HashMap<_, _> = workspace_repos
        .iter()
        .map(|wr| (wr.repo_id, wr.target_branch.clone()))
        .collect();

    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_dir = PathBuf::from(&container_ref);

    // Batch fetch all merges for the workspace to avoid N+1 queries
    let all_merges = Merge::find_by_workspace_id(pool, workspace.id).await?;
    let merges_by_repo: HashMap<Uuid, Vec<Merge>> =
        all_merges
            .into_iter()
            .fold(HashMap::new(), |mut acc, merge| {
                let repo_id = match &merge {
                    Merge::Direct(dm) => dm.repo_id,
                    Merge::Pr(pm) => pm.repo_id,
                };
                acc.entry(repo_id).or_insert_with(Vec::new).push(merge);
                acc
            });

    let mut results = Vec::with_capacity(repositories.len());

    for repo in repositories {
        let Some(target_branch) = target_branches.get(&repo.id).cloned() else {
            continue;
        };

        let repo_merges = merges_by_repo.get(&repo.id).cloned().unwrap_or_default();

        let worktree_path = workspace_dir.join(&repo.name);

        let head_oid = deployment
            .git()
            .get_head_info(&worktree_path)
            .ok()
            .map(|h| h.oid);

        let (is_rebase_in_progress, conflicted_files, conflict_op) = {
            let in_rebase = deployment
                .git()
                .is_rebase_in_progress(&worktree_path)
                .unwrap_or(false);
            let conflicts = deployment
                .git()
                .get_conflicted_files(&worktree_path)
                .unwrap_or_default();
            let op = if conflicts.is_empty() {
                None
            } else {
                deployment
                    .git()
                    .detect_conflict_op(&worktree_path)
                    .unwrap_or(None)
            };
            (in_rebase, conflicts, op)
        };

        let (uncommitted_count, untracked_count) =
            match deployment.git().get_worktree_change_counts(&worktree_path) {
                Ok((a, b)) => (Some(a), Some(b)),
                Err(_) => (None, None),
            };

        let has_uncommitted_changes = uncommitted_count.map(|c| c > 0);

        let target_branch_type = deployment
            .git()
            .find_branch_type(&repo.path, &target_branch)?;

        let (commits_ahead, commits_behind) = match target_branch_type {
            BranchType::Local => {
                let (a, b) = deployment.git().get_branch_status(
                    &repo.path,
                    &workspace.branch,
                    &target_branch,
                )?;
                (Some(a), Some(b))
            }
            BranchType::Remote => {
                let (ahead, behind) = deployment.git().get_remote_branch_status(
                    &repo.path,
                    &workspace.branch,
                    Some(&target_branch),
                )?;
                (Some(ahead), Some(behind))
            }
        };

        let (remote_ahead, remote_behind) = if let Some(Merge::Pr(PrMerge {
            pr_info:
                PullRequestInfo {
                    status: MergeStatus::Open,
                    ..
                },
            ..
        })) = repo_merges.first()
        {
            match deployment
                .git()
                .get_remote_branch_status(&repo.path, &workspace.branch, None)
            {
                Ok((ahead, behind)) => (Some(ahead), Some(behind)),
                Err(_) => (None, None),
            }
        } else {
            (None, None)
        };

        results.push(RepoBranchStatus {
            repo_id: repo.id,
            repo_name: repo.name,
            status: BranchStatus {
                commits_ahead,
                commits_behind,
                has_uncommitted_changes,
                head_oid,
                uncommitted_count,
                untracked_count,
                remote_commits_ahead: remote_ahead,
                remote_commits_behind: remote_behind,
                merges: repo_merges,
                target_branch_name: target_branch,
                is_rebase_in_progress,
                conflict_op,
                conflicted_files,
            },
        });
    }

    Ok(ResponseJson(ApiResponse::success(results)))
}

#[derive(serde::Deserialize, Debug, TS)]
pub struct ChangeTargetBranchRequest {
    pub repo_id: Uuid,
    pub new_target_branch: String,
}

#[derive(serde::Serialize, Debug, TS)]
pub struct ChangeTargetBranchResponse {
    pub repo_id: Uuid,
    pub new_target_branch: String,
    pub status: (usize, usize),
}

#[derive(serde::Deserialize, Debug, TS)]
pub struct RenameBranchRequest {
    pub new_branch_name: String,
}

#[derive(serde::Serialize, Debug, TS)]
pub struct RenameBranchResponse {
    pub branch: String,
}

#[derive(Debug, Serialize, Deserialize, TS)]
#[serde(tag = "type", rename_all = "snake_case")]
#[ts(tag = "type", rename_all = "snake_case")]
pub enum RenameBranchError {
    EmptyBranchName,
    InvalidBranchNameFormat,
    OpenPullRequest,
    BranchAlreadyExists { repo_name: String },
    RebaseInProgress { repo_name: String },
    RenameFailed { repo_name: String, message: String },
}

#[axum::debug_handler]
pub async fn change_target_branch(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<ChangeTargetBranchRequest>,
) -> Result<ResponseJson<ApiResponse<ChangeTargetBranchResponse>>, ApiError> {
    let repo_id = payload.repo_id;
    let new_target_branch = payload.new_target_branch;
    let pool = &deployment.db().pool;

    let repo = Repo::find_by_id(pool, repo_id)
        .await?
        .ok_or(RepoError::NotFound)?;

    if !deployment
        .git()
        .check_branch_exists(&repo.path, &new_target_branch)?
    {
        return Err(ApiError::NotFound(format!(
            "Branch '{new_target_branch}' does not exist in repository '{}'",
            repo.name
        )));
    }

    WorkspaceRepo::update_target_branch(pool, workspace.id, repo_id, &new_target_branch).await?;

    let status =
        deployment
            .git()
            .get_branch_status(&repo.path, &workspace.branch, &new_target_branch)?;

    deployment
        .track_if_analytics_allowed(
            "task_attempt_target_branch_changed",
            serde_json::json!({
                "repo_id": repo_id.to_string(),
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(
        ChangeTargetBranchResponse {
            repo_id,
            new_target_branch,
            status,
        },
    )))
}

#[axum::debug_handler]
pub async fn rename_branch(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<RenameBranchRequest>,
) -> Result<ResponseJson<ApiResponse<RenameBranchResponse, RenameBranchError>>, ApiError> {
    let new_branch_name = payload.new_branch_name.trim();

    if new_branch_name.is_empty() {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RenameBranchError::EmptyBranchName,
        )));
    }
    if !deployment.git().is_branch_name_valid(new_branch_name) {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RenameBranchError::InvalidBranchNameFormat,
        )));
    }
    if new_branch_name == workspace.branch {
        return Ok(ResponseJson(ApiResponse::success(RenameBranchResponse {
            branch: workspace.branch.clone(),
        })));
    }

    let pool = &deployment.db().pool;

    // Fail if workspace has an open PR in any repo
    let merges = Merge::find_by_workspace_id(pool, workspace.id).await?;
    let has_open_pr = merges.into_iter().any(|merge| {
        matches!(merge, Merge::Pr(pr_merge) if matches!(pr_merge.pr_info.status, MergeStatus::Open))
    });
    if has_open_pr {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RenameBranchError::OpenPullRequest,
        )));
    }

    let repos = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;
    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_dir = PathBuf::from(&container_ref);

    for repo in &repos {
        let worktree_path = workspace_dir.join(&repo.name);

        if deployment
            .git()
            .check_branch_exists(&repo.path, new_branch_name)?
        {
            return Ok(ResponseJson(ApiResponse::error_with_data(
                RenameBranchError::BranchAlreadyExists {
                    repo_name: repo.name.clone(),
                },
            )));
        }

        if deployment.git().is_rebase_in_progress(&worktree_path)? {
            return Ok(ResponseJson(ApiResponse::error_with_data(
                RenameBranchError::RebaseInProgress {
                    repo_name: repo.name.clone(),
                },
            )));
        }
    }

    // Rename all repos with rollback
    let old_branch = workspace.branch.clone();
    let mut renamed_repos: Vec<&Repo> = Vec::new();

    for repo in &repos {
        let worktree_path = workspace_dir.join(&repo.name);

        match deployment.git().rename_local_branch(
            &worktree_path,
            &workspace.branch,
            new_branch_name,
        ) {
            Ok(()) => {
                renamed_repos.push(repo);
            }
            Err(e) => {
                // Rollback already renamed repos
                for renamed_repo in &renamed_repos {
                    let rollback_path = workspace_dir.join(&renamed_repo.name);
                    if let Err(rollback_err) = deployment.git().rename_local_branch(
                        &rollback_path,
                        new_branch_name,
                        &old_branch,
                    ) {
                        tracing::error!(
                            "Failed to rollback branch rename in '{}': {}",
                            renamed_repo.name,
                            rollback_err
                        );
                    }
                }
                return Ok(ResponseJson(ApiResponse::error_with_data(
                    RenameBranchError::RenameFailed {
                        repo_name: repo.name.clone(),
                        message: e.to_string(),
                    },
                )));
            }
        }
    }

    Workspace::update_branch_name(pool, workspace.id, new_branch_name).await?;
    // What will become of me?
    let updated_children_count = WorkspaceRepo::update_target_branch_for_children_of_workspace(
        pool,
        workspace.id,
        &old_branch,
        new_branch_name,
    )
    .await?;

    if updated_children_count > 0 {
        tracing::info!(
            "Updated {} child task attempts to target new branch '{}'",
            updated_children_count,
            new_branch_name
        );
    }

    deployment
        .track_if_analytics_allowed(
            "task_attempt_branch_renamed",
            serde_json::json!({
                "updated_children": updated_children_count,
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(RenameBranchResponse {
        branch: new_branch_name.to_string(),
    })))
}

#[axum::debug_handler]
pub async fn rebase_task_attempt(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<RebaseTaskAttemptRequest>,
) -> Result<(StatusCode, ResponseJson<ApiResponse<(), GitOperationError>>), ApiError> {
    let pool = &deployment.db().pool;

    let workspace_repo =
        WorkspaceRepo::find_by_workspace_and_repo_id(pool, workspace.id, payload.repo_id)
            .await?
            .ok_or(RepoError::NotFound)?;

    let repo = Repo::find_by_id(pool, workspace_repo.repo_id)
        .await?
        .ok_or(RepoError::NotFound)?;

    let old_base_branch = payload
        .old_base_branch
        .unwrap_or_else(|| workspace_repo.target_branch.clone());
    let new_base_branch = payload
        .new_base_branch
        .unwrap_or_else(|| workspace_repo.target_branch.clone());

    if deployment
        .git()
        .check_branch_exists(&repo.path, &new_base_branch)?
    {
        WorkspaceRepo::update_target_branch(pool, workspace.id, payload.repo_id, &new_base_branch)
            .await?;
    } else {
        return Err(ApiError::NotFound(format!(
            "Branch '{new_base_branch}' does not exist in the repository"
        )));
    }

    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_path = Path::new(&container_ref);
    let worktree_path = workspace_path.join(&repo.name);

    let result = deployment.git().rebase_branch(
        &repo.path,
        &worktree_path,
        &new_base_branch,
        &old_base_branch,
        &workspace.branch.clone(),
    );
    if let Err(e) = result {
        use services::services::git::GitServiceError;
        return match e {
            GitServiceError::MergeConflicts(msg) => Ok(rebase_conflict_response(
                GitOperationError::MergeConflicts {
                    message: msg,
                    op: ConflictOp::Rebase,
                },
            )),
            GitServiceError::RebaseInProgress => Ok(rebase_conflict_response(
                GitOperationError::RebaseInProgress,
            )),
            other => Err(ApiError::GitService(other)),
        };
    }

    deployment
        .track_if_analytics_allowed(
            "task_attempt_rebased",
            serde_json::json!({
                "workspace_id": workspace.id.to_string(),
                "repo_id": payload.repo_id.to_string(),
            }),
        )
        .await;

    Ok((StatusCode::OK, ResponseJson(ApiResponse::success(()))))
}

fn rebase_conflict_response(
    error: GitOperationError,
) -> (StatusCode, ResponseJson<ApiResponse<(), GitOperationError>>) {
    (
        StatusCode::CONFLICT,
        ResponseJson(ApiResponse::<(), GitOperationError>::error_with_data(error)),
    )
}

#[axum::debug_handler]
pub async fn abort_conflicts_task_attempt(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<AbortConflictsRequest>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;

    let repo = Repo::find_by_id(pool, payload.repo_id)
        .await?
        .ok_or(RepoError::NotFound)?;

    let container_ref = deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;
    let workspace_path = Path::new(&container_ref);
    let worktree_path = workspace_path.join(&repo.name);

    deployment.git().abort_conflicts(&worktree_path)?;

    Ok(ResponseJson(ApiResponse::success(())))
}

#[axum::debug_handler]
pub async fn start_dev_server(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;

    // Get parent task
    let task = workspace
        .parent_task(&deployment.db().pool)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    // Get parent project
    let project = task
        .parent_project(&deployment.db().pool)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    // Stop any existing dev servers for this project
    let existing_dev_servers =
        match ExecutionProcess::find_running_dev_servers_by_project(pool, project.id).await {
            Ok(servers) => servers,
            Err(e) => {
                tracing::error!(
                    "Failed to find running dev servers for project {}: {}",
                    project.id,
                    e
                );
                return Err(ApiError::Workspace(WorkspaceError::ValidationError(
                    e.to_string(),
                )));
            }
        };

    for dev_server in existing_dev_servers {
        tracing::info!(
            "Stopping existing dev server {} for project {}",
            dev_server.id,
            project.id
        );

        if let Err(e) = deployment
            .container()
            .stop_execution(&dev_server, ExecutionProcessStatus::Killed)
            .await
        {
            tracing::error!("Failed to stop dev server {}: {}", dev_server.id, e);
        }
    }

    let repos = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;
    let repos_with_dev_script: Vec<_> = repos
        .iter()
        .filter(|r| r.dev_server_script.as_ref().is_some_and(|s| !s.is_empty()))
        .collect();

    if repos_with_dev_script.is_empty() {
        return Err(ApiError::BadRequest(
            "No dev server script configured for any repository in this workspace".to_string(),
        ));
    }

    let session = match Session::find_latest_by_workspace_id(pool, workspace.id).await? {
        Some(s) => s,
        None => {
            Session::create(
                pool,
                &CreateSession {
                    executor: Some("dev-server".to_string()),
                    model_config_id: None,
                },
                Uuid::new_v4(),
                workspace.id,
            )
            .await?
        }
    };

    for repo in repos_with_dev_script {
        let executor_action = ExecutorAction::new(
            ExecutorActionType::ScriptRequest(ScriptRequest {
                script: repo.dev_server_script.clone().unwrap(),
                language: ScriptRequestLanguage::Bash,
                context: ScriptContext::DevServer,
                working_dir: Some(repo.name.clone()),
            }),
            None,
        );

        deployment
            .container()
            .start_execution(
                &workspace,
                &session,
                &executor_action,
                &ExecutionProcessRunReason::DevServer,
            )
            .await?;
    }

    deployment
        .track_if_analytics_allowed(
            "dev_server_started",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "project_id": project.id.to_string(),
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(())))
}

pub async fn get_task_attempt_children(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<TaskRelationships>>, ApiError> {
    let relationships =
        Task::find_relationships_for_workspace(&deployment.db().pool, &workspace).await?;

    deployment
        .track_if_analytics_allowed(
            "task_attempt_children_viewed",
            serde_json::json!({
                "workspace_id": workspace.id.to_string(),
                "children_count": relationships.children.len(),
                "parent_count": i32::from(relationships.parent_task.is_some()),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(relationships)))
}

pub async fn stop_task_attempt_execution(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    deployment.container().try_stop(&workspace, false).await;

    deployment
        .track_if_analytics_allowed(
            "task_attempt_stopped",
            serde_json::json!({
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(())))
}

#[derive(Debug, Serialize, Deserialize, TS)]
#[serde(tag = "type", rename_all = "snake_case")]
#[ts(tag = "type", rename_all = "snake_case")]
pub enum RunScriptError {
    NoScriptConfigured,
    ProcessAlreadyRunning,
}

#[axum::debug_handler]
pub async fn run_setup_script(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<ExecutionProcess, RunScriptError>>, ApiError> {
    let pool = &deployment.db().pool;

    // Check if any non-dev-server processes are already running for this workspace
    if ExecutionProcess::has_running_non_dev_server_processes_for_workspace(pool, workspace.id)
        .await?
    {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RunScriptError::ProcessAlreadyRunning,
        )));
    }

    deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;

    let task = workspace
        .parent_task(pool)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    let project = task
        .parent_project(pool)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    let repos = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;
    let Some(executor_action) = deployment.container().setup_actions_for_repos(&repos) else {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RunScriptError::NoScriptConfigured,
        )));
    };

    // Get or create a session for setup script
    let session = match Session::find_latest_by_workspace_id(pool, workspace.id).await? {
        Some(s) => s,
        None => {
            Session::create(
                pool,
                &CreateSession {
                    executor: Some("setup-script".to_string()),
                    model_config_id: None,
                },
                Uuid::new_v4(),
                workspace.id,
            )
            .await?
        }
    };

    let execution_process = deployment
        .container()
        .start_execution(
            &workspace,
            &session,
            &executor_action,
            &ExecutionProcessRunReason::SetupScript,
        )
        .await?;

    deployment
        .track_if_analytics_allowed(
            "setup_script_executed",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "project_id": project.id.to_string(),
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(execution_process)))
}

#[axum::debug_handler]
pub async fn run_cleanup_script(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<ExecutionProcess, RunScriptError>>, ApiError> {
    let pool = &deployment.db().pool;

    // Check if any non-dev-server processes are already running for this workspace
    if ExecutionProcess::has_running_non_dev_server_processes_for_workspace(pool, workspace.id)
        .await?
    {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RunScriptError::ProcessAlreadyRunning,
        )));
    }

    deployment
        .container()
        .ensure_container_exists(&workspace)
        .await?;

    let task = workspace
        .parent_task(pool)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    let project = task
        .parent_project(pool)
        .await?
        .ok_or(SqlxError::RowNotFound)?;

    let repos = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;
    let Some(executor_action) = deployment.container().cleanup_actions_for_repos(&repos) else {
        return Ok(ResponseJson(ApiResponse::error_with_data(
            RunScriptError::NoScriptConfigured,
        )));
    };

    // Get or create a session for cleanup script
    let session = match Session::find_latest_by_workspace_id(pool, workspace.id).await? {
        Some(s) => s,
        None => {
            Session::create(
                pool,
                &CreateSession {
                    executor: Some("cleanup-script".to_string()),
                    model_config_id: None,
                },
                Uuid::new_v4(),
                workspace.id,
            )
            .await?
        }
    };

    let execution_process = deployment
        .container()
        .start_execution(
            &workspace,
            &session,
            &executor_action,
            &ExecutionProcessRunReason::CleanupScript,
        )
        .await?;

    deployment
        .track_if_analytics_allowed(
            "cleanup_script_executed",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "project_id": project.id.to_string(),
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(execution_process)))
}

#[axum::debug_handler]
pub async fn gh_cli_setup_handler(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<ExecutionProcess, GhCliSetupError>>, ApiError> {
    match gh_cli_setup::run_gh_cli_setup(&deployment, &workspace).await {
        Ok(execution_process) => {
            deployment
                .track_if_analytics_allowed(
                    "gh_cli_setup_executed",
                    serde_json::json!({
                        "workspace_id": workspace.id.to_string(),
                    }),
                )
                .await;

            Ok(ResponseJson(ApiResponse::success(execution_process)))
        }
        Err(ApiError::Executor(ExecutorError::ExecutableNotFound { program }))
            if program == "brew" =>
        {
            Ok(ResponseJson(ApiResponse::error_with_data(
                GhCliSetupError::BrewMissing,
            )))
        }
        Err(ApiError::Executor(ExecutorError::SetupHelperNotSupported)) => Ok(ResponseJson(
            ApiResponse::error_with_data(GhCliSetupError::SetupHelperNotSupported),
        )),
        Err(ApiError::Executor(err)) => Ok(ResponseJson(ApiResponse::error_with_data(
            GhCliSetupError::Other {
                message: err.to_string(),
            },
        ))),
        Err(err) => Err(err),
    }
}

pub async fn get_task_attempt_repos(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<RepoWithTargetBranch>>>, ApiError> {
    let pool = &deployment.db().pool;

    let repos =
        WorkspaceRepo::find_repos_with_target_branch_for_workspace(pool, workspace.id).await?;

    Ok(ResponseJson(ApiResponse::success(repos)))
}

pub async fn search_workspace_files(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
    Query(search_query): Query<SearchQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<SearchResult>>>, ApiError> {
    if search_query.q.trim().is_empty() {
        return Err(ApiError::BadRequest(
            "Query parameter 'q' is required and cannot be empty".to_string(),
        ));
    }

    let repos =
        WorkspaceRepo::find_repos_for_workspace(&deployment.db().pool, workspace.id).await?;

    let results = deployment
        .project()
        .search_files(
            deployment.file_search_cache().as_ref(),
            &repos,
            &search_query,
        )
        .await?;

    Ok(ResponseJson(ApiResponse::success(results)))
}

pub async fn get_first_user_message(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Option<String>>>, ApiError> {
    let pool = &deployment.db().pool;

    let message = Workspace::get_first_user_message(pool, workspace.id).await?;

    Ok(ResponseJson(ApiResponse::success(message)))
}

pub async fn delete_workspace(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<(StatusCode, ResponseJson<ApiResponse<()>>), ApiError> {
    let pool = &deployment.db().pool;

    // Check for running execution processes
    if ExecutionProcess::has_running_non_dev_server_processes_for_workspace(pool, workspace.id)
        .await?
    {
        return Err(ApiError::Conflict(
            "Cannot delete workspace while processes are running. Stop all processes first."
                .to_string(),
        ));
    }

    // Stop any running dev servers for this workspace
    let dev_servers =
        ExecutionProcess::find_running_dev_servers_by_workspace(pool, workspace.id).await?;

    for dev_server in dev_servers {
        tracing::info!(
            "Stopping dev server {} before deleting workspace {}",
            dev_server.id,
            workspace.id
        );

        if let Err(e) = deployment
            .container()
            .stop_execution(&dev_server, ExecutionProcessStatus::Killed)
            .await
        {
            tracing::error!(
                "Failed to stop dev server {} for workspace {}: {}",
                dev_server.id,
                workspace.id,
                e
            );
        }
    }

    // Gather data needed for background cleanup
    let workspace_dir = workspace.container_ref.clone().map(PathBuf::from);
    let repositories = WorkspaceRepo::find_repos_for_workspace(pool, workspace.id).await?;

    // Nullify parent_workspace_id for any child tasks before deletion
    let children_affected = Task::nullify_children_by_workspace_id(pool, workspace.id).await?;
    if children_affected > 0 {
        tracing::info!(
            "Nullified {} child task references before deleting workspace {}",
            children_affected,
            workspace.id
        );
    }

    // Delete workspace from database (FK CASCADE will handle sessions, execution_processes, etc.)
    let rows_affected = Workspace::delete(pool, workspace.id).await?;

    if rows_affected == 0 {
        return Err(ApiError::Database(SqlxError::RowNotFound));
    }

    deployment
        .track_if_analytics_allowed(
            "workspace_deleted",
            serde_json::json!({
                "workspace_id": workspace.id.to_string(),
                "task_id": workspace.task_id.to_string(),
            }),
        )
        .await;

    // Spawn background cleanup task for filesystem resources
    if let Some(workspace_dir) = workspace_dir {
        let workspace_id = workspace.id;
        tokio::spawn(async move {
            tracing::info!(
                "Starting background cleanup for workspace {} at {}",
                workspace_id,
                workspace_dir.display()
            );

            if let Err(e) = WorkspaceManager::cleanup_workspace(&workspace_dir, &repositories).await
            {
                tracing::error!(
                    "Background workspace cleanup failed for {} at {}: {}",
                    workspace_id,
                    workspace_dir.display(),
                    e
                );
            } else {
                tracing::info!(
                    "Background cleanup completed for workspace {}",
                    workspace_id
                );
            }
        });
    }

    // Return 202 Accepted to indicate deletion was scheduled
    Ok((StatusCode::ACCEPTED, ResponseJson(ApiResponse::success(()))))
}

/// Mark all coding agent turns for a workspace as seen
#[axum::debug_handler]
pub async fn mark_seen(
    Extension(workspace): Extension<Workspace>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    let pool = &deployment.db().pool;

    CodingAgentTurn::mark_seen_by_workspace_id(pool, workspace.id).await?;

    Ok(ResponseJson(ApiResponse::success(())))
}

pub fn router(deployment: &DeploymentImpl) -> Router<DeploymentImpl> {
    let task_attempt_id_router = Router::new()
        .route(
            "/",
            get(get_task_attempt)
                .put(update_workspace)
                .delete(delete_workspace),
        )
        .route("/run-agent-setup", post(run_agent_setup))
        .route("/gh-cli-setup", post(gh_cli_setup_handler))
        .route("/start-dev-server", post(start_dev_server))
        .route("/run-setup-script", post(run_setup_script))
        .route("/run-cleanup-script", post(run_cleanup_script))
        .route("/branch-status", get(get_task_attempt_branch_status))
        .route("/diff/ws", get(stream_task_attempt_diff_ws))
        .route("/merge", post(merge_task_attempt))
        .route("/push", post(push_task_attempt_branch))
        .route("/push/force", post(force_push_task_attempt_branch))
        .route("/rebase", post(rebase_task_attempt))
        .route("/conflicts/abort", post(abort_conflicts_task_attempt))
        .route("/pr", post(pr::create_pr))
        .route("/pr/attach", post(pr::attach_existing_pr))
        .route("/pr/comments", get(pr::get_pr_comments))
        .route("/open-editor", post(open_task_attempt_in_editor))
        .route("/children", get(get_task_attempt_children))
        .route("/stop", post(stop_task_attempt_execution))
        .route("/change-target-branch", post(change_target_branch))
        .route("/rename-branch", post(rename_branch))
        .route("/repos", get(get_task_attempt_repos))
        .route("/search", get(search_workspace_files))
        .route("/first-message", get(get_first_user_message))
        .route("/mark-seen", put(mark_seen))
        .layer(from_fn_with_state(
            deployment.clone(),
            load_workspace_middleware,
        ));

    let task_attempts_router = Router::new()
        .route("/", get(get_task_attempts).post(create_task_attempt))
        .route("/count", get(get_workspace_count))
        .route("/stream/ws", get(stream_workspaces_ws))
        .route("/summary", post(workspace_summary::get_workspace_summaries))
        .nest("/{id}", task_attempt_id_router)
        .nest("/{id}/images", images::router(deployment));

    Router::new().nest("/task-attempts", task_attempts_router)
}
