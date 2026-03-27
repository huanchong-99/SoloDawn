//! LLM client abstractions and implementations.

use std::{future::Future, num::NonZeroU32, sync::Arc, time::Duration};

use async_trait::async_trait;
use governor::{
    Quota, RateLimiter,
    clock::DefaultClock,
    state::{InMemoryState, NotKeyed},
};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use tokio::time::sleep;

use utils::url::normalize_base_url;

use super::{
    config::OrchestratorConfig,
    resilient_llm::{ProviderEvent, ProviderStatusReport},
    types::{LLMMessage, LLMResponse, LLMUsage},
};

/// Defines the LLM client interface used by the orchestrator.
#[async_trait]
pub trait LLMClient: Send + Sync {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse>;

    /// Returns provider status reports. Default returns empty (single-provider clients).
    async fn provider_status(&self) -> Vec<ProviderStatusReport> {
        Vec::new()
    }

    /// Reset a provider's circuit breaker by name. Default returns false.
    async fn reset_provider(&self, _provider_name: &str) -> bool {
        false
    }

    /// Take provider events collected during the last chat call. Default returns empty.
    async fn take_provider_events(&self) -> Vec<ProviderEvent> {
        Vec::new()
    }
}

/// Retries an async operation with exponential backoff.
pub async fn retry_with_backoff<T, E, F, Fut>(max_retries: u32, mut operation: F) -> Result<T, E>
where
    F: FnMut() -> Fut,
    Fut: Future<Output = Result<T, E>>,
{
    let mut last_error = None;

    for attempt in 0..max_retries {
        match operation().await {
            Ok(result) => {
                if attempt > 0 {
                    tracing::info!("Operation succeeded on attempt {}", attempt + 1);
                }
                return Ok(result);
            }
            Err(e) if attempt < max_retries - 1 => {
                tracing::warn!(
                    "Attempt {} failed, retrying in {}ms",
                    attempt + 1,
                    1000 * (attempt + 1)
                );
                last_error = Some(e);
                sleep(Duration::from_millis(1000 * u64::from(attempt + 1))).await;
            }
            Err(e) => {
                tracing::error!("All {} attempts failed", max_retries);
                return Err(e);
            }
        }
    }

    Err(last_error.unwrap())
}

/// Wraps an LLM client with a per-second rate limiter.
pub struct RateLimitedClient<T> {
    inner: T,
    rate_limiter: Arc<RateLimiter<NotKeyed, InMemoryState, DefaultClock>>,
}

impl<T> RateLimitedClient<T> {
    pub fn new(inner: T, requests_per_second: u32) -> anyhow::Result<Self> {
        let rate = NonZeroU32::new(requests_per_second)
            .ok_or_else(|| anyhow::anyhow!("Rate limit must be greater than 0"))?;
        let quota = Quota::per_second(rate);
        let rate_limiter = Arc::new(RateLimiter::direct(quota));

        Ok(Self {
            inner,
            rate_limiter,
        })
    }
}

#[async_trait]
impl<T> LLMClient for RateLimitedClient<T>
where
    T: LLMClient,
{
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        self.rate_limiter
            .check()
            .map_err(|_| anyhow::anyhow!("rate limit exceeded"))?;
        self.inner.chat(messages).await
    }

    /// [G24-001] Transparent forwarding to inner client.
    async fn provider_status(&self) -> Vec<ProviderStatusReport> {
        self.inner.provider_status().await
    }

    /// [G24-001] Transparent forwarding to inner client.
    async fn reset_provider(&self, provider_name: &str) -> bool {
        self.inner.reset_provider(provider_name).await
    }

    /// [G24-001] Transparent forwarding to inner client.
    async fn take_provider_events(&self) -> Vec<ProviderEvent> {
        self.inner.take_provider_events().await
    }
}

/// LLM client for OpenAI-compatible chat endpoints.
pub struct OpenAICompatibleClient {
    client: Client,
    base_url: String,
    api_key: String,
    model: String,
}

/// Mock LLM Client for testing
#[cfg(test)]
pub struct MockLLMClient {
    pub should_fail: bool,
    pub response_content: String,
}

