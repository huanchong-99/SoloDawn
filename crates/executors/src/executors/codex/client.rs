use std::{
    collections::VecDeque,
    io,
    sync::{Arc, OnceLock},
};

use async_trait::async_trait;
use serde::{Serialize, de::DeserializeOwned};
use serde_json::{self, Value};
use tokio::{
    io::{AsyncWrite, AsyncWriteExt, BufWriter},
    sync::Mutex,
};
use workspace_utils::approvals::ApprovalStatus;

use super::{
    jsonrpc::{JsonRpcCallbacks, JsonRpcPeer},
    protocol::{
        ApplyPatchApprovalResponse, ClientInfo, ClientNotification, ClientRequest,
        CommandExecutionApprovalDecision, CommandExecutionRequestApprovalResponse,
        ExecCommandApprovalResponse, FileChangeApprovalDecision, FileChangeRequestApprovalResponse,
        GetAuthStatusParams, GetAuthStatusResponse, InitializeParams, InitializeResponse,
        JSONRPCError, JSONRPCNotification, JSONRPCRequest, JSONRPCResponse, RequestId,
        ReviewDecision, ReviewStartParams, ReviewStartResponse, ReviewTarget, ServerRequest,
        TURN_COMPLETED_METHOD, ThreadForkParams, ThreadForkResponse, ThreadResumeParams,
        ThreadResumeResponse, ThreadStartParams, ThreadStartResponse, TurnCompletedNotification,
        TurnStartParams, TurnStartResponse, TurnStatus, UserInput,
    },
};
use crate::{
    approvals::{ExecutorApprovalError, ExecutorApprovalService},
    executors::{ExecutorError, codex::normalize_logs::Approval},
};

pub struct AppServerClient {
    rpc: OnceLock<JsonRpcPeer>,
    log_writer: LogWriter,
    approvals: Option<Arc<dyn ExecutorApprovalService>>,
    thread_id: Mutex<Option<String>>,
    pending_feedback: Mutex<VecDeque<String>>,
    auto_approve: bool,
}

impl AppServerClient {
    pub fn new(
        log_writer: LogWriter,
        approvals: Option<Arc<dyn ExecutorApprovalService>>,
        auto_approve: bool,
    ) -> Arc<Self> {
        Arc::new(Self {
            rpc: OnceLock::new(),
            log_writer,
            approvals,
            auto_approve,
            thread_id: Mutex::new(None),
            pending_feedback: Mutex::new(VecDeque::new()),
        })
    }

    pub fn connect(&self, peer: JsonRpcPeer) {
        let _ = self.rpc.set(peer);
    }

    fn rpc(&self) -> &JsonRpcPeer {
        self.rpc.get().expect("Codex RPC peer not attached")
    }

    pub async fn initialize(&self) -> Result<(), ExecutorError> {
        let request = ClientRequest::Initialize {
            request_id: self.next_request_id(),
            params: InitializeParams {
                client_info: ClientInfo {
                    name: "vibe-codex-executor".to_string(),
                    title: None,
                    version: env!("CARGO_PKG_VERSION").to_string(),
                },
            },
        };

        self.send_request::<InitializeResponse>(request, "initialize")
            .await?;
        self.send_message(&ClientNotification::Initialized).await
    }

    pub async fn start_thread(
        &self,
        params: ThreadStartParams,
    ) -> Result<ThreadStartResponse, ExecutorError> {
        let request = ClientRequest::ThreadStart {
            request_id: self.next_request_id(),
            params,
        };
        self.send_request(request, "thread/start").await
    }

    pub async fn resume_thread(
        &self,
        params: ThreadResumeParams,
    ) -> Result<ThreadResumeResponse, ExecutorError> {
        let request = ClientRequest::ThreadResume {
            request_id: self.next_request_id(),
            params,
        };
        self.send_request(request, "thread/resume").await
    }

    pub async fn fork_thread(
        &self,
        params: ThreadForkParams,
    ) -> Result<ThreadForkResponse, ExecutorError> {
        let request = ClientRequest::ThreadFork {
            request_id: self.next_request_id(),
            params,
        };
        self.send_request(request, "thread/fork").await
    }

    pub async fn start_turn(
        &self,
        thread_id: String,
        message: String,
    ) -> Result<TurnStartResponse, ExecutorError> {
        let request = ClientRequest::TurnStart {
            request_id: self.next_request_id(),
            params: TurnStartParams {
                thread_id,
                input: vec![UserInput::text(message)],
            },
        };
        self.send_request(request, "turn/start").await
    }

    pub async fn get_auth_status(&self) -> Result<GetAuthStatusResponse, ExecutorError> {
        let request = ClientRequest::GetAuthStatus {
            request_id: self.next_request_id(),
            params: GetAuthStatusParams {
                include_token: Some(true),
                refresh_token: Some(false),
            },
        };
        self.send_request(request, "getAuthStatus").await
    }

