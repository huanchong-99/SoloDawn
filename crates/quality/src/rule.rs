//! 规则类型与严重级别 — 移植自 SonarQube `RuleType.java` + `ImpactSeverityMapper.java`

use serde::{Deserialize, Serialize};

/// 规则类型
///
/// 移植自 SonarQube `RuleType`
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum RuleType {
    /// 代码 Bug — 影响可靠性
    Bug,
    /// 漏洞 — 影响安全性
    Vulnerability,
    /// 代码异味 — 影响可维护性
    CodeSmell,
    /// 安全热点 — 需要人工审查的安全相关代码
    SecurityHotspot,
}

impl RuleType {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Bug => "BUG",
            Self::Vulnerability => "VULNERABILITY",
            Self::CodeSmell => "CODE_SMELL",
            Self::SecurityHotspot => "SECURITY_HOTSPOT",
        }
    }
}

impl std::fmt::Display for RuleType {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Bug => write!(f, "Bug"),
            Self::Vulnerability => write!(f, "Vulnerability"),
            Self::CodeSmell => write!(f, "Code Smell"),
            Self::SecurityHotspot => write!(f, "Security Hotspot"),
        }
    }
}

/// 严重级别
///
/// 移植自 SonarQube severity 体系，参考 `ImpactSeverityMapper`
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize, PartialOrd, Ord)]
pub enum Severity {
    /// 信息 — 不影响质量门
    Info,
    /// 次要 — 低影响
    Minor,
    /// 主要 — 中等影响
    Major,
    /// 严重 — 高影响，默认阻断
    Critical,
    /// 阻断 — 最高影响，必须修复
    Blocker,
}

impl Severity {
    pub fn as_str(&self) -> &'static str {
        match self {
            Self::Info => "INFO",
            Self::Minor => "MINOR",
            Self::Major => "MAJOR",
            Self::Critical => "CRITICAL",
            Self::Blocker => "BLOCKER",
        }
    }

    /// 从 SonarQube 字符串解析
    pub fn from_sonar_str(s: &str) -> Option<Self> {
        match s.to_uppercase().as_str() {
            "INFO" => Some(Self::Info),
            "MINOR" => Some(Self::Minor),
            "MAJOR" => Some(Self::Major),
            "CRITICAL" => Some(Self::Critical),
            "BLOCKER" => Some(Self::Blocker),
            _ => None,
        }
    }

    /// 是否为阻断级别（Critical 或 Blocker）
    pub fn is_blocking(&self) -> bool {
        matches!(self, Self::Critical | Self::Blocker)
    }

    /// Cap severity for **advisory** analyzers — the quality gate's
    /// model-agnostic severity principle.
    ///
    /// ### The principle
    ///
    /// Certain analyzers are *advisory*: their per-rule severity is decided
    /// by **project-local config files** (`.eslintrc`, `#[deny(...)]`
    /// attributes, `clippy.toml`, `.stylelintrc`, …). In this repo those
    /// configs are written by whichever LLM drafted the task, and their
    /// choices vary wildly — `no-explicit-any` lands as `error` in one
    /// model's output and as `warn` in another's. Letting that label drive
    /// the gate's blocking decision means **model taste decides whether the
    /// gate blocks**, which is a contract we cannot honor.
    ///
    /// For advisory sources we therefore ignore the label and cap severity at
    /// `Major` — non-blocking per [`Severity::is_blocking`]. The analyzer's
    /// output is still collected, displayed, and counted; it just never gates.
    ///
    /// ### Where the principle is encoded
    ///
    /// Whether a given analyzer is advisory is **not** determined in this
    /// method's body. It is determined by
    /// [`AnalyzerSource::severity_origin`], which is the single source of
    /// truth. That method's `match` is **exhaustive** — adding a new
    /// [`AnalyzerSource`] variant without declaring its origin is a compile
    /// error, not a silent fall-through. This closes the loophole where a
    /// quietly-added linter could default to authoritative and re-introduce
    /// model-dependent gating.
    ///
    /// ### What is *not* advisory
    ///
    /// Compile checks (`tsc`, `cargo check`), test runners, secret/security
    /// detectors, and our own built-in rule engine report genuine breakage or
    /// risk, not style preference. They keep their reported severity and can
    /// still block. Tool-unavailable sentinels (`*::unavailable`) also stay
    /// blocking — a linter that couldn't even run is an environment failure,
    /// not an advisory opinion.
    pub fn cap_for_advisory(self, source: &AnalyzerSource) -> Severity {
        match source.severity_origin() {
            SeverityOrigin::ProjectConfig => std::cmp::min(self, Severity::Major),
            SeverityOrigin::Tool => self,
        }
    }
}

