use std::sync::Arc;

use tonic::transport::Server;
use tracing_subscriber::EnvFilter;

use runner::proto::runner_service_server::RunnerServiceServer;
use runner::service::RunnerGrpcService;
use services::terminal::process::ProcessManager;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // Initialize tracing
    tracing_subscriber::fmt()
        .with_env_filter(
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info")),
        )
        .init();

    let addr = std::env::var("GITCORTEX_RUNNER_ADDR")
        .unwrap_or_else(|_| "0.0.0.0:50051".to_string())
        .parse()?;

    let process_manager = Arc::new(ProcessManager::new());
    let service = RunnerGrpcService::new(process_manager);

    tracing::info!(%addr, "Runner gRPC server starting");

    Server::builder()
        .add_service(RunnerServiceServer::new(service))
        .serve_with_shutdown(addr, shutdown_signal())
        .await?;

    tracing::info!("Runner gRPC server stopped");
    Ok(())
}

async fn shutdown_signal() {
    let ctrl_c = async {
        tokio::signal::ctrl_c()
            .await
            .expect("Failed to install Ctrl+C handler");
    };

    #[cfg(unix)]
    let terminate = async {
        tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("Failed to install SIGTERM handler")
            .recv()
            .await;
    };

    #[cfg(not(unix))]
    let terminate = std::future::pending::<()>();

    tokio::select! {
        () = ctrl_c => tracing::info!("Received SIGINT, shutting down"),
        () = terminate => tracing::info!("Received SIGTERM, shutting down"),
    }
}
