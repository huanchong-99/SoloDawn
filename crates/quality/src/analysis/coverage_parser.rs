//! Coverage report parser for lcov and Cobertura XML formats.

use anyhow::{self, Context};
use regex::Regex;
use std::path::Path;

/// Parsed coverage report data.
#[derive(Debug, Clone, PartialEq)]
pub struct CoverageReport {
    /// Line coverage as a percentage (0.0–100.0).
    pub line_coverage: f64,
    /// Branch coverage as a percentage (0.0–100.0).
    pub branch_coverage: f64,
    /// Number of lines that were executed.
    pub lines_covered: u64,
    /// Total number of instrumented lines.
    pub lines_total: u64,
    /// Number of branches that were taken.
    pub branches_covered: u64,
    /// Total number of instrumented branches.
    pub branches_total: u64,
}

impl CoverageReport {
    fn compute_percentages(
        lines_covered: u64,
        lines_total: u64,
        branches_covered: u64,
        branches_total: u64,
    ) -> Self {
        let line_coverage = if lines_total == 0 {
            0.0
        } else {
            (lines_covered as f64 / lines_total as f64) * 100.0
        };
        let branch_coverage = if branches_total == 0 {
            0.0
        } else {
            (branches_covered as f64 / branches_total as f64) * 100.0
        };
        Self {
            line_coverage,
            branch_coverage,
            lines_covered,
            lines_total,
            branches_covered,
            branches_total,
        }
    }
}

/// Parse an lcov-format coverage report.
///
/// Recognized record keys (summed across all source files):
/// - `LF:<total>` — total instrumented lines
/// - `LH:<hit>` — lines hit (covered)
/// - `BRF:<total>` — total instrumented branches
/// - `BRH:<hit>` — branches hit (covered)
pub fn parse_lcov(content: &str) -> anyhow::Result<CoverageReport> {
    let mut lines_total: u64 = 0;
    let mut lines_covered: u64 = 0;
    let mut branches_total: u64 = 0;
    let mut branches_covered: u64 = 0;

    for line in content.lines() {
        let line = line.trim();
        if let Some(val) = line.strip_prefix("LF:") {
            lines_total += val
                .trim()
                .parse::<u64>()
                .context("invalid LF value in lcov")?;
        } else if let Some(val) = line.strip_prefix("LH:") {
            lines_covered += val
                .trim()
                .parse::<u64>()
                .context("invalid LH value in lcov")?;
        } else if let Some(val) = line.strip_prefix("BRF:") {
            branches_total += val
                .trim()
                .parse::<u64>()
                .context("invalid BRF value in lcov")?;
        } else if let Some(val) = line.strip_prefix("BRH:") {
            branches_covered += val
                .trim()
                .parse::<u64>()
                .context("invalid BRH value in lcov")?;
        }
    }

    Ok(CoverageReport::compute_percentages(
        lines_covered,
        lines_total,
        branches_covered,
        branches_total,
    ))
}

/// Parse a Cobertura XML coverage report (e.g. as produced by `cargo-tarpaulin`).
///
/// Extracts `line-rate` and `branch-rate` from the `<coverage>` root element.
/// These attributes are decimal fractions (0.0–1.0) and are converted to
/// percentages (0.0–100.0).
///
/// Line and branch counts are extracted from `lines-covered`, `lines-valid`,
/// `branches-covered`, and `branches-valid` attributes when present.
pub fn parse_cobertura(content: &str) -> anyhow::Result<CoverageReport> {
    // Extract the <coverage ...> opening tag (may span multiple lines).
    let coverage_re =
        Regex::new(r"(?s)<coverage\b([^>]*)>").context("failed to compile coverage regex")?;
    let caps = coverage_re
        .captures(content)
        .context("no <coverage> element found in Cobertura XML")?;
    let attrs = &caps[1];

    let line_rate = extract_attr_f64(attrs, "line-rate")
        .context("missing or invalid line-rate attribute")?;
    let branch_rate = extract_attr_f64(attrs, "branch-rate")
        .context("missing or invalid branch-rate attribute")?;

    let lines_covered = extract_attr_u64(attrs, "lines-covered").unwrap_or(0);
    let lines_total = extract_attr_u64(attrs, "lines-valid").unwrap_or(0);
    let branches_covered = extract_attr_u64(attrs, "branches-covered").unwrap_or(0);
    let branches_total = extract_attr_u64(attrs, "branches-valid").unwrap_or(0);

    Ok(CoverageReport {
        line_coverage: line_rate * 100.0,
        branch_coverage: branch_rate * 100.0,
        lines_covered,
        lines_total,
        branches_covered,
        branches_total,
    })
}

