//! Workflow Model
//!
//! Stores workflow configuration and state for multi-terminal orchestration.

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::{FromRow, SqlitePool, Type};
use strum_macros::{Display, EnumString};
use tracing::{debug, instrument};
use ts_rs::TS;
use uuid::Uuid;

// Import Terminal type for batch operations
use super::terminal::Terminal;

/// Workflow Status Enum
#[derive(
    Debug,
    Clone,
    Copy,
    PartialEq,
    Eq,
    Type,
    Serialize,
    Deserialize,
    TS,
    EnumString,
    Display,
    Default,
)]
#[sqlx(type_name = "workflow_status", rename_all = "lowercase")]
#[serde(rename_all = "snake_case")]
#[strum(serialize_all = "lowercase")]
pub enum WorkflowStatus {
    /// Created, waiting for configuration
    #[default]
    Created,
    /// Starting terminals
    Starting,
    /// All terminals ready, waiting for user to confirm start
    Ready,
    /// Running
    Running,
    /// Paused
    Paused,
    /// Merging branches
    Merging,
    /// Completed
    Completed,
    /// Failed
    Failed,
    /// Cancelled
    Cancelled,
}

/// Workflow Task Status Enum
#[derive(
    Debug,
    Clone,
    Copy,
    PartialEq,
    Eq,
    Type,
    Serialize,
    Deserialize,
    TS,
    EnumString,
    Display,
    Default,
)]
#[sqlx(type_name = "workflow_task_status", rename_all = "lowercase")]
#[serde(rename_all = "snake_case")]
#[strum(serialize_all = "lowercase")]
pub enum WorkflowTaskStatus {
    /// Waiting to execute
    #[default]
    Pending,
    /// Running
    Running,
    /// Waiting for review
    ReviewPending,
    /// Completed
    Completed,
    /// Failed
    Failed,
    /// Cancelled
    Cancelled,
}

/// Workflow
///
/// Corresponds to database table: workflow
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
#[allow(clippy::struct_excessive_bools)]
pub struct Workflow {
    /// Primary key ID (UUID as String for compatibility)
    pub id: String,

    /// Associated project ID (BLOB in database)
    pub project_id: Uuid,

    /// Workflow name
    pub name: String,

    /// Workflow description
    pub description: Option<String>,

    /// Status (stored as TEXT in SQLite for backward compatibility).
    ///
    /// # Design Decision
    /// This field is a `String` rather than `WorkflowStatus` enum because SQLite
    /// stores it as TEXT. Migrating to a typed enum field would require:
    /// 1. A database migration to add CHECK constraints
    /// 2. Updating all raw SQL queries that compare/set status strings
    /// 3. Changing the `FromRow` derivation to handle enum deserialization
    ///
    /// This is tracked as a future improvement. In the meantime, use the
    /// `WorkflowStatus` enum and constants in `orchestrator::constants` for
    /// compile-time safety at the application layer.
    pub status: String,

    /// Execution mode: diy | agent_planned
    #[serde(default = "default_execution_mode_diy")]
    pub execution_mode: String,

    /// Initial goal for agent-planned workflows
    pub initial_goal: Option<String>,

    /// Use slash commands
    #[serde(default)]
    pub use_slash_commands: bool,

    /// Enable main Agent
    #[serde(default)]
    pub orchestrator_enabled: bool,

    /// Main Agent API type: 'openai' | 'anthropic' | 'custom'
    pub orchestrator_api_type: Option<String>,

    /// Main Agent API Base URL
    pub orchestrator_base_url: Option<String>,

    /// Main Agent API Key (encrypted storage)
    #[serde(skip)]
    pub orchestrator_api_key: Option<String>,

    /// Main Agent model
    pub orchestrator_model: Option<String>,

    /// Enable error handling terminal
    #[serde(default)]
    pub error_terminal_enabled: bool,

    /// Error handling terminal CLI ID
    pub error_terminal_cli_id: Option<String>,

    /// Error handling terminal model ID
    pub error_terminal_model_id: Option<String>,

    /// Merge terminal CLI ID
    pub merge_terminal_cli_id: String,

    /// Merge terminal model ID
    pub merge_terminal_model_id: String,

    /// Target branch
    pub target_branch: String,

    /// Enable git watcher for this workflow
    #[serde(default = "default_true")]
    pub git_watcher_enabled: bool,

    /// All terminals ready timestamp
    pub ready_at: Option<DateTime<Utc>>,

    /// User confirmed start timestamp
    pub started_at: Option<DateTime<Utc>>,

    /// Completed timestamp
    pub completed_at: Option<DateTime<Utc>>,

    /// Created timestamp
    pub created_at: DateTime<Utc>,

    /// Updated timestamp
    pub updated_at: DateTime<Utc>,

    /// Reason for pause (e.g., "api_exhausted", "user_requested")
    pub pause_reason: Option<String>,
}

