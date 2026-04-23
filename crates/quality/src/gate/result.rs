//! 求值结果 — 移植自 SonarQube `EvaluationResult.java` + `ConditionStatus.java`

use serde::{Deserialize, Serialize};

use crate::gate::status::Level;
use crate::metrics::MetricKey;

/// 度量值（统一的可比较值类型）
///
/// 参考 SonarQube `Measure.ValueType`
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum MeasureValue {
    /// 整数值（错误数、警告数等）
    Int(i64),
    /// 浮点值（覆盖率百分比等）
    Float(f64),
    /// 字符串值（质量等级 A/B/C/D/E 等）
    String(String),
    /// 无值
    None,
}

impl MeasureValue {
    /// 与另一个 MeasureValue 比较
    pub fn compare(&self, other: &MeasureValue) -> Option<std::cmp::Ordering> {
        match (self, other) {
            (MeasureValue::Int(a), MeasureValue::Int(b)) => Some(a.cmp(b)),
            (MeasureValue::Float(a), MeasureValue::Float(b)) => a.partial_cmp(b),
            // E37-05: TODO — `i64 as f64` loses precision for |v| > 2^53.
            // Acceptable for current quality metrics (counts, percentages);
            // revisit if metrics with very large integer magnitudes are added.
            (MeasureValue::Int(a), MeasureValue::Float(b)) => (*a as f64).partial_cmp(b),
            (MeasureValue::Float(a), MeasureValue::Int(b)) => a.partial_cmp(&(*b as f64)),
            (MeasureValue::String(a), MeasureValue::String(b)) => Some(a.cmp(b)),
            _ => None,
        }
    }
}

impl From<i64> for MeasureValue {
    fn from(v: i64) -> Self {
        MeasureValue::Int(v)
    }
}

impl From<i32> for MeasureValue {
    fn from(v: i32) -> Self {
        MeasureValue::Int(v as i64)
    }
}

impl From<f64> for MeasureValue {
    fn from(v: f64) -> Self {
        MeasureValue::Float(v)
    }
}

impl From<String> for MeasureValue {
    fn from(v: String) -> Self {
        MeasureValue::String(v)
    }
}

impl std::fmt::Display for MeasureValue {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            MeasureValue::Int(v) => write!(f, "{}", v),
            MeasureValue::Float(v) => write!(f, "{:.2}", v),
            MeasureValue::String(v) => write!(f, "{}", v),
            MeasureValue::None => write!(f, "N/A"),
        }
    }
}

/// 条件求值结果
///
/// 移植自 SonarQube `EvaluationResult.java`
/// 记录单个条件的求值等级和实际度量值
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub struct EvaluationResult {
    /// 求值等级（OK / WARN / ERROR）
    pub level: Level,
    /// 对应的度量指标
    pub metric: MetricKey,
    /// 实际度量值
    pub value: Option<MeasureValue>,
    /// 人类可读消息（失败原因等）
    pub message: Option<String>,
}

impl EvaluationResult {
    /// 创建 OK 结果
    pub fn ok(metric: MetricKey, value: Option<MeasureValue>) -> Self {
        Self {
            level: Level::Ok,
            metric,
            value,
            message: None,
        }
    }

    /// 创建 WARN 结果
    pub fn warn(metric: MetricKey, value: Option<MeasureValue>, message: impl Into<String>) -> Self {
        Self {
            level: Level::Warn,
            metric,
            value,
            message: Some(message.into()),
        }
    }

    /// 创建 ERROR 结果
    pub fn error(metric: MetricKey, value: Option<MeasureValue>) -> Self {
        Self {
            level: Level::Error,
            metric,
            value,
            message: None,
        }
    }

    /// 创建带消息的 ERROR 结果
    pub fn error_with_message(
        metric: MetricKey,
        value: Option<MeasureValue>,
        message: impl Into<String>,
    ) -> Self {
        Self {
            level: Level::Error,
            metric,
            value,
            message: Some(message.into()),
        }
    }
}
