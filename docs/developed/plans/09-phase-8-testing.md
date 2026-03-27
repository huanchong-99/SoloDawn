# Phase 8: 集成测试与文档

> **状态:** ⬜ 未开始
> **进度追踪:** 查看 `TODO.md`
> **前置条件:** Phase 7 完成

## 概述

编写端到端测试，进行性能优化，完善用户文档。

---

## Phase 8: 集成测试与文档

### Task 8.1: 端到端测试

**状态:** ⬜ 未开始

**前置条件:**
- Phase 7 已完成

**目标:**
编写端到端测试，验证工作流创建、启动、执行的完整流程。

**涉及文件:**
- 创建: `vibe-kanban-main/tests/e2e/workflow_test.rs`

---

**Step 8.1.1: 创建 workflow_test.rs**

```rust
//! 工作流端到端测试

use reqwest::Client;
use serde_json::json;

const BASE_URL: &str = "http://localhost:3001";

#[tokio::test]
async fn test_workflow_lifecycle() {
    let client = Client::new();

    // 1. 获取 CLI 类型
    let res = client.get(format!("{}/api/cli_types", BASE_URL))
        .send().await.unwrap();
    assert!(res.status().is_success());
    let cli_types: Vec<serde_json::Value> = res.json().await.unwrap();
    assert!(!cli_types.is_empty());

    let claude_cli = cli_types.iter()
        .find(|c| c["name"] == "claude-code")
        .expect("Claude CLI not found");

    // 2. 获取模型
    let cli_id = claude_cli["id"].as_str().unwrap();
    let res = client.get(format!("{}/api/cli_types/{}/models", BASE_URL, cli_id))
        .send().await.unwrap();
    assert!(res.status().is_success());
    let models: Vec<serde_json::Value> = res.json().await.unwrap();
    let model_id = models[0]["id"].as_str().unwrap();

    // 3. 创建工作流
    let workflow_req = json!({
        "project_id": "test-project",
        "name": "Test Workflow",
        "use_slash_commands": false,
        "merge_terminal_config": {
            "cli_type_id": cli_id,
            "model_config_id": model_id
        },
        "tasks": [{
            "name": "Test Task",
            "terminals": [{
                "cli_type_id": cli_id,
                "model_config_id": model_id,
                "role": "coder"
            }]
        }]
    });

    let res = client.post(format!("{}/api/workflows", BASE_URL))
        .json(&workflow_req)
        .send().await.unwrap();
    assert!(res.status().is_success());
    let workflow: serde_json::Value = res.json().await.unwrap();
    let workflow_id = workflow["workflow"]["id"].as_str().unwrap();

    // 4. 获取工作流详情
    let res = client.get(format!("{}/api/workflows/{}", BASE_URL, workflow_id))
        .send().await.unwrap();
    assert!(res.status().is_success());

    // 5. 删除工作流
    let res = client.delete(format!("{}/api/workflows/{}", BASE_URL, workflow_id))
        .send().await.unwrap();
    assert!(res.status().is_success());
}

#[tokio::test]
async fn test_cli_detection() {
    let client = Client::new();

    let res = client.get(format!("{}/api/cli_types/detect", BASE_URL))
        .send().await.unwrap();
    assert!(res.status().is_success());

    let detection: Vec<serde_json::Value> = res.json().await.unwrap();
    assert!(!detection.is_empty());

    // 检查返回格式
    for cli in detection {
        assert!(cli.get("cli_type_id").is_some());
        assert!(cli.get("installed").is_some());
    }
}
```

---

**交付物:** `tests/e2e/workflow_test.rs`

---

### Task 8.2: 性能优化

**状态:** ⬜ 未开始

**目标:**
优化数据库查询和 WebSocket 连接管理。

**涉及文件:**
- 修改: 多个文件

---

**Step 8.2.1: 数据库索引优化**

确保以下索引存在（在迁移文件中已添加）：

```sql
-- 工作流查询优化
CREATE INDEX IF NOT EXISTS idx_workflow_project_status ON workflow(project_id, status);

-- 终端查询优化
CREATE INDEX IF NOT EXISTS idx_terminal_workflow_task_status ON terminal(workflow_task_id, status);

-- Git 事件查询优化
CREATE INDEX IF NOT EXISTS idx_git_event_workflow_status ON git_event(workflow_id, process_status);
```

