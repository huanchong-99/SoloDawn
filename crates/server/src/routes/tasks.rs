use std::path::PathBuf;

use anyhow;
use axum::{
    Extension, Json, Router,
    extract::{
        Query, State,
        ws::{Message, WebSocket, WebSocketUpgrade},
    },
    http::StatusCode,
    middleware::from_fn_with_state,
    response::{IntoResponse, Json as ResponseJson},
    routing::{delete, get, post, put},
};
use db::models::{
    image::TaskImage,
    repo::{Repo, RepoError},
    task::{CreateTask, Task, TaskWithAttemptStatus, UpdateTask},
    workspace::{CreateWorkspace, Workspace},
    workspace_repo::{CreateWorkspaceRepo, WorkspaceRepo},
};
use deployment::Deployment;
use executors::profile::ExecutorProfileId;
use futures_util::{SinkExt, StreamExt, TryStreamExt};
use serde::{Deserialize, Serialize};
use services::services::{container::ContainerService, workspace_manager::WorkspaceManager};
use sqlx::Error as SqlxError;
use ts_rs::TS;
use utils::response::ApiResponse;
use uuid::Uuid;

use crate::{
    DeploymentImpl, error::ApiError, middleware::load_task_middleware,
    routes::task_attempts::WorkspaceRepoInput,
};

const WS_HEARTBEAT_INTERVAL_SECS: u64 = 30;

#[derive(Debug, Serialize, Deserialize)]
pub struct TaskQuery {
    pub project_id: Uuid,
}

pub async fn get_tasks(
    State(deployment): State<DeploymentImpl>,
    Query(query): Query<TaskQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<TaskWithAttemptStatus>>>, ApiError> {
    // E29-01: Validate project_id is not nil (empty/null guard at entry).
    if query.project_id.is_nil() {
        return Err(ApiError::BadRequest(
            "project_id must not be empty".to_string(),
        ));
    }

    // TODO(W2-18-02): No auth enforcement here; ownership check must be added
    // at a higher layer (middleware) once multi-user auth lands.

    let tasks =
        Task::find_by_project_id_with_attempt_status(&deployment.db().pool, query.project_id)
            .await?;

    Ok(ResponseJson(ApiResponse::success(tasks)))
}

pub async fn stream_tasks_ws(
    ws: WebSocketUpgrade,
    State(deployment): State<DeploymentImpl>,
    Query(query): Query<TaskQuery>,
) -> impl IntoResponse {
    ws.on_upgrade(move |socket| async move {
        if let Err(e) = handle_tasks_ws(socket, deployment, query.project_id).await {
            tracing::warn!("tasks WS closed: {}", e);
        }
    })
}

async fn handle_tasks_ws(
    socket: WebSocket,
    deployment: DeploymentImpl,
    project_id: Uuid,
) -> anyhow::Result<()> {
    // Get the raw stream and convert LogMsg to WebSocket messages
    let mut stream = deployment
        .events()
        .stream_tasks_raw(project_id)
        .await?
        .map_ok(|msg| msg.to_ws_message_unchecked());

    // Split socket into sender and receiver
    let (mut sender, mut receiver) = socket.split();
    let mut heartbeat =
        tokio::time::interval(tokio::time::Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));

    loop {
        tokio::select! {
            _ = heartbeat.tick() => {
                if sender.send(Message::Ping(Vec::new().into())).await.is_err() {
                    tracing::debug!("tasks WS heartbeat send failed; closing");
                    break;
                }
            }
            item = stream.next() => {
                match item {
                    Some(Ok(msg)) => {
                        if sender.send(msg).await.is_err() {
                            tracing::debug!("tasks WS send failed; client disconnected");
                            break;
                        }
                    }
                    Some(Err(e)) => {
                        tracing::error!("tasks stream error: {}", e);
                        break;
                    }
                    None => break,
                }
            }
            msg = receiver.next() => {
                match msg {
                    Some(Ok(Message::Close(_))) => {
                        tracing::debug!("tasks WS client requested close");
                        break;
                    }
                    Some(Ok(Message::Ping(payload))) => {
                        if sender.send(Message::Pong(payload)).await.is_err() {
                            tracing::debug!("tasks WS failed to respond pong");
                            break;
                        }
                    }
                    Some(Ok(Message::Pong(_))) => {}
                    Some(Ok(_)) => {}
                    Some(Err(e)) => {
                        tracing::debug!("tasks WS receive error: {}", e);
                        break;
                    }
                    None => {
                        tracing::debug!("tasks WS receiver closed");
                        break;
                    }
                }
            }
        }
    }

    let _ = sender.send(Message::Close(None)).await;
    let _ = sender.close().await;

    Ok(())
}

