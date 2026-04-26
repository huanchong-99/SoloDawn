//! Workflow API DTOs with explicit camelCase serialization
//!
//! This module defines the API contract for Workflow responses.
//! All structs use explicit field mappings (no flatten) to prevent conflicts.

use serde::Serialize;
use ts_rs::TS;

/// Workflow detail response DTO
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowDetailDto {
    // Basic workflow fields
    pub id: String,
    pub project_id: String,
    pub name: String,
    pub description: Option<String>,
    pub status: String,
    pub execution_mode: String,
    pub initial_goal: Option<String>,
    pub use_slash_commands: bool,
    pub orchestrator_enabled: bool,
    pub orchestrator_api_type: Option<String>,
    pub orchestrator_base_url: Option<String>,
    pub orchestrator_model: Option<String>,
    pub error_terminal_enabled: bool,
    pub error_terminal_cli_id: Option<String>,
    pub error_terminal_model_id: Option<String>,
    /// Wrapped in Option for backward compatibility with older API clients that
    /// may not send these fields. The underlying DB column is NOT NULL with a default.
    // TODO(G01-005/G17-006): The DB column `merge_terminal_cli_id` is NOT NULL, so this
    // should be `String` not `Option<String>`. Kept as Option for backward compat with
    // older frontend clients that may omit the field. Migrate once all clients are updated.
    pub merge_terminal_cli_id: Option<String>,
    /// See `merge_terminal_cli_id` — same backward-compat rationale.
    pub merge_terminal_model_id: Option<String>,
    pub target_branch: String,
    pub git_watcher_enabled: bool,

    // Timestamps
    pub ready_at: Option<String>,
    pub started_at: Option<String>,
    pub completed_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,

    // Nested data
    pub tasks: Vec<WorkflowTaskDto>,
    pub commands: Vec<WorkflowCommandDto>,
}

/// Workflow task DTO
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowTaskDto {
    pub id: String,
    pub workflow_id: String,
    pub vk_task_id: Option<String>,
    pub name: String,
    pub description: Option<String>,
    pub branch: String,
    pub status: String,
    pub order_index: i32,
    pub started_at: Option<String>,
    pub completed_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,
    pub terminals: Vec<TerminalDto>,
}

/// Terminal DTO
// TODO(G17-003): Migrate `status` from String to a typed enum once frontend consumers are updated.
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct TerminalDto {
    pub id: String,
    pub workflow_task_id: String,
    pub cli_type_id: String,
    pub model_config_id: String,
    pub custom_base_url: Option<String>,
    #[ts(optional)]
    #[serde(skip_serializing)]
    pub custom_api_key: Option<String>,
    pub role: Option<String>,
    pub role_description: Option<String>,
    pub order_index: i32,
    pub status: String,
    // G17-005: Key fields added to DTO for frontend visibility
    pub auto_confirm: bool,
    pub last_commit_hash: Option<String>,
    pub last_commit_message: Option<String>,
    pub started_at: Option<String>,
    pub completed_at: Option<String>,
    pub created_at: String,
    pub updated_at: String,
}

/// Workflow command DTO
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowCommandDto {
    pub id: String,
    pub workflow_id: String,
    pub preset_id: String,
    pub order_index: i32,
    pub custom_params: Option<String>,
    pub created_at: String,
    pub preset: SlashCommandPresetDto,
}

/// Slash command preset DTO
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct SlashCommandPresetDto {
    pub id: String,
    pub command: String,
    pub description: String,
    pub prompt_template: String,
    pub is_system: bool,
    pub created_at: String,
    pub updated_at: String,
}

/// Workflow list item DTO (simplified for list view)
#[derive(Debug, Clone, Serialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowListItemDto {
    pub id: String,
    pub project_id: String,
    pub name: String,
    pub description: Option<String>,
    pub status: String,
    pub execution_mode: String,
    pub created_at: String,
    pub updated_at: String,
    pub tasks_count: i32,
    pub terminals_count: i32,
}

