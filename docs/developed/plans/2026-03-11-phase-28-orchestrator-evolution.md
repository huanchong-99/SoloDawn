# Phase 28: 编排层进化 — 信息流补全 + 闭环补全 + 韧性补全 + 飞书接入

> 日期：2026-03-11
> 状态：待审批
> 前置：Phase 26（审计修复）已完成，Phase 27（Docker）暂缓
> 目标：修复审计发现的 7 个编排层断裂点，接入飞书长连接，实现智能 LLM 熔断与提供商轮转

---

## 一、背景与动机

代码审计确认了编排层的 7 个断裂点：

| # | 断裂点 | 核心问题 |
|---|--------|----------|
| 1 | 终端间零上下文传递 | `build_task_instruction` 不含前序终端信息 |
| 2 | LLM 决策信息不足 | 只看到 commit hash/message，无 terminal log/diff |
| 3 | 不自动合并 | `trigger_merge` 零调用者，`MergeBranch` 不在白名单 |
| 4 | ReviewCode/FixIssues 空壳 | 白名单排除 + 执行分支只打 warn |
| 5 | LLM 失败杀死 Agent | `call_llm` 用 `?` 传播，3 次全失败 agent 死亡 |
| 6 | Planning Draft 单向 | `send_message` 只存不回，无 LLM 对话 |
| 7 | Error Terminal 脱节 | agent 的失败处理不调用 error_handler |

同时需要：
- 接入飞书长连接（WebSocket 模式，非 HTTP webhook）
- 智能 LLM 熔断：5 次失败后自动切换提供商，拉起新终端

---

## 二、总体架构

```
Phase 28A: 信息流补全（断裂点 1, 2）     ← 最高优先级
Phase 28B: 闭环补全（断裂点 3, 4, 7）     ← 高优先级
Phase 28C: 韧性补全（断裂点 5, 6）        ← 高优先级
Phase 28D: 飞书长连接接入                  ← 中优先级
Phase 28E: 智能熔断与提供商轮转            ← 高优先级
```

依赖关系：28A → 28B（信息流是闭环的前提），28C 和 28D 可并行，28E 依赖 28C。

---

## 三、Phase 28A — 信息流补全

### PHASE28A-001: 终端完成上下文采集器

**复杂度**: M | **修改文件**: `crates/services/src/services/orchestrator/agent.rs`

在 `agent.rs` 新增辅助函数：

```rust
/// 采集终端完成时的上下文信息，用于注入 LLM completion prompt
/// 严格限制：terminal_log 摘要 ≤ 2000 chars，diff_stat ≤ 1000 chars
async fn fetch_terminal_completion_context(
    db: &DBService,
    terminal_id: &str,
    commit_hash: &str,
    working_dir: &Path,
) -> anyhow::Result<TerminalCompletionContext>
```

结构体定义（新增到 `types.rs`）：

```rust
pub struct TerminalCompletionContext {
    pub log_summary: String,      // 最后 50 行日志，截断到 2000 chars
    pub diff_stat: String,        // git diff --stat 输出，截断到 1000 chars
    pub commit_body: String,      // commit message 全文（非仅 subject），截断到 500 chars
}
```

实现要点：
1. 查询 `terminal_log` 表最后 50 条记录（`ORDER BY created_at DESC LIMIT 50`），拼接后截断到 2000 chars
2. 执行 `git diff --stat HEAD~1..HEAD` 获取变更统计，截断到 1000 chars
3. 执行 `git show -s --format=%B {commit_hash}` 获取完整 commit body，截断到 500 chars
4. 所有截断处追加 `[...truncated]` 标记

**常量定义**（新增到 `constants.rs`）：

```rust
pub const COMPLETION_CONTEXT_LOG_LINES: usize = 50;
pub const COMPLETION_CONTEXT_LOG_MAX_CHARS: usize = 2000;
pub const COMPLETION_CONTEXT_DIFF_MAX_CHARS: usize = 1000;
pub const COMPLETION_CONTEXT_BODY_MAX_CHARS: usize = 500;
```

**测试要求**：
- 单元测试：日志截断、diff stat 截断、空日志处理
- 集成测试：mock terminal_log 数据 + git repo 验证采集结果

---

### PHASE28A-002: 注入上下文到 LLM Completion Prompt

**复杂度**: S | **依赖**: PHASE28A-001
**修改文件**: `crates/services/src/services/orchestrator/agent.rs` (build_completion_prompt, 行 2018-2040)

修改 `build_completion_prompt` 方法：

```rust
async fn build_completion_prompt(
    &self,
    event: &TerminalCompletionEvent,
) -> anyhow::Result<String> {
    // 现有逻辑：build_terminal_completion_prompt(...)
    let mut prompt = build_terminal_completion_prompt(...);

    // 新增：采集并注入上下文
    if let Ok(ctx) = fetch_terminal_completion_context(
        &self.db, &event.terminal_id, &event.commit_hash, &self.working_dir
    ).await {
        prompt.push_str("\n\n--- Terminal Output Summary ---\n");
        prompt.push_str(&ctx.log_summary);
        prompt.push_str("\n\n--- Changes Summary ---\n");
        prompt.push_str(&ctx.diff_stat);
        if !ctx.commit_body.is_empty() {
            prompt.push_str("\n\n--- Commit Details ---\n");
            prompt.push_str(&ctx.commit_body);
        }
    }
    // ... 现有 agent_planned context 逻辑不变
}
```

