use std::{
    path::{Path, PathBuf},
    process::Stdio,
    sync::Arc,
};

use async_trait::async_trait;
use command_group::AsyncCommandGroup;
use futures::StreamExt;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use tokio::{fs, process::Command};
use ts_rs::TS;
use uuid::Uuid;
use workspace_utils::{msg_store::MsgStore, path::get_solodawn_temp_dir};

use crate::{
    command::{CmdOverrides, CommandBuilder, apply_overrides},
    env::ExecutionEnv,
    executors::{
        AppendPrompt, AvailabilityInfo, ExecutorError, SpawnedChild, StandardCodingAgentExecutor,
    },
    logs::{
        NormalizedEntry, NormalizedEntryType, plain_text_processor::PlainTextLogProcessor,
        stderr_processor::normalize_stderr_logs, utils::EntryIndexProvider,
    },
    stdout_dup,
};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, TS, JsonSchema)]
pub struct Copilot {
    #[serde(default)]
    pub append_prompt: AppendPrompt,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub model: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub allow_all_tools: Option<bool>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub allow_tool: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub deny_tool: Option<String>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub add_dir: Option<Vec<String>>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub disable_mcp_server: Option<Vec<String>>,
    #[serde(flatten)]
    pub cmd: CmdOverrides,
}

impl Copilot {
    /// Package version for @github/copilot
    const COPILOT_NPX_VERSION: &'static str = "1.0.68";

    fn build_command_builder(&self, log_dir: &str) -> CommandBuilder {
        let base_cmd = format!("npx -y @github/copilot@{}", Self::COPILOT_NPX_VERSION);
        let mut builder = CommandBuilder::new(&base_cmd).params([
            "--no-color",
            "--no-auto-update",
            "--log-level",
            "debug",
            "--log-dir",
            log_dir,
        ]);

        if self.allow_all_tools.unwrap_or(false) {
            builder = builder.extend_params(["--allow-all-tools"]);
        }

        if let Some(model) = &self.model {
            builder = builder.extend_params(["--model", model]);
        }

        if let Some(tool) = &self.allow_tool {
            builder = builder.extend_params(["--allow-tool", tool]);
        }

        if let Some(tool) = &self.deny_tool {
            builder = builder.extend_params(["--deny-tool", tool]);
        }

        if let Some(dirs) = &self.add_dir {
            for dir in dirs {
                builder = builder.extend_params(["--add-dir", dir]);
            }
        }

        if let Some(servers) = &self.disable_mcp_server {
            for server in servers {
                builder = builder.extend_params(["--disable-mcp-server", server]);
            }
        }

        apply_overrides(builder, &self.cmd)
    }
}

#[async_trait]
impl StandardCodingAgentExecutor for Copilot {
    async fn spawn(
        &self,
        current_dir: &Path,
        prompt: &str,
        env: &ExecutionEnv,
    ) -> Result<SpawnedChild, ExecutorError> {
        let log_dir = Self::create_temp_log_dir(current_dir).await?;
        // Deterministic session id: set the UUID for the new session via
        // `--session-id` instead of scraping it from the log directory.
        let session_id = Uuid::new_v4().to_string();
        let combined_prompt = self.append_prompt.combine_prompt(prompt);
        let command_parts = self
            .build_command_builder(&log_dir.to_string_lossy())
            .extend_params([
                "--session-id",
                session_id.as_str(),
                "--prompt",
                combined_prompt.as_str(),
            ])
            .build_initial()?;
        let (program_path, args) = command_parts.into_resolved().await?;

        let mut command = Command::new(program_path);
        command
            .kill_on_drop(true)
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .current_dir(current_dir)
            .args(&args)
            .env("NODE_NO_WARNINGS", "1");

        env.clone()
            .with_profile(&self.cmd)
            .apply_to_command(&mut command);

        let mut child = command.group_spawn()?;

        let (_, appender) = stdout_dup::tee_stdout_with_appender(&mut child)?;
        appender.append_line(format!("{}{}", Self::SESSION_PREFIX, session_id));

        Ok(child.into())
    }

