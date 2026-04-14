use serde::{Deserialize, Serialize};

/// Identifies which wire protocol / response format to expect.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub enum ApiFormat {
    OpenAIChat,
    AnthropicMessages,
    Google,
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

/// Normalize a base URL for LLM API providers.
///
/// # Deprecation
/// Prefer [`resolve_endpoint`] which implements Full URL Endpoint Mode —
/// the URL is used as-is without appending `/v1`.
#[deprecated(
    since = "0.0.154",
    note = "Use `resolve_endpoint` instead. Full URL Endpoint Mode means no path manipulation."
)]
pub fn normalize_base_url(api_type: &str, raw_url: &str) -> String {
    let trimmed = raw_url.trim_end_matches('/');
    match api_type {
        "openai" | "anthropic" | "anthropic-compatible" => {
            if trimmed.ends_with("/v1") {
                trimmed.to_string()
            } else {
                format!("{trimmed}/v1")
            }
        }
        _ => trimmed.to_string(),
    }
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

    // ---- Legacy normalize_base_url tests (deprecated) ----

    #[allow(deprecated)]
    #[test]
    fn legacy_official_openai_gets_v1() {
        assert_eq!(
            normalize_base_url("openai", "https://api.openai.com"),
            "https://api.openai.com/v1"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_compatible_no_v1_appended() {
        assert_eq!(
            normalize_base_url("openai-compatible", "https://open.bigmodel.cn/api/paas/v4"),
            "https://open.bigmodel.cn/api/paas/v4"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_already_has_v1_not_doubled() {
        assert_eq!(
            normalize_base_url("openai", "https://api.openai.com/v1"),
            "https://api.openai.com/v1"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_trailing_slash_stripped() {
        assert_eq!(
            normalize_base_url("openai-compatible", "https://example.com/api/"),
            "https://example.com/api"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_anthropic_official_gets_v1() {
        assert_eq!(
            normalize_base_url("anthropic", "https://api.anthropic.com"),
            "https://api.anthropic.com/v1"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_anthropic_compatible_gets_v1() {
        assert_eq!(
            normalize_base_url("anthropic-compatible", "https://open.bigmodel.cn/api/anthropic"),
            "https://open.bigmodel.cn/api/anthropic/v1"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_anthropic_compatible_already_has_v1() {
        assert_eq!(
            normalize_base_url("anthropic-compatible", "https://open.bigmodel.cn/api/anthropic/v1"),
            "https://open.bigmodel.cn/api/anthropic/v1"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_empty_api_type_no_v1() {
        assert_eq!(
            normalize_base_url("", "https://example.com/api"),
            "https://example.com/api"
        );
    }

    #[allow(deprecated)]
    #[test]
    fn legacy_google_type_no_v1() {
        assert_eq!(
            normalize_base_url("google", "https://generativelanguage.googleapis.com"),
            "https://generativelanguage.googleapis.com"
        );
    }
}