**关键约束**：上下文注入失败时静默降级（`if let Ok`），不阻断主流程。

**测试要求**：
- 验证 prompt 包含上下文段落
- 验证采集失败时 prompt 仍正常生成

---

### PHASE28A-003: 跨终端上下文传递（Handoff Notes）

**复杂度**: M | **依赖**: 无
**修改文件**: `crates/services/src/services/orchestrator/agent.rs` (build_task_instruction, 行 2862-2939)

新增辅助函数：

```rust
/// 获取同一 task 中前一个已完成终端的上下文
/// 严格限制：总注入 ≤ 1500 chars，仅取紧邻前序终端
async fn fetch_previous_terminal_context(
    db: &DBService,
    task_id: &str,
    current_terminal_order: i32,
) -> anyhow::Result<Option<PreviousTerminalContext>>
```

```rust
pub struct PreviousTerminalContext {
    pub role: String,              // 前序终端角色
    pub status: String,            // completed/failed
    pub commit_message: String,    // 最后一次 commit message，截断 500 chars
    pub handoff_notes: String,     // 从 commit body 提取的 handoff 段落，截断 800 chars
}
```

修改 `build_task_instruction`（当前是静态方法，需改为接收 `Option<PreviousTerminalContext>` 参数）：

```rust
fn build_task_instruction(
    workflow_id: &str,
    task: &db::models::WorkflowTask,
    terminal: &db::models::Terminal,
    total_terminals: usize,
    prev_context: Option<&PreviousTerminalContext>,  // 新增参数
) -> String
```

在指令末尾追加（仅当 `prev_context.is_some()`）：

```
--- Previous Terminal Context ---
Role: {role} | Status: {status}
Last Commit: {commit_message}
Handoff Notes: {handoff_notes}
```

**Handoff Notes 提取逻辑**：
- 从 commit body 中查找 `HANDOFF:` 或 `Handoff Notes:` 标记后的内容
- 如无标记，取 commit body 去掉 METADATA 块后的剩余文本
- 截断到 800 chars

**常量**（`constants.rs`）：

```rust
pub const HANDOFF_CONTEXT_MAX_CHARS: usize = 1500;
pub const HANDOFF_COMMIT_MAX_CHARS: usize = 500;
pub const HANDOFF_NOTES_MAX_CHARS: usize = 800;
```

**调用点修改**：
- `dispatch_next_terminal`（agent.rs:1267）调用 `build_task_instruction` 前，先调用 `fetch_previous_terminal_context`
- `auto_dispatch_initial_terminals`（agent.rs:2700+）同理，首个终端传 `None`

**测试要求**：
- 单元测试：handoff notes 提取（有标记/无标记/空 body）
- 集成测试：两个终端串行，验证第二个终端指令包含第一个的上下文

---

## 四、Phase 28B — 闭环补全

### PHASE28B-001: Workflow 完成后自动合并

**复杂度**: M | **依赖**: 无
**修改文件**: `crates/services/src/services/orchestrator/agent.rs` (auto_sync_workflow_completion, 行 3154-3210)

1. 在 `OrchestratorConfig` (`config.rs`) 新增字段：

```rust
pub auto_merge_on_completion: bool,  // default: true
```

2. 修改 `auto_sync_workflow_completion`，在标记 completed 后触发合并：

```rust
async fn auto_sync_workflow_completion(&self) -> anyhow::Result<()> {
    // ... 现有检查逻辑不变 ...

    // 标记 completed（现有逻辑，行 3190）
    Workflow::set_status(&self.db.pool, &self.workflow_id, WORKFLOW_STATUS_COMPLETED).await?;

    // 新增：自动合并
    if self.config.auto_merge_on_completion {
        match self.execute_auto_merge().await {
            Ok(_) => {
                tracing::info!(workflow_id = %self.workflow_id, "Auto-merge completed");
            }
            Err(e) => {
                // 合并失败不杀 workflow，设为 merging 状态等待人工处理
                tracing::warn!(workflow_id = %self.workflow_id, error = %e, "Auto-merge failed, setting status to merging");
                Workflow::set_status(&self.db.pool, &self.workflow_id, "merging").await?;
                self.publish_system_event("auto_merge_failed", &e.to_string()).await;
            }
        }
    }
    Ok(())
}
```

3. 新增 `execute_auto_merge` 方法，复用 `trigger_merge`（agent.rs:3225）的逻辑：

```rust
async fn execute_auto_merge(&self) -> anyhow::Result<()> {
    // 获取 workflow 的所有 task 分支
    let tasks = WorkflowTask::find_by_workflow(&self.db.pool, &self.workflow_id).await?;
    for task in &tasks {
        if task.status == TASK_STATUS_COMPLETED {
            // 调用现有 trigger_merge 逻辑（squash merge task branch → target branch）
            self.trigger_merge(&task.id, &task.branch_name).await?;
        }
    }
    Ok(())
}
```

**数据库迁移**：无需（auto_merge_on_completion 从 config 读取，不持久化）