    pub async fn start_review(
        &self,
        thread_id: String,
        target: ReviewTarget,
    ) -> Result<ReviewStartResponse, ExecutorError> {
        let request = ClientRequest::ReviewStart {
            request_id: self.next_request_id(),
            params: ReviewStartParams {
                thread_id,
                target,
                delivery: None,
            },
        };
        self.send_request(request, "review/start").await
    }

    async fn handle_server_request(
        &self,
        peer: &JsonRpcPeer,
        request: ServerRequest,
    ) -> Result<(), ExecutorError> {
        match request {
            ServerRequest::ApplyPatchApproval { request_id, params } => {
                let input = serde_json::to_value(&params)
                    .map_err(|err| ExecutorError::Io(io::Error::other(err.to_string())))?;
                let status = self
                    .request_tool_approval_or_deny("edit", input, &params.call_id)
                    .await;
                self.log_approval_response(&params.call_id, "codex.apply_patch", &status)
                    .await?;
                let (decision, feedback) = self.review_decision(&status);
                let response = ApplyPatchApprovalResponse { decision };
                send_server_response(peer, request_id, response).await?;
                if let Some(message) = feedback {
                    tracing::debug!("queueing patch denial feedback: {message}");
                    self.enqueue_feedback(message).await;
                }
                Ok(())
            }
            ServerRequest::ExecCommandApproval { request_id, params } => {
                let input = serde_json::to_value(&params)
                    .map_err(|err| ExecutorError::Io(io::Error::other(err.to_string())))?;
                let status = self
                    .request_tool_approval_or_deny("bash", input, &params.call_id)
                    .await;
                self.log_approval_response(&params.call_id, "codex.exec_command", &status)
                    .await?;
                let (decision, feedback) = self.review_decision(&status);
                let response = ExecCommandApprovalResponse { decision };
                send_server_response(peer, request_id, response).await?;
                if let Some(message) = feedback {
                    tracing::debug!("queueing exec denial feedback: {message}");
                    self.enqueue_feedback(message).await;
                }
                Ok(())
            }
            ServerRequest::CommandExecutionRequestApproval { request_id, params } => {
                let input = serde_json::to_value(&params)
                    .map_err(|err| ExecutorError::Io(io::Error::other(err.to_string())))?;
                let status = self
                    .request_tool_approval_or_deny("bash", input, &params.item_id)
                    .await;
                self.log_approval_response(&params.item_id, "codex.exec_command", &status)
                    .await?;
                let (decision, feedback) = self.command_execution_decision(&status);
                let response = CommandExecutionRequestApprovalResponse { decision };
                send_server_response(peer, request_id, response).await?;
                if let Some(message) = feedback {
                    tracing::debug!("queueing exec denial feedback: {message}");
                    self.enqueue_feedback(message).await;
                }
                Ok(())
            }
            ServerRequest::FileChangeRequestApproval { request_id, params } => {
                let input = serde_json::to_value(&params)
                    .map_err(|err| ExecutorError::Io(io::Error::other(err.to_string())))?;
                let status = self
                    .request_tool_approval_or_deny("edit", input, &params.item_id)
                    .await;
                self.log_approval_response(&params.item_id, "codex.apply_patch", &status)
                    .await?;
                let (decision, feedback) = self.file_change_decision(&status);
                let response = FileChangeRequestApprovalResponse { decision };
                send_server_response(peer, request_id, response).await?;
                if let Some(message) = feedback {
                    tracing::debug!("queueing patch denial feedback: {message}");
                    self.enqueue_feedback(message).await;
                }
                Ok(())
            }
        }
    }

    async fn request_tool_approval_or_deny(
        &self,
        tool_name: &str,
        tool_input: Value,
        tool_call_id: &str,
    ) -> ApprovalStatus {
        match self
            .request_tool_approval(tool_name, tool_input, tool_call_id)
            .await
        {
            Ok(status) => status,
            Err(err) => {
                tracing::error!("failed to request {tool_name} approval: {err}");
                ApprovalStatus::Denied {
                    reason: Some("approval service error".to_string()),
                }
            }
        }
    }

    async fn log_approval_response(
        &self,
        call_id: &str,
        tool_name: &str,
        status: &ApprovalStatus,
    ) -> Result<(), ExecutorError> {
        self.log_writer
            .log_raw(
                &Approval::approval_response(
                    call_id.to_string(),
                    tool_name.to_string(),
                    status.clone(),
                )
                .raw(),
            )
            .await
    }