---

**Step 8.2.2: 连接池配置**

在 `DBService` 中配置连接池：

```rust
let pool = SqlitePoolOptions::new()
    .max_connections(10)
    .min_connections(2)
    .acquire_timeout(Duration::from_secs(30))
    .idle_timeout(Duration::from_secs(600))
    .connect(&database_url)
    .await?;
```

---

**交付物:** 优化后的代码

---

### Task 8.3: 用户文档

**状态:** ⬜ 未开始

**目标:**
更新用户文档，说明新功能的使用方法。

**涉及文件:**
- 修改: `README.md`
- 创建: `docs/workflow-guide.md`

---

**Step 8.3.1: 更新 README.md**

添加工作流功能说明章节。

---

**Step 8.3.2: 创建 workflow-guide.md**

```markdown
# SoloDawn 工作流使用指南

## 概述

SoloDawn 工作流允许您协调多个 AI 编码代理并行完成复杂的软件开发任务。

## 创建工作流

1. 进入项目页面
2. 点击"创建工作流"按钮
3. 按照向导配置：
   - 工作流名称和描述
   - 并行任务（每个任务对应一个 Git 分支）
   - 每个任务的终端配置（CLI 类型和模型）
   - 合并终端配置

## 工作流状态

| 状态 | 说明 |
|------|------|
| created | 已创建，等待启动 |
| starting | 正在启动终端 |
| ready | 所有终端就绪，等待确认 |
| running | 正在执行 |
| merging | 正在合并分支 |
| completed | 已完成 |
| failed | 失败 |

## 终端调试

在工作流运行时，您可以：
1. 切换到"终端调试"标签页
2. 选择要查看的终端
3. 实时查看终端输出
4. 必要时手动输入命令

## 最佳实践

1. 将大任务拆分为独立的并行任务
2. 每个任务使用独立的 Git 分支
3. 配置审核终端以确保代码质量
4. 使用合适的模型（复杂任务用 Opus，简单任务用 Sonnet）
```

---

**交付物:** `docs/workflow-guide.md`

**验收标准:**
1. 文档清晰易懂
2. 包含所有主要功能说明

---

## Phase 8 完成检查清单

- [ ] Task 8.1: 端到端测试完成
- [ ] Task 8.2: 性能优化完成
- [ ] Task 8.3: 用户文档完成

---

## 附录
| 数据库模型 | `crates/db/src/models/workflow.rs` | 工作流模型 |
| 数据库模型 | `crates/db/src/models/terminal.rs` | 终端模型 |
| 数据库模型 | `crates/db/src/models/cli_type.rs` | CLI 类型模型 |
| API 路由 | `crates/server/src/routes/workflows.rs` | 工作流 API |
| API 路由 | `crates/server/src/routes/cli_types.rs` | CLI 类型 API |
| CC-Switch | `crates/cc-switch/src/lib.rs` | CC-Switch 入口 |
| CC-Switch | `crates/cc-switch/src/switcher.rs` | 模型切换服务 |
| 服务层 | `crates/services/src/services/cc_switch.rs` | CC-Switch 服务封装 |

### B. API 端点汇总

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | /api/cli_types | 获取所有 CLI 类型 |
| GET | /api/cli_types/detect | 检测已安装的 CLI |
| GET | /api/cli_types/:id/models | 获取 CLI 的模型列表 |
| GET | /api/workflows | 获取工作流列表 |
| POST | /api/workflows | 创建工作流 |
| GET | /api/workflows/:id | 获取工作流详情 |
| DELETE | /api/workflows/:id | 删除工作流 |
| PUT | /api/workflows/:id/status | 更新工作流状态 |
| POST | /api/workflows/:id/start | 启动工作流 |
| GET | /api/workflows/presets/commands | 获取斜杠命令预设 |

### C. 数据库表汇总

| 表名 | 说明 |
|------|------|
| cli_type | CLI 类型 |
| model_config | 模型配置 |
| slash_command_preset | 斜杠命令预设 |
| workflow | 工作流 |
| workflow_command | 工作流命令关联 |
| workflow_task | 工作流任务 |
| terminal | 终端 |
| terminal_log | 终端日志 |
| git_event | Git 事件 |

---

*文档版本: 2.0*
*创建日期: 2026-01-16*
*最后更新: 2026-01-17*
