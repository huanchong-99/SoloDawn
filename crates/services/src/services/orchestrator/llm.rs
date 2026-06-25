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
use utils::url::{ApiFormat, resolve_endpoint};

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

/// Marker error for LLM failures that should be retried with long exponential
/// backoff before giving up — HTTP 408/429/500/502/503/504/529, connect
/// timeouts, and connection errors. `ResilientLLMClient` downcasts this to
/// decide whether to retry the provider (for hours if needed) or fail fast.
///
/// Contrast with non-retryable failures (401/403/400/parse errors), which
/// return a plain `anyhow::Error` and surface immediately.
#[derive(Debug, thiserror::Error)]
#[error("{0}")]
pub struct RetryableLlmError(pub String);

/// HTTP status codes that indicate a transient, retryable failure.
fn is_retryable_status(status: reqwest::StatusCode) -> bool {
    matches!(status.as_u16(), 408 | 429 | 500 | 502 | 503 | 504 | 529)
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
                let msg = format!("OpenAI-compatible LLM request failed: {e}");
                // Timeouts and connection errors are transient: wrap as
                // retryable so ResilientLLMClient backs off and retries.
                if e.is_timeout() || e.is_connect() {
                    anyhow::Error::from(RetryableLlmError(msg))
                } else {
                    anyhow::anyhow!(msg)
                }
            })?;

        tracing::info!(
            status = %response.status(),
            "OpenAI-compatible LLM response received"
        );

        if !response.status().is_success() {
            let status = response.status();
            let body = response.text().await.unwrap_or_default();
            let msg = format!("LLM API error: {status} - {}", &body[..body.len().min(200)]);
            // Retryable status (429/5xx/overload) → long backoff retry;
            // non-retryable (401/403/400) → fail fast.
            if is_retryable_status(status) {
                return Err(anyhow::Error::from(RetryableLlmError(msg)));
            }
            return Err(anyhow::anyhow!(msg));
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
                return Ok(LLMResponse {
                    content: content.to_string(),
                    usage: None,
                });
            }
            // { "output": "..." } (OpenAI Responses API string form)
            if let Some(output) = json.get("output").and_then(|o| o.as_str()) {
                return Ok(LLMResponse {
                    content: output.to_string(),
                    usage: None,
                });
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
                    return Ok(LLMResponse {
                        content: text,
                        usage: None,
                    });
                }
            }
            // Error object
            if let Some(error) = json.get("error") {
                let msg = error
                    .get("message")
                    .and_then(|m| m.as_str())
                    .unwrap_or("unknown");
                return Err(anyhow::anyhow!("LLM API returned error: {msg}"));
            }
        }

        tracing::error!(
            body_len = body.len(),
            body_preview = %body.chars().take(500).collect::<String>(),
            "Failed to parse OpenAI-compatible response in all known formats"
        );
        Err(anyhow::anyhow!(
            "error decoding response body: unrecognized format"
        ))
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
            .await
            .map_err(|e| {
                tracing::error!(url = %url, "Anthropic-compatible LLM request failed: {e}");
                let msg = format!("Anthropic-compatible LLM request failed: {e}");
                if e.is_timeout() || e.is_connect() {
                    anyhow::Error::from(RetryableLlmError(msg))
                } else {
                    anyhow::anyhow!(msg)
                }
            })?;

        let status = response.status();
        tracing::debug!(
            status = %status,
            "Anthropic-compatible LLM response received"
        );

        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            let msg = format!("LLM API error: {status} - {}", &body[..body.len().min(200)]);
            if is_retryable_status(status) {
                return Err(anyhow::Error::from(RetryableLlmError(msg)));
            }
            return Err(anyhow::anyhow!(msg));
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
                            if let Some(text) =
                                event.pointer("/delta/text").and_then(|t| t.as_str())
                            {
                                content.push_str(text);
                            }
                        }
                        Some("message_start") => {
                            if let Some(u) = event
                                .pointer("/message/usage/input_tokens")
                                .and_then(serde_json::Value::as_i64)
                            {
                                input_tokens = u as i32;
                            }
                        }
                        Some("message_delta") => {
                            if let Some(u) = event
                                .pointer("/usage/output_tokens")
                                .and_then(serde_json::Value::as_i64)
                            {
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
                if let Some(u) = json
                    .pointer("/usage/input_tokens")
                    .and_then(serde_json::Value::as_i64)
                {
                    input_tokens = u as i32;
                }
                if let Some(u) = json
                    .pointer("/usage/output_tokens")
                    .and_then(serde_json::Value::as_i64)
                {
                    output_tokens = u as i32;
                }
            }
        }

        if content.is_empty() {
            tracing::warn!(
                body_len = body.len(),
                body_preview = %body.chars().take(200).collect::<String>(),
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
// Interactive Claude single-turn client (no-`-p`, transcript-read, ToS-clean)
// ============================================================================
//
// Replaces the removed `ClaudeCodeNativeClient` impersonation client (which
// extracted the OAuth token and POSTed to api.anthropic.com with a spoofed
// `cch` hash + "You are Claude Code" identity — a live ToS violation). Instead
// this drives the GENUINE `claude` binary as a single-turn interactive run on
// the user's own machine and reads the answer back from the on-disk transcript
// JSONL, exactly like the no-`-p` coding-agent transport (see
// docs/developed/plans/2026-06-15-no-p-interactive-transport.md). No extra API
// key is required and subscription users stay off the Agent SDK credit pool.

use executors::executors::claude::{ClaudeCode, ClaudeContentItem, ClaudeJson};

/// Returns `true` when native Claude Code subscription OAuth credentials are
/// present locally (`~/.claude/.credentials.json`). Used to decide whether the
/// interactive single-turn transport is available as a planning fallback.
fn native_claude_credentials_present() -> bool {
    dirs::home_dir()
        .map(|h| h.join(".claude").join(".credentials.json"))
        .is_some_and(|p| p.exists())
}

/// Single-turn LLM client that drives the genuine `claude` binary interactively
/// (no `-p`, no stream-json control protocol) and reads the assistant answer
/// from the on-disk session transcript JSONL.
///
/// On each `chat()` it: provisions a per-call isolated CLAUDE home via
/// `cc_switch::create_interactive_isolated_home`, copies the user's native
/// `~/.claude/.credentials.json` (+ `settings.json`) into it, builds the
/// interactive argv via `ClaudeCode::build_interactive_command_parts`, spawns a
/// piped one-shot (stdin closed → one turn then exit), then parses the final
/// `assistant` message text out of the transcript. The home is removed after.
///
/// ## ToS / billing
/// - Drives the UNMODIFIED genuine binary; we never extract the OAuth token into
///   SoloDawn's own auth path (cc_switch only copies the creds file for the
///   binary to consume), so this is the subscription/interactive surface, not
///   the metered Agent SDK / `-p` surface.
pub struct InteractiveClaudeClient {
    model: String,
}

impl InteractiveClaudeClient {
    /// Maximum wall-clock for a single interactive planning turn.
    const TURN_TIMEOUT: Duration = Duration::from_secs(300);
    /// How long to wait for the transcript file to materialize after exit.
    const TRANSCRIPT_SETTLE: Duration = Duration::from_millis(1500);
    /// How many times to re-spawn the interactive turn when it fails to complete
    /// cleanly (truncated/interrupted/empty). The transport is flaky enough that
    /// a single spawn is unreliable, but a clean turn lands often enough that a
    /// handful of retries drives availability to ~100%. The common case succeeds
    /// on the first attempt; only genuine failures pay the retry cost.
    const MAX_TRANSPORT_ATTEMPTS: u32 = 6;
    /// Brief pause between spawn attempts (lets transient subscription hiccups
    /// clear before the next try).
    const RETRY_BACKOFF: Duration = Duration::from_millis(1500);

    /// Billing-routing env keys scrubbed from the off-pool subscription authoring
    /// child (PRD §12). Mirrors `cc_switch::BILLING_ENV_KEYS`: any of these
    /// inherited from the parent process would force pay-as-you-go billing or
    /// redirect to a relay endpoint, defeating the subscription/OAuth surface.
    const BILLING_SCRUB_ENV: [&'static str; 4] = [
        "ANTHROPIC_API_KEY",
        "ANTHROPIC_AUTH_TOKEN",
        "ANTHROPIC_BASE_URL",
        "CLAUDE_CODE_OAUTH_TOKEN",
    ];

    pub fn new(model: &str) -> Self {
        Self {
            model: model.to_string(),
        }
    }

    /// Flatten orchestrator messages into a single prompt string (system blocks
    /// first, then the conversation), matching how the removed native client
    /// collapsed system + user content into one request.
    fn flatten_prompt(messages: &[LLMMessage]) -> String {
        let mut system = String::new();
        let mut convo = String::new();
        for m in messages {
            if m.role == "system" {
                if !system.is_empty() {
                    system.push('\n');
                }
                system.push_str(&m.content);
            } else {
                if !convo.is_empty() {
                    convo.push_str("\n\n");
                }
                convo.push_str(&m.content);
            }
        }
        if system.is_empty() {
            convo
        } else if convo.is_empty() {
            system
        } else {
            format!("{system}\n\n{convo}")
        }
    }

    /// Pick the substantive assistant response from a transcript and return its
    /// joined text plus its `stop_reason`. A single interactive turn's transcript
    /// can carry several `assistant` envelopes — a thinking-only one with no text,
    /// the real answer, and occasionally a trailing fragment (e.g. a lone `[`) —
    /// so we keep the LONGEST non-empty text (ties prefer the later one). This is
    /// robust against trailing fragments while still matching "last wins" when the
    /// last message is the substantive one. Only `Text` blocks count (thinking is
    /// excluded), matching the prior behaviour.
    fn extract_best_assistant(transcript: &str) -> Option<(String, Option<String>)> {
        let mut best: Option<(String, Option<String>)> = None;
        for line in transcript.lines() {
            let trimmed = line.trim();
            if trimmed.is_empty() {
                continue;
            }
            let Ok(ClaudeJson::Assistant { message, .. }) =
                serde_json::from_str::<ClaudeJson>(trimmed)
            else {
                continue;
            };
            let mut text = String::new();
            for item in &message.content {
                if let ClaudeContentItem::Text { text: t } = item {
                    if !text.is_empty() {
                        text.push('\n');
                    }
                    text.push_str(t);
                }
            }
            if text.is_empty() {
                continue;
            }
            let take = best.as_ref().map_or(true, |(bt, _)| text.len() >= bt.len());
            if take {
                best = Some((text, message.stop_reason.clone()));
            }
        }
        best
    }

    /// Validate a finished transcript. The substantive assistant message MUST
    /// carry a clean finish marker (`stop_reason == "end_turn"` / `"stop_sequence"`).
    /// A truncated or interrupted turn (the well-known failure mode of this
    /// transport: the captured text stops mid-JSON, or there is no assistant text
    /// at all) returns `Err` so the caller retries with a fresh spawn. The raw
    /// transcript head is logged on failure for troubleshooting.
    fn validate_turn(transcript: &str) -> anyhow::Result<LLMResponse> {
        match Self::extract_best_assistant(transcript) {
            Some((text, stop_reason)) => {
                let clean_finish = matches!(
                    stop_reason.as_deref(),
                    Some("end_turn" | "stop_sequence")
                );
                // claude intermittently reports end_turn yet emits a JSON object
                // that is missing its final closing braces, so a clean finish is
                // necessary but NOT sufficient — the structure must also close.
                let complete_json = Self::looks_structurally_complete(&text);
                if clean_finish && complete_json {
                    Ok(LLMResponse {
                        content: text,
                        usage: None,
                    })
                } else {
                    let why = if !clean_finish {
                        "turn did not finish cleanly (interrupted)"
                    } else {
                        "JSON content is structurally incomplete (truncated / unbalanced)"
                    };
                    tracing::warn!(
                        stop_reason = ?stop_reason,
                        content_len = text.len(),
                        clean_finish,
                        complete_json,
                        transcript_len = transcript.len(),
                        transcript_head = %transcript.chars().take(1200).collect::<String>(),
                        "InteractiveClaudeClient response unusable: {why}"
                    );
                    Err(anyhow::anyhow!(
                        "interactive response unusable: {why} (stop_reason={stop_reason:?})"
                    ))
                }
            }
            None => {
                tracing::warn!(
                    transcript_len = transcript.len(),
                    transcript_head = %transcript.chars().take(1200).collect::<String>(),
                    "InteractiveClaudeClient transcript had no assistant text"
                );
                Err(anyhow::anyhow!(
                    "interactive transcript had no assistant text"
                ))
            }
        }
    }

    /// For a response meant to be a single JSON value (it starts with `{` or `[`),
    /// verify the top-level structure actually closes. claude occasionally emits a
    /// JSON object missing its final closing braces while still reporting
    /// `end_turn`; this catches that so `chat()` can retry. Responses that are not
    /// JSON-leading (prose, or prose-wrapped JSON) are accepted as-is — their
    /// callers do their own extraction. Braces inside string literals are ignored.
    fn looks_structurally_complete(text: &str) -> bool {
        let t = text.trim_start();
        let starts_json = matches!(t.as_bytes().first(), Some(b'{' | b'['));
        if !starts_json {
            return true;
        }
        let mut depth: i32 = 0;
        let mut in_str = false;
        let mut escape = false;
        for &b in t.as_bytes() {
            if in_str {
                if escape {
                    escape = false;
                } else if b == b'\\' {
                    escape = true;
                } else if b == b'"' {
                    in_str = false;
                }
                continue;
            }
            match b {
                b'"' => in_str = true,
                b'{' => {
                    depth += 1;
                }
                b'[' => {
                    depth += 1;
                }
                b'}' => {
                    depth -= 1;
                    if depth == 0 {
                        return true;
                    }
                }
                b']' => {
                    depth -= 1;
                    if depth == 0 {
                        return true;
                    }
                }
                _ => {}
            }
        }
        false
    }

    /// One spawn of the genuine `claude` binary: provision a fresh isolated home,
    /// deliver the prompt, wait for the turn, read the transcript, and return the
    /// response ONLY if the turn completed cleanly (`validate_turn`). Any failure
    /// (spawn error, non-zero exit, timeout, truncated/empty turn) returns `Err`
    /// so `chat()` can retry. Always cleans up the isolated home; optionally
    /// preserves the raw transcript for offline inspection.
    async fn run_single_turn(&self, prompt: &str, attempt: u32) -> anyhow::Result<LLMResponse> {
        use std::process::Stdio;

        let working_dir = std::env::temp_dir().join("solodawn").join("planning-scratch");
        std::fs::create_dir_all(&working_dir)
            .map_err(|e| anyhow::anyhow!("Failed to create planning scratch dir: {e}"))?;

        // Provision a fresh isolated CLAUDE home for this turn.
        let home =
            crate::services::cc_switch::create_interactive_isolated_home(None, &working_dir)?;

        // Copy native credentials (+ settings) into the isolated home so the
        // genuine binary authenticates in the sandbox.
        if let Some(user_home) = dirs::home_dir() {
            let src_creds = user_home.join(".claude").join(".credentials.json");
            let dst_creds = home.home_dir.join(".credentials.json");
            if let Err(e) = std::fs::copy(&src_creds, &dst_creds) {
                crate::services::terminal::process::ProcessManager::cleanup_logical_session_home(
                    &home.home_dir,
                );
                return Err(anyhow::anyhow!(
                    "Failed to copy native Claude credentials into isolated home: {e}"
                ));
            }
            let src_settings = user_home.join(".claude").join("settings.json");
            if src_settings.exists() {
                let dst_settings = home.home_dir.join("settings.json");
                if !dst_settings.exists() {
                    let _ = std::fs::copy(&src_settings, &dst_settings);
                }
            }
        } else {
            crate::services::terminal::process::ProcessManager::cleanup_logical_session_home(
                &home.home_dir,
            );
            return Err(anyhow::anyhow!("Cannot determine home directory"));
        }

        let claude: ClaudeCode = serde_json::from_value(serde_json::json!({
            "interactive": true,
            "interactive_session_id": home.session_uuid,
            "model": self.model,
        }))
        .map_err(|e| anyhow::anyhow!("Failed to build interactive ClaudeCode config: {e}"))?;

        let (program, mut args) = claude
            .build_interactive_command_parts()
            .map_err(|e| anyhow::anyhow!("Failed to build interactive argv: {e}"))?
            .into_resolved()
            .await
            .map_err(|e| anyhow::anyhow!("Failed to resolve claude executable: {e}"))?;

        // SHORT prompts pass positionally; LARGE prompts (review with code context)
        // would overflow the Windows command-line cap (os error 206), so pipe them
        // through stdin (EOF yields the same one-turn-then-exit).
        const STDIN_PROMPT_THRESHOLD: usize = 8000;
        let deliver_via_stdin = prompt.len() > STDIN_PROMPT_THRESHOLD;
        if !deliver_via_stdin {
            args.push(prompt.to_string());
        }

        let home_dir_str = home.home_dir.to_string_lossy().to_string();
        let mut command = tokio::process::Command::new(&program);
        command
            .kill_on_drop(true)
            .stdin(if deliver_via_stdin {
                Stdio::piped()
            } else {
                Stdio::null()
            })
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .current_dir(&working_dir)
            .args(&args)
            .env("CLAUDE_CONFIG_DIR", &home_dir_str)
            .env("CLAUDE_HOME", &home_dir_str)
            .env_remove("PORT")
            .env_remove("BACKEND_PORT")
            .env_remove("FRONTEND_PORT");

        // Scrub billing-routing env so a subscription user stays off-pool.
        for key in Self::BILLING_SCRUB_ENV {
            command.env_remove(key);
        }

        tracing::debug!(
            attempt,
            model = %self.model,
            session_uuid = %home.session_uuid,
            transcript = %home.transcript_path.display(),
            "InteractiveClaudeClient single-turn run starting"
        );

        let run = async {
            let output = if deliver_via_stdin {
                use tokio::io::AsyncWriteExt;
                let mut child = command
                    .spawn()
                    .map_err(|e| anyhow::anyhow!("Failed to spawn interactive claude: {e}"))?;
                if let Some(mut child_stdin) = child.stdin.take() {
                    child_stdin
                        .write_all(prompt.as_bytes())
                        .await
                        .map_err(|e| {
                            anyhow::anyhow!("Failed to write prompt to claude stdin: {e}")
                        })?;
                    // EOF signals end-of-turn so claude processes and exits.
                    let _ = child_stdin.shutdown().await;
                }
                child
                    .wait_with_output()
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to await interactive claude: {e}"))?
            } else {
                command
                    .output()
                    .await
                    .map_err(|e| anyhow::anyhow!("Failed to spawn interactive claude: {e}"))?
            };
            Ok::<_, anyhow::Error>(output)
        };

        let result = match tokio::time::timeout(Self::TURN_TIMEOUT, run).await {
            Ok(inner) => inner,
            Err(_) => Err(anyhow::anyhow!(
                "Interactive claude single-turn run timed out after {}s",
                Self::TURN_TIMEOUT.as_secs()
            )),
        };

        let outcome = match result {
            Ok(output) => {
                if !output.status.success() {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    Err(anyhow::anyhow!(
                        "Interactive claude exited with {}: {}",
                        output.status,
                        stderr.chars().take(300).collect::<String>()
                    ))
                } else {
                    // Give the transcript a brief moment to flush, then validate it.
                    tokio::time::sleep(Self::TRANSCRIPT_SETTLE).await;
                    match tokio::fs::read_to_string(&home.transcript_path).await {
                        Ok(transcript) => Self::validate_turn(&transcript),
                        Err(e) => Err(anyhow::anyhow!(
                            "Failed to read interactive claude transcript {}: {e}",
                            home.transcript_path.display()
                        )),
                    }
                }
            }
            Err(e) => Err(e),
        };

        // Optional debug capture: preserve the raw transcript before cleanup so
        // transport failures can be inspected offline. Gated on
        // SOLODAWN_DEBUG_TRANSCRIPTS (a directory path); no-op when unset.
        if let Ok(dir) = std::env::var("SOLODAWN_DEBUG_TRANSCRIPTS") {
            if !dir.trim().is_empty() && home.transcript_path.exists() {
                let _ = std::fs::create_dir_all(dir.trim());
                let dst = std::path::Path::new(dir.trim())
                    .join(format!("{}.jsonl", home.session_uuid));
                if let Err(e) = std::fs::copy(&home.transcript_path, &dst) {
                    tracing::debug!("Failed to preserve debug transcript: {e}");
                }
            }
        }

        // Always clean up the isolated home (RB-37 secret cleanup) afterwards.
        crate::services::terminal::process::ProcessManager::cleanup_logical_session_home(
            &home.home_dir,
        );

        outcome
    }
}

#[async_trait]
impl LLMClient for InteractiveClaudeClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        let prompt = Self::flatten_prompt(&messages);

        // The interactive single-turn transport is intermittently unreliable: a
        // given spawn may return a truncated / interrupted turn (the captured
        // assistant text stops mid-output and carries no `stop_reason="end_turn"`).
        // A clean turn lands a good fraction of the time, so we retry the whole
        // spawn until ONE completes cleanly. This drives availability close to
        // 100% for BOTH the acceptance review and the orchestrator's decision
        // calls — the latter previously degraded (tasks auto-completed without
        // review, "stuck foundation" recovery) whenever a decision response came
        // back truncated. Each attempt provisions a fresh isolated home, and
        // `run_single_turn` only returns `Ok` for a cleanly finished turn.
        let mut last_err = anyhow::anyhow!("interactive transport produced no completed turn");
        for attempt in 1..=Self::MAX_TRANSPORT_ATTEMPTS {
            if attempt > 1 {
                tokio::time::sleep(Self::RETRY_BACKOFF).await;
            }
            match self.run_single_turn(&prompt, attempt).await {
                Ok(resp) => {
                    if attempt > 1 {
                        tracing::info!(
                            attempt,
                            model = %self.model,
                            "Interactive transport recovered on retry"
                        );
                    }
                    return Ok(resp);
                }
                Err(e) => {
                    tracing::warn!(
                        attempt,
                        max = Self::MAX_TRANSPORT_ATTEMPTS,
                        model = %self.model,
                        error = %e,
                        "Interactive transport attempt did not complete cleanly; retrying"
                    );
                    last_err = e;
                }
            }
        }
        tracing::error!(
            attempts = Self::MAX_TRANSPORT_ATTEMPTS,
            model = %self.model,
            error = %last_err,
            "Interactive transport exhausted all attempts without a clean turn"
        );
        Err(last_err)
    }
}