#[cfg(test)]
impl MockLLMClient {
    pub fn new() -> Self {
        Self {
            should_fail: false,
            response_content: "Mock response for testing".to_string(),
        }
    }

    pub fn with_response(content: &str) -> Self {
        Self {
            should_fail: false,
            response_content: content.to_string(),
        }
    }

    pub fn that_fails() -> Self {
        Self {
            should_fail: true,
            response_content: String::new(),
        }
    }
}

#[cfg(test)]
impl Default for MockLLMClient {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
#[async_trait]
impl LLMClient for MockLLMClient {
    async fn chat(&self, _messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        if self.should_fail {
            return Err(anyhow::anyhow!("Mock LLM client error"));
        }

        Ok(LLMResponse {
            content: self.response_content.clone(),
            usage: Some(LLMUsage {
                prompt_tokens: 10,
                completion_tokens: 20,
                total_tokens: 30,
            }),
        })
    }
}

#[derive(Debug, Serialize)]
struct ChatRequest {
    model: String,
    messages: Vec<ChatMessage>,
    temperature: Option<f32>,
    max_tokens: Option<i32>,
}

#[derive(Debug, Serialize, Deserialize)]
struct ChatMessage {
    role: String,
    content: String,
}

#[derive(Debug, Deserialize)]
struct ChatResponse {
    choices: Vec<ChatChoice>,
    usage: Option<UsageInfo>,
}

#[derive(Debug, Deserialize)]
struct ChatChoice {
    message: ChatMessage,
}

#[allow(clippy::struct_field_names)]
#[derive(Debug, Deserialize)]
struct UsageInfo {
    prompt_tokens: i32,
    completion_tokens: i32,
    total_tokens: i32,
}

impl OpenAICompatibleClient {
    pub fn new(config: &OrchestratorConfig) -> Self {
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(30))
            .timeout(Duration::from_secs(config.timeout_secs))
            .build()
            .expect("Failed to create HTTP client");

        let base_url = normalize_base_url(&config.api_type, &config.base_url);

        tracing::info!(
            api_type = %config.api_type,
            input_url = %config.base_url,
            final_base_url = %base_url,
            "OpenAI-compatible LLM client URL normalized"
        );

        Self {
            client,
            base_url,
            api_key: config.api_key.clone(),
            model: config.model.clone(),
        }
    }

    /// Perform a single chat request without retry logic
    async fn chat_once(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        let url = format!("{}/chat/completions", self.base_url);

        let chat_messages: Vec<ChatMessage> = messages
            .into_iter()
            .map(|m| ChatMessage {
                role: m.role,
                content: m.content,
            })
            .collect();

        let request = ChatRequest {
            model: self.model.clone(),
            messages: chat_messages,
            temperature: Some(0.7),
            max_tokens: Some(2048),
        };

        tracing::info!(
            url = %url,
            model = %self.model,
            msg_count = request.messages.len(),
            "OpenAI-compatible LLM request starting"
        );

        let response = self
            .client
            .post(&url)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("Content-Type", "application/json")
            .json(&request)
            .send()
            .await
            .map_err(|e| {
                tracing::error!(url = %url, "OpenAI-compatible LLM request failed: {e}");
                e
            })?;

        tracing::info!(
            status = %response.status(),
            "OpenAI-compatible LLM response received"
        );

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow::anyhow!("LLM API error: {status} - {body}"));
        }

        let chat_response: ChatResponse = response.json().await?;
        // [G24-005] Return an error when the API returns no choices instead of
        // silently producing an empty string that downstream code cannot distinguish
        // from a legitimate empty response.
        let content = chat_response
            .choices
            .first()
            .map(|c| c.message.content.clone())
            .ok_or_else(|| anyhow::anyhow!("LLM API returned empty choices array"))?;

        let usage = chat_response.usage.map(|u| LLMUsage {
            prompt_tokens: u.prompt_tokens,
            completion_tokens: u.completion_tokens,
            total_tokens: u.total_tokens,
        });

        Ok(LLMResponse { content, usage })
    }
}

