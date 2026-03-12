//! Integration tests for CLI detection API
//!
//! These tests verify that CLI detection works through the HTTP API,
//! returning proper installation status for available and unavailable CLIs.

use server::{Deployment, DeploymentImpl};

/// Helper: Setup test environment with CLI types
async fn setup_test() -> DeploymentImpl {
    DeploymentImpl::new()
        .await
        .expect("Failed to create deployment")
}

#[tokio::test]
async fn test_cli_detection_api() {
    let _deployment = setup_test().await;

    // TODO: Implement actual CLI detection test
    // 1. Create test CLI types in database
    // 2. Call GET /api/cli_types/detect
    // 3. Verify response includes installation status
    // 4. Verify response includes version info for installed CLIs
    // 5. Verify response includes install_guide_url for uninstalled CLIs
}

#[tokio::test]
async fn test_cli_detection_returns_installed_flag() {
    let _deployment = setup_test().await;

    // TODO: Test that API returns proper CliDetectionStatus with installed flag
    // 1. Create CLI type for system command (e.g., "echo" or "cmd")
    // 2. Create CLI type for non-existent command
    // 3. Call detection API
    // 4. Assert installed=true for system command
    // 5. Assert installed=false for non-existent command
}