impl Workflow {
    /// Set API key with encryption
    pub fn set_api_key(&mut self, plaintext: &str) -> anyhow::Result<()> {
        self.orchestrator_api_key = Some(crate::encryption::encrypt(plaintext)?);
        tracing::debug!("API key encrypted for workflow {}", self.id);
        Ok(())
    }

    /// Get API key with decryption
    pub fn get_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.orchestrator_api_key {
            None => Ok(None),
            Some(encoded) => crate::encryption::decrypt(encoded).map(Some),
        }
    }
}

/// Workflow Task
///
/// Corresponds to database table: workflow_task
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowTask {
    /// Primary key ID (UUID as String)
    pub id: String,

    /// Associated workflow ID
    pub workflow_id: String,

    /// Associated solodawn task ID (optional)
    pub vk_task_id: Option<Uuid>,

    /// Task name
    pub name: String,

    /// Task description
    pub description: Option<String>,

    /// Git branch name
    pub branch: String,

    /// Status (stored as TEXT in SQLite for backward compatibility).
    ///
    /// # Design Decision
    /// Same rationale as `Workflow.status` — kept as `String` to avoid a
    /// large-scale migration. Use `WorkflowTaskStatus` enum and
    /// `orchestrator::constants::TASK_STATUS_*` for application-layer safety.
    pub status: String,

    /// Task order
    pub order_index: i32,

    /// Started timestamp
    pub started_at: Option<DateTime<Utc>>,

    /// Completed timestamp
    pub completed_at: Option<DateTime<Utc>>,

    /// Created timestamp
    pub created_at: DateTime<Utc>,

    /// Updated timestamp
    pub updated_at: DateTime<Utc>,
}

/// Slash Command Preset
///
/// Corresponds to database table: slash_command_preset
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct SlashCommandPreset {
    /// Primary key ID
    pub id: String,

    /// Command name, e.g., '/write-code'
    pub command: String,

    /// Command description
    pub description: String,

    /// Prompt template
    pub prompt_template: Option<String>,

    /// Is system built-in
    #[serde(default)]
    pub is_system: bool,

    /// Created timestamp
    pub created_at: DateTime<Utc>,

    /// Updated timestamp
    pub updated_at: DateTime<Utc>,
}

/// Workflow Command Association
///
/// Corresponds to database table: workflow_command
///
/// [G15-010] TODO: The `preset_id` column references `slash_command_preset.id`
/// but the FK has no ON DELETE CASCADE. Deleting a preset while workflow_command
/// rows reference it will cause a foreign-key violation. A future migration
/// should add `ON DELETE CASCADE` or `ON DELETE SET NULL` to this FK.
#[derive(Debug, Clone, FromRow, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowCommand {
    /// Primary key ID
    pub id: String,

    /// Associated workflow ID
    pub workflow_id: String,

    /// Associated preset ID
    pub preset_id: String,

    /// Execution order
    pub order_index: i32,

    /// Custom parameters (JSON format)
    pub custom_params: Option<String>,

    /// Created timestamp
    pub created_at: DateTime<Utc>,
}

/// Create Workflow Task Request
#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CreateWorkflowTaskRequest {
    /// Task ID (optional, auto-generated if not provided)
    pub id: Option<String>,
    /// Task name
    pub name: String,
    /// Task description
    pub description: Option<String>,
    /// Git branch name (optional, auto-generated)
    pub branch: Option<String>,
    /// Task order index
    pub order_index: i32,
    /// Terminals for this task
    pub terminals: Vec<CreateTerminalRequest>,
}

/// Inline model config for custom/temporary model configurations
/// Used when frontend creates new model configs that don't exist in database
#[derive(Debug, Clone, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct InlineModelConfig {
    /// Display name for the model
    pub display_name: String,
    /// API model ID (e.g., "glm-4-plus", "claude-sonnet-4")
    pub model_id: String,
}

/// Default function for auto_confirm field - defaults to true for safety
fn default_auto_confirm_true() -> bool {
    true
}

/// Default function for git_watcher_enabled - defaults to true
fn default_true() -> bool {
    true
}

/// Default execution mode for new workflows
fn default_execution_mode_diy() -> String {
    "diy".to_string()
}

/// Create Terminal Request
#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CreateTerminalRequest {
    /// Terminal ID (optional, auto-generated if not provided)
    pub id: Option<String>,
    /// CLI type ID
    pub cli_type_id: String,
    /// Model config ID
    pub model_config_id: String,
    /// Inline model config (auto-created when model_config_id not found in database)
    pub model_config: Option<InlineModelConfig>,
    /// Custom base URL (overrides model config)
    pub custom_base_url: Option<String>,
    /// Custom API key (encrypted storage)
    pub custom_api_key: Option<String>,
    /// Role description (optional)
    pub role: Option<String>,
    /// Role description (optional)
    pub role_description: Option<String>,
    /// Terminal order index within task
    pub order_index: i32,
    /// Auto-confirm mode: skip CLI permission prompts (defaults to true)
    #[serde(default = "default_auto_confirm_true")]
    pub auto_confirm: bool,
}

