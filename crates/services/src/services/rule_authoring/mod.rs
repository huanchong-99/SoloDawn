//! Rule-authoring LLM invoker (dual-source, user-selectable).
//!
//! The quality-gate rule-authoring pipeline (PRD §7.2 / §8.6,
//! `docs/quality/PRD-ai-editable-quality-rules.md`) lets a project owner draft a
//! declarative rule from natural language using EITHER:
//!
//! - their own **metered API-key model** (billed to their key, exempt from the
//!   subscription credit pool), or
//! - their official **Claude subscription** (driven through the genuine `claude`
//!   binary via the EXISTING no-`-p` interactive native-OAuth transport, OFF the
//!   metered Agent-SDK pool).
//!
//! Both choices are surfaced as the user's globally-configured models, selected
//! in the existing model-picker UI; the server resolves the selection to a
//! backend here.
//!
//! ## Reuse-first (PRD Reuse Map / D9)
//!
//! This module builds NO new client and NO new transport. It is a thin seam over
//! two already-built, trait-compatible runners that both implement
//! [`LLMClient`](crate::services::orchestrator::LLMClient):
//!
//! - metered: [`create_llm_client`](crate::services::orchestrator::create_llm_client)
//!   from a resolved [`OrchestratorConfig`];
//! - subscription: [`create_interactive_claude_client`](crate::services::orchestrator::create_interactive_claude_client),
//!   which constructs the existing `InteractiveClaudeClient`.
//!
//! The dispatch is driven entirely by
//! [`InteractiveAuthMode::resolve`](crate::services::cc_switch::InteractiveAuthMode::resolve)
//! on the user-selected model's `(api_key, base_url)` pair — the same
//! discriminator the `-p` path uses — so authoring never changes WHICH credential
//! a user gets, only that it is reused for a single authoring turn.

pub mod adversary;
pub mod empirical_test;
pub mod generate;
pub mod json;
pub mod pipeline;
pub mod prompts;
pub mod reverse_engineer;
pub mod types;

#[cfg(test)]
mod tests_pipeline;

pub use pipeline::{
    AuthoringAgents, MAX_AUTHORING_ROUNDS, author_rule, persist_run, revalidate_rule_body,
    run_authoring,
};
pub use types::{
    AdversaryFindings, AuthorOutcome, AuthorRunResult, AuthoredCandidate, AuthoringBackend,
    EmpiricalReport, ExampleKind, ExampleResult, GeneratedRule, RoundTripVerdict, RuleBodyEnvelope,
    RuleExample,
};

use std::sync::Arc;

use async_trait::async_trait;
use db::models::ModelConfig;
use once_cell::sync::Lazy;
use sqlx::SqlitePool;
use tokio::sync::Semaphore;

use crate::services::{
    cc_switch::InteractiveAuthMode,
    orchestrator::{
        LLMClient, OrchestratorConfig, create_interactive_claude_client, create_llm_client,
        types::LLMMessage,
    },
};

/// Process-global serialization gate for the **subscription** authoring backend.
///
/// The subscription backend drives the genuine `claude` binary through the
/// interactive transport, which provisions an isolated `CLAUDE_HOME`, copies the
/// user's OAuth credentials into it, and tails an on-disk transcript per turn.
/// Best practice is to run those turns serially (one permit): concurrent
/// single-turn `claude` spawns contend on the user's single subscription session
/// and on shared scratch state, and offer no real throughput win for one-shot
/// authoring calls.
///
/// The **metered** HTTP backend is deliberately EXEMPT — it is ordinary
/// rate-limited HTTP against the user's own key and may run concurrently.
///
/// One permit, acquired for the full duration of each subscription turn and
/// released when the returned guard drops.
pub static AUTHORING_SUBSCRIPTION_GATE: Lazy<Semaphore> = Lazy::new(|| Semaphore::new(1));

/// Default Anthropic base URL used when a user selected an **official**
/// (api-key, no `base_url`) Anthropic model for the metered backend.
///
/// [`OrchestratorConfig::from_workflow`] requires a non-`None` `base_url`, and
/// [`OrchestratorConfig::validate`] requires it non-empty; an official Anthropic
/// model legitimately stores no `base_url`, so we supply the canonical endpoint.
/// `resolve_endpoint` strips a trailing `/v1`, so the bare host is correct.
const DEFAULT_ANTHROPIC_BASE_URL: &str = "https://api.anthropic.com";

/// Default OpenAI base URL used when a user selected an **official**
/// (api-key, no `base_url`) OpenAI model for the metered backend.
const DEFAULT_OPENAI_BASE_URL: &str = "https://api.openai.com/v1";

