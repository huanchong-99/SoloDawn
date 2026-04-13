//! # SoloDawn Quality Gate Engine
//!
//! 全栈代码质量门引擎，提供代码质量分析、条件求值和报告聚合能力。
//!
//! ## 设计来源
//!
//! 质量门模型设计参考 SonarQube (LGPL-3.0)：
//! - `QualityGate` / `Condition` / `ConditionEvaluator` / `EvaluationResult`
//! - SARIF 2.1.0 报告标准
//!
//! 详情参见: <https://github.com/SonarSource/sonarqube>
//!
//! ## 模块结构
//!
//! - `gate` — 质量门模型：门禁定义、条件、求值引擎、状态
//! - `provider` — 分析器 provider：Rust/Frontend/Repo/Security/Sonar
//! - `engine` — 执行引擎：编排 provider → 收集报告 → 求值 → 决策
//! - `issue` — 质量问题模型
//! - `rule` — 规则类型与严重级别
//! - `sarif` — SARIF 2.1.0 报告解析
//! - `report` — 报告聚合器
//! - `metrics` — 度量指标定义
//! - `config` — 配置加载（quality-gate.yaml）

pub mod analysis;
pub mod config;
pub mod discovery;
pub mod engine;
pub mod gate;
pub mod issue;
pub mod metrics;
pub mod provider;
pub mod report;
pub mod rule;
pub mod rules;
pub mod sarif;
