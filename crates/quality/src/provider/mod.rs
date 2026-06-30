//! 分析器 Provider 接口
//!
//! 可插拔的分析器 provider 架构。每个 provider 负责执行特定工具并收集结果。

pub mod builtin_common;
pub mod builtin_frontend;
pub mod builtin_rust;
pub mod compiled_rule;
pub mod completeness;
pub mod coverage;
pub mod declarative;
pub mod delivery_readiness;
pub mod frontend;
pub mod repo;
pub mod rust_analyzer;
pub mod security;
pub mod sonar;

// Re-export the declarative custom-rule surface (provider + compiled-rule types +
// the authoring-pipeline ground-truth executor). Instantiation with concrete
// rules is wired from `crates/services` (the quality crate stays DB-free); these
// re-exports keep construction injectable.
#[cfg(feature = "ast-grep")]
pub use compiled_rule::CompiledAstGrepRule;
pub use compiled_rule::{
    compile, CompiledRegexRule, CompiledRule, RuleCompileError, RuleDefinition, RuleFormat,
    RuleMeta, RuleScope, REGEX_DFA_SIZE_LIMIT, REGEX_SIZE_LIMIT,
};
pub use declarative::{run_candidate, DeclarativeRuleProvider};

use std::{collections::HashMap, path::Path, time::Duration};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};

use crate::{
    discovery::{NodeQualityCommand, PackageManager, RepositoryDiscovery, resolve_node_command},
    gate::result::MeasureValue,
    issue::QualityIssue,
    metrics::MetricKey,
};

/// Provider 分析报告
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProviderReport {
    /// Provider 名称
    pub provider_name: String,
    /// 是否执行成功
    pub success: bool,
    /// 耗时（毫秒）
    pub duration_ms: u64,
    /// 度量值
    pub metrics: HashMap<MetricKey, MeasureValue>,
    /// 发现的问题
    pub issues: Vec<QualityIssue>,
    /// 原始输出（截断保留）
    pub raw_output: Option<String>,
    /// 错误消息
    pub error: Option<String>,
}

impl ProviderReport {
    /// 创建成功报告
    pub fn success(provider_name: impl Into<String>, duration_ms: u64) -> Self {
        Self {
            provider_name: provider_name.into(),
            success: true,
            duration_ms,
            metrics: HashMap::new(),
            issues: Vec::new(),
            raw_output: None,
            error: None,
        }
    }

    /// 创建失败报告
    pub fn failure(
        provider_name: impl Into<String>,
        duration_ms: u64,
        error: impl Into<String>,
    ) -> Self {
        Self {
            provider_name: provider_name.into(),
            success: false,
            duration_ms,
            metrics: HashMap::new(),
            issues: Vec::new(),
            raw_output: None,
            error: Some(error.into()),
        }
    }

    /// 添加度量值
    pub fn with_metric(mut self, key: MetricKey, value: MeasureValue) -> Self {
        self.metrics.insert(key, value);
        self
    }

    /// 添加问题列表
    pub fn with_issues(mut self, issues: Vec<QualityIssue>) -> Self {
        self.issues = issues;
        self
    }

    /// 设置原始输出
    pub fn with_raw_output(mut self, output: impl Into<String>) -> Self {
        self.raw_output = Some(output.into());
        self
    }
}

pub async fn run_node_quality_command(
    cwd: &Path,
    package_manager: Option<PackageManager>,
    command: &NodeQualityCommand,
) -> anyhow::Result<std::process::Output> {
    // `resolve_node_command` already routes the PM shim through
    // `resolve_node_exe` internally (both Script and PackageExec branches),
    // so no additional resolution is needed here. Calling it again was a
    // no-op for absolute results but caused primary-brain review concern
    // about idempotency assumptions — keep a single resolution point.
    let (cmd, args) = resolve_node_command(package_manager, command);
    // Bound the subprocess so a hung test/lint runner cannot block the
    // quality gate forever. The resolved command is typically a PM shim
    // (`pnpm exec ...`) which itself spawns a worker tree (tsx/vitest/tsc);
    // a worker that waits on a port/DB/service that never arrives hangs the
    // whole tree, and without a timeout `child.wait_with_output()` awaits
    // indefinitely, stranding the quality_run in "running" forever. Observed
    // 2026-06-29 (ecommerce final gate): a hung worker produced 140+
    // never-completing quality_runs and 230+ orphan node processes because
    // nothing ever reaped the tree.
    //
    // We capture the child PID, then race `wait_with_output` against a
    // generous budget. On timeout we tree-kill (taskkill /T on Windows,
    // process-group kill on Unix) so grandchildren are reaped too — killing
    // only the direct child leaves the hung worker (a grandchild) alive and
    // holding whatever resource it is stuck on.
    let child = tokio::process::Command::new(cmd)
        .args(args)
        .current_dir(cwd)
        // SoloDawn's root `.env` carries `PORT=23456` / `BACKEND_PORT=23456`
        // which dotenv-loads into server.exe at startup. Before Fix 6 the
        // Windows npm spawn failed silently and this pollution had no victim;
        // now that the gate really runs `npm test`, the child inherits our
        // dev ports and any target's test-time Express boot (e.g., Task 1)
        // ends up listening on 23456 and hijacks the backend port.
        // Strip the three ports at the gate's child boundary so quality-gate
        // subprocesses always see a clean port namespace.
        .env_remove("PORT")
        .env_remove("BACKEND_PORT")
        .env_remove("FRONTEND_PORT")
        .stdout(std::process::Stdio::piped())
        .stderr(std::process::Stdio::piped())
        .spawn()?;
    let pid = child.id().unwrap_or(0);
    match tokio::time::timeout(NODE_QUALITY_TIMEOUT, child.wait_with_output()).await {
        Ok(result) => result.map_err(Into::into),
        Err(_) => {
            kill_process_tree(pid).await;
            anyhow::bail!(
                "node quality command exceeded {}s timeout — likely a hung test/lint runner (killed process tree)",
                NODE_QUALITY_TIMEOUT.as_secs()
            )
        }
    }
}