/// Backend-agnostic authoring LLM.
///
/// One method, `complete`, takes a system prompt and a user prompt and returns
/// the assistant's text. Both the metered and the subscription backend implement
/// it by WRAPPING an existing [`LLMClient`] (they do not build a new client), so
/// every stage of the authoring pipeline can stay backend-agnostic behind a
/// single `&dyn AuthoringLlm`.
///
/// Object-safe via [`async_trait`] (`Box<dyn AuthoringLlm>`).
#[async_trait]
pub trait AuthoringLlm: Send + Sync {
    /// Run one authoring completion. `system` is the role/instruction prompt,
    /// `user` is the task payload (NL request, current rules, candidate, etc.).
    /// Returns the assistant's reply text.
    async fn complete(&self, system: &str, user: &str) -> anyhow::Result<String>;
}

/// Flatten a `(system, user)` authoring prompt into the orchestrator's
/// [`LLMMessage`] vector — the same `[system, user]` shape every existing stage
/// builds (e.g. `audit_plan::generate_audit_plan`). An empty `system` is omitted
/// so the subscription transport (which collapses system+user into one prompt)
/// is not handed a stray blank block.
fn build_messages(system: &str, user: &str) -> Vec<LLMMessage> {
    let mut messages = Vec::with_capacity(2);
    if !system.is_empty() {
        messages.push(LLMMessage {
            role: "system".to_string(),
            content: system.to_string(),
        });
    }
    messages.push(LLMMessage {
        role: "user".to_string(),
        content: user.to_string(),
    });
    messages
}

/// Metered authoring backend: wraps an existing [`LLMClient`] built from the
/// user's own API-key model (`create_llm_client`). Billed to the user's key and
/// exempt from the subscription credit pool / serialization gate.
pub struct MeteredAuthoringLlm {
    client: Arc<dyn LLMClient>,
}

impl MeteredAuthoringLlm {
    /// Wrap an already-constructed [`LLMClient`] (e.g. from `create_llm_client`,
    /// or `MockLLMClient` in tests) as the metered authoring backend.
    pub fn new(client: Arc<dyn LLMClient>) -> Self {
        Self { client }
    }
}

#[async_trait]
impl AuthoringLlm for MeteredAuthoringLlm {
    async fn complete(&self, system: &str, user: &str) -> anyhow::Result<String> {
        let response = self.client.chat(build_messages(system, user)).await?;
        Ok(response.content)
    }
}

/// Subscription authoring backend: wraps the existing interactive native-OAuth
/// [`LLMClient`] (the genuine `claude` binary, off the metered pool). Each turn
/// acquires the process-global [`AUTHORING_SUBSCRIPTION_GATE`] permit so
/// subscription turns run serially.
pub struct SubscriptionAuthoringLlm {
    client: Arc<dyn LLMClient>,
}

impl SubscriptionAuthoringLlm {
    /// Wrap an already-constructed interactive [`LLMClient`] (e.g. from
    /// `create_interactive_claude_client`) as the subscription authoring backend.
    pub fn new(client: Arc<dyn LLMClient>) -> Self {
        Self { client }
    }
}

#[async_trait]
impl AuthoringLlm for SubscriptionAuthoringLlm {
    async fn complete(&self, system: &str, user: &str) -> anyhow::Result<String> {
        // Serialize subscription turns: hold the single permit for the whole
        // turn. `acquire()` only errors if the semaphore is closed, which never
        // happens for this process-global static.
        let _permit = AUTHORING_SUBSCRIPTION_GATE
            .acquire()
            .await
            .map_err(|e| anyhow::anyhow!("authoring subscription gate closed: {e}"))?;
        let response = self.client.chat(build_messages(system, user)).await?;
        Ok(response.content)
    }
}

/// Resolve the user-selected, globally-configured model into an authoring
/// backend.
///
/// Dispatch (PRD §8.6) — by [`InteractiveAuthMode::resolve`] on the resolved
/// model's `(api_key, base_url)`:
///
/// - **native-OAuth** (no api key) → subscription backend over the existing
///   interactive transport, OFF the metered pool;
/// - **official key / relay** (has api key) → metered backend over
///   `create_llm_client`, billed to the user's key.
///
/// Unsupported providers are rejected with a clear error: the metered path runs
/// `OrchestratorConfig::validate`, whose `api_type` whitelist excludes `google`
/// (offered by the model picker but not authorable via this backend).
///
/// `model_config_id` is the user's explicit selection from the picker; passing it
/// avoids the precedence ambiguity that an implicit `None` fallthrough would
/// introduce (PRD §7.2 billing guarantee).
pub async fn build_authoring_client(
    pool: &SqlitePool,
    project_id: &str,
    model_config_id: &str,
    cli_type_id: &str,
) -> anyhow::Result<Box<dyn AuthoringLlm>> {
    let (client, _backend) =
        build_authoring_client_with_backend(pool, project_id, model_config_id, cli_type_id).await?;
    Ok(client)
}

