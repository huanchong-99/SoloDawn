use std::sync::Arc;

use super::{
    client::{AppServerClient, LogWriter},
    jsonrpc::{ExitSignalSender, JsonRpcPeer},
    protocol::{ReviewTarget, ThreadForkParams, ThreadStartParams},
};
use crate::{approvals::ExecutorApprovalService, executors::ExecutorError};

#[allow(clippy::too_many_arguments)]
pub async fn launch_codex_review(
    thread_params: ThreadStartParams,
    resume_session: Option<String>,
    review_target: ReviewTarget,
    child_stdout: tokio::process::ChildStdout,
    child_stdin: tokio::process::ChildStdin,
    log_writer: LogWriter,
    exit_signal_tx: ExitSignalSender,
    approvals: Option<Arc<dyn ExecutorApprovalService>>,
    auto_approve: bool,
) -> Result<(), ExecutorError> {
    let client = AppServerClient::new(log_writer, approvals, auto_approve);
    let rpc_peer = JsonRpcPeer::spawn(child_stdin, child_stdout, client.clone(), exit_signal_tx);
    client.connect(rpc_peer);
    client.initialize().await?;
    let auth_status = client.get_auth_status().await?;
    if auth_status.requires_openai_auth.unwrap_or(true) && auth_status.auth_method.is_none() {
        return Err(ExecutorError::AuthRequired(
            "Codex authentication required".to_string(),
        ));
    }

    let thread_id = if let Some(session_id) = resume_session {
        // Fork the previous session into a fresh thread so the review runs on
        // its own session id (replaces the old manual rollout-file copy +
        // resumeConversation flow).
        let params = ThreadForkParams::from_thread_start(session_id, thread_params);
        let response = client
            .fork_thread(params)
            .await
            .map_err(|e| ExecutorError::FollowUpNotSupported(e.to_string()))?;
        tracing::debug!("forked session for review, response {:?}", response);
        response.thread.id
    } else {
        let response = client.start_thread(thread_params).await?;
        response.thread.id
    };

    client.register_session(&thread_id).await?;

    client.start_review(thread_id, review_target).await?;

    Ok(())
}