pub async fn get_task(
    Extension(task): Extension<Task>,
    State(_deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Task>>, ApiError> {
    Ok(ResponseJson(ApiResponse::success(task)))
}

pub async fn create_task(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateTask>,
) -> Result<ResponseJson<ApiResponse<Task>>, ApiError> {
    let id = Uuid::new_v4();

    tracing::debug!(
        "Creating task '{}' in project {}",
        payload.title,
        payload.project_id
    );

    let task = Task::create(&deployment.db().pool, &payload, id).await?;

    if let Some(image_ids) = &payload.image_ids {
        TaskImage::associate_many_dedup(&deployment.db().pool, task.id, image_ids).await?;
    }

    deployment
        .track_if_analytics_allowed(
            "task_created",
            serde_json::json!({
            "task_id": task.id.to_string(),
            "project_id": payload.project_id,
            "has_description": task.description.is_some(),
            "has_images": payload.image_ids.is_some(),
            }),
        )
        .await;

    Ok(ResponseJson(ApiResponse::success(task)))
}

#[derive(Debug, Deserialize, TS)]
pub struct CreateAndStartTaskRequest {
    pub task: CreateTask,
    pub executor_profile_id: ExecutorProfileId,
    pub repos: Vec<WorkspaceRepoInput>,
}

pub async fn create_task_and_start(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateAndStartTaskRequest>,
) -> Result<ResponseJson<ApiResponse<TaskWithAttemptStatus>>, ApiError> {
    if payload.repos.is_empty() {
        return Err(ApiError::BadRequest(
            "At least one repository is required".to_string(),
        ));
    }

    let pool = &deployment.db().pool;

    let task_id = Uuid::new_v4();
    let task = Task::create(pool, &payload.task, task_id).await?;

    if let Some(image_ids) = &payload.task.image_ids {
        TaskImage::associate_many_dedup(pool, task.id, image_ids).await?;
    }

    deployment
        .track_if_analytics_allowed(
            "task_created",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "project_id": task.project_id,
                "has_description": task.description.is_some(),
                "has_images": payload.task.image_ids.is_some(),
            }),
        )
        .await;

    let attempt_id = Uuid::new_v4();
    let git_branch_name = deployment
        .container()
        .git_branch_from_workspace(&attempt_id, &task.title)
        .await;

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

    let workspace = Workspace::create(
        pool,
        &CreateWorkspace {
            branch: git_branch_name,
            agent_working_dir,
        },
        attempt_id,
        task.id,
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
    WorkspaceRepo::create_many(&deployment.db().pool, workspace.id, &workspace_repos).await?;

    let is_attempt_running = match deployment
        .container()
        .start_workspace(&workspace, payload.executor_profile_id.clone())
        .await
    {
        Ok(_) => true,
        Err(err) => {
            tracing::error!("Failed to start task attempt: {err}");
            false
        }
    };
    deployment
        .track_if_analytics_allowed(
            "task_attempt_started",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "executor": &payload.executor_profile_id.executor,
                "variant": &payload.executor_profile_id.variant,
                "workspace_id": workspace.id.to_string(),
            }),
        )
        .await;

    let task = Task::find_by_id(pool, task.id)
        .await?
        .ok_or_else(|| ApiError::NotFound(format!("Task {} not found", task.id)))?;

    tracing::info!("Started attempt for task {}", task.id);
    Ok(ResponseJson(ApiResponse::success(TaskWithAttemptStatus {
        task,
        has_in_progress_attempt: is_attempt_running,
        last_attempt_failed: false,
        executor: payload.executor_profile_id.executor.to_string(),
    })))
}

