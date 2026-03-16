//! Function Length rule — flags TypeScript/JavaScript functions that exceed a configurable line limit.

use regex::Regex;

use crate::issue::QualityIssue;
use crate::rule::{RuleType, Severity};
use crate::rules::{Rule, TsRule, TsAnalysisContext, RuleConfig};

/// Checks that individual functions do not exceed a maximum number of lines.
///
/// Defaults to 50 lines. Configurable via the `max_lines` parameter.
pub struct FunctionLengthRule {
    fn_decl_re: Regex,
    arrow_fn_re: Regex,
    method_re: Regex,
}

impl Default for FunctionLengthRule {
    fn default() -> Self {
        Self {
            fn_decl_re: Regex::new(r"function\s+(\w+)").unwrap(),
            arrow_fn_re: Regex::new(r"(\w+)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[^=])\s*=>")
                .unwrap(),
            method_re: Regex::new(r"(?:async\s+)?(\w+)\s*\([^)]*\)\s*\{").unwrap(),
        }
    }
}

impl Rule for FunctionLengthRule {
    fn id(&self) -> &str {
        "ts:function-length"
    }

    fn name(&self) -> &str {
        "Function Length"
    }

    fn description(&self) -> &str {
        "Checks that functions do not exceed a maximum number of lines"
    }

    fn rule_type(&self) -> RuleType {
        RuleType::CodeSmell
    }

    fn default_severity(&self) -> Severity {
        Severity::Major
    }

    fn default_config(&self) -> RuleConfig {
        let mut config = RuleConfig::default();
        config.params.insert("max_lines".to_string(), "50".to_string());
        config
    }
}

/// Information about a detected function declaration.
struct FunctionInfo {
    name: String,
    /// 0-based line index where the function starts.
    start_line: usize,
}

impl TsRule for FunctionLengthRule {
    fn analyze(&self, ctx: &TsAnalysisContext) -> Vec<QualityIssue> {
        let max_lines = ctx.config.get_param_usize("max_lines", 50);
        let mut issues = Vec::new();

        // Collect function start positions (line index, name).
        let mut functions: Vec<FunctionInfo> = Vec::new();

        for (i, line) in ctx.lines.iter().enumerate() {
            let trimmed = line.trim();

            // Skip comments and imports
            if trimmed.starts_with("//") || trimmed.starts_with('*') || trimmed.starts_with("/*") || trimmed.starts_with("import ") {
                continue;
            }

            // Check for function declarations: `function foo(`
            if let Some(caps) = self.fn_decl_re.captures(line) {
                let name = caps.get(1).map_or("anonymous", |m| m.as_str()).to_string();
                functions.push(FunctionInfo { name, start_line: i });
                continue;
            }

            // Check for arrow functions: `const foo = (...) =>`
            if let Some(caps) = self.arrow_fn_re.captures(line) {
                let name = caps.get(1).map_or("anonymous", |m| m.as_str()).to_string();
                // Only treat as top-level arrow if it looks like a const/let/var assignment
                if trimmed.starts_with("const ")
                    || trimmed.starts_with("let ")
                    || trimmed.starts_with("var ")
                    || trimmed.starts_with("export ")
                {
                    functions.push(FunctionInfo { name, start_line: i });
                    continue;
                }
            }

            // Check for method definitions: `methodName(...) {`
            if let Some(caps) = self.method_re.captures(line) {
                let name = caps.get(1).map_or("anonymous", |m| m.as_str()).to_string();
                // Avoid matching control-flow keywords
                if !matches!(
                    name.as_str(),
                    "if" | "else" | "for" | "while" | "switch" | "catch" | "return" | "function"
                ) {
                    functions.push(FunctionInfo { name, start_line: i });
                }
            }
        }

        // For each function, track brace depth to find the closing brace, then count lines.
        for func in &functions {
            let mut depth: i32 = 0;
            let mut found_open = false;
            let mut end_line: Option<usize> = None;

            for (i, line) in ctx.lines.iter().enumerate().skip(func.start_line) {
                for ch in line.chars() {
                    if ch == '{' {
                        depth += 1;
                        found_open = true;
                    } else if ch == '}' {
                        depth -= 1;
                    }
                }
                if found_open && depth <= 0 {
                    end_line = Some(i);
                    break;
                }
            }

            if let Some(end) = end_line {
                let line_count = end - func.start_line + 1;
                if line_count > max_lines {
                    let issue = QualityIssue::new(
                        self.id(),
                        self.rule_type(),
                        self.default_severity(),
                        crate::rule::AnalyzerSource::TypeScript,
                        format!(
                            "Function '{}' has {} lines (maximum allowed is {})",
                            func.name, line_count, max_lines
                        ),
                    )
                    .with_location(ctx.file_path, (func.start_line + 1) as u32);
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

    fn make_ctx<'a>(
        content: &'a str,
        lines: &'a [&'a str],
        config: &'a RuleConfig,
    ) -> TsAnalysisContext<'a> {
        TsAnalysisContext {
            file_path: "test.ts",
            content,
            lines,
            config,
        }
    }

    #[test]
    fn short_function_no_issue() {
        let src = r#"function greet(name: string) {
    console.log("Hello");
    console.log(name);
    return true;
}"#;
        let lines: Vec<&str> = src.lines().collect();
        let mut config = RuleConfig::default();
        config.params.insert("max_lines".to_string(), "10".to_string());
        let rule = FunctionLengthRule::default();
        let ctx = make_ctx(src, &lines, &config);
        let issues = rule.analyze(&ctx);
        assert!(issues.is_empty(), "Short function should produce no issues");
    }

    #[test]
    fn long_function_creates_issue() {
        // Build a function with 20 lines (exceeds max_lines=10)
        let mut body_lines: Vec<String> = Vec::new();
        body_lines.push("function longFunction() {".to_string());
        for i in 0..18 {
            body_lines.push(format!("    const x{} = {};", i, i));
        }
        body_lines.push("}".to_string());
        let src = body_lines.join("\n");
        let lines: Vec<&str> = src.lines().collect();

        let mut config = RuleConfig::default();
        config.params.insert("max_lines".to_string(), "10".to_string());

        let rule = FunctionLengthRule::default();
        let ctx = make_ctx(&src, &lines, &config);
        let issues = rule.analyze(&ctx);

        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("longFunction"));
        assert!(issues[0].message.contains("20 lines"));
        assert_eq!(issues[0].rule_id, "ts:function-length");
    }

    #[test]
    fn arrow_function_detected() {
        let mut body_lines: Vec<String> = Vec::new();
        body_lines.push("const handler = async (req, res) => {".to_string());
        for i in 0..14 {
            body_lines.push(format!("    doSomething({});", i));
        }
        body_lines.push("}".to_string());
        let src = body_lines.join("\n");
        let lines: Vec<&str> = src.lines().collect();

        let mut config = RuleConfig::default();
        config.params.insert("max_lines".to_string(), "10".to_string());

        let rule = FunctionLengthRule::default();
        let ctx = make_ctx(&src, &lines, &config);
        let issues = rule.analyze(&ctx);

        assert_eq!(issues.len(), 1);
        assert!(issues[0].message.contains("handler"));
    }
}
