//! Type assertion detection rule for TypeScript.
//!
//! Detects `as Type` and `<Type>` assertions that bypass type safety.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext, RuleConfig};

/// Detects type assertions (`as Type` and `<Type>value`) in TypeScript code.
///
/// Type assertions bypass the compiler's type checking and can hide real bugs.
/// The safe `as const` pattern is excluded from detection.
#[derive(Debug)]
pub struct TypeAssertionRule {
    as_pattern: Regex,
    angle_pattern: Regex,
    comment_pattern: Regex,
}

impl Default for TypeAssertionRule {
    fn default() -> Self {
        Self {
            // Match `as SomeType` where the target type is PascalCase-like
            // (starts with an uppercase letter). This avoids false positives
            // where `as` appears adjacent to lowercase identifiers like
            // `async as` keyword sequences. `as const` is intentionally
            // excluded by the starts-with-uppercase requirement, but we
            // still filter it explicitly below for defence in depth.
            as_pattern: Regex::new(r"\bas\s+[A-Z]\w*")
                .expect("invalid as-assertion regex"),
            // Match `<SomeType>` followed by a word char or paren (value expression),
            // but not common JSX/HTML-like patterns
            angle_pattern: Regex::new(r#"<(\w+)>\s*[\w\(\[\{"']"#)
                .expect("invalid angle-bracket assertion regex"),
            comment_pattern: Regex::new(r"^\s*(?://|/\*|\*)")
                .expect("invalid comment regex"),
        }
    }
}

impl Rule for TypeAssertionRule {
    fn id(&self) -> &str {
        "ts:type-assertion"
    }

    fn name(&self) -> &str {
        "Type Assertion"
    }

    fn description(&self) -> &str {
        "Detects type assertions (as Type and <Type>) that bypass type safety"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Minor
    }

    fn default_config(&self) -> RuleConfig {
        RuleConfig::default()
    }
}

impl TsRule for TypeAssertionRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let mut issues = Vec::new();
        let is_tsx = ctx.file_path.ends_with(".tsx");

        for (i, line) in ctx.lines.iter().enumerate() {
            let trimmed = line.trim();

            // Skip comment lines
            if self.comment_pattern.is_match(line) {
                continue;
            }

            // Skip import/export rename lines (`import { X as Y }`, `export { X as Y }`)
            if trimmed.starts_with("import ") || trimmed.starts_with("export ") {
                continue;
            }

            // Detect `as Type` assertions (excluding `as const`)
            for m in self.as_pattern.find_iter(line) {
                let matched = m.as_str();
                // Skip safe `as const` pattern
                if matched.trim().ends_with("const") {
                    continue;
                }
                let issue = QualityIssue::new(
                    "ts:type-assertion",
                    RuleType::CodeSmell,
                    Severity::Minor,
                    crate::rule::AnalyzerSource::Other("built-in".into()),
                    format!(
                        "Type assertion '{}' bypasses type safety; consider using type guards instead",
                        matched.trim()
                    ),
                )
                .with_location(ctx.file_path.to_string(), (i as u32) + 1);
                issues.push(issue);
            }

            // Detect angle-bracket assertions `<Type>value`, but skip .tsx files
            // because `<Tag>` in TSX is JSX, not a type assertion.
            if !is_tsx {
                for caps in self.angle_pattern.captures_iter(line) {
                    let tag = caps.get(1).map(|m| m.as_str()).unwrap_or("");
                    // Skip common HTML/JSX tag names that are unlikely to be type assertions
                    // (extra safety even for .ts files)
                    if matches!(
                        tag,
                        "div" | "span" | "p" | "a" | "br" | "hr" | "img" | "input"
                    ) {
                        continue;
                    }
                    let issue = QualityIssue::new(
                        "ts:type-assertion",
                        RuleType::CodeSmell,
                        Severity::Minor,
                        crate::rule::AnalyzerSource::Other("built-in".into()),
                        format!(
                            "Angle-bracket type assertion '<{}>' bypasses type safety; prefer 'as' syntax or type guards",
                            tag
                        ),
                    )
                    .with_location(ctx.file_path.to_string(), (i as u32) + 1);
                    issues.push(issue);
                }
            }
        }

        issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn make_context<'a>(
        file_path: &'a str,
        content: &'a str,
        lines: &'a [&'a str],
        config: &'a RuleConfig,
    ) -> TsAnalysisContext<'a> {
        TsAnalysisContext {
            file_path,
            content,
            lines,
            config,
        }
    }

    #[test]
    fn detects_as_type_assertion() {
        let src = r#"
const x = someValue as string;
const y = foo as number;
const z = bar as const;
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("test.ts", src, &lines, &config);
        let rule = TypeAssertionRule::default();
        let issues = rule.analyze(&ctx);
        // Should detect `as string` and `as number`, but NOT `as const`
        assert_eq!(issues.len(), 2, "expected 2 issues, got {}: {:?}",
            issues.len(), issues.iter().map(|i| &i.message).collect::<Vec<_>>());
        assert!(issues[0].message.contains("as string"));
        assert!(issues[1].message.contains("as number"));
    }

    #[test]
    fn detects_angle_bracket_assertion_in_ts_files() {
        let src = r#"
const x = <string>someValue;
const y = <number>otherValue;
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("test.ts", src, &lines, &config);
        let rule = TypeAssertionRule::default();
        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 2, "expected 2 angle-bracket assertion issues");
        assert!(issues[0].message.contains("<string>"));
        assert!(issues[1].message.contains("<number>"));
    }

    #[test]
    fn skips_angle_bracket_assertions_in_tsx_files() {
        let src = r#"
const x = <string>someValue;
const y = <number>otherValue;
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("component.tsx", src, &lines, &config);
        let rule = TypeAssertionRule::default();
        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "should skip angle-bracket patterns in .tsx files");
    }

    #[test]
    fn skips_import_export_renames() {
        let src = r#"
import { useState as useStateAlias } from 'react';
export { default as MyComponent } from './MyComponent';
import type { Foo as Bar } from 'baz';
export { Something as Other };
const x = value as string;
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("test.ts", src, &lines, &config);
        let rule = TypeAssertionRule::default();
        let issues = rule.analyze(&ctx);
        // Should only detect the real assertion on `value as string`, not import/export renames
        assert_eq!(issues.len(), 1, "expected 1 issue, got {}: {:?}",
            issues.len(), issues.iter().map(|i| &i.message).collect::<Vec<_>>());
        assert!(issues[0].message.contains("as string"));
    }

    #[test]
    fn skips_comment_lines() {
        let src = r#"
// const x = value as string;
/* const y = value as number; */
* value as any
const z = real as unknown;
"#;
        let lines: Vec<&str> = src.lines().collect();
        let config = RuleConfig::default();
        let ctx = make_context("test.ts", src, &lines, &config);
        let rule = TypeAssertionRule::default();
        let issues = rule.analyze(&ctx);
        assert_eq!(issues.len(), 1, "should only detect the non-comment assertion");
        assert!(issues[0].message.contains("as unknown"));
    }
}
