//! Headless self-test module for SoloDawn server.
//!
//! Provides a `self-test` subcommand that boots the server with a temporary
//! database, exercises every API endpoint, and reports structured results.
//! Designed to run on clean CI environments (GitHub Actions) with no UI.

pub mod orchestration;
pub mod runner;
pub mod tests;

use std::time::Instant;

use serde::Serialize;

/// Result of a single test case.
#[derive(Debug, Serialize)]
pub struct TestResult {
    pub name: String,
    pub group: String,
    pub passed: bool,
    pub duration_ms: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<String>,
}

/// Aggregated report for the entire self-test run.
#[derive(Debug, Serialize)]
pub struct SelfTestReport {
    pub total: usize,
    pub passed: usize,
    pub failed: usize,
    pub duration_ms: u64,
    pub results: Vec<TestResult>,
}

/// Entry point for the self-test subcommand.
///
/// Returns exit code: 0 = all passed, 1 = failures.
pub async fn run(json: bool, filter: Option<String>, orchestration: bool) -> i32 {
    // Initialize minimal tracing — write to stderr so stdout is clean JSON
    let env_filter = tracing_subscriber::EnvFilter::try_new(
        std::env::var("RUST_LOG").unwrap_or_else(|_| "warn,server=info".to_string()),
    )
    .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("warn"));
    let _ = tracing_subscriber::fmt()
        .with_writer(std::io::stderr)
        .with_env_filter(env_filter)
        .try_init();

    eprintln!("SoloDawn Self-Test — starting server...");

    let server = match runner::TestServer::start().await {
        Ok(s) => s,
        Err(e) => {
            eprintln!("FATAL: Failed to start test server: {e}");
            return 1;
        }
    };

    eprintln!(
        "Server running on {} — executing tests...",
        server.base_url
    );

    let start = Instant::now();

    // Parse filter groups
    let filter_groups: Option<Vec<String>> = filter.map(|f| {
        f.split(',')
            .map(|s| s.trim().to_lowercase())
            .filter(|s| !s.is_empty())
            .collect()
    });

    let mut ctx = tests::TestContext::new(server.base_url.clone(), server.temp_dir());
    let mut results = tests::run_all_tests(&mut ctx, filter_groups.as_deref()).await;

    // Run orchestration E2E tests if requested
    if orchestration {
        let orch_results =
            orchestration::run_orchestration_tests(&server.base_url, &server.temp_dir()).await;
        results.extend(orch_results);
    }

    let total_duration = start.elapsed();

    let passed = results.iter().filter(|r| r.passed).count();
    let failed = results.iter().filter(|r| !r.passed).count();

    let report = SelfTestReport {
        total: results.len(),
        passed,
        failed,
        duration_ms: total_duration.as_millis() as u64,
        results,
    };

    if json {
        println!(
            "{}",
            serde_json::to_string_pretty(&report).unwrap_or_else(|_| "{}".to_string())
        );
    } else {
        print_human_report(&report);
    }

    // Shutdown
    eprintln!("Shutting down test server...");
    server.shutdown().await;

    i32::from(report.failed > 0)
}

fn print_human_report(report: &SelfTestReport) {
    eprintln!("\n{}", "=".repeat(72));
    eprintln!("  SoloDawn Self-Test Report");
    eprintln!("{}", "=".repeat(72));

    let mut current_group = String::new();
    for r in &report.results {
        if r.group != current_group {
            current_group.clone_from(&r.group);
            eprintln!("\n  [{current_group}]");
        }
        let icon = if r.passed { "PASS" } else { "FAIL" };
        let duration = format!("{}ms", r.duration_ms);
        eprintln!("    [{icon}] {:<50} {duration:>6}", r.name);
        if let Some(err) = &r.error {
            // Truncate long errors
            let short = if err.len() > 120 {
                format!("{}...", &err[..120])
            } else {
                err.clone()
            };
            eprintln!("           -> {short}");
        }
    }

    eprintln!("\n{}", "-".repeat(72));
    eprintln!(
        "  Total: {}  Passed: {}  Failed: {}  Duration: {}ms",
        report.total, report.passed, report.failed, report.duration_ms
    );
    eprintln!("{}", "=".repeat(72));
}
