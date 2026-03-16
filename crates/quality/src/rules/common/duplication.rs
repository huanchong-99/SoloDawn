//! Code duplication rule — detects duplicated code blocks within a file using line-based hashing.

use std::collections::HashMap;

use crate::issue::QualityIssue;
use crate::rule::{AnalyzerSource, RuleType, Severity};
use crate::rules::{CommonAnalysisContext, CommonRule, Rule};

/// Detects duplicated code blocks within a single file using rolling-window line hashing.
#[derive(Debug)]
pub struct DuplicationRule;

impl Default for DuplicationRule {
    fn default() -> Self {
        Self
    }
}

impl Rule for DuplicationRule {
    fn id(&self) -> &str {
        "common:duplication"
    }

    fn name(&self) -> &str {
        "Code Duplication"
    }

    fn description(&self) -> &str {
        "Detects duplicated code blocks within a file using line-based hashing"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }
}

/// Returns true if the line looks like a comment-only line.
fn is_comment_line(line: &str) -> bool {
    let trimmed = line.trim();
    trimmed.starts_with("//")
        || trimmed.starts_with('#')
        || trimmed.starts_with("/*")
        || trimmed.starts_with('*')
        || trimmed.starts_with("--")
        || trimmed.starts_with(';')
}

/// Simple hash function for a slice of strings.
fn hash_window(lines: &[String]) -> u64 {
    use std::hash::{Hash, Hasher};
    let mut hasher = std::collections::hash_map::DefaultHasher::new();
    for line in lines {
        line.hash(&mut hasher);
    }
    hasher.finish()
}

/// Normalize source text into a list of (original_line_number, normalized_line) pairs.
/// Skips blank lines and comment-only lines; trims whitespace.
fn normalize_lines(text: &str) -> Vec<(usize, String)> {
    text.lines()
        .enumerate()
        .filter_map(|(idx, line)| {
            let trimmed = line.trim();
            if trimmed.is_empty() || is_comment_line(trimmed) {
                None
            } else {
                Some((idx + 1, trimmed.to_string()))
            }
        })
        .collect()
}

impl CommonRule for DuplicationRule {
    fn analyze(&self, ctx: &CommonAnalysisContext) -> Vec<QualityIssue> {
        if !ctx.is_text {
            return vec![];
        }

        let text = match ctx.text {
            Some(t) => t,
            None => return vec![],
        };

        let min_lines = ctx.config.get_param_usize("min_lines", 10);
        let normalized = normalize_lines(text);

        if normalized.len() < min_lines {
            return vec![];
        }

        let severity = ctx
            .config
            .severity_override
            .unwrap_or_else(|| self.default_severity());

        // Build rolling windows and track first occurrence of each hash.
        // key: hash, value: line number of first occurrence
        let mut seen: HashMap<u64, usize> = HashMap::new();
        // Track which starting lines have already been reported as duplicates.
        let mut reported: HashMap<u64, bool> = HashMap::new();
        let mut issues: Vec<QualityIssue> = Vec::new();

        let window_count = normalized.len() - min_lines + 1;
        for i in 0..window_count {
            let window: Vec<String> = normalized[i..i + min_lines]
                .iter()
                .map(|(_, line)| line.clone())
                .collect();
            let h = hash_window(&window);
            let current_line = normalized[i].0;

            if let Some(&first_line) = seen.get(&h) {
                if let std::collections::hash_map::Entry::Vacant(e) = reported.entry(h) {
                    e.insert(true);

                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        severity,
                        AnalyzerSource::Other("builtin".to_string()),
                        format!(
                            "Duplicated code block ({} lines): first at line {}, repeated at line {}",
                            min_lines, first_line, current_line
                        ),
                    )
                    .with_location(ctx.file_path, current_line as u32)
                    .with_effort(20);

                    issues.push(issue);
                }
            } else {
                seen.insert(h, current_line);
            }
        }

        issues
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::rules::RuleConfig;

    fn default_config() -> RuleConfig {
        RuleConfig::default()
    }

    #[test]
    fn no_duplication_produces_no_issues() {
        let rule = DuplicationRule::default();
        // 15 unique lines
        let content: String = (0..15)
            .map(|i| format!("let x{} = {};", i, i))
            .collect::<Vec<_>>()
            .join("\n");
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/main.rs",
            content: content.as_bytes(),
            is_text: true,
            text: Some(&content),
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Expected no issues when there is no duplication");
    }

    #[test]
    fn duplicated_block_is_detected() {
        let rule = DuplicationRule::default();
        // Create a block of 10 lines, then some unique lines, then repeat the same block.
        let block: Vec<String> = (0..10)
            .map(|i| format!("    let val = compute({});", i))
            .collect();
        let separator: Vec<String> = (0..5)
            .map(|i| format!("    let unique_{} = {};", i, i * 100))
            .collect();

        let mut lines = Vec::new();
        lines.extend(block.clone());
        lines.extend(separator);
        lines.extend(block);

        let content = lines.join("\n");
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "src/lib.rs",
            content: content.as_bytes(),
            is_text: true,
            text: Some(&content),
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(
            !issues.is_empty(),
            "Expected at least one duplication issue"
        );
        assert_eq!(issues[0].rule_id, "common:duplication");
        assert_eq!(issues[0].severity, Severity::Major);
        assert!(issues[0].message.contains("Duplicated code block"));
    }

    #[test]
    fn binary_file_is_skipped() {
        let rule = DuplicationRule::default();
        let content = b"\x00\x01\x02\x03";
        let config = default_config();

        let ctx = CommonAnalysisContext {
            file_path: "image.png",
            content: content.as_slice(),
            is_text: false,
            text: None,
            config: &config,
        };

        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Binary files should produce no issues");
    }
}