**测试要求**：
- 单元测试：auto_merge 开启/关闭时的行为
- 单元测试：合并失败时 workflow 状态变为 merging 而非 failed
- 集成测试：workflow 完成 → 自动合并 → 分支合入主分支

---

### PHASE28B-002: 启用 ReviewCode / FixIssues / MergeBranch 指令

**复杂度**: L | **依赖**: PHASE28B-001
**修改文件**: `crates/services/src/services/orchestrator/agent.rs` (行 2089-2103, 2484-2491)

**Step 1: 加入白名单**（agent.rs:2089-2103）

在 `is_instruction_whitelisted` 中添加：

```rust
OrchestratorInstruction::ReviewCode { .. } => true,
OrchestratorInstruction::FixIssues { .. } => true,
OrchestratorInstruction::MergeBranch { .. } => true,
```

**Step 2: 实现 ReviewCode 执行逻辑**（agent.rs:2484）

```rust
OrchestratorInstruction::ReviewCode { terminal_id, commit_hash } => {
    // 1. 获取被审查终端的 task 信息
    let terminal = Terminal::find_by_id(&self.db.pool, &terminal_id).await?
        .ok_or_else(|| anyhow!("Terminal not found: {}", terminal_id))?;
    let task = WorkflowTask::find_by_id(&self.db.pool, &terminal.task_id).await?
        .ok_or_else(|| anyhow!("Task not found: {}", terminal.task_id))?;

    // 2. 创建 reviewer 终端
    let review_terminal = Terminal::create(&self.db.pool, CreateTerminalParams {
        task_id: task.id.clone(),
        name: format!("review-{}", &terminal_id[..8]),
        role: "reviewer".to_string(),
        role_description: format!(
            "Review changes from terminal {} (commit {}). Check code quality, correctness, and test coverage.",
            terminal_id, commit_hash
        ),
        // ... 其他字段从 task 默认配置继承
    }).await?;

    // 3. 构建包含 diff 上下文的审查指令
    let diff_stat = self.fetch_diff_for_review(&commit_hash).await.unwrap_or_default();
    let instruction = format!(
        "Review the following changes:\n{}\n\nCommit: {}\n\nIf approved, commit with status: review_pass. If issues found, commit with status: review_reject and list issues.",
        diff_stat, commit_hash
    );

    // 4. 启动并派发
    self.dispatch_terminal(&review_terminal.id, &instruction).await?;
}
```

**Step 3: 实现 FixIssues 执行逻辑**

```rust
OrchestratorInstruction::FixIssues { terminal_id, issues } => {
    let terminal = Terminal::find_by_id(&self.db.pool, &terminal_id).await?...;
    let task = WorkflowTask::find_by_id(&self.db.pool, &terminal.task_id).await?...;

    let fix_terminal = Terminal::create(&self.db.pool, CreateTerminalParams {
        task_id: task.id.clone(),
        name: format!("fix-{}", &terminal_id[..8]),
        role: "fixer".to_string(),
        role_description: format!("Fix the following issues:\n{}", issues.join("\n")),
        // ...
    }).await?;

    let instruction = format!(
        "Fix these issues found during code review:\n{}\n\nAfter fixing, commit with appropriate metadata.",
        issues.iter().enumerate().map(|(i, s)| format!("{}. {}", i+1, s)).collect::<Vec<_>>().join("\n")
    );

    self.dispatch_terminal(&fix_terminal.id, &instruction).await?;
}
```

**Step 4: 实现 MergeBranch 执行逻辑**

```rust
OrchestratorInstruction::MergeBranch { task_id, .. } => {
    let task = WorkflowTask::find_by_id(&self.db.pool, &task_id).await?...;
    self.trigger_merge(&task.id, &task.branch_name).await?;
}
```

**Step 5: Review Pass/Reject 自动推进**

修改 `handle_git_review_pass`（agent.rs:1890-1926），在状态更新后：

```rust
// 现有逻辑：更新 terminal 状态为 review_passed
// 新增：检查是否所有终端都完成，如果是则自动推进合并
let task_state = self.get_or_init_task_state(&task_id).await?;
if task_state.is_all_terminals_done() {
    self.auto_sync_workflow_completion().await?;
}
```

修改 `handle_git_review_reject`（agent.rs:1928-1967），在状态更新后：

```rust
// 现有逻辑：更新 terminal 状态为 review_rejected
// 新增：自动创建修复终端
if let Some(issues) = metadata.issues {
    let fix_instruction = OrchestratorInstruction::FixIssues {
        terminal_id: reviewed_terminal_id.clone(),
        issues: issues.iter().map(|i| i.message.clone()).collect(),
    };
    self.execute_single_instruction(fix_instruction).await?;
}
```

**测试要求**：
- 单元测试：ReviewCode 创建 reviewer 终端并派发
- 单元测试：FixIssues 创建 fixer 终端并注入 issues
- 单元测试：MergeBranch 调用 trigger_merge
- 集成测试：review_pass → 自动推进完成
- 集成测试：review_reject → 自动创建修复终端

---

### PHASE28B-003: 连接 Error Handler 到 Agent

**复杂度**: S | **依赖**: 无
**修改文件**: `crates/services/src/services/orchestrator/agent.rs` (handle_git_terminal_failed, 行 1970-2002)

修改 `handle_git_terminal_failed`：

