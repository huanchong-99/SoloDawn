use axum::{
    Router,
    extract::{Json, Query, State},
    http::{Response, StatusCode},
    response::Json as ResponseJson,
    routing::{get, post},
};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use ts_rs::TS;
use utils::{api::oauth::StatusResponse, response::ApiResponse};
use uuid::Uuid;

use crate::{DeploymentImpl, error::ApiError};

/// Response from GET /api/auth/token - returns the current access token
#[derive(Debug, Serialize, TS)]
#[ts(export)]
pub struct TokenResponse {
    pub access_token: String,
    pub expires_at: Option<DateTime<Utc>>,
}

/// Response from GET /api/auth/user - returns the current user ID
#[derive(Debug, Serialize, TS)]
#[ts(export)]
pub struct CurrentUserResponse {
    pub user_id: String,
}

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
        .route("/auth/handoff/init", post(handoff_init))
        .route("/auth/handoff/complete", get(handoff_complete))
        .route("/auth/logout", post(logout))
        .route("/auth/status", get(status))
        .route("/auth/token", get(get_token))
        .route("/auth/user", get(get_current_user))
}

#[derive(Debug, Deserialize)]
#[allow(dead_code)]
struct HandoffInitPayload {
    provider: String,
    return_to: String,
}

#[derive(Debug, Serialize)]
struct HandoffInitResponseBody {
    handoff_id: Uuid,
    authorize_url: String,
}

async fn handoff_init(
    State(_deployment): State<DeploymentImpl>,
    Json(_payload): Json<HandoffInitPayload>,
) -> Result<ResponseJson<ApiResponse<HandoffInitResponseBody>>, ApiError> {
    Err(ApiError::BadRequest(
        "OAuth authentication is not supported in this version.".to_string(),
    ))
}

#[derive(Debug, Deserialize)]
#[allow(dead_code)]
struct HandoffCompleteQuery {
    handoff_id: Uuid,
    #[serde(default)]
    app_code: Option<String>,
    #[serde(default)]
    error: Option<String>,
}

async fn handoff_complete(
    State(_deployment): State<DeploymentImpl>,
    Query(_query): Query<HandoffCompleteQuery>,
) -> Result<Response<String>, ApiError> {
    Err(ApiError::BadRequest(
        "OAuth authentication is not supported in this version.".to_string(),
    ))
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

/// Returns the current access token (auto-refreshes if needed)
async fn get_token(
    State(_deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<TokenResponse>>, ApiError> {
    Err(ApiError::BadRequest(
        "OAuth authentication is not supported in this version.".to_string(),
    ))
}

async fn get_current_user(
    State(_deployment): State<DeploymentImpl>,
) -> Result<ResponseJson<ApiResponse<CurrentUserResponse>>, ApiError> {
    Err(ApiError::BadRequest(
        "OAuth authentication is not supported in this version.".to_string(),
    ))
}
