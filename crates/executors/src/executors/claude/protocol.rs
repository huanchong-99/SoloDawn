use std::sync::Arc;

use futures::FutureExt;
use tokio::{
    io::{AsyncBufReadExt, AsyncWriteExt, BufReader},
    process::{ChildStdin, ChildStdout},
    sync::{Mutex, oneshot},
};

use super::types::{CLIMessage, ControlRequestType, ControlResponseMessage, ControlResponseType};
use crate::executors::{
    ExecutorError,
    claude::{
        client::ClaudeAgentClient,
        types::{Message, PermissionMode, SDKControlRequest, SDKControlRequestType},
    },
};

/// Handles bidirectional control protocol communication
#[derive(Clone)]
pub struct ProtocolPeer {
    stdin: Arc<Mutex<ChildStdin>>,
}

impl ProtocolPeer {
    pub fn spawn(
        stdin: ChildStdin,
        stdout: ChildStdout,
        client: Arc<ClaudeAgentClient>,
        interrupt_rx: oneshot::Receiver<()>,
    ) -> Self {
        let peer = Self {
            stdin: Arc::new(Mutex::new(stdin)),
        };

        let reader_peer = peer.clone();
        tokio::spawn(async move {
            if let Err(e) = reader_peer.read_loop(stdout, client, interrupt_rx).await {
                tracing::error!("Protocol reader loop error: {}", e);
            }
        });

        peer
    }

    async fn read_loop(
        &self,
        stdout: ChildStdout,
        client: Arc<ClaudeAgentClient>,
        interrupt_rx: oneshot::Receiver<()>,
    ) -> Result<(), ExecutorError> {
        let mut reader = BufReader::new(stdout);
        // Byte buffer for `read_until`: it accumulates partially-read bytes
        // IN PLACE across cancelled reads, which is what makes the select!
        // loop below cancellation-safe (see the read branch comment).
        let mut buffer: Vec<u8> = Vec::new();
        // Fuse the receiver so it returns Pending forever after completing
        let mut interrupt_rx = interrupt_rx.fuse();

        loop {
            tokio::select! {
                read_result = reader.read_until(b'\n', &mut buffer) => {
                    match read_result {
                        // EOF: this call appended no bytes. An unterminated
                        // tail left in `buffer` (child exited mid-line after a
                        // cancelled read) is dropped, matching the previous
                        // `read_line` behavior at EOF.
                        Ok(0) => break,
                        Ok(_) => {
                            // Cancellation safety (why `read_until`, not
                            // `read_line`): tokio's `read_line` moves the
                            // caller's String into its future, so when the
                            // interrupt branch below wins mid-read the partial
                            // bytes are dropped with the future and the line
                            // is torn. `read_until` borrows `buffer` and
                            // appends in place, completing only at the
                            // delimiter or EOF — a cancelled call leaves the
                            // partial bytes in `buffer` and the next call
                            // appends the remainder, so lines are never torn.
                            let line_owned =
                                match String::from_utf8(std::mem::take(&mut buffer)) {
                                    Ok(s) => s,
                                    Err(e) => {
                                        tracing::error!(
                                            "Error reading stdout: CLI emitted invalid UTF-8: {e}"
                                        );
                                        break;
                                    }
                                };
                            // E33-07: `trim()` strips both leading and trailing
                            // whitespace (including the `\n` terminator). This
                            // is acceptable here because the Claude CLI emits
                            // one JSON object per line, and any leading/
                            // trailing whitespace is never semantically
                            // significant inside the line framing.
                            let line = line_owned.trim();
                            if line.is_empty() {
                                continue;
                            }
                            // Parse message using typed enum
                            match serde_json::from_str::<CLIMessage>(line) {
                                Ok(CLIMessage::ControlRequest {
                                    request_id,
                                    request,
                                }) => {
                                    self.handle_control_request(&client, request_id, request)
                                        .await;
                                }
                                Ok(CLIMessage::ControlResponse { .. }) => {}
                                Ok(CLIMessage::Result(_)) => {
                                    client.on_non_control(line).await?;
                                    break;
                                }
                                _ => {
                                    client.on_non_control(line).await?;
                                }
                            }
                        }
                        Err(e) => {
                            tracing::error!("Error reading stdout: {}", e);
                            break;
                        }
                    }
                }
                _ = &mut interrupt_rx => {
                    if let Err(e) = self.interrupt().await {
                        tracing::debug!("Failed to send interrupt to Claude: {e}");
                    }
                }
            }
        }
        Ok(())
    }

