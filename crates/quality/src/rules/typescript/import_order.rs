//! Import order rule — checks that import statements follow a consistent group ordering
//! in TypeScript/JavaScript files.

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{Rule, TsAnalysisContext, TsRule};
use regex::Regex;

/// Node.js built-in module names (without the `node:` prefix).
const NODE_BUILTINS: &[&str] = &[
    "assert", "async_hooks", "buffer", "child_process", "cluster", "console", "constants",
    "crypto", "dgram", "diagnostics_channel", "dns", "domain", "events", "fs", "http", "http2",
    "https", "inspector", "module", "net", "os", "path", "perf_hooks", "process", "punycode",
    "querystring", "readline", "repl", "stream", "string_decoder", "sys", "timers", "tls",
    "trace_events", "tty", "url", "util", "v8", "vm", "wasi", "worker_threads", "zlib",
];

/// Import group classification, ordered from lowest to highest expected position.
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
enum ImportGroup {
    /// Node.js built-in modules (`node:` prefix or bare built-in names)
    NodeBuiltin = 1,
    /// External packages (no relative path, not an alias)
    External = 2,
    /// Internal alias imports (`@/` or `~/`)
    InternalAlias = 3,
    /// Relative imports (`./` or `../`)
    Relative = 4,
}

impl std::fmt::Display for ImportGroup {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::NodeBuiltin => write!(f, "Node builtin"),
            Self::External => write!(f, "External package"),
            Self::InternalAlias => write!(f, "Internal alias"),
            Self::Relative => write!(f, "Relative import"),
        }
    }
}

/// Classify an import specifier into its group.
fn classify_import(specifier: &str) -> ImportGroup {
    // Relative imports
    if specifier.starts_with("./") || specifier.starts_with("../") {
        return ImportGroup::Relative;
    }

    // Internal aliases
    if specifier.starts_with("@/") || specifier.starts_with("~/") {
        return ImportGroup::InternalAlias;
    }

    // Node builtins: explicit `node:` prefix
    if specifier.starts_with("node:") {
        return ImportGroup::NodeBuiltin;
    }

    // Node builtins: bare name (e.g. `fs`, `path`)
    let base = specifier.split('/').next().unwrap_or(specifier);
    if NODE_BUILTINS.contains(&base) {
        return ImportGroup::NodeBuiltin;
    }

    // Everything else is an external package
    ImportGroup::External
}

/// Checks that import statements in TypeScript/JavaScript files follow the
/// canonical group order:
///
/// 1. Node built-ins (`node:fs`, `path`, etc.)
/// 2. External packages (`react`, `lodash`, etc.)
/// 3. Internal aliases (`@/utils`, `~/lib`, etc.)
/// 4. Relative imports (`./foo`, `../bar`)
///
/// A violation is reported whenever a later import belongs to an earlier group
/// than a preceding import.
#[derive(Debug)]
pub struct ImportOrderRule;

impl Default for ImportOrderRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for ImportOrderRule {
    fn id(&self) -> &str {
        "ts:import-order"
    }

    fn name(&self) -> &str {
        "Import Order"
    }

    fn description(&self) -> &str {
        "Checks that import statements follow the canonical group order: \
         Node builtins, external packages, internal aliases, relative imports"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Info
    }
}

impl TsRule for ImportOrderRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        // Match `import ... from '...'` and side-effect `import '...'`
        let re_from =
            Regex::new(r#"import\s+.*\s+from\s+['"]([^'"]+)['"]"#).expect("valid regex");
        let re_side = Regex::new(r#"import\s+['"]([^'"]+)['"]"#).expect("valid regex");

        // Collect (line_number, group) for each import
        let mut imports: Vec<(u32, ImportGroup, String)> = Vec::new();

        for (idx, line) in ctx.lines.iter().enumerate() {
            let trimmed = line.trim();
            if !trimmed.starts_with("import") {
                continue;
            }

            let specifier = re_from
                .captures(trimmed)
                .or_else(|| re_side.captures(trimmed))
                .and_then(|caps| caps.get(1))
                .map(|m| m.as_str().to_string());

            if let Some(spec) = specifier {
                let group = classify_import(&spec);
                imports.push(((idx + 1) as u32, group, spec));
            }
        }

        let mut issues = Vec::new();
        let mut max_group = None::<ImportGroup>;

        for (line_number, group, specifier) in &imports {
            if let Some(prev_max) = max_group {
                if *group < prev_max {
                    let message = format!(
                        "Import `{}` ({}) should appear before {} imports. \
                         Expected order: Node builtins > external > internal aliases > relative.",
                        specifier, group, prev_max,
                    );

                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        severity,
                        AnalyzerSource::Other("builtin".to_string()),
                        message,
                    )
                    .with_location(ctx.file_path, *line_number)
                    .with_effort(2);

                    issues.push(issue);
                }
            }

            if max_group.map_or(true, |prev| *group > prev) {
                max_group = Some(*group);
            }
        }

        issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn run_rule(source: &str) -> Vec<QualityIssue> {
        let rule = ImportOrderRule::default();
        let lines: Vec<&str> = source.lines().collect();
        let config = RuleConfig::default();
        let ctx = TsAnalysisContext {
            file_path: "src/example.ts",
            content: source,
            lines: &lines,
            config: &config,
        };
        rule.analyze(&ctx)
    }

    #[test]
    fn correct_order_produces_no_issues() {
        let source = r#"import fs from 'node:fs';
import path from 'path';
import React from 'react';
import lodash from 'lodash';
import { util } from '@/utils';
import { lib } from '~/lib';
import { foo } from './foo';
import { bar } from '../bar';
"#;
        let issues = run_rule(source);
        assert!(
            issues.is_empty(),
            "Expected no issues for correctly ordered imports, got {}",
            issues.len()
        );
    }

    #[test]
    fn detects_out_of_order_imports() {
        let source = r#"import { foo } from './foo';
import React from 'react';
"#;
        let issues = run_rule(source);
        assert_eq!(issues.len(), 1, "Expected 1 issue for out-of-order import");
        assert_eq!(issues[0].rule_id, "ts:import-order");
        assert_eq!(issues[0].line, Some(2));
    }

    #[test]
    fn detects_relative_before_alias() {
        let source = r#"import { bar } from '../bar';
import { util } from '@/utils';
"#;
        let issues = run_rule(source);
        assert_eq!(issues.len(), 1);
        assert_eq!(issues[0].line, Some(2));
    }

    #[test]
    fn rule_metadata_is_correct() {
        let rule = ImportOrderRule::default();
        assert_eq!(rule.id(), "ts:import-order");
        assert_eq!(rule.name(), "Import Order");
        assert_eq!(rule.rule_type(), RuleType::CodeSmell);
        assert_eq!(rule.default_severity(), Severity::Info);
    }
}
