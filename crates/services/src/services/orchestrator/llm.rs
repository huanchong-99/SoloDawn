//! LLM client abstractions and implementations.

use std::{num::NonZeroU32, sync::Arc, time::Duration};

use async_trait::async_trait;
use governor::{
    Quota, RateLimiter,
    clock::DefaultClock,
    state::{InMemoryState, NotKeyed},
};
use reqwest::Client;
use serde::{Deserialize, Serialize};
use twox_hash::XxHash64;

use utils::url::{ApiFormat, resolve_endpoint};

#[allow(deprecated, unused_imports)]
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
        // Use until_ready() to wait for a token instead of check() which rejects
        // immediately. With check(), a transient rate-limit rejection gets
        // misclassified as a provider failure by ResilientLLMClient's circuit
        // breaker, potentially marking a healthy provider as dead.
        self.rate_limiter.until_ready().await;
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

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);

        tracing::info!(
            api_type = %config.api_type,
            input_url = %config.base_url,
            resolved_url = %endpoint.url,
            api_format = %endpoint.api_format,
            "OpenAI-compatible LLM client endpoint resolved"
        );

        Self {
            client,
            base_url: endpoint.url,
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
            max_tokens: Some(16384),
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

        // Read body as text first for multi-format parsing (Rectifier pattern)
        let body = response.text().await.map_err(|e| {
            tracing::error!("Failed to read OpenAI-compatible response body: {e}");
            e
        })?;

        // Strategy 1: Standard OpenAI format { "choices": [{ "message": { "content" } }] }
        if let Ok(chat_response) = serde_json::from_str::<ChatResponse>(&body) {
            if let Some(choice) = chat_response.choices.first() {
                let usage = chat_response.usage.map(|u| LLMUsage {
                    prompt_tokens: u.prompt_tokens,
                    completion_tokens: u.completion_tokens,
                    total_tokens: u.total_tokens,
                });
                return Ok(LLMResponse {
                    content: choice.message.content.clone(),
                    usage,
                });
            } else {
                return Err(anyhow::anyhow!("LLM returned empty choices"));
            }
        }

        // Strategy 2: Try alternative response formats from third-party gateways
        if let Ok(json) = serde_json::from_str::<serde_json::Value>(&body) {
            // { "content": "..." } direct
            if let Some(content) = json.get("content").and_then(|c| c.as_str()) {
                return Ok(LLMResponse { content: content.to_string(), usage: None });
            }
            // { "output": "..." } (OpenAI Responses API string form)
            if let Some(output) = json.get("output").and_then(|o| o.as_str()) {
                return Ok(LLMResponse { content: output.to_string(), usage: None });
            }
            // { "output": [{ "type": "message", "content": [{ "type": "output_text", "text": "..." }] }] }
            if let Some(arr) = json.get("output").and_then(|o| o.as_array()) {
                let mut text = String::new();
                for item in arr {
                    if item.get("type").and_then(|t| t.as_str()) == Some("message") {
                        if let Some(blocks) = item.get("content").and_then(|c| c.as_array()) {
                            for b in blocks {
                                if b.get("type").and_then(|t| t.as_str()) == Some("output_text") {
                                    if let Some(t) = b.get("text").and_then(|t| t.as_str()) {
                                        text.push_str(t);
                                    }
                                }
                            }
                        }
                    }
                }
                if !text.is_empty() {
                    return Ok(LLMResponse { content: text, usage: None });
                }
            }
            // Error object
            if let Some(error) = json.get("error") {
                let msg = error.get("message").and_then(|m| m.as_str()).unwrap_or("unknown");
                return Err(anyhow::anyhow!("LLM API returned error: {msg}"));
            }
        }

        tracing::error!(
            body_len = body.len(),
            body_preview = %body.chars().take(500).collect::<String>(),
            "Failed to parse OpenAI-compatible response in all known formats"
        );
        Err(anyhow::anyhow!("error decoding response body: unrecognized format"))
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
    chat_url: String,
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
    /// Always true — some Anthropic-compatible proxies only support streaming.
    stream: bool,
}

#[derive(Debug, Serialize, Deserialize)]
struct AnthropicMessage {
    role: String,
    content: String,
}

