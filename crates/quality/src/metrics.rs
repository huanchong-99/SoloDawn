//! 度量指标定义
//!
//! 定义质量门使用的所有度量指标。
//! 参考 SonarQube `ScannerMetrics.java` 和 `SoftwareQualitiesMetrics.java`

use serde::{Deserialize, Serialize};

/// 度量指标 Key
///
/// SoloDawn 的度量指标体系，结合 SonarQube 模式和项目实际分析工具
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum MetricKey {
    // ── Rust 分析器指标 ──
    /// Cargo check 错误数
    #[serde(rename = "cargo_check_errors")]
    CargoCheckErrors,
    /// Clippy 警告数
    #[serde(rename = "clippy_warnings")]
    ClippyWarnings,
    /// Clippy 错误数
    #[serde(rename = "clippy_errors")]
    ClippyErrors,
    /// Cargo fmt 差异数（未格式化文件）
    #[serde(rename = "fmt_violations")]
    FmtViolations,
    /// Rust 测试失败数
    #[serde(rename = "rust_test_failures")]
    RustTestFailures,

    // ── Frontend 分析器指标 ──
    /// ESLint 错误数
    #[serde(rename = "eslint_errors")]
    EslintErrors,
    /// ESLint 警告数
    #[serde(rename = "eslint_warnings")]
    EslintWarnings,
    /// TypeScript 类型检查错误数
    #[serde(rename = "tsc_errors")]
    TscErrors,
    /// Frontend 测试失败数
    #[serde(rename = "frontend_test_failures")]
    FrontendTestFailures,
    /// 测试代码引入了 `@testing-library/*` 之类，但 package.json 未声明对应依赖
    #[serde(rename = "frontend_test_deps_missing")]
    FrontendTestDepsMissing,

    // ── 通用指标（SonarQube 风格）──
    /// 测试失败总数
    #[serde(rename = "test_failures")]
    TestFailures,
    /// 测试覆盖率 (%)
    #[serde(rename = "test_coverage")]
    TestCoverage,
    /// Bug 数
    #[serde(rename = "bugs")]
    Bugs,
    /// 新增 Bug 数（变化量）
    #[serde(rename = "new_bugs")]
    NewBugs,
    /// 代码异味数
    #[serde(rename = "code_smells")]
    CodeSmells,
    /// 漏洞数
    #[serde(rename = "vulnerabilities")]
    Vulnerabilities,
    /// 重复行比率 (%)
    #[serde(rename = "duplicated_lines_density")]
    DuplicatedLinesDensity,
    /// 安全审计问题数
    #[serde(rename = "security_issues")]
    SecurityIssues,
    /// 潜在 ReDoS 风险数量
    #[serde(rename = "redos_risks")]
    RedosRisks,

    // ── Repo/Infra 指标 ──
    /// 类型生成检查失败
    #[serde(rename = "generate_types_check_failures")]
    GenerateTypesCheckFailures,
    /// DB 准备检查失败
    #[serde(rename = "prepare_db_check_failures")]
    PrepareDbCheckFailures,

    // ── SonarQube 集成指标 ──
    /// Sonar 质量门状态
    #[serde(rename = "sonar_quality_gate_status")]
    SonarQualityGateStatus,
    /// Sonar 问题总数
    #[serde(rename = "sonar_issues")]
    SonarIssues,
    /// Sonar Blocker 级别问题数
    #[serde(rename = "sonar_blocker_issues")]
    SonarBlockerIssues,
    /// Sonar Critical 级别问题数
    #[serde(rename = "sonar_critical_issues")]
    SonarCriticalIssues,

    // ── Built-in Rust 分析指标 ──
    /// 内置 Rust 规则发现的问题总数
    #[serde(rename = "builtin_rust_issues")]
    BuiltinRustIssues,
    /// 内置 Rust 规则发现的 Critical+ 问题数
    #[serde(rename = "builtin_rust_critical")]
    BuiltinRustCritical,
    /// 最高圈复杂度
    #[serde(rename = "rust_cyclomatic_complexity")]
    RustCyclomaticComplexity,
    /// 最高认知复杂度
    #[serde(rename = "rust_cognitive_complexity")]
    RustCognitiveComplexity,

    // ── Built-in Frontend 分析指标 ──
    /// 内置前端规则发现的问题总数
    #[serde(rename = "builtin_frontend_issues")]
    BuiltinFrontendIssues,
    /// 内置前端规则发现的 Critical+ 问题数
    #[serde(rename = "builtin_frontend_critical")]
    BuiltinFrontendCritical,

    // ── Built-in Common 分析指标 ──
    /// 内置通用规则发现的问题总数
    #[serde(rename = "builtin_common_issues")]
    BuiltinCommonIssues,
    /// 重复代码块数
    #[serde(rename = "duplicated_blocks")]
    DuplicatedBlocks,
    /// 检测到的密钥/凭证数
    #[serde(rename = "secrets_detected")]
    SecretsDetected,

    // ── Coverage 指标 ──
    /// 行覆盖率 (%)
    #[serde(rename = "line_coverage")]
    LineCoverage,
    /// 分支覆盖率 (%)
    #[serde(rename = "branch_coverage")]
    BranchCoverage,

    // ── Completeness 指标 ──
    /// 测试文件缺失（项目有源文件但零测试文件时 = 1）
    #[serde(rename = "test_file_absence")]
    TestFileAbsence,
    /// TODO/FIXME 密度百分比（TODO 行数 / 总行数 * 100）
    #[serde(rename = "todo_density")]
    TodoDensity,
    /// 占位/空洞测试数量
    #[serde(rename = "stub_test_count")]
    StubTestCount,
    /// 覆盖率配置排除核心业务层的可疑项数量
    #[serde(rename = "coverage_exclusion_issues")]
    CoverageExclusionIssues,

    // ── Quality engine internal sentinels ──
    /// 强制模式下，仓库已发现 target 但没有任何 provider 能评估任一 gate condition
    /// 的兜底信号。等同于"安检员手里没拿到清单"，必须 fail-closed。
    #[serde(rename = "quality_gate_empty_scan")]
    QualityGateEmptyScan,
}

