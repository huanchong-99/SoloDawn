use std::path::PathBuf;

use thiserror::Error;

pub mod editor;
mod versions;

pub use editor::EditorOpenError;

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error(transparent)]
    Io(#[from] std::io::Error),
    #[error(transparent)]
    Json(#[from] serde_json::Error),
    #[error("Validation error: {0}")]
    ValidationError(String),
}

pub type Config = versions::v9::Config;
pub type NotificationConfig = versions::v9::NotificationConfig;
pub type EditorConfig = versions::v9::EditorConfig;
pub type ThemeMode = versions::v9::ThemeMode;
pub type SoundFile = versions::v9::SoundFile;
pub type EditorType = versions::v9::EditorType;
pub type GitHubConfig = versions::v9::GitHubConfig;
pub type UiLanguage = versions::v9::UiLanguage;
pub type ShowcaseState = versions::v9::ShowcaseState;
pub type WorkflowModelLibraryItem = versions::v9::WorkflowModelLibraryItem;

/// Will always return config, trying old schemas or eventually returning default
pub fn load_config_from_file(config_path: &PathBuf) -> Config {
    if let Ok(raw_config) = std::fs::read_to_string(config_path) {
        Config::from(raw_config)
    } else {
        tracing::info!("No config file found, creating one");
        Config::default()
    }
}

/// Saves the config to the given path
pub fn save_config_to_file(config: &Config, config_path: &PathBuf) -> Result<(), ConfigError> {
    let raw_config = serde_json::to_string_pretty(config)?;
    std::fs::write(config_path, raw_config)?;
    Ok(())
}
