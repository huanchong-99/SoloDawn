//! Concierge tool definitions and execution logic.

use anyhow::{Context, Result, anyhow};
use serde::{Deserialize, Serialize};
use sqlx::SqlitePool;

use db::models::concierge::ConciergeSession;
use db::models::project::Project;
use db::models::project_repo::ProjectRepo;
use db::models::repo::Repo;
use db::models::terminal::Terminal;
use db::models::workflow::{Workflow, WorkflowTask};

// ---------------------------------------------------------------------------
// Tool call parsing
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolCall {
    pub tool: String,
    #[serde(default)]
    pub params: serde_json::Value,
}

/// Try to extract a tool call from an LLM response.
/// Returns None if no fenced JSON tool call block is found.
pub fn parse_tool_call(response: &str) -> Option<ToolCall> {
    // Try fenced JSON first, then inline JSON
    let json_str = extract_fenced_json(response).or_else(|| extract_inline_json(response))?;
    let parsed: ToolCall = serde_json::from_str(json_str).ok()?;
    if parsed.tool.is_empty() {
        return None;
    }
    Some(parsed)
}

fn extract_fenced_json(text: &str) -> Option<&str> {
    // Try ```json\n...\n``` first
    if let Some(start) = text.find("```json\n") {
        let content_start = start + 8;
        if let Some(end) = text[content_start..].find("\n```") {
            return Some(&text[content_start..content_start + end]);
        }
    }
    // Try ```\n...\n```
    if let Some(start) = text.find("```\n") {
        let content_start = start + 4;
        if let Some(end) = text[content_start..].find("\n```") {
            let candidate = text[content_start..content_start + end].trim();
            if candidate.starts_with('{') {
                return Some(candidate);
            }
        }
    }
    None
}

/// Extract inline JSON from text like: `some text {"tool":"x","params":{}} more text`
///
/// Handles braces inside JSON string values correctly by tracking whether
/// the parser is inside a quoted string literal (with backslash-escape awareness).
fn extract_inline_json(text: &str) -> Option<&str> {
    // Fast path: try parsing the entire text as a ToolCall directly
    if text.trim().starts_with('{')
        && serde_json::from_str::<serde_json::Value>(text.trim()).is_ok()
    {
        let trimmed = text.trim();
        // Verify it has a "tool" field before returning
        if trimmed.contains(r#""tool""#) {
            // Return slice of original text
            let offset = text.find(trimmed)?;
            return Some(&text[offset..offset + trimmed.len()]);
        }
    }

    // Find the first `{"tool"` pattern
    let needle = r#"{"tool""#;
    let start = text.find(needle)?;
    let candidate = &text[start..];

    // Find matching closing brace by counting braces, respecting string literals.
    // Inside a JSON string (after an unescaped `"`), braces are not structural.
    let mut depth = 0i32;
    let mut in_string = false;
    let mut escape_next = false;
    let mut end = 0;

    for (i, ch) in candidate.char_indices() {
        if escape_next {
            escape_next = false;
            continue;
        }
        if in_string {
            match ch {
                '\\' => escape_next = true,
                '"' => in_string = false,
                _ => {}
            }
            continue;
        }
        match ch {
            '"' => in_string = true,
            '{' => depth += 1,
            '}' => {
                depth -= 1;
                if depth == 0 {
                    end = i + 1;
                    break;
                }
            }
            _ => {}
        }
    }

    if end > 0 {
        Some(&candidate[..end])
    } else {
        None
    }
}

// ---------------------------------------------------------------------------
// Tool execution
// ---------------------------------------------------------------------------