#[async_trait]
impl LLMClient for OpenAICompatibleClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        // G24-006: keep internal retry at 1 attempt (no internal retry) so that
        // ResilientLLMClient's cross-provider retry loop is the sole retry layer.
        // Stacking 3 inner retries × N providers leads to excessive backoff delays
        // and confusing failure counts in the circuit breaker.
        self.chat_once(messages).await
    }
}

// ============================================================================
// Anthropic-compatible LLM client
// ============================================================================

/// LLM client for Anthropic-compatible APIs (POST /v1/messages).
///
/// Used when `api_type` is `"anthropic"`. Handles the Anthropic message format
/// with `x-api-key` header and structured content blocks.
pub struct AnthropicCompatibleClient {
    client: Client,
    base_url: String,
    api_key: String,
    model: String,
}

#[derive(Debug, Serialize)]
struct AnthropicRequest {
    model: String,
    messages: Vec<AnthropicMessage>,
    max_tokens: i32,
    #[serde(skip_serializing_if = "Option::is_none")]
    system: Option<String>,
}

#[derive(Debug, Serialize, Deserialize)]
struct AnthropicMessage {
    role: String,
    content: String,
}

#[derive(Debug, Deserialize)]
struct AnthropicResponse {
    content: Vec<AnthropicContentBlock>,
    usage: Option<AnthropicUsage>,
}

#[derive(Debug, Deserialize)]
struct AnthropicContentBlock {
    text: Option<String>,
}

#[derive(Debug, Deserialize)]
struct AnthropicUsage {
    input_tokens: i32,
    output_tokens: i32,
}

impl AnthropicCompatibleClient {
    pub fn new(config: &OrchestratorConfig) -> Self {
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(30))
            .timeout(Duration::from_secs(config.timeout_secs))
            .build()
            .expect("Failed to create HTTP client");

        let base_url = normalize_base_url(&config.api_type, &config.base_url);

        tracing::info!(
            api_type = %config.api_type,
            input_url = %config.base_url,
            final_base_url = %base_url,
            "Anthropic-compatible LLM client URL normalized"
        );

        Self {
            client,
            base_url,
            api_key: config.api_key.clone(),
            model: config.model.clone(),
        }
    }

    async fn chat_once(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        let url = format!("{}/messages", self.base_url);

        // Extract system message and convert the rest
        let mut system_prompt = None;
        let mut api_messages = Vec::new();
        for m in &messages {
            if m.role == "system" {
                system_prompt = Some(m.content.clone());
            } else {
                api_messages.push(AnthropicMessage {
                    role: m.role.clone(),
                    content: m.content.clone(),
                });
            }
        }

        let msg_count = api_messages.len();
        let request = AnthropicRequest {
            model: self.model.clone(),
            messages: api_messages,
            max_tokens: 2048,
            system: system_prompt,
        };

        tracing::debug!(
            url = %url,
            model = %self.model,
            msg_count = msg_count,
            "Anthropic-compatible LLM request starting"
        );

        // Send both x-api-key (official Anthropic) and Authorization: Bearer
        // (third-party Anthropic-compatible providers like ZhipuAI). Real Anthropic
        // ignores Bearer; ZhipuAI ignores x-api-key. Both work simultaneously.
        let response = self
            .client
            .post(&url)
            .header("x-api-key", &self.api_key)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .header("anthropic-version", "2023-06-01")
            .header("Content-Type", "application/json")
            .json(&request)
            .send()
            .await?;

        let status = response.status();
        tracing::debug!(
            status = %status,
            "Anthropic-compatible LLM response received"
        );

        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow::anyhow!("LLM API error: {status} - {body}"));
        }

        let api_response: AnthropicResponse = response.json().await?;
        let content = api_response
            .content
            .iter()
            .filter_map(|block| block.text.as_deref())
            .collect::<Vec<_>>()
            .join("");

        if content.is_empty() {
            return Err(anyhow::anyhow!("Anthropic API returned empty content"));
        }

        let usage = api_response.usage.map(|u| LLMUsage {
            prompt_tokens: u.input_tokens,
            completion_tokens: u.output_tokens,
            total_tokens: u.input_tokens + u.output_tokens,
        });

        Ok(LLMResponse { content, usage })
    }
}