```rust
async fn handle_git_terminal_failed(
    &self,
    terminal_id: &str,
    task_id: &str,
    commit_hash: &str,
    commit_message: &str,
) -> anyhow::Result<()> {
    tracing::warn!(terminal_id, task_id, "Terminal reported failure via git commit");

    // 替换现有的重复逻辑，直接委托给 error_handler
    self.error_handler.handle_terminal_failure(
        &self.db,
        &self.workflow_id,
        terminal_id,
        task_id,
        &self.message_bus,
    ).await?;

    self.awaken();
    Ok(())
}
```

删除 agent.rs:3362 处的死代码 wrapper 方法。

**测试要求**：
- 单元测试：terminal 失败时 error_handler 被调用
- 单元测试：error_terminal_enabled 时自动激活 error terminal

---

## 五、Phase 28C — 韧性补全

### PHASE28C-001: Agent 事件循环容错

**复杂度**: M | **依赖**: 无
**修改文件**: `crates/services/src/services/orchestrator/agent.rs`

**Step 1: call_llm 错误不再杀死 agent**

新增 `call_llm_safe` 方法，包装现有 `call_llm`，返回 `Option<String>`：
- 成功时重置 `state.error_count = 0`，返回 `Some(response)`
- 失败时递增 `state.error_count`，发布 `llm_call_failed` 系统事件，返回 `None`
- 不使用 `?` 操作符，错误不传播

**Step 2: handle_terminal_completed 使用 call_llm_safe**

修改 agent.rs:1020-1030，将 `self.call_llm(&prompt).await?` 替换为 `self.call_llm_safe(&prompt).await`：
- `Some(response)` -> 解析并执行指令（现有逻辑）
- `None` -> 跳过 LLM 决策，仅做 auto-dispatch 降级

**Step 3: 连续失败阈值**

新增常量 `MAX_CONSECUTIVE_LLM_FAILURES: u32 = 10`（constants.rs）。
当 `error_count >= 10` 时发布 `llm_provider_exhausted` 事件，触发 Phase 28E 的提供商切换。

**Step 4: 无 LLM 时的降级能力**

agent 在 LLM 不可用时仍能：
- 处理终端完成事件（更新状态）
- 自动派发下一个终端（auto-dispatch）
- 解析 git commit metadata
- 标记 task/workflow 完成

唯一需要 LLM 的是复杂路由决策，这些在 LLM 恢复后重新处理。

**测试要求**：
- 单元测试：LLM 失败时 agent 事件循环继续运行
- 单元测试：error_count 正确递增和重置
- 单元测试：达到阈值时发布 provider_exhausted 事件
- 集成测试：mock LLM 返回错误，验证 auto-dispatch 仍正常工作

---

### PHASE28C-002: 状态持久化激活

**复杂度**: M | **依赖**: PHASE28C-001
**修改文件**: `crates/services/src/services/orchestrator/agent.rs`

**Step 1: 防抖持久化**

新增到 `OrchestratorAgent` 结构体：
- `last_state_save: Arc<Mutex<Instant>>` — 上次保存时间
- `persistence: Arc<StatePersistence>` — 持久化服务引用

新增常量 `STATE_SAVE_DEBOUNCE_SECS: u64 = 5`（constants.rs）。

新增 `maybe_save_state` 方法：
- 检查距上次保存是否超过 5 秒（防抖）
- 读取当前 state，调用 `persistence.save_state()`
- 保存失败只 warn 日志，不阻断主流程

**Step 2: 在关键检查点调用**

插入 `self.maybe_save_state().await` 的位置：
- `call_llm` 成功返回后（agent.rs:2131 附近）
- `handle_terminal_completed` 状态更新后（agent.rs:974 附近）
- `handle_git_event` 处理完成后（agent.rs:1490 附近）
- `auto_sync_workflow_completion` 标记完成后（agent.rs:3190 附近）

**Step 3: Agent 创建时传入 persistence**

在 `start_workflow_reserved`（runtime.rs:365-380）创建 agent 时传入 `self.persistence.clone()`。

**测试要求**：
- 单元测试：防抖逻辑（5 秒内多次调用只保存一次）
- 单元测试：保存失败不阻断主流程
- 集成测试：agent 运行后 DB 中 orchestrator_state 非空

---

### PHASE28C-003: 崩溃恢复实现

**复杂度**: L | **依赖**: PHASE28C-002
**修改文件**: `crates/services/src/services/orchestrator/runtime.rs` (行 730-835)

修改 `recover_running_workflows`，区分三种情况：

1. **有持久化状态**：调用新增的 `resume_workflow` 恢复 agent
2. **无持久化状态**：标记 failed（无法恢复）
3. **恢复出错**：标记 failed 并记录错误

新增 `resume_workflow` 方法：
- 从 workflow DB 记录构建 `OrchestratorConfig`
- 创建 `OrchestratorAgent` 并调用 `restore_state(persisted)` 注入恢复的对话历史、task states、token count
- 复用 `start_workflow_reserved` 的 spawn 逻辑启动 agent

在 `OrchestratorAgent` 新增 `restore_state` 方法：
- 将 `PersistedState` 写入 `SharedOrchestratorState`
- 恢复 conversation_history、task_states、total_tokens_used、error_count、workflow_planning_complete

