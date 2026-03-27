//! Security Integration Tests
//!
//! Tests security-related functionality including:
//! - API key not exposed in API responses
//! - API key encryption in database
//!
//! Prerequisites:
//! - Server running on http://localhost:3001
//! - Database initialized with seed data
//! - Server MUST be started with SOLODAWN_ENCRYPTION_KEY environment variable set
//!   Example: SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012" cargo run --bin server
//!
//! IMPORTANT: The test process sets SOLODAWN_ENCRYPTION_KEY for its own direct DB access,
//! but the server process must have been started with the SAME key for proper encryption verification.

use std::time::Duration;

use db::DBService;
use reqwest::Client;
use serde_json::{Value, json};
use uuid::Uuid;

const SERVER_URL: &str = "http://localhost:3001";
const API_BASE: &str = "http://localhost:3001/api";
/// 32-byte encryption key (256 bits) for AES-256-GCM
/// IMPORTANT: This must match the key used to start the server
const ENCRYPTION_KEY: &str = "12345678901234567890123456789012";

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

/// Get test API key from environment or use fallback
fn get_test_api_key() -> String {
    std::env::var("TEST_ORCHESTRATOR_API_KEY").unwrap_or_else(|_| "sk-test-key-12345".to_string())
}

/// Set encryption key for tests
fn ensure_encryption_key() {
    unsafe { std::env::set_var("SOLODAWN_ENCRYPTION_KEY", ENCRYPTION_KEY) };
}

/// Ensure server is running before executing tests
async fn ensure_server_running() {
    let client = client();
    let response = client
        .get(format!("{}/cli_types", API_BASE))
        .timeout(Duration::from_secs(5))
        .send()
        .await;

    if response.is_err() {
        panic!(
            "Server is not running on {}. Please start the server first.",
            SERVER_URL
        );
    }
}

/// Helper: Get first CLI type ID from the API
async fn get_first_cli_type(client: &Client) -> String {
    let cli_response = client
        .get(format!("{}/cli_types", API_BASE))
        .send()
        .await
        .expect("Failed to GET /cli_types - server may not be running");

    assert_eq!(
        cli_response.status(),
        200,
        "GET /cli_types returned error status: {}",
        cli_response.status()
    );

    let cli_types: Vec<Value> = cli_response
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
        .get(format!("{}/cli_types/{}/models", API_BASE, cli_type_id))
        .send()
        .await
        .unwrap_or_else(|e| panic!(
            "Failed to GET /cli_types/{}/models - server may not be running: {}",
            cli_type_id, e
        ));

    assert_eq!(
        models_response.status(),
        200,
        "GET /cli_types/{}/models returned error status: {}",
        cli_type_id,
        models_response.status()
    );

    let models: Vec<Value> = models_response
        .json()
        .await
        .expect("Failed to parse models response");

    assert!(
        !models.is_empty(),
        "No models found for CLI type {}",
        cli_type_id
    );

    models[0]["id"]
        .as_str()
        .expect("Model ID should be a string")
        .to_string()
}

/// Extract workflow ID from various response formats
fn extract_workflow_id(response: &Value) -> Option<String> {
    response
        .pointer("/data/id")
        .and_then(|v| v.as_str())
        .or_else(|| {
            response
                .pointer("/data/workflow/id")
                .and_then(|v| v.as_str())
        })
        .or_else(|| response.pointer("/workflow/id").and_then(|v| v.as_str()))
        .map(|id| id.to_string())
}

#[tokio::test]
async fn test_api_key_not_exposed_in_api_response() {
    ensure_server_running().await;
    ensure_encryption_key();
    let client = client();

    let project_id = test_project_id();
    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;
    let api_key = get_test_api_key();

    let payload = json!({
        "projectId": project_id,
        "name": "Security Test Workflow",
        "description": "Security test for API key exposure",
        "useSlashCommands": false,
        "orchestratorConfig": {
            "apiType": "anthropic",
            "baseUrl": "https://api.anthropic.com",
            "apiKey": api_key,
            "model": "claude-sonnet-4-20250514"
        },
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id,
            "modelConfigId": model_id
        },
        "targetBranch": "main",
        "tasks": [
            {
                "name": "Security Task",
                "description": "Security task",
                "orderIndex": 0,
                "terminals": [
                    {
                        "cliTypeId": cli_type_id,
                        "modelConfigId": model_id,
                        "orderIndex": 0
                    }
                ]
            }
        ]
    });

    let response = client
        .post(format!("{}/workflows", API_BASE))
        .json(&payload)
        .send()
        .await
        .expect("Failed to create workflow");

    let status = response.status();
    let response_text = response.text().await.expect("Failed to read response");
    assert_eq!(
        status, 200,
        "Workflow creation failed with status: {}",
        status
    );

    // Check that API key is not exposed in response
    assert!(
        !response_text.contains("orchestratorApiKey"),
        "API response exposed orchestratorApiKey"
    );
    assert!(
        !response_text.contains("orchestrator_api_key"),
        "API response exposed orchestrator_api_key"
    );
    assert!(
        !response_text.contains(&get_test_api_key()),
        "API response exposed plaintext API key"
    );

    // Parse response and extract workflow ID
    let response_json: Value =
        serde_json::from_str(&response_text).expect("Failed to parse workflow response JSON");
    let workflow_id =
        extract_workflow_id(&response_json).expect("Workflow ID not found in response");

    // Verify GET request also doesn't expose the key
    let get_response = client
        .get(format!("{}/workflows/{}", API_BASE, workflow_id))
        .send()
        .await
        .expect("Failed to get workflow");

    let get_text = get_response
        .text()
        .await
        .expect("Failed to read get response");
    assert!(
        !get_text.contains("orchestratorApiKey"),
        "GET response exposed orchestratorApiKey"
    );
    assert!(
        !get_text.contains("orchestrator_api_key"),
        "GET response exposed orchestrator_api_key"
    );
    assert!(
        !get_text.contains(&get_test_api_key()),
        "GET response exposed plaintext API key"
    );

    // Cleanup
    let _ = client
        .delete(format!("{}/workflows/{}", API_BASE, workflow_id))
        .send()
        .await;
}

