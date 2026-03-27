# Phase 8.5: 代码质量修复

> **状态:** 🚨 紧急
> **进度追踪:** 查看 `TODO.md`
> **前置条件:** Phase 7 完成
> **来源:** 2026-01-19 代码审计报告 (评级: C)

## 概述

根据 Senior Code Auditor 的深度审计，项目当前评分为 **C (68/100)**，存在严重的代码质量问题。本 Phase 专注于修复这些问题，确保代码达到生产可用标准。

**审计结论:** Conditional Pass - 功能完整但实现方式低劣，存在严重安全漏洞。

---

## P0 - 严重问题修复 (生产环境阻塞)

> 这些问题必须立即修复，否则项目无法进入生产环境。

### Task 8.5.1: 实现 execute_instruction 核心逻辑

**优先级:** P0 - 严重

**问题:** `crates/services/src/services/orchestrator/agent.rs:211` 处的核心功能只有 TODO 占位符，Orchestrator 无法真正控制终端。

**涉及文件:**
- `crates/services/src/services/orchestrator/agent.rs`
- `crates/services/src/services/orchestrator/llm.rs`
- `crates/services/src/services/orchestrator/message_bus.rs`

---

**Step 8.5.1.1: 实现 SendToTerminal 指令**

```rust
OrchestratorInstruction::SendToTerminal { terminal_id, message } => {
    tracing::info!("Sending to terminal {}: {}", terminal_id, message);

    // 1. 获取终端信息
    let terminal = self.db.get_terminal(&terminal_id).await
        .map_err(|e| anyhow::anyhow!("Failed to get terminal: {}", e))?;

    // 2. 获取终端的 PTY 会话
    let pty_session_id = terminal.pty_session_id
        .ok_or_else(|| anyhow::anyhow!("Terminal {} has no PTY session", terminal_id))?;

    // 3. 通过消息总线发送消息
    self.message_bus.send_to_terminal(
        &pty_session_id,
        &message
    ).await
    .map_err(|e| anyhow::anyhow!("Failed to send message: {}", e))?;

    tracing::debug!("Message sent to terminal {}", terminal_id);
}
```

---

**Step 8.5.1.2: 实现 CompleteWorkflow 指令**

```rust
OrchestratorInstruction::CompleteWorkflow { success, reason } => {
    tracing::info!(
        "Completing workflow: success={}, reason={}",
        success, reason
    );

    // 1. 更新工作流状态
    let new_status = if success {
        WorkflowStatus::Completed
    } else {
        WorkflowStatus::Failed
    };

    workflow_dao::update_workflow_status(
        &self.db.pool,
        &self.config.workflow_id,
        new_status
    ).await
    .map_err(|e| anyhow::anyhow!("Failed to update workflow status: {}", e))?;

    // 2. 发送工作流完成事件
    self.message_bus.publish(
        &format!("workflow:{}", self.config.workflow_id),
        serde_json::to_vec(&WorkflowEvent::Completed {
            workflow_id: self.config.workflow_id.clone(),
            success,
            reason: reason.clone(),
        })?
    ).await
    .map_err(|e| anyhow::anyhow!("Failed to publish completion event: {}", e))?;

    // 3. 停止主 Agent
    self.state.write().await.run_state = OrchestratorRunState::Idle;
}
```

---

**Step 8.5.1.3: 实现 FailWorkflow 指令**

```rust
OrchestratorInstruction::FailWorkflow { error, severity } => {
    tracing::error!("Workflow failed: {} (severity: {:?})", error, severity);

    // 1. 根据严重程度决定是否启动错误处理终端
    if let Some(error_terminal_id) = &self.config.error_terminal_id {
        if matches!(severity, ErrorSeverity::Major | ErrorSeverity::Critical) {
            // 向错误处理终端发送错误信息
            let error_message = format!(
                "[ERROR] Workflow failed: {}\n\nPlease investigate and fix.",
                error
            );
            // 发送到错误处理终端...
        }
    }

    // 2. 更新工作流状态为失败
    workflow_dao::update_workflow_status(
        &self.db.pool,
        &self.config.workflow_id,
        WorkflowStatus::Failed
    ).await?;

    // 3. 发送失败事件
    self.message_bus.publish(
        &format!("workflow:{}", self.config.workflow_id),
        serde_json::to_vec(&WorkflowEvent::Failed {
            workflow_id: self.config.workflow_id.clone(),
            error,
            severity,
        })?
    ).await?;

    self.state.write().await.run_state = OrchestratorRunState::Idle;
}
```