impl MetricKey {
    /// 返回指标的字符串 key
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::CargoCheckErrors => "cargo_check_errors",
            Self::ClippyWarnings => "clippy_warnings",
            Self::ClippyErrors => "clippy_errors",
            Self::FmtViolations => "fmt_violations",
            Self::RustTestFailures => "rust_test_failures",
            Self::EslintErrors => "eslint_errors",
            Self::EslintWarnings => "eslint_warnings",
            Self::TscErrors => "tsc_errors",
            Self::FrontendTestFailures => "frontend_test_failures",
            Self::FrontendTestDepsMissing => "frontend_test_deps_missing",
            Self::TestFailures => "test_failures",
            Self::TestCoverage => "test_coverage",
            Self::Bugs => "bugs",
            Self::NewBugs => "new_bugs",
            Self::CodeSmells => "code_smells",
            Self::Vulnerabilities => "vulnerabilities",
            Self::DuplicatedLinesDensity => "duplicated_lines_density",
            Self::SecurityIssues => "security_issues",
            Self::RedosRisks => "redos_risks",
            Self::GenerateTypesCheckFailures => "generate_types_check_failures",
            Self::PrepareDbCheckFailures => "prepare_db_check_failures",
            Self::SonarQualityGateStatus => "sonar_quality_gate_status",
            Self::SonarIssues => "sonar_issues",
            Self::SonarBlockerIssues => "sonar_blocker_issues",
            Self::SonarCriticalIssues => "sonar_critical_issues",
            Self::BuiltinRustIssues => "builtin_rust_issues",
            Self::BuiltinRustCritical => "builtin_rust_critical",
            Self::RustCyclomaticComplexity => "rust_cyclomatic_complexity",
            Self::RustCognitiveComplexity => "rust_cognitive_complexity",
            Self::BuiltinFrontendIssues => "builtin_frontend_issues",
            Self::BuiltinFrontendCritical => "builtin_frontend_critical",
            Self::BuiltinCommonIssues => "builtin_common_issues",
            Self::DuplicatedBlocks => "duplicated_blocks",
            Self::SecretsDetected => "secrets_detected",
            Self::LineCoverage => "line_coverage",
            Self::BranchCoverage => "branch_coverage",
            Self::TestFileAbsence => "test_file_absence",
            Self::TodoDensity => "todo_density",
            Self::StubTestCount => "stub_test_count",
            Self::CoverageExclusionIssues => "coverage_exclusion_issues",
            Self::QualityGateEmptyScan => "quality_gate_empty_scan",
        }
    }

    /// 返回人类可读名称
    pub fn display_name(&self) -> &'static str {
        match self {
            Self::CargoCheckErrors => "Cargo Check Errors",
            Self::ClippyWarnings => "Clippy Warnings",
            Self::ClippyErrors => "Clippy Errors",
            Self::FmtViolations => "Format Violations",
            Self::RustTestFailures => "Rust Test Failures",
            Self::EslintErrors => "ESLint Errors",
            Self::EslintWarnings => "ESLint Warnings",
            Self::TscErrors => "TypeScript Errors",
            Self::FrontendTestFailures => "Frontend Test Failures",
            Self::FrontendTestDepsMissing => "Frontend Test Deps Missing",
            Self::TestFailures => "Test Failures",
            Self::TestCoverage => "Test Coverage",
            Self::Bugs => "Bugs",
            Self::NewBugs => "New Bugs",
            Self::CodeSmells => "Code Smells",
            Self::Vulnerabilities => "Vulnerabilities",
            Self::DuplicatedLinesDensity => "Duplicated Lines (%)",
            Self::SecurityIssues => "Security Issues",
            Self::RedosRisks => "ReDoS Risks",
            Self::GenerateTypesCheckFailures => "Type Generation Failures",
            Self::PrepareDbCheckFailures => "DB Preparation Failures",
            Self::SonarQualityGateStatus => "Sonar Quality Gate",
            Self::SonarIssues => "Sonar Issues",
            Self::SonarBlockerIssues => "Sonar Blocker Issues",
            Self::SonarCriticalIssues => "Sonar Critical Issues",
            Self::BuiltinRustIssues => "Built-in Rust Issues",
            Self::BuiltinRustCritical => "Built-in Rust Critical",
            Self::RustCyclomaticComplexity => "Rust Cyclomatic Complexity",
            Self::RustCognitiveComplexity => "Rust Cognitive Complexity",
            Self::BuiltinFrontendIssues => "Built-in Frontend Issues",
            Self::BuiltinFrontendCritical => "Built-in Frontend Critical",
            Self::BuiltinCommonIssues => "Built-in Common Issues",
            Self::DuplicatedBlocks => "Duplicated Blocks",
            Self::SecretsDetected => "Secrets Detected",
            Self::LineCoverage => "Line Coverage (%)",
            Self::BranchCoverage => "Branch Coverage (%)",
            Self::TestFileAbsence => "Test File Absence",
            Self::TodoDensity => "TODO Density (%)",
            Self::StubTestCount => "Stub Test Count",
            Self::CoverageExclusionIssues => "Coverage Exclusion Issues",
            Self::QualityGateEmptyScan => "Empty Quality Scan",
        }
    }
}

impl std::fmt::Display for MetricKey {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.display_name())
    }
}