    async fn handle_control_request(
        &self,
        client: &Arc<ClaudeAgentClient>,
        request_id: String,
        request: ControlRequestType,
    ) {
        match request {
            ControlRequestType::CanUseTool {
                tool_name,
                input,
                permission_suggestions,
                blocked_paths: _,
                tool_use_id,
            } => {
                match client
                    .on_can_use_tool(tool_name, input, permission_suggestions, tool_use_id)
                    .await
                {
                    Ok(result) => match serde_json::to_value(result) {
                        Ok(value) => {
                            if let Err(e) = self.send_hook_response(request_id, value).await {
                                tracing::error!("Failed to send permission result: {e}");
                            }
                        }
                        Err(e) => {
                            tracing::error!("Failed to serialize permission result: {e}");
                            if let Err(e2) = self.send_error(request_id, e.to_string()).await {
                                tracing::error!("Failed to send error response: {e2}");
                            }
                        }
                    },
                    Err(e) => {
                        tracing::error!("Error in on_can_use_tool: {e}");
                        if let Err(e2) = self.send_error(request_id, e.to_string()).await {
                            tracing::error!("Failed to send error response: {e2}");
                        }
                    }
                }
            }
            ControlRequestType::HookCallback {
                callback_id,
                input,
                tool_use_id,
            } => match client.on_hook_callback(&callback_id, input, tool_use_id) {
                Ok(hook_output) => {
                    if let Err(e) = self.send_hook_response(request_id, hook_output).await {
                        tracing::error!("Failed to send hook callback result: {e}");
                    }
                }
                Err(e) => {
                    tracing::error!("Error in on_hook_callback: {e}");
                    if let Err(e2) = self.send_error(request_id, e.to_string()).await {
                        tracing::error!("Failed to send error response: {e2}");
                    }
                }
            },
        }
    }

    pub async fn send_hook_response(
        &self,
        request_id: String,
        hook_output: serde_json::Value,
    ) -> Result<(), ExecutorError> {
        self.send_json(&ControlResponseMessage::new(ControlResponseType::Success {
            request_id,
            response: Some(hook_output),
        }))
        .await
    }

    /// Send error response to CLI
    async fn send_error(&self, request_id: String, error: String) -> Result<(), ExecutorError> {
        self.send_json(&ControlResponseMessage::new(ControlResponseType::Error {
            request_id,
            error: Some(error),
        }))
        .await
    }

    async fn send_json<T: serde::Serialize>(&self, message: &T) -> Result<(), ExecutorError> {
        let json = serde_json::to_string(message)?;
        let mut stdin = self.stdin.lock().await;
        stdin.write_all(json.as_bytes()).await?;
        stdin.write_all(b"\n").await?;
        stdin.flush().await?;
        Ok(())
    }

    pub async fn send_user_message(&self, content: String) -> Result<(), ExecutorError> {
        let message = Message::new_user(content);
        self.send_json(&message).await
    }

    pub async fn initialize(&self, hooks: Option<serde_json::Value>) -> Result<(), ExecutorError> {
        self.send_json(&SDKControlRequest::new(SDKControlRequestType::Initialize {
            hooks,
        }))
        .await
    }
    pub async fn interrupt(&self) -> Result<(), ExecutorError> {
        self.send_json(&SDKControlRequest::new(SDKControlRequestType::Interrupt {}))
            .await
    }

    pub async fn set_permission_mode(&self, mode: PermissionMode) -> Result<(), ExecutorError> {
        self.send_json(&SDKControlRequest::new(
            SDKControlRequestType::SetPermissionMode { mode },
        ))
        .await
    }
}