---

**交付物:** 完整实现的 `execute_instruction` 方法

**验收标准:**
- [ ] 所有指令分支都有实际实现
- [ ] 没有 TODO 占位符
- [ ] 所有错误路径都有日志记录
- [ ] 单元测试覆盖所有指令类型

---

### Task 8.5.2: API Key 加密存储

**优先级:** P0 - 严重 (安全漏洞)

**问题:** `crates/db/src/models/workflow.rs:106` 处的 API Key 以明文存储，存在严重安全风险。

**涉及文件:**
- `crates/db/src/models/workflow.rs`
- `Cargo.toml` (添加依赖)

---

**Step 8.5.2.1: 添加加密依赖**

```toml
# Cargo.toml
[dependencies]
aes-gcm = "0.10"
base64 = "0.21"
rand = "0.8"
```

---

**Step 8.5.2.2: 重构 Workflow 模型**

```rust
use aes_gcm::{
    aead::{Aead, AeadCore, KeyInit, OsRng},
    Aes256Gcm, Nonce, Key
};
use base64::{Engine as _, engine::general_purpose};

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Workflow {
    pub workflow_id: String,
    // ... 其他字段 ...

    // ❌ 删除明文字段
    // pub orchestrator_api_key: Option<String>,

    // ✅ 添加加密字段
    #[serde(skip_serializing)]  // 不直接序列化到 API 响应
    pub orchestrator_api_key_encrypted: Option<String>,

    // ... 其他字段 ...
}

impl Workflow {
    const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";

    /// 获取加密密钥 (从环境变量)
    fn get_encryption_key() -> anyhow::Result<[u8; 32]> {
        std::env::var(Self::ENCRYPTION_KEY_ENV)
            .map_err(|_| anyhow::anyhow!(
                "Encryption key not found. Please set {} environment variable.",
                Self::ENCRYPTION_KEY_ENV
            ))?
            .as_bytes()
            .try_into()
            .map_err(|_| anyhow::anyhow!("Invalid encryption key length. Must be 32 bytes."))
    }

    /// 设置 API Key (加密存储)
    pub fn set_api_key(&mut self, plaintext: &str) -> anyhow::Result<()> {
        let key = Self::get_encryption_key()?;
        let cipher = Aes256Gcm::new(Key::<Aes256Gcm>::from_slice(&key));
        let nonce = Aes256Gcm::generate_nonce(&mut OsRng);

        let ciphertext = cipher.encrypt(&nonce, plaintext.as_bytes())
            .map_err(|e| anyhow::anyhow!("Encryption failed: {}", e))?;

        // 组合 nonce + ciphertext
        let mut combined = nonce.to_vec();
        combined.extend_from_slice(&ciphertext);

        // Base64 编码
        self.orchestrator_api_key_encrypted = Some(
            general_purpose::STANDARD.encode(&combined)
        );

        Ok(())
    }

    /// 获取 API Key (解密)
    pub fn get_api_key(&self) -> anyhow::Result<Option<String>> {
        match &self.orchestrator_api_key_encrypted {
            None => Ok(None),
            Some(encoded) => {
                let key = Self::get_encryption_key()?;
                let combined = general_purpose::STANDARD.decode(encoded)
                    .map_err(|e| anyhow::anyhow!("Base64 decode failed: {}", e))?;

                if combined.len() < 12 {
                    return Err(anyhow::anyhow!("Invalid encrypted data length"));
                }

                let (nonce_bytes, ciphertext) = combined.split_at(12);
                let nonce = Nonce::from_slice(nonce_bytes);
                let cipher = Aes256Gcm::new(Key::<Aes256Gcm>::from_slice(&key));

                let plaintext_bytes = cipher.decrypt(nonce, ciphertext)
                    .map_err(|e| anyhow::anyhow!("Decryption failed: {}", e))?;

                Ok(Some(String::from_utf8(plaintext_bytes)
                    .map_err(|e| anyhow::anyhow!("Invalid UTF-8: {}", e))?))
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_encryption_decryption() {
        std::env::set_var("SOLODAWN_ENCRYPTION_KEY", "12345678901234567890123456789012");

        let mut workflow = Workflow {
            workflow_id: "test".to_string(),
            orchestrator_api_key_encrypted: None,
            // ... 其他字段 ...
        };

        // 测试加密
        workflow.set_api_key("sk-test-key-12345").unwrap();
        assert!(workflow.orchestrator_api_key_encrypted.is_some());

        // 测试解密
        let decrypted = workflow.get_api_key().unwrap().unwrap();
        assert_eq!(decrypted, "sk-test-key-12345");
    }
}
```

