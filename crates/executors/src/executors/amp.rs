use std::{path::Path, process::Stdio, sync::Arc};

use async_trait::async_trait;
use command_group::AsyncCommandGroup;
use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use tokio::{io::AsyncWriteExt, process::Command};
use ts_rs::TS;
use workspace_utils::{msg_store::MsgStore, shell::resolve_executable_path_blocking};

use crate::{
    command::{CmdOverrides, CommandBuilder, apply_overrides},
    env::ExecutionEnv,
    executors::{
        AppendPrompt, AvailabilityInfo, ExecutorError, SpawnedChild, StandardCodingAgentExecutor,
        claude::{ClaudeLogProcessor, HistoryStrategy},
    },
    logs::{stderr_processor::normalize_stderr_logs, utils::EntryIndexProvider},
};

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, TS, JsonSchema)]
pub struct Amp {
    #[serde(default)]
    pub append_prompt: AppendPrompt,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    #[schemars(
        title = "Dangerously Allow All",
        description = "Deprecated: no longer supported by Amp CLI (permissions moved to Amp's settings/plugin system); this option has no effect."
    )]
    pub dangerously_allow_all: Option<bool>,
    #[serde(flatten)]
    pub cmd: CmdOverrides,
}

impl Amp {
    /// Build the command line. `thread_args` are subcommand words (e.g.
    /// `threads continue <id>`) which must precede the flags.
    /// `--no-archive-after-execute` keeps execute-mode threads continuable.
    fn build_command_builder(&self, thread_args: &[&str]) -> CommandBuilder {
        let builder = CommandBuilder::new("npx -y @ampcode/cli@0.0.1782995668-g845e5b")
            .params(thread_args.iter().copied())
            .extend_params(["--execute", "--stream-json", "--no-archive-after-execute"]);
        apply_overrides(builder, &self.cmd)
    }
}

#[async_trait]
impl StandardCodingAgentExecutor for Amp {
    async fn spawn(
        &self,
        current_dir: &Path,
        prompt: &str,
        env: &ExecutionEnv,
    ) -> Result<SpawnedChild, ExecutorError> {
        let command_parts = self.build_command_builder(&[]).build_initial()?;
        let (executable_path, args) = command_parts.into_resolved().await?;

        let combined_prompt = self.append_prompt.combine_prompt(prompt);

        let mut command = Command::new(executable_path);
        command
            .kill_on_drop(true)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .current_dir(current_dir)
            .args(&args);

        env.clone()
            .with_profile(&self.cmd)
            .apply_to_command(&mut command);

        let mut child = command.group_spawn()?;

        // Feed the prompt in, then close the pipe so amp sees EOF
        if let Some(mut stdin) = child.inner().stdin.take() {
            stdin.write_all(combined_prompt.as_bytes()).await?;
            stdin.shutdown().await?;
        }

        Ok(child.into())
    }

    async fn spawn_follow_up(
        &self,
        current_dir: &Path,
        prompt: &str,
        session_id: &str,
        env: &ExecutionEnv,
    ) -> Result<SpawnedChild, ExecutorError> {
        // Continue the existing thread (`amp threads fork` no longer exists upstream)
        let continue_line = self
            .build_command_builder(&["threads", "continue", session_id])
            .build_follow_up(&[])?;
        let (continue_program, continue_args) = continue_line.into_resolved().await?;

        let combined_prompt = self.append_prompt.combine_prompt(prompt);

        let mut command = Command::new(continue_program);
        command
            .kill_on_drop(true)
            .stdin(Stdio::piped())
            .stdout(Stdio::piped())
            .stderr(Stdio::piped())
            .current_dir(current_dir)
            .args(&continue_args);

        env.clone()
            .with_profile(&self.cmd)
            .apply_to_command(&mut command);

        let mut child = command.group_spawn()?;

        // Feed the prompt in, then close the pipe so amp sees EOF
        if let Some(mut stdin) = child.inner().stdin.take() {
            stdin.write_all(combined_prompt.as_bytes()).await?;
            stdin.shutdown().await?;
        }

        Ok(child.into())
    }

    fn normalize_logs(&self, msg_store: Arc<MsgStore>, current_dir: &Path) {
        let entry_index_provider = EntryIndexProvider::start_from(&msg_store);

        // Process stdout logs (Amp's stream JSON output) using Claude's log processor
        ClaudeLogProcessor::process_logs(
            msg_store.clone(),
            current_dir,
            entry_index_provider.clone(),
            HistoryStrategy::AmpResume,
        );

        // Process stderr logs using the standard stderr processor
        normalize_stderr_logs(msg_store, entry_index_provider);
    }

    // MCP configuration methods
    fn default_mcp_config_path(&self) -> Option<std::path::PathBuf> {
        dirs::home_dir().map(|home| home.join(".config").join("amp").join("settings.json"))
    }

    fn get_availability_info(&self) -> AvailabilityInfo {
        let config_found = self.default_mcp_config_path().is_some_and(|p| p.exists());
        let binary_found = resolve_executable_path_blocking("amp").is_some();

        if config_found || binary_found {
            AvailabilityInfo::InstallationFound
        } else {
            AvailabilityInfo::NotFound
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn amp() -> Amp {
        Amp {
            append_prompt: AppendPrompt::default(),
            dangerously_allow_all: Some(true),
            cmd: CmdOverrides::default(),
        }
    }

    #[test]
    fn initial_build_uses_new_package_and_params() {
        let command = amp()
            .build_command_builder(&[])
            .build_initial()
            .expect("should build");
        assert_eq!(
            command.args(),
            [
                "-y",
                "@ampcode/cli@0.0.1782995668-g845e5b",
                "--execute",
                "--stream-json",
                "--no-archive-after-execute",
            ]
        );
    }

    #[test]
    fn follow_up_build_puts_threads_continue_before_flags() {
        let command = amp()
            .build_command_builder(&["threads", "continue", "T-123"])
            .build_follow_up(&[])
            .expect("should build");
        assert_eq!(
            command.args(),
            [
                "-y",
                "@ampcode/cli@0.0.1782995668-g845e5b",
                "threads",
                "continue",
                "T-123",
                "--execute",
                "--stream-json",
                "--no-archive-after-execute",
            ]
        );
    }
}