**测试要求**：
- 单元测试：有持久化状态时恢复 agent 并继续运行
- 单元测试：无持久化状态时标记 failed
- 集成测试：模拟 agent 崩溃 -> 重启 -> 恢复 -> 继续处理事件

---

### PHASE28C-004: Planning Draft 接入 LLM 对话

**复杂度**: M | **依赖**: 无
**修改文件**: `crates/server/src/routes/planning_drafts.rs` (send_message, 行 237-266)

修改 `send_message`，在存储用户消息后：

1. 查询该 draft 的所有历史消息
2. 构建 LLM 对话（首条为 `WorkspacePlanning` system prompt，激活 config.rs 中的死代码）
3. 调用 LLM 生成 assistant 回复
4. 存储 assistant 回复到 DB
5. 返回 user + assistant 两条消息

新增辅助函数：
- `build_planning_conversation(draft, messages) -> Vec<LLMMessage>`：将 draft messages 转为 LLM 消息格式
- `get_planning_llm_client(deployment) -> Box<dyn LLMClient>`：从 deployment 配置获取 LLM client

**测试要求**：
- 单元测试：send_message 返回 user + assistant 两条消息
- 单元测试：LLM 失败时返回 500 错误
- 集成测试：多轮对话，验证历史正确传递

---

## 六、Phase 28D — 飞书长连接接入

### PHASE28D-001: 飞书连接器 Crate

**复杂度**: XL | **依赖**: 无

新建 `crates/feishu-connector/`，结构如下：

```
crates/feishu-connector/
  Cargo.toml
  src/
    lib.rs          # 公共 API 导出
    client.rs       # FeishuClient — WebSocket 生命周期管理
    auth.rs         # 鉴权：获取 WSS endpoint URL + tenant_access_token 管理
    protocol.rs     # protobuf Frame 编解码（pbbp2 schema）
    events.rs       # 事件类型定义（im.message.receive_v1 等）
    messages.rs     # 发送消息 REST API（text, interactive, reply）
    reconnect.rs    # 重连逻辑（ClientConfig 驱动，含 jitter）
    types.rs        # 共享类型定义
```

**Cargo.toml 关键依赖**：
- tokio (full), tokio-tungstenite (native-tls), prost, reqwest (json)
- serde, serde_json, tracing, anyhow, bytes, flate2 (gzip 解压)

**auth.rs 核心**：
- `FeishuAuth` 结构体持有 app_id, app_secret, base_url
- `acquire_ws_endpoint()`: POST /callback/ws/endpoint 获取 WSS URL
- `get_tenant_token()`: POST /open-apis/auth/v3/tenant_access_token/internal，自动缓存，过期前 5 分钟刷新

**client.rs 核心**：
- `FeishuClient` 管理 WebSocket 连接生命周期
- `connect()`: 获取 WSS endpoint -> 建立连接 -> 启动 ping 循环(120s) -> 启动消息接收循环
- `subscribe() -> mpsc::Receiver<FeishuEvent>`: 事件输出通道
- 断线时按 ClientConfig 参数重连（ReconnectCount, ReconnectInterval, ReconnectNonce）

**protocol.rs 核心**：
- pbbp2 Frame 编解码（protobuf binary frames）
- `encode_frame / decode_frame`: 往返编解码
- `build_ping_frame / build_event_response`: 构建控制帧和事件响应帧
- 支持 gzip payload 解压

**events.rs 核心**：
- `FeishuEvent` 结构体（schema, header, event payload）
- `parse_message_event()`: 解析 im.message.receive_v1 为 ReceivedMessage
- ReceivedMessage 包含 message_id, chat_id, chat_type, sender_open_id, message_type, content

**messages.rs 核心**：
- `FeishuMessenger` 通过 REST API 发送消息（WebSocket 仅接收）
- `send_text(chat_id, text)`: POST /open-apis/im/v1/messages
- `reply_text(message_id, text)`: POST /open-apis/im/v1/messages/{id}/reply
- `send_card(chat_id, card)`: 发送 interactive card（结构化回执）
- 所有请求携带 Bearer {tenant_access_token}

**reconnect.rs 核心**：
- `ReconnectPolicy` 根据 ClientConfig 计算重连延迟（含随机 jitter）
- pong 帧可能携带新的 ClientConfig，动态更新策略
- 超过 ReconnectCount 时停止重连（-1 表示无限重试）

**测试要求**：
- 单元测试：Frame 编解码往返一致性
- 单元测试：tenant_token 缓存和过期刷新
- 单元测试：重连策略（jitter 范围、最大次数）
- 单元测试：消息事件解析
- 集成测试：mock WebSocket server 验证连接生命周期

---

### PHASE28D-002: 飞书服务集成

**复杂度**: L | **依赖**: PHASE28D-001
**新建文件**: `crates/services/src/services/feishu.rs`
**修改文件**: `crates/services/src/services/mod.rs`

`FeishuService` 结构体：
- 持有 `FeishuClient`（WebSocket）、`FeishuMessenger`（REST）、`DBService`、`SharedMessageBus`
- 内置 CircuitBreaker 保护飞书 API 调用

