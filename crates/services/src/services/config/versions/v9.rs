use executors::{executors::BaseCodingAgent, profile::ExecutorProfileId};
use serde::{Deserialize, Serialize};
use ts_rs::TS;
pub use v8::{
    EditorConfig, EditorType, GitHubConfig, NotificationConfig, ShowcaseState, SoundFile,
    ThemeMode, UiLanguage, WorkflowModelLibraryItem,
};

use crate::services::config::versions::v8;

fn default_git_branch_prefix() -> String {
    "vk".to_string()
}

fn default_pr_auto_description_enabled() -> bool {
    // PR auto-description is user-initiated (generated when the user opens
    // the create-PR dialog, where the description is editable before
    // submit). Defaulting to `true` preserves the pre-v9 behavior and does
    // not send data without user interaction.
    true
}

fn default_workflow_model_library() -> Vec<WorkflowModelLibraryItem> {
    Vec::new()
}

#[allow(clippy::struct_excessive_bools, clippy::struct_field_names)]
#[derive(Clone, Debug, Serialize, Deserialize, TS)]
pub struct Config {
    pub config_version: String,
    pub theme: ThemeMode,
    pub executor_profile: ExecutorProfileId,
    pub disclaimer_acknowledged: bool,
    pub onboarding_acknowledged: bool,
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
    #[serde(default)]
    pub setup_wizard_completed: bool,
}

impl Config {
    fn from_v8_config(old_config: v8::Config) -> Self {
        Self {
            config_version: "v9".to_string(),
            theme: old_config.theme,
            executor_profile: old_config.executor_profile,
            disclaimer_acknowledged: old_config.disclaimer_acknowledged,
            onboarding_acknowledged: old_config.onboarding_acknowledged,
            notifications: old_config.notifications,
            editor: old_config.editor,
            github: old_config.github,
            analytics_enabled: old_config.analytics_enabled,
            workspace_dir: old_config.workspace_dir,
            last_app_version: old_config.last_app_version,
            show_release_notes: old_config.show_release_notes,
            language: old_config.language,
            git_branch_prefix: old_config.git_branch_prefix,
            showcases: old_config.showcases,
            pr_auto_description_enabled: old_config.pr_auto_description_enabled,
            pr_auto_description_prompt: old_config.pr_auto_description_prompt,
            beta_workspaces: old_config.beta_workspaces,
            beta_workspaces_invitation_sent: old_config.beta_workspaces_invitation_sent,
            workflow_model_library: old_config.workflow_model_library,
            setup_wizard_completed: old_config.onboarding_acknowledged,
        }
    }

    pub fn from_previous_version(raw_config: &str) -> Self {
        let old_config = v8::Config::from(raw_config.to_string());
        Self::from_v8_config(old_config)
    }
}

impl From<String> for Config {
    fn from(raw_config: String) -> Self {
        if let Ok(config) = serde_json::from_str::<Config>(&raw_config)
            && config.config_version == "v9"
        {
            return config;
        }

        let config = Self::from_previous_version(&raw_config);
        tracing::info!("Config upgraded to v9");
        config
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            config_version: "v9".to_string(),
            theme: ThemeMode::System,
            executor_profile: ExecutorProfileId::new(BaseCodingAgent::ClaudeCode),
            disclaimer_acknowledged: false,
            onboarding_acknowledged: false,
            notifications: NotificationConfig::default(),
            editor: EditorConfig::default(),
            github: GitHubConfig::default(),
            // Opt-in by default: there is currently no first-run consent
            // flow (see DisclaimerDialog / OnboardingDialog), so analytics
            // must default to OFF until the user explicitly enables it in
            // Settings -> General.
            analytics_enabled: false,
            workspace_dir: None,
            last_app_version: None,
            show_release_notes: false,
            language: UiLanguage::default(),
            git_branch_prefix: default_git_branch_prefix(),
            showcases: ShowcaseState::default(),
            // PR auto-description is user-triggered at PR creation time (the
            // user sees and can edit the generated text before submitting),
            // so defaulting to enabled does not bypass user consent. Kept
            // `true` intentionally to match the pre-v9 behavior.
            pr_auto_description_enabled: true,
            pr_auto_description_prompt: None,
            beta_workspaces: false,
            beta_workspaces_invitation_sent: false,
            workflow_model_library: default_workflow_model_library(),
            setup_wizard_completed: false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::Config;

    #[test]
    fn setup_wizard_completed_defaults_to_false() {
        let raw = r#"{
            "config_version":"v9",
            "theme":"SYSTEM",
            "executor_profile":{"executor":"CLAUDE_CODE"},
            "disclaimer_acknowledged":true,
            "onboarding_acknowledged":true,
            "notifications":{"sound_enabled":true,"push_enabled":true,"sound_file":"COW_MOOING"},
            "editor":{"editorType":"VS_CODE","customCommand":null,"remoteSshHost":null,"remoteSshUser":null},
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
            "beta_workspaces_invitation_sent":false,
            "workflow_model_library":[]
        }"#;

        let parsed = serde_json::from_str::<Config>(raw).expect("config should deserialize");
        assert!(!parsed.setup_wizard_completed);
    }

    #[test]
    fn migration_from_v8_sets_setup_wizard_from_onboarding() {
        let v8_raw = r#"{
            "config_version":"v8",
            "theme":"SYSTEM",
            "executor_profile":{"executor":"CLAUDE_CODE"},
            "disclaimer_acknowledged":true,
            "onboarding_acknowledged":true,
            "notifications":{"sound_enabled":true,"push_enabled":true,"sound_file":"COW_MOOING"},
            "editor":{"editorType":"VS_CODE","customCommand":null,"remoteSshHost":null,"remoteSshUser":null},
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
            "beta_workspaces_invitation_sent":false,
            "workflow_model_library":[]
        }"#;

        let migrated = Config::from(v8_raw.to_string());
        assert_eq!(migrated.config_version, "v9");
        assert!(migrated.setup_wizard_completed);
    }
}