---

**Step 8.5.2.3: 创建数据库迁移**

```sql
-- migrations/YYYYMMDDHHMMSS_encrypt_api_keys.sql

-- 1. 添加新的加密列
ALTER TABLE workflow ADD COLUMN orchestrator_api_key_encrypted TEXT;

-- 2. 迁移现有数据 (需要在应用层处理，因为需要加密密钥)
-- 这一步在应用启动时处理

-- 3. 删除旧列 (在确认迁移成功后)
-- ALTER TABLE workflow DROP COLUMN orchestrator_api_key;
```

---

**Step 8.5.2.4: 应用启动时迁移数据**

```rust
// 在 DBService::new 中添加
pub async fn migrate_api_keys_to_encrypted(pool: &SqlitePool) -> anyhow::Result<()> {
    let encryption_key = std::env::var("SOLODAWN_ENCRYPTION_KEY")
        .unwrap_or_default();

    if encryption_key.is_empty() || encryption_key.len() != 32 {
        tracing::warn!("Encryption key not set, skipping API key migration");
        return Ok(());
    }

    // 查询所有有明文 API Key 的工作流
    let workflows = sqlx::query!(
        "SELECT workflow_id, orchestrator_api_key FROM workflow WHERE orchestrator_api_key IS NOT NULL"
    )
    .fetch_all(pool)
    .await?;

    for row in workflows {
        if let Some(plaintext) = row.orchestrator_api_key {
            let mut workflow = Workflow { /* ... */ };
            workflow.set_api_key(&plaintext)?;

            sqlx::query!(
                "UPDATE workflow SET orchestrator_api_key_encrypted = ? WHERE workflow_id = ?",
                workflow.orchestrator_api_key_encrypted,
                row.workflow_id
            )
            .execute(pool)
            .await?;
        }
    }

    Ok(())
}
```

---

**交付物:** 完整的 API Key 加密/解密实现

**验收标准:**
- [ ] API Key 以加密形式存储在数据库中
- [ ] 提供加密/解密方法
- [ ] 单元测试验证加解密正确性
- [ ] 现有数据迁移脚本
- [ ] 环境变量配置说明文档

---

### Task 8.5.3: 实现 handle_git_event 实际逻辑

**优先级:** P0 - 严重

**问题:** `crates/services/src/services/orchestrator/agent.rs:156-172` 处的 Git 事件处理完全是空实现。

**涉及文件:**
- `crates/services/src/services/orchestrator/agent.rs`
- `crates/services/src/services/git_watcher/commit_parser.rs`

---

**Step 8.5.3.1: 解析 Git 提交元数据**

```rust
async fn handle_git_event(
    &self,
    workflow_id: &str,
    commit_hash: &str,
    branch: &str,
    message: &str,
) -> anyhow::Result<()> {
    tracing::info!(
        "Git event: {} on branch {} - {}",
        commit_hash, branch, message
    );

    // 1. 解析提交信息中的元数据
    let metadata = commit_parser::parse_commit_metadata(message)?;

    // 2. 验证元数据中的 workflow_id 是否匹配
    if metadata.workflow_id != workflow_id {
        tracing::warn!(
            "Workflow ID mismatch: expected {}, got {}",
            workflow_id, metadata.workflow_id
        );
        return Ok(());
    }

    // 3. 根据状态类型生成相应事件
    match metadata.status.as_str() {
        "completed" => {
            self.handle_terminal_completed(
                &metadata.terminal_id,
                &metadata.task_id,
                commit_hash,
                message,
                metadata.files_changed,
            ).await?;
        }
        "review_pass" => {
            self.handle_review_pass(
                &metadata.terminal_id,
                &metadata.task_id,
                &metadata.reviewed_terminal,
            ).await?;
        }
        "review_reject" => {
            self.handle_review_reject(
                &metadata.terminal_id,
                &metadata.task_id,
                &metadata.reviewed_terminal,
                &metadata.issues,
            ).await?;
        }
        "failed" => {
            self.handle_terminal_failed(
                &metadata.terminal_id,
                &metadata.task_id,
                message,
            ).await?;
        }
        _ => {
            tracing::warn!("Unknown status in commit: {}", metadata.status);
        }
    }

    Ok(())
}
```