    async fn spawn_follow_up(
        &self,
        current_dir: &Path,
        prompt: &str,
        session_id: &str,
        env: &ExecutionEnv,
    ) -> Result<SpawnedChild, ExecutorError> {
        let log_dir = Self::create_temp_log_dir(current_dir).await?;
        let combined_prompt = self.append_prompt.combine_prompt(prompt);
        let command_parts = self
            .build_command_builder(&log_dir.to_string_lossy())
            .build_follow_up(&[
                "--resume".to_string(),
                session_id.to_string(),
                "--prompt".to_string(),
                combined_prompt,
            ])?;
        let (program_path, args) = command_parts.into_resolved().await?;

        let mut command = Command::new(program_path);

        command
            .kill_on_drop(true)
            .stdin(Stdio::null())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .current_dir(current_dir)
            .args(&args)
            .env("NODE_NO_WARNINGS", "1");

        env.clone()
            .with_profile(&self.cmd)
            .apply_to_command(&mut command);

        let mut child = command.group_spawn()?;

        let (_, appender) = stdout_dup::tee_stdout_with_appender(&mut child)?;
        appender.append_line(format!("{}{}", Self::SESSION_PREFIX, session_id));

        Ok(child.into())
    }

    /// Parses both stderr and stdout logs for Copilot executor using PlainTextLogProcessor.
    ///
    /// Each entry is converted into an `AssistantMessage` or `ErrorMessage` and emitted as patches.
    fn normalize_logs(&self, msg_store: Arc<MsgStore>, _worktree_path: &Path) {
        let entry_index_counter = EntryIndexProvider::start_from(&msg_store);
        normalize_stderr_logs(msg_store.clone(), entry_index_counter.clone());

        // Normalize Agent logs
        tokio::spawn(async move {
            let mut stdout_lines = msg_store.stdout_lines_stream();

            let mut processor = Self::create_simple_stdout_normalizer(entry_index_counter);

            while let Some(Ok(line)) = stdout_lines.next().await {
                if let Some(session_id) = line.strip_prefix(Self::SESSION_PREFIX) {
                    msg_store.push_session_id(session_id.trim().to_string());
                    continue;
                }

                for patch in processor.process(line + "\n") {
                    msg_store.push_patch(patch);
                }
            }
        });
    }

    // MCP configuration methods
    fn default_mcp_config_path(&self) -> Option<std::path::PathBuf> {
        dirs::home_dir().map(|home| home.join(".copilot").join("mcp-config.json"))
    }

    fn get_availability_info(&self) -> AvailabilityInfo {
        let mcp_config_found = self.default_mcp_config_path().is_some_and(|p| p.exists());

        let installation_indicator_found =
            dirs::home_dir().is_some_and(|home| home.join(".copilot").join("config.json").exists());

        if mcp_config_found || installation_indicator_found {
            AvailabilityInfo::InstallationFound
        } else {
            AvailabilityInfo::NotFound
        }
    }
}

impl Copilot {
    fn create_simple_stdout_normalizer(
        index_provider: EntryIndexProvider,
    ) -> PlainTextLogProcessor {
        PlainTextLogProcessor::builder()
            .normalized_entry_producer(Box::new(|content: String| NormalizedEntry {
                timestamp: None,
                entry_type: NormalizedEntryType::AssistantMessage,
                content,
                metadata: None,
            }))
            .transform_lines(Box::new(|lines| {
                for line in lines.iter_mut() {
                    *line = strip_ansi_escapes::strip_str(&line);
                }
            }))
            .index_provider(index_provider)
            .build()
    }

    async fn create_temp_log_dir(current_dir: &Path) -> Result<PathBuf, ExecutorError> {
        let base_log_dir = get_solodawn_temp_dir().join("copilot_logs");
        fs::create_dir_all(&base_log_dir)
            .await
            .map_err(ExecutorError::Io)?;

        let run_log_dir = base_log_dir
            .join(current_dir.file_name().unwrap_or_default())
            .join(Uuid::new_v4().to_string());
        fs::create_dir_all(&run_log_dir)
            .await
            .map_err(ExecutorError::Io)?;

        Ok(run_log_dir)
    }

    const SESSION_PREFIX: &'static str = "[copilot-session] ";
}
