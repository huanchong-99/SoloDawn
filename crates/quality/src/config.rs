//! 质量门配置加载
//!
//! 从 `quality/quality-gate.yaml` 加载质量门策略定义

use std::path::Path;

use serde::{Deserialize, Serialize};

use crate::{
    gate::{
        QualityGate, QualityGateLevel,
        condition::{Condition, Operator},
    },
    metrics::MetricKey,
};

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
    /// Completeness checks (skeleton services, test absence, migration debris, TODO density)
    #[serde(default = "default_true")]
    pub completeness: bool,
    /// Delivery authenticity/readiness checks for real-entry tests, conventions, runtime smells
    #[serde(default = "default_true")]
    pub delivery_readiness: bool,
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
    "solodawn".to_string()
}

/// SoloDawn 自带的中央 `quality/quality-gate.yaml` 策略 — 编译期嵌入。
///
/// 外部输出仓库（orchestrator 跑出来的项目）通常不会自带 `quality/quality-gate.yaml`，
/// 不应该让 gate 静默跌回到内部的 `default_config`（一个会被空扫描"绿过"的空清单）。
/// 通过 `build.rs` 把策略文件复制到 `$OUT_DIR/quality-gate.yaml`，编译期 inline。
/// 这种做法在 in-workspace / vendored / out-of-tree 三种构建场景下都安全（v1 因
/// 直接 `include_str!("../../../quality/...")` 在 out-of-tree 时崩溃，被主脑驳回）。
pub const BUNDLED_CENTRAL_POLICY: &str =
    include_str!(concat!(env!("OUT_DIR"), "/quality-gate.yaml"));

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
    ///
    /// 解析顺序：
    /// 1. 项目自带的 `quality/quality-gate.yaml` / `.yml` / `.quality-gate.yaml`
    /// 2. SoloDawn 编译期嵌入的中央策略 `BUNDLED_CENTRAL_POLICY`
    /// 3. 最后兜底：`Self::default_config()`（仅当中央策略也解析失败时）
    pub fn load_from_project(project_root: &Path) -> anyhow::Result<Self> {
        let paths = [
            project_root.join("quality/quality-gate.yaml"),
            project_root.join("quality/quality-gate.yml"),
            project_root.join(".quality-gate.yaml"),
        ];

        for path in &paths {
            if path.exists() {
                tracing::debug!(
                    project_root = %project_root.display(),
                    policy = %path.display(),
                    "Loaded quality gate policy from project file",
                );
                return Self::load_from_file(path);
            }
        }

        // Repo 没有本地策略 → 用编译期嵌入的中央 SoloDawn 严格策略
        match Self::from_yaml(BUNDLED_CENTRAL_POLICY) {
            Ok(cfg) => {
                tracing::info!(
                    project_root = %project_root.display(),
                    "Quality gate using bundled central policy (no repo-local quality-gate.yaml found)"
                );
                Ok(cfg)
            }
            Err(e) => {
                // 中央策略本身坏了 → 退到最朴素的 default，避免 orchestrator 整体崩
                tracing::error!(
                    error = %e,
                    "BUNDLED_CENTRAL_POLICY failed to parse; falling back to default_config"
                );
                Ok(Self::default_config())
            }
        }
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
            // NOTE: `EslintErrors` is intentionally absent from branch / repo
            // gates. ESLint is advisory (see `Severity::cap_for_advisory`);
            // its severity is a project-local `.eslintrc` decision that varies
            // per model run, so the gate cannot honor it. Compile (`tsc`) and
            // test failures are the authoritative frontend blockers.
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
                        metric: MetricKey::TscErrors,
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
                        metric: MetricKey::TscErrors,
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

    #[test]
    fn bundled_central_policy_parses_in_enforce_mode() {
        // Compile-time include must always be a valid YAML and configure enforce mode.
        let cfg = QualityGateConfig::from_yaml(BUNDLED_CENTRAL_POLICY)
            .expect("BUNDLED_CENTRAL_POLICY is a hard contract — must always parse");
        assert_eq!(
            cfg.mode,
            QualityGateMode::Enforce,
            "central SoloDawn policy must default to enforce"
        );
        assert!(!cfg.terminal_gate.conditions.is_empty());
        assert!(!cfg.branch_gate.conditions.is_empty());
        assert!(!cfg.repo_gate.conditions.is_empty());
    }

    #[test]
    fn load_from_project_falls_back_to_bundled_when_repo_has_no_policy() {
        // Simulate an external orchestrator-output repo that does NOT carry any
        // local quality-gate.yaml. The loader must hand back the bundled
        // central enforce policy, NOT the lenient shadow default_config.
        let dir =
            std::env::temp_dir().join(format!("quality-config-fallback-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(&dir).unwrap();

        let cfg = QualityGateConfig::load_from_project(&dir).unwrap();
        assert_eq!(
            cfg.mode,
            QualityGateMode::Enforce,
            "external repo without local policy must inherit central enforce mode \
             (root cause of Task 1 R3 false-pass)"
        );

        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn load_from_project_prefers_repo_local_policy_over_bundled() {
        // If the repo brings its own policy, that wins.
        let dir =
            std::env::temp_dir().join(format!("quality-config-local-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(dir.join("quality")).unwrap();
        std::fs::write(
            dir.join("quality/quality-gate.yaml"),
            r#"mode: shadow
terminal_gate:
  name: "Local Override"
  conditions:
    - metric: cargo_check_errors
      operator: "GT"
      threshold: "0"
branch_gate:
  name: "Local Branch"
  conditions: []
repo_gate:
  name: "Local Repo"
  conditions: []
providers:
  rust: true
  frontend: true
  repo: false
  security: false
  sonar: false
  builtin_rust: true
  builtin_frontend: true
  builtin_common: true
  coverage: false
sonar:
  host_url: ""
  project_key: ""
"#,
        )
        .unwrap();

        let cfg = QualityGateConfig::load_from_project(&dir).unwrap();
        assert_eq!(cfg.mode, QualityGateMode::Shadow);
        assert_eq!(cfg.terminal_gate.name, "Local Override");

        let _ = std::fs::remove_dir_all(&dir);
    }
}