impl std::fmt::Display for Severity {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(f, "{}", self.as_str())
    }
}

/// 严重级别的真源 — 分析器的 per-rule 严重级别由谁决定？
///
/// 这是 [`Severity::cap_for_advisory`] 的决策依据：只有 `ProjectConfig`
/// 出身的分析器才会被封顶到非阻断。将此概念提升为显式类型的目的是
/// **通过穷举 match 强制每个新 [`AnalyzerSource`] 变体在编译期表态**，
/// 消除"`_` 兜底默默放行"这一结构性弱点 —— 新增 linter 时，
/// 编译器会拒绝编译，而不是悄悄退化回模型依赖的阻断行为。
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum SeverityOrigin {
    /// 严重级别由工具自身硬编码 —— 编译错误、测试失败、密钥检测、
    /// 内置规则引擎、SonarQube 分析等。标签 = 真实信号，质量门完整信任。
    Tool,
    /// 严重级别来自项目本地配置（`.eslintrc`、`clippy.toml`、
    /// `.stylelintrc` …）。在本仓库这些文件由 LLM 生成，随模型口味
    /// 变化。标签 = 风格偏好 ≠ 真实信号，质量门必须封顶。
    ProjectConfig,
}

/// 分析器来源标识
#[derive(Debug, Clone, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum AnalyzerSource {
    /// cargo clippy
    Clippy,
    /// cargo check
    CargoCheck,
    /// cargo fmt
    CargoFmt,
    /// cargo test
    CargoTest,
    /// pnpm lint (ESLint)
    EsLint,
    /// pnpm check (TypeScript)
    TypeScript,
    /// pnpm test:run (Vitest)
    Vitest,
    /// SonarQube 本地分析
    Sonar,
    /// 安全审计脚本
    SecurityAudit,
    /// 其他
    Other(String),
}

impl AnalyzerSource {
    /// 声明该分析器的严重级别真源 — 见 [`SeverityOrigin`]。
    ///
    /// **此 `match` 是穷举的**：新增 [`AnalyzerSource`] 变体而不加对应 arm
    /// 会直接编译错误，无法静默降级。这是整个咨询性封顶机制的唯一真源，
    /// [`Severity::cap_for_advisory`] 只读此返回值，不自行判断。
    ///
    /// ### 分类原则与 Clippy 归属
    ///
    /// `ProjectConfig` 与 `Tool` 的分界不是"该工具是否支持项目级配置"，
    /// 而是"本仓库实际使用的 per-rule 严重级别由谁主导"。按此标准：
    ///
    /// - **ESLint** 的 severity 几乎**完全**由 `.eslintrc` 的 `"error"` /
    ///   `"warn"` 字面值决定，工具默认几乎不参与 —— 而本仓库的
    ///   `.eslintrc` 由 LLM 起草、随模型口味漂移，故分类为 `ProjectConfig`。
    /// - **Clippy** 的 severity 则**主导于工具内置分组**（correctness /
    ///   suspicious / style / pedantic …），项目本地 `#[deny(...)]`
    ///   覆盖属于例外而非常态。因此本仓库 Clippy 分类为 `Tool`
    ///   才是对齐原则的正确选择，不是历史债务。
    ///
    /// 若日后仓库开始大量注入 `#[deny(clippy::...)]`，Clippy 会从
    /// "工具主导" 滑向 "项目主导"，届时单改此处 `Clippy` 的 arm 即可
    /// 完成切换 —— 迁移对所有解析器站点统一生效（它们都已走
    /// [`QualityIssue::new_capped`]），无需改动分析器代码。
    pub fn severity_origin(&self) -> SeverityOrigin {
        match self {
            // 严重级别由项目本地配置决定 —— 模型口味，不能阻断
            Self::EsLint => SeverityOrigin::ProjectConfig,

            // 严重级别由工具硬编码 —— 真实信号，允许阻断
            Self::CargoCheck
            | Self::CargoFmt
            | Self::CargoTest
            | Self::Clippy
            | Self::TypeScript
            | Self::Vitest
            | Self::Sonar
            | Self::SecurityAudit
            | Self::Other(_) => SeverityOrigin::Tool,
        }
    }
}

