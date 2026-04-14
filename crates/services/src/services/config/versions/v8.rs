use executors::{executors::BaseCodingAgent, profile::ExecutorProfileId};
use serde::{Deserialize, Serialize};
use ts_rs::TS;
pub use v7::{
    EditorConfig, EditorType, GitHubConfig, NotificationConfig, ShowcaseState, SoundFile,
    ThemeMode, UiLanguage,
};

use crate::services::config::versions::v7;

fn default_git_branch_prefix() -> String {
    "vk".to_string()
}

fn default_pr_auto_description_enabled() -> bool {
    true
}

fn default_workflow_model_library() -> Vec<WorkflowModelLibraryItem> {
    Vec::new()
}

#[derive(Clone, Debug, Serialize, Deserialize, TS)]
#[serde(rename_all = "camelCase")]
pub struct WorkflowModelLibraryItem {
    pub id: String,
    pub display_name: String,
    #[serde(default)]
    pub cli_type_id: Option<String>,
    pub api_type: String,
    pub base_url: String,
    pub api_key: String,
    pub model_id: String,
    pub is_verified: bool,
}

#[allow(clippy::struct_excessive_bools, clippy::struct_field_names)]
#[derive(Clone, Debug, Serialize, Deserialize, TS)]
pub struct Config {
    pub config_version: String,
    pub theme: ThemeMode,
    pub executor_profile: ExecutorProfileId,
    pub disclaimer_acknowledged: bool,
    pub onboarding_acknowledged: bool,
    #[serde(default)]
    pub github_login_acknowledged: bool,
    #[serde(default)]
    pub login_acknowledged: bool,
    #[serde(default)]
    pub telemetry_acknowledged: bool,
    pub notifications: NotificationConfig,
    pub editor: EditorConfig,
    pub github: GitHubConfig,
    pub analytics_enabled: bool,
    pub workspace_dir: Option<String>,
    pub last_app_version: Option<String>,
    pub show_release_notes: bool,
    #[serde(default)]
    pub language: UiLanguage,
    #[serde(default = "default_git_branch_prefix")]
    pub git_branch_prefix: String,
    #[serde(default)]
    pub showcases: ShowcaseState,
    #[serde(default = "default_pr_auto_description_enabled")]
    pub pr_auto_description_enabled: bool,
    #[serde(default)]
    pub pr_auto_description_prompt: Option<String>,
    #[serde(default)]
    pub beta_workspaces: bool,
    #[serde(default)]
    pub beta_workspaces_invitation_sent: bool,
    #[serde(default = "default_workflow_model_library")]
    pub workflow_model_library: Vec<WorkflowModelLibraryItem>,
}

impl Config {
    fn from_v7_config(old_config: v7::Config) -> Self {
        // Convert Option<bool> to bool: None or Some(true) become true, Some(false) stays false
        let analytics_enabled = old_config.analytics_enabled.unwrap_or(true);

        Self {
            config_version: "v8".to_string(),
            theme: old_config.theme,
            executor_profile: old_config.executor_profile,
            disclaimer_acknowledged: old_config.disclaimer_acknowledged,
            onboarding_acknowledged: old_config.onboarding_acknowledged,
            github_login_acknowledged: old_config.github_login_acknowledged,
            login_acknowledged: old_config.login_acknowledged,
            telemetry_acknowledged: old_config.telemetry_acknowledged,
            notifications: old_config.notifications,
            editor: old_config.editor,
            github: old_config.github,
            analytics_enabled,
            workspace_dir: old_config.workspace_dir,
            last_app_version: old_config.last_app_version,
            show_release_notes: old_config.show_release_notes,
            language: old_config.language,
            git_branch_prefix: old_config.git_branch_prefix,
            showcases: old_config.showcases,
            pr_auto_description_enabled: true,
            pr_auto_description_prompt: None,
            beta_workspaces: false,
            beta_workspaces_invitation_sent: false,
            workflow_model_library: default_workflow_model_library(),
        }
    }

    pub fn from_previous_version(raw_config: &str) -> Self {
        let old_config = v7::Config::from(raw_config.to_string());
        Self::from_v7_config(old_config)
    }
}

impl From<String> for Config {
    fn from(raw_config: String) -> Self {
        if let Ok(config) = serde_json::from_str::<Config>(&raw_config)
            && config.config_version == "v8"
        {
            return config;
        }

        let config = Self::from_previous_version(&raw_config);
        tracing::info!("Config upgraded to v8");
        config
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            config_version: "v8".to_string(),
            theme: ThemeMode::System,
            executor_profile: ExecutorProfileId::new(BaseCodingAgent::ClaudeCode),
            disclaimer_acknowledged: false,
            onboarding_acknowledged: false,
            github_login_acknowledged: false,
            login_acknowledged: false,
            telemetry_acknowledged: false,
            notifications: NotificationConfig::default(),
            editor: EditorConfig::default(),
            github: GitHubConfig::default(),
            analytics_enabled: true,
            workspace_dir: None,
            last_app_version: None,
            show_release_notes: false,
            language: UiLanguage::default(),
            git_branch_prefix: default_git_branch_prefix(),
            showcases: ShowcaseState::default(),
            pr_auto_description_enabled: true,
            pr_auto_description_prompt: None,
            beta_workspaces: false,
            beta_workspaces_invitation_sent: false,
            workflow_model_library: default_workflow_model_library(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{Config, WorkflowModelLibraryItem};

    #[test]
    fn missing_workflow_model_library_defaults_to_empty() {
        let raw = r#"{
            "config_version":"v8",
            "theme":"SYSTEM",
            "executor_profile":{"executor":"CLAUDE_CODE"},
            "disclaimer_acknowledged":true,
            "onboarding_acknowledged":true,
            "notifications":{"sound_enabled":true,"push_enabled":true,"sound_file":"COW_MOOING"},
            "editor":{"editor_type":"VS_CODE","custom_command":null,"remote_ssh_host":null,"remote_ssh_user":null},
            "github":{"pat":null,"oauth_token":null,"username":null,"primary_email":null,"default_pr_base":"main"},
            "analytics_enabled":true,
            "workspace_dir":null,
            "last_app_version":"0.0.153",
            "show_release_notes":false,
            "language":"ZH_HANS",
            "git_branch_prefix":"vk",
            "showcases":{"seen_features":[]},
            "pr_auto_description_enabled":true,
            "pr_auto_description_prompt":null,
            "beta_workspaces":false,
            "beta_workspaces_invitation_sent":false
        }"#;

        let parsed = serde_json::from_str::<Config>(raw).expect("config should deserialize");
        assert!(parsed.workflow_model_library.is_empty());
    }

    #[test]
    fn workflow_model_library_item_uses_camel_case_keys() {
        let item = WorkflowModelLibraryItem {
            id: "id-1".to_string(),
            display_name: "OpenAI".to_string(),
            cli_type_id: Some("cli-codex".to_string()),
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model_id: "gpt-4.1".to_string(),
            is_verified: true,
        };

        let value = serde_json::to_value(item).expect("item should serialize");
        assert_eq!(value["displayName"], "OpenAI");
        assert_eq!(value["cliTypeId"], "cli-codex");
        assert_eq!(value["apiType"], "openai");
        assert_eq!(value["baseUrl"], "https://api.openai.com/v1");
        assert_eq!(value["apiKey"], "sk-test");
        assert_eq!(value["modelId"], "gpt-4.1");
        assert_eq!(value["isVerified"], true);
    }
}