/// Execute a tool call and return the result as a human-readable string.
pub async fn execute_tool(
    pool: &SqlitePool,
    session: &ConciergeSession,
    tool_call: &ToolCall,
    shared_config: Option<&std::sync::Arc<tokio::sync::RwLock<crate::services::config::Config>>>,
) -> Result<String> {
    match tool_call.tool.as_str() {
        "create_project" => execute_create_project(pool, session, &tool_call.params).await,
        "list_projects" => execute_list_projects(pool).await,
        "select_project" => execute_select_project(pool, session, &tool_call.params).await,
        "list_cli_types" => execute_list_cli_types(pool, shared_config).await,
        "create_workflow" => {
            execute_create_workflow(pool, session, &tool_call.params, shared_config).await
        }
        "list_workflows" => execute_list_workflows(pool, session, &tool_call.params).await,
        "get_workflow_status" => execute_get_workflow_status(pool, &tool_call.params).await,
        "select_workflow" => execute_select_workflow(pool, session, &tool_call.params).await,
        "list_tasks" => execute_list_tasks(pool, session, &tool_call.params).await,
        "get_task_detail" => execute_get_task_detail(pool, &tool_call.params).await,
        "toggle_progress_notifications" => {
            execute_toggle_progress(pool, session, &tool_call.params).await
        }
        "toggle_feishu_sync" => {
            execute_toggle_feishu_sync(pool, session, &tool_call.params).await
        }
        "show_overview" => execute_show_overview(pool).await,
        // Runtime tools need orchestrator access — marked for agent-level handling
        "send_to_orchestrator" | "prepare_workflow" | "start_workflow" => Ok(format!(
            "RUNTIME_TOOL:{}:{}",
            tool_call.tool,
            serde_json::to_string(&tool_call.params).unwrap_or_default()
        )),
        other => Err(anyhow!("Unknown tool: {other}")),
    }
}

// ---------------------------------------------------------------------------
// Individual tool implementations
// ---------------------------------------------------------------------------

async fn execute_create_project(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let name = params["name"]
        .as_str()
        .context("Missing 'name' parameter")?;
    let repo_path = params["repo_path"]
        .as_str()
        .context("Missing 'repo_path' parameter")?;

    // Validate repo_path: reject path traversal components and dangerous patterns
    let repo_path_buf = std::path::PathBuf::from(repo_path);
    for component in repo_path_buf.components() {
        match component {
            std::path::Component::ParentDir => {
                return Ok(
                    "Rejected: repo_path must not contain '..' path traversal components."
                        .to_string(),
                );
            }
            std::path::Component::Normal(seg) => {
                let s = seg.to_string_lossy();
                if s.starts_with('.') && s != ".git" {
                    return Ok(format!(
                        "Rejected: repo_path contains suspicious hidden segment '{s}'."
                    ));
                }
            }
            _ => {}
        }
    }
    if !repo_path_buf.is_absolute() {
        return Ok("Rejected: repo_path must be an absolute path.".to_string());
    }

    // Check for duplicate name
    let existing = Project::find_all(pool).await?;
    for p in &existing {
        if p.name == name {
            return Ok(format!("Project '{name}' already exists (id: {})", p.id));
        }
    }

    // Create directory and init git
    let path = std::path::Path::new(repo_path);
    if !path.exists() {
        std::fs::create_dir_all(path)
            .with_context(|| format!("Failed to create directory: {repo_path}"))?;
    }
    if !path.join(".git").exists() {
        std::process::Command::new("git")
            .args(["init"])
            .current_dir(path)
            .output()
            .with_context(|| format!("Failed to git init at {repo_path}"))?;
    }

    // Create project in DB
    let project_id = uuid::Uuid::new_v4();
    let create_data = db::models::project::CreateProject {
        name: name.to_string(),
        repositories: vec![],
    };
    let project = match Project::create(pool, &create_data, project_id).await {
        Ok(p) => p,
        Err(e) => {
            return Ok(format!(
                "Failed to create project '{name}': {e}. The name might already exist."
            ));
        }
    };
    let pid_str = project.id.to_string();

    // Create repo and link
    let repo = match Repo::find_or_create(pool, path, repo_path).await {
        Ok(r) => r,
        Err(e) => {
            return Ok(format!(
                "Project '{name}' created but failed to link repository: {e}"
            ));
        }
    };
    if let Err(e) = ProjectRepo::create(pool, project.id, repo.id).await {
        return Ok(format!(
            "Project '{name}' created but failed to bind repo: {e}"
        ));
    }

    // Update default_agent_working_dir
    let _ = Project::update(
        pool,
        project.id,
        &db::models::project::UpdateProject {
            name: None,
            default_agent_working_dir: Some(repo_path.to_string()),
        },
    )
    .await;

    // Focus session on new project
    ConciergeSession::update_active_project(pool, &session.id, Some(&pid_str)).await?;

    Ok(format!(
        "Project '{name}' created successfully.\n- ID: {pid_str}\n- Path: {repo_path}\n- Git repository initialized"
    ))
}