/// Create Workflow Request
#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct CreateWorkflowRequest {
    /// Project ID
    pub project_id: String,
    /// Workflow name
    pub name: String,
    /// Workflow description
    pub description: Option<String>,
    /// Workflow execution mode
    #[serde(default = "default_execution_mode_diy")]
    pub execution_mode: String,
    /// High-level goal for agent-planned workflows
    pub initial_goal: Option<String>,
    /// Use slash commands
    pub use_slash_commands: bool,
    /// Workflow commands with custom parameters (in order)
    pub commands: Option<Vec<WorkflowCommandRequest>>,
    /// Main Agent configuration
    pub orchestrator_config: Option<OrchestratorConfig>,
    /// Error handling terminal configuration
    pub error_terminal_config: Option<TerminalConfig>,
    /// Merge terminal configuration
    pub merge_terminal_config: TerminalConfig,
    /// Target branch
    pub target_branch: Option<String>,
    /// Enable git watcher (default true)
    pub git_watcher_enabled: Option<bool>,

    // ========== 新增字段 ==========
    /// Workflow tasks with terminals
    #[serde(default)]
    pub tasks: Vec<CreateWorkflowTaskRequest>,
}

/// Workflow command request for creating workflow
#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct WorkflowCommandRequest {
    /// Preset ID
    pub preset_id: String,
    /// Custom parameters (JSON string)
    pub custom_params: Option<String>,
}

/// Main Agent Configuration
#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct OrchestratorConfig {
    pub api_type: String,
    pub base_url: String,
    pub api_key: String,
    pub model: String,
}

/// Terminal Configuration
#[derive(Debug, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
#[ts(export)]
pub struct TerminalConfig {
    pub cli_type_id: String,
    pub model_config_id: String,
    /// Inline model config (auto-created when model_config_id not found in database)
    pub model_config: Option<InlineModelConfig>,
    pub custom_base_url: Option<String>,
    pub custom_api_key: Option<String>,
}

impl Workflow {
    /// Create workflow
    pub async fn create(pool: &SqlitePool, workflow: &Workflow) -> sqlx::Result<Self> {
        sqlx::query_as::<_, Workflow>(
            r"
            INSERT INTO workflow (
                id, project_id, name, description, status,
                execution_mode, initial_goal,
                use_slash_commands, orchestrator_enabled,
                orchestrator_api_type, orchestrator_base_url,
                orchestrator_api_key, orchestrator_model,
                error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
                merge_terminal_cli_id, merge_terminal_model_id,
                target_branch, git_watcher_enabled, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22)
            RETURNING *
            "
        )
        .bind(&workflow.id)
        .bind(workflow.project_id)
        .bind(&workflow.name)
        .bind(&workflow.description)
        .bind(&workflow.status)
        .bind(&workflow.execution_mode)
        .bind(&workflow.initial_goal)
        .bind(workflow.use_slash_commands)
        .bind(workflow.orchestrator_enabled)
        .bind(&workflow.orchestrator_api_type)
        .bind(&workflow.orchestrator_base_url)
        .bind(&workflow.orchestrator_api_key)
        .bind(&workflow.orchestrator_model)
        .bind(workflow.error_terminal_enabled)
        .bind(&workflow.error_terminal_cli_id)
        .bind(&workflow.error_terminal_model_id)
        .bind(&workflow.merge_terminal_cli_id)
        .bind(&workflow.merge_terminal_model_id)
        .bind(&workflow.target_branch)
        .bind(workflow.git_watcher_enabled)
        .bind(workflow.created_at)
        .bind(workflow.updated_at)
        .fetch_one(pool)
        .await
    }