impl WorkflowDetailDto {
    /// Convert from DB models to DTO
    pub fn from_workflow(
        workflow: &db::models::Workflow,
        tasks: &[db::models::WorkflowTask],
        commands: &[(db::models::WorkflowCommand, db::models::SlashCommandPreset)],
    ) -> Self {
        let tasks_dto = tasks
            .iter()
            .map(|task| {
                WorkflowTaskDto::from_workflow_task(task, &[]) // terminals will be loaded separately
            })
            .collect();

        let commands_dto = commands
            .iter()
            .map(|(cmd, preset)| WorkflowCommandDto::from_models(cmd, preset))
            .collect();

        Self {
            id: workflow.id.clone(),
            project_id: workflow.project_id.to_string(),
            name: workflow.name.clone(),
            description: workflow.description.clone(),
            status: workflow.status.clone(),
            execution_mode: workflow.execution_mode.clone(),
            initial_goal: workflow.initial_goal.clone(),
            use_slash_commands: workflow.use_slash_commands,
            orchestrator_enabled: workflow.orchestrator_enabled,
            orchestrator_api_type: workflow.orchestrator_api_type.clone(),
            orchestrator_base_url: workflow.orchestrator_base_url.clone(),
            orchestrator_model: workflow.orchestrator_model.clone(),
            error_terminal_enabled: workflow.error_terminal_enabled,
            error_terminal_cli_id: workflow.error_terminal_cli_id.clone(),
            error_terminal_model_id: workflow.error_terminal_model_id.clone(),
            merge_terminal_cli_id: Some(workflow.merge_terminal_cli_id.clone()),
            merge_terminal_model_id: Some(workflow.merge_terminal_model_id.clone()),
            target_branch: workflow.target_branch.clone(),
            git_watcher_enabled: workflow.git_watcher_enabled,
            ready_at: workflow.ready_at.map(|dt| dt.to_rfc3339()),
            started_at: workflow.started_at.map(|dt| dt.to_rfc3339()),
            completed_at: workflow.completed_at.map(|dt| dt.to_rfc3339()),
            created_at: workflow.created_at.to_rfc3339(),
            updated_at: workflow.updated_at.to_rfc3339(),
            tasks: tasks_dto,
            commands: commands_dto,
        }
    }

    /// Convert from DB models to DTO with terminals
    pub fn from_workflow_with_terminals(
        workflow: &db::models::Workflow,
        tasks_with_terminals: &[(db::models::WorkflowTask, Vec<db::models::Terminal>)],
        commands: &[(db::models::WorkflowCommand, db::models::SlashCommandPreset)],
    ) -> Self {
        let tasks_dto: Vec<WorkflowTaskDto> = tasks_with_terminals
            .iter()
            .map(|(task, terminals)| WorkflowTaskDto::from_workflow_task(task, terminals))
            .collect();

        let commands_dto = commands
            .iter()
            .map(|(cmd, preset)| WorkflowCommandDto::from_models(cmd, preset))
            .collect();

        Self {
            id: workflow.id.clone(),
            project_id: workflow.project_id.to_string(),
            name: workflow.name.clone(),
            description: workflow.description.clone(),
            status: workflow.status.clone(),
            execution_mode: workflow.execution_mode.clone(),
            initial_goal: workflow.initial_goal.clone(),
            use_slash_commands: workflow.use_slash_commands,
            orchestrator_enabled: workflow.orchestrator_enabled,
            orchestrator_api_type: workflow.orchestrator_api_type.clone(),
            orchestrator_base_url: workflow.orchestrator_base_url.clone(),
            orchestrator_model: workflow.orchestrator_model.clone(),
            error_terminal_enabled: workflow.error_terminal_enabled,
            error_terminal_cli_id: workflow.error_terminal_cli_id.clone(),
            error_terminal_model_id: workflow.error_terminal_model_id.clone(),
            merge_terminal_cli_id: Some(workflow.merge_terminal_cli_id.clone()),
            merge_terminal_model_id: Some(workflow.merge_terminal_model_id.clone()),
            target_branch: workflow.target_branch.clone(),
            git_watcher_enabled: workflow.git_watcher_enabled,
            ready_at: workflow.ready_at.map(|dt| dt.to_rfc3339()),
            started_at: workflow.started_at.map(|dt| dt.to_rfc3339()),
            completed_at: workflow.completed_at.map(|dt| dt.to_rfc3339()),
            created_at: workflow.created_at.to_rfc3339(),
            updated_at: workflow.updated_at.to_rfc3339(),
            tasks: tasks_dto,
            commands: commands_dto,
        }
    }
}