pub async fn update_task(
    Extension(existing_task): Extension<Task>,
    State(deployment): State<DeploymentImpl>,

    Json(payload): Json<UpdateTask>,
) -> Result<ResponseJson<ApiResponse<Task>>, ApiError> {
    ensure_shared_task_auth(&existing_task, &deployment).await?;

    // E29-09: Validate optional fields when they are supplied.
    const MAX_TITLE_LEN: usize = 256;
    const MAX_DESCRIPTION_LEN: usize = 64 * 1024;
    if let Some(ref t) = payload.title {
        if t.trim().is_empty() {
            return Err(ApiError::BadRequest(
                "title must not be empty".to_string(),
            ));
        }
        if t.len() > MAX_TITLE_LEN {
            return Err(ApiError::BadRequest(format!(
                "title must not exceed {MAX_TITLE_LEN} characters"
            )));
        }
    }
    if let Some(ref d) = payload.description {
        if d.len() > MAX_DESCRIPTION_LEN {
            return Err(ApiError::BadRequest(format!(
                "description must not exceed {MAX_DESCRIPTION_LEN} characters"
            )));
        }
    }

    // Use existing values if not provided in update
    let title = payload.title.unwrap_or(existing_task.title);
    let description = match payload.description {
        Some(s) if s.trim().is_empty() => None, // Empty string = clear description
        Some(s) => Some(s),                     // Non-empty string = update description
        None => existing_task.description,      // Field omitted = keep existing
    };
    let status = payload.status.unwrap_or(existing_task.status);
    // M38 (known limitation): `parent_workspace_id: Option<Uuid>` is a two-state field,
    // so callers cannot distinguish "leave unchanged" from "clear to NULL" — omitting the
    // field and sending `null` both deserialize to `None`, which currently overwrites the
    // existing value with NULL on every update. A proper fix requires a tri-state
    // (`Option<Option<Uuid>>` with `#[serde(default, deserialize_with = "deserialize_some")]`
    // or a `Patch<T>` wrapper) and would ripple through `Task::update`, the MCP task server
    // (`crates/server/src/mcp/task_server.rs`), generated TS types (`shared/types.ts`), and
    // the frontend (`frontend/src/lib/api.ts`, `useTaskMutations`). Deferred until those
    // call sites can be updated atomically. For now, clients MUST always send the current
    // `parent_workspace_id` value when updating a task to avoid accidentally clearing it.
    let parent_workspace_id = payload.parent_workspace_id;

    let task = Task::update(
        &deployment.db().pool,
        existing_task.id,
        existing_task.project_id,
        title,
        description,
        status,
        parent_workspace_id,
    )
    .await?;

    if let Some(image_ids) = &payload.image_ids {
        TaskImage::delete_by_task_id(&deployment.db().pool, task.id).await?;
        TaskImage::associate_many_dedup(&deployment.db().pool, task.id, image_ids).await?;
    }

    // Note: Remote sharing features have been removed
    if task.shared_task_id.is_some() {
        tracing::debug!(
            "Task {} has shared_task_id but sharing is not supported",
            task.id
        );
    }

    Ok(ResponseJson(ApiResponse::success(task)))
}

async fn ensure_shared_task_auth(
    _existing_task: &Task,
    _deployment: &local_deployment::LocalDeployment,
) -> Result<(), ApiError> {
    // Note: Remote sharing features have been removed
    Ok(())
}

