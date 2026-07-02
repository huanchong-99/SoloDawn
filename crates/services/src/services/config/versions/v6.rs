use std::str::FromStr;

use anyhow::Error;
use executors::{executors::BaseCodingAgent, profile::ExecutorProfileId};
use serde::{Deserialize, Serialize};
use ts_rs::TS;
use utils;
pub use v5::{EditorConfig, EditorType, GitHubConfig, NotificationConfig, SoundFile, ThemeMode};

use crate::services::config::versions::v5;

#[derive(Clone, Copy, Debug, Serialize, Deserialize, TS, Default)]
#[ts(export)]
#[serde(rename_all = "SCREAMING_SNAKE_CASE")]
pub enum UiLanguage {
    #[default]
    Browser, // Detect from browser
    En,     // Force English
    Ja,     // Force Japanese
    Es,     // Force Spanish
    Ko,     // Force Korean
    ZhHans, // Force Simplified Chinese
    ZhHant, // Force Traditional Chinese
}

#[allow(clippy::struct_excessive_bools, clippy::struct_field_names)]
#[derive(Clone, Debug, Serialize, Deserialize, TS)]
pub struct Config {
    pub config_version: String,
    pub theme: ThemeMode,
    pub executor_profile: ExecutorProfileId,
    pub disclaimer_acknowledged: bool,
    pub onboarding_acknowledged: bool,
    pub github_login_acknowledged: bool,
    pub telemetry_acknowledged: bool,
    pub notifications: NotificationConfig,
    pub editor: EditorConfig,
    pub github: GitHubConfig,
    pub analytics_enabled: Option<bool>,
    pub workspace_dir: Option<String>,
    pub last_app_version: Option<String>,
    pub show_release_notes: bool,
    #[serde(default)]
    pub language: UiLanguage,
}

impl Config {
    // Result kept for uniformity with the migration chain (v2::from_previous_version
    // can still Err on an unparseable v1 config); this level always succeeds because
    // it chains down via v5::Config::from on a failed direct parse.
    #[allow(clippy::unnecessary_wraps)]
    pub fn from_previous_version(raw_config: &str) -> Result<Self, Error> {
        let old_config = match serde_json::from_str::<v5::Config>(raw_config) {
            Ok(cfg) => cfg,
            Err(e) => {
                // The on-disk config is older than v5. Chain down through v5's own
                // migration instead of returning Err here — From<String> maps an Err
                // to Self::default(), which would silently discard the user's data.
                tracing::warn!("Direct v5 parse failed ({e}); chaining migration from an older version");
                v5::Config::from(raw_config.to_string())
            }
        };

        // Backup custom profiles.json if it exists (v6 migration may break compatibility)
        match utils::assets::profiles_path() {
            Ok(profiles_path) => {
                if profiles_path.exists() {
                    let backup_name = format!(
                        "profiles_v5_backup_{}.json",
                        std::time::SystemTime::now()
                            .duration_since(std::time::UNIX_EPOCH)
                            .unwrap()
                            .as_secs()
                    );
                    if let Some(parent) = profiles_path.parent() {
                        let backup_path = parent.join(backup_name);
                        if let Err(e) = std::fs::rename(&profiles_path, &backup_path) {
                            tracing::warn!("Failed to backup profiles.json: {}", e);
                        } else {
                            tracing::info!("Custom profiles.json backed up to {:?}", backup_path);
                            tracing::info!(
                                "Please review your custom profiles after migration to v6"
                            );
                        }
                    } else {
                        tracing::warn!(
                            "Failed to resolve profiles backup directory for {:?}",
                            profiles_path
                        );
                    }
                }
            }
            Err(e) => {
                tracing::warn!("Failed to resolve profiles path for backup: {}", e);
            }
        }

        // Validate and convert ProfileVariantLabel
        let old_coding_agent = old_config.profile.profile.to_uppercase();
        let base_coding_agent =
            BaseCodingAgent::from_str(&old_coding_agent).unwrap_or(BaseCodingAgent::ClaudeCode);
        let executor_profile = ExecutorProfileId::new(base_coding_agent);

        Ok(Self {
            config_version: "v6".to_string(),
            theme: old_config.theme,
            executor_profile,
            disclaimer_acknowledged: old_config.disclaimer_acknowledged,
            onboarding_acknowledged: old_config.onboarding_acknowledged,
            github_login_acknowledged: old_config.github_login_acknowledged,
            telemetry_acknowledged: old_config.telemetry_acknowledged,
            notifications: old_config.notifications,
            editor: old_config.editor,
            github: old_config.github,
            analytics_enabled: old_config.analytics_enabled,
            workspace_dir: old_config.workspace_dir,
            last_app_version: old_config.last_app_version,
            show_release_notes: old_config.show_release_notes,
            language: UiLanguage::default(),
        })
    }
}

impl From<String> for Config {
    fn from(raw_config: String) -> Self {
        if let Ok(config) = serde_json::from_str::<Config>(&raw_config)
            && config.config_version == "v6"
        {
            return config;
        }

        match Self::from_previous_version(&raw_config) {
            Ok(config) => {
                tracing::info!("Config upgraded to v6");
                config
            }
            Err(e) => {
                tracing::warn!("Config migration failed: {}, using default", e);
                Self::default()
            }
        }
    }
}

impl Default for Config {
    fn default() -> Self {
        Self {
            config_version: "v6".to_string(),
            theme: ThemeMode::System,
            executor_profile: ExecutorProfileId::new(BaseCodingAgent::ClaudeCode),
            disclaimer_acknowledged: false,
            onboarding_acknowledged: false,
            github_login_acknowledged: false,
            telemetry_acknowledged: false,
            notifications: NotificationConfig::default(),
            editor: EditorConfig::default(),
            github: GitHubConfig::default(),
            analytics_enabled: None,
            workspace_dir: None,
            last_app_version: None,
            show_release_notes: false,
            language: UiLanguage::default(),
        }
    }
}
