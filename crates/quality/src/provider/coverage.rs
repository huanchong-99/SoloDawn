//! Local coverage report provider
//!
//! Parses coverage reports from well-known locations (tarpaulin, llvm-cov, lcov)
//! without requiring any external service.

use async_trait::async_trait;
use std::path::Path;
use std::time::Instant;
use tracing::{debug, info, warn};

use crate::analysis::coverage_parser;
use crate::gate::result::MeasureValue;
use crate::metrics::MetricKey;
use crate::provider::{ProviderReport, QualityProvider};

/// Well-known coverage report locations relative to the project root.
const COVERAGE_REPORT_PATHS: &[&str] = &[
    "target/tarpaulin/cobertura.xml",
    "target/llvm-cov/lcov.info",
    "coverage/lcov.info",
    "frontend/coverage/lcov.info",
];

/// A [`QualityProvider`] that discovers and parses local coverage reports.
///
/// Searches for coverage data in well-known locations produced by common Rust
/// and frontend coverage tools, then aggregates line and branch coverage
/// percentages into a [`ProviderReport`].
#[derive(Debug)]
pub struct CoverageProvider;

impl Default for CoverageProvider {
    fn default() -> Self {
        Self
    }
}

#[async_trait]
impl QualityProvider for CoverageProvider {
    fn name(&self) -> &str {
        "coverage"
    }

    fn supported_metrics(&self) -> Vec<MetricKey> {
        vec![MetricKey::LineCoverage, MetricKey::BranchCoverage]
    }

    async fn analyze(
        &self,
        project_root: &Path,
        _changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport> {
        let start = Instant::now();

        let mut total_lines_covered: u64 = 0;
        let mut total_lines_total: u64 = 0;
        let mut total_branches_covered: u64 = 0;
        let mut total_branches_total: u64 = 0;
        let mut reports_found: usize = 0;

        for relative_path in COVERAGE_REPORT_PATHS {
            let report_path = project_root.join(relative_path);
            if !report_path.exists() {
                debug!("Coverage report not found: {}", report_path.display());
                continue;
            }

            info!("Found coverage report: {}", report_path.display());

            match coverage_parser::detect_and_parse(&report_path) {
                Ok(report) => {
                    debug!(
                        "Parsed {}: line={:.1}% ({}/{}), branch={:.1}% ({}/{})",
                        relative_path,
                        report.line_coverage,
                        report.lines_covered,
                        report.lines_total,
                        report.branch_coverage,
                        report.branches_covered,
                        report.branches_total,
                    );
                    total_lines_covered += report.lines_covered;
                    total_lines_total += report.lines_total;
                    total_branches_covered += report.branches_covered;
                    total_branches_total += report.branches_total;
                    reports_found += 1;
                }
                Err(e) => {
                    warn!("Failed to parse coverage report {}: {}", report_path.display(), e);
                }
            }
        }

        let duration_ms = start.elapsed().as_millis() as u64;

        if reports_found == 0 {
            info!("No coverage reports found; returning empty report");
            return Ok(ProviderReport::success("coverage", duration_ms));
        }

        let line_coverage = if total_lines_total == 0 {
            0.0
        } else {
            (total_lines_covered as f64 / total_lines_total as f64) * 100.0
        };

        let branch_coverage = if total_branches_total == 0 {
            0.0
        } else {
            (total_branches_covered as f64 / total_branches_total as f64) * 100.0
        };

        info!(
            "Aggregated coverage from {} report(s): line={:.1}%, branch={:.1}%",
            reports_found, line_coverage, branch_coverage,
        );

        let report = ProviderReport::success("coverage", duration_ms)
            .with_metric(MetricKey::LineCoverage, MeasureValue::Float(line_coverage))
            .with_metric(MetricKey::BranchCoverage, MeasureValue::Float(branch_coverage));

        Ok(report)
    }
}