impl AnthropicCompatibleClient {
    pub fn new(config: &OrchestratorConfig) -> Self {
        let client = Client::builder()
            .connect_timeout(Duration::from_secs(30))
            .timeout(Duration::from_secs(config.timeout_secs))
            .build()
            .expect("Failed to create HTTP client");

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);

        tracing::info!(
            api_type = %config.api_type,
            input_url = %config.base_url,
            resolved_url = %endpoint.url,
            api_format = %endpoint.api_format,
            "Anthropic-compatible LLM client endpoint resolved"
        );

        let chat_url = endpoint.chat_endpoint();

        Self {
            client,
            chat_url,
            api_key: config.api_key.clone(),
            model: config.model.clone(),
        }
    }

    async fn chat_once(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        let url = self.chat_url.clone();

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
            max_tokens: 16384,
            system: system_prompt,
            stream: true,
        };

        tracing::debug!(
            url = %url,
            model = %self.model,
            msg_count = msg_count,
            "Anthropic-compatible LLM request starting (streaming)"
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

        // Parse SSE stream and accumulate text content + usage
        let body = response.text().await?;
        let mut content = String::new();
        let mut input_tokens: i32 = 0;
        let mut output_tokens: i32 = 0;

        for line in body.lines() {
            let line = line.trim();
            if let Some(data) = line.strip_prefix("data: ") {
                if data == "[DONE]" {
                    break;
                }
                if let Ok(event) = serde_json::from_str::<serde_json::Value>(data) {
                    match event.get("type").and_then(|t| t.as_str()) {
                        Some("content_block_delta") => {
                            if let Some(text) = event
                                .pointer("/delta/text")
                                .and_then(|t| t.as_str())
                            {
                                content.push_str(text);
                            }
                        }
                        Some("message_start") => {
                            if let Some(u) = event.pointer("/message/usage/input_tokens").and_then(serde_json::Value::as_i64) {
                                input_tokens = u as i32;
                            }
                        }
                        Some("message_delta") => {
                            if let Some(u) = event.pointer("/usage/output_tokens").and_then(serde_json::Value::as_i64) {
                                output_tokens = u as i32;
                            }
                        }
                        _ => {}
                    }
                }
            }
        }

        // Fallback: if SSE parsing yielded nothing, the provider may have
        // returned a standard (non-streaming) JSON response despite stream=true.
        // Try to extract content from the raw body as a regular Anthropic response.
        if content.is_empty() {
            if let Ok(json) = serde_json::from_str::<serde_json::Value>(&body) {
                // Standard Anthropic response: { "content": [{ "type": "text", "text": "..." }] }
                if let Some(blocks) = json.get("content").and_then(|c| c.as_array()) {
                    for block in blocks {
                        if block.get("type").and_then(|t| t.as_str()) == Some("text") {
                            if let Some(text) = block.get("text").and_then(|t| t.as_str()) {
                                content.push_str(text);
                            }
                        }
                    }
                }
                // Also try extracting usage from the non-streaming response
                if let Some(u) = json.pointer("/usage/input_tokens").and_then(serde_json::Value::as_i64) {
                    input_tokens = u as i32;
                }
                if let Some(u) = json.pointer("/usage/output_tokens").and_then(serde_json::Value::as_i64) {
                    output_tokens = u as i32;
                }
            }
        }

        if content.is_empty() {
            tracing::warn!(
                body_len = body.len(),
                body_preview = %body.chars().take(500).collect::<String>(),
                "Anthropic API returned empty content after SSE + JSON fallback parsing"
            );
            return Err(anyhow::anyhow!("Anthropic API returned empty content"));
        }

        let usage = if input_tokens > 0 || output_tokens > 0 {
            Some(LLMUsage {
                prompt_tokens: input_tokens,
                completion_tokens: output_tokens,
                total_tokens: input_tokens + output_tokens,
            })
        } else {
            None
        };

        Ok(LLMResponse { content, usage })
    }
}

#[async_trait]
impl LLMClient for AnthropicCompatibleClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        self.chat_once(messages).await
    }
}

// ============================================================================
// Claude Code Native Client (OAuth-based, for Max/Pro subscribers)
// ============================================================================