    /// Find workflow by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, Workflow>(r"SELECT * FROM workflow WHERE id = ?")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Find workflows by project
    #[instrument(skip(pool), fields(project_id))]
    pub async fn find_by_project(pool: &SqlitePool, project_id: Uuid) -> sqlx::Result<Vec<Self>> {
        let start = std::time::Instant::now();
        let result = sqlx::query_as::<_, Workflow>(
            r"
            SELECT * FROM workflow
            WHERE project_id = ?
            ORDER BY created_at DESC
            ",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await;

        let elapsed = start.elapsed();
        debug!(
            project_id = %project_id,
            count = result.as_ref().map_or(0, Vec::len),
            duration_ms = elapsed.as_millis(),
            "find_by_project query completed"
        );

        result
    }
}

/// Workflow with task and terminal counts (optimized for list view)
#[derive(Debug, Clone, FromRow)]
pub struct WorkflowWithCounts {
    pub id: String,
    pub project_id: Uuid,
    pub name: String,
    pub description: Option<String>,
    pub status: String,
    pub execution_mode: String,
    pub created_at: chrono::DateTime<Utc>,
    pub updated_at: chrono::DateTime<Utc>,
    pub tasks_count: i64,
    pub terminals_count: i64,
}

impl Workflow {
    /// Find workflows by project ID with task and terminal counts (optimized single query)
    ///
    /// This avoids the N+1 query problem by using LEFT JOIN and COUNT in a single query.
    #[instrument(skip(pool), fields(project_id))]
    pub async fn find_by_project_with_counts(
        pool: &SqlitePool,
        project_id: Uuid,
    ) -> sqlx::Result<Vec<WorkflowWithCounts>> {
        let start = std::time::Instant::now();
        // NOTE(E38-14): `COUNT(DISTINCT t.id)` / `COUNT(DISTINCT term.id)`
        // are not backed by a dedicated covering index. If this list grows and
        // the query shows up in slow-query traces, consider indexes on
        // workflow_task(workflow_id) and terminal(workflow_task_id) (both
        // should already exist as FK indexes) and verify EXPLAIN output.
        let result = sqlx::query_as::<_, WorkflowWithCounts>(
            r"
            SELECT
                w.id,
                w.project_id,
                w.name,
                w.description,
                w.status,
                w.execution_mode,
                w.created_at,
                w.updated_at,
                COUNT(DISTINCT t.id) as tasks_count,
                COUNT(DISTINCT term.id) as terminals_count
            FROM workflow w
            LEFT JOIN workflow_task t ON w.id = t.workflow_id
            LEFT JOIN terminal term ON t.id = term.workflow_task_id
            WHERE w.project_id = ?
            GROUP BY w.id
            ORDER BY w.created_at DESC
            ",
        )
        .bind(project_id)
        .fetch_all(pool)
        .await;

        let elapsed = start.elapsed();
        debug!(
            project_id = %project_id,
            count = result.as_ref().map_or(0, Vec::len),
            duration_ms = elapsed.as_millis(),
            "find_by_project_with_counts query completed"
        );

        result
    }

    /// Update workflow status (generic, no CAS protection).
    ///
    /// # Caller Responsibility
    /// This method performs an unconditional status update. Callers MUST ensure
    /// the workflow is in a valid source state before invoking this method to
    /// prevent concurrent state regression. For critical transitions (e.g.
    /// starting→ready, ready→running), prefer dedicated CAS methods like
    /// `set_ready` or `set_started`.
    /// Update workflow status.
    ///
    /// [G15-002] Terminal states (`completed`, `failed`, `cancelled`) are protected:
    /// once a workflow reaches one of these states it cannot be overwritten by a
    /// concurrent update. The WHERE clause excludes these final states so the UPDATE
    /// is a no-op if the workflow has already been finalized.
    ///
    /// When transitioning to a terminal state, `completed_at` is set automatically
    /// to prevent dangling incomplete records.
    pub async fn update_status(pool: &SqlitePool, id: &str, status: &str) -> sqlx::Result<()> {
        Self::update_status_with_reason(pool, id, status, None).await
    }

    /// Update workflow status, optionally recording a `pause_reason`.
    ///
    /// R8: `pause_reason` had a column reserved (`crates/db/src/models/workflow.rs`)
    /// but no write path. The new gate-loop classifier auto-pauses workflows
    /// instead of auto-failing them, and needs to record WHY:
    ///   - "quality_gate_plateau"
    ///   - "quality_gate_regression"
    ///   - "quality_gate_wallclock"
    ///   - "user_requested" (manual pause via UI)
    ///   - "recovery_artifact" (R7-PB1 restart-recovery)
    ///
    /// Passing `reason = None` preserves existing behavior; passing
    /// `Some("...")` writes the reason atomically with the status change.
    /// Whether the row's CAS guard fires (status NOT IN terminal states)
    /// is identical between the two helpers.
    pub async fn update_status_with_reason(
        pool: &SqlitePool,
        id: &str,
        status: &str,
        reason: Option<&str>,
    ) -> sqlx::Result<()> {
        let now = Utc::now();
        let is_terminal_state = matches!(status, "completed" | "failed" | "cancelled");
        if is_terminal_state {
            sqlx::query(
                r"
                UPDATE workflow
                SET status = ?, pause_reason = ?, completed_at = COALESCE(completed_at, ?), updated_at = ?
                WHERE id = ?
                  AND status NOT IN ('completed', 'failed', 'cancelled')
                ",
            )
            .bind(status)
            .bind(reason)
            .bind(now)
            .bind(now)
            .bind(id)
            .execute(pool)
            .await?;
        } else {
            sqlx::query(
                r"
                UPDATE workflow
                SET status = ?, pause_reason = ?, updated_at = ?
                WHERE id = ?
                  AND status NOT IN ('completed', 'failed', 'cancelled')
                ",
            )
            .bind(status)
            .bind(reason)
            .bind(now)
            .bind(id)
            .execute(pool)
            .await?;
        }
        Ok(())
    }