async fn execute_list_projects(pool: &SqlitePool) -> Result<String> {
    let projects = Project::find_all(pool).await?;
    if projects.is_empty() {
        return Ok("No projects found. You can create one with create_project.".to_string());
    }
    let mut result = format!("Found {} project(s):\n", projects.len());
    for p in &projects {
        let dir = p
            .default_agent_working_dir
            .as_deref()
            .unwrap_or("(no path)");
        result.push_str(&format!("- {} (id: {}, path: {})\n", p.name, p.id, dir));
    }
    Ok(result)
}

async fn execute_select_project(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let project_id_str = params["project_id"]
        .as_str()
        .context("Missing 'project_id' parameter")?;
    let project_id: uuid::Uuid = project_id_str
        .parse()
        .context("Invalid project_id UUID")?;
    let project = Project::find_by_id(pool, project_id)
        .await?
        .context("Project not found")?;
    ConciergeSession::update_active_project(pool, &session.id, Some(project_id_str)).await?;
    Ok(format!(
        "Switched to project '{}' ({})",
        project.name, project.id
    ))
}

async fn execute_list_cli_types(
    pool: &SqlitePool,
    shared_config: Option<&std::sync::Arc<tokio::sync::RwLock<crate::services::config::Config>>>,
) -> Result<String> {
    use db::models::cli_type::CliType;
    use std::collections::HashMap;

    let cli_types = CliType::find_all(pool).await?;

    // Read configured models from config.json (the source of truth for API keys)
    let config_models = match shared_config {
        Some(cfg) => {
            let cfg = cfg.read().await;
            cfg.workflow_model_library
                .iter()
                .filter(|m| !m.api_key.is_empty())
                .cloned()
                .collect::<Vec<_>>()
        }
        None => vec![],
    };

    if config_models.is_empty() {
        return Ok(
            "No models with API keys configured. Please configure at least one model in Settings → Models."
                .to_string(),
        );
    }

    // Group config models by CLI type
    let mut by_cli: HashMap<String, Vec<&crate::services::config::WorkflowModelLibraryItem>> =
        HashMap::new();
    for m in &config_models {
        let cli_id = m.cli_type_id.clone().unwrap_or_else(|| "unknown".to_string());
        by_cli.entry(cli_id).or_default().push(m);
    }

    let mut result = String::from("Available AI CLI tools and models (only those with API keys):\n\n");
    let mut index = 1;
    for ct in &cli_types {
        if let Some(models) = by_cli.get(&ct.id) {
            result.push_str(&format!("**{}** (cli_type_id: `{}`)\n", ct.display_name, ct.id));
            for m in models {
                let verified = if m.is_verified { " ✅" } else { "" };
                result.push_str(&format!(
                    "  {}. {} (model_config_id: `{}`, model: {}){}\n",
                    index, m.display_name, m.id, m.model_id, verified
                ));
                index += 1;
            }
            result.push('\n');
        }
    }

    // Also show models whose cli_type_id doesn't match any known CLI
    for (cli_id, models) in &by_cli {
        if cli_types.iter().any(|ct| &ct.id == cli_id) {
            continue;
        }
        result.push_str(&format!("**{}**\n", cli_id));
        for m in models {
            let verified = if m.is_verified { " ✅" } else { "" };
            result.push_str(&format!(
                "  {}. {} (model_config_id: `{}`, model: {}){}\n",
                index, m.display_name, m.id, m.model_id, verified
            ));
            index += 1;
        }
        result.push('\n');
    }

    Ok(result)
}

