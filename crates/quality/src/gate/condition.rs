//! 条件模型 — 移植自 SonarQube `Condition.java`
//!
//! 一个条件由度量指标（Metric）、比较操作符（Operator）和阈值（Threshold）组成。
//! 当度量值触发阈值时，条件状态为 ERROR。

use serde::{Deserialize, Serialize};

use crate::metrics::MetricKey;

/// 比较操作符
///
/// 移植自 SonarQube `Condition.Operator`:
/// - `GREATER_THAN` ("GT") — 度量值大于阈值时触发
/// - `LESS_THAN` ("LT") — 度量值小于阈值时触发
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
pub enum Operator {
    /// 大于 — 度量值 > 阈值时条件失败。用于 "错误数不得超过 N"
    #[serde(rename = "GT")]
    GreaterThan,
    /// 小于 — 度量值 < 阈值时条件失败。用于 "覆盖率不得低于 N%"
    #[serde(rename = "LT")]
    LessThan,
}

impl Operator {
    /// 从数据库字符串值解析
    pub fn from_db_value(s: &str) -> anyhow::Result<Self> {
        match s {
            "GT" => Ok(Self::GreaterThan),
            "LT" => Ok(Self::LessThan),
            _ => anyhow::bail!("Unsupported operator value: '{}'", s),
        }
    }

    /// 转为数据库字符串值
    pub fn to_db_value(&self) -> &'static str {
        match self {
            Self::GreaterThan => "GT",
            Self::LessThan => "LT",
        }
    }

    /// 执行比较：给定 comparison 结果（measure.cmp(threshold)），判断是否触发
    pub fn is_triggered(&self, comparison: std::cmp::Ordering) -> bool {
        match self {
            Self::GreaterThan => comparison == std::cmp::Ordering::Greater,
            Self::LessThan => comparison == std::cmp::Ordering::Less,
        }
    }
}

impl std::fmt::Display for Operator {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            Self::GreaterThan => write!(f, ">"),
            Self::LessThan => write!(f, "<"),
        }
    }
}

/// 质量门条件
///
/// 移植自 SonarQube `Condition.java`：
/// ```text
/// Condition = Metric + Operator + ErrorThreshold
/// ```
///
/// 示例：
/// - `clippy_warnings > 0` → "Clippy 警告数不得超过 0"
/// - `test_coverage < 80` → "测试覆盖率不得低于 80%"
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Condition {
    /// 度量指标
    pub metric: MetricKey,
    /// 比较操作符
    pub operator: Operator,
    /// 错误阈值（字符串，支持 int/float/percent）
    pub error_threshold: String,
    /// 是否只检查新增代码的变化量
    /// 参考 SonarQube: metric key 以 "new_" 开头时自动启用
    pub use_variation: bool,
}

impl Condition {
    /// 创建新条件
    pub fn new(metric: MetricKey, operator: Operator, error_threshold: impl Into<String>) -> Self {
        let metric_key_str = metric.as_str();
        Self {
            metric,
            operator,
            error_threshold: error_threshold.into(),
            use_variation: metric_key_str.starts_with("new_"),
        }
    }

    /// 解析阈值为 f64
    pub fn parse_threshold_f64(&self) -> anyhow::Result<f64> {
        self.error_threshold.parse::<f64>().map_err(|e| {
            anyhow::anyhow!(
                "Quality Gate: Unable to parse threshold '{}' for metric {}: {}",
                self.error_threshold,
                self.metric,
                e
            )
        })
    }

    /// 解析阈值为 i64
    pub fn parse_threshold_i64(&self) -> anyhow::Result<i64> {
        // 参考 SonarQube: 含小数点时截断
        if self.error_threshold.contains('.') {
            let dot_pos = self.error_threshold.find('.').unwrap();
            self.error_threshold[..dot_pos]
                .parse::<i64>()
                .map_err(|e| anyhow::anyhow!("Failed to parse threshold: {}", e))
        } else {
            self.error_threshold
                .parse::<i64>()
                .map_err(|e| anyhow::anyhow!("Failed to parse threshold: {}", e))
        }
    }

    /// 生成人类可读的描述
    pub fn description(&self) -> String {
        format!("{} {} {}", self.metric, self.operator, self.error_threshold)
    }
}

impl PartialEq for Condition {
    fn eq(&self, other: &Self) -> bool {
        // 参考 SonarQube: 条件相等性基于 metric 判定
        self.metric == other.metric
    }
}

impl Eq for Condition {}

impl std::hash::Hash for Condition {
    fn hash<H: std::hash::Hasher>(&self, state: &mut H) {
        self.metric.hash(state);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_operator_comparison() {
        assert!(Operator::GreaterThan.is_triggered(std::cmp::Ordering::Greater));
        assert!(!Operator::GreaterThan.is_triggered(std::cmp::Ordering::Equal));
        assert!(!Operator::GreaterThan.is_triggered(std::cmp::Ordering::Less));

        assert!(Operator::LessThan.is_triggered(std::cmp::Ordering::Less));
        assert!(!Operator::LessThan.is_triggered(std::cmp::Ordering::Equal));
        assert!(!Operator::LessThan.is_triggered(std::cmp::Ordering::Greater));
    }

    #[test]
    fn test_condition_equality_by_metric() {
        let c1 = Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0");
        let c2 = Condition::new(MetricKey::ClippyWarnings, Operator::LessThan, "10");
        assert_eq!(c1, c2); // 同 metric 视为相同条件
    }

    #[test]
    fn test_threshold_parsing() {
        let c = Condition::new(MetricKey::TestCoverage, Operator::LessThan, "80.5");
        assert!((c.parse_threshold_f64().unwrap() - 80.5).abs() < f64::EPSILON);
        assert_eq!(c.parse_threshold_i64().unwrap(), 80);
    }

    #[test]
    fn test_use_variation_auto_detect() {
        let c1 = Condition::new(MetricKey::NewBugs, Operator::GreaterThan, "0");
        assert!(c1.use_variation);

        let c2 = Condition::new(MetricKey::ClippyWarnings, Operator::GreaterThan, "0");
        assert!(!c2.use_variation);
    }
}
