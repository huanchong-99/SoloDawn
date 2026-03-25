//! Orchestration E2E tests — exercises the full terminal-spawn-inject-execute
//! pipeline with a real AI CLI and real API key.
//!
//! These tests are opt-in: they only run when `E2E_API_KEY` env var is set.
//! On CI, the key comes from GitHub Secrets. Locally, set it manually.

use std::path::Path;
use std::time::{Duration, Instant};

use serde_json::{Value, json};

use super::TestResult;

/// Check if orchestration tests should run (API key available).
fn orchestration_enabled() -> bool {
    std::env::var("E2E_API_KEY")
        .map(|v| !v.is_empty())
        .unwrap_or(false)
}

fn e2e_api_key() -> String {
    std::env::var("E2E_API_KEY").unwrap_or_default()
}

fn e2e_base_url() -> String {
    std::env::var("E2E_BASE_URL")
        .unwrap_or_else(|_| "https://open.bigmodel.cn/api/anthropic".to_string())
}

fn e2e_model() -> String {
    std::env::var("E2E_MODEL").unwrap_or_else(|_| "glm-5".to_string())
}

/// Run all orchestration E2E tests. Returns results for each test.
pub async fn run_orchestration_tests(
    base_url: &str,
    temp_dir: &Path,
) -> Vec<TestResult> {
    if !orchestration_enabled() {
        eprintln!("Orchestration tests SKIPPED — E2E_API_KEY not set");
        return vec![TestResult {
            name: "orchestration_skipped".to_string(),
            group: "orchestration".to_string(),
            passed: true,
            duration_ms: 0,
            error: Some("Skipped: E2E_API_KEY not set".to_string()),
        }];
    }

    eprintln!("Running orchestration E2E tests...");
    eprintln!("  Model: {}", e2e_model());
    eprintln!("  Base URL: {}", e2e_base_url());

    let client = reqwest::Client::builder()
        .timeout(Duration::from_secs(60))
        .build()
        .expect("Failed to build reqwest client");

    let api = |path: &str| format!("{base_url}/api{path}");

    let mut results = Vec::new();

    // Test 1: CLI Installation Verification
    let start = Instant::now();
    let r = test_cli_installed().await;
    results.push(TestResult {
        name: "cli_installed".to_string(),
        group: "orchestration".to_string(),
        passed: r.is_ok(),
        duration_ms: start.elapsed().as_millis() as u64,
        error: r.err(),
    });

    // If CLI not installed, skip remaining tests
    if !results.last().map_or(false, |r| r.passed) {
        eprintln!("Claude Code not installed — skipping remaining orchestration tests");
        return results;
    }

    // Test 2: Configure model credentials for GLM-5
    let start = Instant::now();
    let model_config_id = test_configure_model(&client, &api).await;
    results.push(TestResult {
        name: "configure_model".to_string(),
        group: "orchestration".to_string(),
        passed: model_config_id.is_ok(),
        duration_ms: start.elapsed().as_millis() as u64,
        error: model_config_id.as_ref().err().cloned(),
    });

    let model_config_id = match model_config_id {
        Ok(id) => id,
        Err(_) => return results,
    };

    // Test 3: Setup git repo
    let repo_path = temp_dir.join("orch-test-repo");
    let start = Instant::now();
    let repo_setup = setup_git_repo(&repo_path).await;
    results.push(TestResult {
        name: "setup_git_repo".to_string(),
        group: "orchestration".to_string(),
        passed: repo_setup.is_ok(),
        duration_ms: start.elapsed().as_millis() as u64,
        error: repo_setup.err(),
    });

    if !results.last().map_or(false, |r| r.passed) {
        return results;
    }

    // Test 4: Create project
    let start = Instant::now();
    let project_id = test_create_project(&client, &api, &repo_path).await;
    results.push(TestResult {
        name: "create_project".to_string(),
        group: "orchestration".to_string(),
        passed: project_id.is_ok(),
        duration_ms: start.elapsed().as_millis() as u64,
        error: project_id.as_ref().err().cloned(),
    });

    let project_id = match project_id {
        Ok(id) => id,
        Err(_) => return results,
    };

    // Test 5: Create workflow + prepare + start + monitor
    let start = Instant::now();
    let workflow_result = test_full_workflow(
        &client,
        &api,
        &project_id,
        &model_config_id,
        &repo_path,
    )
    .await;
    results.push(TestResult {
        name: "full_workflow_execution".to_string(),
        group: "orchestration".to_string(),
        passed: workflow_result.is_ok(),
        duration_ms: start.elapsed().as_millis() as u64,
        error: workflow_result.err(),
    });

    // Cleanup project
    let _ = client.delete(api(&format!("/projects/{project_id}"))).send().await;

    results
}

