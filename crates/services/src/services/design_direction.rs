//! Design direction: resolves the design style selected for a planning round
//! and renders the "Design Direction" prompt section injected into the
//! workflow goal. Builtin presets are vendored adaptations of high-star
//! open-source design skills (see LICENSE) seeded at startup; users manage
//! additional styles through the design-styles API.

use db::models::design_style::DesignStyle;
use db::models::system_settings::SystemSetting;
use sqlx::SqlitePool;

/// system_settings key holding the fallback style slug ("" = none).
pub const SETTING_DEFAULT_STYLE: &str = "default_design_style";

/// Cap for injected style content, in bytes (largest preset is ~8 KB).
const STYLE_MAX_BYTES: usize = 9000;

struct SeedStyle {
    slug: &'static str,
    name: &'static str,
    description: &'static str,
    source_name: &'static str,
    source_url: &'static str,
    license: &'static str,
    content: &'static str,
}

/// Builtin presets. Content files carry their own attribution headers;
/// the LICENSE file carries the full third-party notices.
const SEED_STYLES: [SeedStyle; 6] = [
    SeedStyle {
        slug: "anthropic-frontend-design",
        name: "Anthropic Frontend Design",
        description: "Distinctive, production-grade UI direction from Anthropic's official frontend-design skill: deliberate palette, typography and layout choices that avoid templated AI aesthetics.",
        source_name: "anthropics/skills — frontend-design",
        source_url: "https://github.com/anthropics/skills",
        license: "Apache-2.0",
        content: include_str!("../../assets/design_styles/anthropic-frontend-design.md"),
    },
    SeedStyle {
        slug: "taste-minimalist-editorial",
        name: "Minimalist Editorial",
        description: "Premium utilitarian minimalism: warm monochrome palette, editorial serif headings, flat bento grids, muted pastel accents, near-invisible motion.",
        source_name: "Leonxlnx/taste-skill — minimalist-ui",
        source_url: "https://github.com/Leonxlnx/taste-skill",
        license: "MIT",
        content: include_str!("../../assets/design_styles/taste-minimalist-editorial.md"),
    },
    SeedStyle {
        slug: "taste-industrial-brutalist",
        name: "Industrial Brutalist",
        description: "Swiss industrial print meets tactical telemetry: rigid visible grids, extreme type-scale contrast, hazard-red accents, zero border radius, analog degradation textures.",
        source_name: "Leonxlnx/taste-skill — industrial-brutalist-ui",
        source_url: "https://github.com/Leonxlnx/taste-skill",
        license: "MIT",
        content: include_str!("../../assets/design_styles/taste-industrial-brutalist.md"),
    },
    SeedStyle {
        slug: "taste-soft-premium",
        name: "Soft Premium",
        description: "Polished high-end aesthetic: soft depth, generous whitespace, refined gradients and spring-based motion.",
        source_name: "Leonxlnx/taste-skill — soft-skill",
        source_url: "https://github.com/Leonxlnx/taste-skill",
        license: "MIT",
        content: include_str!("../../assets/design_styles/taste-soft-premium.md"),
    },
    SeedStyle {
        slug: "impeccable-design-language",
        name: "Impeccable Design Language",
        description: "Anti-pattern-driven design rules from pbakaus/impeccable: typography, color, spacing and motion constraints that keep AI-built UIs out of generic territory.",
        source_name: "pbakaus/impeccable",
        source_url: "https://github.com/pbakaus/impeccable",
        license: "Apache-2.0",
        content: include_str!("../../assets/design_styles/impeccable-design-language.md"),
    },
    SeedStyle {
        slug: "emil-design-engineering",
        name: "Emil Design Engineering",
        description: "Animation and interaction craft from Emil Kowalski's design-engineering skill: precise durations and easings, purposeful motion, disciplined restraint.",
        source_name: "emilkowalski/skills — emil-design-eng",
        source_url: "https://github.com/emilkowalski/skills",
        license: "MIT",
        content: include_str!("../../assets/design_styles/emil-design-engineering.md"),
    },
];

/// Idempotently seed / refresh the builtin presets. User `enabled` choices
/// survive re-seeding; content and attribution follow the shipped assets.
pub async fn ensure_builtin_styles(pool: &SqlitePool) -> anyhow::Result<()> {
    for seed in &SEED_STYLES {
        let content = super::architecture_knowledge::strip_attribution_header(seed.content).trim();
        DesignStyle::upsert_builtin(
            pool,
            seed.slug,
            seed.name,
            seed.description,
            content,
            seed.source_name,
            seed.source_url,
            seed.license,
        )
        .await?;
    }
    Ok(())
}

