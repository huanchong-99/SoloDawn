use axum::{
    Json,
    extract::multipart::MultipartError,
    http::StatusCode,
    response::{IntoResponse, Response},
};
use db::models::{
    execution_process::ExecutionProcessError, project::ProjectError,
    project_repo::ProjectRepoError, repo::RepoError, scratch::ScratchError, session::SessionError,
    workspace::WorkspaceError,
};
use deployment::DeploymentError;
use executors::executors::ExecutorError;
use git2::Error as Git2Error;
use services::services::{
    config::{ConfigError, EditorOpenError},
    container::ContainerError,
    git::GitServiceError,
    git_host::GitHostError,
    image::ImageError,
    project::ProjectServiceError,
    repo::RepoError as RepoServiceError,
    worktree_manager::WorktreeError,
};
use thiserror::Error;
use utils::response::ApiResponse;

#[derive(Debug, Error, ts_rs::TS)]
#[ts(type = "string")]
pub enum ApiError {
    #[error(transparent)]
    Project(#[from] ProjectError),
    #[error(transparent)]
    Repo(#[from] RepoError),
    #[error(transparent)]
    Workspace(#[from] WorkspaceError),
    #[error(transparent)]
    Session(#[from] SessionError),
    #[error(transparent)]
    ScratchError(#[from] ScratchError),
    #[error(transparent)]
    ExecutionProcess(#[from] ExecutionProcessError),
    #[error(transparent)]
    GitService(#[from] GitServiceError),
    #[error(transparent)]
    GitHost(#[from] GitHostError),
    #[error(transparent)]
    Deployment(#[from] DeploymentError),
    #[error(transparent)]
    Container(#[from] ContainerError),
    #[error(transparent)]
    Executor(#[from] ExecutorError),
    #[error(transparent)]
    Database(#[from] sqlx::Error),
    #[error(transparent)]
    Worktree(#[from] WorktreeError),
    #[error(transparent)]
    Config(#[from] ConfigError),
    #[error(transparent)]
    Image(#[from] ImageError),
    #[error("Multipart error: {0}")]
    Multipart(#[from] MultipartError),
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
    #[error(transparent)]
    EditorOpen(#[from] EditorOpenError),
    #[error("Unauthorized")]
    Unauthorized,
    #[error("Internal error: {0}")]
    Internal(String),
    #[error("Not found: {0}")]
    NotFound(String),
    #[error("Bad request: {0}")]
    BadRequest(String),
    #[error("Conflict: {0}")]
    Conflict(String),
    #[error("Forbidden: {0}")]
    Forbidden(String),
}

impl From<&'static str> for ApiError {
    fn from(msg: &'static str) -> Self {
        ApiError::BadRequest(msg.to_string())
    }
}

impl From<Git2Error> for ApiError {
    fn from(err: Git2Error) -> Self {
        ApiError::GitService(GitServiceError::from(err))
    }
}

impl IntoResponse for ApiError {
    fn into_response(self) -> Response {
        let (status_code, error_type) = match &self {
            ApiError::Project(project_err) => match project_err {
                ProjectError::ProjectNotFound => (StatusCode::NOT_FOUND, "ProjectError"),
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "ProjectError"),
            },
            ApiError::Repo(repo_err) => match repo_err {
                RepoError::NotFound => (StatusCode::NOT_FOUND, "ProjectRepoError"),
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "ProjectRepoError"),
            },
            ApiError::Workspace(workspace_err) => match workspace_err {
                WorkspaceError::TaskNotFound
                | WorkspaceError::ProjectNotFound
                | WorkspaceError::BranchNotFound(_)
                | WorkspaceError::Database(sqlx::Error::RowNotFound) => {
                    (StatusCode::NOT_FOUND, "WorkspaceError")
                }
                WorkspaceError::ValidationError(_) => (StatusCode::BAD_REQUEST, "WorkspaceError"),
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "WorkspaceError"),
            },
            ApiError::Session(session_err) => match session_err {
                SessionError::NotFound
                | SessionError::WorkspaceNotFound
                | SessionError::Database(sqlx::Error::RowNotFound) => {
                    (StatusCode::NOT_FOUND, "SessionError")
                }
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "SessionError"),
            },
            ApiError::ScratchError(_) => (StatusCode::INTERNAL_SERVER_ERROR, "ScratchError"),
            ApiError::ExecutionProcess(err) => match err {
                ExecutionProcessError::ExecutionProcessNotFound => {
                    (StatusCode::NOT_FOUND, "ExecutionProcessError")
                }
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "ExecutionProcessError"),
            },
            // Promote certain GitService errors to conflict status with concise messages
            ApiError::GitService(git_err) => match git_err {
                services::services::git::GitServiceError::MergeConflicts(_)
                | services::services::git::GitServiceError::RebaseInProgress => {
                    (StatusCode::CONFLICT, "GitServiceError")
                }
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "GitServiceError"),
            },
            ApiError::GitHost(_) => (StatusCode::INTERNAL_SERVER_ERROR, "GitHostError"),
            ApiError::Deployment(_) => (StatusCode::INTERNAL_SERVER_ERROR, "DeploymentError"),
            ApiError::Container(container_err) => match container_err {
                ContainerError::Sqlx(sqlx::Error::RowNotFound) => {
                    (StatusCode::NOT_FOUND, "ContainerError")
                }
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "ContainerError"),
            },
            ApiError::Executor(_) => (StatusCode::INTERNAL_SERVER_ERROR, "ExecutorError"),
            ApiError::Database(db_err) => match db_err {
                sqlx::Error::RowNotFound => (StatusCode::NOT_FOUND, "DatabaseError"),
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "DatabaseError"),
            },
            ApiError::Worktree(_) => (StatusCode::INTERNAL_SERVER_ERROR, "WorktreeError"),
            ApiError::Config(config_err) => match config_err {
                ConfigError::ValidationError(_) => (StatusCode::BAD_REQUEST, "ConfigError"),
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "ConfigError"),
            },
            ApiError::Image(img_err) => match img_err {
                ImageError::InvalidFormat => (StatusCode::BAD_REQUEST, "InvalidImageFormat"),
                ImageError::TooLarge(_, _) => (StatusCode::PAYLOAD_TOO_LARGE, "ImageTooLarge"),
                ImageError::NotFound => (StatusCode::NOT_FOUND, "ImageNotFound"),
                _ => (StatusCode::INTERNAL_SERVER_ERROR, "ImageError"),
            },
            ApiError::Io(_) => (StatusCode::INTERNAL_SERVER_ERROR, "IoError"),
            ApiError::EditorOpen(err) => match err {
                EditorOpenError::LaunchFailed { .. } => {
                    (StatusCode::INTERNAL_SERVER_ERROR, "EditorLaunchError")
                }
                _ => (StatusCode::BAD_REQUEST, "EditorOpenError"),
            },
            ApiError::Multipart(_) => (StatusCode::BAD_REQUEST, "MultipartError"),
            ApiError::Unauthorized => (StatusCode::UNAUTHORIZED, "Unauthorized"),
            ApiError::Internal(_) => (StatusCode::INTERNAL_SERVER_ERROR, "InternalError"),
            ApiError::NotFound(_) => (StatusCode::NOT_FOUND, "NotFound"),
            ApiError::BadRequest(_) => (StatusCode::BAD_REQUEST, "BadRequest"),
            ApiError::Conflict(_) => (StatusCode::CONFLICT, "ConflictError"),
            ApiError::Forbidden(_) => (StatusCode::FORBIDDEN, "ForbiddenError"),
        };

        let error_message = match &self {
            ApiError::Image(img_err) => match img_err {
                ImageError::InvalidFormat => "This file type is not supported. Please upload an image file (PNG, JPG, GIF, WebP, or BMP).".to_string(),
                ImageError::TooLarge(size, max) => {
                    #[allow(clippy::cast_precision_loss)]
                    let size_mb = *size as f64 / 1_048_576.0;
                    #[allow(clippy::cast_precision_loss)]
                    let max_mb = *max as f64 / 1_048_576.0;
                    format!(
                        "This image is too large ({size_mb:.1} MB). Maximum file size is {max_mb:.1} MB."
                    )
                }
                ImageError::NotFound => "Image not found.".to_string(),
                _ => {
                    "Failed to process image. Please try again.".to_string()
                }
            },
            ApiError::GitService(git_err) => match git_err {
                services::services::git::GitServiceError::MergeConflicts(msg) => msg.clone(),
                services::services::git::GitServiceError::RebaseInProgress => {
                    "A rebase is already in progress. Resolve conflicts or abort the rebase, then retry.".to_string()
                }
                _ => {
                    tracing::error!(error_type, error = %self, "Internal server error");
                    "Internal server error".to_string()
                }
            },
            ApiError::Container(ContainerError::Sqlx(sqlx::Error::RowNotFound)) => {
                "Container not found.".to_string()
            }
            ApiError::Multipart(_) => "Failed to upload file. Please ensure the file is valid and try again.".to_string(),
            ApiError::Unauthorized => "Unauthorized. Please sign in again.".to_string(),
            ApiError::Internal(detail) => {
                // [G35-004] Log the detailed internal error for debugging but return
                // a generic message to the client to avoid leaking server internals.
                tracing::error!(error_type, detail = %detail, "Internal server error");
                "An internal error occurred. Please try again.".to_string()
            }
            ApiError::NotFound(msg) => msg.clone(),
            ApiError::BadRequest(msg) | ApiError::Conflict(msg) | ApiError::Forbidden(msg) => {
                msg.clone()
            }
            _ => {
                if status_code == StatusCode::INTERNAL_SERVER_ERROR {
                    // [G35-003/G35-004] Log the full internal error for debugging but
                    // return a generic message to the client to avoid leaking internals.
                    tracing::error!(error_type, error = %self, "Internal server error");
                    "Internal server error".to_string()
                } else {
                    // Non-500 errors are client-facing (4xx) and safe to expose the type.
                    format!("{error_type}: {self}")
                }
            }
        };
        let response = ApiResponse::<()>::error(&error_message);
        (status_code, Json(response)).into_response()
    }
}

impl From<ProjectServiceError> for ApiError {
    fn from(err: ProjectServiceError) -> Self {
        match err {
            ProjectServiceError::Database(db_err) => ApiError::Database(db_err),
            ProjectServiceError::Io(io_err) => ApiError::Io(io_err),
            ProjectServiceError::Project(proj_err) => ApiError::Project(proj_err),
            ProjectServiceError::PathNotFound(path) => {
                ApiError::BadRequest(format!("Path does not exist: {}", path.display()))
            }
            ProjectServiceError::PathNotDirectory(path) => {
                ApiError::BadRequest(format!("Path is not a directory: {}", path.display()))
            }
            ProjectServiceError::NotGitRepository(path) => {
                ApiError::BadRequest(format!("Path is not a git repository: {}", path.display()))
            }
            ProjectServiceError::DuplicateGitRepoPath => ApiError::Conflict(
                "A project with this git repository path already exists".to_string(),
            ),
            ProjectServiceError::DuplicateRepositoryName => ApiError::Conflict(
                "A repository with this name already exists in the project".to_string(),
            ),
            ProjectServiceError::RepositoryNotFound => {
                ApiError::NotFound("Repository not found".to_string())
            }
            ProjectServiceError::GitError(msg) => {
                ApiError::BadRequest(format!("Git operation failed: {msg}"))
            }
        }
    }
}

impl From<RepoServiceError> for ApiError {
    fn from(err: RepoServiceError) -> Self {
        match err {
            RepoServiceError::Database(db_err) => ApiError::Database(db_err),
            RepoServiceError::Io(io_err) => ApiError::Io(io_err),
            RepoServiceError::PathNotFound(path) => {
                ApiError::BadRequest(format!("Path does not exist: {}", path.display()))
            }
            RepoServiceError::PathNotDirectory(path) => {
                ApiError::BadRequest(format!("Path is not a directory: {}", path.display()))
            }
            RepoServiceError::NotGitRepository(path) => {
                ApiError::BadRequest(format!("Path is not a git repository: {}", path.display()))
            }
            RepoServiceError::NotFound => ApiError::NotFound("Repository not found".to_string()),
            RepoServiceError::DirectoryAlreadyExists(path) => {
                ApiError::BadRequest(format!("Directory already exists: {}", path.display()))
            }
            RepoServiceError::Git(git_err) => ApiError::BadRequest(format!("Git error: {git_err}")),
            RepoServiceError::InvalidFolderName(name) => {
                ApiError::BadRequest(format!("Invalid folder name: {name}"))
            }
        }
    }
}

impl From<ProjectRepoError> for ApiError {
    fn from(err: ProjectRepoError) -> Self {
        match err {
            ProjectRepoError::Database(db_err) => ApiError::Database(db_err),
            ProjectRepoError::NotFound => {
                ApiError::NotFound("Repository not found in project".to_string())
            }
            ProjectRepoError::AlreadyExists => {
                ApiError::Conflict("Repository already exists in project".to_string())
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn status_of(error: ApiError) -> StatusCode {
        error.into_response().status()
    }

    #[test]
    fn repo_service_not_found_maps_to_404() {
        let error = ApiError::from(RepoServiceError::NotFound);
        assert!(matches!(error, ApiError::NotFound(_)));
        assert_eq!(status_of(error), StatusCode::NOT_FOUND);
    }

    #[test]
    fn project_repo_not_found_maps_to_404() {
        let error = ApiError::from(ProjectRepoError::NotFound);
        assert!(matches!(error, ApiError::NotFound(_)));
        assert_eq!(status_of(error), StatusCode::NOT_FOUND);
    }

    #[test]
    fn workspace_validation_maps_to_400() {
        let error = ApiError::Workspace(WorkspaceError::ValidationError("invalid".to_string()));
        assert_eq!(status_of(error), StatusCode::BAD_REQUEST);
    }

    #[test]
    fn config_validation_maps_to_400() {
        let error = ApiError::Config(ConfigError::ValidationError("invalid".to_string()));
        assert_eq!(status_of(error), StatusCode::BAD_REQUEST);
    }

    #[test]
    fn sqlx_row_not_found_maps_to_404() {
        assert_eq!(
            status_of(ApiError::Database(sqlx::Error::RowNotFound)),
            StatusCode::NOT_FOUND
        );
    }
}
