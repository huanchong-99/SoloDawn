use std::sync::Arc;
use std::time::UNIX_EPOCH;

use tokio_stream::wrappers::ReceiverStream;
use tonic::{Request, Response, Status};

use services::terminal::process::{ProcessManager, SpawnCommand, SpawnEnv};

use crate::proto::runner_service_server::RunnerService;
use crate::proto::{
    HealthRequest, HealthResponse, IsRunningRequest, IsRunningResponse, KillTerminalRequest,
    KillTerminalResponse, ResizeRequest, ResizeResponse, SpawnTerminalRequest,
    SpawnTerminalResponse, StreamOutputRequest, TerminalOutputChunk, WriteInputRequest,
    WriteInputResponse,
};

/// gRPC service implementation for the Runner, backed by ProcessManager.
pub struct RunnerGrpcService {
    process_manager: Arc<ProcessManager>,
}

impl RunnerGrpcService {
    pub fn new(process_manager: Arc<ProcessManager>) -> Self {
        Self { process_manager }
    }
}

#[tonic::async_trait]
impl RunnerService for RunnerGrpcService {
    type StreamOutputStream = ReceiverStream<Result<TerminalOutputChunk, Status>>;

    async fn spawn_terminal(
        &self,
        request: Request<SpawnTerminalRequest>,
    ) -> Result<Response<SpawnTerminalResponse>, Status> {
        let req = request.into_inner();

        let mut env = SpawnEnv::new();
        env.set = req.env_set;
        env.unset = req.env_unset;

        let config = SpawnCommand {
            command: req.command,
            args: req.args,
            working_dir: req.working_dir.into(),
            env,
        };

        let cols = if req.cols > 0 { req.cols as u16 } else { 80 };
        let rows = if req.rows > 0 { req.rows as u16 } else { 24 };

        match self
            .process_manager
            .spawn_pty_with_config(&req.terminal_id, &config, cols, rows)
            .await
        {
            Ok(handle) => {
                tracing::info!(
                    terminal_id = %req.terminal_id,
                    pid = handle.pid,
                    "Terminal spawned via gRPC"
                );
                Ok(Response::new(SpawnTerminalResponse {
                    success: true,
                    error: String::new(),
                    pid: handle.pid,
                }))
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %req.terminal_id,
                    error = %e,
                    "Failed to spawn terminal via gRPC"
                );
                Ok(Response::new(SpawnTerminalResponse {
                    success: false,
                    error: e.to_string(),
                    pid: 0,
                }))
            }
        }
    }

    async fn kill_terminal(
        &self,
        request: Request<KillTerminalRequest>,
    ) -> Result<Response<KillTerminalResponse>, Status> {
        let req = request.into_inner();

        match self.process_manager.kill_terminal(&req.terminal_id).await {
            Ok(()) => {
                tracing::info!(terminal_id = %req.terminal_id, "Terminal killed via gRPC");
                Ok(Response::new(KillTerminalResponse { success: true }))
            }
            Err(e) => {
                tracing::warn!(
                    terminal_id = %req.terminal_id,
                    error = %e,
                    "Failed to kill terminal via gRPC"
                );
                Err(Status::not_found(e.to_string()))
            }
        }
    }

    async fn is_running(
        &self,
        request: Request<IsRunningRequest>,
    ) -> Result<Response<IsRunningResponse>, Status> {
        let req = request.into_inner();
        let running = self.process_manager.is_running(&req.terminal_id).await;
        Ok(Response::new(IsRunningResponse { running }))
    }

    async fn resize_terminal(
        &self,
        request: Request<ResizeRequest>,
    ) -> Result<Response<ResizeResponse>, Status> {
        let req = request.into_inner();

        match self
            .process_manager
            .resize(&req.terminal_id, req.cols as u16, req.rows as u16)
            .await
        {
            Ok(()) => Ok(Response::new(ResizeResponse { success: true })),
            Err(e) => {
                tracing::warn!(
                    terminal_id = %req.terminal_id,
                    error = %e,
                    "Failed to resize terminal via gRPC"
                );
                Err(Status::not_found(e.to_string()))
            }
        }
    }

    async fn write_input(
        &self,
        request: Request<WriteInputRequest>,
    ) -> Result<Response<WriteInputResponse>, Status> {
        let req = request.into_inner();

        let handle = self
            .process_manager
            .get_handle(&req.terminal_id)
            .await
            .ok_or_else(|| Status::not_found(format!("Terminal not found: {}", req.terminal_id)))?;

        let writer = handle
            .writer
            .ok_or_else(|| Status::internal("No PTY writer available"))?;

        let mut writer_guard = writer
            .lock()
            .map_err(|e| Status::internal(format!("Failed to lock PTY writer: {e}")))?;

        writer_guard
            .write_all(&req.data)
            .map_err(|e| Status::internal(format!("Failed to write to PTY: {e}")))?;

        writer_guard
            .flush()
            .map_err(|e| Status::internal(format!("Failed to flush PTY: {e}")))?;

        Ok(Response::new(WriteInputResponse { success: true }))
    }

    async fn stream_output(
        &self,
        request: Request<StreamOutputRequest>,
    ) -> Result<Response<Self::StreamOutputStream>, Status> {
        let req = request.into_inner();
        let from_seq = if req.from_seq > 0 {
            Some(req.from_seq)
        } else {
            None
        };

        let mut subscription = self
            .process_manager
            .subscribe_output(&req.terminal_id, from_seq)
            .await
            .map_err(|e| Status::not_found(e.to_string()))?;

        let (tx, rx) = tokio::sync::mpsc::channel(128);

        tokio::spawn(async move {
            loop {
                match subscription.recv().await {
                    Ok(chunk) => {
                        let timestamp = chunk
                            .timestamp
                            .duration_since(UNIX_EPOCH)
                            .map(|d| d.as_millis() as i64)
                            .unwrap_or(0);

                        let output_chunk = TerminalOutputChunk {
                            seq: chunk.seq,
                            data: chunk.text.into_bytes(),
                            timestamp,
                        };

                        if tx.send(Ok(output_chunk)).await.is_err() {
                            // Client disconnected
                            break;
                        }
                    }
                    Err(_) => {
                        // Broadcast channel closed (terminal exited)
                        break;
                    }
                }
            }
        });

        Ok(Response::new(ReceiverStream::new(rx)))
    }

    async fn health(
        &self,
        _request: Request<HealthRequest>,
    ) -> Result<Response<HealthResponse>, Status> {
        let terminals = self.process_manager.list_running().await;
        Ok(Response::new(HealthResponse {
            healthy: true,
            active_terminals: terminals.len() as u32,
        }))
    }
}
