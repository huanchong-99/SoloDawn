use std::fmt;

use serde::{Deserialize, Serialize};

/// Identifies which wire protocol / response format to expect.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ApiFormat {
    OpenAIChat,
    AnthropicMessages,
    Google,
}

impl fmt::Display for ApiFormat {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ApiFormat::OpenAIChat => write!(f, "OpenAIChat"),
            ApiFormat::AnthropicMessages => write!(f, "AnthropicMessages"),
            ApiFormat::Google => write!(f, "Google"),
        }
    }
}

/// The result of resolving a user-provided URL + api_type into a concrete
/// endpoint description.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ResolvedEndpoint {
    /// The URL to use — stripped of trailing slashes but otherwise unmodified.
    pub url: String,
    /// Which wire protocol to speak against this URL.
    pub api_format: ApiFormat,
}

impl ResolvedEndpoint {
    /// Returns the full chat endpoint URL by appending the protocol-specific
    /// path to the base URL.
    ///
    /// - `OpenAIChat` → `{url}/chat/completions`
    /// - `AnthropicMessages` → `{url}/v1/messages` (adds `/v1` if missing)
    /// - `Google` → `{url}` (Google uses query parameters, not path suffixes)
    ///
    /// For `AnthropicMessages`, both shapes are accepted:
    /// - `https://api.anthropic.com` → `https://api.anthropic.com/v1/messages`
    /// - `https://api.anthropic.com/v1` → `https://api.anthropic.com/v1/messages`
    ///
    /// Anthropic's wire protocol (including third-party compatible providers
    /// like Zhipu GLM) requires `/v1/messages`; appending only `/messages`
    /// yields 404 against both `api.anthropic.com` and
    /// `open.bigmodel.cn/api/anthropic`.
    pub fn chat_endpoint(&self) -> String {
        match self.api_format {
            ApiFormat::OpenAIChat => format!("{}/chat/completions", self.url),
            ApiFormat::AnthropicMessages => {
                let base = self.url.trim_end_matches('/');
                if base.ends_with("/v1") {
                    format!("{base}/messages")
                } else {
                    format!("{base}/v1/messages")
                }
            }
            ApiFormat::Google => self.url.clone(),
        }
    }
}

/// Resolve a raw user-provided base URL and an api_type string into a
/// `ResolvedEndpoint`.
///
/// **Full URL Endpoint Mode**: the URL is used exactly as the user typed it
/// (only trailing slashes are stripped).  No `/v1`, `/chat/completions`, or
/// `/messages` suffixes are appended here — those are the responsibility of
/// the HTTP client layer (`chat_endpoint()`).
///
/// The `api_type` string determines only the *protocol* (`ApiFormat`), not
/// any URL manipulation.
pub fn resolve_endpoint(api_type: &str, raw_url: &str) -> ResolvedEndpoint {
    let url = raw_url.trim_end_matches('/').to_string();
    let api_format = match api_type {
        "anthropic" | "anthropic-compatible" => ApiFormat::AnthropicMessages,
        "google" => ApiFormat::Google,
        _ => ApiFormat::OpenAIChat,
    };
    ResolvedEndpoint { url, api_format }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn resolve_openai_defaults_to_openai_chat() {
        let ep = resolve_endpoint("openai", "https://api.openai.com");
        assert_eq!(ep.url, "https://api.openai.com");
        assert_eq!(ep.api_format, ApiFormat::OpenAIChat);
    }

    #[test]
    fn resolve_anthropic_uses_anthropic_messages() {
        let ep = resolve_endpoint("anthropic", "https://api.anthropic.com");
        assert_eq!(ep.url, "https://api.anthropic.com");
        assert_eq!(ep.api_format, ApiFormat::AnthropicMessages);
    }

    #[test]
    fn resolve_anthropic_compatible_uses_anthropic_messages() {
        let ep = resolve_endpoint(
            "anthropic-compatible",
            "https://open.bigmodel.cn/api/anthropic",
        );
        assert_eq!(ep.url, "https://open.bigmodel.cn/api/anthropic");
        assert_eq!(ep.api_format, ApiFormat::AnthropicMessages);
    }

    #[test]
    fn resolve_google_uses_google_format() {
        let ep = resolve_endpoint("google", "https://generativelanguage.googleapis.com");
        assert_eq!(ep.url, "https://generativelanguage.googleapis.com");
        assert_eq!(ep.api_format, ApiFormat::Google);
    }

    #[test]
    fn resolve_strips_trailing_slash() {
        let ep = resolve_endpoint("openai-compatible", "https://example.com/api/");
        assert_eq!(ep.url, "https://example.com/api");
    }

    #[test]
    fn resolve_preserves_v1_if_user_included_it() {
        let ep = resolve_endpoint("openai-compatible", "https://example.com/v1");
        assert_eq!(ep.url, "https://example.com/v1");
        assert_eq!(ep.api_format, ApiFormat::OpenAIChat);
    }

    #[test]
    fn resolve_preserves_zhipuai_v4() {
        let ep = resolve_endpoint("openai-compatible", "https://open.bigmodel.cn/api/paas/v4");
        assert_eq!(ep.url, "https://open.bigmodel.cn/api/paas/v4");
        assert_eq!(ep.api_format, ApiFormat::OpenAIChat);
    }
}
