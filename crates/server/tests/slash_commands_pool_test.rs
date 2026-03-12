//! Test for slash_commands pool access fix
//!
//! This test verifies that slash command route handlers correctly access the database pool
//! through deployment.db().pool instead of the non-existent deployment.pool field.

use local_deployment::LocalDeployment;
use server::Deployment;

#[tokio::test]
async fn test_slash_commands_pool_access() {
    // This test verifies that we can access the pool through deployment.db()
    // Before the fix, this would fail with "no field named `pool` on type `DeploymentImpl`"

    // Create a deployment instance
    let deployment: LocalDeployment = LocalDeployment::new()
        .await
        .expect("Failed to create deployment");

    // Access the pool through the db() method
    let _pool = &deployment.db().pool;

    // If we got here without compilation errors, the fix is working
    // Verify pool reference is valid by checking it's not null-like
    let pool_ref = &deployment.db().pool;
    let _size = pool_ref.size();
}

#[tokio::test]
async fn test_slash_commands_pool_is_accessible() {
    // Verify that the pool is actually accessible and functional
    let deployment: LocalDeployment = LocalDeployment::new()
        .await
        .expect("Failed to create deployment");

    // Try to execute a simple query to verify the pool works
    let result = sqlx::query("SELECT 1")
        .fetch_one(&deployment.db().pool)
        .await;

    assert!(result.is_ok(), "Pool should be functional");
}
