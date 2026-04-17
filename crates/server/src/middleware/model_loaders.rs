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

use crate::{DeploymentImpl, error::ApiError, middleware::auth::{RequestContext, assert_authorized}};

// W2-18-02: Project load is gated below by `assert_authorized`, which closes
// the dev-mode passthrough hole when `SOLODAWN_REQUIRE_AUTH=1`. Per-user
// project ownership still requires G24 (principal with project scope on
// `RequestContext`); until that lands, any holder of a valid shared API
// token can load any project by UUID.
pub async fn load_project_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(project_id): Path<Uuid>,
    request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Defense-in-depth: opt-in authz gate. Falls through to the legacy
    // "dev mode" behavior unless SOLODAWN_REQUIRE_AUTH is set.
    let default_ctx = RequestContext { authenticated: false };
    let ctx = request
        .extensions()
        .get::<RequestContext>()
        .cloned()
        .unwrap_or(default_ctx);
    if let Err(resp) = assert_authorized(&ctx) {
        return Ok(resp);
    }

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

// W2-18-03: Same posture as `load_project_middleware` — the opt-in gate
// below closes the dev-mode passthrough hole, but per-user task ownership
// still requires G24. The phrase "validate it belongs to the project" in
// the inline comment further below remains aspirational until principal
// scoping exists.
pub async fn load_task_middleware(
    State(deployment): State<DeploymentImpl>,
    Path(task_id): Path<Uuid>,
    request: Request,
    next: Next,
) -> Result<Response, ApiError> {
    // Defense-in-depth: opt-in authz gate. Falls through to the legacy
    // "dev mode" behavior unless SOLODAWN_REQUIRE_AUTH is set.
    let default_ctx = RequestContext { authenticated: false };
    let ctx = request
        .extensions()
        .get::<RequestContext>()
        .cloned()
        .unwrap_or(default_ctx);
    if let Err(resp) = assert_authorized(&ctx) {
        return Ok(resp);
    }

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
    // Same opt-in gate as `load_project_middleware`.
    let default_ctx = RequestContext { authenticated: false };
    let ctx = request
        .extensions()
        .get::<RequestContext>()
        .cloned()
        .unwrap_or(default_ctx);
    if let Err(resp) = assert_authorized(&ctx) {
        return Ok(resp);
    }

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
    // Same opt-in gate as `load_project_middleware`.
    let default_ctx = RequestContext { authenticated: false };
    let ctx = request
        .extensions()
        .get::<RequestContext>()
        .cloned()
        .unwrap_or(default_ctx);
    if let Err(resp) = assert_authorized(&ctx) {
        return Ok(resp);
    }

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
    // Same opt-in gate as `load_project_middleware`.
    let default_ctx = RequestContext { authenticated: false };
    let ctx = request
        .extensions()
        .get::<RequestContext>()
        .cloned()
        .unwrap_or(default_ctx);
    if let Err(resp) = assert_authorized(&ctx) {
        return Ok(resp);
    }

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
    // Same opt-in gate as `load_project_middleware`.
    let default_ctx = RequestContext { authenticated: false };
    let ctx = request
        .extensions()
        .get::<RequestContext>()
        .cloned()
        .unwrap_or(default_ctx);
    if let Err(resp) = assert_authorized(&ctx) {
        return Ok(resp);
    }

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
