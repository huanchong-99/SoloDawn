//! 质量门模型 — 移植自 SonarQube qualitygate 模块
//!
//! 设计参考:
//! - `QualityGate.java` — 门禁定义
//! - `Condition.java` — 条件模型
//! - `ConditionEvaluator.java` — 条件求值引擎
//! - `EvaluationResult.java` — 求值结果
//! - `QualityGateStatus.java` — 聚合状态

pub mod condition;
pub mod evaluator;
pub mod result;
pub mod status;

use serde::{Deserialize, Serialize};
use uuid::Uuid;

use self::{condition::Condition, result::EvaluationResult, status::QualityGateStatus};

/// 质量门定义
///
/// 设计移植自 SonarQube `QualityGate.java`
/// 一个质量门由唯一标识、名称和一组条件组成。
/// 所有条件都通过时，质量门状态为 OK；任一条件失败则为 ERROR。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QualityGate {
    /// 唯一标识
    pub id: String,
    /// 门禁名称（如 "Terminal Gate", "Branch Gate", "Repo Gate"）
    pub name: String,
    /// 条件集合
    pub conditions: Vec<Condition>,
}

impl QualityGate {
    /// 创建新的质量门
    pub fn new(name: impl Into<String>, conditions: Vec<Condition>) -> Self {
        Self {
            id: Uuid::new_v4().to_string(),
            name: name.into(),
            conditions,
        }
    }

    /// 从配置创建，使用指定 ID
    pub fn with_id(
        id: impl Into<String>,
        name: impl Into<String>,
        conditions: Vec<Condition>,
    ) -> Self {
        Self {
            id: id.into(),
            name: name.into(),
            conditions,
        }
    }

    /// 评估所有条件，返回聚合状态与详细结果
    pub fn evaluate(&self, results: &[EvaluationResult]) -> QualityGateDecision {
        // 参考 SonarQube: 任一条件为 ERROR 则整体为 ERROR
        let overall_status = if results.iter().any(|r| r.level == status::Level::Error) {
            QualityGateStatus::Error
        } else if results.iter().any(|r| r.level == status::Level::Warn) {
            QualityGateStatus::Warn
        } else {
            QualityGateStatus::Ok
        };

        QualityGateDecision {
            gate_id: self.id.clone(),
            gate_name: self.name.clone(),
            status: overall_status,
            condition_results: results.to_vec(),
        }
    }
}

/// 质量门决策结果
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QualityGateDecision {
    /// 质量门 ID
    pub gate_id: String,
    /// 质量门名称
    pub gate_name: String,
    /// 聚合状态
    pub status: QualityGateStatus,
    /// 各条件的详细求值结果
    pub condition_results: Vec<EvaluationResult>,
}

impl QualityGateDecision {
    /// 是否通过 — only `Ok` counts as passed.
    /// `Warn` (missing metrics / provider failure) is NOT passed in enforce mode
    /// because quality was not actually verified.
    pub fn is_passed(&self) -> bool {
        self.status == QualityGateStatus::Ok
    }

    /// 是否被阻断
    pub fn is_blocked(&self) -> bool {
        self.status == QualityGateStatus::Error
    }

    /// 获取所有失败的条件结果
    pub fn failed_conditions(&self) -> Vec<&EvaluationResult> {
        self.condition_results
            .iter()
            .filter(|r| r.level == status::Level::Error)
            .collect()
    }
}

/// 质量门层级
///
/// 参按 TODO.md 设计的三层质量门
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum QualityGateLevel {
    /// 终端级 — 每次 checkpoint commit 触发，快速阻断低级错误
    Terminal,
    /// 任务/分支级 — 任务最后一个终端通过后，覆盖整个 task branch
    Branch,
    /// 仓库级 — 合并主分支前的完整检查
    Repo,
}

impl std::fmt::Display for QualityGateLevel {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::Terminal => write!(f, "Terminal Gate"),
            Self::Branch => write!(f, "Branch Gate"),
            Self::Repo => write!(f, "Repo Gate"),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        gate::{
            condition::{Condition, Operator},
            result::EvaluationResult,
        },
        metrics::MetricKey,
    };

    #[test]
    fn test_quality_gate_all_pass() {
        let gate = QualityGate::new(
            "Test Gate",
            vec![
                Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0"),
                Condition::new(MetricKey::TestFailures, Operator::GreaterThan, "0"),
            ],
        );

        let results = vec![
            EvaluationResult::ok(MetricKey::ClippyWarnings, Some(0.into())),
            EvaluationResult::ok(MetricKey::TestFailures, Some(0.into())),
        ];

        let decision = gate.evaluate(&results);
        assert!(decision.is_passed());
        assert_eq!(decision.status, QualityGateStatus::Ok);
    }

    #[test]
    fn test_quality_gate_one_fail() {
        let gate = QualityGate::new(
            "Test Gate",
            vec![Condition::new(
                MetricKey::ClippyWarnings,
                Operator::GreaterThan,
                "0",
            )],
        );

        let results = vec![EvaluationResult::error(
            MetricKey::ClippyWarnings,
            Some(5.into()),
        )];

        let decision = gate.evaluate(&results);
        assert!(decision.is_blocked());
        assert_eq!(decision.failed_conditions().len(), 1);
    }
}
