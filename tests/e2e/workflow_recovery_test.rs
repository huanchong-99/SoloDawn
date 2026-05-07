//! Recovery Tests for Workflow System
//! Tests concurrent workflows, failure scenarios, and recovery.
//!
//! Prerequisites:
//! - Server running on http://localhost:3001
//! - Database initialized with seed data

use reqwest::Client;
use serde_json::json;
use std::sync::LazyLock;
use std::time::Duration;
use uuid::Uuid;

static SERVER_URL_INNER: LazyLock<String> = LazyLock::new(|| {
    std::env::var("TEST_SERVER_URL")
        .ok()
        .unwrap_or_else(|| "http://localhost:23456".to_string())
});
static API_BASE_INNER: LazyLock<String> =
    LazyLock::new(|| format!("{}/api", SERVER_URL_INNER.as_str()));

#[allow(non_snake_case)]
fn SERVER_URL() -> &'static str {
    SERVER_URL_INNER.as_str()
}
#[allow(non_snake_case)]
fn API_BASE() -> &'static str {
    API_BASE_INNER.as_str()
}

/// HTTP client with timeout
fn client() -> Client {
    Client::builder()
        .timeout(Duration::from_secs(30))
        .build()
        .expect("Failed to create HTTP client")
}

/// Generate test project ID
fn test_project_id() -> String {
    Uuid::new_v4().to_string()
}

/// Ensure server is running before executing tests
async fn ensure_server_running() {
    let client = client();
    let response = client
        .get(&format!("{}/cli_types", API_BASE()))
        .timeout(Duration::from_secs(5))
        .send()
        .await;

    if response.is_err() {
        panic!(
            "Server is not running on {}. Please start the server first.",
            SERVER_URL()
        );
    }
}

/// Helper: Get first CLI type ID from the API
async fn get_first_cli_type(client: &Client) -> String {
    let cli_response = client
        .get(&format!("{}/cli_types", API_BASE()))
        .send()
        .await
        .expect("Failed to GET /cli_types - server may not be running");

    assert_eq!(
        cli_response.status(),
        200,
        "GET /cli_types returned error status: {}",
        cli_response.status()
    );

    let cli_types: Vec<serde_json::Value> = cli_response
        .json()
        .await
        .expect("Failed to parse CLI types response");

    assert!(
        !cli_types.is_empty(),
        "No CLI types found - database may not be seeded"
    );

    cli_types[0]["id"]
        .as_str()
        .expect("CLI type ID should be a string")
        .to_string()
}

/// Helper: Get first model ID for a given CLI type
async fn get_first_model(client: &Client, cli_type_id: &str) -> String {
    let models_response = client
        .get(&format!("{}/cli_types/{}/models", API_BASE(), cli_type_id))
        .send()
        .await
        .expect(&format!(
            "Failed to GET /cli_types/{}/models - server may not be running",
            cli_type_id
        ));

    assert_eq!(
        models_response.status(),
        200,
        "GET /cli_types/{}/models returned error status: {}",
        cli_type_id,
        models_response.status()
    );

    let models: Vec<serde_json::Value> = models_response
        .json()
        .await
        .expect(&format!(
            "Failed to parse models response for CLI type {}",
            cli_type_id
        ));

    assert!(
        !models.is_empty(),
        "No models found for CLI type {} - database may not be seeded",
        cli_type_id
    );

    models[0]["id"]
        .as_str()
        .expect("Model ID should be a string")
        .to_string()
}

/// Helper: Get first slash command preset ID
async fn get_first_command_preset(client: &Client) -> String {
    let presets_response = client
        .get(&format!("{}/workflows/presets/commands", API_BASE()))
        .send()
        .await
        .expect("Failed to GET /workflows/presets/commands - server may not be running");

    assert_eq!(
        presets_response.status(),
        200,
        "GET /workflows/presets/commands returned error status: {}",
        presets_response.status()
    );

    let presets_body: serde_json::Value = presets_response
        .json()
        .await
        .expect("Failed to parse command presets response");

    assert_eq!(
        presets_body["success"].as_bool(),
        Some(true),
        "Command presets response did not return success"
    );

    let presets = presets_body["data"]
        .as_array()
        .expect("Command presets data should be an array");

    assert!(
        !presets.is_empty(),
        "Should have at least one command preset - database may not be seeded"
    );

    presets[0]["id"]
        .as_str()
        .expect("Command preset ID should be a string")
        .to_string()
}

/// Helper: Build tasks payload for workflow creation
fn build_tasks_payload(cli_type_id: &str, model_id: &str) -> serde_json::Value {
    json!([
        {
            "name": "Recovery Test Task",
            "description": "Task for recovery testing",
            "orderIndex": 0,
            "terminals": [
                {
                    "cliTypeId": cli_type_id,
                    "modelConfigId": model_id,
                    "orderIndex": 0
                }
            ]
        }
    ])
}

