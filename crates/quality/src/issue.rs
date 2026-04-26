//! 质量问题模型 — 移植自 SonarQube `DefaultIssue.java`
//!
//! 表示代码质量分析中发现的单个问题。

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::rule::{AnalyzerSource, RuleType, Severity};

/// 单个质量问题
///
/// 设计参考 SonarQube `DefaultIssue`，简化为 SoloDawn 场景所需字段
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QualityIssue {
    /// 唯一标识
    pub id: String,
    /// 规则 ID（如 "clippy::unwrap_used", "sonar:S1234"）
    pub rule_id: String,
    /// 规则类型
    pub rule_type: RuleType,
    /// 严重级别
    pub severity: Severity,
    /// 分析器来源
    pub source: AnalyzerSource,
    /// 问题消息
    pub message: String,
    /// 文件路径（相对于项目根）
    pub file_path: Option<String>,
    /// 起始行号
    pub line: Option<u32>,
    /// 结束行号
    pub end_line: Option<u32>,
    /// 起始列号
    pub column: Option<u32>,
    /// 结束列号
    pub end_column: Option<u32>,
    /// 预计修复耗时（分钟）
    pub effort_minutes: Option<i32>,
    /// 是否为新增代码引入（区分新增问题和历史债务）
    pub is_new: bool,
    /// 发现时间
    pub created_at: DateTime<Utc>,
    /// 附加上下文（如代码片段、建议修复等）
    pub context: Option<String>,
}

impl QualityIssue {
    /// 创建新的质量问题（**不应用咨询性封顶**）
    ///
    /// 此构造器写入原始 `severity`，不经过
    /// [`Severity::cap_for_advisory`]。仅用于**有意绕过封顶**的场景：
    /// - `*::unavailable` 哨兵 —— 工具未能启动 = 环境故障，必须阻断
    /// - `quality_engine::empty_scan` 等基础设施信号 —— meta-level，
    ///   非分析器发现
    ///
    /// 分析器解析出的发现一律应使用 [`Self::new_capped`]，让原则
    /// ([`AnalyzerSource::severity_origin`]) 在单一真源处决定阻断。
    pub fn new(
        rule_id: impl Into<String>,
        rule_type: RuleType,
        severity: Severity,
        source: AnalyzerSource,
        message: impl Into<String>,
    ) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            rule_id: rule_id.into(),
            rule_type,
            severity,
            source,
            message: message.into(),
            file_path: None,
            line: None,
            end_line: None,
            column: None,
            end_column: None,
            effort_minutes: None,
            is_new: true,
            created_at: Utc::now(),
            context: None,
        }
    }

    /// 创建质量问题，**自动应用咨询性封顶**。
    ///
    /// 这是所有**分析器解析器**（clippy、tsc、vitest、eslint 等）
    /// 的**首选构造器**。原始 `raw_severity` 会经过
    /// [`Severity::cap_for_advisory`] —— 若 `source` 的
    /// [`AnalyzerSource::severity_origin`] 为 `ProjectConfig`（如 ESLint），
    /// 严重级别将被封顶到 `Major`（非阻断）；若为 `Tool`（如 TypeScript、
    /// CargoTest），原样透传。
    ///
    /// ### 为何不让 `new` 默认封顶？
    ///
    /// 一些场景 *必须* 绕过封顶 —— `*::unavailable` 哨兵代表工具未能
    /// 启动（环境故障），无论该工具是否咨询性都得阻断。若 `new`
    /// 默认封顶，这些场景就得专门开后门；反之把原则移到 `new_capped`，
    /// `new` 保留为低阶 primitive，更清晰。
    ///
    /// ### 未来扩展
    ///
    /// 新增分析器解析器时用此构造器，严重级别写源生硬编码值即可 ——
    /// 质量门的咨询性封顶机制会自动生效，无需改动解析器。
    pub fn new_capped(
        rule_id: impl Into<String>,
        rule_type: RuleType,
        raw_severity: Severity,
        source: AnalyzerSource,
        message: impl Into<String>,
    ) -> Self {
        let severity = raw_severity.cap_for_advisory(&source);
        Self::new(rule_id, rule_type, severity, source, message)
    }

    /// 设置文件位置信息
    pub fn with_location(mut self, file_path: impl Into<String>, line: u32) -> Self {
        self.file_path = Some(file_path.into());
        self.line = Some(line);
        self
    }

    /// 设置完整的位置范围
    pub fn with_range(
        mut self,
        file_path: impl Into<String>,
        line: u32,
        column: u32,
        end_line: u32,
        end_column: u32,
    ) -> Self {
        self.file_path = Some(file_path.into());
        self.line = Some(line);
        self.column = Some(column);
        self.end_line = Some(end_line);
        self.end_column = Some(end_column);
        self
    }

    /// 设置修复耗时估计
    pub fn with_effort(mut self, minutes: i32) -> Self {
        self.effort_minutes = Some(minutes);
        self
    }

    /// 设置为历史债务问题
    pub fn as_legacy(mut self) -> Self {
        self.is_new = false;
        self
    }

    /// 设置上下文
    pub fn with_context(mut self, context: impl Into<String>) -> Self {
        self.context = Some(context.into());
        self
    }

    /// 是否为阻断级别问题
    pub fn is_blocking(&self) -> bool {
        self.severity.is_blocking()
    }

    /// 生成格式化的位置字符串（file:line）
    pub fn location_string(&self) -> String {
        match (&self.file_path, self.line) {
            (Some(path), Some(line)) => format!("{}:{}", path, line),
            (Some(path), None) => path.clone(),
            _ => "unknown".to_string(),
        }
    }

    /// 生成终端回流用的结构化修复描述
    pub fn to_fix_instruction(&self) -> String {
        let mut instruction = format!("[{}] {} ({})\n", self.severity, self.message, self.rule_id);
        if let Some(ref path) = self.file_path {
            instruction.push_str(&format!("  File: {}", path));
            if let Some(line) = self.line {
                instruction.push_str(&format!(":{}", line));
            }
            instruction.push('\n');
        }
        if let Some(ref ctx) = self.context {
            instruction.push_str(&format!("  Context: {}\n", ctx));
        }
        instruction
    }
}

/// 质量问题摘要统计
#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct IssueSummary {
    pub total: usize,
    pub blocker: usize,
    pub critical: usize,
    pub major: usize,
    pub minor: usize,
    pub info: usize,
    pub new_issues: usize,
    pub blocking_issues: usize,
}

impl IssueSummary {
    /// 从问题列表生成摘要
    pub fn from_issues(issues: &[QualityIssue]) -> Self {
        let mut summary = Self {
            total: issues.len(),
            ..Self::default()
        };
        for issue in issues {
            match issue.severity {
                Severity::Blocker => summary.blocker += 1,
                Severity::Critical => summary.critical += 1,
                Severity::Major => summary.major += 1,
                Severity::Minor => summary.minor += 1,
                Severity::Info => summary.info += 1,
            }
            if issue.is_new {
                summary.new_issues += 1;
            }
            if issue.is_blocking() {
                summary.blocking_issues += 1;
            }
        }
        summary
    }

    /// 生成一行摘要文本
    pub fn one_line_summary(&self) -> String {
        format!(
            "{} issues ({} blocker, {} critical, {} major, {} minor, {} info) | {} new | {} blocking",
            self.total,
            self.blocker,
            self.critical,
            self.major,
            self.minor,
            self.info,
            self.new_issues,
            self.blocking_issues
        )
    }
}
