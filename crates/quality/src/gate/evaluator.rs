//! 条件求值引擎 — 移植自 SonarQube `ConditionEvaluator.java`
//!
//! 核心逻辑：给定一个 Condition 和一个度量值，判定是否触发阈值。
//!
//! 参考 SonarQube 实现:
//! 1. 检查度量类型是否支持
//! 2. 解析度量值为可比较类型
//! 3. 解析阈值为同类型可比较值
//! 4. 按操作符执行比较
//! 5. 返回 EvaluationResult (Level + Value)

use crate::{
    gate::{
        condition::Condition,
        result::{EvaluationResult, MeasureValue},
    },
    metrics::MetricKey,
};

/// 条件求值引擎
///
/// 移植自 SonarQube `ConditionEvaluator`
/// 无状态，所有方法为纯函数
pub struct ConditionEvaluator;

impl ConditionEvaluator {
    /// 对单个条件求值
    ///
    /// # 参数
    /// - `condition`: 质量门条件定义
    /// - `measure_value`: 实际度量值
    ///
    /// # 返回
    /// - `EvaluationResult`: 求值等级和实际值
    pub fn evaluate(
        condition: &Condition,
        measure_value: Option<&MeasureValue>,
    ) -> EvaluationResult {
        // Missing metric → ERROR (fail-closed).  If a metric is defined as a gate
        // condition but has no value, quality was NOT verified — this must block in
        // enforce mode.  Previously returned WARN which `is_passed()` treated as
        // passed, silently letting unverified code through.
        let measure = match measure_value {
            Some(v) => v,
            None => {
                return EvaluationResult::error_with_message(
                    condition.metric,
                    None,
                    format!(
                        "Metric {} has no value — provider failed or metric not collected; quality cannot be verified",
                        condition.metric
                    ),
                );
            }
        };

        // G33-001: Sentinel MeasureValue::Int(-1) means "metric was not collected"
        // (tool couldn't run, dependencies missing, command failed silently, etc.).
        // Previously `-1 GT 0` evaluated to false and silently passed the gate.
        // Treat -1 as Unknown and fail-closed — the gate CANNOT verify quality.
        if let MeasureValue::Int(-1) = measure {
            return EvaluationResult::error_with_message(
                condition.metric,
                Some(measure.clone()),
                format!(
                    "Metric {} unavailable (-1 sentinel) — tool did not run, quality cannot be verified",
                    condition.metric
                ),
            );
        }

        // 解析阈值为可比较值
        let threshold = match Self::parse_threshold(condition) {
            Ok(t) => t,
            Err(e) => {
                return EvaluationResult::error_with_message(
                    condition.metric,
                    Some(measure.clone()),
                    format!("Failed to parse threshold: {}", e),
                );
            }
        };

        // 执行比较
        match measure.compare(&threshold) {
            Some(ordering) => {
                if condition.operator.is_triggered(ordering) {
                    EvaluationResult::error_with_message(
                        condition.metric,
                        Some(measure.clone()),
                        format!(
                            "{} is {} (threshold: {} {})",
                            condition.metric,
                            measure,
                            condition.operator,
                            condition.error_threshold
                        ),
                    )
                } else {
                    EvaluationResult::ok(condition.metric, Some(measure.clone()))
                }
            }
            None => {
                // 类型不匹配，无法比较
                EvaluationResult::error_with_message(
                    condition.metric,
                    Some(measure.clone()),
                    format!(
                        "Cannot compare measure value {} with threshold {}",
                        measure, condition.error_threshold
                    ),
                )
            }
        }
    }

    /// 批量求值多个条件
    pub fn evaluate_all(
        conditions: &[Condition],
        measures: &std::collections::HashMap<MetricKey, MeasureValue>,
    ) -> Vec<EvaluationResult> {
        conditions
            .iter()
            .map(|condition| {
                let measure = measures.get(&condition.metric);
                Self::evaluate(condition, measure)
            })
            .collect()
    }