impl std::fmt::Display for AnalyzerSource {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Clippy => write!(f, "clippy"),
            Self::CargoCheck => write!(f, "cargo-check"),
            Self::CargoFmt => write!(f, "cargo-fmt"),
            Self::CargoTest => write!(f, "cargo-test"),
            Self::EsLint => write!(f, "eslint"),
            Self::TypeScript => write!(f, "typescript"),
            Self::Vitest => write!(f, "vitest"),
            Self::Sonar => write!(f, "sonarqube"),
            Self::SecurityAudit => write!(f, "security-audit"),
            Self::Other(name) => write!(f, "{}", name),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // These tests lock in the model-agnostic severity principle described on
    // `Severity::cap_for_advisory`. If one of them fails, someone either
    // (a) let an advisory linter block again, or (b) accidentally capped a
    // non-advisory analyzer. Either is a contract violation.

    #[test]
    fn eslint_error_label_is_capped_to_major_and_nonblocking() {
        // Even if a model writes `"severity": "error"` into .eslintrc,
        // the gate's internal severity stays below the blocking threshold.
        let raw = Severity::Critical;
        let capped = raw.cap_for_advisory(&AnalyzerSource::EsLint);
        assert_eq!(capped, Severity::Major);
        assert!(!capped.is_blocking(), "ESLint findings must never block");
    }

    #[test]
    fn eslint_blocker_label_is_still_capped() {
        let raw = Severity::Blocker;
        let capped = raw.cap_for_advisory(&AnalyzerSource::EsLint);
        assert_eq!(capped, Severity::Major);
        assert!(!capped.is_blocking());
    }

    #[test]
    fn eslint_lower_severities_pass_through_unchanged() {
        // Capping only reduces; it never inflates a minor finding into a major one.
        for sev in [Severity::Info, Severity::Minor, Severity::Major] {
            assert_eq!(sev.cap_for_advisory(&AnalyzerSource::EsLint), sev);
        }
    }

    #[test]
    fn authoritative_sources_are_not_capped() {
        // tsc / cargo check / tests / security audit report genuine breakage.
        // Their severities must flow through untouched so they can still block.
        let sources = [
            AnalyzerSource::CargoCheck,
            AnalyzerSource::TypeScript,
            AnalyzerSource::Vitest,
            AnalyzerSource::CargoTest,
            AnalyzerSource::Clippy,
            AnalyzerSource::SecurityAudit,
            AnalyzerSource::Sonar,
        ];
        for source in sources {
            assert_eq!(
                Severity::Critical.cap_for_advisory(&source),
                Severity::Critical,
                "{:?} must keep Critical severity — it is not advisory",
                source
            );
            assert!(Severity::Critical.cap_for_advisory(&source).is_blocking());
        }
    }

    #[test]
    fn severity_origin_classification_is_pinned() {
        // Tier 2 lock-in: every currently-known AnalyzerSource has a
        // declared SeverityOrigin. The compiler already forbids a new
        // variant from being added without an arm (the match in
        // `severity_origin` is exhaustive), so this test is not about
        // preventing omission — it's about making any *reclassification*
        // (e.g. flipping Clippy from Tool to ProjectConfig) visible as a
        // test diff in code review.
        use SeverityOrigin::*;
        let expectations: &[(AnalyzerSource, SeverityOrigin)] = &[
            (AnalyzerSource::EsLint, ProjectConfig),
            (AnalyzerSource::CargoCheck, Tool),
            (AnalyzerSource::CargoFmt, Tool),
            (AnalyzerSource::CargoTest, Tool),
            (AnalyzerSource::Clippy, Tool),
            (AnalyzerSource::TypeScript, Tool),
            (AnalyzerSource::Vitest, Tool),
            (AnalyzerSource::Sonar, Tool),
            (AnalyzerSource::SecurityAudit, Tool),
            (AnalyzerSource::Other("arbitrary".into()), Tool),
        ];
        for (source, expected) in expectations {
            assert_eq!(
                source.severity_origin(),
                *expected,
                "{:?} severity_origin classification changed — confirm this is intentional",
                source
            );
        }
    }

    #[test]
    fn cap_routes_through_severity_origin() {
        // Integration check: cap_for_advisory's behavior matches what
        // severity_origin declares. If they ever drift (e.g. someone
        // hard-codes a case in cap_for_advisory's body again), this
        // catches it.
        for source in [
            AnalyzerSource::EsLint,
            AnalyzerSource::Clippy,
            AnalyzerSource::TypeScript,
            AnalyzerSource::CargoCheck,
            AnalyzerSource::Vitest,
            AnalyzerSource::Other("future-linter".into()),
        ] {
            let capped = Severity::Critical.cap_for_advisory(&source);
            match source.severity_origin() {
                SeverityOrigin::ProjectConfig => {
                    assert_eq!(capped, Severity::Major);
                    assert!(!capped.is_blocking());
                }
                SeverityOrigin::Tool => {
                    assert_eq!(capped, Severity::Critical);
                    assert!(capped.is_blocking());
                }
            }
        }
    }
}
