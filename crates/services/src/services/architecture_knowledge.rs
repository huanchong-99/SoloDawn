//! Architecture knowledge base.
//!
//! Syncs architecture guidance from configurable GitHub repos (builtin:
//! study8677/awesome-architecture, MIT) into `architecture_entry` rows and
//! builds the "Architecture Guidance" prompt section injected into workflow
//! goals at materialization: a self-answered methodology checklist (adapted
//! from study8677/architecture-copilot, MIT) plus digests of the reference
//! templates that best match the requirement text.
//!
//! Every operation here is fail-open: sync errors are recorded on the source
//! row and planning proceeds without guidance rather than blocking.

use db::models::architecture_entry::ArchitectureEntry;
use db::models::architecture_source::ArchitectureSource;
use db::models::system_settings::SystemSetting;
use serde::Deserialize;
use sqlx::SqlitePool;
use std::time::Duration;

/// Builtin knowledge source coordinates.
const BUILTIN_OWNER: &str = "study8677";
const BUILTIN_REPO: &str = "awesome-architecture";
const BUILTIN_BRANCH: &str = "main";
const BUILTIN_NAME: &str = "Awesome Architecture";

/// system_settings key: "true"/"1" (or missing) enables guidance injection.
pub const SETTING_GUIDANCE_ENABLED: &str = "architecture_guidance_enabled";

/// Methodology checklist vendored from architecture-copilot (see LICENSE).
const METHODOLOGY: &str = include_str!("../../assets/architecture/methodology.md");

/// Skip upstream files larger than this (defensive; templates are ~30 KB).
const MAX_FILE_BYTES: u64 = 512 * 1024;
/// Cap for each injected template digest, in bytes (UTF-8 safe).
const DIGEST_MAX_BYTES: usize = 3000;
/// How many matched templates to inject.
const MAX_MATCHES: usize = 2;
/// Minimum match score before a template is considered relevant.
const MIN_MATCH_SCORE: u32 = 3;
/// Background loop: check every 6 h, re-sync sources older than 24 h.
const BACKGROUND_CHECK_SECS: u64 = 6 * 60 * 60;
const STALE_AFTER_SECS: i64 = 24 * 60 * 60;
/// Delay before the first background sync so server startup stays snappy.
const STARTUP_DELAY_SECS: u64 = 20;

#[derive(Debug, Deserialize)]
struct GitTreeResponse {
    sha: String,
    #[serde(default)]
    truncated: bool,
    #[serde(default)]
    tree: Vec<GitTreeNode>,
}

#[derive(Debug, Deserialize)]
struct GitTreeNode {
    path: String,
    #[serde(rename = "type")]
    node_type: String,
    sha: String,
    #[serde(default)]
    size: Option<u64>,
}

/// Result of one source sync.
#[derive(Debug, Clone, serde::Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SyncOutcome {
    pub source_id: String,
    pub tree_sha: String,
    /// True when the tree sha matched the previous sync and nothing was fetched.
    pub unchanged: bool,
    pub fetched: u32,
    pub removed: u64,
    pub total_entries: i64,
}

/// Ensure the builtin knowledge source row exists (idempotent).
pub async fn ensure_builtin_source(pool: &SqlitePool) -> anyhow::Result<()> {
    if ArchitectureSource::find_by_coords(pool, BUILTIN_OWNER, BUILTIN_REPO, BUILTIN_BRANCH)
        .await?
        .is_some()
    {
        return Ok(());
    }
    let mut source = ArchitectureSource::new(
        BUILTIN_NAME,
        BUILTIN_OWNER,
        BUILTIN_REPO,
        BUILTIN_BRANCH,
        &["templates/"],
    );
    source.builtin = true;
    ArchitectureSource::insert(pool, &source).await?;
    tracing::info!(source_id = %source.id, "Seeded builtin architecture knowledge source");
    Ok(())
}

/// Whether guidance injection is enabled. Defaults to true when the setting
/// row is absent (feature ships on; users opt out in settings).
pub async fn guidance_enabled(pool: &SqlitePool) -> bool {
    match SystemSetting::get(pool, SETTING_GUIDANCE_ENABLED).await {
        Ok(Some(v)) => v.trim().eq_ignore_ascii_case("true") || v.trim() == "1",
        Ok(None) => true,
        Err(e) => {
            tracing::warn!("Failed to read {SETTING_GUIDANCE_ENABLED}: {e}");
            true
        }
    }
}