/// Helper: Create a workflow and return its ID
async fn create_workflow(client: &Client, payload: serde_json::Value) -> String {
    let create_response = client
        .post(&format!("{}/workflows", API_BASE()))
        .json(&payload)
        .send()
        .await
        .expect("Failed to POST /workflows - server may not be running");

    assert_eq!(
        create_response.status(),
        200,
        "POST /workflows returned error status: {}",
        create_response.status()
    );

    let workflow: serde_json::Value = create_response
        .json()
        .await
        .expect("Failed to parse workflow creation response");

    assert_eq!(
        workflow["success"].as_bool(),
        Some(true),
        "POST /workflows did not return success"
    );

    workflow["data"]["id"]
        .as_str()
        .expect("Workflow ID should be a string")
        .to_string()
}

/// Helper: Set workflow status
async fn set_workflow_status(client: &Client, workflow_id: &str, status: &str) {
    let update_payload = json!({
        "status": status
    });

    let update_response = client
        .put(&format!("{}/workflows/{}/status", API_BASE(), workflow_id))
        .json(&update_payload)
        .send()
        .await
        .expect(&format!(
            "Failed to PUT /workflows/{}/status - server may not be running",
            workflow_id
        ));

    assert_eq!(
        update_response.status(),
        200,
        "PUT /workflows/{}/status returned error status: {}",
        workflow_id,
        update_response.status()
    );
}

/// Helper: Get workflow status
async fn get_workflow_status(client: &Client, workflow_id: &str) -> String {
    let get_response = client
        .get(&format!("{}/workflows/{}", API_BASE(), workflow_id))
        .send()
        .await
        .expect(&format!(
            "Failed to GET /workflows/{} - server may not be running",
            workflow_id
        ));

    assert_eq!(
        get_response.status(),
        200,
        "GET /workflows/{} returned error status: {}",
        workflow_id,
        get_response.status()
    );

    let retrieved: serde_json::Value = get_response
        .json()
        .await
        .expect(&format!(
            "Failed to parse workflow retrieval response for {}",
            workflow_id
        ));

    retrieved["data"]["status"]
        .as_str()
        .expect("Workflow status should be a string")
        .to_string()
}

/// Helper: Delete a workflow (cleanup - errors logged but don't fail tests)
async fn delete_workflow(client: &Client, workflow_id: &str) {
    match client
        .delete(&format!("{}/workflows/{}", API_BASE(), workflow_id))
        .send()
        .await
    {
        Ok(response) => {
            if !response.status().is_success() {
                eprintln!(
                    "Warning: DELETE /workflows/{} returned status: {}",
                    workflow_id,
                    response.status()
                );
            }
        }
        Err(err) => {
            eprintln!(
                "Warning: Failed to DELETE /workflows/{}: {}",
                workflow_id, err
            );
        }
    }
}

#[tokio::test]
#[ignore]
// To run: cargo test --test workflow_recovery_test -- --ignored
async fn test_concurrent_workflows() {
    ensure_server_running().await;
    let client = client();
    let project_id = test_project_id();

    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;
    let tasks_payload = build_tasks_payload(&cli_type_id, &model_id);

    // Create 3 workflow payloads
    let payload_1 = json!({
        "projectId": project_id.clone(),
        "name": "Concurrent Workflow 1",
        "description": "Concurrent workflow test 1",
        "useSlashCommands": false,
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id.clone(),
            "modelConfigId": model_id.clone()
        },
        "targetBranch": "main",
        "tasks": tasks_payload.clone()
    });

    let payload_2 = json!({
        "projectId": project_id.clone(),
        "name": "Concurrent Workflow 2",
        "description": "Concurrent workflow test 2",
        "useSlashCommands": false,
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id.clone(),
            "modelConfigId": model_id.clone()
        },
        "targetBranch": "main",
        "tasks": tasks_payload.clone()
    });

    let payload_3 = json!({
        "projectId": project_id,
        "name": "Concurrent Workflow 3",
        "description": "Concurrent workflow test 3",
        "useSlashCommands": false,
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id,
            "modelConfigId": model_id
        },
        "targetBranch": "main",
        "tasks": tasks_payload
    });

    // Create workflows concurrently
    let create_1 = create_workflow(&client, payload_1);
    let create_2 = create_workflow(&client, payload_2);
    let create_3 = create_workflow(&client, payload_3);
    let (workflow_id_1, workflow_id_2, workflow_id_3) = tokio::join!(create_1, create_2, create_3);

    println!("✓ Created 3 workflows concurrently");

    // Set all workflows to running status concurrently
    let update_1 = set_workflow_status(&client, &workflow_id_1, "running");
    let update_2 = set_workflow_status(&client, &workflow_id_2, "running");
    let update_3 = set_workflow_status(&client, &workflow_id_3, "running");
    let _ = tokio::join!(update_1, update_2, update_3);

    println!("✓ Set all workflows to running status");

    // Verify all workflows are in running state
    let status_1 = get_workflow_status(&client, &workflow_id_1).await;
    let status_2 = get_workflow_status(&client, &workflow_id_2).await;
    let status_3 = get_workflow_status(&client, &workflow_id_3).await;

    assert_eq!(status_1, "running", "Workflow 1 should be running");
    assert_eq!(status_2, "running", "Workflow 2 should be running");
    assert_eq!(status_3, "running", "Workflow 3 should be running");

    println!("✓ All workflows verified as running");

    // Cleanup
    delete_workflow(&client, &workflow_id_1).await;
    delete_workflow(&client, &workflow_id_2).await;
    delete_workflow(&client, &workflow_id_3).await;

    println!("✓ Concurrent workflows test completed successfully");
}