/// Auto-detect the coverage format by file extension or content, then parse.
///
/// - `.info` → lcov
/// - `.xml` → Cobertura XML
/// - Otherwise, peek at content to decide.
pub fn detect_and_parse(path: &Path) -> anyhow::Result<CoverageReport> {
    let content =
        std::fs::read_to_string(path).with_context(|| format!("reading {}", path.display()))?;

    match path.extension().and_then(|e| e.to_str()) {
        Some("info") => parse_lcov(&content),
        Some("xml") => parse_cobertura(&content),
        _ => {
            // Content-based detection: if it contains a <coverage element treat
            // it as Cobertura, otherwise try lcov.
            if content.contains("<coverage") {
                parse_cobertura(&content)
            } else {
                parse_lcov(&content)
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/// Extract a named XML attribute as `f64`.
fn extract_attr_f64(attrs: &str, name: &str) -> Option<f64> {
    let pattern = format!(r#"{}="([^"]*)""#, regex::escape(name));
    let re = Regex::new(&pattern).ok()?;
    let caps = re.captures(attrs)?;
    caps[1].parse::<f64>().ok()
}

/// Extract a named XML attribute as `u64`.
fn extract_attr_u64(attrs: &str, name: &str) -> Option<u64> {
    let pattern = format!(r#"{}="([^"]*)""#, regex::escape(name));
    let re = Regex::new(&pattern).ok()?;
    let caps = re.captures(attrs)?;
    caps[1].parse::<u64>().ok()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_lcov() {
        let lcov = "\
TN:
SF:/src/main.rs
DA:1,1
DA:2,0
DA:3,1
LF:3
LH:2
BRF:4
BRH:3
end_of_record
SF:/src/lib.rs
DA:1,1
LF:7
LH:5
BRF:6
BRH:4
end_of_record
";
        let report = parse_lcov(lcov).expect("failed to parse lcov");
        assert_eq!(report.lines_total, 10); // 3 + 7
        assert_eq!(report.lines_covered, 7); // 2 + 5
        assert_eq!(report.branches_total, 10); // 4 + 6
        assert_eq!(report.branches_covered, 7); // 3 + 4
        assert!((report.line_coverage - 70.0).abs() < 0.01);
        assert!((report.branch_coverage - 70.0).abs() < 0.01);
    }

    #[test]
    fn test_parse_cobertura() {
        let xml = r#"<?xml version="1.0" ?>
<!DOCTYPE coverage SYSTEM "http://cobertura.sourceforge.net/xml/coverage-04.dtd">
<coverage line-rate="0.85" branch-rate="0.72"
          lines-covered="170" lines-valid="200"
          branches-covered="36" branches-valid="50"
          complexity="0" version="0.1" timestamp="1234567890">
  <packages>
    <package name="mypackage" line-rate="0.85" branch-rate="0.72" complexity="0">
    </package>
  </packages>
</coverage>
"#;
        let report = parse_cobertura(xml).expect("failed to parse cobertura");
        assert!((report.line_coverage - 85.0).abs() < 0.01);
        assert!((report.branch_coverage - 72.0).abs() < 0.01);
        assert_eq!(report.lines_covered, 170);
        assert_eq!(report.lines_total, 200);
        assert_eq!(report.branches_covered, 36);
        assert_eq!(report.branches_total, 50);
    }

    #[test]
    fn test_parse_lcov_no_branches() {
        let lcov = "\
TN:
SF:/src/main.rs
LF:10
LH:8
end_of_record
";
        let report = parse_lcov(lcov).expect("failed to parse lcov without branches");
        assert_eq!(report.lines_total, 10);
        assert_eq!(report.lines_covered, 8);
        assert!((report.line_coverage - 80.0).abs() < 0.01);
        assert_eq!(report.branches_total, 0);
        assert_eq!(report.branches_covered, 0);
        assert!((report.branch_coverage - 0.0).abs() < 0.01);
    }
}