fn http_client() -> anyhow::Result<reqwest::Client> {
    Ok(reqwest::Client::builder()
        .timeout(Duration::from_secs(30))
        .user_agent("SoloDawn-architecture-sync")
        .build()?)
}

/// Optional GitHub token for higher API rate limits.
fn github_token() -> Option<String> {
    std::env::var("SOLODAWN_GITHUB_TOKEN")
        .or_else(|_| std::env::var("GITHUB_TOKEN"))
        .ok()
        .map(|t| t.trim().to_string())
        .filter(|t| !t.is_empty())
}

/// Sync one source: list the git tree, fetch changed markdown files, upsert
/// entries, drop entries whose upstream files vanished.
pub async fn sync_source(
    pool: &SqlitePool,
    source: &ArchitectureSource,
) -> anyhow::Result<SyncOutcome> {
    let client = http_client()?;

    let tree_url = format!(
        "https://api.github.com/repos/{}/{}/git/trees/{}?recursive=1",
        source.owner, source.repo, source.branch
    );
    let mut req = client
        .get(&tree_url)
        .header("Accept", "application/vnd.github+json");
    if let Some(token) = github_token() {
        req = req.bearer_auth(token);
    }
    let resp = req.send().await?;
    if !resp.status().is_success() {
        anyhow::bail!("GitHub tree request failed: HTTP {}", resp.status());
    }
    let tree: GitTreeResponse = resp.json().await?;
    if tree.truncated {
        tracing::warn!(
            source_id = %source.id,
            "GitHub tree response truncated; sync covers only the listed files"
        );
    }

    let include_paths = source.include_path_list();
    let wanted: Vec<&GitTreeNode> = tree
        .tree
        .iter()
        .filter(|n| n.node_type == "blob")
        .filter(|n| is_wanted_path(&n.path, &include_paths))
        .filter(|n| n.size.unwrap_or(0) <= MAX_FILE_BYTES)
        .collect();
    let keep_paths: Vec<String> = wanted.iter().map(|n| n.path.clone()).collect();

    if source.last_tree_sha.as_deref() == Some(tree.sha.as_str()) {
        let total = ArchitectureEntry::count_by_source(pool, &source.id).await?;
        ArchitectureSource::record_sync(pool, &source.id, Some(&tree.sha), "ok").await?;
        return Ok(SyncOutcome {
            source_id: source.id.clone(),
            tree_sha: tree.sha,
            unchanged: true,
            fetched: 0,
            removed: 0,
            total_entries: total,
        });
    }

    let existing = ArchitectureEntry::sha_index(pool, &source.id).await?;
    let mut fetched: u32 = 0;
    for node in &wanted {
        let unchanged = existing
            .iter()
            .any(|(path, sha)| path == &node.path && sha == &node.sha);
        if unchanged {
            continue;
        }
        let raw_url = format!(
            "https://raw.githubusercontent.com/{}/{}/{}/{}",
            source.owner, source.repo, source.branch, node.path
        );
        let content = match client.get(&raw_url).send().await {
            Ok(r) if r.status().is_success() => match r.text().await {
                Ok(t) => t,
                Err(e) => {
                    tracing::warn!(path = %node.path, "Failed to read file body: {e}");
                    continue;
                }
            },
            Ok(r) => {
                tracing::warn!(path = %node.path, status = %r.status(), "Raw fetch failed");
                continue;
            }
            Err(e) => {
                tracing::warn!(path = %node.path, "Raw fetch failed: {e}");
                continue;
            }
        };
        let parsed = parse_entry(&node.path, &content);
        let entry = ArchitectureEntry::new(
            &source.id,
            &node.path,
            &parsed.category,
            &parsed.slug,
            &parsed.title,
            &parsed.keywords,
            &parsed.digest,
            &content,
            &node.sha,
        );
        ArchitectureEntry::upsert(pool, &entry).await?;
        fetched += 1;
    }

    let removed = ArchitectureEntry::delete_missing(pool, &source.id, &keep_paths).await?;
    let total = ArchitectureEntry::count_by_source(pool, &source.id).await?;
    ArchitectureSource::record_sync(pool, &source.id, Some(&tree.sha), "ok").await?;
    tracing::info!(
        source_id = %source.id,
        fetched,
        removed,
        total,
        "Architecture knowledge sync complete"
    );
    Ok(SyncOutcome {
        source_id: source.id.clone(),
        tree_sha: tree.sha,
        unchanged: false,
        fetched,
        removed,
        total_entries: total,
    })
}

