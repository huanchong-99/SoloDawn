//! Claude model resolution for native-subscription (OAuth) runs.
//!
//! Native subscription users have no API-key model configs, so every LLM
//! consumer (orchestrator brain, prompt handler, planning drafts, rule
//! authoring, terminal launches) needs a Claude model id it can hand to the
//! `claude` CLI. The single source of truth is the official `model_config`
//! rows seeded for `cli-claude-code` — the user switches models by picking a
//! different official row (or a per-workflow override); the constant below is
//! only the last-resort fallback when the database is unreadable.

use db::models::ModelConfig;
use sqlx::SqlitePool;

/// CLI type id of the Claude Code rows in `model_config`.
pub const CLAUDE_CODE_CLI_TYPE_ID: &str = "cli-claude-code";

/// Last-resort Claude model for subscription (native OAuth) runs when neither
/// a user choice nor the DB default is resolvable. Keep in sync with the
/// seeded default row (`model-claude-sonnet`, refreshed by migration
/// `20260703110000_refresh_official_claude_models.sql`).
pub const DEFAULT_NATIVE_CLAUDE_MODEL: &str = "claude-sonnet-5";

/// Sentinel model id historically sent by the frontend's native-subscription
/// entry. It means "use the account default" and must never reach the CLI as
/// a literal `--model` / `ANTHROPIC_MODEL` value.
pub const NATIVE_SUBSCRIPTION_SENTINEL: &str = "subscription-default";

/// Whether `model` is a concrete Claude model id the CLI can be pinned to.
pub fn is_concrete_claude_model(model: &str) -> bool {
    model.starts_with("claude-") && model != NATIVE_SUBSCRIPTION_SENTINEL
}

/// Resolve the Claude model for a native-subscription LLM call:
/// preferred (when already a concrete `claude-*` id) → the DB default
/// official Claude Code model → [`DEFAULT_NATIVE_CLAUDE_MODEL`].
pub async fn resolve_native_claude_model(pool: &SqlitePool, preferred: Option<&str>) -> String {
    if let Some(preferred) = preferred {
        let preferred = preferred.trim();
        if is_concrete_claude_model(preferred) {
            return preferred.to_string();
        }
    }
    default_native_claude_model(pool).await
}

/// The DB default official Claude Code model, falling back to
/// [`DEFAULT_NATIVE_CLAUDE_MODEL`] when the row is missing or non-Claude.
pub async fn default_native_claude_model(pool: &SqlitePool) -> String {
    if let Ok(Some(model_config)) =
        ModelConfig::find_default_for_cli(pool, CLAUDE_CODE_CLI_TYPE_ID).await
    {
        if let Some(api_model_id) = model_config.api_model_id {
            if is_concrete_claude_model(&api_model_id) {
                return api_model_id;
            }
        }
    }
    DEFAULT_NATIVE_CLAUDE_MODEL.to_string()
}

/// The official Claude Code models a native-subscription user can switch
/// between (default first). Rows without a concrete `claude-*` id are
/// dropped — the CLI could not launch with them.
pub async fn official_native_claude_models(pool: &SqlitePool) -> Vec<ModelConfig> {
    match ModelConfig::find_official_for_cli(pool, CLAUDE_CODE_CLI_TYPE_ID).await {
        Ok(models) => models
            .into_iter()
            .filter(|model| {
                model
                    .api_model_id
                    .as_deref()
                    .is_some_and(is_concrete_claude_model)
            })
            .collect(),
        Err(e) => {
            tracing::warn!(error = %e, "Failed to load official Claude models");
            Vec::new()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn concrete_claude_model_accepts_current_ids() {
        for id in [
            "claude-sonnet-5",
            "claude-opus-4-8",
            "claude-haiku-4-5",
            "claude-fable-5",
        ] {
            assert!(is_concrete_claude_model(id), "{id} should be concrete");
        }
    }

    #[test]
    fn concrete_claude_model_rejects_sentinel_and_foreign_ids() {
        for id in ["subscription-default", "gpt-4o", "glm-5", "", "sonnet"] {
            assert!(!is_concrete_claude_model(id), "{id} should be rejected");
        }
    }
}
