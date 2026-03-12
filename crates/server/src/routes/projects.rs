use std::path::{Component, Path as FsPath, PathBuf};

use anyhow;
use axum::{
    Extension, Json, Router,
    extract::{
        Path, Query, State,
        ws::{Message, WebSocket, WebSocketUpgrade},
    },
    http::StatusCode,
    middleware::from_fn_with_state,
    response::{IntoResponse, Json as ResponseJson},
    routing::{get, post},
};
use db::models::{
    project::{CreateProject, Project, ProjectError, SearchResult, UpdateProject},
    project_repo::{CreateProjectRepo, ProjectRepo},
    repo::Repo,
};
use deployment::Deployment;
use futures_util::{SinkExt, StreamExt, TryStreamExt};
use serde::{Deserialize, Serialize};
use services::services::{file_search::SearchQuery, project::ProjectServiceError};
use ts_rs::TS;
use utils::{
    api::projects::{RemoteProject, RemoteProjectMembersResponse},
    response::ApiResponse,
};
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError, middleware::load_project_middleware};

const WS_HEARTBEAT_INTERVAL_SECS: u64 = 30;

#[derive(Deserialize, TS)]
pub struct LinkToExistingRequest {
    pub remote_project_id: Uuid,
}

#[derive(Deserialize, TS)]
pub struct CreateRemoteProjectRequest {
    pub organization_id: Uuid,
    pub name: String,
}

#[derive(Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct ResolveProjectByPathRequest {
    pub path: String,
}

#[derive(Serialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct ResolveProjectByPathResponse {
    pub project_id: String,
}