/// Resolve the effective style for a draft: the draft's own selection, else
/// the system default, else none. Disabled or missing styles resolve to
/// none (fail-open, with a warning so the drop is observable).
pub async fn resolve_style(
    pool: &SqlitePool,
    draft_style_slug: Option<&str>,
) -> Option<DesignStyle> {
    let slug = match draft_style_slug.map(str::trim).filter(|s| !s.is_empty()) {
        Some(slug) => Some(slug.to_string()),
        None => match SystemSetting::get(pool, SETTING_DEFAULT_STYLE).await {
            Ok(value) => value.map(|v| v.trim().to_string()).filter(|v| !v.is_empty()),
            Err(e) => {
                tracing::warn!("Failed to read {SETTING_DEFAULT_STYLE}: {e}");
                None
            }
        },
    }?;

    match DesignStyle::find_enabled_by_slug(pool, &slug).await {
        Ok(Some(style)) => Some(style),
        Ok(None) => {
            tracing::warn!(slug = %slug, "Selected design style missing or disabled; continuing without design direction");
            None
        }
        Err(e) => {
            tracing::warn!(slug = %slug, "Failed to resolve design style: {e}");
            None
        }
    }
}

/// Render the prompt section carried inside the workflow goal.
pub fn build_design_direction_section(style: &DesignStyle) -> String {
    let content = style.content.trim();
    let capped = if content.len() <= STYLE_MAX_BYTES {
        content
    } else {
        &content[..content.floor_char_boundary(STYLE_MAX_BYTES)]
    };
    format!(
        "## Design Direction (style: {name})\n\
         All user-facing UI built for this project must follow the design direction below. \
         Carry these constraints into every UI-related task instruction; non-UI tasks may \
         ignore this section.\n\n{capped}",
        name = style.name,
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    async fn test_pool() -> SqlitePool {
        let pool = SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE design_style (
                id TEXT PRIMARY KEY,
                slug TEXT NOT NULL UNIQUE,
                name TEXT NOT NULL,
                description TEXT NOT NULL DEFAULT '',
                content TEXT NOT NULL,
                source_name TEXT,
                source_url TEXT,
                license TEXT,
                builtin INTEGER NOT NULL DEFAULT 0,
                enabled INTEGER NOT NULL DEFAULT 1,
                created_at DATETIME NOT NULL DEFAULT (datetime('now')),
                updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();
        sqlx::query(
            r"
            CREATE TABLE system_settings (
                key TEXT PRIMARY KEY,
                value TEXT NOT NULL,
                description TEXT,
                updated_at TEXT NOT NULL DEFAULT (datetime('now'))
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();
        pool
    }

    #[tokio::test]
    async fn seeds_builtin_styles_and_preserves_enabled() {
        let pool = test_pool().await;
        ensure_builtin_styles(&pool).await.unwrap();
        let styles = DesignStyle::find_all(&pool).await.unwrap();
        assert_eq!(styles.len(), SEED_STYLES.len());
        assert!(styles.iter().all(|s| s.builtin && s.enabled));
        assert!(styles.iter().all(|s| !s.content.starts_with("<!--")));
        assert!(styles.iter().all(|s| s.license.is_some()));

        // Disable one, re-seed, the choice must survive while content refreshes.
        let target = &styles[0];
        DesignStyle::set_enabled(&pool, &target.id, false).await.unwrap();
        ensure_builtin_styles(&pool).await.unwrap();
        let after = DesignStyle::find_by_slug(&pool, &target.slug)
            .await
            .unwrap()
            .unwrap();
        assert!(!after.enabled, "re-seeding must not re-enable a disabled preset");
        assert!(after.builtin);
    }

    #[tokio::test]
    async fn resolve_precedence_draft_then_default_then_none() {
        let pool = test_pool().await;
        ensure_builtin_styles(&pool).await.unwrap();

        // No selection anywhere → none.
        assert!(resolve_style(&pool, None).await.is_none());

        // System default applies when the draft has no selection.
        SystemSetting::set(&pool, SETTING_DEFAULT_STYLE, "taste-minimalist-editorial")
            .await
            .unwrap();
        let resolved = resolve_style(&pool, None).await.unwrap();
        assert_eq!(resolved.slug, "taste-minimalist-editorial");

        // Draft selection wins over the default.
        let resolved = resolve_style(&pool, Some("emil-design-engineering")).await.unwrap();
        assert_eq!(resolved.slug, "emil-design-engineering");

        // Disabled style resolves to none.
        let style = DesignStyle::find_by_slug(&pool, "emil-design-engineering")
            .await
            .unwrap()
            .unwrap();
        DesignStyle::set_enabled(&pool, &style.id, false).await.unwrap();
        assert!(resolve_style(&pool, Some("emil-design-engineering")).await.is_none());

        // Unknown slug resolves to none.
        assert!(resolve_style(&pool, Some("no-such-style")).await.is_none());
    }

    #[test]
    fn section_carries_style_name_and_content() {
        let style = DesignStyle::new("s", "My Style", "d", "Use warm monochrome.");
        let section = build_design_direction_section(&style);
        assert!(section.starts_with("## Design Direction (style: My Style)"));
        assert!(section.contains("Use warm monochrome."));
        assert!(section.contains("non-UI tasks may"));
    }
}
