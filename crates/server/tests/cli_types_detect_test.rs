//! Test for CLI types detection API
//!
//! This test verifies that:
//! 1. The CliDetector import path is correct
//! 2. Arc<DBService> can be properly created from deployment

use std::sync::Arc;

use server::{Deployment, DeploymentImpl};

#[tokio::test]
async fn test_cli_detector_arc_creation() {
    // Create a deployment instance
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");

    // This should fail initially because:
    // 1. Import path is wrong (services::terminal::detector vs services::services::terminal::detector)
    // 2. Arc::new(deployment.db()) creates Arc<&DBService> instead of Arc<DBService>
    use services::services::terminal::detector::CliDetector;

    // Try to create Arc<DBService> - this will fail with current code
    let db = Arc::new(deployment.db().clone());

    // This should work after fixes
    let _detector = CliDetector::new(db);
}

#[tokio::test]
async fn test_cli_detector_correct_import_path() {
    // Verify the correct import path works
    use services::services::terminal::detector::CliDetector;

    // Create a deployment
    let deployment = DeploymentImpl::new()
        .await
        .expect("Failed to create deployment");

    // Create Arc<DBService> properly
    let db = Arc::new(deployment.db().clone());

    // Create detector - this tests both fixes:
    // 1. Correct import path
    // 2. Proper Arc<DBService> creation
    let _detector = CliDetector::new(db);
}