impl WorkflowTaskDto {
    pub fn from_workflow_task(
        task: &db::models::WorkflowTask,
        terminals: &[db::models::Terminal],
    ) -> Self {
        let terminals_dto = terminals.iter().map(TerminalDto::from_terminal).collect();

        Self {
            id: task.id.clone(),
            workflow_id: task.workflow_id.clone(),
            vk_task_id: task.vk_task_id.map(|uuid| uuid.to_string()),
            name: task.name.clone(),
            description: task.description.clone(),
            branch: task.branch.clone(),
            status: task.status.clone(),
            order_index: task.order_index,
            started_at: task.started_at.map(|dt| dt.to_rfc3339()),
            completed_at: task.completed_at.map(|dt| dt.to_rfc3339()),
            created_at: task.created_at.to_rfc3339(),
            updated_at: task.updated_at.to_rfc3339(),
            terminals: terminals_dto,
        }
    }
}

impl TerminalDto {
    pub fn from_terminal(terminal: &db::models::Terminal) -> Self {
        Self {
            id: terminal.id.clone(),
            workflow_task_id: terminal.workflow_task_id.clone(),
            cli_type_id: terminal.cli_type_id.clone(),
            model_config_id: terminal.model_config_id.clone(),
            custom_base_url: terminal.custom_base_url.clone(),
            custom_api_key: None, // Never expose API keys in DTOs
            role: terminal.role.clone(),
            role_description: terminal.role_description.clone(),
            order_index: terminal.order_index,
            status: terminal.status.clone(),
            // G17-005: Key fields for frontend visibility
            auto_confirm: terminal.auto_confirm,
            last_commit_hash: terminal.last_commit_hash.clone(),
            last_commit_message: terminal.last_commit_message.clone(),
            started_at: terminal.started_at.map(|dt| dt.to_rfc3339()),
            completed_at: terminal.completed_at.map(|dt| dt.to_rfc3339()),
            created_at: terminal.created_at.to_rfc3339(),
            updated_at: terminal.updated_at.to_rfc3339(),
        }
    }
}

impl WorkflowCommandDto {
    pub fn from_models(
        command: &db::models::WorkflowCommand,
        preset: &db::models::SlashCommandPreset,
    ) -> Self {
        Self {
            id: command.id.clone(),
            workflow_id: command.workflow_id.clone(),
            preset_id: command.preset_id.clone(),
            order_index: command.order_index,
            custom_params: command.custom_params.clone(),
            created_at: command.created_at.to_rfc3339(),
            preset: SlashCommandPresetDto::from_model(preset),
        }
    }
}

