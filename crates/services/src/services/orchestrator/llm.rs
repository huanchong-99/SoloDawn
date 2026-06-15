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
            return Err(anyhow::anyhow!(
                "LLM API error: {status} - {}",
                &body[..body.len().min(200)]
            ));
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
            .await?;

        let status = response.status();
        tracing::debug!(
            status = %status,
            "Anthropic-compatible LLM response received"
        );

        if !status.is_success() {
            let body = response.text().await.unwrap_or_default();
            return Err(anyhow::anyhow!(
                "LLM API error: {status} - {}",
                &body[..body.len().min(200)]
            ));
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
    const TRANSCRIPT_SETTLE: Duration = Duration::from_millis(500);

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

    /// Parse the on-disk transcript JSONL and return the FINAL assistant
    /// message's joined text blocks. Reuses the executor's public
    /// `ClaudeJson` / `ClaudeContentItem` envelope types (the transcript bodies
    /// are byte-identical to what `ClaudeLogProcessor` parses).
    fn extract_final_assistant_text(transcript: &str) -> Option<String> {
        let mut last: Option<String> = None;
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
            if !text.is_empty() {
                last = Some(text);
            }
        }
        last
    }
}

#[async_trait]
impl LLMClient for InteractiveClaudeClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        use std::process::Stdio;

        // Scratch working dir for the single-turn run. The transcript slug is
        // derived from this dir (see cc_switch::slug_working_dir); a stable
        // scratch dir keeps the path deterministic and avoids polluting any
        // real project's transcript folder.
        let working_dir = std::env::temp_dir().join("solodawn").join("planning-scratch");
        std::fs::create_dir_all(&working_dir)
            .map_err(|e| anyhow::anyhow!("Failed to create planning scratch dir: {e}"))?;

        // Provision a fresh isolated CLAUDE home for this turn.
        let home =
            crate::services::cc_switch::create_interactive_isolated_home(None, &working_dir)?;

        // Copy native credentials (+ settings) into the isolated home so the
        // genuine binary authenticates in the sandbox. We do NOT read the token
        // ourselves — the binary consumes the copied file.
        if let Some(user_home) = dirs::home_dir() {
            let src_creds = user_home.join(".claude").join(".credentials.json");
            let dst_creds = home.home_dir.join(".credentials.json");
            if let Err(e) = std::fs::copy(&src_creds, &dst_creds) {
                // Cleanup before bailing.
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

        // Build the interactive argv. `ClaudeCode` has private fields, so
        // construct via serde (the public interactive fields carry `#[serde]`
        // attributes) rather than struct literal.
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

        // Prompt is passed positionally to interactive `claude` (single turn).
        let prompt = Self::flatten_prompt(&messages);
        args.push(prompt);

        let home_dir_str = home.home_dir.to_string_lossy().to_string();
        let mut command = tokio::process::Command::new(&program);
        command
            .kill_on_drop(true)
            // Closed stdin => non-TTY one-turn-then-exit.
            .stdin(Stdio::null())
            .stdout(Stdio::null())
            .stderr(Stdio::piped())
            .current_dir(&working_dir)
            .args(&args)
            // Isolated home as BOTH CLAUDE_CONFIG_DIR (real redirect in 2.1.177)
            // and CLAUDE_HOME (so RB-37 cleanup still recognizes the dir).
            .env("CLAUDE_CONFIG_DIR", &home_dir_str)
            .env("CLAUDE_HOME", &home_dir_str)
            // Never let an inherited api-key force pay-as-you-go billing on a
            // subscription user; the genuine binary uses the copied OAuth creds.
            .env_remove("ANTHROPIC_API_KEY")
            // R6 port-leak hygiene: strip SoloDawn dev ports from the child.
            .env_remove("PORT")
            .env_remove("BACKEND_PORT")
            .env_remove("FRONTEND_PORT");

        tracing::debug!(
            model = %self.model,
            session_uuid = %home.session_uuid,
            transcript = %home.transcript_path.display(),
            "InteractiveClaudeClient single-turn planning run starting"
        );

        let run = async {
            let output = command
                .output()
                .await
                .map_err(|e| anyhow::anyhow!("Failed to spawn interactive claude: {e}"))?;
            Ok::<_, anyhow::Error>(output)
        };

        let result = match tokio::time::timeout(Self::TURN_TIMEOUT, run).await {
            Ok(inner) => inner,
            Err(_) => Err(anyhow::anyhow!(
                "Interactive claude single-turn run timed out after {}s",
                Self::TURN_TIMEOUT.as_secs()
            )),
        };

        let chat_result = match result {
            Ok(output) => {
                if !output.status.success() {
                    let stderr = String::from_utf8_lossy(&output.stderr);
                    Err(anyhow::anyhow!(
                        "Interactive claude exited with {}: {}",
                        output.status,
                        stderr.chars().take(300).collect::<String>()
                    ))
                } else {
                    // Give the transcript a brief moment to flush, then read it.
                    tokio::time::sleep(Self::TRANSCRIPT_SETTLE).await;
                    match tokio::fs::read_to_string(&home.transcript_path).await {
                        Ok(transcript) => {
                            match Self::extract_final_assistant_text(&transcript) {
                                Some(content) => Ok(LLMResponse {
                                    content,
                                    usage: None,
                                }),
                                None => Err(anyhow::anyhow!(
                                    "Interactive claude transcript had no assistant text"
                                )),
                            }
                        }
                        Err(e) => Err(anyhow::anyhow!(
                            "Failed to read interactive claude transcript {}: {e}",
                            home.transcript_path.display()
                        )),
                    }
                }
            }
            Err(e) => Err(e),
        };

        // Always clean up the isolated home (RB-37 secret cleanup) afterwards.
        crate::services::terminal::process::ProcessManager::cleanup_logical_session_home(
            &home.home_dir,
        );

        chat_result
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

    if config.fallback_providers.is_empty() {
        build_single_client(config)
    } else {
        // Multi-provider resilient path.
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

        Ok(Box::new(super::resilient_llm::ResilientLLMClient::new(
            providers,
        )))
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
        let _ = rustls::crypto::ring::default_provider().install_default();

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
            InteractiveClaudeClient::extract_final_assistant_text(transcript),
            Some("final answer".to_string())
        );
    }

    #[test]
    fn test_extract_final_assistant_text_joins_multiple_text_blocks() {
        let transcript = r#"{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}]}}"#;
        assert_eq!(
            InteractiveClaudeClient::extract_final_assistant_text(transcript),
            Some("line1\nline2".to_string())
        );
    }

    #[test]
    fn test_extract_final_assistant_text_none_when_no_assistant() {
        let transcript = r#"{"type":"system","subtype":"init","session_id":"s"}"#;
        assert_eq!(
            InteractiveClaudeClient::extract_final_assistant_text(transcript),
            None
        );
    }
}
