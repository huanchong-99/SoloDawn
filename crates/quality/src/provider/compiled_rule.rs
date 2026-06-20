//! Declarative custom-rule representation + compilation (the G3 DB-free boundary).
//!
//! A custom quality rule is **pure data** — never executable code — authored once
//! (AI-assisted, multi-agent-validated, human-confirmed) and then enforced
//! deterministically and LLM-free every gate run. See
//! `docs/quality/PRD-ai-editable-quality-rules.md` §6 (rule format) and §8.2/8.3
//! (provider + severity contract).
//!
//! This module is intentionally **DB-free**: it defines the serializable
//! [`RuleDefinition`] (which mirrors the `custom_rule` row body the DB persists)
//! and the [`CompiledRule`] the provider executes. Loading definitions from the
//! database happens later, in `crates/services`; the quality crate only ever sees
//! already-deserialized definitions and the compiled rules built from them.
//!
//! ## Phasing (D5)
//! Every rule carries a [`RuleFormat`] discriminant so both formats coexist and a
//! simple token ban never pulls the AST path:
//! - [`RuleFormat::Regex`] (P1, ships first, **zero new dependencies**) — a
//!   scoped-regex rule built on the crate's existing `regex` 1.x, modeled on
//!   `rules/typescript/console_usage.rs`.
//! - [`RuleFormat::AstGrep`] (P2, additive) — ast-grep YAML, compiled to a
//!   structural matcher. **Gated behind the OFF-by-default `ast-grep` cargo
//!   feature**: the stock build pulls zero ast-grep/tree-sitter deps and compiling
//!   this format still returns [`RuleCompileError::NotYetSupported`] exactly as
//!   before. Only a build with `--features ast-grep` parses the YAML (via
//!   [`ast_grep_config::from_yaml_string`]) into a [`CompiledAstGrepRule`].
//!
//! ## Regex sandbox (the safety property)
//! Rust `regex` 1.x is linear-time `O(m*n)` with no backtracking / lookaround /
//! backreferences, so ReDoS-by-backtracking is impossible by construction. The
//! one remaining vector is DFA memory blowup (e.g. `a{0,1000000}`); we bound it by
//! compiling **once at load** through [`RegexBuilder`] with explicit
//! [`REGEX_SIZE_LIMIT`] / [`REGEX_DFA_SIZE_LIMIT`], rejecting an oversized or
//! invalid pattern as a clear compile error (surfaced as a user-facing 400 by the
//! caller) — never at scan time.

#[cfg(feature = "ast-grep")]
use ast_grep_config::{from_yaml_string, GlobalRules, RuleConfig};
#[cfg(feature = "ast-grep")]
use ast_grep_language::SupportLang;
use regex::{Regex, RegexBuilder};
use serde::{Deserialize, Serialize};

use crate::rule::{RuleType, Severity};

/// Compiled-regex program-size cap (bytes). Bounds the NFA/DFA compiled from an
/// **untrusted** (AI/user-authored) pattern so a hostile pattern cannot blow up
/// memory at load. 1 MiB matches the PRD §6.4 sandbox spec.
pub const REGEX_SIZE_LIMIT: usize = 1 << 20;

/// Compiled lazy-DFA cache-size cap (bytes) for the same untrusted pattern. Caps
/// the additional DFA memory the matcher may use while scanning. 1 MiB per PRD
/// §6.4.
pub const REGEX_DFA_SIZE_LIMIT: usize = 1 << 20;

/// The declarative rule-format discriminant (D5).
///
/// Serialized as a lowercase token matching the `custom_rule.rule_format` CHECK
/// enum (`'regex'` | `'ast_grep'`).
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, ts_rs::TS)]
#[serde(rename_all = "snake_case")]
#[ts(export)]
pub enum RuleFormat {
    /// P1 scoped-regex format (ships first; zero new deps).
    Regex,
    /// P2 ast-grep structural format (reserved; not yet implemented).
    AstGrep,
}