核心方法：
- `start()`: 连接 WebSocket + 启动事件处理循环
- `handle_event(event)`: 按 event_type 路由（目前只处理 im.message.receive_v1）
- `handle_message(event)`: 解析消息 -> 处理斜杠命令(/bind, /unbind) -> 查找会话绑定 -> 转发到 submit_orchestrator_chat -> 回复结果
- `handle_bind(msg, text)`: 解析 workflow_id，创建 ExternalConversationBinding（provider="feishu"）
- `handle_unbind(msg)`: 停用当前会话绑定

消息回复模板：
- 绑定成功/失败
- 未绑定提示（"请先使用 /bind <workflow_id> 绑定工作流"）
- 编排命令执行结果（成功摘要 / 失败原因）

在 `mod.rs` 注册：`pub mod feishu;`

**测试要求**：
- 单元测试：消息路由（/bind, /unbind, 普通消息）
- 单元测试：未绑定时的提示回复
- 集成测试：mock FeishuClient + mock orchestrator chat 验证全链路

---

### PHASE28D-003: ChatConnector Trait 抽象

**复杂度**: M | **依赖**: PHASE28D-002
**新建文件**: `crates/services/src/services/chat_connector.rs`
**修改文件**: `crates/server/src/routes/chat_integrations.rs`

定义统一 trait：

```rust
#[async_trait]
pub trait ChatConnector: Send + Sync {
    async fn send_message(&self, conversation_id: &str, content: &str) -> anyhow::Result<String>;
    async fn send_reply(&self, conversation_id: &str, message_id: &str, content: &str) -> anyhow::Result<String>;
    fn provider_name(&self) -> &str;
    fn is_connected(&self) -> bool;
}
```

实现：
- `TelegramConnector`: 包装现有 webhook 出站逻辑
- `FeishuConnector`: 包装 `FeishuMessenger`

在 `chat_integrations.rs` 中，出站消息通过 trait 发送，而非硬编码提供商逻辑。

**测试要求**：
- 单元测试：trait 实现的 provider_name 返回正确值
- 集成测试：通过 trait 发送消息验证路由正确

---

### PHASE28D-004: 数据库与配置

**复杂度**: S | **依赖**: 无

**新建迁移** `crates/db/migrations/20260311120000_add_feishu_connector.sql`：

