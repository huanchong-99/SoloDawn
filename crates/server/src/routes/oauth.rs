use axum::{
    Router,
    extract::State,
    http::StatusCode,
    response::Json as ResponseJson,
    routing::{get, post},
};
use utils::{api::oauth::StatusResponse, response::ApiResponse};

use crate::{DeploymentImpl, error::ApiError};

// NOTE(W2-34-03): If/when OAuth is re-enabled in this deployment, the
// pre-authentication endpoints below (`/auth/handoff/init`,
// `/auth/handoff/complete`, `/auth/token`) MUST be moved into
// `unauthed_routes` in `mod.rs` — mirroring the setup move performed for
// G08 — because clients hitting them will not yet possess an API token.
// Today every handler is a stub that returns BadRequest ("OAuth
// authentication is not supported in this version."), so leaving them in
// the authed zone is observationally harmless; do not move them while
// they are stubs or the unauthed surface grows without purpose. Reassess
// together with the real OAuth implementation.
pub fn router() -> Router<DeploymentImpl> {
    Router::new()
        .route("/auth/logout", post(logout))
        .route("/auth/status", get(status))
}

async fn logout(State(_deployment): State<DeploymentImpl>) -> Result<StatusCode, ApiError> {
    Err(ApiError::BadRequest(
        "OAuth authentication is not supported in this version.".to_string(),
    ))
}

async fn status(
    State(deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<StatusResponse>>, ApiError> {
    use utils::api::oauth::LoginStatus;

    match deployment.get_login_status().await {
        LoginStatus::LoggedOut => Ok(ResponseJson(ApiResponse::success(StatusResponse {
            logged_in: false,
            profile: None,
            degraded: None,
        }))),
        LoginStatus::LoggedIn { profile } => {
            Ok(ResponseJson(ApiResponse::success(StatusResponse {
                logged_in: true,
                profile: Some(profile),
                degraded: None,
            })))
        }
    }
}