/// A serializable custom-rule definition — the DB-free input to compilation.
///
/// This is the self-contained shape the quality crate consumes. The
/// `crates/services` layer assembles one of these from a `custom_rule` row: the
/// matcher fields (`pattern`, `languages`, `extensions`, glob scopes) come from
/// the JSON `rule_body`; the identity/severity fields (`rule_id`, `name`,
/// `severity`, `rule_type`) come from the sibling columns. Keeping the assembly
/// out of this crate preserves the verified G3 boundary (quality stays DB-free).
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct RuleDefinition {
    /// Stable rule id (the `custom_rule.id`, stringified). Used as the
    /// `QualityIssue::rule_id`.
    pub rule_id: String,
    /// Human-readable rule name (the `custom_rule.name`).
    pub name: String,
    /// Format discriminant (D5).
    pub rule_format: RuleFormat,
    /// The matcher pattern. For [`RuleFormat::Regex`] this is a Rust-`regex`
    /// pattern; for [`RuleFormat::AstGrep`] it is the ast-grep YAML document.
    pub pattern: String,
    /// Issue severity as authored (capped to `Major` at issue-construction time
    /// via [`crate::rule::AnalyzerSource::CustomRule`] — a rule can never
    /// self-escalate to `Blocker`; D3).
    pub severity: Severity,
    /// Issue type reported for every match.
    pub rule_type: RuleType,
    /// Message attached to every match.
    pub message: String,
    /// Target languages (informational/provenance; concrete file selection uses
    /// [`Self::extensions`]). Defaults empty.
    #[serde(default)]
    pub languages: Vec<String>,
    /// File extensions this rule applies to, **without** the leading dot
    /// (e.g. `"rs"`, `"ts"`). Empty = applies to every collected source file.
    #[serde(default)]
    pub extensions: Vec<String>,
    /// Glob patterns a file path must match to be scanned. Empty = no include
    /// filter (every file passes the include stage).
    #[serde(default)]
    pub include_globs: Vec<String>,
    /// Glob patterns that exclude a file path from scanning (applied after
    /// includes). Empty = nothing extra excluded.
    #[serde(default)]
    pub exclude_globs: Vec<String>,
}

/// A compiled, ready-to-run custom rule. Construct via [`compile`].
///
/// The [`CompiledRule::Regex`] variant owns a [`Regex`] compiled once with the
/// size limits above; the provider runs it with `find_iter` and never recompiles.
///
/// The [`CompiledRule::AstGrep`] variant exists **only** under the `ast-grep`
/// feature; with the feature off this enum has a single `Regex` variant and every
/// `match` over it is exhaustive with just that arm.
#[derive(Debug, Clone)]
pub enum CompiledRule {
    /// A compiled scoped-regex rule (P1).
    Regex(CompiledRegexRule),
    /// A compiled ast-grep structural rule (P2). Present only with the `ast-grep`
    /// feature enabled.
    #[cfg(feature = "ast-grep")]
    AstGrep(CompiledAstGrepRule),
}

/// The compiled form of a [`RuleFormat::Regex`] [`RuleDefinition`].
#[derive(Debug, Clone)]
pub struct CompiledRegexRule {
    /// Identity/severity/message metadata carried from the definition.
    pub meta: RuleMeta,
    /// File-scope filters (extensions + include/exclude globs), precompiled.
    pub scope: RuleScope,
    /// The size-limited compiled pattern.
    pub regex: Regex,
}

/// The compiled form of a [`RuleFormat::AstGrep`] [`RuleDefinition`].
///
/// Holds the parsed ast-grep [`RuleConfig`] (whose `matcher` the provider runs via
/// `find_all`) plus the resolved [`SupportLang`] used to parse each scanned source
/// file into an AST. Only built under the `ast-grep` feature.
///
/// The config is held behind an [`Arc`] for two reasons: ast-grep's
/// [`RuleConfig`]/`RuleCore` implement **neither `Clone` nor `Debug`**, and the
/// provider clones the whole rule set into a `spawn_blocking` task each scan —
/// `Arc` makes that a cheap refcount bump (mirroring the `Regex` arm's
/// `Arc`-backed clone) and lets this struct derive `Clone`. `Debug` is implemented
/// by hand below since the inner config cannot derive it.
#[cfg(feature = "ast-grep")]
#[derive(Clone)]
pub struct CompiledAstGrepRule {
    /// Identity/severity/message metadata carried from the definition.
    pub meta: RuleMeta,
    /// File-scope filters (extensions + include/exclude globs), precompiled.
    pub scope: RuleScope,
    /// The parsed ast-grep rule. Its `matcher` (a `RuleCore`) is the structural
    /// pattern the provider runs against each parsed file.
    pub config: std::sync::Arc<RuleConfig<SupportLang>>,
    /// The grammar used to parse scanned source into an AST before matching.
    pub lang: SupportLang,
}