/// Same resolution as [`build_authoring_client`], but ALSO returns the
/// [`AuthoringBackend`] the dispatch selected.
///
/// The API layer needs the backend twice: to pass it as the
/// `backend` argument of [`run_authoring`]/[`author_rule`] (so the persisted run
/// records which transport ran) and to echo it back to the caller in the
/// `AuthorRuleResult.engine.backend` field (PRD §10). `build_authoring_client`
/// stays a thin wrapper that drops the backend so its existing callers and tests
/// are unaffected.
pub async fn build_authoring_client_with_backend(
    pool: &SqlitePool,
    _project_id: &str,
    model_config_id: &str,
    cli_type_id: &str,
) -> anyhow::Result<(Box<dyn AuthoringLlm>, AuthoringBackend)> {
    let model = ModelConfig::resolve_preferred_or_default(pool, Some(model_config_id), cli_type_id)
        .await?
        .ok_or_else(|| {
            anyhow::anyhow!(
                "no usable model config found for selection '{model_config_id}' (cli_type '{cli_type_id}')"
            )
        })?;

    // Decrypt the key via the model itself (never read encrypted_api_key
    // directly — decryption is automatic, AES-256-GCM).
    let api_key = model.get_api_key()?;
    let base_url = model.base_url.as_deref();

    match InteractiveAuthMode::resolve(api_key.as_deref(), base_url) {
        InteractiveAuthMode::NativeOauth => {
            // Subscription / native-OAuth: reuse the existing interactive
            // transport. `create_interactive_claude_client` returns `None` when
            // no local subscription credentials exist.
            let model_name = resolve_model_name(&model);
            let client = create_interactive_claude_client(&model_name).ok_or_else(|| {
                anyhow::anyhow!(
                    "selected source has no usable subscription credentials \
                     (no ~/.claude/.credentials.json); pick a model with an API key \
                     or log in via `claude login`"
                )
            })?;
            Ok((
                Box::new(SubscriptionAuthoringLlm::new(Arc::from(client))),
                AuthoringBackend::Subscription,
            ))
        }
        InteractiveAuthMode::OfficialKey | InteractiveAuthMode::Relay => {
            // Metered, pool-exempt HTTP via the existing LLMClient.
            let key = api_key.ok_or_else(|| {
                // Unreachable given the match arm (api_key is Some here), but
                // keeps the error explicit rather than unwrapping.
                anyhow::anyhow!("selected metered source is missing its API key")
            })?;
            let api_type = require_supported_api_type(model.api_type.as_deref())?;
            let resolved_base_url = resolve_metered_base_url(api_type, base_url);
            let model_name = resolve_model_name(&model);

            let config = OrchestratorConfig::from_workflow(
                Some(api_type),
                Some(&resolved_base_url),
                Some(&key),
                Some(&model_name),
            )
            .ok_or_else(|| {
                anyhow::anyhow!("failed to build authoring LLM config for the selected source")
            })?;

            // `validate()` (inside create_llm_client) rejects api_type=google and
            // any other non-whitelisted provider with a clear message.
            let client = create_llm_client(&config)?;
            Ok((
                Box::new(MeteredAuthoringLlm::new(Arc::from(client))),
                AuthoringBackend::Metered,
            ))
        }
    }
}

/// The `api_model_id` if set, else the config `name` — mirrors
/// `CCSwitchService::resolve_model_name`.
fn resolve_model_name(model: &ModelConfig) -> String {
    model
        .api_model_id
        .clone()
        .unwrap_or_else(|| model.name.clone())
}

/// Validate the selected model's `api_type` against the metered backend's
/// whitelist BEFORE building config, so unsupported providers (notably `google`)
/// are rejected with a clear, source-specific error instead of a generic
/// validation failure deeper in.
fn require_supported_api_type(api_type: Option<&str>) -> anyhow::Result<&str> {
    let api_type = api_type.ok_or_else(|| {
        anyhow::anyhow!("selected metered source has no api_type; cannot author with it")
    })?;
    const SUPPORTED: [&str; 4] = [
        "openai",
        "anthropic",
        "openai-compatible",
        "anthropic-compatible",
    ];
    if SUPPORTED.contains(&api_type) {
        Ok(api_type)
    } else {
        Err(anyhow::anyhow!(
            "provider '{api_type}' is not supported for rule authoring \
             (supported: {SUPPORTED:?}); pick an OpenAI- or Anthropic-compatible source"
        ))
    }
}

