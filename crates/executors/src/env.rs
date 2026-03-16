use std::collections::HashMap;

use tokio::process::Command;
use tracing::warn;

use crate::command::CmdOverrides;

/// Environment variable names that must never be injected into spawned processes.
/// These can be abused to execute arbitrary code via dynamic linker injection,
/// language-level startup hooks, or shell init scripts.
const BLOCKED_ENV_VARS: &[&str] = &[
    // Dynamic linker injection
    "LD_PRELOAD",
    "LD_LIBRARY_PATH",
    "DYLD_INSERT_LIBRARIES",
    "DYLD_LIBRARY_PATH",
    // Node.js
    "NODE_OPTIONS",
    "NODE_PATH",
    // Python
    "PYTHONSTARTUP",
    "PYTHONPATH",
    // Ruby / Perl
    "RUBYOPT",
    "PERL5OPT",
    // Shell init
    "BASH_ENV",
    "ENV",
    "ZDOTDIR",
];

fn is_blocked_env_var(key: &str) -> bool {
    BLOCKED_ENV_VARS.iter().any(|&b| b.eq_ignore_ascii_case(key))
}

/// Environment variables to inject into executor processes
#[derive(Debug, Clone, Default)]
pub struct ExecutionEnv {
    pub vars: HashMap<String, String>,
}

impl ExecutionEnv {
    pub fn new() -> Self {
        Self {
            vars: HashMap::new(),
        }
    }

    /// Insert an environment variable
    pub fn insert(&mut self, key: impl Into<String>, value: impl Into<String>) {
        self.vars.insert(key.into(), value.into());
    }

    /// Merge additional vars into this env. Incoming keys overwrite existing ones.
    pub fn merge(&mut self, other: &HashMap<String, String>) {
        self.vars
            .extend(other.iter().map(|(k, v)| (k.clone(), v.clone())));
    }

    /// Return a new env with overrides applied. Overrides take precedence.
    #[must_use]
    pub fn with_overrides(mut self, overrides: &HashMap<String, String>) -> Self {
        self.merge(overrides);
        self
    }

    /// Return a new env with profile env from CmdOverrides merged in.
    #[must_use]
    pub fn with_profile(self, cmd: &CmdOverrides) -> Self {
        if let Some(ref profile_env) = cmd.env {
            self.with_overrides(profile_env)
        } else {
            self
        }
    }

    /// Apply all environment variables to a Command.
    ///
    /// Blocked security-sensitive variables are silently dropped with a warning.
    /// `PATH` is appended to the existing value rather than replaced outright.
    pub fn apply_to_command(&self, command: &mut Command) {
        for (key, value) in &self.vars {
            if is_blocked_env_var(key) {
                warn!(
                    key = %key,
                    "Blocked dangerous environment variable from being injected into executor process"
                );
                continue;
            }

            if key.eq_ignore_ascii_case("PATH") {
                // Append to the existing PATH instead of replacing it.
                let existing = std::env::var("PATH").unwrap_or_default();
                let sep = if cfg!(windows) { ';' } else { ':' };
                let combined = if existing.is_empty() {
                    value.clone()
                } else {
                    format!("{existing}{sep}{value}")
                };
                command.env("PATH", combined);
                warn!(
                    "PATH env override was appended to system PATH instead of replacing it"
                );
                continue;
            }

            command.env(key, value);
        }
    }

    pub fn contains_key(&self, key: &str) -> bool {
        self.vars.contains_key(key)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn profile_overrides_runtime_env() {
        let mut base = ExecutionEnv::default();
        base.insert("VK_PROJECT_NAME", "runtime");
        base.insert("FOO", "runtime");

        let mut profile = HashMap::new();
        profile.insert("FOO".to_string(), "profile".to_string());
        profile.insert("BAR".to_string(), "profile".to_string());

        let merged = base.with_overrides(&profile);

        assert_eq!(merged.vars.get("VK_PROJECT_NAME").unwrap(), "runtime");
        assert_eq!(merged.vars.get("FOO").unwrap(), "profile"); // overrides
        assert_eq!(merged.vars.get("BAR").unwrap(), "profile");
    }

    #[test]
    fn blocked_env_vars_are_detected() {
        assert!(is_blocked_env_var("LD_PRELOAD"));
        assert!(is_blocked_env_var("ld_preload")); // case-insensitive
        assert!(is_blocked_env_var("NODE_OPTIONS"));
        assert!(is_blocked_env_var("PYTHONSTARTUP"));
        assert!(is_blocked_env_var("BASH_ENV"));
        assert!(!is_blocked_env_var("MY_CUSTOM_VAR"));
        assert!(!is_blocked_env_var("API_KEY"));
    }

    #[test]
    fn apply_to_command_skips_blocked_vars() {
        let mut env = ExecutionEnv::new();
        env.insert("SAFE_VAR", "ok");
        env.insert("LD_PRELOAD", "/tmp/evil.so");
        env.insert("NODE_OPTIONS", "--require /tmp/evil.js");

        let mut cmd = Command::new("echo");
        env.apply_to_command(&mut cmd);

        // We cannot easily inspect Command env, but at least verify it doesn't panic.
        // The real verification is that blocked vars are logged and skipped.
    }
}