// `RuleConfig`/`RuleCore` are not `Debug`; print the inspectable metadata instead
// of the opaque matcher so `CompiledRule` can keep its `#[derive(Debug)]`.
#[cfg(feature = "ast-grep")]
impl std::fmt::Debug for CompiledAstGrepRule {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("CompiledAstGrepRule")
            .field("meta", &self.meta)
            .field("scope", &self.scope)
            .field("lang", &self.lang)
            .field("config", &"<ast-grep matcher>")
            .finish()
    }
}

/// Identity + reporting metadata shared by every compiled-rule variant.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RuleMeta {
    /// Stable rule id (becomes `QualityIssue::rule_id`).
    pub rule_id: String,
    /// Human-readable rule name.
    pub name: String,
    /// Severity as authored (capped at issue construction).
    pub severity: Severity,
    /// Reported issue type.
    pub rule_type: RuleType,
    /// Message attached to each match.
    pub message: String,
}

/// Precompiled file-scope filters for a rule.
#[derive(Debug, Clone)]
pub struct RuleScope {
    /// Lowercased extensions (no leading dot). Empty = all files.
    pub extensions: Vec<String>,
    /// Compiled include globs. Empty = include everything.
    pub include_globs: Vec<glob::Pattern>,
    /// Compiled exclude globs.
    pub exclude_globs: Vec<glob::Pattern>,
}

impl RuleScope {
    /// Whether a (project-relative, forward-slash-normalized) path is in scope for
    /// this rule. Extension filter first, then include globs (empty = all), then
    /// exclude globs.
    pub fn matches_path(&self, rel_path: &str) -> bool {
        if !self.extensions.is_empty() {
            let ext_ok =
                extension_of(rel_path).is_some_and(|ext| self.extensions.iter().any(|e| e == &ext));
            if !ext_ok {
                return false;
            }
        }

        // `**` must be able to cross `/`, while a single `*` must not — that is
        // exactly `require_literal_separator = true` (the same option clang-sys's
        // generated globber and standard glob usage rely on).
        let opts = glob::MatchOptions {
            require_literal_separator: true,
            ..glob::MatchOptions::new()
        };

        if !self.include_globs.is_empty()
            && !self
                .include_globs
                .iter()
                .any(|p| p.matches_with(rel_path, opts))
        {
            return false;
        }

        if self
            .exclude_globs
            .iter()
            .any(|p| p.matches_with(rel_path, opts))
        {
            return false;
        }

        true
    }
}

/// Lowercased file extension (no leading dot) of a forward-slash path, if any.
fn extension_of(rel_path: &str) -> Option<String> {
    let file = rel_path.rsplit(['/', '\\']).next().unwrap_or(rel_path);
    // A leading-dot dotfile (".gitignore") has no extension here, matching
    // `std::path::Path::extension` semantics.
    let (stem, ext) = file.rsplit_once('.')?;
    if stem.is_empty() {
        return None;
    }
    Some(ext.to_ascii_lowercase())
}