---

**Step 8.5.3.2: 实现终端完成处理**

```rust
async fn handle_terminal_completed(
    &self,
    terminal_id: &str,
    task_id: &str,
    commit_hash: &str,
    commit_message: &str,
    files_changed: Vec<FileChange>,
) -> anyhow::Result<()> {
    tracing::info!(
        "Terminal {} completed task {} (commit: {})",
        terminal_id, task_id, commit_hash
    );

    // 1. 更新终端状态
    terminal_dao::update_terminal_status(
        &self.db.pool,
        terminal_id,
        TerminalStatus::Completed
    ).await?;

    // 2. 发送终端完成事件
    let event = WorkflowEvent::TerminalCompleted {
        workflow_id: self.config.workflow_id.clone(),
        task_id: task_id.to_string(),
        terminal_id: terminal_id.to_string(),
        commit_hash: commit_hash.to_string(),
        commit_message: commit_message.to_string(),
        files_changed,
    };

    self.message_bus.publish(
        &format!("workflow:{}", self.config.workflow_id),
        serde_json::to_vec(&event)?
    ).await?;

    // 3. 唤醒主 Agent 处理
    self.awaken().await?;

    Ok(())
}
```

---

**Step 8.5.3.3: 实现审核通过处理**

```rust
async fn handle_review_pass(
    &self,
    reviewer_terminal_id: &str,
    task_id: &str,
    reviewed_terminal_id: &str,
) -> anyhow::Result<()> {
    tracing::info!(
        "Terminal {} approved work from {}",
        reviewer_terminal_id, reviewed_terminal_id
    );

    // 1. 更新被审核终端的状态
    terminal_dao::update_terminal_status(
        &self.db.pool,
        reviewed_terminal_id,
        TerminalStatus::ReviewPassed
    ).await?;

    // 2. 发送审核通过事件
    let event = WorkflowEvent::ReviewPassed {
        workflow_id: self.config.workflow_id.clone(),
        task_id: task_id.to_string(),
        reviewer_terminal_id: reviewer_terminal_id.to_string(),
        reviewed_terminal_id: reviewed_terminal_id.to_string(),
    };

    self.message_bus.publish(
        &format!("workflow:{}", self.config.workflow_id),
        serde_json::to_vec(&event)?
    ).await?;

    // 3. 唤醒主 Agent
    self.awaken().await?;

    Ok(())
}
```

---

**Step 8.5.3.4: 实现审核打回处理**

```rust
async fn handle_review_reject(
    &self,
    reviewer_terminal_id: &str,
    task_id: &str,
    reviewed_terminal_id: &str,
    issues: Vec<Issue>,
) -> anyhow::Result<()> {
    tracing::warn!(
        "Terminal {} rejected work from {}: {} issues found",
        reviewer_terminal_id, reviewed_terminal_id, issues.len()
    );

    // 1. 更新被审核终端状态
    terminal_dao::update_terminal_status(
        &self.db.pool,
        reviewed_terminal_id,
        TerminalStatus::ReviewRejected
    ).await?;

    // 2. 发送审核打回事件
    let event = WorkflowEvent::ReviewRejected {
        workflow_id: self.config.workflow_id.clone(),
        task_id: task_id.to_string(),
        reviewer_terminal_id: reviewer_terminal_id.to_string(),
        reviewed_terminal_id: reviewed_terminal_id.to_string(),
        issues,
    };

    self.message_bus.publish(
        &format!("workflow:{}", self.config.workflow_id),
        serde_json::to_vec(&event)?
    ).await?;

    // 3. 唤醒主 Agent
    self.awaken().await?;

    Ok(())
}
```

---

**交付物:** 完整的 Git 事件处理逻辑

**验收标准:**
- [ ] 所有状态类型都有对应处理
- [ ] 事件正确发送到消息总线
- [ ] 终端状态正确更新
- [ ] 单元测试覆盖所有状态类型