async fn execute_create_workflow(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
    shared_config: Option<&std::sync::Arc<tokio::sync::RwLock<crate::services::config::Config>>>,
) -> Result<String> {
    let project_id_str = session
        .active_project_id
        .as_deref()
        .context("No active project. Use select_project or create_project first.")?;
    let project_id: uuid::Uuid = project_id_str
        .parse()
        .context("Invalid active project_id")?;
    let name = params["name"]
        .as_str()
        .context("Missing 'name' parameter")?;
    let description = params["description"].as_str().unwrap_or("");
    let initial_goal = params["initial_goal"].as_str().unwrap_or(description);

    let cli_type_id = params["cli_type_id"]
        .as_str()
        .context("Missing 'cli_type_id'. Use list_cli_types to see available options, then ask the user to choose.")?;
    let model_config_id = params["model_config_id"]
        .as_str()
        .context("Missing 'model_config_id'. Use list_cli_types to see available options, then ask the user to choose.")?;

    // Ensure the model_config_id exists in the database (it may only be in config.json).
    // The workflow table has FK constraints on merge_terminal_model_id → model_config(id).
    {
        use db::models::cli_type::ModelConfig;
        let existing = ModelConfig::find_by_id(pool, model_config_id).await?;
        if existing.is_none() {
            // Model not in DB — sync from config.json
            if let Some(cfg) = shared_config {
                let cfg = cfg.read().await;
                if let Some(item) = cfg.workflow_model_library.iter().find(|m| m.id == model_config_id) {
                    // Create the model_config record
                    ModelConfig::create_custom(
                        pool,
                        model_config_id,
                        cli_type_id,
                        &item.display_name,
                        &item.model_id,
                    )
                    .await?;
                    // Set credentials (base_url, api_type, encrypted_api_key)
                    if !item.api_key.is_empty() {
                        if let Ok(encrypted) = ConciergeSession::encrypt_api_key(&item.api_key) {
                            let _ = ModelConfig::update_credentials(
                                pool,
                                model_config_id,
                                &encrypted,
                                Some(&item.base_url),
                                &item.api_type,
                            )
                            .await;
                        }
                    }
                    tracing::info!(
                        model_config_id = model_config_id,
                        "Synced model config from config.json to database for workflow FK"
                    );
                }
            }
        }
    }

    // Build a minimal agent-planned Workflow struct
    let now = chrono::Utc::now();
    let workflow = Workflow {
        id: uuid::Uuid::new_v4().to_string(),
        project_id,
        name: name.to_string(),
        description: Some(description.to_string()),
        status: "created".to_string(),
        execution_mode: "agent_planned".to_string(),
        initial_goal: Some(initial_goal.to_string()),
        use_slash_commands: false,
        orchestrator_enabled: true,
        orchestrator_api_type: session.llm_api_type.clone(),
        orchestrator_base_url: session.llm_base_url.clone(),
        orchestrator_api_key: session.llm_api_key_encrypted.clone(),
        orchestrator_model: session.llm_model_id.clone(),
        error_terminal_enabled: false,
        error_terminal_cli_id: None,
        error_terminal_model_id: None,
        merge_terminal_cli_id: cli_type_id.to_string(),
        merge_terminal_model_id: model_config_id.to_string(),
        target_branch: "main".to_string(),
        git_watcher_enabled: true,
        ready_at: None,
        started_at: None,
        completed_at: None,
        pause_reason: None,
        created_at: now,
        updated_at: now,
    };

    let created = match Workflow::create(pool, &workflow).await {
        Ok(w) => w,
        Err(e) => {
            return Ok(format!("Failed to create workflow '{name}': {e}"));
        }
    };
    let wid = created.id.clone();

    // Create a companion Task + Workspace so the sidebar "活跃" list picks it up.
    // The workspace system (old) requires a task_id → workspace_id link.
    {
        use db::models::task::{CreateTask, Task};
        use db::models::workspace::{CreateWorkspace, Workspace};

        let task_id = uuid::Uuid::new_v4();
        let ws_id = uuid::Uuid::new_v4();

        // Create a task record linked to the project
        if let Ok(_task) = Task::create(
            pool,
            &CreateTask {
                project_id,
                title: name.to_string(),
                description: Some(initial_goal.to_string()),
                status: None,
                parent_workspace_id: None,
                image_ids: None,
                shared_task_id: None,
            },
            task_id,
        )
        .await
        {
            // Create a workspace record linked to the task
            if let Ok(ws) = Workspace::create(
                pool,
                &CreateWorkspace {
                    branch: "main".to_string(),
                    agent_working_dir: None,
                },
                ws_id,
                task_id,
            )
            .await
            {
                // Set workspace name
                let _ = sqlx::query(
                    "UPDATE workspaces SET name = ?1, updated_at = datetime('now') WHERE id = ?2",
                )
                .bind(name)
                .bind(ws.id)
                .execute(pool)
                .await;
            }
        }
    }

    // Focus session
    ConciergeSession::update_active_workflow(pool, &session.id, Some(&wid)).await?;

    Ok(format!(
        "Workflow '{name}' created.\n- ID: {wid}\n- Status: created\n- Mode: agent_planned\nReady to prepare and start."
    ))
}