pub async fn delete_task(
    Extension(task): Extension<Task>,
    State(deployment): State<DeploymentImpl>,
) -> Result<(StatusCode, ResponseJson<ApiResponse<()>>), ApiError> {
    // TODO(W2-18-08): TOCTOU between ownership check and delete.
    //
    // The `Task` extension is populated by the task-loader middleware, which
    // also performs ownership/authorization against the project scope. Between
    // that check and the `Task::delete` call below, the task row could be
    // reparented, transferred, or mutated concurrently — meaning the delete
    // executes against a row whose ownership is no longer what the middleware
    // validated.
    //
    // Fix forward: either (a) re-read-and-validate the row inside the same
    // transaction (`SELECT ... FOR UPDATE` / sqlite equivalent via `BEGIN
    // IMMEDIATE`) before `Task::delete`, or (b) push ownership into the
    // `DELETE ... WHERE id = ?1 AND project_id = ?2 AND <owner guard>` SQL
    // itself and treat `rows_affected == 0` as a 404/403. Option (b) is
    // preferred since it is atomic at the DB layer and needs no extra round
    // trip. Revisit when `Task::delete` signature is updated.
    ensure_shared_task_auth(&task, &deployment).await?;

    let pool = &deployment.db().pool;

    // Gather task attempts data needed for background cleanup
    let attempts = Workspace::fetch_all(pool, Some(task.id))
        .await
        .map_err(|e| {
            tracing::error!("Failed to fetch task attempts for task {}: {}", task.id, e);
            ApiError::Workspace(e)
        })?;

    // Stop any running execution processes before deletion
    for workspace in &attempts {
        deployment.container().try_stop(workspace, true).await;
    }

    let repositories = WorkspaceRepo::find_unique_repos_for_task(pool, task.id).await?;

    // Collect workspace directories that need cleanup
    let workspace_dirs: Vec<PathBuf> = attempts
        .iter()
        .filter_map(|attempt| attempt.container_ref.as_ref().map(PathBuf::from))
        .collect();

    // Note: Remote sharing features have been removed
    if task.shared_task_id.is_some() {
        tracing::debug!(
            "Task {} has shared_task_id but deletion will proceed locally",
            task.id
        );
    }

    // Use a transaction to ensure atomicity: either all operations succeed or all are rolled back
    //
    // TODO(E29-15): Rollback is not guaranteed on panic. sqlx::Transaction's
    // Drop impl schedules an async rollback but cannot await it, so a panic
    // mid-transaction may leave the connection in an ambiguous state. If this
    // hot path grows more complex, switch to an RAII guard that performs an
    // explicit rollback-on-drop via block_in_place / spawn_blocking, or move
    // the whole body behind an explicit commit helper that aborts the task on
    // any error path. Current code only has two awaited statements between
    // begin() and commit(), so panic risk is low but non-zero.
    let mut tx = pool.begin().await?;

    // Nullify parent_workspace_id for all child tasks before deletion
    // This breaks parent-child relationships to avoid foreign key constraint violations
    let mut total_children_affected = 0u64;
    for attempt in &attempts {
        let children_affected =
            Task::nullify_children_by_workspace_id(&mut *tx, attempt.id).await?;
        total_children_affected += children_affected;
    }

    // Delete task from database (FK CASCADE will handle task_attempts)
    let rows_affected = Task::delete(&mut *tx, task.id).await?;

    if rows_affected == 0 {
        return Err(ApiError::Database(SqlxError::RowNotFound));
    }

    // Commit the transaction - if this fails, all changes are rolled back
    tx.commit().await?;

    if total_children_affected > 0 {
        tracing::info!(
            "Nullified {} child task references before deleting task {}",
            total_children_affected,
            task.id
        );
    }

    deployment
        .track_if_analytics_allowed(
            "task_deleted",
            serde_json::json!({
                "task_id": task.id.to_string(),
                "project_id": task.project_id.to_string(),
                "attempt_count": attempts.len(),
            }),
        )
        .await;

    let task_id = task.id;
    let pool = pool.clone();
    tokio::spawn(async move {
        tracing::info!(
            "Starting background cleanup for task {} ({} workspaces, {} repos)",
            task_id,
            workspace_dirs.len(),
            repositories.len()
        );

        for workspace_dir in &workspace_dirs {
            if let Err(e) = WorkspaceManager::cleanup_workspace(workspace_dir, &repositories).await
            {
                tracing::error!(
                    "Background workspace cleanup failed for task {} at {}: {}",
                    task_id,
                    workspace_dir.display(),
                    e
                );
            }
        }

        match Repo::delete_orphaned(&pool).await {
            Ok(count) if count > 0 => {
                tracing::info!("Deleted {} orphaned repo records", count);
            }
            Err(e) => {
                tracing::error!("Failed to delete orphaned repos: {}", e);
            }
            _ => {}
        }

        tracing::info!("Background cleanup completed for task {}", task_id);
    });

    // Return 202 Accepted to indicate deletion was scheduled
    Ok((StatusCode::ACCEPTED, ResponseJson(ApiResponse::success(()))))
}

#[derive(Debug, Serialize, Deserialize, TS)]
pub struct ShareTaskResponse {
    pub shared_task_id: Uuid,
}

pub async fn share_task(
    Extension(_task): Extension<Task>,
    State(_deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<ShareTaskResponse>>, ApiError> {
    Err(ApiError::BadRequest(
        "Remote task sharing is not supported in this version.".to_string(),
    ))
}

pub fn router(deployment: &DeploymentImpl) -> Router<DeploymentImpl> {
    let task_actions_router = Router::new()
        .route("/", put(update_task))
        .route("/", delete(delete_task))
        .route("/share", post(share_task));

    let task_id_router = Router::new()
        .route("/", get(get_task))
        .merge(task_actions_router)
        .layer(from_fn_with_state(deployment.clone(), load_task_middleware));

    let inner = Router::new()
        .route("/", get(get_tasks).post(create_task))
        .route("/stream/ws", get(stream_tasks_ws))
        .route("/create-and-start", post(create_task_and_start))
        .nest("/{task_id}", task_id_router);

    // mount under /projects/:project_id/tasks
    Router::new().nest("/tasks", inner)
}