/// Credentials read from `~/.claude/.credentials.json`.
#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ClaudeCredentials {
    #[serde(rename = "claudeAiOauth")]
    claude_ai_oauth: Option<ClaudeOAuth>,
}

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
struct ClaudeOAuth {
    access_token: String,
}

/// Anthropic Messages API request body used by the native client.
/// Uses structured system content (array of text blocks) unlike the
/// third-party `AnthropicRequest` which uses a plain string.
#[derive(Debug, Serialize)]
struct NativeAnthropicRequest {
    model: String,
    messages: Vec<AnthropicMessage>,
    max_tokens: i32,
    system: Vec<SystemTextBlock>,
    stream: bool,
}

#[derive(Debug, Serialize)]
struct SystemTextBlock {
    r#type: String,
    text: String,
}

/// LLM client that calls `api.anthropic.com/v1/messages` using the locally
/// authenticated Claude Code CLI credentials (OAuth token from Max/Pro
/// subscription). The request format mirrors what Claude Code CLI sends so
/// the billing header routes through the user's existing subscription.
///
/// ## Security
/// - OAuth token is read from disk on each `chat()` call (never cached long-term)
/// - Token is never logged, stored in DB, or included in tracing output
pub struct ClaudeCodeNativeClient {
    client: Client,
    model: String,
    org_id: String,
    cc_version: String,
}

impl ClaudeCodeNativeClient {
    /// Attempt to create a native client by reading `~/.claude/.credentials.json`.
    /// Returns `None` if credentials are missing or unreadable.
    pub fn try_new(model: &str) -> Option<Self> {
        let home = dirs::home_dir()?;
        let creds_path = home.join(".claude").join(".credentials.json");
        let creds_str = std::fs::read_to_string(&creds_path).ok()?;
        let creds: ClaudeCredentials = serde_json::from_str(&creds_str).ok()?;
        let oauth = creds.claude_ai_oauth?;
        if oauth.access_token.is_empty() {
            return None;
        }

        // Detect Claude Code CLI version
        let cc_version = detect_cc_version().unwrap_or_else(|| "2.1.92".to_string());

        let client = Client::builder()
            .connect_timeout(Duration::from_secs(30))
            .timeout(Duration::from_secs(300))
            .build()
            .ok()?;

        tracing::info!(
            cc_version = %cc_version,
            "Claude Code native client initialized (OAuth credentials found)"
        );

        Some(Self {
            client,
            model: model.to_string(),
            org_id: "51e1b9ba-604d-4b8b-bdd6-719dddbc7e65".to_string(),
            cc_version,
        })
    }

    /// Read OAuth access token from credentials file.
    /// Called per-request to pick up token refreshes.
    fn read_access_token() -> anyhow::Result<String> {
        let home = dirs::home_dir()
            .ok_or_else(|| anyhow::anyhow!("Cannot determine home directory"))?;
        let creds_path = home.join(".claude").join(".credentials.json");
        let creds_str = std::fs::read_to_string(&creds_path)
            .map_err(|e| anyhow::anyhow!("Cannot read Claude credentials: {e}"))?;
        let creds: ClaudeCredentials = serde_json::from_str(&creds_str)
            .map_err(|e| anyhow::anyhow!("Cannot parse Claude credentials: {e}"))?;
        let token = creds
            .claude_ai_oauth
            .and_then(|o| if o.access_token.is_empty() { None } else { Some(o.access_token) })
            .ok_or_else(|| anyhow::anyhow!("Claude OAuth token not found in credentials"))?;
        Ok(token)
    }

    /// Compute the Claude Code integrity hash (cch).
    ///
    /// Algorithm: xxhash64(body_with_cch=00000, seed=0x6E52736AC806831E) & 0xFFFFF
    /// Result is a 5-character lowercase hex string.
    fn compute_cch(body_with_placeholder: &str) -> String {
        const CCH_SEED: u64 = 0x6E52_736A_C806_831E;
        let hash = XxHash64::oneshot(CCH_SEED, body_with_placeholder.as_bytes());
        let masked = hash & 0xFFFFF;
        format!("{masked:05x}")
    }