---

## P1 - 代码清理

### Task 8.5.4: 移除未使用的导入

**优先级:** P1

**问题:** 编译时有 7 个未使用导入警告。

**涉及文件:**
- `crates/services/src/services/orchestrator/agent.rs`
- 其他有警告的文件

---

**Step 8.5.4.1: 运行编译检查**

```bash
cargo clippy --warnings 2>&1 | grep "unused"
```

---

**Step 8.5.4.2: 移除未使用的导入**

示例修复:
```rust
// ❌ 移除
use std::collections::HashMap;  // 未使用

// ✅ 保留确实使用的导入
use anyhow::{anyhow, Result};
use serde_json::json;
```

---

**交付物:** 无编译警告的代码

**验收标准:**
- [ ] `cargo clippy` 无未使用导入警告
- [ ] `cargo build` 无警告

---

### Task 8.5.5: 移除/使用未使用的 db 字段

**优先级:** P1

**问题:** `OrchestratorAgent` 结构体中的 `db` 字段从未使用，触发 `dead_code` 警告。

**涉及文件:**
- `crates/services/src/services/orchestrator/agent.rs`

---

**Step 8.5.5.1: 分析 db 字段是否需要**

在 `execute_instruction` 实现中，需要查询终端信息，因此需要保留 `db` 字段。

---

**Step 8.5.5.2: 在 execute_instruction 中使用 db 字段**

已在 Task 8.5.1 的实现中使用。

---

**交付物:** 无 dead_code 警告

**验收标准:**
- [ ] `db` 字段被实际使用
- [ ] 无 `dead_code` 警告

---

### Task 8.5.6: 统一命名规范

**优先级:** P1

**问题:** Rust 和 TypeScript 命名不一致，缺少 `serde` 配置。

**涉及文件:**
- `crates/db/src/models/*.rs`
- `frontend/src/types/*.ts`

---

**Step 8.5.6.1: 统一 Rust Serde 配置**

```rust
// crates/db/src/models/workflow.rs

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]  // ← 添加这行
pub struct Workflow {
    pub workflow_id: String,
    pub cli_type_id: String,
    pub orchestrator_api_key_encrypted: Option<String>,
    pub order_index: i32,
    // ...
}
```

---

**Step 8.5.6.2: 统一 TypeScript 类型定义**

```typescript
// frontend/src/types/workflow.ts

export interface Workflow {
  workflowId: string;      // camelCase
  cliTypeId: string;       // camelCase
  orchestratorApiKeyEncrypted?: string;  // camelCase
  orderIndex: number;      // camelCase
  // ...
}
```

---

**交付物:** 命名统一的代码

**验收标准:**
- [ ] 所有 Rust 结构体有 `#[serde(rename_all = "camelCase")]`
- [ ] TypeScript 类型使用 camelCase
- [ ] API 响应字段名一致

---

### Task 8.5.7: 添加错误重试机制

**优先级:** P1

**问题:** LLM 请求网络错误直接失败，没有重试。

**涉及文件:**
- `crates/services/src/services/orchestrator/llm.rs`

---

**Step 8.5.7.1: 实现重试辅助函数**

```rust
use tokio::time::{sleep, Duration};

pub async fn retry_with_backoff<T, E, F, Fut>(
    max_retries: u32,
    mut f: F,
) -> Result<T, E>
where
    F: FnMut() -> Fut,
    Fut: std::future::Future<Output = Result<T, E>>,
{
    let mut last_error = None;

    for attempt in 0..max_retries {
        match f().await {
            Ok(result) => return Ok(result),
            Err(e) if attempt < max_retries - 1 => {
                tracing::warn!(
                    "Attempt {} failed, retrying in {}ms: {}",
                    attempt + 1,
                    1000 * (attempt + 1),
                    e
                );
                last_error = Some(e);
                sleep(Duration::from_millis(1000 * (attempt + 1) as u64)).await;
            }
            Err(e) => return Err(e),
        }
    }

    Err(last_error.unwrap())
}
```

---

**Step 8.5.7.2: 在 LLM 请求中使用重试**

```rust
impl OpenAIClient {
    pub async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        retry_with_backoff(3, || async {
            self.chat_once(messages.clone()).await
        }).await
    }

    async fn chat_once(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        // 原有的请求逻辑
        // ...
    }
}
```