/// Sync wrapper that records failures on the source row instead of bubbling.
pub async fn sync_source_recorded(pool: &SqlitePool, source: &ArchitectureSource) -> Option<SyncOutcome> {
    match sync_source(pool, source).await {
        Ok(outcome) => Some(outcome),
        Err(e) => {
            tracing::warn!(source_id = %source.id, "Architecture sync failed: {e}");
            let status = format!("error: {e}");
            let status = status.get(..status.floor_char_boundary(300)).unwrap_or(&status);
            if let Err(record_err) =
                ArchitectureSource::record_sync(pool, &source.id, None, status).await
            {
                tracing::warn!(source_id = %source.id, "Failed to record sync error: {record_err}");
            }
            None
        }
    }
}

/// Spawn the background sync loop: after a short startup delay, re-sync any
/// enabled source that has not synced successfully in the last 24 h, then
/// re-check every 6 h.
pub fn spawn_background_sync(pool: SqlitePool) {
    tokio::spawn(async move {
        tokio::time::sleep(Duration::from_secs(STARTUP_DELAY_SECS)).await;
        loop {
            match ArchitectureSource::find_enabled(&pool).await {
                Ok(sources) => {
                    for source in sources {
                        let stale = source
                            .last_synced_at
                            .map(|t| (chrono::Utc::now() - t).num_seconds() >= STALE_AFTER_SECS)
                            .unwrap_or(true);
                        let last_ok = source.last_sync_status.as_deref() == Some("ok");
                        if stale || !last_ok {
                            sync_source_recorded(&pool, &source).await;
                        }
                    }
                }
                Err(e) => tracing::warn!("Failed to list architecture sources: {e}"),
            }
            tokio::time::sleep(Duration::from_secs(BACKGROUND_CHECK_SECS)).await;
        }
    });
}

/// Build the "Architecture Guidance" section for a workflow goal, or `None`
/// when disabled. Matching failures degrade to methodology-only guidance.
pub async fn build_architecture_context(
    pool: &SqlitePool,
    requirement_text: &str,
) -> Option<String> {
    if !guidance_enabled(pool).await {
        return None;
    }

    let mut section = String::from("## Architecture Guidance\n\n");
    section.push_str(strip_attribution_header(METHODOLOGY).trim());
    section.push('\n');

    if requirement_text.trim().len() >= 10 {
        match ArchitectureEntry::find_matchable(pool).await {
            Ok(entries) => {
                for entry in top_matches(&entries, requirement_text) {
                    section.push_str(&format!(
                        "\n### Reference architecture: {} ({})\n\
                         Condensed from the matched architecture template — treat these as the \
                         decision forks and failure modes this kind of system must resolve:\n\n",
                        entry.title, entry.slug
                    ));
                    section.push_str(cap_str(&entry.digest, DIGEST_MAX_BYTES));
                    section.push('\n');
                }
            }
            Err(e) => tracing::warn!("Architecture entry matching failed: {e}"),
        }
    }

    Some(section)
}

/// Score all entries against the requirement text and return the winners.
fn top_matches<'a>(
    entries: &'a [ArchitectureEntry],
    requirement_text: &str,
) -> Vec<&'a ArchitectureEntry> {
    let haystack = requirement_text.to_lowercase();
    let mut scored: Vec<(u32, &ArchitectureEntry)> = entries
        .iter()
        .filter(|e| !e.digest.trim().is_empty())
        .map(|e| (match_score(e, &haystack), e))
        .filter(|(score, _)| *score >= MIN_MATCH_SCORE)
        .collect();
    scored.sort_by(|a, b| b.0.cmp(&a.0).then_with(|| a.1.slug.cmp(&b.1.slug)));
    scored.into_iter().take(MAX_MATCHES).map(|(_, e)| e).collect()
}