    /// Build the request body with correct cch hash.
    fn build_body(
        &self,
        messages: Vec<LLMMessage>,
    ) -> anyhow::Result<String> {
        // Separate system messages from conversation
        let mut user_system_prompt = String::new();
        let mut api_messages = Vec::new();
        for m in &messages {
            if m.role == "system" {
                if !user_system_prompt.is_empty() {
                    user_system_prompt.push('\n');
                }
                user_system_prompt.push_str(&m.content);
            } else {
                api_messages.push(AnthropicMessage {
                    role: m.role.clone(),
                    content: m.content.clone(),
                });
            }
        }

        // Build system blocks: billing header + Claude Code identity + user system prompt
        let billing_text = format!(
            "x-anthropic-billing-header: cc_version={}; cc_entrypoint=cli; cch=00000;",
            self.cc_version
        );

        let mut system_blocks = vec![
            SystemTextBlock {
                r#type: "text".to_string(),
                text: billing_text,
            },
            SystemTextBlock {
                r#type: "text".to_string(),
                text: "You are Claude Code, Anthropic's official CLI for Claude.".to_string(),
            },
        ];

        // Append user's system prompt as a third block if present
        if !user_system_prompt.is_empty() {
            system_blocks.push(SystemTextBlock {
                r#type: "text".to_string(),
                text: user_system_prompt,
            });
        }

        let request = NativeAnthropicRequest {
            model: self.model.clone(),
            messages: api_messages,
            max_tokens: 16384,
            system: system_blocks,
            stream: false,
        };

        // Serialize with placeholder cch=00000
        let body_placeholder = serde_json::to_string(&request)
            .map_err(|e| anyhow::anyhow!("Failed to serialize request: {e}"))?;

        // Compute actual cch and replace placeholder
        let cch = Self::compute_cch(&body_placeholder);
        let body = body_placeholder.replacen("cch=00000", &format!("cch={cch}"), 1);

        Ok(body)
    }

    /// Parse SSE stream response (same format as AnthropicCompatibleClient).
    fn parse_sse_response(body: &str) -> anyhow::Result<LLMResponse> {
        let mut content = String::new();
        let mut input_tokens: i32 = 0;
        let mut output_tokens: i32 = 0;

        for line in body.lines() {
            let line = line.trim();
            if let Some(data) = line.strip_prefix("data: ") {
                if data == "[DONE]" {
                    break;
                }
                if let Ok(event) = serde_json::from_str::<serde_json::Value>(data) {
                    match event.get("type").and_then(|t| t.as_str()) {
                        Some("content_block_delta") => {
                            if let Some(text) = event.pointer("/delta/text").and_then(|t| t.as_str()) {
                                content.push_str(text);
                            }
                        }
                        Some("message_start") => {
                            if let Some(u) = event.pointer("/message/usage/input_tokens").and_then(serde_json::Value::as_i64) {
                                input_tokens = u as i32;
                            }
                        }
                        Some("message_delta") => {
                            if let Some(u) = event.pointer("/usage/output_tokens").and_then(serde_json::Value::as_i64) {
                                output_tokens = u as i32;
                            }
                        }
                        _ => {}
                    }
                }
            }
        }

        // Fallback: non-streaming JSON response
        if content.is_empty() {
            if let Ok(json) = serde_json::from_str::<serde_json::Value>(body) {
                if let Some(blocks) = json.get("content").and_then(|c| c.as_array()) {
                    for block in blocks {
                        if block.get("type").and_then(|t| t.as_str()) == Some("text") {
                            if let Some(text) = block.get("text").and_then(|t| t.as_str()) {
                                content.push_str(text);
                            }
                        }
                    }
                }
                if let Some(u) = json.pointer("/usage/input_tokens").and_then(serde_json::Value::as_i64) {
                    input_tokens = u as i32;
                }
                if let Some(u) = json.pointer("/usage/output_tokens").and_then(serde_json::Value::as_i64) {
                    output_tokens = u as i32;
                }
            }
        }

        if content.is_empty() {
            return Err(anyhow::anyhow!("Claude Code native API returned empty content"));
        }

        let usage = if input_tokens > 0 || output_tokens > 0 {
            Some(LLMUsage {
                prompt_tokens: input_tokens,
                completion_tokens: output_tokens,
                total_tokens: input_tokens + output_tokens,
            })
        } else {
            None
        };

        Ok(LLMResponse { content, usage })
    }
}

