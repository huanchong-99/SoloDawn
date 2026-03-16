//! 质量门配置加载
//!
//! 从 `quality/quality-gate.yaml` 加载质量门策略定义

use serde::{Deserialize, Serialize};
use std::path::Path;

use crate::gate::condition::{Condition, Operator};
use crate::gate::{QualityGate, QualityGateLevel};
use crate::metrics::MetricKey;

/// 质量门配置文件结构
///
/// 对应 `quality/quality-gate.yaml`
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct QualityGateConfig {
    /// 质量门模式
    pub mode: QualityGateMode,
    /// 终端级质量门
    pub terminal_gate: GateDefinition,
    /// 分支级质量门
    pub branch_gate: GateDefinition,
    /// 仓库级质量门
    pub repo_gate: GateDefinition,
    /// Provider 配置
    #[serde(default)]
    pub providers: ProvidersConfig,
    /// SonarQube 配置
    #[serde(default)]
    pub sonar: SonarConfig,
}

/// 质量门运行模式
///
/// 参考 TODO.md D8: feature flag `QUALITY_GATE_MODE`
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum QualityGateMode {
    /// 关闭 — 走旧流程，质量门完全不参与
    Off,
    /// 影子模式 — 运行分析、记录结果，但不阻断任何流程
    #[default]
    Shadow,
    /// 警告模式 — 分析并回流问题到终端，但不硬性阻断合并
    Warn,
    /// 强制模式 — 硬性门禁，不通过则阻断
    Enforce,
}

/// 单个质量门定义
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GateDefinition {
    /// 门禁名称
    pub name: String,
    /// 条件列表
    pub conditions: Vec<ConditionConfig>,
}

/// 条件配置（YAML 友好格式）
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConditionConfig {
    /// 度量指标
    pub metric: MetricKey,
    /// 操作符: "GT" 或 "LT"
    pub operator: String,
    /// 阈值
    pub threshold: String,
}

impl ConditionConfig {
    /// 转换为 Condition
    pub fn to_condition(&self) -> anyhow::Result<Condition> {
        let operator = Operator::from_db_value(&self.operator)?;
        Ok(Condition::new(self.metric, operator, &self.threshold))
    }
}

/// Provider 配置
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ProvidersConfig {
    #[serde(default = "default_true")]
    pub rust: bool,
    #[serde(default = "default_true")]
    pub frontend: bool,
    #[serde(default = "default_true")]
    pub repo: bool,
    #[serde(default = "default_true")]
    pub security: bool,
    #[serde(default = "default_true")]
    pub sonar: bool,
    /// Built-in Rust static analysis rules
    #[serde(default = "default_true")]
    pub builtin_rust: bool,
    /// Built-in TypeScript/JavaScript static analysis rules
    #[serde(default = "default_true")]
    pub builtin_frontend: bool,
    /// Built-in language-agnostic rules (duplication, secrets, etc.)
    #[serde(default = "default_true")]
    pub builtin_common: bool,
    /// Coverage report parsing (lcov, cobertura, tarpaulin)
    #[serde(default = "default_true")]
    pub coverage: bool,
}

fn default_true() -> bool {
    true
}

/// SonarQube 配置
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SonarConfig {
    /// SonarQube 服务地址
    #[serde(default = "default_sonar_host")]
    pub host_url: String,
    /// 项目 key
    #[serde(default = "default_project_key")]
    pub project_key: String,
    /// 认证 token（可通过环境变量 SONAR_TOKEN 覆盖）
    pub token: Option<String>,
}

impl Default for SonarConfig {
    fn default() -> Self {
        Self {
            host_url: default_sonar_host(),
            project_key: default_project_key(),
            token: None,
        }
    }
}

fn default_sonar_host() -> String {
    "http://localhost:9000".to_string()
}

fn default_project_key() -> String {
    "gitcortex".to_string()
}