    async fn request_tool_approval(
        &self,
        tool_name: &str,
        tool_input: Value,
        tool_call_id: &str,
    ) -> Result<ApprovalStatus, ExecutorError> {
        // TODO: Revisit this arbitrary delay; see E33-08. It appears to give the
        // approval UI/tooling a short grace window to register the incoming tool
        // before the approval prompt races ahead. A constant makes the intent
        // discoverable, but the underlying race should be fixed properly.
        const APPROVAL_WINDOW_DELAY: std::time::Duration = std::time::Duration::from_millis(20);
        tokio::time::sleep(APPROVAL_WINDOW_DELAY).await;
        if self.auto_approve {
            return Ok(ApprovalStatus::Approved);
        }
        Ok(self
            .approvals
            .as_ref()
            .ok_or(ExecutorApprovalError::ServiceUnavailable)?
            .request_tool_approval(tool_name, tool_input, tool_call_id)
            .await?)
    }

    pub async fn register_session(&self, thread_id: &str) -> Result<(), ExecutorError> {
        {
            let mut guard = self.thread_id.lock().await;
            guard.replace(thread_id.to_string());
        }
        self.flush_pending_feedback().await;
        Ok(())
    }

    async fn send_message<M>(&self, message: &M) -> Result<(), ExecutorError>
    where
        M: Serialize + Sync,
    {
        self.rpc().send(message).await
    }

    async fn send_request<R>(&self, request: ClientRequest, label: &str) -> Result<R, ExecutorError>
    where
        R: DeserializeOwned + std::fmt::Debug,
    {
        let request_id = request_id(&request);
        self.rpc().request(request_id, &request, label).await
    }

    fn next_request_id(&self) -> RequestId {
        self.rpc().next_request_id()
    }

    /// Feedback message for a denial, if the user supplied a reason.
    fn denial_feedback(status: &ApprovalStatus) -> Option<String> {
        match status {
            ApprovalStatus::Denied { reason } => reason
                .as_deref()
                .map(str::trim)
                .filter(|s| !s.is_empty())
                .map(ToString::to_string),
            _ => None,
        }
    }

    fn review_decision(&self, status: &ApprovalStatus) -> (ReviewDecision, Option<String>) {
        if self.auto_approve {
            return (ReviewDecision::ApprovedForSession, None);
        }

        match status {
            ApprovalStatus::Approved => (ReviewDecision::Approved, None),
            ApprovalStatus::Denied { .. } => {
                (ReviewDecision::Denied, Self::denial_feedback(status))
            }
            ApprovalStatus::TimedOut | ApprovalStatus::Pending => (ReviewDecision::Denied, None),
        }
    }

    fn command_execution_decision(
        &self,
        status: &ApprovalStatus,
    ) -> (CommandExecutionApprovalDecision, Option<String>) {
        if self.auto_approve {
            return (CommandExecutionApprovalDecision::AcceptForSession, None);
        }

        match status {
            ApprovalStatus::Approved => (CommandExecutionApprovalDecision::Accept, None),
            ApprovalStatus::Denied { .. } => (
                CommandExecutionApprovalDecision::Decline,
                Self::denial_feedback(status),
            ),
            ApprovalStatus::TimedOut | ApprovalStatus::Pending => {
                (CommandExecutionApprovalDecision::Decline, None)
            }
        }
    }

    fn file_change_decision(
        &self,
        status: &ApprovalStatus,
    ) -> (FileChangeApprovalDecision, Option<String>) {
        if self.auto_approve {
            return (FileChangeApprovalDecision::AcceptForSession, None);
        }

        match status {
            ApprovalStatus::Approved => (FileChangeApprovalDecision::Accept, None),
            ApprovalStatus::Denied { .. } => (
                FileChangeApprovalDecision::Decline,
                Self::denial_feedback(status),
            ),
            ApprovalStatus::TimedOut | ApprovalStatus::Pending => {
                (FileChangeApprovalDecision::Decline, None)
            }
        }
    }

    async fn enqueue_feedback(&self, message: String) {
        if message.trim().is_empty() {
            return;
        }
        let mut guard = self.pending_feedback.lock().await;
        guard.push_back(message);
    }

    async fn flush_pending_feedback(&self) {
        let messages: Vec<String> = {
            let mut guard = self.pending_feedback.lock().await;
            guard.drain(..).collect()
        };

        if messages.is_empty() {
            return;
        }

        let Some(thread_id) = self.thread_id.lock().await.clone() else {
            tracing::warn!(
                "pending Codex feedback but thread id unavailable; dropping {} messages",
                messages.len()
            );
            return;
        };

        for message in messages {
            let trimmed = message.trim();
            if trimmed.is_empty() {
                continue;
            }
            self.spawn_feedback_message(thread_id.clone(), trimmed);
        }
    }

