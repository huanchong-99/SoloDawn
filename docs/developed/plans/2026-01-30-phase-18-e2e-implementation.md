# Phase 18 端到端测试实施计划

> **创建时间:** 2026-01-30
> **工作区:** E:/SoloDawn-phase-18-e2e
> **分支:** feature/phase-18-e2e-tests

## 概述

基于 Codex MCP 分析，实施 Phase 18 的端到端测试，覆盖完整的工作流生命周期。

## Task 18.1 - 端到端全流程测试

### 关键路径覆盖

1. **Workflow 创建验证**
   - 任务为空/终端为空时返回错误
   - CLI 或 model 不存在时返回错误
   - CLI-model 不匹配时返回错误
   - 分支名去重（`workflow/{id}/{slug}-2`）

2. **状态转换验证**
   - 非 ready 状态时 start 返回错误
   - merge 在 created/ready 状态下失败
   - 状态流：created → ready → starting → running → completed

3. **Happy Path E2E**
   - create → ready → start → Git commit detection → LLM 完成 → merge

### 测试用例

| 测试名称 | 描述 |
|---------|------|
| `test_workflow_create_validation` | 验证创建时的各种校验 |
| `test_workflow_status_transitions` | 验证状态转换规则 |
| `test_workflow_full_lifecycle_e2e` | 完整生命周期测试 |
| `test_workflow_branch_naming` | 分支命名去重测试 |

## Task 18.2 - 并发/失败/恢复场景测试

### 测试用例

| 测试名称 | 描述 |
|---------|------|
| `test_concurrent_workflow_limit` | 并发 workflow 超限拒绝 |
| `test_concurrent_start_same_workflow` | 同一 workflow 并发 start |
| `test_terminal_failed_commit` | 终端失败后状态更新 |
| `test_llm_network_failure` | LLM API 500/超时重试 |
| `test_workflow_recovery` | 恢复中断的 workflow |

## 测试基础设施

### Mock 策略

1. **LLM API**: 使用 `wiremock` mock `/chat/completions`
2. **CLI 工具**: 避免真实 CLI，通过 GitWatcher 触发 commit event
3. **测试数据库**: SQLite in-memory + `sqlx::migrate!`
4. **Git 提交事件**: 临时 repo + `git commit -m` 写入 `---METADATA---`

### 环境变量

```bash
SOLODAWN_ENCRYPTION_KEY=12345678901234567890123456789012
```

## 实施步骤

### Step 1: 添加测试依赖
- [x] 添加 `wiremock` 到 server/Cargo.toml dev-dependencies

### Step 2: 创建测试辅助函数
- [x] `init_test_repo()` - 初始化临时 git 仓库 (phase18_git_watcher.rs)
- [x] `create_commit_with_metadata()` - 创建带 metadata 的 commit (phase18_git_watcher.rs)
- [x] `setup_db()` - 设置内存数据库 (phase18_scenarios.rs)
- [x] `seed_project()` - 创建测试项目 (phase18_scenarios.rs)
- [x] `create_ready_workflow()` - 创建就绪状态的 workflow (phase18_scenarios.rs)
- [x] `create_workflow_task()` - 创建工作流任务 (phase18_scenarios.rs)
- [x] `create_terminal()` - 创建终端 (phase18_scenarios.rs)
- [x] `set_encryption_key()` with EnvGuard - 线程安全的环境变量设置 (phase18_scenarios.rs)
- [x] `assert_cli_model_exists()` - 验证种子数据 (phase18_scenarios.rs)

### Step 3: 实现 Task 18.1 测试
- [x] `test_workflow_status_transition_created_to_ready` - 状态转换测试
- [x] `test_workflow_with_tasks_and_terminals` - 工作流结构测试
- [x] `test_commit_metadata_parsing` - 提交元数据解析测试
- [x] `test_workflow_task_status_transitions` - 任务状态转换测试
- [x] `test_workflow_find_by_project` - 按项目查找工作流测试

### Step 4: 实现 Task 18.2 测试
- [x] `test_message_bus_terminal_completion_event` - 消息总线事件测试
- [x] `test_terminal_status_update_on_failure` - 终端失败状态更新测试
- [x] `test_multiple_terminals_in_task` - 多终端任务测试
- [x] `test_orchestrator_runtime_creation` - 编排器运行时创建测试
- [x] `test_terminal_last_commit_update` - 终端最后提交更新测试
- [x] `test_terminal_recovery_marks_waiting_as_failed` - 恢复孤立终端测试
- [x] `test_workflow_recovery_with_mixed_terminal_states` - 混合状态恢复测试
- [x] Git Watcher 测试 (9个测试用例)

### Step 5: 验证与清理
- [x] 运行所有测试 (22个测试全部通过)
- [x] 修复失败的测试
- [x] 提交代码

## 完成状态

**Phase 18 E2E 测试实施已完成** ✅

### 测试统计
- `phase18_scenarios.rs`: 13 个测试
- `phase18_git_watcher.rs`: 9 个测试
- **总计: 22 个测试全部通过**

### 关键改进
1. 使用 async mutex (ENV_MUTEX) 实现线程安全的环境变量访问
2. RAII EnvGuard 模式确保环境变量正确清理
3. 添加恢复场景测试覆盖孤立终端处理

## 文件结构

```
crates/server/tests/
├── phase18_e2e_workflow.rs      # E2E 全流程测试
└── phase18_scenarios.rs         # 并发/失败/恢复场景测试

crates/services/tests/
└── phase18_scenarios.rs         # 服务层场景测试
```