#[tokio::test]
async fn test_api_key_encrypted_in_database() {
    ensure_server_running().await;
    ensure_encryption_key();
    let client = client();

    let project_id = test_project_id();
    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;
    let api_key = get_test_api_key();

    let payload = json!({
        "projectId": project_id,
        "name": "Security Test Workflow (DB)",
        "description": "Security test for DB encryption",
        "useSlashCommands": false,
        "orchestratorConfig": {
            "apiType": "anthropic",
            "baseUrl": "https://api.anthropic.com",
            "apiKey": api_key,
            "model": "claude-sonnet-4-20250514"
        },
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id,
            "modelConfigId": model_id
        },
        "targetBranch": "main",
        "tasks": [
            {
                "name": "Security Task",
                "description": "Security task",
                "orderIndex": 0,
                "terminals": [
                    {
                        "cliTypeId": cli_type_id,
                        "modelConfigId": model_id,
                        "orderIndex": 0
                    }
                ]
            }
        ]
    });

    let response = client
        .post(format!("{}/workflows", API_BASE))
        .json(&payload)
        .send()
        .await
        .expect("Failed to create workflow");

    let status = response.status();
    let response_text = response.text().await.expect("Failed to read response");
    assert_eq!(
        status, 200,
        "Workflow creation failed with status: {}",
        status
    );

    // Parse response and extract workflow ID
    let response_json: Value =
        serde_json::from_str(&response_text).expect("Failed to parse workflow response JSON");
    let workflow_id =
        extract_workflow_id(&response_json).expect("Workflow ID not found in response");

    // Query database directly to verify encryption
    let db = DBService::new().await.expect("Failed to open database");

    let workflow = db::models::Workflow::find_by_id(&db.pool, &workflow_id)
        .await
        .expect("Failed to query workflow")
        .expect("Workflow not found in database");

    // Verify the encrypted value is stored
    let encrypted = workflow
        .orchestrator_api_key
        .clone()
        .expect("Encrypted API key not stored");

    assert_ne!(
        encrypted, api_key,
        "API key should be encrypted at rest (stored value differs from plaintext)"
    );

    // Verify decryption works correctly
    let decrypted = workflow
        .get_api_key()
        .expect("Failed to decrypt API key")
        .expect("Decrypted API key missing");

    assert_eq!(
        decrypted, api_key,
        "Decrypted API key should match the original plaintext key"
    );

    // Cleanup
    let _ = client
        .delete(format!("{}/workflows/{}", API_BASE, workflow_id))
        .send()
        .await;
}