/// Errors that can occur while compiling a [`RuleDefinition`] into a
/// [`CompiledRule`]. The caller surfaces these as a user-facing 400 (the rule is
/// refused persistence) — never at scan time.
#[derive(Debug, thiserror::Error)]
pub enum RuleCompileError {
    /// The regex pattern is invalid, or exceeds [`REGEX_SIZE_LIMIT`] /
    /// [`REGEX_DFA_SIZE_LIMIT`] when compiled (the untrusted-pattern bound).
    #[error("rule '{rule_id}': invalid or oversized regex pattern: {source}")]
    InvalidRegex {
        /// The offending rule's id.
        rule_id: String,
        /// The underlying `regex` build error.
        #[source]
        source: regex::Error,
    },
    /// An include/exclude glob failed to compile.
    #[error("rule '{rule_id}': invalid glob '{glob}': {source}")]
    InvalidGlob {
        /// The offending rule's id.
        rule_id: String,
        /// The pattern that failed to compile.
        glob: String,
        /// The underlying glob pattern error.
        #[source]
        source: glob::PatternError,
    },
    /// The rule uses a format that this build cannot compile yet. P2 implements
    /// [`RuleFormat::AstGrep`]; until then this is returned for that variant.
    #[error("rule '{rule_id}': rule_format {format:?} is not yet supported (P2)")]
    NotYetSupported {
        /// The offending rule's id.
        rule_id: String,
        /// The format that is not yet implemented.
        format: RuleFormat,
    },
    /// The ast-grep YAML rule body is invalid (parse/validation failure), or names
    /// an unsupported language. Surfaced as a user-facing 400 at authoring time,
    /// never at scan time. Only constructible under the `ast-grep` feature.
    #[cfg(feature = "ast-grep")]
    #[error("rule '{rule_id}': invalid ast-grep rule: {message}")]
    InvalidAstGrep {
        /// The offending rule's id.
        rule_id: String,
        /// The underlying parse/validation message from `from_yaml_string` (or a
        /// language-resolution message).
        message: String,
    },
}

/// Compile a [`RuleDefinition`] into a runnable [`CompiledRule`].
///
/// For [`RuleFormat::Regex`] the pattern is built **once** through
/// [`RegexBuilder`] with explicit [`REGEX_SIZE_LIMIT`] / [`REGEX_DFA_SIZE_LIMIT`]
/// so an untrusted pattern cannot blow up memory; an invalid or oversized pattern
/// returns [`RuleCompileError::InvalidRegex`].
///
/// For [`RuleFormat::AstGrep`] the behavior depends on the `ast-grep` feature:
/// - **feature off (default):** returns [`RuleCompileError::NotYetSupported`] —
///   identical to before P2, and no ast-grep/tree-sitter code is linked.
/// - **feature on:** parses the rule body as an ast-grep YAML document via
///   [`from_yaml_string`], takes the first [`RuleConfig`], resolves the
///   [`SupportLang`], and builds a [`CompiledAstGrepRule`]; a parse/validation or
///   language-resolution failure returns [`RuleCompileError::InvalidAstGrep`].
pub fn compile(def: &RuleDefinition) -> Result<CompiledRule, RuleCompileError> {
    match def.rule_format {
        RuleFormat::Regex => {
            let regex = RegexBuilder::new(&def.pattern)
                .size_limit(REGEX_SIZE_LIMIT)
                .dfa_size_limit(REGEX_DFA_SIZE_LIMIT)
                .build()
                .map_err(|source| RuleCompileError::InvalidRegex {
                    rule_id: def.rule_id.clone(),
                    source,
                })?;

            Ok(CompiledRule::Regex(CompiledRegexRule {
                meta: rule_meta(def),
                scope: build_scope(def)?,
                regex,
            }))
        }
        #[cfg(feature = "ast-grep")]
        RuleFormat::AstGrep => compile_ast_grep(def),
        // P2 extension point — without the `ast-grep` feature, the structural
        // format is still unsupported (zero new deps in the stock build).
        #[cfg(not(feature = "ast-grep"))]
        RuleFormat::AstGrep => Err(RuleCompileError::NotYetSupported {
            rule_id: def.rule_id.clone(),
            format: RuleFormat::AstGrep,
        }),
    }
}

/// Identity/severity/message metadata extracted from a definition.
fn rule_meta(def: &RuleDefinition) -> RuleMeta {
    RuleMeta {
        rule_id: def.rule_id.clone(),
        name: def.name.clone(),
        severity: def.severity,
        rule_type: def.rule_type,
        message: def.message.clone(),
    }
}