#[async_trait]
impl LLMClient for ClaudeCodeNativeClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        let token = Self::read_access_token()?;
        let body = self.build_body(messages)?;

        tracing::debug!(
            model = %self.model,
            body_len = body.len(),
            "Claude Code native API request"
        );

        let response = self
            .client
            .post("https://api.anthropic.com/v1/messages")
            .header("x-api-key", &token)
            .header("anthropic-version", "2023-06-01")
            .header("anthropic-beta", "interleaved-thinking-2025-05-14")
            .header("anthropic-organization", &self.org_id)
            .header("Content-Type", "application/json")
            .body(body)
            .send()
            .await?;

        let status = response.status();
        if !status.is_success() {
            let err_body = response.text().await.unwrap_or_default();
            // Never log the token — only log status and truncated error body
            tracing::warn!(
                status = %status,
                err_preview = %err_body.chars().take(300).collect::<String>(),
                "Claude Code native API error"
            );
            return Err(anyhow::anyhow!("Claude Code native API error: {status}"));
        }

        let resp_body = response.text().await?;
        Self::parse_sse_response(&resp_body)
    }
}

/// Detect Claude Code CLI version from `claude --version`.
fn detect_cc_version() -> Option<String> {
    let output = std::process::Command::new("claude")
        .arg("--version")
        // R6 port-leak hygiene: strip SoloDawn dev ports consistently across
        // every subprocess spawn. `claude --version` doesn't consume these,
        // but keeping the strip uniform prevents future drift.
        .env_remove("PORT")
        .env_remove("BACKEND_PORT")
        .env_remove("FRONTEND_PORT")
        .output()
        .ok()?;
    let version_str = String::from_utf8_lossy(&output.stdout);
    // Output format: "claude v2.1.92" or similar
    let version = version_str.trim();
    version
        .strip_prefix("claude v")
        .or_else(|| version.strip_prefix("claude/"))
        .or_else(|| {
            // Try to extract version number from anywhere in the string
            version.split_whitespace().find(|s| s.chars().next().is_some_and(|c| c.is_ascii_digit()))
        })
        .map(|v| v.trim().to_string())
}

/// Try to create a Claude Code native LLM client for the Planning LLM.
/// Returns `None` if Claude Code CLI is not authenticated locally.
pub fn create_claude_code_native_client(model: &str) -> Option<Box<dyn LLMClient>> {
    ClaudeCodeNativeClient::try_new(model).map(|c| Box::new(c) as Box<dyn LLMClient>)
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
/// Build a single rate-limited LLM client based on api_type.
///
/// Uses `resolve_endpoint` to determine both the URL and `ApiFormat` from the
/// configured `api_type`. Protocol selection is driven entirely by `api_type`,
/// not by URL content inspection.
fn build_single_client(config: &OrchestratorConfig) -> anyhow::Result<Box<dyn LLMClient>> {
    let rps = config.rate_limit_requests_per_second;
    let endpoint = resolve_endpoint(&config.api_type, &config.base_url);

    match endpoint.api_format {
        ApiFormat::AnthropicMessages => {
            let client = AnthropicCompatibleClient::new(config);
            let client = RateLimitedClient::new(client, rps)?;
            Ok(Box::new(client))
        }
        ApiFormat::OpenAIChat | ApiFormat::Google => {
            let client = OpenAICompatibleClient::new(config);
            let client = RateLimitedClient::new(client, rps)?;
            Ok(Box::new(client))
        }
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
    async fn rate_limiter_waits_instead_of_rejecting() {
        // until_ready() blocks until a token is available rather than
        // rejecting immediately, so the third request should succeed
        // after a short wait — not fail with "rate limit exceeded".
        let client = RateLimitedClient::new(MockLLMClient::new(), 2).expect("rate limit");
        let messages = test_messages();

        assert!(client.chat(messages.clone()).await.is_ok());
        assert!(client.chat(messages.clone()).await.is_ok());

        // Third request will block briefly until a token refills, then succeed.
        let result = tokio::time::timeout(
            Duration::from_secs(3),
            client.chat(messages.clone()),
        )
        .await;
        assert!(result.is_ok(), "until_ready should unblock within the timeout");
        assert!(result.unwrap().is_ok(), "chat should succeed after waiting");
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
        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::OpenAIChat, "openai-compatible must use OpenAI format regardless of URL content");
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
        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::AnthropicMessages);
    }

    #[test]
    fn test_unknown_type_defaults_to_openai() {
        let config = OrchestratorConfig {
            api_type: String::new(),
            base_url: "https://proxy.example.com/anthropic/v1".to_string(),
            api_key: "test-key".to_string(),
            model: "test".to_string(),
            ..Default::default()
        };
        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::OpenAIChat, "unknown api_type defaults to OpenAI, no URL guessing");
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
        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::OpenAIChat);
    }
}