async fn execute_list_workflows(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let project_id_str = params["project_id"]
        .as_str()
        .or(session.active_project_id.as_deref());

    let workflows: Vec<Workflow> = match project_id_str {
        Some(pid) => {
            let uuid: uuid::Uuid = pid.parse().context("Invalid project_id UUID")?;
            Workflow::find_by_project(pool, uuid).await?
        }
        None => {
            sqlx::query_as::<_, Workflow>(
                "SELECT * FROM workflow ORDER BY created_at DESC LIMIT 50",
            )
            .fetch_all(pool)
            .await?
        }
    };

    if workflows.is_empty() {
        return Ok("No workflows found.".to_string());
    }

    let mut result = format!("Found {} workflow(s):\n", workflows.len());
    for w in &workflows {
        result.push_str(&format!(
            "- {} (id: {}, status: {})\n",
            w.name, w.id, w.status
        ));
    }
    Ok(result)
}

async fn execute_get_workflow_status(
    pool: &SqlitePool,
    params: &serde_json::Value,
) -> Result<String> {
    let workflow_id = params["workflow_id"]
        .as_str()
        .context("Missing 'workflow_id' parameter")?;
    let workflow = Workflow::find_by_id(pool, workflow_id)
        .await?
        .context("Workflow not found")?;

    let tasks = WorkflowTask::find_by_workflow(pool, workflow_id).await?;

    let mut result = format!(
        "Workflow: {}\n- Status: {}\n- Mode: {}\n",
        workflow.name, workflow.status, workflow.execution_mode
    );
    if !tasks.is_empty() {
        result.push_str(&format!("\nTasks ({}):\n", tasks.len()));
        for t in &tasks {
            result.push_str(&format!(
                "  - {} (id: {}, status: {})\n",
                t.name, t.id, t.status
            ));
        }
    }
    Ok(result)
}

async fn execute_select_workflow(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let workflow_id = params["workflow_id"]
        .as_str()
        .context("Missing 'workflow_id' parameter")?;
    let workflow = Workflow::find_by_id(pool, workflow_id)
        .await?
        .context("Workflow not found")?;
    ConciergeSession::update_active_workflow(pool, &session.id, Some(workflow_id)).await?;
    Ok(format!(
        "Switched to workflow '{}' (status: {})",
        workflow.name, workflow.status
    ))
}