// ============================================================================
// Individual test implementations
// ============================================================================

/// Find the `claude` binary, checking PATH and common npm global locations.
fn find_claude_binary() -> Result<String, String> {
    // On Windows, try `claude.cmd` first (npm installs .cmd wrappers)
    if cfg!(windows) {
        // Try claude.cmd via PATH
        if let Ok(output) = std::process::Command::new("cmd")
            .args(["/C", "claude", "--version"])
            .output()
        {
            if output.status.success() {
                eprintln!("  Found claude via cmd /C");
                return Ok("claude".to_string());
            }
        }

        // Check common npm global install locations
        let candidates = [
            std::env::var("APPDATA")
                .map(|d| format!("{d}\\npm\\claude.cmd"))
                .unwrap_or_default(),
            // npm prefix -g location on CI runners
            "C:\\npm\\prefix\\claude.cmd".to_string(),
        ];

        for candidate in &candidates {
            if !candidate.is_empty() && std::path::Path::new(candidate).exists() {
                eprintln!("  Found claude at: {candidate}");
                return Ok(candidate.clone());
            }
        }

        // Try `where claude` to find it in PATH
        if let Ok(output) = std::process::Command::new("where").arg("claude").output() {
            if output.status.success() {
                let path = String::from_utf8_lossy(&output.stdout);
                let first_line = path.lines().next().unwrap_or("").trim();
                if !first_line.is_empty() {
                    eprintln!("  Found claude via `where`: {first_line}");
                    return Ok(first_line.to_string());
                }
            }
        }
    } else {
        // Unix: try directly
        if let Ok(output) = std::process::Command::new("claude").arg("--version").output() {
            if output.status.success() {
                return Ok("claude".to_string());
            }
        }
    }

    Err("Claude Code CLI not found in PATH or common locations. Is it installed?".to_string())
}

async fn test_cli_installed() -> Result<(), String> {
    let claude_bin = find_claude_binary()?;

    // On Windows, run through cmd /C to resolve .cmd wrappers
    let output = if cfg!(windows) {
        std::process::Command::new("cmd")
            .args(["/C", &claude_bin, "--version"])
            .output()
    } else {
        std::process::Command::new(&claude_bin)
            .arg("--version")
            .output()
    }
    .map_err(|e| format!("Failed to run `{claude_bin} --version`: {e}"))?;

    if !output.status.success() {
        let stderr = String::from_utf8_lossy(&output.stderr);
        return Err(format!(
            "`{claude_bin} --version` exited with {}: {stderr}",
            output.status
        ));
    }

    let version = String::from_utf8_lossy(&output.stdout);
    eprintln!("  Claude Code version: {}", version.trim());
    Ok(())
}

async fn test_configure_model(
    client: &reqwest::Client,
    api: &dyn Fn(&str) -> String,
) -> Result<String, String> {
    // Use the pre-seeded model-claude-sonnet and update its credentials
    // to point to ZhipuAI GLM-5 endpoint
    let model_id = "model-claude-sonnet";

    let resp = client
        .put(api(&format!(
            "/cli_types/cli-claude-code/models/{model_id}/credentials"
        )))
        .json(&json!({
            "apiModelId": e2e_model(),
            "baseUrl": e2e_base_url(),
            "apiKey": e2e_api_key(),
            "apiType": "anthropic"
        }))
        .send()
        .await
        .map_err(|e| format!("Failed to update model credentials: {e}"))?;

    let status = resp.status().as_u16();
    let body = resp.text().await.unwrap_or_default();

    if status >= 400 {
        return Err(format!(
            "Update model credentials returned {status}: {}",
            &body[..body.len().min(300)]
        ));
    }

    eprintln!("  Configured model {model_id} with GLM-5 credentials");
    Ok(model_id.to_string())
}

async fn setup_git_repo(repo_path: &Path) -> Result<(), String> {
    std::fs::create_dir_all(repo_path).map_err(|e| format!("mkdir: {e}"))?;

    let run = |args: &[&str]| -> Result<(), String> {
        let output = std::process::Command::new("git")
            .args(args)
            .current_dir(repo_path)
            .output()
            .map_err(|e| format!("git {}: {e}", args[0]))?;
        if !output.status.success() {
            return Err(format!(
                "git {} failed: {}",
                args[0],
                String::from_utf8_lossy(&output.stderr)
            ));
        }
        Ok(())
    };

    run(&["init"])?;
    run(&["config", "user.email", "e2e@gitcortex.dev"])?;
    run(&["config", "user.name", "GitCortex E2E"])?;

    // Create a README so the repo isn't empty
    std::fs::write(repo_path.join("README.md"), "# E2E Test Repo\n")
        .map_err(|e| format!("write README: {e}"))?;
    run(&["add", "."])?;
    run(&["commit", "-m", "Initial commit"])?;

    eprintln!("  Git repo initialized at {}", repo_path.display());
    Ok(())
}

