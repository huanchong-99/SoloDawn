/// Normalize a base URL for LLM API providers.
///
/// For official providers (`"openai"`, `"anthropic"`), ensures the `/v1` path
/// is present. For compatible/third-party providers, uses the URL exactly as
/// configured by the user to avoid corrupting provider-specific paths (e.g.,
/// ZhipuAI uses `/v4` not `/v1`).
///
/// Trailing slashes are always stripped.
pub fn normalize_base_url(api_type: &str, raw_url: &str) -> String {
    let trimmed = raw_url.trim_end_matches('/');
    match api_type {
        // All types that use the /v1 path convention.
        "openai" | "anthropic" | "anthropic-compatible" => {
            if trimmed.ends_with("/v1") {
                trimmed.to_string()
            } else {
                format!("{trimmed}/v1")
            }
        }
        // For "openai-compatible", "google", etc. — use the URL as provided.
        _ => trimmed.to_string(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn official_openai_gets_v1() {
        assert_eq!(
            normalize_base_url("openai", "https://api.openai.com"),
            "https://api.openai.com/v1"
        );
    }

    #[test]
    fn compatible_no_v1_appended() {
        assert_eq!(
            normalize_base_url(
                "openai-compatible",
                "https://open.bigmodel.cn/api/paas/v4"
            ),
            "https://open.bigmodel.cn/api/paas/v4"
        );
    }

    #[test]
    fn already_has_v1_not_doubled() {
        assert_eq!(
            normalize_base_url("openai", "https://api.openai.com/v1"),
            "https://api.openai.com/v1"
        );
    }

    #[test]
    fn trailing_slash_stripped() {
        assert_eq!(
            normalize_base_url("openai-compatible", "https://example.com/api/"),
            "https://example.com/api"
        );
    }

    #[test]
    fn anthropic_official_gets_v1() {
        assert_eq!(
            normalize_base_url("anthropic", "https://api.anthropic.com"),
            "https://api.anthropic.com/v1"
        );
    }

    #[test]
    fn anthropic_compatible_gets_v1() {
        assert_eq!(
            normalize_base_url(
                "anthropic-compatible",
                "https://open.bigmodel.cn/api/anthropic"
            ),
            "https://open.bigmodel.cn/api/anthropic/v1"
        );
    }

    #[test]
    fn anthropic_compatible_already_has_v1() {
        assert_eq!(
            normalize_base_url(
                "anthropic-compatible",
                "https://open.bigmodel.cn/api/anthropic/v1"
            ),
            "https://open.bigmodel.cn/api/anthropic/v1"
        );
    }

    #[test]
    fn empty_api_type_no_v1() {
        assert_eq!(
            normalize_base_url("", "https://example.com/api"),
            "https://example.com/api"
        );
    }

    #[test]
    fn google_type_no_v1() {
        assert_eq!(
            normalize_base_url(
                "google",
                "https://generativelanguage.googleapis.com"
            ),
            "https://generativelanguage.googleapis.com"
        );
    }

    #[test]
    fn zhipuai_v4_preserved() {
        assert_eq!(
            normalize_base_url(
                "openai-compatible",
                "https://open.bigmodel.cn/api/paas/v4"
            ),
            "https://open.bigmodel.cn/api/paas/v4"
        );
    }
}
