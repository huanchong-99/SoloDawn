use std::{collections::HashMap, path::PathBuf};

use schemars::JsonSchema;
use serde::{Deserialize, Serialize};
use thiserror::Error;
use ts_rs::TS;
use workspace_utils::shell::resolve_executable_path;

use crate::executors::ExecutorError;

#[derive(Debug, Error)]
pub enum CommandBuildError {
    #[error("base command cannot be parsed: {0}")]
    InvalidBase(String),
    #[error("base command is empty after parsing")]
    EmptyCommand,
    #[error("failed to quote command: {0}")]
    QuoteError(#[from] shlex::QuoteError),
}

#[derive(Debug, Clone)]
pub struct CommandParts {
    program: String,
    args: Vec<String>,
}

impl CommandParts {
    pub fn new(program: String, args: Vec<String>) -> Self {
        Self { program, args }
    }

    pub async fn into_resolved(self) -> Result<(PathBuf, Vec<String>), ExecutorError> {
        let CommandParts { program, args } = self;
        let executable = resolve_executable_path(&program)
            .await
            .ok_or(ExecutorError::ExecutableNotFound { program })?;
        Ok((executable, args))
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, TS, JsonSchema, Default)]
pub struct CmdOverrides {
    #[schemars(
        title = "Base Command Override",
        description = "Override the base command with a custom command"
    )]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub base_command_override: Option<String>,
    #[schemars(
        title = "Additional Parameters",
        description = "Additional parameters to append to the base command"
    )]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub additional_params: Option<Vec<String>>,
    #[schemars(
        title = "Environment Variables",
        description = "Environment variables to set when running the executor"
    )]
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub env: Option<HashMap<String, String>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, TS, JsonSchema)]
pub struct CommandBuilder {
    /// Base executable command (e.g., "npx -y @anthropic-ai/claude-code@latest")
    pub base: String,
    /// Optional parameters to append to the base command
    pub params: Option<Vec<String>>,
}

impl CommandBuilder {
    pub fn new<S: Into<String>>(base: S) -> Self {
        Self {
            base: base.into(),
            params: None,
        }
    }

    #[must_use]
    pub fn params<I>(mut self, params: I) -> Self
    where
        I: IntoIterator,
        I::Item: Into<String>,
    {
        self.params = Some(params.into_iter().map(Into::into).collect());
        self
    }

    #[must_use]
    pub fn override_base<S: Into<String>>(mut self, base: S) -> Self {
        self.base = base.into();
        self
    }

    #[must_use]
    pub fn extend_params<I>(mut self, more: I) -> Self
    where
        I: IntoIterator,
        I::Item: Into<String>,
    {
        let extra: Vec<String> = more.into_iter().map(Into::into).collect();
        match &mut self.params {
            Some(p) => p.extend(extra),
            None => self.params = Some(extra),
        }
        self
    }

    pub fn build_initial(&self) -> Result<CommandParts, CommandBuildError> {
        self.build(&[])
    }

    pub fn build_follow_up(
        &self,
        additional_args: &[String],
    ) -> Result<CommandParts, CommandBuildError> {
        self.build(additional_args)
    }

    fn build(&self, additional_args: &[String]) -> Result<CommandParts, CommandBuildError> {
        let mut parts = split_command_line(&self.base)?;
        let program = parts.remove(0);

        if let Some(ref params) = self.params {
            parts.extend(params.iter().cloned());
        }
        parts.extend(additional_args.iter().cloned());

        Ok(CommandParts::new(program, parts))
    }
}

fn split_command_line(input: &str) -> Result<Vec<String>, CommandBuildError> {
    #[cfg(windows)]
    {
        let parts = winsplit::split(input);
        if parts.is_empty() {
            Err(CommandBuildError::EmptyCommand)
        } else {
            Ok(parts)
        }
    }

    #[cfg(not(windows))]
    {
        let parts =
            shlex::split(input).ok_or_else(|| CommandBuildError::InvalidBase(input.to_string()))?;
        if parts.is_empty() {
            Err(CommandBuildError::EmptyCommand)
        } else {
            Ok(parts)
        }
    }
}

pub fn apply_overrides(builder: CommandBuilder, overrides: &CmdOverrides) -> CommandBuilder {
    let builder = if let Some(ref base) = overrides.base_command_override {
        builder.override_base(base.clone())
    } else {
        builder
    };
    if let Some(ref extra) = overrides.additional_params {
        builder.extend_params(extra.clone())
    } else {
        builder
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn empty_base_returns_empty_command_error() {
        let builder = CommandBuilder::new("");
        let err = builder
            .build_initial()
            .expect_err("empty base should not build command");
        assert!(matches!(err, CommandBuildError::EmptyCommand));
    }

    #[test]
    fn split_empty_input_returns_empty_command_error() {
        let err = split_command_line("").expect_err("empty command line should fail");
        assert!(matches!(err, CommandBuildError::EmptyCommand));
    }

    #[test]
    fn simple_base_builds_correctly() {
        let builder = CommandBuilder::new("claude --print");
        let command = builder.build_initial().expect("should build");
        assert_eq!(command.program, "claude");
        assert_eq!(command.args, vec!["--print"]);
    }

    #[test]
    fn params_appended_to_base() {
        let builder = CommandBuilder::new("claude").params(["--print", "--verbose"]);
        let command = builder.build_initial().expect("should build");
        assert_eq!(command.program, "claude");
        assert_eq!(command.args, vec!["--print", "--verbose"]);
    }

    #[test]
    fn follow_up_appends_additional_args() {
        let builder = CommandBuilder::new("claude").params(["--print"]);
        let additional = vec!["--resume".to_string(), "session-123".to_string()];
        let command = builder
            .build_follow_up(&additional)
            .expect("should build follow-up");
        assert_eq!(command.program, "claude");
        assert_eq!(command.args, vec!["--print", "--resume", "session-123"]);
    }

    #[test]
    fn override_base_replaces_command() {
        let builder = CommandBuilder::new("claude --print").override_base("gemini --run");
        let command = builder.build_initial().expect("should build");
        assert_eq!(command.program, "gemini");
        assert_eq!(command.args, vec!["--run"]);
    }

    #[test]
    fn extend_params_adds_to_existing() {
        let builder = CommandBuilder::new("claude")
            .params(["--print"])
            .extend_params(["--verbose"]);
        let command = builder.build_initial().expect("should build");
        assert_eq!(command.program, "claude");
        assert_eq!(command.args, vec!["--print", "--verbose"]);
    }

    #[test]
    fn follow_up_args_not_reparsed() {
        let builder = CommandBuilder::new("agent --mode run").params(["--prompt", "line one"]);
        let additional = vec![
            "--resume".to_string(),
            "session with spaces".to_string(),
            "unterminated\"quote".to_string(),
        ];

        let command = builder
            .build_follow_up(&additional)
            .expect("follow-up args should not be reparsed");

        assert_eq!(command.program, "agent");
        assert_eq!(
            command.args,
            vec![
                "--mode",
                "run",
                "--prompt",
                "line one",
                "--resume",
                "session with spaces",
                "unterminated\"quote",
            ]
        );
    }
}