impl SlashCommandPresetDto {
    pub fn from_model(preset: &db::models::SlashCommandPreset) -> Self {
        Self {
            id: preset.id.clone(),
            command: preset.command.clone(),
            description: preset.description.clone(),
            prompt_template: preset.prompt_template.clone().unwrap_or_default(),
            is_system: preset.is_system,
            created_at: preset.created_at.to_rfc3339(),
            updated_at: preset.updated_at.to_rfc3339(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_workflow_detail_dto_serialization() {
        let dto = WorkflowDetailDto {
            id: "wf-test".to_string(),
            project_id: "proj-test".to_string(),
            name: "Test Workflow".to_string(),
            description: Some("Test description".to_string()),
            status: "created".to_string(),
            execution_mode: "diy".to_string(),
            initial_goal: None,
            use_slash_commands: true,
            orchestrator_enabled: true,
            orchestrator_api_type: Some("openai-compatible".to_string()),
            orchestrator_base_url: Some("https://api.test.com".to_string()),
            orchestrator_model: Some("gpt-4o".to_string()),
            error_terminal_enabled: true,
            error_terminal_cli_id: Some("cli-test".to_string()),
            error_terminal_model_id: Some("model-test".to_string()),
            merge_terminal_cli_id: Some("cli-merge".to_string()),
            merge_terminal_model_id: Some("model-merge".to_string()),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: None,
            started_at: None,
            completed_at: None,
            created_at: "2026-01-24T10:00:00Z".to_string(),
            updated_at: "2026-01-24T10:00:00Z".to_string(),
            tasks: vec![],
            commands: vec![],
        };

        let json = serde_json::to_string(&dto).unwrap();

        // Verify camelCase serialization
        assert!(json.contains("\"projectId\""));
        assert!(json.contains("\"executionMode\""));
        assert!(json.contains("\"useSlashCommands\""));
        assert!(json.contains("\"orchestratorEnabled\""));
        assert!(json.contains("\"createdAt\""));
        assert!(json.contains("\"updatedAt\""));

        // Verify no snake_case
        assert!(!json.contains("\"project_id\""));
        assert!(!json.contains("\"execution_mode\""));
        assert!(!json.contains("\"use_slash_commands\""));
        assert!(!json.contains("\"created_at\""));
    }

    #[test]
    fn test_status_enum_valid_values() {
        let valid_statuses = vec![
            "created",
            "starting",
            "ready",
            "running",
            "paused",
            "merging",
            "completed",
            "failed",
            "cancelled",
        ];

        for status in valid_statuses {
            let dto = WorkflowDetailDto {
                id: "wf-test".to_string(),
                project_id: "proj-test".to_string(),
                name: "Test".to_string(),
                description: None,
                status: status.to_string(),
                execution_mode: "diy".to_string(),
                initial_goal: None,
                use_slash_commands: false,
                orchestrator_enabled: false,
                orchestrator_api_type: None,
                orchestrator_base_url: None,
                orchestrator_model: None,
                error_terminal_enabled: false,
                error_terminal_cli_id: None,
                error_terminal_model_id: None,
                merge_terminal_cli_id: None,
                merge_terminal_model_id: None,
                target_branch: "main".to_string(),
                git_watcher_enabled: true,
                ready_at: None,
                started_at: None,
                completed_at: None,
                created_at: "2026-01-24T10:00:00Z".to_string(),
                updated_at: "2026-01-24T10:00:00Z".to_string(),
                tasks: vec![],
                commands: vec![],
            };

            let json = serde_json::to_string(&dto).unwrap();
            assert!(json.contains(&format!("\"status\":\"{}\"", status)));
        }
    }

    #[test]
    fn test_task_status_enum_valid_values() {
        let valid_statuses = vec![
            "pending",
            "running",
            "review_pending",
            "completed",
            "failed",
            "cancelled",
        ];

        for status in valid_statuses {
            let dto = WorkflowTaskDto {
                id: "task-test".to_string(),
                workflow_id: "wf-test".to_string(),
                vk_task_id: None,
                name: "Test Task".to_string(),
                description: None,
                branch: "workflow/test".to_string(),
                status: status.to_string(),
                order_index: 0,
                started_at: None,
                completed_at: None,
                created_at: "2026-01-24T10:00:00Z".to_string(),
                updated_at: "2026-01-24T10:00:00Z".to_string(),
                terminals: vec![],
            };

            let json = serde_json::to_string(&dto).unwrap();
            assert!(json.contains(&format!("\"status\":\"{}\"", status)));
        }
    }
}

#[cfg(test)]
mod conversion_tests {
    use chrono::Utc;
    use db::models::Workflow;
    use uuid::Uuid;

    use super::*;

    #[test]
    fn test_convert_workflow_to_dto() {
        // This test will fail until we implement the conversion
        let workflow = Workflow {
            id: Uuid::new_v4().to_string(),
            project_id: Uuid::new_v4(),
            name: "Test Workflow".to_string(),
            description: Some("Test".to_string()),
            status: "created".to_string(),
            execution_mode: "diy".to_string(),
            initial_goal: Some("Ship dual-mode orchestration".to_string()),
            use_slash_commands: true,
            orchestrator_enabled: true,
            orchestrator_api_type: Some("openai-compatible".to_string()),
            orchestrator_base_url: None,
            orchestrator_model: Some("gpt-4o".to_string()),
            orchestrator_api_key: None,
            error_terminal_enabled: true,
            error_terminal_cli_id: Some("cli-test".to_string()),
            error_terminal_model_id: Some("model-test".to_string()),
            merge_terminal_cli_id: "cli-merge".to_string(),
            merge_terminal_model_id: "model-merge".to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: None,
            started_at: None,
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
            pause_reason: None,
        };

        let dto = WorkflowDetailDto::from_workflow(&workflow, &[], &[]);

        assert_eq!(dto.name, "Test Workflow");
        assert_eq!(dto.status, "created");
        assert_eq!(dto.execution_mode, "diy");
        assert_eq!(
            dto.initial_goal.as_deref(),
            Some("Ship dual-mode orchestration")
        );
        assert!(dto.use_slash_commands);
    }
}