    /// Set workflow to ready (only from 'starting' state).
    ///
    /// Uses CAS to ensure workflow is in 'starting' state before transitioning
    /// to 'ready'. Returns error if the workflow is not in 'starting' state.
    pub async fn set_ready(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'ready', ready_at = ?, updated_at = ?
            WHERE id = ? AND status = 'starting'
            ",
        )
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;

        if result.rows_affected() == 0 {
            return Err(sqlx::Error::RowNotFound);
        }

        Ok(())
    }

    /// Set workflow started
    ///
    /// Uses Compare-And-Set (CAS) to ensure workflow is in 'ready' or 'paused'
    /// state before transitioning to 'running'. Returns error if CAS fails.
    pub async fn set_started(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'running', started_at = ?, updated_at = ?
            WHERE id = ? AND status IN ('ready', 'paused')
            ",
        )
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;

        // Check CAS succeeded
        if result.rows_affected() == 0 {
            return Err(sqlx::Error::RowNotFound);
        }

        Ok(())
    }

    /// Atomically transition workflow from 'completed' to 'merging' (CAS).
    ///
    /// Returns `true` if the transition succeeded (exactly one row updated),
    /// `false` if the workflow was not in 'completed' state (another merge
    /// is already in progress or the workflow is in an incompatible state).
    pub async fn set_merging(pool: &SqlitePool, id: &str) -> anyhow::Result<bool> {
        let result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'merging', updated_at = datetime('now')
            WHERE id = ? AND status = 'completed'
            ",
        )
        .bind(id)
        .execute(pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }

    /// Atomically transition workflow from 'merging' back to 'completed'.
    ///
    /// Used after a successful merge or to roll back on merge failure.
    pub async fn set_merge_completed(pool: &SqlitePool, id: &str) -> anyhow::Result<()> {
        sqlx::query(
            r"
            UPDATE workflow
            SET status = 'completed', updated_at = datetime('now')
            WHERE id = ? AND status = 'merging'
            ",
        )
        .bind(id)
        .execute(pool)
        .await?;

        Ok(())
    }

    /// Atomically transition workflow from 'running' to 'completed' (CAS).
    ///
    /// Returns `true` if the transition succeeded, `false` if the workflow
    /// was no longer in 'running' state (e.g., already paused or merging).
    /// This prevents auto-sync completion from overwriting concurrent state changes.
    pub async fn set_completed_from_running(pool: &SqlitePool, id: &str) -> anyhow::Result<bool> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE workflow
            SET status = 'completed', completed_at = COALESCE(completed_at, ?), updated_at = ?
            WHERE id = ? AND status = 'running'
            ",
        )
        .bind(now)
        .bind(now)
        .bind(id)
        .execute(pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }

    /// Delete workflow
    pub async fn delete(pool: &SqlitePool, id: &str) -> sqlx::Result<u64> {
        let result = sqlx::query("DELETE FROM workflow WHERE id = ?")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(result.rows_affected())
    }

    /// Create workflow with tasks and terminals in a single transaction
    ///
    /// This is a batch operation that creates a workflow along with its associated
    /// workflow tasks and terminals atomically. If any part fails, the entire
    /// transaction is rolled back.
    pub async fn create_with_tasks(
        pool: &SqlitePool,
        workflow: &Workflow,
        tasks: Vec<(WorkflowTask, Vec<Terminal>)>,
    ) -> anyhow::Result<()> {
        // Pre-flight: verify reference tables are populated (common cause of FK failures)
        let cli_count: (i64,) =
            sqlx::query_as("SELECT COUNT(*) FROM cli_type")
                .fetch_one(pool)
                .await?;
        if cli_count.0 == 0 {
            anyhow::bail!(
                "cli_type table is empty. Run `pnpm run prepare-db` to initialize seed data."
            );
        }

        let mut tx = pool.begin().await?;

        // Create workflow
        sqlx::query(
            r"
            INSERT INTO workflow (
                id, project_id, name, description, status,
                execution_mode, initial_goal,
                use_slash_commands, orchestrator_enabled,
                orchestrator_api_type, orchestrator_base_url,
                orchestrator_api_key, orchestrator_model,
                error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
                merge_terminal_cli_id, merge_terminal_model_id,
                target_branch, git_watcher_enabled, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22)
            "
        )
        .bind(&workflow.id)
        .bind(workflow.project_id)
        .bind(&workflow.name)
        .bind(&workflow.description)
        .bind(&workflow.status)
        .bind(&workflow.execution_mode)
        .bind(&workflow.initial_goal)
        .bind(workflow.use_slash_commands)
        .bind(workflow.orchestrator_enabled)
        .bind(&workflow.orchestrator_api_type)
        .bind(&workflow.orchestrator_base_url)
        .bind(&workflow.orchestrator_api_key)
        .bind(&workflow.orchestrator_model)
        .bind(workflow.error_terminal_enabled)
        .bind(&workflow.error_terminal_cli_id)
        .bind(&workflow.error_terminal_model_id)
        .bind(&workflow.merge_terminal_cli_id)
        .bind(&workflow.merge_terminal_model_id)
        .bind(&workflow.target_branch)
        .bind(workflow.git_watcher_enabled)
        .bind(workflow.created_at)
        .bind(workflow.updated_at)
        .execute(&mut *tx)
        .await?;

        // Create tasks and terminals
        for (task, terminals) in tasks {
            sqlx::query(
                r"
                INSERT INTO workflow_task (
                    id, workflow_id, vk_task_id, name, description,
                    branch, status, order_index, created_at, updated_at
                ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
                ",
            )
            .bind(&task.id)
            .bind(&task.workflow_id)
            .bind(task.vk_task_id)
            .bind(&task.name)
            .bind(&task.description)
            .bind(&task.branch)
            .bind(&task.status)
            .bind(task.order_index)
            .bind(task.created_at)
            .bind(task.updated_at)
            .execute(&mut *tx)
            .await?;

            // Create terminals for this task
            for terminal in terminals {
                sqlx::query(
                    r"
                    INSERT INTO terminal (
                        id, workflow_task_id, cli_type_id, model_config_id,
                        custom_base_url, custom_api_key, role, role_description,
                        order_index, status, auto_confirm, created_at, updated_at
                    ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)
                    ",
                )
                .bind(&terminal.id)
                .bind(&terminal.workflow_task_id)
                .bind(&terminal.cli_type_id)
                .bind(&terminal.model_config_id)
                .bind(&terminal.custom_base_url)
                .bind(&terminal.custom_api_key)
                .bind(&terminal.role)
                .bind(&terminal.role_description)
                .bind(terminal.order_index)
                .bind(&terminal.status)
                .bind(terminal.auto_confirm)
                .bind(terminal.created_at)
                .bind(terminal.updated_at)
                .execute(&mut *tx)
                .await?;
            }
        }

        tx.commit().await?;
        Ok(())
    }
}