---

**交付物:** 带重试机制的 LLM 客户端

**验收标准:**
- [ ] 网络错误自动重试最多 3 次
- [ ] 指数退避延迟 (1s, 2s, 3s)
- [ ] 重试日志记录

---

## P2 - 代码重构

### Task 8.5.8: 重构魔法数字

**优先级:** P2

**问题:** `MAX_HISTORY` 等魔法数字硬编码。

**涉及文件:**
- `crates/services/src/services/orchestrator/state.rs`
- `crates/services/src/services/orchestrator/config.rs`

---

**Step 8.5.8.1: 创建可配置结构**

```rust
#[derive(Debug, Clone)]
pub struct OrchestratorConfig {
    pub max_conversation_history: usize,
    pub llm_timeout_secs: u64,
    pub max_retries: u32,
    pub retry_delay_ms: u64,
}

impl Default for OrchestratorConfig {
    fn default() -> Self {
        Self {
            max_conversation_history: 50,
            llm_timeout_secs: 120,
            max_retries: 3,
            retry_delay_ms: 1000,
        }
    }
}
```

---

**交付物:** 可配置的参数

**验收标准:**
- [ ] 所有魔法数字提取为常量或配置
- [ ] 提供默认值
- [ ] 支持从环境变量/配置文件读取

---

### Task 8.5.9: 重构硬编码字符串

**优先级:** P2

**问题:** 字符串前缀硬编码。

---

**Step 8.5.9.1: 提取字符串常量**

```rust
pub const WORKFLOW_TOPIC_PREFIX: &str = "workflow:";
pub const TERMINAL_TOPIC_PREFIX: &str = "terminal:";
pub const GIT_EVENT_TOPIC_PREFIX: &str = "git_event:";

pub const GIT_COMMIT_METADATA_SEPARATOR: &str = "---METADATA---";

pub const ENCRYPTION_KEY_ENV: &str = "SOLODAWN_ENCRYPTION_KEY";
```

---

**交付物:** 集中管理的字符串常量

**验收标准:**
- [ ] 所有硬编码字符串提取为常量
- [ ] 常量有清晰的文档注释

---

### Task 8.5.10: 完善状态机转换

**优先级:** P2

**问题:** 状态直接修改，没有验证合法性。

---

**Step 8.5.10.1: 实现显式状态转换**

```rust
impl OrchestratorState {
    pub fn transition_to(
        &mut self,
        new_state: OrchestratorRunState
    ) -> anyhow::Result<()> {
        let valid_transitions = match (self.run_state, new_state) {
            (OrchestratorRunState::Idle, OrchestratorRunState::Processing) => true,
            (OrchestratorRunState::Processing, OrchestratorRunState::Idle) => true,
            (OrchestratorRunState::Processing, OrchestratorRunState::AwaitingGit) => true,
            (OrchestratorRunState::AwaitingGit, OrchestratorRunState::Processing) => true,
            (from, to) => {
                tracing::error!("Invalid state transition: {:?} → {:?}", from, to);
                false
            }
        };

        if valid_transitions {
            tracing::debug!("State transition: {:?} → {:?}", self.run_state, new_state);
            self.run_state = new_state;
            Ok(())
        } else {
            Err(anyhow::anyhow!(
                "Invalid state transition: {:?} → {:?}",
                self.run_state, new_state
            ))
        }
    }
}
```

---

**交付物:** 显式状态机

**验收标准:**
- [ ] 所有状态转换通过 `transition_to` 方法
- [ ] 非法转换返回错误
- [ ] 状态转换有日志记录

---

### Task 8.5.11: LLM 提示词模板化

**优先级:** P2

**问题:** 提示词字符串拼接，难以维护。

---

**Step 8.5.11.1: 使用 Handlebars**

```toml
handlebars = "5.0"
```

```rust
use handlebars::Handlebars;

lazy_static! {
    static ref TEMPLATES: Handlebars<'static> = {
        let mut hb = Handlebars::new();
        hb.register_template_string("terminal_completion", include_str!("templates/terminal_completion.hbs")).unwrap();
        hb
    };
}

pub fn build_completion_prompt(event: &TerminalCompletionEvent) -> String {
    let data = serde_json::to_value(event).unwrap();
    TEMPLATES.render("terminal_completion", &data)
        .unwrap_or_else(|e| {
            tracing::error!("Failed to render template: {}", e);
            format!("Terminal {} completed", event.terminal_id)
        })
}
```