/// Try to create a single-turn interactive Claude LLM client for the Planning
/// LLM. Returns `None` if native Claude Code subscription credentials are not
/// present locally (`~/.claude/.credentials.json`).
///
/// Replaces the removed `create_claude_code_native_client`: native/subscription
/// users get planning via the genuine `claude` binary (one-turn, transcript
/// read) instead of the impersonation client — no extra API key, pool-safe,
/// ToS-clean.
pub fn create_interactive_claude_client(model: &str) -> Option<Box<dyn LLMClient>> {
    if !native_claude_credentials_present() {
        return None;
    }
    Some(Box::new(InteractiveClaudeClient::new(model)) as Box<dyn LLMClient>)
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

    // Always wrap in ResilientLLMClient, even for a single primary provider.
    // A single provider still benefits from long exponential-backoff retry on
    // transient failures (429/529/503/timeout/connect) so a multi-hour outage
    // does not cascade into MAX_CONSECUTIVE_LLM_FAILURES marking the workflow
    // failed. The previous `is_empty()` short-circuit returned a bare
    // RateLimitedClient with zero retries.
    let primary_name = format!("primary({})", config.model);
    let rps = config.rate_limit_requests_per_second;
    let primary_client: Box<dyn LLMClient> = build_single_client(config)?;

    let mut providers: Vec<(String, Box<dyn LLMClient>)> = vec![(primary_name, primary_client)];

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
        providers
            .iter()
            .map(|(n, _)| n.as_str())
            .collect::<Vec<_>>(),
    );

    Ok(Box::new(super::resilient_llm::ResilientLLMClient::new(providers)))
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
        let result =
            tokio::time::timeout(Duration::from_secs(3), client.chat(messages.clone())).await;
        assert!(
            result.is_ok(),
            "until_ready should unblock within the timeout"
        );
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
        assert_eq!(
            endpoint.api_format,
            ApiFormat::OpenAIChat,
            "openai-compatible must use OpenAI format regardless of URL content"
        );
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
        assert_eq!(
            endpoint.api_format,
            ApiFormat::OpenAIChat,
            "unknown api_type defaults to OpenAI, no URL guessing"
        );
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
        assert_eq!(
            endpoint.api_format,
            ApiFormat::OpenAIChat,
            "ZhipuAI openai-compatible should use OpenAI format"
        );
        assert_eq!(
            endpoint.url, "https://open.bigmodel.cn/api/paas/v4",
            "URL preserved, no /v1 appended"
        );

        let chat_url = endpoint.chat_endpoint();
        assert_eq!(
            chat_url,
            "https://open.bigmodel.cn/api/paas/v4/chat/completions"
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

        let endpoint = resolve_endpoint(&config.api_type, &config.base_url);
        assert_eq!(endpoint.api_format, ApiFormat::AnthropicMessages);
        // Key fix: no /v1 appended for anthropic-compatible
        assert_eq!(endpoint.url, "https://open.bigmodel.cn/api/anthropic");

        let msg_url = endpoint.chat_endpoint();
        assert_eq!(
            msg_url,
            "https://open.bigmodel.cn/api/anthropic/v1/messages"
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
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

        let cases = vec![
            ("openai", "https://api.openai.com"),
            ("openai-compatible", "https://open.bigmodel.cn/api/paas/v4"),
            ("anthropic", "https://api.anthropic.com"),
            (
                "anthropic-compatible",
                "https://open.bigmodel.cn/api/anthropic",
            ),
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
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

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
mod interactive_claude_tests {
    use super::*;

    #[test]
    fn test_flatten_prompt_system_and_user() {
        let messages = vec![
            LLMMessage {
                role: "system".to_string(),
                content: "You plan.".to_string(),
            },
            LLMMessage {
                role: "user".to_string(),
                content: "Build X.".to_string(),
            },
        ];
        let prompt = InteractiveClaudeClient::flatten_prompt(&messages);
        assert_eq!(prompt, "You plan.\n\nBuild X.");
    }

    #[test]
    fn test_flatten_prompt_multiple_system_blocks() {
        let messages = vec![
            LLMMessage {
                role: "system".to_string(),
                content: "A".to_string(),
            },
            LLMMessage {
                role: "system".to_string(),
                content: "B".to_string(),
            },
            LLMMessage {
                role: "user".to_string(),
                content: "C".to_string(),
            },
        ];
        assert_eq!(
            InteractiveClaudeClient::flatten_prompt(&messages),
            "A\nB\n\nC"
        );
    }

    #[test]
    fn test_flatten_prompt_user_only() {
        let messages = vec![LLMMessage {
            role: "user".to_string(),
            content: "hi".to_string(),
        }];
        assert_eq!(InteractiveClaudeClient::flatten_prompt(&messages), "hi");
    }

    #[test]
    fn test_extract_final_assistant_text_picks_last() {
        // Two assistant envelopes; the last one's text must win.
        let transcript = concat!(
            r#"{"type":"system","subtype":"init","session_id":"s"}"#,
            "\n",
            r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"first"}]}}"#,
            "\n",
            r#"{"type":"user","message":{"role":"user","content":[{"type":"text","text":"again"}]}}"#,
            "\n",
            r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"thinking","thinking":"hmm"},{"type":"text","text":"final answer"}]}}"#,
            "\n",
        );
        assert_eq!(
            InteractiveClaudeClient::extract_best_assistant(transcript).map(|(t, _)| t),
            Some("final answer".to_string())
        );
    }

    #[test]
    fn test_extract_final_assistant_text_joins_multiple_text_blocks() {
        let transcript = r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}}"#;
        assert_eq!(
            InteractiveClaudeClient::extract_best_assistant(transcript).map(|(t, _)| t),
            Some("line1\nline2".to_string())
        );
    }

    #[test]
    fn test_extract_final_assistant_text_none_when_no_assistant() {
        let transcript = r#"{"type":"system","subtype":"init","session_id":"s"}"#;
        assert_eq!(
            InteractiveClaudeClient::extract_best_assistant(transcript).map(|(t, _)| t),
            None
        );
    }

    #[test]
    fn test_looks_structurally_complete() {
        let c = InteractiveClaudeClient::looks_structurally_complete;
        // Complete object / array close → usable.
        assert!(c(r#"{"a":1,"b":{"c":2}}"#));
        assert!(c(r#"  [{"x":1},{"y":2}]  "#));
        // The real failure mode: a review JSON missing its final closing braces.
        assert!(!c(r#"{"total_score":56,"dimensions":{"buildability":{"score":12}"#));
        assert!(!c(r#"{"a":1,"b":["#));
        // Braces inside string literals must not be counted.
        assert!(c(r#"{"s":"a } b { c"}"#));
        assert!(c(r#"{"s":"esc \" still in string } {"}"#));
        // Non-JSON-leading content is accepted (callers extract themselves).
        assert!(c("需求已经很清晰，规格如下…"));
    }

    #[test]
    fn test_validate_turn_accepts_complete_end_turn() {
        let transcript = r#"{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"{\"ok\":true}"}]}}"#;
        let r = InteractiveClaudeClient::validate_turn(transcript).unwrap();
        assert_eq!(r.content, r#"{"ok":true}"#);
    }

    #[test]
    fn test_validate_turn_rejects_truncated_json_even_with_end_turn() {
        // end_turn, but the JSON object is missing its final closing braces.
        let transcript = r#"{"type":"assistant","message":{"role":"assistant","stop_reason":"end_turn","content":[{"type":"text","text":"{\"total_score\":56,\"dimensions\":{\"buildability\":{\"score\":12}"}]}}"#;
        assert!(InteractiveClaudeClient::validate_turn(transcript).is_err());
    }

    #[test]
    fn test_validate_turn_rejects_interrupted_turn() {
        // No stop_reason (interrupted) → retry, even though the JSON is complete.
        let transcript = r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"{\"ok\":true}"}]}}"#;
        assert!(InteractiveClaudeClient::validate_turn(transcript).is_err());
    }

    /// Manual ground-truth capture. Spawns the real `claude` binary on the
    /// subscription and hammers a review-sized prompt to observe how often the
    /// interactive transport returns a clean vs. truncated/degenerate response,
    /// and what `stop_reason` / `turn_duration` the transcript carries.
    ///
    /// Run with:
    ///   set SOLODAWN_DEBUG_TRANSCRIPTS=E:\SoloDawn\runtime-logs\transcripts
    ///   cargo test -p services --lib -- --ignored --nocapture capture_interactive_transport
    #[tokio::test]
    #[ignore = "spawns real claude on the subscription; run manually to capture transcripts"]
    async fn capture_interactive_transport() {
        let client = InteractiveClaudeClient::new("claude-sonnet-4-6");
        // A review-sized prompt: an audit rubric + a code blob + the same deeply
        // nested dimensions-JSON demand build_scoring_review_prompt uses.
        let code = "function add(a,b){return a+b}\nmodule.exports={add};\n".repeat(40);
        let prompt = format!(
            "You are a strict code auditor. Score the code below across five dimensions \
             (buildability 0-20, functional_completeness 0-25, code_quality split into \
             architecture/standards/security 0-10 each, test_quality 0-15, engineering_docs 0-10). \
             For every dimension cite concrete evidence (file + line). Be exhaustive.\n\n\
             ## Code under review\n```js\n{code}\n```\n\n\
             Respond with ONLY one raw JSON object, no markdown, no prose:\n\
             {{\"total_score\": <sum>, \"dimensions\": {{\
             \"buildability\": {{\"score\": <0-20>, \"max_score\": 20, \"details\": \"evidence\"}}, \
             \"functional_completeness\": {{\"score\": <0-25>, \"max_score\": 25, \"details\": \"evidence\"}}, \
             \"code_quality\": {{\"architecture\": {{\"score\": <0-10>, \"max_score\": 10, \"details\": \"evidence\"}}, \
             \"standards\": {{\"score\": <0-10>, \"max_score\": 10, \"details\": \"evidence\"}}, \
             \"security\": {{\"score\": <0-10>, \"max_score\": 10, \"details\": \"evidence\"}}}}, \
             \"test_quality\": {{\"score\": <0-15>, \"max_score\": 15, \"details\": \"evidence\"}}, \
             \"engineering_docs\": {{\"score\": <0-10>, \"max_score\": 10, \"details\": \"evidence\"}}}}, \
             \"fix_instructions\": \"...\"}}"
        );
        let n = 8usize;
        let mut ok = 0usize;
        for i in 0..n {
            match client
                .chat(vec![LLMMessage {
                    role: "user".to_string(),
                    content: prompt.clone(),
                }])
                .await
            {
                Ok(r) => {
                    let parses = !crate::services::orchestrator::types::AuditScoreResult::parse(
                        &r.content,
                    )
                    .parse_failed;
                    let head: String = r.content.chars().take(120).collect();
                    eprintln!(
                        "[{i}] OK len={} parses={} head={:?}",
                        r.content.len(),
                        parses,
                        head
                    );
                    if parses {
                        ok += 1;
                    }
                }
                Err(e) => eprintln!("[{i}] ERR {e}"),
            }
        }
        eprintln!("=== {ok}/{n} produced a parseable review ===");
    }

    /// PRD §12: the off-pool subscription authoring child must scrub ALL
    /// billing-routing env, not just the api-key. Lock the full set so a future
    /// edit cannot silently drop one and reintroduce the billing-leak.
    #[test]
    fn billing_scrub_env_covers_all_billing_routing_keys() {
        let keys = InteractiveClaudeClient::BILLING_SCRUB_ENV;
        for required in [
            "ANTHROPIC_API_KEY",
            "ANTHROPIC_AUTH_TOKEN",
            "ANTHROPIC_BASE_URL",
            "CLAUDE_CODE_OAUTH_TOKEN",
        ] {
            assert!(
                keys.contains(&required),
                "BILLING_SCRUB_ENV must scrub {required} (PRD §12); got {keys:?}"
            );
        }
    }
}