#[tokio::test]
async fn test_terminal_api_key_encrypted_in_database() {
    ensure_server_running().await;
    ensure_encryption_key();
    let client = client();

    let project_id = test_project_id();
    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;
    let terminal_api_key = "sk-terminal-test-key-12345";

    let payload = json!({
        "projectId": project_id,
        "name": "Terminal Security Test Workflow",
        "description": "Security test for terminal API key encryption",
        "useSlashCommands": false,
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id,
            "modelConfigId": model_id
        },
        "targetBranch": "main",
        "tasks": [
            {
                "name": "Terminal Security Task",
                "description": "Terminal security task",
                "orderIndex": 0,
                "terminals": [
                    {
                        "cliTypeId": cli_type_id,
                        "modelConfigId": model_id,
                        "customApiKey": terminal_api_key,
                        "orderIndex": 0
                    }
                ]
            }
        ]
    });

    let response = client
        .post(format!("{}/workflows", API_BASE))
        .json(&payload)
        .send()
        .await
        .expect("Failed to create workflow");

    let status = response.status();
    let response_text = response.text().await.expect("Failed to read response");
    assert_eq!(
        status, 200,
        "Workflow creation failed with status: {}",
        status
    );

    // Check that terminal API key is not exposed in response
    assert!(
        !response_text.contains("customApiKey"),
        "API response exposed customApiKey"
    );
    assert!(
        !response_text.contains("custom_api_key"),
        "API response exposed custom_api_key"
    );
    assert!(
        !response_text.contains(terminal_api_key),
        "API response exposed plaintext terminal API key"
    );

    // Parse response and extract workflow ID
    let response_json: Value =
        serde_json::from_str(&response_text).expect("Failed to parse workflow response JSON");
    let workflow_id =
        extract_workflow_id(&response_json).expect("Workflow ID not found in response");

    // Query database directly to verify encryption
    let db = DBService::new().await.expect("Failed to open database");

    // Get the first terminal from the first task
    let tasks = db::models::WorkflowTask::find_by_workflow(&db.pool, &workflow_id)
        .await
        .expect("Failed to query tasks");

    assert!(!tasks.is_empty(), "No tasks found for workflow");

    let terminals = db::models::Terminal::find_by_task(&db.pool, &tasks[0].id)
        .await
        .expect("Failed to query terminals");

    assert!(!terminals.is_empty(), "No terminals found for task");

    let terminal = &terminals[0];

    // Verify the encrypted value is stored
    let encrypted = terminal
        .custom_api_key
        .as_ref()
        .expect("Encrypted terminal API key not stored");

    assert_ne!(
        encrypted, terminal_api_key,
        "Terminal API key should be encrypted at rest (stored value differs from plaintext)"
    );

    // Verify decryption works correctly
    let decrypted = terminal
        .get_custom_api_key()
        .expect("Failed to decrypt terminal API key")
        .expect("Decrypted terminal API key missing");

    assert_eq!(
        decrypted, terminal_api_key,
        "Decrypted terminal API key should match the original plaintext key"
    );

    // Verify GET request also doesn't expose terminal API keys
    let get_response = client
        .get(format!("{}/workflows/{}", API_BASE, workflow_id))
        .send()
        .await
        .expect("Failed to get workflow");

    let get_text = get_response
        .text()
        .await
        .expect("Failed to read get response");
    assert!(
        !get_text.contains(terminal_api_key),
        "GET response exposed plaintext terminal API key"
    );

    // Cleanup
    let _ = client
        .delete(format!("{}/workflows/{}", API_BASE, workflow_id))
        .send()
        .await;
}

#[tokio::test]
async fn test_terminal_api_key_not_exposed_in_dto() {
    ensure_server_running().await;
    ensure_encryption_key();
    let client = client();

    let project_id = test_project_id();
    let cli_type_id = get_first_cli_type(&client).await;
    let model_id = get_first_model(&client, &cli_type_id).await;

    let payload = json!({
        "projectId": project_id,
        "name": "Terminal DTO Security Test",
        "description": "Security test for terminal DTO",
        "useSlashCommands": false,
        "mergeTerminalConfig": {
            "cliTypeId": cli_type_id,
            "modelConfigId": model_id
        },
        "targetBranch": "main",
        "tasks": [
            {
                "name": "DTO Security Task",
                "description": "DTO security task",
                "orderIndex": 0,
                "terminals": [
                    {
                        "cliTypeId": cli_type_id,
                        "modelConfigId": model_id,
                        "customApiKey": "sk-test-dto-key",
                        "orderIndex": 0
                    }
                ]
            }
        ]
    });

    let response = client
        .post(format!("{}/workflows", API_BASE))
        .json(&payload)
        .send()
        .await
        .expect("Failed to create workflow");

    let status = response.status();
    let response_text = response.text().await.expect("Failed to read response");
    assert_eq!(status, 200, "Workflow creation failed: {}", response_text);

    // Parse response
    let response_json: Value =
        serde_json::from_str(&response_text).expect("Failed to parse workflow response JSON");

    // Check that terminals array exists and doesn't expose API keys
    if let Some(tasks) = response_json.pointer("/data/tasks") {
        if let Some(terminals) = tasks[0].pointer("/terminals") {
            if let Some(terminal) = terminals.as_array().and_then(|t| t.first()) {
                // Verify customApiKey is null or not present
                let api_key = terminal.pointer("/customApiKey");
                assert!(
                    api_key.is_none()
                        || api_key
                            .and_then(|v| v.as_str())
                            .map(|s| s.is_empty())
                            .unwrap_or(false),
                    "Terminal DTO exposed customApiKey: {:?}",
                    api_key
                );
            }
        }
    }

    // Cleanup
    if let Some(workflow_id) = extract_workflow_id(&response_json) {
        let _ = client
            .delete(format!("{}/workflows/{}", API_BASE, workflow_id))
            .send()
            .await;
    }
}