#[async_trait]
impl LLMClient for AnthropicCompatibleClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        self.chat_once(messages).await
    }
}

/// Build terminal completion prompt
///
/// This helper function encapsulates the logic for building prompts
/// to avoid string concatenation in business logic.
pub fn build_terminal_completion_prompt(
    terminal_id: &str,
    task_id: &str,
    commit_hash: &str,
    commit_message: &str,
) -> String {
    format!(
        "Terminal {terminal_id} has completed task {task_id}.\n\n\
         Commit: {commit_hash}\n\
         Message: {commit_message}\n\n\
         Please analyze the results and decide on the next step."
    )
}

/// Validates configuration and returns a rate-limited LLM client.
///
/// When `config.fallback_providers` is non-empty, the returned client is a
/// [`ResilientLLMClient`] that wraps the primary provider plus all fallbacks
/// with automatic circuit-breaking and failover.  Otherwise the original
/// single-provider path is used (fully backward compatible).
/// Determine whether to use Anthropic protocol based on api_type and base_url.
fn should_use_anthropic_protocol(config: &OrchestratorConfig) -> bool {
    // Explicit api_type takes priority — user knows their endpoint best
    match config.api_type.as_str() {
        "anthropic" | "anthropic-compatible" => return true,
        "openai" | "openai-compatible" | "google" => return false,
        _ => {}
    }
    // Auto-detect only when api_type is not explicitly set
    let url_lower = config.base_url.to_lowercase();
    url_lower.contains("/anthropic")
}

/// Build a single rate-limited LLM client based on api_type and base_url.
fn build_single_client(config: &OrchestratorConfig) -> anyhow::Result<Box<dyn LLMClient>> {
    let rps = config.rate_limit_requests_per_second;
    if should_use_anthropic_protocol(config) {
        let client = AnthropicCompatibleClient::new(config);
        let client = RateLimitedClient::new(client, rps)?;
        Ok(Box::new(client))
    } else {
        let client = OpenAICompatibleClient::new(config);
        let client = RateLimitedClient::new(client, rps)?;
        Ok(Box::new(client))
    }
}

pub fn create_llm_client(config: &OrchestratorConfig) -> anyhow::Result<Box<dyn LLMClient>> {
    config.validate().map_err(|e| anyhow::anyhow!(e))?;

    if config.fallback_providers.is_empty() {
        build_single_client(config)
    } else {
        // Multi-provider resilient path.
        let primary_name = format!("primary({})", config.model);
        let rps = config.rate_limit_requests_per_second;
        let primary_client: Box<dyn LLMClient> = build_single_client(config)?;

        let mut providers: Vec<(String, Box<dyn LLMClient>)> =
            vec![(primary_name, primary_client)];

        let mut fallbacks: Vec<_> = config.fallback_providers.clone();
        fallbacks.sort_by_key(|p| p.priority);

        for fb in &fallbacks {
            let fb_config = OrchestratorConfig {
                api_type: fb.api_type.clone(),
                base_url: fb.base_url.clone(),
                api_key: fb.api_key.clone(),
                model: fb.model.clone(),
                timeout_secs: config.timeout_secs,
                rate_limit_requests_per_second: rps,
                ..Default::default()
            };
            let fb_client = build_single_client(&fb_config)?;
            providers.push((fb.name.clone(), fb_client));
        }

        tracing::info!(
            "ResilientLLMClient created with {} providers: {:?}",
            providers.len(),
            providers.iter().map(|(n, _)| n.as_str()).collect::<Vec<_>>(),
        );

        Ok(Box::new(
            super::resilient_llm::ResilientLLMClient::new(providers),
        ))
    }
}

#[cfg(test)]
mod rate_limit_tests {
    use tokio::time::sleep;

    use super::*;