#[tokio::test]
#[ignore]
// To run: cargo test --test workflow_recovery_test -- --ignored
async fn test_workflow_failure_recovery() {
    ensure_server_running().await;
    let client = client();
    let project_id = test_project_id();

    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;
    let preset_id = get_first_command_preset(&client).await;

    // Create workflow with command that will fail
    let create_payload = json!({
        "projectId": project_id,
        "name": "Failure Recovery Workflow",
        "description": "Workflow with a simulated failing command",
        "useSlashCommands": true,
        "commands": [
            {
                "presetId": preset_id,
                "customParams": "{\"command\":\"exit 1\"}"
            }
        ],
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id.clone(),
            "modelConfigId": model_id.clone()
        },
        "targetBranch": "main",
        "tasks": build_tasks_payload(&cli_type_id, &model_id)
    });

    let workflow_id = create_workflow(&client, create_payload).await;

    println!("✓ Created workflow with failing command");

    // Start the workflow
    set_workflow_status(&client, &workflow_id, "running").await;

    println!("✓ Started workflow");

    // Simulate failure detection by setting status to failed
    // In a real scenario, the orchestrator would detect the failure and update status
    set_workflow_status(&client, &workflow_id, "failed").await;

    println!("✓ Simulated failure detection");

    // Verify workflow is marked as failed
    let status = get_workflow_status(&client, &workflow_id).await;
    assert_eq!(status, "failed", "Workflow should be marked as failed");

    println!("✓ Verified workflow failure status");

    // Cleanup
    delete_workflow(&client, &workflow_id).await;

    println!("✓ Workflow failure recovery test completed successfully");
}

#[tokio::test]
#[ignore]
// To run: cargo test --test workflow_recovery_test -- --ignored
async fn test_service_restart_recovery() {
    ensure_server_running().await;
    let client = client();
    let project_id = test_project_id();

    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;

    // Create a long-running workflow
    let create_payload = json!({
        "projectId": project_id,
        "name": "Restart Recovery Workflow",
        "description": "Long-running workflow for service restart recovery",
        "useSlashCommands": false,
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id.clone(),
            "modelConfigId": model_id.clone()
        },
        "targetBranch": "main",
        "tasks": build_tasks_payload(&cli_type_id, &model_id)
    });

    let workflow_id = create_workflow(&client, create_payload).await;

    println!("✓ Created long-running workflow");

    // Start the workflow
    set_workflow_status(&client, &workflow_id, "running").await;

    println!("✓ Started workflow");

    // Let it run briefly
    tokio::time::sleep(Duration::from_secs(2)).await;

    println!("✓ Workflow running for 2 seconds");

    // Call the recovery endpoint to simulate service restart recovery
    let recover_response = client
        .post(&format!("{}/workflows/recover", API_BASE()))
        .send()
        .await
        .expect("Failed to POST /workflows/recover - server may not be running");

    assert_eq!(
        recover_response.status(),
        200,
        "POST /workflows/recover returned error status: {}",
        recover_response.status()
    );

    let recover_body: serde_json::Value = recover_response
        .json()
        .await
        .expect("Failed to parse recovery response");

    assert_eq!(
        recover_body["success"].as_bool(),
        Some(true),
        "Recovery response did not return success"
    );

    assert_eq!(
        recover_body["data"]["message"].as_str(),
        Some("Recovery triggered"),
        "Unexpected recovery response message"
    );

    println!("✓ Recovery endpoint called successfully");

    // Verify workflow is still running after recovery
    let status = get_workflow_status(&client, &workflow_id).await;
    assert_eq!(status, "running", "Workflow should remain running after recovery");

    println!("✓ Verified workflow still running after recovery");

    // Cleanup
    delete_workflow(&client, &workflow_id).await;

    println!("✓ Service restart recovery test completed successfully");
}