    fn spawn_feedback_message(&self, thread_id: String, feedback: &str) {
        let peer = self.rpc().clone();
        let request = ClientRequest::TurnStart {
            request_id: peer.next_request_id(),
            params: TurnStartParams {
                thread_id,
                input: vec![UserInput::text(format!("User feedback: {feedback}"))],
            },
        };
        tokio::spawn(async move {
            if let Err(err) = peer
                .request::<TurnStartResponse, _>(request_id(&request), &request, "turn/start")
                .await
            {
                tracing::error!("failed to send feedback follow-up message: {err}");
            }
        });
    }
}

#[async_trait]
impl JsonRpcCallbacks for AppServerClient {
    async fn on_request(
        &self,
        peer: &JsonRpcPeer,
        raw: &str,
        request: JSONRPCRequest,
    ) -> Result<(), ExecutorError> {
        self.log_writer.log_raw(raw).await?;
        match ServerRequest::try_from(request.clone()) {
            Ok(server_request) => self.handle_server_request(peer, server_request).await,
            Err(err) => {
                tracing::debug!("Unhandled server request `{}`: {err}", request.method);
                let response = JSONRPCResponse {
                    id: request.id,
                    result: Value::Null,
                };
                peer.send(&response).await
            }
        }
    }

    async fn on_response(
        &self,
        _peer: &JsonRpcPeer,
        raw: &str,
        _response: &JSONRPCResponse,
    ) -> Result<(), ExecutorError> {
        self.log_writer.log_raw(raw).await
    }

    async fn on_error(
        &self,
        _peer: &JsonRpcPeer,
        raw: &str,
        _error: &JSONRPCError,
    ) -> Result<(), ExecutorError> {
        self.log_writer.log_raw(raw).await
    }

    async fn on_notification(
        &self,
        _peer: &JsonRpcPeer,
        raw: &str,
        notification: JSONRPCNotification,
    ) -> Result<bool, ExecutorError> {
        self.log_writer.log_raw(raw).await?;

        // `turn/completed` replaces both the v1 `task_complete` and
        // `turn_aborted` events as the terminal signal for a turn.
        if notification.method != TURN_COMPLETED_METHOD {
            return Ok(false);
        }
        let Some(params) = notification.params else {
            return Ok(false);
        };
        let completed: TurnCompletedNotification = match serde_json::from_value(params) {
            Ok(value) => value,
            Err(err) => {
                tracing::warn!("failed to decode turn/completed notification: {err}");
                return Ok(false);
            }
        };
        match completed.turn.status {
            // Finishing the read loop without an error emits a Success exit signal.
            TurnStatus::Completed => Ok(true),
            TurnStatus::InProgress => Ok(false),
            // Returning an error makes the read loop emit a Failure exit signal.
            status @ (TurnStatus::Interrupted | TurnStatus::Failed | TurnStatus::Unknown) => {
                tracing::debug!(?status, "codex turn ended without completing");
                Err(ExecutorError::Io(io::Error::other(format!(
                    "codex turn ended with status {status:?}"
                ))))
            }
        }
    }

    async fn on_non_json(&self, raw: &str) -> Result<(), ExecutorError> {
        self.log_writer.log_raw(raw).await?;
        Ok(())
    }
}

async fn send_server_response<T>(
    peer: &JsonRpcPeer,
    request_id: RequestId,
    response: T,
) -> Result<(), ExecutorError>
where
    T: Serialize,
{
    let payload = JSONRPCResponse {
        id: request_id,
        result: serde_json::to_value(response)
            .map_err(|err| ExecutorError::Io(io::Error::other(err.to_string())))?,
    };

    peer.send(&payload).await
}

fn request_id(request: &ClientRequest) -> RequestId {
    match request {
        ClientRequest::Initialize { request_id, .. }
        | ClientRequest::ThreadStart { request_id, .. }
        | ClientRequest::ThreadResume { request_id, .. }
        | ClientRequest::ThreadFork { request_id, .. }
        | ClientRequest::TurnStart { request_id, .. }
        | ClientRequest::GetAuthStatus { request_id, .. }
        | ClientRequest::ReviewStart { request_id, .. } => request_id.clone(),
    }
}

#[derive(Clone)]
pub struct LogWriter {
    writer: Arc<Mutex<BufWriter<Box<dyn AsyncWrite + Send + Unpin>>>>,
}

impl LogWriter {
    pub fn new(writer: impl AsyncWrite + Send + Unpin + 'static) -> Self {
        Self {
            writer: Arc::new(Mutex::new(BufWriter::new(Box::new(writer)))),
        }
    }

    pub async fn log_raw(&self, raw: &str) -> Result<(), ExecutorError> {
        let mut guard = self.writer.lock().await;
        guard
            .write_all(raw.as_bytes())
            .await
            .map_err(ExecutorError::Io)?;
        guard.write_all(b"\n").await.map_err(ExecutorError::Io)?;
        guard.flush().await.map_err(ExecutorError::Io)?;
        Ok(())
    }
}
