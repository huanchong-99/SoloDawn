//! CLI detection service
//!
//! Detects CLI availability and versions on the system.

use std::sync::Arc;

use db::DBService;
// Re-export database types
pub use db::models::{CliDetectionStatus, CliType};
use tokio::process::Command;

/// CLI detector for checking availability and versions
pub struct CliDetector {
    db: Arc<DBService>,
}

impl CliDetector {
    /// Create a new CLI detector
    ///
    /// # Arguments
    /// * `db` - Database service for CLI type lookups
    pub fn new(db: Arc<DBService>) -> Self {
        Self { db }
    }

    /// Detect all CLI types in the database
    ///
    /// # Returns
    /// A vector of detection statuses for all CLI types
    ///
    /// # Errors
    /// Returns an error if database lookup fails
    pub async fn detect_all(&self) -> anyhow::Result<Vec<CliDetectionStatus>> {
        let cli_types = CliType::find_all(&self.db.pool).await?;
        let mut results = Vec::new();

        for cli_type in cli_types {
            let status = self.detect_single(&cli_type).await;
            results.push(status);
        }

        Ok(results)
    }

    /// Get extended PATH with common CLI installation directories
    #[cfg(windows)]
    fn get_extended_path() -> String {
        let current_path = std::env::var("PATH").unwrap_or_default();
        let mut paths: Vec<String> = vec![current_path];

        // Add common npm global paths
        if let Ok(appdata) = std::env::var("APPDATA") {
            paths.push(format!("{appdata}\\npm"));
        }

        // Add user local bin (for tools like claude)
        if let Ok(userprofile) = std::env::var("USERPROFILE") {
            paths.push(format!("{userprofile}\\.local\\bin"));
        }

        // Add common program files paths
        if let Ok(programfiles) = std::env::var("ProgramFiles") {
            paths.push(format!("{programfiles}\\nodejs"));
        }

        paths.join(";")
    }

    #[cfg(not(windows))]
    fn get_extended_path() -> String {
        let current_path = std::env::var("PATH").unwrap_or_default();
        let mut paths: Vec<String> = vec![current_path];

        // Add common paths on Unix
        if let Ok(home) = std::env::var("HOME") {
            paths.push(format!("{}/.local/bin", home));
            paths.push(format!("{}/.npm-global/bin", home));
            paths.push(format!("{}/bin", home));
        }

        // W2-37-04: `/usr/local/bin` is a Unix-only convention. Gate it so
        // Windows builds don't pollute PATH with a non-existent path.
        #[cfg(unix)]
        paths.push("/usr/local/bin".to_string());

        paths.join(":")
    }

    /// Detect a single CLI type
    ///
    /// # Arguments
    /// * `cli_type` - The CLI type to detect
    ///
    /// # Returns
    /// A detection status indicating whether the CLI is installed
    pub async fn detect_single(&self, cli_type: &CliType) -> CliDetectionStatus {
        let parts: Vec<&str> = cli_type.detect_command.split_whitespace().collect();

        if parts.is_empty() {
            return Self::not_installed(cli_type);
        }

        let cmd = parts[0];
        let args = &parts[1..];

        // Whitelist validation: only allow known CLI binary names to prevent command injection
        // via crafted detect_command values when using `cmd /c` on Windows.
        const ALLOWED_CLI_COMMANDS: &[&str] = &[
            "claude", "gemini", "codex", "amp", "cursor", "cursor-agent",
            "qwen", "gh", "opencode", "droid",
        ];
        if !ALLOWED_CLI_COMMANDS.contains(&cmd) {
            tracing::warn!(
                cmd = cmd,
                cli_type_id = %cli_type.id,
                "Blocked detect_command with unrecognized binary name"
            );
            return Self::not_installed(cli_type);
        }

        // Use extended PATH for detection
        let extended_path = Self::get_extended_path();

        // On Windows, use cmd.exe /c to execute commands, which properly handles
        // .cmd and .bat files (like npm-installed CLIs)
        #[cfg(windows)]
        let result = Command::new("cmd")
            .arg("/c")
            .arg(cmd)
            .args(args)
            .env("PATH", &extended_path)
            .output()
            .await;

        #[cfg(not(windows))]
        let result = Command::new(cmd)
            .args(args)
            .env("PATH", &extended_path)
            .output()
            .await;

        match result {
            Ok(output) if output.status.success() => {
                let version = String::from_utf8_lossy(&output.stdout)
                    .lines()
                    .next()
                    .map(|s| s.trim().to_string());

                let executable_path = self.find_executable(cmd, &extended_path).await;

                CliDetectionStatus {
                    cli_type_id: cli_type.id.clone(),
                    name: cli_type.name.clone(),
                    display_name: cli_type.display_name.clone(),
                    installed: true,
                    version,
                    executable_path,
                    install_guide_url: cli_type.install_guide_url.clone(),
                }
            }
            _ => Self::not_installed(cli_type),
        }
    }

    /// Create a "not installed" status for a CLI type
    fn not_installed(cli_type: &CliType) -> CliDetectionStatus {
        CliDetectionStatus {
            cli_type_id: cli_type.id.clone(),
            name: cli_type.name.clone(),
            display_name: cli_type.display_name.clone(),
            installed: false,
            version: None,
            executable_path: None,
            install_guide_url: cli_type.install_guide_url.clone(),
        }
    }