async fn execute_list_tasks(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let workflow_id = params["workflow_id"]
        .as_str()
        .or(session.active_workflow_id.as_deref())
        .context("No active workflow. Use select_workflow first.")?;

    let tasks = WorkflowTask::find_by_workflow(pool, workflow_id).await?;
    if tasks.is_empty() {
        return Ok("No tasks in this workflow yet.".to_string());
    }

    let mut result = format!("Tasks in workflow ({}):\n", tasks.len());
    for (i, t) in tasks.iter().enumerate() {
        result.push_str(&format!(
            "  {}. {} (id: {}, status: {})\n",
            i + 1,
            t.name,
            t.id,
            t.status
        ));
    }
    Ok(result)
}

async fn execute_get_task_detail(
    pool: &SqlitePool,
    params: &serde_json::Value,
) -> Result<String> {
    let task_id = params["task_id"]
        .as_str()
        .context("Missing 'task_id' parameter")?;
    let task = WorkflowTask::find_by_id(pool, task_id)
        .await?
        .context("Task not found")?;

    let terminals: Vec<Terminal> = Terminal::find_by_task(pool, task_id).await?;

    let mut result = format!(
        "Task: {}\n- ID: {}\n- Status: {}\n- Branch: {}\n",
        task.name, task.id, task.status, task.branch
    );
    if !terminals.is_empty() {
        result.push_str(&format!("\nTerminals ({}):\n", terminals.len()));
        for t in &terminals {
            result.push_str(&format!(
                "  - terminal (id: {}, status: {}, cli: {})\n",
                t.id, t.status, t.cli_type_id
            ));
        }
    }
    Ok(result)
}

async fn execute_toggle_progress(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let enabled = params["enabled"].as_bool().unwrap_or(false);
    ConciergeSession::update_progress_notifications(pool, &session.id, enabled).await?;
    Ok(if enabled {
        "Real-time progress notifications enabled.".to_string()
    } else {
        "Progress notifications disabled. Only completion summaries.".to_string()
    })
}

async fn execute_toggle_feishu_sync(
    pool: &SqlitePool,
    session: &ConciergeSession,
    params: &serde_json::Value,
) -> Result<String> {
    let enabled = params["enabled"].as_bool().unwrap_or(false);
    ConciergeSession::update_feishu_sync(pool, &session.id, enabled).await?;
    Ok(if enabled {
        "Feishu sync enabled.".to_string()
    } else {
        "Feishu sync disabled.".to_string()
    })
}

async fn execute_show_overview(pool: &SqlitePool) -> Result<String> {
    let projects = Project::find_all(pool).await?;
    let workflows: Vec<Workflow> =
        sqlx::query_as::<_, Workflow>("SELECT * FROM workflow ORDER BY created_at DESC LIMIT 100")
            .fetch_all(pool)
            .await?;

    let mut result = format!(
        "=== SoloDawn Overview ===\nProjects: {}\nWorkflows: {}\n",
        projects.len(),
        workflows.len()
    );

    if !projects.is_empty() {
        result.push_str("\nProjects:\n");
        for p in &projects {
            let pid = p.id.to_string();
            let wf_count = workflows
                .iter()
                .filter(|w| w.project_id.to_string() == pid)
                .count();
            result.push_str(&format!("  - {} ({} workflow(s))\n", p.name, wf_count));
        }
    }

    let running: Vec<_> = workflows.iter().filter(|w| w.status == "running").collect();
    if !running.is_empty() {
        result.push_str(&format!("\nRunning workflows ({}):\n", running.len()));
        for w in &running {
            result.push_str(&format!("  - {} ({})\n", w.name, w.id));
        }
    }

    Ok(result)
}