#[cfg(test)]
mod url_normalization_tests {
    #[allow(deprecated, unused_imports)]
    use utils::url::normalize_base_url;

    #[allow(deprecated)]
    #[test]
    fn test_openai_official_gets_v1() {
        let url = normalize_base_url("openai", "https://api.openai.com");
        assert_eq!(url, "https://api.openai.com/v1");
    }

    #[allow(deprecated)]
    #[test]
    fn test_openai_compatible_no_v1_append() {
        let url = normalize_base_url(
            "openai-compatible",
            "https://open.bigmodel.cn/api/paas/v4",
        );
        assert_eq!(url, "https://open.bigmodel.cn/api/paas/v4");
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

        assert!(config.validate().is_ok(), "Config should be valid");

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::OpenAIChat, "ZhipuAI openai-compatible should use OpenAI format");
        assert_eq!(endpoint.url, "https://open.bigmodel.cn/api/paas/v4", "URL preserved, no /v1 appended");

        let chat_url = endpoint.chat_endpoint();
        assert_eq!(chat_url, "https://open.bigmodel.cn/api/paas/v4/chat/completions");
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

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::AnthropicMessages);
        // Key fix: no /v1 appended for anthropic-compatible
        assert_eq!(endpoint.url, "https://open.bigmodel.cn/api/anthropic");

        let msg_url = endpoint.chat_endpoint();
        assert_eq!(msg_url, "https://open.bigmodel.cn/api/anthropic/v1/messages");
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

        assert!(config.validate().is_ok());

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::OpenAIChat);
        assert_eq!(endpoint.url, "https://api.openai.com");

        let chat_url = endpoint.chat_endpoint();
        assert_eq!(chat_url, "https://api.openai.com/chat/completions");
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

        assert!(config.validate().is_ok());

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::AnthropicMessages);
        assert_eq!(endpoint.url, "https://api.anthropic.com");

        let msg_url = endpoint.chat_endpoint();
        assert_eq!(msg_url, "https://api.anthropic.com/v1/messages");
    }

    #[test]
    fn test_anthropic_chat_endpoint_preserves_existing_v1() {
        // If the user already put /v1 on the base URL, chat_endpoint must
        // not double it. This covers migrations from the legacy
        // normalize_base_url path that appended /v1.
        let cases = [
            (
                "https://api.anthropic.com/v1",
                "https://api.anthropic.com/v1/messages",
            ),
            (
                "https://open.bigmodel.cn/api/anthropic/v1",
                "https://open.bigmodel.cn/api/anthropic/v1/messages",
            ),
            (
                "https://open.bigmodel.cn/api/anthropic/v1/",
                "https://open.bigmodel.cn/api/anthropic/v1/messages",
            ),
        ];
        for (input, expected) in cases {
            let endpoint = resolve_endpoint("anthropic-compatible", input);
            assert_eq!(endpoint.chat_endpoint(), expected, "input={input}");
        }
    }

    #[test]
    fn test_protocol_detection_all_explicit_types() {
        let cases: Vec<(&str, ApiFormat)> = vec![
            ("anthropic", ApiFormat::AnthropicMessages),
            ("anthropic-compatible", ApiFormat::AnthropicMessages),
            ("openai", ApiFormat::OpenAIChat),
            ("openai-compatible", ApiFormat::OpenAIChat),
            ("google", ApiFormat::Google),
        ];

        for (api_type, expected_format) in cases {
            let endpoint = resolve_endpoint(api_type, "https://example.com");
            assert_eq!(
                endpoint.api_format, expected_format,
                "api_type '{}' should resolve to {:?}",
                api_type, expected_format
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

#[cfg(test)]
mod claude_code_native_tests {
    use super::*;

    #[test]
    fn test_cch_computation_deterministic() {
        let body = r#"{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hello"}],"max_tokens":16384,"system":[{"type":"text","text":"x-anthropic-billing-header: cc_version=2.1.92; cc_entrypoint=cli; cch=00000;"},{"type":"text","text":"You are Claude Code, Anthropic's official CLI for Claude."}],"stream":true}"#;
        let cch = ClaudeCodeNativeClient::compute_cch(body);
        // Must be exactly 5 hex characters
        assert_eq!(cch.len(), 5, "cch must be 5 hex characters");
        assert!(
            cch.chars().all(|c| c.is_ascii_hexdigit()),
            "cch must be all hex digits, got: {cch}"
        );
        // Must be deterministic
        let cch2 = ClaudeCodeNativeClient::compute_cch(body);
        assert_eq!(cch, cch2, "cch must be deterministic");
    }

    #[test]
    fn test_cch_changes_with_different_input() {
        let body1 = r#"{"model":"a","messages":[],"cch=00000"}"#;
        let body2 = r#"{"model":"b","messages":[],"cch=00000"}"#;
        let cch1 = ClaudeCodeNativeClient::compute_cch(body1);
        let cch2 = ClaudeCodeNativeClient::compute_cch(body2);
        assert_ne!(cch1, cch2, "Different inputs should produce different cch values");
    }

    #[test]
    fn test_cch_masked_to_20_bits() {
        // The cch value is hash & 0xFFFFF, so max value is 0xFFFFF = 1048575
        let body = r"test body with cch=00000 placeholder";
        let cch = ClaudeCodeNativeClient::compute_cch(body);
        let val = u64::from_str_radix(&cch, 16).expect("cch must be valid hex");
        assert!(val <= 0xFFFFF, "cch value {val} exceeds 20-bit mask");
    }

    #[test]
    fn test_detect_cc_version_format() {
        // This test just validates the function doesn't panic
        // Actual version detection depends on CLI installation
        let _version = detect_cc_version();
    }

    /// Ad-hoc upstream-acceptance probe for a candidate model ID under the
    /// local Claude Code subscription. Marked `#[ignore]` — run manually with:
    ///
    ///   cargo test -p services --lib -- --ignored test_probe_subscription_model_acceptance
    ///
    /// Reads `~/.claude/.credentials.json`, sends a minimal "hi" prompt, and
    /// asserts a 2xx response. Used to validate that
    /// `claude-sonnet-4-6`/`claude-opus-4-6`/etc. are accepted by the
    /// subscription endpoint BEFORE swapping the hardcoded default in
    /// agent.rs:172 / :500 and planning_drafts.rs:339.
    #[tokio::test]
    #[ignore = "network + real OAuth token — manual run only (see doc comment)"]
    async fn test_probe_subscription_model_acceptance() {
        // reqwest is configured with `rustls-tls-webpki-roots-no-provider` at
        // the workspace level — the app (server.exe) installs the ring
        // crypto provider at startup, but unit tests must do it themselves.
        let _ = rustls::crypto::ring::default_provider().install_default();

        // Change this to test a different candidate model.
        let candidate = std::env::var("PROBE_MODEL")
            .unwrap_or_else(|_| "claude-sonnet-4-6".to_string());

        let client = ClaudeCodeNativeClient::try_new(&candidate)
            .expect("Claude Code OAuth credentials must be present at ~/.claude/.credentials.json");

        let messages = vec![LLMMessage {
            role: "user".to_string(),
            content: "Respond with the single word 'ok' and nothing else."
                .to_string(),
        }];

        match client.chat(messages).await {
            Ok(resp) => {
                println!(
                    "UPSTREAM ACCEPTED model={} content={:?} usage={:?}",
                    candidate, resp.content, resp.usage
                );
            }
            Err(e) => {
                panic!(
                    "UPSTREAM REJECTED model={candidate}: {e}. \
                     Do NOT swap the hardcoded default to this ID."
                );
            }
        }
    }
}