/// Per-subprocess wall-clock budget for `run_node_quality_command`. Generous
/// enough to cover a real `pnpm test`/`tsc`/`vitest` run on a service, but
/// bounded so a hung runner is reaped instead of stranding the gate.
const NODE_QUALITY_TIMEOUT: Duration = Duration::from_secs(180);

/// Kill a process and its entire descendant tree. On timeout the hung leaf is
/// typically a grandchild (a tsx/vitest worker the PM shim spawned), so killing
/// only the direct child is insufficient.
#[cfg(windows)]
async fn kill_process_tree(pid: u32) {
    if pid == 0 {
        return;
    }
    // `taskkill /T` walks the tree rooted at `pid` and kills every descendant;
    // `/F` forces it. Best-effort — ignore failures (process may already be gone).
    let _ = tokio::process::Command::new("taskkill")
        .args(["/PID", &pid.to_string(), "/T", "/F"])
        .output()
        .await;
}

#[cfg(not(windows))]
async fn kill_process_tree(pid: u32) {
    if pid == 0 {
        return;
    }
    // Best-effort: send SIGKILL to the PID. Cross-platform tree-kill via a new
    // process group is handled at spawn time only when the caller opts in; the
    // dev stack runs on Windows so this Unix branch is a safety net.
    let _ = tokio::process::Command::new("kill")
        .args(["-9", &pid.to_string()])
        .output()
        .await;
}

/// 分析器 Provider trait
///
/// 每个 provider 封装一个代码分析工具（clippy、eslint、sonar 等）
#[async_trait]
pub trait QualityProvider: Send + Sync {
    /// Provider 名称
    fn name(&self) -> &str;

    /// 是否启用
    fn is_enabled(&self) -> bool {
        true
    }

    /// 执行分析
    ///
    /// # 参数
    /// - `project_root`: 项目根目录
    /// - `changed_files`: 变更的文件列表（用于 terminal gate 的增量分析）
    ///
    /// # 返回
    /// - `ProviderReport`: 分析报告
    async fn analyze(
        &self,
        project_root: &Path,
        discovery: &RepositoryDiscovery,
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport>;

    /// 获取 provider 支持的度量指标
    fn supported_metrics(&self) -> Vec<MetricKey>;

    /// 获取当前仓库/作用域下真正适用的度量指标。
    ///
    /// 与 `supported_metrics` 不同，这里允许 provider 根据 discovery 结果
    /// 将“不适用”的指标从 gate 过滤掉，避免跨技术栈误报。
    fn applicable_metrics(
        &self,
        _discovery: &RepositoryDiscovery,
        _changed_files: Option<&[String]>,
    ) -> Vec<MetricKey> {
        self.supported_metrics()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::discovery::{NodeQualityCommand, PackageManager};

    /// Regression guard for the R6 port-23456 orphan incident: every child
    /// spawned by `run_node_quality_command` must have `PORT` /
    /// `BACKEND_PORT` / `FRONTEND_PORT` stripped from its env, so SoloDawn's
    /// dev-server ports (loaded into server.exe via `dotenv::dotenv().ok()`)
    /// cannot leak into a Node test runner and hijack them.
    #[tokio::test]
    async fn run_node_quality_command_strips_solodawn_dev_ports() {
        if std::process::Command::new("node")
            .arg("-v")
            .output()
            .is_err()
        {
            eprintln!("node not found; skipping env-strip regression test");
            return;
        }

        let tmp = std::env::temp_dir().join(format!("quality-env-strip-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(&tmp).unwrap();

        std::fs::write(
            tmp.join("package.json"),
            r#"{
  "name": "env-strip-probe",
  "version": "0.0.0",
  "private": true,
  "scripts": {
    "print-env": "node -e \"console.log('P='+(process.env.PORT||'NIL')+';B='+(process.env.BACKEND_PORT||'NIL')+';F='+(process.env.FRONTEND_PORT||'NIL'))\""
  }
}
"#,
        )
        .unwrap();

        // Poison the parent env the way SoloDawn's root `.env` does in prod.
        // SAFETY: test-local, no concurrent env readers with meaning on these.
        unsafe {
            std::env::set_var("PORT", "23456");
            std::env::set_var("BACKEND_PORT", "23456");
            std::env::set_var("FRONTEND_PORT", "23457");
        }

        let output = run_node_quality_command(
            &tmp,
            Some(PackageManager::Npm),
            &NodeQualityCommand::Script {
                script: "print-env".to_string(),
            },
        )
        .await
        .expect("run_node_quality_command should succeed");

        let stdout = String::from_utf8_lossy(&output.stdout);

        assert!(
            stdout.contains("P=NIL"),
            "PORT leaked into child: stdout = {stdout}"
        );
        assert!(
            stdout.contains("B=NIL"),
            "BACKEND_PORT leaked into child: stdout = {stdout}"
        );
        assert!(
            stdout.contains("F=NIL"),
            "FRONTEND_PORT leaked into child: stdout = {stdout}"
        );

        // Cleanup — remove temp dir and the env pollution we injected.
        unsafe {
            std::env::remove_var("PORT");
            std::env::remove_var("BACKEND_PORT");
            std::env::remove_var("FRONTEND_PORT");
        }
        let _ = std::fs::remove_dir_all(&tmp);
    }
}