/// Deterministic keyword-overlap score. `haystack` must be lowercased.
fn match_score(entry: &ArchitectureEntry, haystack: &str) -> u32 {
    let mut score = 0u32;
    // Slug tokens ("realtime", "chat") hit English requirements.
    for token in entry.slug.split(['-', '_']) {
        if token.len() >= 3 && haystack.contains(&token.to_lowercase()) {
            score += 3;
        }
    }
    // The (usually Chinese) title hits zh requirements.
    let title = entry.title.trim().to_lowercase();
    if title.len() >= 2 && haystack.contains(&title) {
        score += 4;
    }
    // Product/tech names ("slack", "stripe") are the most distinctive
    // signal a requirement can carry, so one hit alone clears MIN_MATCH_SCORE.
    for keyword in entry.keyword_list() {
        let kw = keyword.trim().to_lowercase();
        if kw.len() >= 2 && haystack.contains(&kw) {
            score += 3;
        }
    }
    score
}

struct ParsedEntry {
    category: String,
    slug: String,
    title: String,
    keywords: Vec<String>,
    digest: String,
}

/// Whether a tree path should be synced: inside an include prefix, markdown,
/// not an underscore-prefixed spec file, and not the include root's own index.
fn is_wanted_path(path: &str, include_paths: &[String]) -> bool {
    if !path.to_lowercase().ends_with(".md") {
        return false;
    }
    let basename = path.rsplit('/').next().unwrap_or(path);
    if basename.starts_with('_') {
        return false;
    }
    include_paths.iter().any(|prefix| {
        let prefix = prefix.trim_start_matches('/');
        let prefix = prefix.trim_end_matches('/');
        if prefix.is_empty() {
            return false;
        }
        // "<prefix>/README.md" is the human index of the directory, skip it.
        path.strip_prefix(&format!("{prefix}/"))
            .is_some_and(|rest| rest != "README.md")
    })
}

fn parse_entry(path: &str, content: &str) -> ParsedEntry {
    let category = derive_category(path);
    let slug = derive_slug(path);
    let title = extract_title(content).unwrap_or_else(|| slug.clone());
    let keywords = extract_keywords(&slug, &title, content);
    let digest = extract_digest(content);
    ParsedEntry {
        category,
        slug,
        title,
        keywords,
        digest,
    }
}

fn derive_category(path: &str) -> String {
    let lowered = path.to_lowercase();
    if lowered.contains("template") {
        "template".to_string()
    } else if lowered.contains("tutorial") {
        "tutorial".to_string()
    } else if lowered.contains("case") {
        "case".to_string()
    } else {
        "other".to_string()
    }
}

/// `templates/realtime-chat/README.md` → `realtime-chat`; `tutorial/01-intro.md` → `01-intro`.
fn derive_slug(path: &str) -> String {
    let segments: Vec<&str> = path.split('/').filter(|s| !s.is_empty()).collect();
    match segments.as_slice() {
        [.., dir, file] if file.eq_ignore_ascii_case("readme.md") => (*dir).to_string(),
        [.., file] => file.trim_end_matches(".md").trim_end_matches(".MD").to_string(),
        [] => path.to_string(),
    }
}

fn extract_title(content: &str) -> Option<String> {
    content.lines().find_map(|line| {
        let trimmed = line.trim();
        trimmed.strip_prefix("# ").map(|t| {
            t.trim()
                .trim_start_matches(|c: char| !c.is_alphanumeric() && !is_cjk(c))
                .trim()
                .to_string()
        })
    })
}

fn is_cjk(c: char) -> bool {
    matches!(c, '\u{4E00}'..='\u{9FFF}' | '\u{3400}'..='\u{4DBF}')
}