async fn test_create_project(
    client: &reqwest::Client,
    api: &dyn Fn(&str) -> String,
    repo_path: &Path,
) -> Result<String, String> {
    let resp = client
        .post(api("/projects"))
        .json(&json!({
            "name": "E2E Orchestration Test",
            "repositories": [{
                "displayName": "e2e-repo",
                "gitRepoPath": repo_path.to_string_lossy()
            }]
        }))
        .send()
        .await
        .map_err(|e| format!("Create project failed: {e}"))?;

    let status = resp.status().as_u16();
    let body: Value = resp.json().await.map_err(|e| format!("Parse response: {e}"))?;

    if status >= 400 {
        return Err(format!("Create project returned {status}: {body}"));
    }

    let id = body
        .get("data")
        .and_then(|d| d.get("id"))
        .or_else(|| body.get("id"))
        .and_then(|v| v.as_str())
        .ok_or_else(|| format!("No project ID in response: {body}"))?;

    eprintln!("  Created project: {id}");
    Ok(id.to_string())
}

async fn test_full_workflow(
    client: &reqwest::Client,
    api: &dyn Fn(&str) -> String,
    project_id: &str,
    model_config_id: &str,
    repo_path: &Path,
) -> Result<(), String> {
    // Step 1: Create workflow
    eprintln!("  [workflow] Creating...");
    let resp = client
        .post(api("/workflows"))
        .json(&json!({
            "projectId": project_id,
            "name": "E2E Orchestration Test Workflow",
            "executionMode": "diy",
            "useSlashCommands": false,
            "orchestratorConfig": {
                "apiType": "anthropic",
                "baseUrl": e2e_base_url(),
                "apiKey": e2e_api_key(),
                "modelId": e2e_model()
            },
            "mergeTerminalConfig": {
                "cliTypeId": "cli-claude-code",
                "modelConfigId": model_config_id
            },
            "targetBranch": "main",
            "tasks": [{
                "name": "E2E Simple Task",
                "orderIndex": 0,
                "terminals": [{
                    "cliTypeId": "cli-claude-code",
                    "modelConfigId": model_config_id,
                    "role": "Coder",
                    "orderIndex": 0,
                    "autoConfirm": true
                }]
            }]
        }))
        .send()
        .await
        .map_err(|e| format!("Create workflow: {e}"))?;

    let status = resp.status().as_u16();
    let body: Value = resp.json().await.map_err(|e| format!("Parse: {e}"))?;
    if status >= 400 {
        return Err(format!("Create workflow {status}: {body}"));
    }

    let data = body.get("data").unwrap_or(&body);
    let workflow_id = data
        .get("id")
        .and_then(|v| v.as_str())
        .ok_or_else(|| format!("No workflow ID: {body}"))?
        .to_string();

    // Extract terminal ID for log collection
    let terminal_id = data
        .get("tasks")
        .and_then(|t| t.as_array())
        .and_then(|t| t.first())
        .and_then(|t| t.get("terminals"))
        .and_then(|t| t.as_array())
        .and_then(|t| t.first())
        .and_then(|t| t.get("id"))
        .and_then(|v| v.as_str())
        .unwrap_or("unknown")
        .to_string();

    eprintln!("  [workflow] Created: {workflow_id}");
    eprintln!("  [workflow] Terminal: {terminal_id}");

    // Step 2: Prepare workflow (spawns PTY)
    eprintln!("  [workflow] Preparing (spawning PTY)...");
    let resp = client
        .post(api(&format!("/workflows/{workflow_id}/prepare")))
        .send()
        .await
        .map_err(|e| format!("Prepare workflow: {e}"))?;

    let status = resp.status().as_u16();
    let body_text = resp.text().await.unwrap_or_default();
    if status >= 400 {
        // Collect terminal logs for diagnosis
        let logs = collect_terminal_logs(client, api, &terminal_id).await;
        return Err(format!(
            "Prepare workflow returned {status}: {}\n\nTerminal logs:\n{}",
            &body_text[..body_text.len().min(500)],
            logs
        ));
    }

    eprintln!("  [workflow] Prepared successfully");

    // Wait for terminals to be ready
    tokio::time::sleep(Duration::from_secs(3)).await;

    // Step 3: Start workflow (orchestrator begins)
    eprintln!("  [workflow] Starting orchestrator...");
    let resp = client
        .post(api(&format!("/workflows/{workflow_id}/start")))
        .send()
        .await
        .map_err(|e| format!("Start workflow: {e}"))?;

    let status = resp.status().as_u16();
    let body_text = resp.text().await.unwrap_or_default();
    if status >= 400 {
        let logs = collect_terminal_logs(client, api, &terminal_id).await;
        return Err(format!(
            "Start workflow returned {status}: {}\n\nTerminal logs:\n{}",
            &body_text[..body_text.len().min(500)],
            logs
        ));
    }

    eprintln!("  [workflow] Started — polling for completion...");

    // Step 4: Poll for workflow completion
    let poll_start = Instant::now();
    let max_wait = Duration::from_secs(300); // 5 minutes max
    let poll_interval = Duration::from_secs(5);

    loop {
        if poll_start.elapsed() > max_wait {
            let logs = collect_terminal_logs(client, api, &terminal_id).await;
            // Stop the workflow to clean up
            let _ = client
                .post(api(&format!("/workflows/{workflow_id}/stop")))
                .send()
                .await;
            return Err(format!(
                "Workflow did not complete within 5 minutes\n\nTerminal logs:\n{}",
                logs
            ));
        }

        tokio::time::sleep(poll_interval).await;

        let resp = client
            .get(api(&format!("/workflows/{workflow_id}")))
            .send()
            .await;

        let resp = match resp {
            Ok(r) => r,
            Err(e) => {
                eprintln!("    Poll error: {e}");
                continue;
            }
        };

        let body: Value = match resp.json().await {
            Ok(b) => b,
            Err(_) => continue,
        };

        let data = body.get("data").unwrap_or(&body);
        let status = data
            .get("status")
            .and_then(|v| v.as_str())
            .unwrap_or("unknown");

        let elapsed = poll_start.elapsed().as_secs();
        eprintln!("    [{elapsed}s] Workflow status: {status}");

        match status {
            "completed" => {
                eprintln!("  [workflow] COMPLETED successfully!");
                break;
            }
            "failed" => {
                let logs = collect_terminal_logs(client, api, &terminal_id).await;
                return Err(format!(
                    "Workflow failed\n\nTerminal logs:\n{}",
                    logs
                ));
            }
            _ => continue,
        }
    }

    // Step 5: Verify results
    eprintln!("  [workflow] Verifying results...");

    // Check that the terminal completed
    let logs = collect_terminal_logs(client, api, &terminal_id).await;
    eprintln!("  [workflow] Terminal logs ({} chars)", logs.len());

    // Check if any file was created in the repo
    let entries: Vec<_> = std::fs::read_dir(repo_path)
        .map(|rd| {
            rd.filter_map(|e| e.ok())
                .map(|e| e.file_name().to_string_lossy().to_string())
                .collect()
        })
        .unwrap_or_default();
    eprintln!("  [workflow] Repo files: {:?}", entries);

    // Check git log for new commits
    let git_log = std::process::Command::new("git")
        .args(["log", "--oneline", "-5"])
        .current_dir(repo_path)
        .output()
        .map(|o| String::from_utf8_lossy(&o.stdout).to_string())
        .unwrap_or_default();
    eprintln!("  [workflow] Git log:\n{git_log}");

    // Cleanup: stop and delete workflow
    let _ = client
        .post(api(&format!("/workflows/{workflow_id}/stop")))
        .send()
        .await;
    let _ = client
        .delete(api(&format!("/workflows/{workflow_id}")))
        .send()
        .await;

    Ok(())
}