/// Build the precompiled [`RuleScope`] (extensions + include/exclude globs) shared
/// by every compiled-rule variant.
fn build_scope(def: &RuleDefinition) -> Result<RuleScope, RuleCompileError> {
    Ok(RuleScope {
        extensions: def
            .extensions
            .iter()
            .map(|e| e.trim_start_matches('.').to_ascii_lowercase())
            .collect(),
        include_globs: compile_globs(&def.rule_id, &def.include_globs)?,
        exclude_globs: compile_globs(&def.rule_id, &def.exclude_globs)?,
    })
}

/// Compile a [`RuleFormat::AstGrep`] definition into a [`CompiledAstGrepRule`].
///
/// `def.pattern` carries the ast-grep YAML rule document. We parse it with
/// [`from_yaml_string`] (one document → one or more [`RuleConfig`]; we take the
/// first), resolve the [`SupportLang`] from `def.languages` (falling back to the
/// rule's own `language:` via the parsed config), and pair them. Any failure is an
/// [`RuleCompileError::InvalidAstGrep`] surfaced at authoring time.
#[cfg(feature = "ast-grep")]
fn compile_ast_grep(def: &RuleDefinition) -> Result<CompiledRule, RuleCompileError> {
    let invalid = |message: String| RuleCompileError::InvalidAstGrep {
        rule_id: def.rule_id.clone(),
        message,
    };

    let configs: Vec<RuleConfig<SupportLang>> =
        from_yaml_string::<SupportLang>(&def.pattern, &GlobalRules::default())
            .map_err(|e| invalid(e.to_string()))?;

    let config = configs
        .into_iter()
        .next()
        .ok_or_else(|| invalid("ast-grep YAML contained no rule".to_string()))?;

    // The grammar to parse scanned source with: prefer the definition's declared
    // language, else the rule document's own `language:` field (the parsed
    // config's language). Both are `SupportLang`; FromStr accepts rs/ts/tsx/js
    // and the long names.
    let lang = resolve_lang(def, &config).map_err(invalid)?;

    Ok(CompiledRule::AstGrep(CompiledAstGrepRule {
        meta: rule_meta(def),
        scope: build_scope(def)?,
        config: std::sync::Arc::new(config),
        lang,
    }))
}

/// Resolve the [`SupportLang`] for an ast-grep rule: the first parseable entry in
/// `def.languages`, else the language the rule document itself declared (carried on
/// the parsed [`RuleConfig`]). Returns an error message on failure.
#[cfg(feature = "ast-grep")]
fn resolve_lang(
    def: &RuleDefinition,
    config: &RuleConfig<SupportLang>,
) -> Result<SupportLang, String> {
    for raw in &def.languages {
        if let Ok(lang) = raw.parse::<SupportLang>() {
            return Ok(lang);
        }
    }
    // No (valid) language on the definition — defer to the rule document, which
    // ast-grep already validated and stored on the config during parsing.
    Ok(config.language)
}