    fn test_messages() -> Vec<LLMMessage> {
        vec![LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }]
    }

    #[tokio::test]
    async fn rate_limiter_blocks_excessive_requests() {
        let client = RateLimitedClient::new(MockLLMClient::new(), 2).expect("rate limit");
        let messages = test_messages();

        assert!(client.chat(messages.clone()).await.is_ok());
        assert!(client.chat(messages.clone()).await.is_ok());

        let result = client.chat(messages.clone()).await;
        assert!(result.is_err());
        assert!(result.unwrap_err().to_string().contains("rate limit"));
    }

    #[tokio::test]
    async fn rate_limiter_refills_after_duration() {
        let client = RateLimitedClient::new(MockLLMClient::new(), 2).expect("rate limit");
        let messages = test_messages();

        client.chat(messages.clone()).await.expect("first");
        client.chat(messages.clone()).await.expect("second");

        sleep(Duration::from_millis(1100)).await;

        let result = client.chat(messages.clone()).await;
        assert!(result.is_ok());
    }
}

#[cfg(test)]
mod anthropic_protocol_tests {
    use super::*;

    #[test]
    fn test_explicit_openai_compatible_with_anthropic_url() {
        let config = OrchestratorConfig {
            api_type: "openai-compatible".to_string(),
            base_url: "https://open.bigmodel.cn/api/anthropic".to_string(),
            api_key: "test-key".to_string(),
            model: "glm-5".to_string(),
            ..Default::default()
        };
        assert!(!should_use_anthropic_protocol(&config));
    }

    #[test]
    fn test_explicit_anthropic_type() {
        let config = OrchestratorConfig {
            api_type: "anthropic-compatible".to_string(),
            base_url: "https://api.anthropic.com".to_string(),
            api_key: "test-key".to_string(),
            model: "claude-sonnet-4-20250514".to_string(),
            ..Default::default()
        };
        assert!(should_use_anthropic_protocol(&config));
    }

    #[test]
    fn test_unknown_type_with_anthropic_url_autodetects() {
        let config = OrchestratorConfig {
            api_type: String::new(),
            base_url: "https://proxy.example.com/anthropic/v1".to_string(),
            api_key: "test-key".to_string(),
            model: "test".to_string(),
            ..Default::default()
        };
        assert!(should_use_anthropic_protocol(&config));
    }

    #[test]
    fn test_unknown_type_without_anthropic_url() {
        let config = OrchestratorConfig {
            api_type: String::new(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "test-key".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };
        assert!(!should_use_anthropic_protocol(&config));
    }
}

#[cfg(test)]
mod url_normalization_tests {
    use utils::url::normalize_base_url;

    #[test]
    fn test_openai_official_gets_v1() {
        let url = normalize_base_url("openai", "https://api.openai.com");
        assert_eq!(url, "https://api.openai.com/v1");
    }

    #[test]
    fn test_openai_compatible_no_v1_append() {
        let url = normalize_base_url(
            "openai-compatible",
            "https://open.bigmodel.cn/api/paas/v4",
        );
        assert_eq!(url, "https://open.bigmodel.cn/api/paas/v4");
    }

    #[test]
    fn test_openai_already_has_v1_not_doubled() {
        let url = normalize_base_url("openai", "https://api.openai.com/v1");
        assert_eq!(url, "https://api.openai.com/v1");
    }

    #[test]
    fn test_anthropic_official_gets_v1() {
        let url = normalize_base_url("anthropic", "https://api.anthropic.com");
        assert_eq!(url, "https://api.anthropic.com/v1");
    }

    #[test]
    fn test_anthropic_compatible_no_v1_append() {
        let url = normalize_base_url(
            "anthropic-compatible",
            "https://open.bigmodel.cn/api/anthropic",
        );
        assert_eq!(url, "https://open.bigmodel.cn/api/anthropic");
    }

    #[test]
    fn test_trailing_slash_stripped() {
        let url = normalize_base_url("openai-compatible", "https://example.com/api/");
        assert_eq!(url, "https://example.com/api");
    }

    #[test]
    fn test_zhipuai_v4_preserved() {
        let url = normalize_base_url(
            "openai-compatible",
            "https://open.bigmodel.cn/api/paas/v4",
        );
        assert_eq!(url, "https://open.bigmodel.cn/api/paas/v4");
    }

