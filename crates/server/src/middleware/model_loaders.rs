use axum::{
    extract::{Path, Request, State},
    middleware::Next,
    response::Response,
};
use db::models::{
    execution_process::ExecutionProcess, project::Project, session::Session, tag::Tag, task::Task,
    workspace::Workspace,
};
use deployment::Deployment;
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

// TODO(W2-18-02): Missing ownership/access check on project load.
// This middleware loads a Project by ID for any authenticated caller. Today,
// SoloDawn uses a single shared API token (see `middleware/auth.rs`), so there
// is no per-user identity on the request and no `user_id`/`tenant_id` available
// to verify ownership against. In a multi-user / team deployment this is an
// IDOR (Insecure Direct Object Reference) risk: any valid token holder can
// reference any project UUID.
//
// To fix properly, the auth layer must be extended to:
//   1. Identify the caller (user_id / tenant_id / api_key_id) and insert it as
//      a request extension (e.g. `AuthContext`).
//   2. This middleware would then pull that extension and confirm the loaded
//      `project.owner_id` (or team membership) matches, returning 403/404
//      otherwise.
// Until such identity context exists, no ownership check is possible here.
pub async fn load_project_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(project_id): Path<Uuid>,
    request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Load the project from the database
    let project = match Project::find_by_id(&deployment.db().pool, project_id).await {
        Ok(Some(project)) => project,
        Ok(None) => {
            tracing::warn!("Project {} not found", project_id);
            return Err(ApiError::NotFound(format!("Project {project_id} not found")));
        }
        Err(e) => {
            tracing::error!("Failed to fetch project {}: {}", project_id, e);
            return Err(ApiError::Internal(format!("Failed to fetch project: {e}")));
        }
    };

    // Insert the project as an extension
    let mut request = request;
    request.extensions_mut().insert(project);

    // Continue with the next middleware/handler
    Ok(next.run(request).await)
}

// TODO(W2-18-03): Missing ownership/access check on task load.
// Same class of issue as W2-18-02: a Task is loaded by ID with no verification
// that the authenticated caller owns (or has access to) its parent Project.
// SoloDawn currently authenticates via a single shared API token
// (`middleware/auth.rs`), so there is no per-user identity on the request to
// check against. In multi-user/team scenarios this is an IDOR risk.
//
// To fix properly:
//   1. Extend auth to attach an `AuthContext { user_id, .. }` request extension.
//   2. Here, load the task, then load its parent project, and verify the
//      project's owner (or team) matches the caller. Return 403/404 otherwise.
// The contract comment below ("validate it belongs to the project") is also
// currently aspirational — no such validation is performed today.
pub async fn load_task_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(task_id): Path<Uuid>,
    request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Load the task and validate it belongs to the project
    let task = match Task::find_by_id(&deployment.db().pool, task_id).await {
        Ok(Some(task)) => task,
        Ok(None) => {
            tracing::warn!("Task {} not found", task_id);
            return Err(ApiError::NotFound(format!("Task {task_id} not found")));
        }
        Err(e) => {
            tracing::error!("Failed to fetch task {}: {}", task_id, e);
            return Err(ApiError::Internal(format!("Failed to fetch task: {e}")));
        }
    };

    // Insert both models as extensions
    let mut request = request;
    request.extensions_mut().insert(task);

    // Continue with the next middleware/handler
    Ok(next.run(request).await)
}

pub async fn load_workspace_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(workspace_id): Path<Uuid>,
    mut request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Load the Workspace from the database
    let workspace = match Workspace::find_by_id(&deployment.db().pool, workspace_id).await {
        Ok(Some(w)) => w,
        Ok(None) => {
            tracing::warn!("Workspace {} not found", workspace_id);
            return Err(ApiError::NotFound(format!("Workspace {workspace_id} not found")));
        }
        Err(e) => {
            tracing::error!("Failed to fetch Workspace {}: {}", workspace_id, e);
            return Err(ApiError::Internal(format!("Failed to fetch workspace: {e}")));
        }
    };

    // Insert the workspace into extensions
    request.extensions_mut().insert(workspace);

    // Continue on
    Ok(next.run(request).await)
}

pub async fn load_execution_process_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(process_id): Path<Uuid>,
    mut request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Load the execution process from the database
    let execution_process =
        match ExecutionProcess::find_by_id(&deployment.db().pool, process_id).await {
            Ok(Some(process)) => process,
            Ok(None) => {
                tracing::warn!("ExecutionProcess {} not found", process_id);
                return Err(ApiError::NotFound(format!("ExecutionProcess {process_id} not found")));
            }
            Err(e) => {
                tracing::error!("Failed to fetch execution process {}: {}", process_id, e);
                return Err(ApiError::Internal(format!("Failed to fetch execution process: {e}")));
            }
        };

    // Inject the execution process into the request
    request.extensions_mut().insert(execution_process);

    // Continue to the next middleware/handler
    Ok(next.run(request).await)
}

// Middleware that loads and injects Tag based on the tag_id path parameter
pub async fn load_tag_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(tag_id): Path<Uuid>,
    request: axum::extract::Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Load the tag from the database
    let tag = match Tag::find_by_id(&deployment.db().pool, tag_id).await {
        Ok(Some(tag)) => tag,
        Ok(None) => {
            tracing::warn!("Tag {} not found", tag_id);
            return Err(ApiError::NotFound(format!("Tag {tag_id} not found")));
        }
        Err(e) => {
            tracing::error!("Failed to fetch tag {}: {}", tag_id, e);
            return Err(ApiError::Internal(format!("Failed to fetch tag: {e}")));
        }
    };

    // Insert the tag as an extension
    let mut request = request;
    request.extensions_mut().insert(tag);

    // Continue with the next middleware/handler
    Ok(next.run(request).await)
}

pub async fn load_session_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(session_id): Path<Uuid>,
    mut request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    let session = match Session::find_by_id(&deployment.db().pool, session_id).await {
        Ok(Some(session)) => session,
        Ok(None) => {
            tracing::warn!("Session {} not found", session_id);
            return Err(ApiError::NotFound(format!("Session {session_id} not found")));
        }
        Err(e) => {
            tracing::error!("Failed to fetch session {}: {}", session_id, e);
            return Err(ApiError::Internal(format!("Failed to fetch session: {e}")));
        }
    };

    request.extensions_mut().insert(session);
    Ok(next.run(request).await)
}