/// Keywords for requirement matching: slug tokens plus Latin product/tech
/// names appearing near the top of the document (e.g. "WhatsApp", "Slack").
fn extract_keywords(slug: &str, title: &str, content: &str) -> Vec<String> {
    let mut keywords: Vec<String> = Vec::new();
    fn push(keywords: &mut Vec<String>, kw: String) {
        let kw = kw.trim().to_string();
        if kw.len() >= 2 && !keywords.iter().any(|k| k.eq_ignore_ascii_case(&kw)) {
            keywords.push(kw);
        }
    }

    for token in slug.split(['-', '_']) {
        if token.len() >= 3 {
            push(&mut keywords, token.to_lowercase());
        }
    }
    push(&mut keywords, title.to_string());

    // Latin names near the top of the doc: product references like
    // "WhatsApp、Slack、微信" or "Stripe / PayPal".
    let head_end = content.floor_char_boundary(800.min(content.len()));
    let head = &content[..head_end];
    for candidate in head.split(|c: char| !c.is_ascii_alphanumeric() && c != '.') {
        let word = candidate.trim_matches('.');
        if (4..=20).contains(&word.len())
            && word.chars().next().is_some_and(|c| c.is_ascii_uppercase())
            && word.chars().all(|c| c.is_ascii_alphanumeric() || c == '.')
        {
            push(&mut keywords, word.to_lowercase());
        }
        if keywords.len() >= 16 {
            break;
        }
    }
    keywords.truncate(16);
    keywords
}

/// Heading keywords that mark the highest-value sections of a template
/// (key decisions & trade-offs, scaling & bottlenecks, anti-patterns).
const DIGEST_HEADING_KEYWORDS: [&str; 12] = [
    "关键决策",
    "权衡",
    "规模化",
    "瓶颈",
    "误区",
    "反模式",
    "key decision",
    "trade-off",
    "tradeoff",
    "scal",
    "bottleneck",
    "anti-pattern",
];

/// Extract the digest: the sections whose headings match
/// [`DIGEST_HEADING_KEYWORDS`], else the document head as fallback.
fn extract_digest(content: &str) -> String {
    let mut digest = String::new();
    let mut capturing = false;
    for line in content.lines() {
        let trimmed = line.trim_start();
        let heading_level = trimmed.chars().take_while(|c| *c == '#').count();
        if (2..=3).contains(&heading_level) {
            let lowered = trimmed.to_lowercase();
            capturing = DIGEST_HEADING_KEYWORDS.iter().any(|kw| lowered.contains(kw));
            if capturing {
                digest.push_str(trimmed);
                digest.push('\n');
            }
            continue;
        }
        if heading_level == 1 {
            capturing = false;
            continue;
        }
        if capturing {
            digest.push_str(line);
            digest.push('\n');
        }
        if digest.len() > DIGEST_MAX_BYTES * 2 {
            break;
        }
    }

    if digest.trim().is_empty() {
        // Fallback: skip the title line, take the document head.
        let body: String = content
            .lines()
            .skip_while(|l| l.trim().is_empty() || l.trim_start().starts_with('#'))
            .collect::<Vec<_>>()
            .join("\n");
        return cap_str(&body, 1500).trim().to_string();
    }
    cap_str(&digest, DIGEST_MAX_BYTES).trim().to_string()
}

/// Byte-cap a string on a valid UTF-8 boundary.
fn cap_str(s: &str, max_bytes: usize) -> &str {
    if s.len() <= max_bytes {
        s
    } else {
        &s[..s.floor_char_boundary(max_bytes)]
    }
}

/// Strip the leading `<!-- ... -->` attribution comment from a vendored asset
/// so prompt payloads stay clean; the attribution lives in the file + LICENSE.
pub(crate) fn strip_attribution_header(content: &str) -> &str {
    let trimmed = content.trim_start();
    if let Some(rest) = trimmed.strip_prefix("<!--") {
        if let Some(end) = rest.find("-->") {
            return rest[end + 3..].trim_start();
        }
    }
    content
}

#[cfg(test)]
mod tests {
    use super::*;

    fn entry_with(slug: &str, title: &str, keywords: &[&str], digest: &str) -> ArchitectureEntry {
        ArchitectureEntry::new(
            "src-1",
            &format!("templates/{slug}/README.md"),
            "template",
            slug,
            title,
            &keywords.iter().map(|s| (*s).to_string()).collect::<Vec<_>>(),
            digest,
            "full content",
            "sha-1",
        )
    }

