//! 报告聚合器
//!
//! 统一多个 Provider 的输出为一份完整的质量报告

use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::{
    gate::{QualityGateDecision, status::QualityGateStatus},
    issue::{IssueSummary, QualityIssue},
    provider::ProviderReport,
};

/// 聚合质量报告
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QualityReport {
    /// 报告 ID
    pub id: String,
    /// 质量门决策
    pub decision: Option<QualityGateDecision>,
    /// 各 Provider 报告
    pub provider_reports: Vec<ProviderReport>,
    /// 所有问题（聚合）
    pub all_issues: Vec<QualityIssue>,
    /// 问题摘要
    pub summary: IssueSummary,
    /// 总耗时（毫秒）
    pub total_duration_ms: u64,
    /// 创建时间
    pub created_at: DateTime<Utc>,
}

impl QualityReport {
    /// 从多个 Provider 报告构建聚合报告
    pub fn aggregate(reports: Vec<ProviderReport>) -> Self {
        let mut all_issues = Vec::new();
        let mut total_duration_ms = 0u64;

        for report in &reports {
            all_issues.extend(report.issues.clone());
            total_duration_ms += report.duration_ms;
        }

        let summary = IssueSummary::from_issues(&all_issues);

        Self {
            id: Uuid::new_v4().to_string(),
            decision: None,
            provider_reports: reports,
            all_issues,
            summary,
            total_duration_ms,
            created_at: Utc::now(),
        }
    }

    /// 设置质量门决策
    pub fn with_decision(mut self, decision: QualityGateDecision) -> Self {
        self.decision = Some(decision);
        self
    }

    /// 获取聚合状态
    pub fn overall_status(&self) -> QualityGateStatus {
        self.decision
            .as_ref()
            .map(|d| d.status)
            .unwrap_or(QualityGateStatus::Ok)
    }

    /// 是否通过
    pub fn is_passed(&self) -> bool {
        self.decision
            .as_ref()
            .map(|d| d.is_passed())
            .unwrap_or(true)
    }

    /// 只获取新引入的问题
    pub fn new_issues(&self) -> Vec<&QualityIssue> {
        self.all_issues.iter().filter(|i| i.is_new).collect()
    }

    /// 只获取阻断级别问题
    pub fn blocking_issues(&self) -> Vec<&QualityIssue> {
        self.all_issues.iter().filter(|i| i.is_blocking()).collect()
    }

    /// 生成终端回流用的修复指令
    pub fn to_fix_instructions(&self) -> String {
        let blocking = self.blocking_issues();
        if blocking.is_empty() {
            return String::new();
        }

        let mut instructions = format!(
            "=== Quality Gate FAILED ===\n{}\n\nBlocking issues ({}):\n\n",
            self.summary.one_line_summary(),
            blocking.len()
        );

        for (i, issue) in blocking.iter().enumerate() {
            instructions.push_str(&format!("{}. {}", i + 1, issue.to_fix_instruction()));
        }

        instructions.push_str("\nPlease fix the above issues and commit again.\n");
        instructions
    }

    /// 生成简短的状态行
    pub fn status_line(&self) -> String {
        let status = self.overall_status();
        let icon = match status {
            QualityGateStatus::Ok => "✅",
            QualityGateStatus::Warn => "⚠️",
            QualityGateStatus::Error => "❌",
        };
        format!(
            "{} Quality Gate: {} | {} | {}ms",
            icon,
            status,
            self.summary.one_line_summary(),
            self.total_duration_ms
        )
    }
}