impl WorkflowTask {
    /// Create workflow task
    pub async fn create(pool: &SqlitePool, task: &WorkflowTask) -> sqlx::Result<Self> {
        sqlx::query_as::<_, WorkflowTask>(
            r"
            INSERT INTO workflow_task (
                id, workflow_id, vk_task_id, name, description,
                branch, status, order_index, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
            RETURNING *
            ",
        )
        .bind(&task.id)
        .bind(&task.workflow_id)
        .bind(task.vk_task_id)
        .bind(&task.name)
        .bind(&task.description)
        .bind(&task.branch)
        .bind(&task.status)
        .bind(task.order_index)
        .bind(task.created_at)
        .bind(task.updated_at)
        .fetch_one(pool)
        .await
    }

    /// Find tasks by workflow
    pub async fn find_by_workflow(pool: &SqlitePool, workflow_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, WorkflowTask>(
            r"
            SELECT * FROM workflow_task
            WHERE workflow_id = ?
            ORDER BY order_index ASC
            ",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    /// Find workflow task by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, WorkflowTask>(r"SELECT * FROM workflow_task WHERE id = ?")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Update workflow task status (generic, no CAS protection).
    ///
    /// # Caller Responsibility
    /// This method performs an unconditional status update. Callers MUST verify
    /// the task is not already in a terminal state (completed/failed/cancelled)
    /// before calling, to prevent overwriting finalized results. For safe
    /// transitions, check `task.status` before invoking this method.
    /// Update task status.
    ///
    /// [G15-003] Terminal states (`completed`, `failed`, `cancelled`) are protected:
    /// once a task reaches one of these states it cannot be overwritten by a
    /// concurrent update.
    ///
    /// [G15-009] When transitioning to a terminal state (`completed`, `failed`,
    /// `cancelled`), `completed_at` is set automatically to prevent dangling
    /// incomplete records.
    pub async fn update_status(pool: &SqlitePool, id: &str, status: &str) -> sqlx::Result<()> {
        let now = Utc::now();
        let is_terminal_state = matches!(status, "completed" | "failed" | "cancelled");
        if is_terminal_state {
            sqlx::query(
                r"
                UPDATE workflow_task
                SET status = ?, completed_at = COALESCE(completed_at, ?), updated_at = ?
                WHERE id = ?
                  AND status NOT IN ('completed', 'failed', 'cancelled')
                ",
            )
            .bind(status)
            .bind(now)
            .bind(now)
            .bind(id)
            .execute(pool)
            .await?;
        } else {
            sqlx::query(
                r"
                UPDATE workflow_task
                SET status = ?, updated_at = ?
                WHERE id = ?
                  AND status NOT IN ('completed', 'failed', 'cancelled')
                ",
            )
            .bind(status)
            .bind(now)
            .bind(id)
            .execute(pool)
            .await?;
        }
        Ok(())
    }

    /// Compare-and-set task status for critical transitions.
    ///
    /// Returns `Ok(true)` when the transition succeeds (row matched
    /// `expected_status`), `Ok(false)` when the current status did not match.
    /// Use this for transitions where concurrent updates could cause state
    /// regression (e.g. running → completed while another thread tries
    /// running → failed).
    pub async fn update_status_cas(
        pool: &SqlitePool,
        id: &str,
        expected_status: &str,
        next_status: &str,
    ) -> sqlx::Result<bool> {
        let now = Utc::now();
        let result = sqlx::query(
            r"
            UPDATE workflow_task
            SET status = ?, updated_at = ?
            WHERE id = ? AND status = ?
            ",
        )
        .bind(next_status)
        .bind(now)
        .bind(id)
        .bind(expected_status)
        .execute(pool)
        .await?;

        Ok(result.rows_affected() > 0)
    }
}

impl SlashCommandPreset {
    /// Hard cap used by `find_all` (W2-15-10).
    pub const FIND_ALL_MAX_ROWS: i64 = 500;

    /// Get slash command presets for the command palette.
    ///
    /// W2-15-10: cap the list at [`Self::FIND_ALL_MAX_ROWS`] rows so a large
    /// user-defined preset set cannot fan an unbounded result into the UI. The
    /// palette only renders a bounded list, so deeper pagination can be added
    /// later without changing current callers.
    pub async fn find_all(pool: &SqlitePool) -> sqlx::Result<Vec<Self>> {
        let limit: i64 = Self::FIND_ALL_MAX_ROWS;
        sqlx::query_as::<_, SlashCommandPreset>(
            r"
            SELECT * FROM slash_command_preset
            ORDER BY is_system DESC, command ASC
            LIMIT ?
            ",
        )
        .bind(limit)
        .fetch_all(pool)
        .await
    }

    /// Find a preset by ID
    pub async fn find_by_id(pool: &SqlitePool, id: &str) -> sqlx::Result<Option<Self>> {
        sqlx::query_as::<_, SlashCommandPreset>(r"SELECT * FROM slash_command_preset WHERE id = ?")
            .bind(id)
            .fetch_optional(pool)
            .await
    }

    /// Create a new slash command preset
    pub async fn create(
        pool: &SqlitePool,
        command: &str,
        description: &str,
        prompt_template: Option<&str>,
    ) -> sqlx::Result<Self> {
        let id = Uuid::new_v4().to_string();
        let now = Utc::now();
        sqlx::query_as::<_, SlashCommandPreset>(
            r"
            INSERT INTO slash_command_preset (id, command, description, prompt_template, is_system, created_at, updated_at)
            VALUES (?1, ?2, ?3, ?4, 0, ?5, ?5)
            RETURNING *
            ",
        )
        .bind(&id)
        .bind(command)
        .bind(description)
        .bind(prompt_template)
        .bind(now)
        .fetch_one(pool)
        .await
    }

    /// Update a slash command preset
    pub async fn update(
        pool: &SqlitePool,
        id: &str,
        command: Option<&str>,
        description: Option<&str>,
        prompt_template: Option<&str>,
    ) -> sqlx::Result<Self> {
        let now = Utc::now();
        sqlx::query_as::<_, SlashCommandPreset>(
            r"
            UPDATE slash_command_preset
            SET command = COALESCE(?2, command),
                description = COALESCE(?3, description),
                prompt_template = COALESCE(?4, prompt_template),
                updated_at = ?5
            WHERE id = ?1 AND is_system = 0
            RETURNING *
            ",
        )
        .bind(id)
        .bind(command)
        .bind(description)
        .bind(prompt_template)
        .bind(now)
        .fetch_one(pool)
        .await
    }

    /// Delete a slash command preset (only non-system presets)
    pub async fn delete(pool: &SqlitePool, id: &str) -> sqlx::Result<()> {
        sqlx::query(r"DELETE FROM slash_command_preset WHERE id = ? AND is_system = 0")
            .bind(id)
            .execute(pool)
            .await?;
        Ok(())
    }
}

impl WorkflowCommand {
    /// Get commands by workflow
    pub async fn find_by_workflow(pool: &SqlitePool, workflow_id: &str) -> sqlx::Result<Vec<Self>> {
        sqlx::query_as::<_, WorkflowCommand>(
            r"
            SELECT * FROM workflow_command
            WHERE workflow_id = ?
            ORDER BY order_index ASC
            ",
        )
        .bind(workflow_id)
        .fetch_all(pool)
        .await
    }

    /// Add command to workflow
    pub async fn create(
        pool: &SqlitePool,
        workflow_id: &str,
        preset_id: &str,
        order_index: i32,
        custom_params: Option<&str>,
    ) -> sqlx::Result<Self> {
        let id = Uuid::new_v4().to_string();
        let now = Utc::now();
        sqlx::query_as::<_, WorkflowCommand>(
            r"
            INSERT INTO workflow_command (id, workflow_id, preset_id, order_index, custom_params, created_at)
            VALUES (?1, ?2, ?3, ?4, ?5, ?6)
            RETURNING *
            "
        )
        .bind(&id)
        .bind(workflow_id)
        .bind(preset_id)
        .bind(order_index)
        .bind(custom_params)
        .bind(now)
        .fetch_one(pool)
        .await
    }
}

#[cfg(test)]
mod encryption_tests {
    use serial_test::serial;
    use temp_env::with_var;

    /// Create a test Workflow with default fields
    fn test_workflow(id: &str) -> super::Workflow {
        super::Workflow {
            id: id.to_string(),
            project_id: uuid::Uuid::nil(),
            name: "Test Workflow".to_string(),
            description: None,
            status: "pending".to_string(),
            execution_mode: "diy".to_string(),
            initial_goal: None,
            use_slash_commands: false,
            orchestrator_enabled: false,
            orchestrator_api_type: None,
            orchestrator_base_url: None,
            orchestrator_api_key: None,
            orchestrator_model: None,
            error_terminal_enabled: false,
            error_terminal_cli_id: None,
            error_terminal_model_id: None,
            merge_terminal_cli_id: "merge-cli".to_string(),
            merge_terminal_model_id: "merge-model".to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: None,
            started_at: None,
            completed_at: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
            pause_reason: None,
        }
    }

    use super::*;

    #[test]
    fn test_create_terminal_request_auto_confirm_defaults_to_true() {
        let request: CreateTerminalRequest = serde_json::from_value(serde_json::json!({
            "cliTypeId": "claude-code",
            "modelConfigId": "model-1",
            "orderIndex": 0
        }))
        .expect("deserialization should succeed");

        assert!(
            request.auto_confirm,
            "auto_confirm should default to true when not specified"
        );
    }

    #[test]
    fn test_create_terminal_request_auto_confirm_respects_explicit_false() {
        let request: CreateTerminalRequest = serde_json::from_value(serde_json::json!({
            "cliTypeId": "claude-code",
            "modelConfigId": "model-1",
            "orderIndex": 0,
            "autoConfirm": false
        }))
        .expect("deserialization should succeed");

        assert!(
            !request.auto_confirm,
            "auto_confirm should respect explicit false value"
        );
    }

    #[test]
    fn test_create_terminal_request_auto_confirm_respects_explicit_true() {
        let request: CreateTerminalRequest = serde_json::from_value(serde_json::json!({
            "cliTypeId": "claude-code",
            "modelConfigId": "model-1",
            "orderIndex": 0,
            "autoConfirm": true
        }))
        .expect("deserialization should succeed");

        assert!(
            request.auto_confirm,
            "auto_confirm should respect explicit true value"
        );
    }

    #[test]
    #[serial]
    fn test_api_key_encryption_decryption() {
        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let mut workflow = test_workflow("test-workflow");

                // Test encryption
                let original_key = "sk-test-key-12345";
                workflow.set_api_key(original_key).unwrap();

                assert!(workflow.orchestrator_api_key.is_some());
                assert_ne!(
                    workflow.orchestrator_api_key.as_ref().unwrap(),
                    original_key,
                    "Encrypted key should not match original"
                );

                // Test decryption
                let decrypted_key = workflow.get_api_key().unwrap().unwrap();
                assert_eq!(decrypted_key, original_key);
            },
        );
    }

    #[test]
    #[serial]
    fn test_api_key_encryption_missing_env_key() {
        with_var("SOLODAWN_ENCRYPTION_KEY", Option::<&str>::None, || {
            let mut workflow = test_workflow("test-workflow");

            // Should fail without encryption key
            let result = workflow.set_api_key("sk-test");
            assert!(result.is_err());
            assert!(
                result
                    .unwrap_err()
                    .to_string()
                    .contains("SOLODAWN_ENCRYPTION_KEY")
            );
        });
    }

    #[test]
    #[serial]
    fn test_api_key_encryption_invalid_key_length() {
        with_var("SOLODAWN_ENCRYPTION_KEY", Some("short"), || {
            let mut workflow = test_workflow("test-workflow");

            let result = workflow.set_api_key("sk-test");
            assert!(result.is_err());
            assert!(result.unwrap_err().to_string().contains("32 bytes"));
        });
    }

    #[test]
    #[serial]
    fn test_api_key_none_when_not_set() {
        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let workflow = test_workflow("test-workflow");

                let key = workflow.get_api_key().unwrap();
                assert!(key.is_none());
            },
        );
    }

    #[test]
    #[serial]
    fn test_api_key_serialization_skips_encrypted() {
        with_var(
            "SOLODAWN_ENCRYPTION_KEY",
            Some("12345678901234567890123456789012"),
            || {
                let mut workflow = test_workflow("test-workflow");

                workflow.set_api_key("sk-test").unwrap();

                // Serialize to JSON
                let json = serde_json::to_string(&workflow).unwrap();

                // Encrypted field should not be in JSON (due to #[serde(skip)])
                assert!(!json.contains("orchestrator_api_key"));
                assert!(!json.contains("sk-test"));
            },
        );
    }
}