fn compile_globs(rule_id: &str, globs: &[String]) -> Result<Vec<glob::Pattern>, RuleCompileError> {
    globs
        .iter()
        .map(|g| {
            glob::Pattern::new(g).map_err(|source| RuleCompileError::InvalidGlob {
                rule_id: rule_id.to_string(),
                glob: g.clone(),
                source,
            })
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn regex_def(rule_id: &str, pattern: &str) -> RuleDefinition {
        RuleDefinition {
            rule_id: rule_id.to_string(),
            name: "test rule".to_string(),
            rule_format: RuleFormat::Regex,
            pattern: pattern.to_string(),
            severity: Severity::Major,
            rule_type: RuleType::CodeSmell,
            message: "matched".to_string(),
            languages: vec![],
            extensions: vec![],
            include_globs: vec![],
            exclude_globs: vec![],
        }
    }

    #[test]
    // `let-else` (not an exhaustive `match`) keeps these binds compiling with the
    // `ast-grep` feature ON (the enum then has a second variant). With the feature
    // OFF the single-variant bind is irrefutable, so the otherwise-useless `else`
    // is explicitly allowed rather than removed.
    #[cfg_attr(not(feature = "ast-grep"), allow(irrefutable_let_patterns))]
    fn compiles_valid_regex() {
        let def = regex_def("r1", r"\bdbg!\s*\(");
        let compiled = compile(&def).expect("valid regex should compile");
        let CompiledRule::Regex(r) = compiled else {
            panic!("expected Regex variant");
        };
        assert_eq!(r.meta.rule_id, "r1");
        assert!(r.regex.is_match("    dbg!(x);"));
    }

    #[test]
    fn invalid_regex_is_rejected() {
        // Unbalanced group is a syntax error.
        let def = regex_def("bad", r"console\.log(");
        let err = compile(&def).expect_err("invalid regex must be rejected");
        assert!(matches!(err, RuleCompileError::InvalidRegex { .. }));
    }

    #[test]
    fn oversized_pattern_is_rejected_by_size_limit() {
        // A huge bounded repetition compiles to an enormous program; the explicit
        // size_limit must reject it rather than letting it blow up memory. This
        // is the untrusted-pattern bound from PRD §6.4.
        let huge = format!(r"(?:{}){{0,100000}}", "a".repeat(2000));
        let def = regex_def("huge", &huge);
        let err = compile(&def).expect_err("oversized pattern must be rejected");
        assert!(
            matches!(err, RuleCompileError::InvalidRegex { .. }),
            "expected InvalidRegex from the size limit, got {err:?}"
        );
    }

    // Without the `ast-grep` feature the structural format is still unsupported and
    // compiling it must return `NotYetSupported` exactly as before P2 landed.
    #[cfg(not(feature = "ast-grep"))]
    #[test]
    fn ast_grep_is_not_yet_supported() {
        let mut def = regex_def("ag", "id: x");
        def.rule_format = RuleFormat::AstGrep;
        let err = compile(&def).expect_err("ast_grep must report NotYetSupported in P1");
        assert!(matches!(
            err,
            RuleCompileError::NotYetSupported {
                format: RuleFormat::AstGrep,
                ..
            }
        ));
    }

    #[test]
    fn invalid_glob_is_rejected() {
        let mut def = regex_def("g", "x");
        def.exclude_globs = vec!["a/**b".to_string()]; // `**` must be a whole segment
        let err = compile(&def).expect_err("invalid glob must be rejected");
        assert!(matches!(err, RuleCompileError::InvalidGlob { .. }));
    }

    #[test]
    #[cfg_attr(not(feature = "ast-grep"), allow(irrefutable_let_patterns))]
    fn scope_extension_filter() {
        let mut def = regex_def("ext", "x");
        def.extensions = vec!["rs".to_string()];
        let CompiledRule::Regex(r) = compile(&def).unwrap() else {
            panic!("expected Regex variant");
        };
        assert!(r.scope.matches_path("src/main.rs"));
        assert!(!r.scope.matches_path("src/app.ts"));
    }

    #[test]
    #[cfg_attr(not(feature = "ast-grep"), allow(irrefutable_let_patterns))]
    fn scope_exclude_glob_double_star() {
        let mut def = regex_def("excl", "x");
        def.exclude_globs = vec!["**/tests/**".to_string()];
        let CompiledRule::Regex(r) = compile(&def).unwrap() else {
            panic!("expected Regex variant");
        };
        assert!(r.scope.matches_path("src/lib.rs"));
        assert!(!r.scope.matches_path("crates/q/tests/it.rs"));
    }

    #[test]
    #[cfg_attr(not(feature = "ast-grep"), allow(irrefutable_let_patterns))]
    fn scope_single_star_does_not_cross_separator() {
        let mut def = regex_def("inc", "x");
        def.include_globs = vec!["src/*.ts".to_string()];
        let CompiledRule::Regex(r) = compile(&def).unwrap() else {
            panic!("expected Regex variant");
        };
        assert!(r.scope.matches_path("src/app.ts"));
        // `*` must not match across `/`, so a nested file is excluded.
        assert!(!r.scope.matches_path("src/nested/app.ts"));
    }

    #[test]
    fn rule_definition_round_trips_through_json() {
        // The services layer deserializes the matcher half of this shape from the
        // `custom_rule.rule_body` JSON; lock the serde contract.
        let def = RuleDefinition {
            rule_id: "j1".to_string(),
            name: "json".to_string(),
            rule_format: RuleFormat::Regex,
            pattern: r"console\.log".to_string(),
            severity: Severity::Minor,
            rule_type: RuleType::CodeSmell,
            message: "no console.log".to_string(),
            languages: vec!["typescript".to_string()],
            extensions: vec!["ts".to_string()],
            include_globs: vec![],
            exclude_globs: vec!["**/*.spec.ts".to_string()],
        };
        let json = serde_json::to_string(&def).unwrap();
        let back: RuleDefinition = serde_json::from_str(&json).unwrap();
        assert_eq!(def, back);
        // rule_format serializes as the snake_case token the DB CHECK expects.
        assert!(json.contains("\"regex\""));
    }
}

/// Compilation tests for the P2 ast-grep structural format. Only built when the
/// `ast-grep` feature is enabled (CI runs these under `--all-features`).
#[cfg(all(test, feature = "ast-grep"))]
mod ast_grep_tests {
    use super::*;

    /// An `AstGrep`-format definition carrying `yaml` as its rule body, scoped to
    /// the given extensions, with `languages` driving grammar resolution.
    fn ast_grep_def(rule_id: &str, yaml: &str, languages: &[&str], exts: &[&str]) -> RuleDefinition {
        RuleDefinition {
            rule_id: rule_id.to_string(),
            name: "ast-grep rule".to_string(),
            rule_format: RuleFormat::AstGrep,
            pattern: yaml.to_string(),
            severity: Severity::Major,
            rule_type: RuleType::CodeSmell,
            message: "structural match".to_string(),
            languages: languages.iter().map(|s| s.to_string()).collect(),
            extensions: exts.iter().map(|s| s.to_string()).collect(),
            include_globs: vec![],
            exclude_globs: vec![],
        }
    }

    const RUST_UNWRAP_YAML: &str = "id: no-unwrap\nlanguage: rust\nrule:\n  pattern: $A.unwrap()\n";

    #[test]
    fn compiles_valid_rust_ast_grep_rule() {
        let def = ast_grep_def("ag-rust", RUST_UNWRAP_YAML, &["rust"], &["rs"]);
        let compiled = compile(&def).expect("valid ast-grep YAML should compile");
        match compiled {
            CompiledRule::AstGrep(r) => {
                assert_eq!(r.meta.rule_id, "ag-rust");
                assert_eq!(r.lang, SupportLang::Rust);
                // Debug impl must not panic on the opaque config.
                let _ = format!("{r:?}");
            }
            CompiledRule::Regex(_) => panic!("expected AstGrep variant"),
        }
    }

    #[test]
    fn language_falls_back_to_rule_document() {
        // No `languages` on the definition → resolve from the rule's own
        // `language:` field carried on the parsed config.
        let def = ast_grep_def("ag-fallback", RUST_UNWRAP_YAML, &[], &["rs"]);
        let CompiledRule::AstGrep(r) = compile(&def).expect("compile") else {
            panic!("expected AstGrep variant");
        };
        assert_eq!(r.lang, SupportLang::Rust);
    }

    #[test]
    fn invalid_ast_grep_yaml_is_rejected() {
        // `rule:` is required; a document without it fails ast-grep validation.
        let bad = "id: broken\nlanguage: rust\n";
        let def = ast_grep_def("ag-bad", bad, &["rust"], &["rs"]);
        let err = compile(&def).expect_err("invalid ast-grep YAML must be rejected");
        assert!(
            matches!(err, RuleCompileError::InvalidAstGrep { .. }),
            "expected InvalidAstGrep, got {err:?}"
        );
    }

    #[test]
    fn malformed_yaml_is_rejected() {
        // Not even YAML-parseable → still InvalidAstGrep (never a panic).
        let def = ast_grep_def("ag-garbage", "::: not yaml :::", &["rust"], &["rs"]);
        let err = compile(&def).expect_err("malformed YAML must be rejected");
        assert!(matches!(err, RuleCompileError::InvalidAstGrep { .. }));
    }
}