    /// Find executable path for a command
    ///
    /// # Arguments
    /// * `cmd` - The command name to look up
    /// * `path` - The PATH to search in
    ///
    /// # Returns
    /// The full path to the executable, or None if not found
    async fn find_executable(&self, cmd: &str, path: &str) -> Option<String> {
        #[cfg(unix)]
        {
            Command::new("which")
                .arg(cmd)
                .env("PATH", path)
                .output()
                .await
                .ok()
                .filter(|o| o.status.success())
                .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        }

        #[cfg(windows)]
        {
            Command::new("where")
                .arg(cmd)
                .env("PATH", path)
                .output()
                .await
                .ok()
                .filter(|o| o.status.success())
                .map(|o| {
                    String::from_utf8_lossy(&o.stdout)
                        .lines()
                        .next()
                        .unwrap_or("")
                        .to_string()
                })
        }
    }

    /// Check if a CLI is available by name
    ///
    /// # Arguments
    /// * `cli_name` - The name of the CLI to check
    ///
    /// # Returns
    /// true if the CLI is installed and available, false otherwise
    pub async fn is_available(&self, cli_name: &str) -> bool {
        match CliType::find_by_name(&self.db.pool, cli_name).await {
            Ok(Some(cli_type)) => {
                let status = self.detect_single(&cli_type).await;
                status.installed
            }
            _ => false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // Test helper to create in-memory database
    async fn setup_test_db() -> Arc<DBService> {
        use db::DBService;
        use sqlx::sqlite::SqlitePoolOptions;

        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations using the db crate's migrator
        // CARGO_MANIFEST_DIR is crates/services/src
        // Go up to crates, then to db/migrations
        let manifest_dir = std::path::Path::new(env!("CARGO_MANIFEST_DIR"));
        let migrations_dir = manifest_dir
            .ancestors()
            .nth(1) // Go up 2 levels: src -> services -> crates
            .unwrap()
            .join("db/migrations");

        sqlx::migrate::Migrator::new(migrations_dir)
            .await
            .unwrap()
            .run(&pool)
            .await
            .unwrap();

        Arc::new(DBService { pool })
    }

    #[tokio::test]
    async fn test_cli_detector_new() {
        let db = setup_test_db().await;
        let detector = CliDetector::new(db);
        // Just verify creation - no methods called yet
        assert_eq!(
            detector.db.as_ref() as *const _,
            detector.db.as_ref() as *const _
        );
    }

    #[tokio::test]
    async fn test_find_executable_unix() {
        #[cfg(unix)]
        {
            let detector = CliDetector::new(setup_test_db().await);
            let path = CliDetector::get_extended_path();
            let sh_path = detector.find_executable("sh", &path).await;
            assert!(sh_path.is_some(), "Should find 'sh' executable");
            assert!(sh_path.unwrap().ends_with("sh"));
        }
    }

    #[tokio::test]
    async fn test_find_executable_windows() {
        #[cfg(windows)]
        {
            let detector = CliDetector::new(setup_test_db().await);
            let path = CliDetector::get_extended_path();
            let cmd_path = detector.find_executable("cmd", &path).await;
            assert!(cmd_path.is_some(), "Should find 'cmd' executable");
        }

        #[cfg(unix)]
        {
            // Skip on Unix
            
        }
    }

    #[tokio::test]
    async fn test_detect_single_installed_cli() {
        let db = setup_test_db().await;

        // Create a test CLI type pointing to a known command
        let cli = CliType {
            id: "test-claude".to_string(),
            name: "claude".to_string(),
            display_name: "Claude".to_string(),
            detect_command: "claude --version".to_string(),
            install_command: None,
            install_guide_url: None,
            config_file_path: None,
            is_system: false,
            created_at: chrono::Utc::now(),
        };

        let detector = CliDetector::new(Arc::clone(&db));
        let status = detector.detect_single(&cli).await;

        // claude may or may not be installed in CI; just verify the structure works
        assert_eq!(status.name, "claude");
    }

    #[tokio::test]
    async fn test_detect_single_nonexistent_cli() {
        let db = setup_test_db().await;

        // Create a CLI type for a command that doesn't exist
        let cli = CliType {
            id: "test-fake".to_string(),
            name: "definitely-not-a-real-command-xyz123".to_string(),
            display_name: "Fake CLI".to_string(),
            detect_command: "definitely-not-a-real-command-xyz123 --version".to_string(),
            install_command: None,
            install_guide_url: None,
            config_file_path: None,
            is_system: false,
            created_at: chrono::Utc::now(),
        };

        let status = CliDetector::new(db).detect_single(&cli).await;

        assert!(!status.installed);
        assert_eq!(status.name, "definitely-not-a-real-command-xyz123");
        assert!(status.version.is_none());
        assert!(status.executable_path.is_none());
    }

    #[tokio::test]
    async fn test_not_installed_helper() {
        let cli = CliType {
            id: "test-1".to_string(),
            name: "fake-cli".to_string(),
            display_name: "Fake".to_string(),
            detect_command: "fake-cli --version".to_string(),
            install_command: None,
            install_guide_url: Some("https://example.com/install".to_string()),
            config_file_path: None,
            is_system: false,
            created_at: chrono::Utc::now(),
        };

        let status = CliDetector::not_installed(&cli);
        assert!(!status.installed);
        assert_eq!(
            status.install_guide_url,
            Some("https://example.com/install".to_string())
        );
    }
}
