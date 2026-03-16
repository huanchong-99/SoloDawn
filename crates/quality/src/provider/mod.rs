//! 分析器 Provider 接口
//!
//! 可插拔的分析器 provider 架构。每个 provider 负责执行特定工具并收集结果。

pub mod builtin_common;
pub mod builtin_frontend;
pub mod builtin_rust;
pub mod coverage;
pub mod frontend;
pub mod repo;
pub mod rust_analyzer;
pub mod security;
pub mod sonar;

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;

use crate::gate::result::MeasureValue;
use crate::issue::QualityIssue;
use crate::metrics::MetricKey;

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
    pub fn failure(provider_name: impl Into<String>, duration_ms: u64, error: impl Into<String>) -> Self {
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
        changed_files: Option<&[String]>,
    ) -> anyhow::Result<ProviderReport>;

    /// 获取 provider 支持的度量指标
    fn supported_metrics(&self) -> Vec<MetricKey>;
}