/// Collect terminal logs for diagnosis on failure.
async fn collect_terminal_logs(
    client: &reqwest::Client,
    api: &dyn Fn(&str) -> String,
    terminal_id: &str,
) -> String {
    let resp = client
        .get(api(&format!("/terminals/{terminal_id}/logs")))
        .send()
        .await;

    let resp = match resp {
        Ok(r) => r,
        Err(e) => return format!("[Failed to fetch logs: {e}]"),
    };

    let body: Value = match resp.json().await {
        Ok(b) => b,
        Err(e) => return format!("[Failed to parse logs: {e}]"),
    };

    // Extract log entries
    let logs = body
        .get("data")
        .and_then(|d| d.as_array())
        .or_else(|| body.as_array());

    match logs {
        Some(entries) => {
            let mut output = String::new();
            for entry in entries.iter().take(100) {
                let log_type = entry
                    .get("logType")
                    .or_else(|| entry.get("log_type"))
                    .and_then(|v| v.as_str())
                    .unwrap_or("?");
                let content = entry
                    .get("content")
                    .and_then(|v| v.as_str())
                    .unwrap_or("");
                output.push_str(&format!("[{log_type}] {content}\n"));
            }
            if output.is_empty() {
                "[No log entries]".to_string()
            } else {
                output
            }
        }
        None => format!("[Unexpected log format: {}]", &body.to_string()[..body.to_string().len().min(500)]),
    }
}