    #[test]
    fn wanted_path_filters_indexes_and_specs() {
        let includes = vec!["templates/".to_string()];
        assert!(is_wanted_path("templates/realtime-chat/README.md", &includes));
        assert!(is_wanted_path("templates/nested/deep/notes.md", &includes));
        assert!(!is_wanted_path("templates/README.md", &includes), "include-root index skipped");
        assert!(!is_wanted_path("templates/_TEMPLATE.md", &includes), "underscore spec skipped");
        assert!(!is_wanted_path("tutorial/01-intro.md", &includes), "outside include");
        assert!(!is_wanted_path("templates/realtime-chat/diagram.png", &includes));
    }

    #[test]
    fn slug_and_category_derivation() {
        assert_eq!(derive_slug("templates/realtime-chat/README.md"), "realtime-chat");
        assert_eq!(derive_slug("tutorial/01-intro.md"), "01-intro");
        assert_eq!(derive_category("templates/x/README.md"), "template");
        assert_eq!(derive_category("tutorial/01.md"), "tutorial");
        assert_eq!(derive_category("cases/saas/README.md"), "case");
        assert_eq!(derive_category("en/random.md"), "other");
    }

    #[test]
    fn title_extraction_strips_decorations() {
        assert_eq!(
            extract_title("# 🗺️ 实时通讯\n\nbody").as_deref(),
            Some("实时通讯")
        );
        assert_eq!(extract_title("no heading here"), None);
    }

    #[test]
    fn digest_prefers_key_sections() {
        let content = "# 实时通讯\n\nintro text\n\n## 架构全景图\nboxes\n\n## 关键决策与权衡\n长连接怎么扩展\n\n## 规模化与瓶颈\n群扩散\n\n## 参考\nlinks\n";
        let digest = extract_digest(content);
        assert!(digest.contains("关键决策与权衡"));
        assert!(digest.contains("规模化与瓶颈"));
        assert!(!digest.contains("架构全景图"));
        assert!(!digest.contains("links"));
    }

    #[test]
    fn digest_falls_back_to_head() {
        let content = "# Title\n\nplain body without key sections\nmore body\n";
        let digest = extract_digest(content);
        assert!(digest.contains("plain body"));
    }

    #[test]
    fn keywords_pick_up_product_names() {
        let content = "# 实时通讯\n\n> 代表产品:WhatsApp、Slack、微信\n\nbody";
        let keywords = extract_keywords("realtime-chat", "实时通讯", content);
        assert!(keywords.contains(&"realtime".to_string()));
        assert!(keywords.contains(&"chat".to_string()));
        assert!(keywords.contains(&"whatsapp".to_string()));
        assert!(keywords.contains(&"slack".to_string()));
    }

    #[test]
    fn matching_ranks_relevant_templates() {
        let chat = entry_with(
            "realtime-chat",
            "实时通讯",
            &["whatsapp", "slack", "实时通讯"],
            "## 关键决策\n长连接",
        );
        let pay = entry_with(
            "payment-system",
            "支付系统",
            &["stripe", "支付"],
            "## 关键决策\n幂等",
        );
        let entries = vec![chat, pay];

        let zh = top_matches(&entries, &"我想做一个类似 Slack 的团队实时聊天工具".to_lowercase());
        assert_eq!(zh.len(), 1);
        assert_eq!(zh[0].slug, "realtime-chat");

        let en = top_matches(&entries, &"Build a payment system with Stripe integration".to_lowercase());
        assert_eq!(en.len(), 1);
        assert_eq!(en[0].slug, "payment-system");

        let none = top_matches(&entries, &"写一个命令行贪吃蛇小游戏".to_lowercase());
        assert!(none.is_empty());
    }

    #[test]
    fn attribution_header_is_stripped() {
        let content = "<!--\nAdapted from X\n-->\n\n# Checklist\nbody";
        assert!(strip_attribution_header(content).starts_with("# Checklist"));
        assert_eq!(strip_attribution_header("# No header"), "# No header");
    }

    #[test]
    fn cap_str_respects_char_boundaries() {
        let s = "中文字符串测试";
        let capped = cap_str(s, 7);
        assert!(capped.len() <= 7);
        assert!(s.starts_with(capped));
    }
}