/// Resolve the metered base URL: an official model legitimately stores no
/// `base_url`, so default by protocol family (`OrchestratorConfig::from_workflow`
/// requires `Some(base_url)` and `validate()` requires it non-empty).
fn resolve_metered_base_url(api_type: &str, base_url: Option<&str>) -> String {
    match base_url {
        Some(url) if !url.trim().is_empty() => url.to_string(),
        _ => match api_type {
            "anthropic" | "anthropic-compatible" => DEFAULT_ANTHROPIC_BASE_URL.to_string(),
            _ => DEFAULT_OPENAI_BASE_URL.to_string(),
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::services::orchestrator::MockLLMClient;

    #[test]
    fn build_messages_includes_system_and_user() {
        let messages = build_messages("sys", "usr");
        assert_eq!(messages.len(), 2);
        assert_eq!(messages[0].role, "system");
        assert_eq!(messages[0].content, "sys");
        assert_eq!(messages[1].role, "user");
        assert_eq!(messages[1].content, "usr");
    }

    #[test]
    fn build_messages_omits_empty_system() {
        let messages = build_messages("", "usr");
        assert_eq!(messages.len(), 1);
        assert_eq!(messages[0].role, "user");
    }

    #[test]
    fn require_supported_api_type_accepts_whitelist() {
        for ok in [
            "openai",
            "anthropic",
            "openai-compatible",
            "anthropic-compatible",
        ] {
            assert!(require_supported_api_type(Some(ok)).is_ok(), "{ok}");
        }
    }

    #[test]
    fn require_supported_api_type_rejects_google() {
        let err = require_supported_api_type(Some("google")).unwrap_err();
        assert!(err.to_string().contains("google"));
        assert!(require_supported_api_type(None).is_err());
    }

    #[test]
    fn resolve_metered_base_url_defaults_by_protocol() {
        assert_eq!(
            resolve_metered_base_url("anthropic", None),
            DEFAULT_ANTHROPIC_BASE_URL
        );
        assert_eq!(
            resolve_metered_base_url("anthropic-compatible", Some("  ")),
            DEFAULT_ANTHROPIC_BASE_URL
        );
        assert_eq!(
            resolve_metered_base_url("openai", None),
            DEFAULT_OPENAI_BASE_URL
        );
        assert_eq!(
            resolve_metered_base_url("anthropic", Some("https://relay.example/v1")),
            "https://relay.example/v1"
        );
    }

    /// `MockLLMClient` is usable as an `AuthoringLlm` by wrapping it in the
    /// metered backend (the same wiring real code uses), so every authoring
    /// stage can be unit-tested deterministically without network or a `claude`
    /// binary.
    #[tokio::test]
    async fn mock_llm_client_usable_as_metered_authoring_backend() {
        let mock = Arc::new(MockLLMClient::with_response("drafted rule")) as Arc<dyn LLMClient>;
        let authoring: Box<dyn AuthoringLlm> = Box::new(MeteredAuthoringLlm::new(mock));
        let out = authoring
            .complete("you author rules", "prohibit dbg!")
            .await
            .expect("mock completion");
        assert_eq!(out, "drafted rule");
    }

    #[tokio::test]
    async fn metered_backend_propagates_client_error() {
        let mock = Arc::new(MockLLMClient::that_fails()) as Arc<dyn LLMClient>;
        let authoring = MeteredAuthoringLlm::new(mock);
        let err = authoring.complete("s", "u").await.unwrap_err();
        assert!(err.to_string().contains("Mock LLM client error"));
    }

    /// The subscription backend serializes turns through the global gate; a
    /// successful mock turn still returns its content and releases the permit.
    #[tokio::test]
    async fn subscription_backend_uses_gate_and_returns_content() {
        let mock =
            Arc::new(MockLLMClient::with_response("subscription reply")) as Arc<dyn LLMClient>;
        let authoring = SubscriptionAuthoringLlm::new(mock);
        let out = authoring.complete("sys", "usr").await.expect("turn");
        assert_eq!(out, "subscription reply");
        // Permit was released on drop: the gate is available again.
        assert_eq!(AUTHORING_SUBSCRIPTION_GATE.available_permits(), 1);
    }
}