```sql
CREATE TABLE IF NOT EXISTS feishu_app_config (
    id TEXT PRIMARY KEY NOT NULL,
    app_id TEXT NOT NULL,
    app_secret_encrypted TEXT NOT NULL,
    tenant_key TEXT,
    base_url TEXT NOT NULL DEFAULT 'https://open.feishu.cn',
    enabled INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

现有 `external_conversation_binding` 表已支持任意 provider 字符串，无需 schema 变更。

**环境变量**：
- `GITCORTEX_FEISHU_APP_ID` — 飞书 App ID
- `GITCORTEX_FEISHU_APP_SECRET` — 飞书 App Secret（启动时加密存储）
- `GITCORTEX_FEISHU_ENABLED` — 功能开关，默认 false

**DB Model** 新建 `crates/db/src/models/feishu_config.rs`：
- `FeishuAppConfig` 结构体 + CRUD 方法
- app_secret 使用现有 AES-256-GCM 加密方案（同 workflow API key）

**测试要求**：
- 迁移 up/down 测试
- Model CRUD 测试

---

### PHASE28D-005: Server 集成

**复杂度**: M | **依赖**: PHASE28D-002, PHASE28D-004
**修改文件**: `crates/server/src/main.rs`
**新建文件**: `crates/server/src/routes/feishu.rs`

1. Server 启动时，如果 `GITCORTEX_FEISHU_ENABLED=true`：
   - 从 DB 或环境变量加载飞书配置
   - 创建 `FeishuService` 并启动 WebSocket 连接
   - 注册到 app state

2. 新增管理 API（`routes/feishu.rs`）：
   - `GET /api/integrations/feishu/status` — 连接状态
   - `PUT /api/integrations/feishu/config` — 更新配置
   - `POST /api/integrations/feishu/reconnect` — 手动重连

3. 健康检查扩展：在现有 health endpoint 中包含飞书连接状态

**测试要求**：
- 单元测试：FEISHU_ENABLED=false 时不启动连接
- 集成测试：配置 API CRUD

---

## 七、Phase 28E — 智能熔断与提供商轮转

### PHASE28E-001: ResilientLLMClient 实现

**复杂度**: L | **依赖**: PHASE28C-001
**新建文件**: crates/services/src/services/orchestrator/resilient_llm.rs
**修改文件**: crates/services/src/services/orchestrator/llm.rs, config.rs, mod.rs

**核心设计**：在现有 LLMClient trait 之上构建弹性层，支持多提供商自动切换。

**ProviderConfig 定义**（config.rs 新增）：
- name: String — 提供商标识，如 "openai-primary", "anthropic-fallback"
- api_type, base_url, api_key, model: String — 连接参数
- priority: u32 — 优先级，0 最高

**OrchestratorConfig 扩展**（config.rs）：
- 新增 fallback_providers: Vec<ProviderConfig> — 备选提供商列表
- 新增 provider_failure_threshold: u32 — 熔断阈值，默认 5
- 新增 provider_probe_interval_secs: u64 — 探活间隔，默认 60

**ResilientLLMClient 结构体**：
- providers: Vec<ProviderEntry> — 每个 entry 包含 config + client + state
- active_index: AtomicUsize — 当前活跃提供商索引
- ProviderState 跟踪：consecutive_failures, is_dead, last_failure, last_probe, total_requests, total_failures

**LLMClient trait 实现逻辑**：
1. 获取当前活跃提供商
2. 跳过已熔断的提供商（除非到了探活时间，即 last_failure 超过 60 秒）
3. 尝试调用：成功则重置失败计数并返回；失败则递增计数
4. 连续 5 次失败：标记 is_dead = true，自动切换到下一个提供商（round-robin）
5. 所有提供商都试过且都 dead：返回 "All LLM providers exhausted" 错误
6. 探活成功后自动恢复 is_dead = false

**辅助方法**：
- switch_to_next(current): round-robin 切换到下一个提供商
- should_probe(state): 判断是否到了探活时间（60 秒间隔）
- active_provider_name(): 返回当前活跃提供商名称
- provider_status(): 返回所有提供商健康状态报告

**工厂函数修改**（llm.rs create_llm_client）：
- 无 fallback_providers 时：使用现有单提供商逻辑（向后兼容）
- 有 fallback_providers 时：构建 ResilientLLMClient，primary + fallbacks

**测试要求**：
- 单元测试：primary 成功时不切换
- 单元测试：primary 连续 5 次失败后自动切换到 fallback
- 单元测试：所有提供商都失败时返回 "All providers exhausted"
- 单元测试：dead 提供商在 60 秒后被探活
- 单元测试：探活成功后恢复使用
- 集成测试：mock 两个提供商，primary 挂掉后 fallback 接管

---

### PHASE28E-002: 终端级提供商故障转移

**复杂度**: L | **依赖**: PHASE28E-001, PHASE28C-001
**修改文件**: crates/services/src/services/orchestrator/agent.rs

当编排层 Agent 检测到终端的上游提供商不可用时，自动拉起替代终端。

**触发条件**：
- 终端连续失败且 commit message 包含提供商故障关键词
- 或编排层收到 llm_provider_exhausted 事件

**新增方法 handle_terminal_provider_failure**：
1. 关闭失败终端（set status = failed）
2. 调用 find_alternative_cli_config 选择可用的替代 CLI/模型配置（排除失败终端使用的）
3. 创建替代终端（继承 role, role_description，使用新 CLI/模型）
4. 构建指令，包含失败终端的上下文（通过 PHASE28A-003 的 PreviousTerminalContext）
5. 启动并派发替代终端

**find_alternative_cli_config 逻辑**：
- 查询所有可用的 ModelConfig
- 排除失败终端使用的 cli_type_id
- 按优先级选择第一个可用的
- 无替代配置时返回错误

**集成到失败处理流程**：
在 handle_git_terminal_failed 中增加判断：
- is_provider_failure(commit_message) 为 true -> handle_terminal_provider_failure
- 否则 -> 现有 error_handler 逻辑

**提供商故障判断**（启发式关键词匹配）：
api_error, rate_limit, timeout, connection_refused, service_unavailable, provider_error, authentication_failed

**测试要求**：
- 单元测试：提供商故障判断（关键词匹配）
- 单元测试：替代 CLI 配置选择（排除失败的）
- 单元测试：无替代配置时返回错误
- 集成测试：终端失败 -> 自动创建替代终端 -> 派发任务

---

### PHASE28E-003: 提供商健康监控 API

**复杂度**: S | **依赖**: PHASE28E-001
**新建文件**: crates/server/src/routes/provider_health.rs
**修改文件**: crates/server/src/routes/mod.rs

新增 API：
- GET /api/workflows/{id}/providers/status — 返回该 workflow 的所有 LLM 提供商状态
  - 每个提供商：name, model, is_active, is_dead, consecutive_failures, total_requests, total_failures, last_failure
  - active_provider: 当前活跃提供商名称
- POST /api/workflows/{id}/providers/{name}/reset — 手动重置提供商熔断状态

**WebSocket 事件扩展**：
在 workflow WebSocket 中新增事件类型：
- provider.switched — 提供商切换时推送
- provider.exhausted — 所有提供商耗尽时推送
- provider.recovered — 熔断提供商恢复时推送

**测试要求**：
- 单元测试：状态 API 返回正确格式
- 单元测试：reset API 清除熔断状态

---

## 八、数据库迁移汇总

| 迁移文件 | 内容 | Phase |
|----------|------|-------|
| 20260311120000_add_feishu_connector.sql | feishu_app_config 表 | 28D |

其余修改均为代码层变更，不涉及新增数据库表。现有 external_conversation_binding 表已支持 provider="feishu"，无需 schema 变更。orchestrator_state 列已存在（20260125000000 迁移），无需新增。

---

## 九、实施顺序与依赖图

Phase 28A（信息流补全）
  28A-001 终端完成上下文采集器
  28A-002 注入上下文到 Completion Prompt（依赖 28A-001）
  28A-003 跨终端上下文传递

Phase 28B（闭环补全，依赖 28A 完成）
  28B-001 自动合并
  28B-002 启用 ReviewCode/FixIssues/MergeBranch（依赖 28B-001）
  28B-003 连接 Error Handler

Phase 28C（韧性补全，可与 28B 并行）
  28C-001 Agent 事件循环容错
  28C-002 状态持久化激活（依赖 28C-001）
  28C-003 崩溃恢复实现（依赖 28C-002）
  28C-004 Planning Draft 接入 LLM

Phase 28D（飞书接入，可与 28B/28C 并行）
  28D-004 数据库与配置（无依赖，最先做）
  28D-001 飞书连接器 Crate（无依赖）
  28D-002 飞书服务集成（依赖 28D-001）
  28D-003 ChatConnector Trait（依赖 28D-002）
  28D-005 Server 集成（依赖 28D-002, 28D-004）

Phase 28E（智能熔断，依赖 28C-001）
  28E-001 ResilientLLMClient（依赖 28C-001）
  28E-002 终端级故障转移（依赖 28E-001）
  28E-003 提供商健康监控 API（依赖 28E-001）

**推荐并行策略**（利用多终端）：
- 终端 1-3: Phase 28A（串行，3 个任务）
- 终端 4-5: Phase 28C-001 + 28C-004（可独立于 28A）
- 终端 6: Phase 28D-004 + 28D-001（数据库 + crate 骨架）
- 28A 完成后 -> 终端 1-3 转入 28B
- 28C-001 完成后 -> 终端 4 转入 28C-002 -> 28C-003，终端 5 转入 28E-001
- 28D-001 完成后 -> 终端 6 转入 28D-002 -> 28D-003 -> 28D-005

---

## 十、任务清单汇总

| ID | 任务 | 复杂度 | 依赖 | Phase |
|----|------|--------|------|-------|
| 28A-001 | 终端完成上下文采集器 | M | 无 | 28A |
| 28A-002 | 注入上下文到 Completion Prompt | S | 28A-001 | 28A |
| 28A-003 | 跨终端上下文传递（Handoff Notes） | M | 无 | 28A |
| 28B-001 | Workflow 完成后自动合并 | M | 无 | 28B |
| 28B-002 | 启用 ReviewCode/FixIssues/MergeBranch | L | 28B-001 | 28B |
| 28B-003 | 连接 Error Handler 到 Agent | S | 无 | 28B |
| 28C-001 | Agent 事件循环容错 | M | 无 | 28C |
| 28C-002 | 状态持久化激活 | M | 28C-001 | 28C |
| 28C-003 | 崩溃恢复实现 | L | 28C-002 | 28C |
| 28C-004 | Planning Draft 接入 LLM 对话 | M | 无 | 28C |
| 28D-001 | 飞书连接器 Crate | XL | 无 | 28D |
| 28D-002 | 飞书服务集成 | L | 28D-001 | 28D |
| 28D-003 | ChatConnector Trait 抽象 | M | 28D-002 | 28D |
| 28D-004 | 数据库与配置 | S | 无 | 28D |
| 28D-005 | Server 集成 | M | 28D-002,004 | 28D |
| 28E-001 | ResilientLLMClient 实现 | L | 28C-001 | 28E |
| 28E-002 | 终端级提供商故障转移 | L | 28E-001 | 28E |
| 28E-003 | 提供商健康监控 API | S | 28E-001 | 28E |

**总计**: 18 个任务（S:4, M:7, L:5, XL:1）

---

## 十一、上下文注入大小限制汇总

所有注入点均有严格的字符数上限，防止超长上下文导致 LLM token 爆炸：

| 注入点 | 内容 | 上限 | 常量名 |
|--------|------|------|--------|
| Completion Prompt - 日志摘要 | 最后 50 行 terminal_log | 2000 chars | COMPLETION_CONTEXT_LOG_MAX_CHARS |
| Completion Prompt - Diff 统计 | git diff --stat | 1000 chars | COMPLETION_CONTEXT_DIFF_MAX_CHARS |
| Completion Prompt - Commit 全文 | git show -s --format=%B | 500 chars | COMPLETION_CONTEXT_BODY_MAX_CHARS |
| Task Instruction - 前序上下文 | 前一终端 commit + handoff | 1500 chars | HANDOFF_CONTEXT_MAX_CHARS |
| Task Instruction - Commit 消息 | 前序终端 commit message | 500 chars | HANDOFF_COMMIT_MAX_CHARS |
| Task Instruction - Handoff Notes | 从 commit body 提取 | 800 chars | HANDOFF_NOTES_MAX_CHARS |
| Agent Planned Context | 任务进度摘要 | 3000 chars | AGENT_PLANNED_CONTEXT_MAX_CHARS |

所有截断处追加 "[...truncated]" 标记。采集失败时静默降级，不阻断主流程。

---

## 十二、验收标准

1. **信息流**：LLM completion prompt 包含终端日志摘要和 diff stat；下一个终端指令包含前序终端的 handoff notes
2. **闭环**：workflow 完成后自动合并分支；review_reject 自动创建修复终端
3. **韧性**：LLM 连续失败不杀死 agent；agent 崩溃后重启可恢复；Planning Draft 支持双向对话
4. **飞书**：通过飞书长连接收发消息；/bind /unbind 命令正常工作；消息转发到编排器并回复结果
5. **熔断**：LLM 提供商连续 5 次失败后自动切换；终端提供商故障时自动拉起替代终端；所有提供商状态可监控
6. **无回归**：现有 Telegram 连接器、Session Chat、Workflow API 无回归
7. **测试**：所有新增功能有单元测试和集成测试覆盖