    /// 解析条件阈值为 MeasureValue
    ///
    /// 参考 SonarQube `parseConditionValue`:
    /// 根据度量的类型选择解析方式
    fn parse_threshold(condition: &Condition) -> anyhow::Result<MeasureValue> {
        let threshold_str = &condition.error_threshold;

        // 尝试按整数解析
        if let Ok(v) = threshold_str.parse::<i64>() {
            return Ok(MeasureValue::Int(v));
        }

        // 尝试按浮点数解析
        if let Ok(v) = threshold_str.parse::<f64>() {
            return Ok(MeasureValue::Float(v));
        }

        // 字符串值（质量等级等）
        Ok(MeasureValue::String(threshold_str.clone()))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{
        gate::{
            condition::{Condition, Operator},
            status::Level,
        },
        metrics::MetricKey,
    };

    #[test]
    fn test_evaluate_gt_triggered() {
        let condition = Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0");
        let measure = MeasureValue::Int(5);
        let result = ConditionEvaluator::evaluate(&condition, Some(&measure));
        assert_eq!(result.level, Level::Error);
    }

    #[test]
    fn test_evaluate_gt_not_triggered() {
        let condition = Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0");
        let measure = MeasureValue::Int(0);
        let result = ConditionEvaluator::evaluate(&condition, Some(&measure));
        assert_eq!(result.level, Level::Ok);
    }

    #[test]
    fn test_evaluate_lt_triggered() {
        let condition = Condition::new(MetricKey::TestCoverage, Operator::LessThan, "80");
        let measure = MeasureValue::Float(65.5);
        let result = ConditionEvaluator::evaluate(&condition, Some(&measure));
        assert_eq!(result.level, Level::Error);
    }

    #[test]
    fn test_evaluate_lt_not_triggered() {
        let condition = Condition::new(MetricKey::TestCoverage, Operator::LessThan, "80");
        let measure = MeasureValue::Float(95.0);
        let result = ConditionEvaluator::evaluate(&condition, Some(&measure));
        assert_eq!(result.level, Level::Ok);
    }

    #[test]
    fn test_evaluate_no_measure() {
        let condition = Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0");
        let result = ConditionEvaluator::evaluate(&condition, None);
        assert_eq!(result.level, Level::Error);
        assert!(result.message.is_some());
    }

    #[test]
    fn test_evaluate_minus_one_is_unknown() {
        // G33-001: -1 means the metric was not collected; gate must fail-closed,
        // not silently pass via `-1 GT 0 = false`.
        let condition = Condition::new(MetricKey::TscErrors, Operator::GreaterThan, "0");
        let measure = MeasureValue::Int(-1);
        let result = ConditionEvaluator::evaluate(&condition, Some(&measure));
        assert_eq!(result.level, Level::Error);
        assert!(
            result
                .message
                .unwrap()
                .to_lowercase()
                .contains("unavailable")
        );
    }

    #[test]
    fn test_evaluate_all() {
        let conditions = vec![
            Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0"),
            Condition::new(MetricKey::TestFailures, Operator::GreaterThan, "0"),
            Condition::new(MetricKey::TestCoverage, Operator::LessThan, "80"),
        ];

        let mut measures = std::collections::HashMap::new();
        measures.insert(MetricKey::ClippyWarnings, MeasureValue::Int(0));
        measures.insert(MetricKey::TestFailures, MeasureValue::Int(2));
        measures.insert(MetricKey::TestCoverage, MeasureValue::Float(90.0));

        let results = ConditionEvaluator::evaluate_all(&conditions, &measures);

        assert_eq!(results[0].level, Level::Ok); // clippy: 0 not > 0
        assert_eq!(results[1].level, Level::Error); // tests: 2 > 0
        assert_eq!(results[2].level, Level::Ok); // coverage: 90 not < 80
    }
}