    #[test]
    fn test_empty_api_type_no_v1_append() {
        let url = normalize_base_url("", "https://custom.provider.com/api");
        assert_eq!(url, "https://custom.provider.com/api");
    }

    #[test]
    fn test_google_type_no_v1_append() {
        let url = normalize_base_url("google", "https://generativelanguage.googleapis.com");
        assert_eq!(
            url,
            "https://generativelanguage.googleapis.com"
        );
    }
}

/// Integration tests that verify the full chain from OrchestratorConfig
/// through URL normalization and protocol selection — prevents regressions
/// of the 401 auth bug caused by blind `/v1` URL normalization.
#[cfg(test)]
mod full_chain_tests {
    use super::*;

    #[test]
    fn test_full_chain_zhipuai_openai_compatible() {
        let config = OrchestratorConfig {
            api_type: "openai-compatible".to_string(),
            base_url: "https://open.bigmodel.cn/api/paas/v4".to_string(),
            api_key: "test-key".to_string(),
            model: "glm-5".to_string(),
            ..Default::default()
        };

        // Verify config is valid
        assert!(config.validate().is_ok(), "Config should be valid");

        // Verify protocol selection
        assert!(
            !should_use_anthropic_protocol(&config),
            "ZhipuAI openai-compatible should NOT use Anthropic protocol"
        );

        // Verify URL normalization preserves provider path (no /v1 appended)
        let normalized = normalize_llm_base_url(&config.api_type, &config.base_url);
        assert_eq!(
            normalized, "https://open.bigmodel.cn/api/paas/v4",
            "openai-compatible must NOT append /v1 to provider URL"
        );

        // Verify the final request URL that would be constructed
        let expected_chat_url = format!("{normalized}/chat/completions");
        assert_eq!(
            expected_chat_url,
            "https://open.bigmodel.cn/api/paas/v4/chat/completions",
            "Final chat URL must use provider's v4 path"
        );
    }

    #[test]
    fn test_full_chain_zhipuai_anthropic_compatible() {
        let config = OrchestratorConfig {
            api_type: "anthropic-compatible".to_string(),
            base_url: "https://open.bigmodel.cn/api/anthropic".to_string(),
            api_key: "test-key".to_string(),
            model: "glm-5".to_string(),
            ..Default::default()
        };

        assert!(config.validate().is_ok(), "Config should be valid");

        assert!(
            should_use_anthropic_protocol(&config),
            "ZhipuAI anthropic-compatible SHOULD use Anthropic protocol"
        );

        // Verify URL normalization preserves provider path
        let normalized = normalize_llm_base_url(&config.api_type, &config.base_url);
        assert_eq!(
            normalized, "https://open.bigmodel.cn/api/anthropic",
            "anthropic-compatible must NOT append /v1 to provider URL"
        );

        // Verify the final messages URL for Anthropic protocol
        let expected_msg_url = format!("{normalized}/messages");
        assert_eq!(
            expected_msg_url,
            "https://open.bigmodel.cn/api/anthropic/messages",
            "Final messages URL must use provider's anthropic path"
        );
    }

    #[test]
    fn test_full_chain_official_openai() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4o".to_string(),
            ..Default::default()
        };

        assert!(config.validate().is_ok(), "Config should be valid");

        assert!(
            !should_use_anthropic_protocol(&config),
            "Official OpenAI should NOT use Anthropic protocol"
        );

        let normalized = normalize_llm_base_url(&config.api_type, &config.base_url);
        assert_eq!(
            normalized, "https://api.openai.com/v1",
            "Official openai MUST append /v1"
        );

