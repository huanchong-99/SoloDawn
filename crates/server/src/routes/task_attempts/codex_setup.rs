use std::path::Path;

use db::models::{
    execution_process::{ExecutionProcess, ExecutionProcessRunReason},
    session::{CreateSession, Session},
    workspace::{Workspace, WorkspaceError},
};
use deployment::Deployment;
use executors::{
    actions::{
        ExecutorAction, ExecutorActionType,
        script::{ScriptContext, ScriptRequest, ScriptRequestLanguage},
    },
    command::{CommandBuildError, CommandBuilder, apply_overrides},
    executors::{ExecutorError, codex::Codex},
};
use services::services::container::ContainerService;
use uuid::Uuid;

use crate::error::ApiError;

fn build_bash_command(program_path: &Path, args: &[String]) -> Result<String, ExecutorError> {
    let quote = |value: &str| {
        shlex::try_quote(value)
            .map(std::borrow::Cow::into_owned)
            .map_err(CommandBuildError::QuoteError)
            .map_err(ExecutorError::from)
    };

    let mut escaped_parts = Vec::with_capacity(args.len() + 1);
    escaped_parts.push(quote(program_path.to_string_lossy().as_ref())?);

    for arg in args {
        escaped_parts.push(quote(arg)?);
    }

    Ok(escaped_parts.join(" "))
}

pub async fn run_codex_setup(
    deployment: &crate::DeploymentImpl,
    workspace: &Workspace,
    codex: &Codex,
) -> Result<ExecutionProcess, ApiError> {
    let latest_process = ExecutionProcess::find_latest_by_workspace_and_run_reason(
        &deployment.db().pool,
        workspace.id,
        &ExecutionProcessRunReason::CodingAgent,
    )
    .await?;

    let executor_action = if let Some(latest_process) = latest_process {
        let latest_action = latest_process
            .executor_action()
            .map_err(|e| ApiError::Workspace(WorkspaceError::ValidationError(e.to_string())))?;
        get_setup_helper_action(codex)
            .await?
            .append_action(latest_action.to_owned())
    } else {
        get_setup_helper_action(codex).await?
    };

    deployment
        .container()
        .ensure_container_exists(workspace)
        .await?;

    // Get or create a session for setup scripts
    let session =
        match Session::find_latest_by_workspace_id(&deployment.db().pool, workspace.id).await? {
            Some(s) => s,
            None => {
                // Create a new session for setup scripts
                Session::create(
                    &deployment.db().pool,
                    &CreateSession {
                        executor: Some("codex".to_string()),
                        model_config_id: None,
                    },
                    Uuid::new_v4(),
                    workspace.id,
                )
                .await?
            }
        };

    let execution_process = deployment
        .container()
        .start_execution(
            workspace,
            &session,
            &executor_action,
            &ExecutionProcessRunReason::SetupScript,
        )
        .await?;
    Ok(execution_process)
}

async fn get_setup_helper_action(codex: &Codex) -> Result<ExecutorAction, ApiError> {
    let mut login_command = CommandBuilder::new(Codex::base_command());
    login_command = login_command.extend_params(["login"]);
    login_command = apply_overrides(login_command, &codex.cmd);

    let (program_path, args) = login_command
        .build_initial()
        .map_err(|err| ApiError::Executor(ExecutorError::from(err)))?
        .into_resolved()
        .await
        .map_err(ApiError::Executor)?;
    let login_script =
        build_bash_command(program_path.as_path(), &args).map_err(ApiError::Executor)?;
    let login_request = ScriptRequest {
        script: login_script,
        language: ScriptRequestLanguage::Bash,
        context: ScriptContext::ToolInstallScript,
        working_dir: None,
    };

    Ok(ExecutorAction::new(
        ExecutorActionType::ScriptRequest(login_request),
        None,
    ))
}

#[cfg(test)]
mod command_escape_tests {
    use std::path::Path;

    use executors::{command::CommandBuildError, executors::ExecutorError};

    use super::build_bash_command;

    #[test]
    fn keeps_command_arguments_as_individual_tokens() {
        let program = Path::new("/usr/local/bin/codex");
        let args = vec![
            "login".to_string(),
            "--profile".to_string(),
            "dev;rm -rf /".to_string(),
            "$(touch /tmp/pwned)".to_string(),
        ];

        let script = build_bash_command(program, &args).expect("command should be escaped");
        let tokens = shlex::split(&script).expect("escaped command should be parseable");

        assert_eq!(tokens.len(), args.len() + 1);
        assert_eq!(tokens[0], "/usr/local/bin/codex");
        assert_eq!(tokens[1], "login");
        assert_eq!(tokens[2], "--profile");
        assert_eq!(tokens[3], "dev;rm -rf /");
        assert_eq!(tokens[4], "$(touch /tmp/pwned)");
    }

    #[test]
    fn supports_executable_path_with_spaces() {
        let program = Path::new("/opt/codex cli/bin/codex");
        let args = vec!["login".to_string()];

        let script = build_bash_command(program, &args).expect("command should be escaped");
        let tokens = shlex::split(&script).expect("escaped command should be parseable");

        assert_eq!(tokens[0], "/opt/codex cli/bin/codex");
        assert_eq!(tokens[1], "login");
    }

    #[test]
    fn rejects_null_byte_arguments() {
        let program = Path::new("/usr/local/bin/codex");
        let args = vec!["bad\0arg".to_string()];

        let error = build_bash_command(program, &args).expect_err("null bytes must be rejected");
        assert!(matches!(
            error,
            ExecutorError::CommandBuild(CommandBuildError::QuoteError(_))
        ));
    }
}