impl QualityGateConfig {
    /// 从 YAML 文件加载配置
    pub fn load_from_file(path: &Path) -> anyhow::Result<Self> {
        let content = std::fs::read_to_string(path)
            .map_err(|e| anyhow::anyhow!("Failed to read quality gate config: {}", e))?;
        Self::from_yaml(&content)
    }

    /// 从 YAML 字符串解析
    pub fn from_yaml(yaml: &str) -> anyhow::Result<Self> {
        serde_yaml::from_str(yaml)
            .map_err(|e| anyhow::anyhow!("Failed to parse quality gate config: {}", e))
    }

    /// 从项目根目录自动查找并加载
    pub fn load_from_project(project_root: &Path) -> anyhow::Result<Self> {
        let paths = [
            project_root.join("quality/quality-gate.yaml"),
            project_root.join("quality/quality-gate.yml"),
            project_root.join(".quality-gate.yaml"),
        ];

        for path in &paths {
            if path.exists() {
                return Self::load_from_file(path);
            }
        }

        // 默认配置
        Ok(Self::default_config())
    }

    /// 默认无硬性配置
    pub fn default_config() -> Self {
        Self {
            mode: QualityGateMode::Shadow,
            terminal_gate: GateDefinition {
                name: "Terminal Gate (Default)".to_string(),
                conditions: vec![
                    ConditionConfig {
                        metric: MetricKey::CargoCheckErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::ClippyErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::TscErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::RustTestFailures,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                ],
            },
            branch_gate: GateDefinition {
                name: "Branch Gate (Default)".to_string(),
                conditions: vec![
                    ConditionConfig {
                        metric: MetricKey::CargoCheckErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::ClippyWarnings,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::EslintErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::TestFailures,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                ],
            },
            repo_gate: GateDefinition {
                name: "Repo Gate (Default)".to_string(),
                conditions: vec![
                    ConditionConfig {
                        metric: MetricKey::CargoCheckErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::ClippyWarnings,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::EslintErrors,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::FrontendTestFailures,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                    ConditionConfig {
                        metric: MetricKey::SonarBlockerIssues,
                        operator: "GT".to_string(),
                        threshold: "0".to_string(),
                    },
                ],
            },
            providers: ProvidersConfig::default(),
            sonar: SonarConfig::default(),
        }
    }

    /// 获取指定层级的 QualityGate 实例
    pub fn get_gate(&self, level: QualityGateLevel) -> anyhow::Result<QualityGate> {
        let def = match level {
            QualityGateLevel::Terminal => &self.terminal_gate,
            QualityGateLevel::Branch => &self.branch_gate,
            QualityGateLevel::Repo => &self.repo_gate,
        };

        let conditions: Vec<Condition> = def
            .conditions
            .iter()
            .map(|c| c.to_condition())
            .collect::<anyhow::Result<Vec<_>>>()?;

        Ok(QualityGate::new(&def.name, conditions))
    }

    /// 是否启用质量门
    pub fn is_enabled(&self) -> bool {
        self.mode != QualityGateMode::Off
    }

    /// 是否为强制模式（阻断）
    pub fn is_enforcing(&self) -> bool {
        self.mode == QualityGateMode::Enforce
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_default_config() {
        let config = QualityGateConfig::default_config();
        assert_eq!(config.mode, QualityGateMode::Shadow);
        assert!(!config.terminal_gate.conditions.is_empty());
    }

    #[test]
    fn test_yaml_roundtrip() {
        let config = QualityGateConfig::default_config();
        let yaml = serde_yaml::to_string(&config).unwrap();
        let parsed = QualityGateConfig::from_yaml(&yaml).unwrap();
        assert_eq!(parsed.mode, config.mode);
    }

    #[test]
    fn test_get_gate() {
        let config = QualityGateConfig::default_config();
        let gate = config.get_gate(QualityGateLevel::Terminal).unwrap();
        assert!(!gate.conditions.is_empty());
    }
}