        let expected_chat_url = format!("{normalized}/chat/completions");
        assert_eq!(
            expected_chat_url,
            "https://api.openai.com/v1/chat/completions"
        );
    }

    #[test]
    fn test_full_chain_official_anthropic() {
        let config = OrchestratorConfig {
            api_type: "anthropic".to_string(),
            base_url: "https://api.anthropic.com".to_string(),
            api_key: "sk-ant-test".to_string(),
            model: "claude-sonnet-4-20250514".to_string(),
            ..Default::default()
        };

        assert!(config.validate().is_ok(), "Config should be valid");

        assert!(
            should_use_anthropic_protocol(&config),
            "Official Anthropic SHOULD use Anthropic protocol"
        );

        let normalized = normalize_llm_base_url(&config.api_type, &config.base_url);
        assert_eq!(
            normalized, "https://api.anthropic.com/v1",
            "Official anthropic MUST append /v1"
        );

        let expected_msg_url = format!("{normalized}/messages");
        assert_eq!(
            expected_msg_url,
            "https://api.anthropic.com/v1/messages"
        );
    }

    #[test]
    fn test_protocol_detection_all_explicit_types() {
        let cases = vec![
            ("anthropic", true),
            ("anthropic-compatible", true),
            ("openai", false),
            ("openai-compatible", false),
            ("google", false),
        ];

        for (api_type, expected_anthropic) in cases {
            let config = OrchestratorConfig {
                api_type: api_type.to_string(),
                base_url: "https://example.com".to_string(),
                api_key: "test".to_string(),
                model: "test".to_string(),
                ..Default::default()
            };
            assert_eq!(
                should_use_anthropic_protocol(&config),
                expected_anthropic,
                "api_type '{}' should {}use Anthropic protocol",
                api_type,
                if expected_anthropic { "" } else { "NOT " }
            );
        }
    }

    #[test]
    fn test_create_llm_client_rejects_empty_key() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com".to_string(),
            api_key: String::new(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };
        assert!(
            create_llm_client(&config).is_err(),
            "Empty API key should fail validation via create_llm_client"
        );
    }

    #[test]
    fn test_create_llm_client_rejects_empty_base_url() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: String::new(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };
        assert!(
            create_llm_client(&config).is_err(),
            "Empty base URL should fail validation via create_llm_client"
        );
    }

    #[test]
    fn test_create_llm_client_rejects_empty_model() {
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com".to_string(),
            api_key: "sk-test".to_string(),
            model: String::new(),
            ..Default::default()
        };
        assert!(
            create_llm_client(&config).is_err(),
            "Empty model should fail validation via create_llm_client"
        );
    }

    /// Verify that build_single_client creates clients successfully for all
    /// provider types. Requires rustls crypto provider to be installed since
    /// reqwest uses `rustls-tls-webpki-roots-no-provider`.
    #[tokio::test]
    async fn test_build_single_client_all_provider_types() {
        let _ = rustls::crypto::ring::default_provider().install_default();

        let cases = vec![
            ("openai", "https://api.openai.com"),
            ("openai-compatible", "https://open.bigmodel.cn/api/paas/v4"),
            ("anthropic", "https://api.anthropic.com"),
            ("anthropic-compatible", "https://open.bigmodel.cn/api/anthropic"),
            ("google", "https://generativelanguage.googleapis.com"),
        ];

        for (api_type, base_url) in cases {
            let config = OrchestratorConfig {
                api_type: api_type.to_string(),
                base_url: base_url.to_string(),
                api_key: "test-key".to_string(),
                model: "test-model".to_string(),
                ..Default::default()
            };
            let result = build_single_client(&config);
            assert!(
                result.is_ok(),
                "build_single_client should succeed for api_type '{api_type}'"
            );
        }
    }

    /// Verify create_llm_client (the public entry point) works end-to-end
    /// for a ZhipuAI configuration — the exact scenario that triggered the
    /// original 401 bug.
    #[tokio::test]
    async fn test_create_llm_client_zhipuai_e2e() {
        let _ = rustls::crypto::ring::default_provider().install_default();

        let config = OrchestratorConfig {
            api_type: "openai-compatible".to_string(),
            base_url: "https://open.bigmodel.cn/api/paas/v4".to_string(),
            api_key: "test-key".to_string(),
            model: "glm-5".to_string(),
            ..Default::default()
        };

        let client = create_llm_client(&config);
        assert!(
            client.is_ok(),
            "create_llm_client should succeed for ZhipuAI openai-compatible config"
        );
    }
}