pub async fn get_projects(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<Project>>>, ApiError> {
    let projects = Project::find_all(&deployment.db().pool).await?;
    Ok(ResponseJson(ApiResponse::success(projects)))
}

pub async fn stream_projects_ws(
    ws: WebSocketUpgrade,
    State(deployment): State<DeploymentImpl>,
) -> impl IntoResponse {
    ws.on_upgrade(move |socket| async move {
        if let Err(e) = handle_projects_ws(socket, deployment).await {
            tracing::warn!("projects WS closed: {}", e);
        }
    })
}

async fn handle_projects_ws(socket: WebSocket, deployment: DeploymentImpl) -> anyhow::Result<()> {
    let mut stream = deployment
        .events()
        .stream_projects_raw()
        .await?
        .map_ok(|msg| msg.to_ws_message_unchecked());

    // Split socket into sender and receiver
    let (mut sender, mut receiver) = socket.split();
    let mut heartbeat =
        tokio::time::interval(tokio::time::Duration::from_secs(WS_HEARTBEAT_INTERVAL_SECS));
    let mut client_closed = false;

    loop {
        tokio::select! {
            _ = heartbeat.tick() => {
                if sender.send(Message::Ping(Vec::new().into())).await.is_err() {
                    tracing::debug!("projects WS heartbeat send failed; closing");
                    client_closed = true;
                    break;
                }
            }
            item = stream.next() => {
                match item {
                    Some(Ok(msg)) => {
                        if sender.send(msg).await.is_err() {
                            tracing::debug!("projects WS send failed; client disconnected");
                            client_closed = true;
                            break;
                        }
                    }
                    Some(Err(e)) => {
                        tracing::error!("projects stream error: {}", e);
                        break;
                    }
                    None => break,
                }
            }
            msg = receiver.next() => {
                match msg {
                    Some(Ok(Message::Close(_))) => {
                        tracing::debug!("projects WS client requested close");
                        client_closed = true;
                        break;
                    }
                    Some(Ok(Message::Ping(payload))) => {
                        if sender.send(Message::Pong(payload)).await.is_err() {
                            tracing::debug!("projects WS failed to respond pong");
                            break;
                        }
                    }
                    Some(Ok(Message::Pong(_))) => {}
                    Some(Ok(_)) => {}
                    Some(Err(e)) => {
                        tracing::debug!("projects WS receive error: {}", e);
                        client_closed = true;
                        break;
                    }
                    None => {
                        tracing::debug!("projects WS receiver closed");
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

pub async fn get_project(
    Extension(project): Extension<Project>,
) -> Result<ResponseJson<ApiResponse<Project>>, ApiError> {
    Ok(ResponseJson(ApiResponse::success(project)))
}

pub async fn link_project_to_existing_remote(
    Extension(_project): Extension<Project>,
    State(_deployment): State<DeploymentImpl>,
    Json(_payload): Json<LinkToExistingRequest>,
) -> Result<ResponseJson<ApiResponse<Project>>, ApiError> {
    Err(ApiError::BadRequest(
        "Remote project linking is not supported in this version.".to_string(),
    ))
}

pub async fn create_and_link_remote_project(
    Extension(_project): Extension<Project>,
    State(_deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateRemoteProjectRequest>,
) -> Result<ResponseJson<ApiResponse<Project>>, ApiError> {
    let _repo_name = payload.name.trim().to_string();
    Err(ApiError::BadRequest(
        "Remote project creation is not supported in this version.".to_string(),
    ))
}

pub async fn unlink_project(
    Extension(project): Extension<Project>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Project>>, ApiError> {
    let updated_project = deployment
        .project()
        .unlink_from_remote(&deployment.db().pool, &project)
        .await?;

    Ok(ResponseJson(ApiResponse::success(updated_project)))
}

pub async fn get_remote_project_by_id(
    State(_deployment): State<DeploymentImpl>,
    Path(_remote_project_id): Path<Uuid>,
) -> Result<ResponseJson<ApiResponse<RemoteProject>>, ApiError> {
    Err(ApiError::BadRequest(
        "Remote project features are not supported in this version.".to_string(),
    ))
}

pub async fn get_project_remote_members(
    State(_deployment): State<DeploymentImpl>,
    Extension(_project): Extension<Project>,
) -> Result<ResponseJson<ApiResponse<RemoteProjectMembersResponse>>, ApiError> {
    Err(ApiError::BadRequest(
        "Remote project features are not supported in this version.".to_string(),
    ))
}
#[allow(dead_code)]
async fn apply_remote_project_link(
    deployment: &DeploymentImpl,
    project: Project,
    remote_project: RemoteProject,
) -> Result<Project, ApiError> {
    if project.remote_project_id.is_some() {
        return Err(ApiError::Conflict(
            "Project is already linked to a remote project. Unlink it first.".to_string(),
        ));
    }

    let updated_project = deployment
        .project()
        .link_to_remote(&deployment.db().pool, project.id, remote_project)
        .await?;

    deployment
        .track_if_analytics_allowed(
            "project_linked_to_remote",
            serde_json::json!({
                "project_id": project.id.to_string(),
            }),
        )
        .await;

    Ok(updated_project)
}

pub async fn create_project(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateProject>,
) -> Result<ResponseJson<ApiResponse<Project>>, ApiError> {
    tracing::debug!("Creating project '{}'", payload.name);
    let repo_count = payload.repositories.len();

    match deployment
        .project()
        .create_project(&deployment.db().pool, deployment.repo(), payload)
        .await
    {
        Ok(project) => {
            // Track project creation event
            deployment
                .track_if_analytics_allowed(
                    "project_created",
                    serde_json::json!({
                        "project_id": project.id.to_string(),
                        "repository_count": repo_count,
                        "trigger": "manual",
                    }),
                )
                .await;

            Ok(ResponseJson(ApiResponse::success(project)))
        }
        Err(ProjectServiceError::DuplicateGitRepoPath) => Ok(ResponseJson(ApiResponse::error(
            "Duplicate repository path provided",
        ))),
        Err(ProjectServiceError::DuplicateRepositoryName) => Ok(ResponseJson(ApiResponse::error(
            "Duplicate repository name provided",
        ))),
        Err(ProjectServiceError::PathNotFound(_)) => Ok(ResponseJson(ApiResponse::error(
            "The specified path does not exist",
        ))),
        Err(ProjectServiceError::PathNotDirectory(_)) => Ok(ResponseJson(ApiResponse::error(
            "The specified path is not a directory",
        ))),
        Err(ProjectServiceError::NotGitRepository(_)) => Ok(ResponseJson(ApiResponse::error(
            "The specified directory is not a git repository",
        ))),
        Err(e) => Err(ProjectError::CreateFailed(e.to_string()).into()),
    }
}

/// Resolve a project by repository path.
/// If a project with the given repo path exists, returns its ID.
/// Otherwise, creates a new project with the path and returns the new ID.
pub async fn resolve_project_by_path(
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<ResolveProjectByPathRequest>,
) -> Result<ResponseJson<ApiResponse<ResolveProjectByPathResponse>>, ApiError> {
    let path = payload.path.trim();
    if path.is_empty() {
        return Err(ApiError::BadRequest("path is required".to_string()));
    }

    // Normalize the path
    let normalized_path = deployment
        .repo()
        .normalize_path(path)
        .map_err(|e| ApiError::BadRequest(format!("Invalid path: {e}")))?;

    let normalized_path_str = normalized_path.to_string_lossy().to_string();

    // Check if repo exists and has an associated project
    if let Some(repo) = Repo::find_by_path(&deployment.db().pool, &normalized_path_str).await? {
        let project_ids =
            ProjectRepo::find_project_ids_by_repo_id(&deployment.db().pool, repo.id).await?;
        if let Some(project_id) = project_ids.first() {
            tracing::debug!(
                "Found existing project {} for path {}",
                project_id,
                normalized_path_str
            );
            return Ok(ResponseJson(ApiResponse::success(
                ResolveProjectByPathResponse {
                    project_id: project_id.to_string(),
                },
            )));
        }
    }

    // No existing project found, create a new one
    let name = normalized_path
        .file_name()
        .and_then(|name| name.to_str())
        .filter(|name| !name.trim().is_empty())
        .unwrap_or("untitled")
        .to_string();

    tracing::debug!(
        "Creating new project '{}' for path {}",
        name,
        normalized_path_str
    );

    let create_payload = CreateProject {
        name: name.clone(),
        repositories: vec![CreateProjectRepo {
            display_name: name.clone(),
            git_repo_path: normalized_path_str.clone(),
        }],
    };

    let project = deployment
        .project()
        .create_project(&deployment.db().pool, deployment.repo(), create_payload)
        .await
        .map_err(|e| ApiError::BadRequest(format!("Failed to create project: {e}")))?;

    Ok(ResponseJson(ApiResponse::success(
        ResolveProjectByPathResponse {
            project_id: project.id.to_string(),
        },
    )))
}

pub async fn update_project(
    Extension(existing_project): Extension<Project>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<UpdateProject>,
) -> Result<ResponseJson<ApiResponse<Project>>, StatusCode> {
    match deployment
        .project()
        .update_project(&deployment.db().pool, &existing_project, payload)
        .await
    {
        Ok(project) => Ok(ResponseJson(ApiResponse::success(project))),
        Err(e) => {
            tracing::error!("Failed to update project: {}", e);
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
    }
}

pub async fn delete_project(
    Extension(project): Extension<Project>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<()>>, StatusCode> {
    match deployment
        .project()
        .delete_project(&deployment.db().pool, project.id)
        .await
    {
        Ok(rows_affected) => {
            if rows_affected == 0 {
                Err(StatusCode::NOT_FOUND)
            } else {
                deployment
                    .track_if_analytics_allowed(
                        "project_deleted",
                        serde_json::json!({
                            "project_id": project.id.to_string(),
                        }),
                    )
                    .await;

                Ok(ResponseJson(ApiResponse::success(())))
            }
        }
        Err(e) => {
            tracing::error!("Failed to delete project: {}", e);
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
    }
}

#[derive(serde::Deserialize)]
pub struct OpenEditorRequest {
    #[serde(default)]
    pub editor_type: Option<String>,
    #[serde(default)]
    pub file_path: Option<String>,
    #[serde(default)]
    pub git_repo_path: Option<String>,
}

#[derive(Debug, serde::Serialize, ts_rs::TS)]
pub struct OpenEditorResponse {
    pub url: Option<String>,
}

fn normalize_editor_repo_path(path: &str) -> String {
    path.replace('\\', "/").trim_end_matches('/').to_string()
}

fn resolve_project_repo_for_editor<'a>(
    repositories: &'a [Repo],
    requested_repo_path: Option<&str>,
) -> Result<&'a Repo, ApiError> {
    let default_repo = repositories
        .first()
        .ok_or_else(|| ApiError::BadRequest("Project has no repositories".to_string()))?;

    let Some(requested_repo_path) = requested_repo_path else {
        return Ok(default_repo);
    };

    let requested_repo_path = normalize_editor_repo_path(requested_repo_path);
    repositories
        .iter()
        .find(|repo| {
            normalize_editor_repo_path(&repo.path.to_string_lossy()) == requested_repo_path
                || repo.name == requested_repo_path
        })
        .ok_or_else(|| {
            ApiError::BadRequest("Requested repository is not part of this project".to_string())
        })
}

fn resolve_repo_file_path_for_editor(
    repo_path: &FsPath,
    file_path: &str,
) -> Result<PathBuf, ApiError> {
    let trimmed_file_path = file_path.trim();
    if trimmed_file_path.is_empty() {
        return Ok(repo_path.to_path_buf());
    }

    let relative_path = PathBuf::from(trimmed_file_path);
    if relative_path.is_absolute() {
        return Err(ApiError::BadRequest(
            "file_path must be relative to the repository root".to_string(),
        ));
    }

    if relative_path.components().any(|component| {
        matches!(
            component,
            Component::ParentDir | Component::Prefix(_) | Component::RootDir
        )
    }) {
        return Err(ApiError::BadRequest(
            "file_path must stay within the selected repository".to_string(),
        ));
    }

    Ok(repo_path.join(relative_path))
}

fn resolve_editor_target_file_hint(
    path: &FsPath,
    fallback_is_file: bool,
) -> Result<bool, ApiError> {
    match std::fs::metadata(path) {
        Ok(metadata) if metadata.is_file() => Ok(true),
        Ok(metadata) if metadata.is_dir() => Ok(false),
        Ok(_) => Err(ApiError::BadRequest(
            "open-editor target must be a file or directory".to_string(),
        )),
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => Ok(fallback_is_file),
        Err(err) => Err(ApiError::Io(err)),
    }
}

pub async fn open_project_in_editor(
    Extension(project): Extension<Project>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<Option<OpenEditorRequest>>,
) -> Result<ResponseJson<ApiResponse<OpenEditorResponse>>, ApiError> {
    let repositories = deployment
        .project()
        .get_repositories(&deployment.db().pool, project.id)
        .await?;

    let selected_repo = resolve_project_repo_for_editor(
        &repositories,
        payload
            .as_ref()
            .and_then(|request| request.git_repo_path.as_deref()),
    )?;

    let file_path = payload
        .as_ref()
        .and_then(|request| request.file_path.as_deref())
        .filter(|value| !value.trim().is_empty());

    let (path, is_file_hint) = if let Some(file_path) = file_path {
        let path = resolve_repo_file_path_for_editor(selected_repo.path.as_path(), file_path)?;
        let is_file_hint = resolve_editor_target_file_hint(path.as_path(), true)?;
        (path, is_file_hint)
    } else {
        let path = selected_repo.path.clone();
        let is_file_hint = resolve_editor_target_file_hint(path.as_path(), false)?;
        (path, is_file_hint)
    };

    let editor_config = {
        let config = deployment.config().read().await;
        let editor_type_str = payload.as_ref().and_then(|req| req.editor_type.as_deref());
        config.editor.with_override(editor_type_str)
    };

    match editor_config
        .open_file_with_hint(&path, Some(is_file_hint))
        .await
    {
        Ok(url) => {
            tracing::info!(
                "Opened editor for project {} at path: {}{}",
                project.id,
                path.to_string_lossy(),
                if url.is_some() { " (remote mode)" } else { "" }
            );

            deployment
                .track_if_analytics_allowed(
                    "project_editor_opened",
                    serde_json::json!({
                        "project_id": project.id.to_string(),
                        "editor_type": payload.as_ref().and_then(|req| req.editor_type.as_ref()),
                        "remote_mode": url.is_some(),
                    }),
                )
                .await;

            Ok(ResponseJson(ApiResponse::success(OpenEditorResponse {
                url,
            })))
        }
        Err(e) => {
            tracing::error!("Failed to open editor for project {}: {:?}", project.id, e);
            Err(ApiError::EditorOpen(e))
        }
    }
}

pub async fn search_project_files(
    State(deployment): State<DeploymentImpl>,
    Extension(project): Extension<Project>,
    Query(search_query): Query<SearchQuery>,
) -> Result<ResponseJson<ApiResponse<Vec<SearchResult>>>, StatusCode> {
    if search_query.q.trim().is_empty() {
        return Ok(ResponseJson(ApiResponse::error(
            "Query parameter 'q' is required and cannot be empty",
        )));
    }

    let repositories = match deployment
        .project()
        .get_repositories(&deployment.db().pool, project.id)
        .await
    {
        Ok(repos) => repos,
        Err(e) => {
            tracing::error!("Failed to get repositories: {}", e);
            return Err(StatusCode::INTERNAL_SERVER_ERROR);
        }
    };

    match deployment
        .project()
        .search_files(
            deployment.file_search_cache().as_ref(),
            &repositories,
            &search_query,
        )
        .await
    {
        Ok(results) => Ok(ResponseJson(ApiResponse::success(results))),
        Err(e) => {
            tracing::error!("Failed to search files: {}", e);
            Err(StatusCode::INTERNAL_SERVER_ERROR)
        }
    }
}

pub async fn get_project_repositories(
    Extension(project): Extension<Project>,
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<Vec<Repo>>>, ApiError> {
    let repositories = deployment
        .project()
        .get_repositories(&deployment.db().pool, project.id)
        .await?;
    Ok(ResponseJson(ApiResponse::success(repositories)))
}

pub async fn add_project_repository(
    Extension(project): Extension<Project>,
    State(deployment): State<DeploymentImpl>,
    Json(payload): Json<CreateProjectRepo>,
) -> Result<ResponseJson<ApiResponse<Repo>>, ApiError> {
    tracing::debug!(
        "Adding repository '{}' to project {} (path: {})",
        payload.display_name,
        project.id,
        payload.git_repo_path
    );

    match deployment
        .project()
        .add_repository(
            &deployment.db().pool,
            deployment.repo(),
            project.id,
            &payload,
        )
        .await
    {
        Ok(repository) => {
            deployment
                .track_if_analytics_allowed(
                    "project_repository_added",
                    serde_json::json!({
                        "project_id": project.id.to_string(),
                        "repository_id": repository.id.to_string(),
                    }),
                )
                .await;

            Ok(ResponseJson(ApiResponse::success(repository)))
        }
        Err(ProjectServiceError::PathNotFound(_)) => {
            tracing::warn!(
                "Failed to add repository to project {}: path does not exist",
                project.id
            );
            Ok(ResponseJson(ApiResponse::error(
                "The specified path does not exist",
            )))
        }
        Err(ProjectServiceError::PathNotDirectory(_)) => {
            tracing::warn!(
                "Failed to add repository to project {}: path is not a directory",
                project.id
            );
            Ok(ResponseJson(ApiResponse::error(
                "The specified path is not a directory",
            )))
        }
        Err(ProjectServiceError::NotGitRepository(_)) => {
            tracing::warn!(
                "Failed to add repository to project {}: not a git repository",
                project.id
            );
            Ok(ResponseJson(ApiResponse::error(
                "The specified directory is not a git repository",
            )))
        }
        Err(ProjectServiceError::DuplicateRepositoryName) => {
            tracing::warn!(
                "Failed to add repository to project {}: duplicate repository name",
                project.id
            );
            Ok(ResponseJson(ApiResponse::error(
                "A repository with this name already exists in the project",
            )))
        }
        Err(ProjectServiceError::DuplicateGitRepoPath) => {
            tracing::warn!(
                "Failed to add repository to project {}: duplicate repository path",
                project.id
            );
            Ok(ResponseJson(ApiResponse::error(
                "A repository with this path already exists in the project",
            )))
        }
        Err(e) => Err(e.into()),
    }
}

pub async fn delete_project_repository(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, repo_id)): Path<(Uuid, Uuid)>,
) -> Result<ResponseJson<ApiResponse<()>>, ApiError> {
    tracing::debug!(
        "Removing repository {} from project {}",
        repo_id,
        project_id
    );

    match deployment
        .project()
        .delete_repository(&deployment.db().pool, project_id, repo_id)
        .await
    {
        Ok(()) => {
            deployment
                .track_if_analytics_allowed(
                    "project_repository_removed",
                    serde_json::json!({
                        "project_id": project_id.to_string(),
                        "repository_id": repo_id.to_string(),
                    }),
                )
                .await;

            Ok(ResponseJson(ApiResponse::success(())))
        }
        Err(ProjectServiceError::RepositoryNotFound) => {
            tracing::warn!(
                "Failed to remove repository {} from project {}: not found",
                repo_id,
                project_id
            );
            Ok(ResponseJson(ApiResponse::error("Repository not found")))
        }
        Err(e) => Err(e.into()),
    }
}

pub async fn get_project_repository(
    State(deployment): State<DeploymentImpl>,
    Path((project_id, repo_id)): Path<(Uuid, Uuid)>,
) -> Result<ResponseJson<ApiResponse<ProjectRepo>>, ApiError> {
    match ProjectRepo::find_by_project_and_repo(&deployment.db().pool, project_id, repo_id).await {
        Ok(Some(project_repo)) => Ok(ResponseJson(ApiResponse::success(project_repo))),
        Ok(None) => Err(ApiError::BadRequest(
            "Repository not found in project".to_string(),
        )),
        Err(e) => Err(e.into()),
    }
}

pub fn router(deployment: &DeploymentImpl) -> Router<DeploymentImpl> {
    let project_id_router = Router::new()
        .route(
            "/",
            get(get_project).put(update_project).delete(delete_project),
        )
        .route("/remote/members", get(get_project_remote_members))
        .route("/search", get(search_project_files))
        .route("/open-editor", post(open_project_in_editor))
        .route(
            "/link",
            post(link_project_to_existing_remote).delete(unlink_project),
        )
        .route("/link/create", post(create_and_link_remote_project))
        .route(
            "/repositories",
            get(get_project_repositories).post(add_project_repository),
        )
        .layer(from_fn_with_state(
            deployment.clone(),
            load_project_middleware,
        ));

    let projects_router = Router::new()
        .route("/", get(get_projects).post(create_project))
        .route("/resolve-by-path", post(resolve_project_by_path))
        .route(
            "/{project_id}/repositories/{repo_id}",
            get(get_project_repository).delete(delete_project_repository),
        )
        .route("/stream/ws", get(stream_projects_ws))
        .nest("/{id}", project_id_router);

    Router::new().nest("/projects", projects_router).route(
        "/remote-projects/{remote_project_id}",
        get(get_remote_project_by_id),
    )
}

#[cfg(test)]
mod open_editor_path_tests {
    use std::path::Path;

    use db::models::repo::Repo;
    use tempfile::tempdir;
    use uuid::Uuid;

    use super::{
        normalize_editor_repo_path, resolve_editor_target_file_hint,
        resolve_project_repo_for_editor, resolve_repo_file_path_for_editor,
    };

    fn repo(path: &str, name: &str) -> Repo {
        Repo {
            id: Uuid::nil(),
            path: path.into(),
            name: name.to_string(),
            display_name: name.to_string(),
            setup_script: None,
            cleanup_script: None,
            copy_files: None,
            parallel_setup_script: false,
            dev_server_script: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
        }
    }

    #[test]
    fn resolves_project_repo_by_normalized_git_repo_path() {
        let repositories = vec![repo(r"C:\work\repo-a", "repo-a")];

        let resolved = resolve_project_repo_for_editor(&repositories, Some("C:/work/repo-a/"))
            .expect("repo should resolve");

        assert_eq!(resolved.name, "repo-a");
    }

    #[test]
    fn rejects_parent_dir_file_path_for_project_open_editor() {
        let result = resolve_repo_file_path_for_editor(Path::new("/repo"), "../outside");
        assert!(result.is_err(), "parent traversal must be rejected");
    }

    #[test]
    fn normalizes_repo_path_slashes_and_trailing_separator() {
        let normalized = normalize_editor_repo_path(r"C:\work\repo-a\");
        assert_eq!(normalized, "C:/work/repo-a");
    }

    #[test]
    fn resolves_directory_hint_from_existing_directory_path() {
        let temp = tempdir().expect("temp dir");

        let is_file = resolve_editor_target_file_hint(temp.path(), true).expect("hint");

        assert!(
            !is_file,
            "existing directories must not be treated as files"
        );
    }

    #[test]
    fn falls_back_to_file_hint_for_non_existing_path() {
        let temp = tempdir().expect("temp dir");
        let missing_path = temp.path().join("missing-file.ts");

        let is_file = resolve_editor_target_file_hint(missing_path.as_path(), true).expect("hint");

        assert!(
            is_file,
            "missing targets should keep fallback file hint for remote semantics"
        );
    }
}