---

**交付物:** 模板化的提示词

**验收标准:**
- [ ] 所有提示词使用模板
- [ ] 模板文件单独存放
- [ ] 支持多语言模板

---

### Task 8.5.12: 数据库批量操作优化

**优先级:** P2

**问题:** 循环中单独执行 SQL，效率低。

---

**Step 8.5.12.1: 使用事务**

```rust
pub async fn create_workflow_with_tasks(
    pool: &SqlitePool,
    workflow: &Workflow,
    tasks: Vec<(WorkflowTask, Vec<Terminal>)>,
) -> anyhow::Result<()> {
    let mut tx = pool.begin().await?;

    // 创建工作流
    sqlx::query("INSERT INTO workflow ...")
        .execute(&mut *tx)
        .await?;

    // 批量创建任务和终端
    for (task, terminals) in tasks {
        sqlx::query("INSERT INTO workflow_task ...")
            .execute(&mut *tx)
            .await?;

        for terminal in terminals {
            sqlx::query("INSERT INTO terminal ...")
                .execute(&mut *tx)
                .await?;
        }
    }

    tx.commit().await?;
    Ok(())
}
```

---

**交付物:** 优化的批量操作

**验收标准:**
- [ ] 批量操作使用事务
- [ ] 性能测试验证改进

---

### Task 8.5.13: WebSocket 终端连接超时控制

**优先级:** P2

**问题:** PTY 连接缺少超时控制。

---

**Step 8.5.13.1: 添加超时配置**

```rust
const PTY_CONNECT_TIMEOUT_SECS: u64 = 30;

pub async fn connect_to_pty(
    terminal_id: &str
) -> anyhow::Result<WebSocketStream> {
    tokio::time::timeout(
        Duration::from_secs(PTY_CONNECT_TIMEOUT_SECS),
        do_connect(terminal_id)
    )
    .await
    .map_err(|_| anyhow::anyhow!("PTY connection timeout after {}s", PTY_CONNECT_TIMEOUT_SECS))?
}
```

---

**交付物:** 带超时的 PTY 连接

**验收标准:**
- [ ] 连接超时 30 秒
- [ ] 超时后返回明确错误
- [ ] 超时值可配置

---

## Phase 8.5 完成检查清单

### P0 检查清单
- [ ] Task 8.5.1: execute_instruction 完整实现
- [ ] Task 8.5.2: API Key 加密存储
- [ ] Task 8.5.3: handle_git_event 实际逻辑

### P1 检查清单
- [ ] Task 8.5.4: 移除未使用导入
- [ ] Task 8.5.5: 移除 dead_code
- [ ] Task 8.5.6: 统一命名规范
- [ ] Task 8.5.7: 错误重试机制

### P2 检查清单
- [ ] Task 8.5.8: 重构魔法数字
- [ ] Task 8.5.9: 重构硬编码字符串
- [ ] Task 8.5.10: 完善状态机
- [ ] Task 8.5.11: LLM 提示词模板化
- [ ] Task 8.5.12: 数据库批量操作
- [ ] Task 8.5.13: WebSocket 超时控制

---

## 附录

### A. 审计问题清单参考

| 问题 | 位置 | 严重程度 |
|------|------|----------|
| execute_instruction TODO | agent.rs:211 | P0 |
| API Key 明文存储 | workflow.rs:106 | P0 |
| handle_git_event 空实现 | agent.rs:156 | P0 |
| 未使用导入 | 多处 | P1 |
| dead_code db 字段 | agent.rs:21 | P1 |
| 命名不一致 | 多处 | P1 |
| 缺少重试机制 | llm.rs:126 | P1 |
| 魔法数字 | state.rs:110 | P2 |
| 硬编码字符串 | 多处 | P2 |
| 状态机未验证 | 多处 | P2 |
| 提示词字符串拼接 | agent.rs:175 | P2 |
| 数据库非批量操作 | workflows.rs:228 | P2 |
| WebSocket 无超时 | terminal_ws.rs | P2 |

### B. 代码规范速查

详见 `TODO.md` 中的"代码规范"章节。

---

*文档版本: 1.0*
*创建日期: 2026-01-19*
*来源: 代码审计报告*
