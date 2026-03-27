# SoloDawn 用户使用指南

> 版本: 0.0.153
> 更新日期: 2026-01-30

## 目录

1. [简介](#简介)
2. [安装与配置](#安装与配置)
3. [快速开始](#快速开始)
4. [核心概念](#核心概念)
5. [工作流管理](#工作流管理)
6. [终端操作](#终端操作)
7. [常见问题](#常见问题)

---

## 简介

SoloDawn 是一个智能代码编排平台，通过工作流自动化管理多个 AI 编码代理，实现复杂开发任务的并行执行和协调。

### 主要功能

- **工作流编排**: 创建和管理多任务工作流
- **多终端支持**: 同时运行多个 AI 编码代理
- **Git 集成**: 自动分支管理和提交追踪
- **实时监控**: WebSocket 实时状态更新
- **智能合并**: 自动化代码合并和冲突处理

---

## 安装与配置

### 系统要求

- **操作系统**: Windows 10+, macOS 10.15+, Linux (Ubuntu 20.04+)
- **内存**: 最低 4GB，推荐 8GB+
- **磁盘**: 最低 1GB 可用空间
- **Git**: 2.30+
- **Node.js**: 18+ (前端)
- **Rust**: 1.75+ (后端编译)

### 安装步骤

#### 1. 克隆仓库

```bash
git clone https://github.com/your-org/solodawn.git
cd solodawn
```

#### 2. 配置环境变量

创建 `.env` 文件：

```bash
# 必需配置
SOLODAWN_ENCRYPTION_KEY=your-32-character-encryption-key

# 可选配置
DATABASE_URL=sqlite:./data/solodawn.db
SERVER_PORT=3001
LOG_LEVEL=info
```

**重要**: `SOLODAWN_ENCRYPTION_KEY` 必须是 32 字符的字符串，用于加密 API 密钥。

#### 3. 构建后端

```bash
cargo build --release
```

#### 4. 安装前端依赖

```bash
cd frontend
pnpm install
pnpm build
```

#### 5. 启动服务

```bash
# 启动后端服务
./target/release/server

# 或使用开发模式
cargo run --package server
```

---

## 快速开始

### 创建第一个工作流

1. **打开 Web 界面**

   访问 `http://localhost:3001`

2. **创建项目**

   - 点击 "新建项目"
   - 输入项目名称和本地路径
   - 选择 Git 仓库

3. **创建工作流**

   - 在项目页面点击 "新建工作流"
   - 填写工作流名称和描述
   - 配置目标分支（默认 `main`）

4. **添加任务**

   - 点击 "添加任务"
   - 输入任务名称和描述
   - 为任务添加终端（选择 CLI 类型和模型）

5. **启动工作流**

   - 确认所有配置正确
   - 点击 "启动" 按钮
   - 监控执行进度

### 命令行快速创建

```bash
# 使用 API 创建工作流
curl -X POST http://localhost:3001/api/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "projectId": "your-project-id",
    "name": "Feature Implementation",
    "description": "Implement new feature",
    "targetBranch": "main",
    "tasks": [{
      "name": "Task 1",
      "description": "First task",
      "orderIndex": 0,
      "terminals": [{
        "cliTypeId": "claude-code",
        "modelConfigId": "claude-sonnet",
        "orderIndex": 0
      }]
    }]
  }'
```

---

## 核心概念

### 工作流 (Workflow)

工作流是任务的容器，代表一个完整的开发目标。

**状态流转**:
```
created → ready → starting → running → completed
                                    ↘ failed
```

### 任务 (Task)

任务是工作流中的独立工作单元，可以包含多个终端。

**状态**:
- `pending`: 等待执行
- `running`: 正在执行
- `completed`: 已完成
- `failed`: 执行失败

### 终端 (Terminal)

终端是实际执行代码的 AI 代理实例。

**支持的 CLI 类型**:
- `claude-code`: Claude Code CLI
- `cursor`: Cursor AI
- `windsurf`: Windsurf AI

### 分支策略

每个工作流自动创建独立分支：
```
workflow/{workflow-id}/{workflow-name-slug}
```

如果分支名已存在，会自动添加后缀：
```
workflow/{workflow-id}/{workflow-name-slug}-2
```

---

## 工作流管理

### 创建工作流

**必需字段**:
- `projectId`: 项目 ID
- `name`: 工作流名称
- `targetBranch`: 目标分支
- `tasks`: 至少一个任务

**可选字段**:
- `description`: 工作流描述
- `useSlashCommands`: 是否使用斜杠命令
- `orchestratorConfig`: 编排器配置（LLM API）

### 工作流操作

| 操作 | 说明 | 前置状态 |
|------|------|----------|
| 启动 | 开始执行工作流 | `ready` |
| 暂停 | 暂停执行 | `running` |
| 恢复 | 恢复执行 | `paused` |
| 取消 | 取消工作流 | 任意非终态 |
| 合并 | 合并到目标分支 | `completed` |

### 监控工作流

当前版本使用轮询方式获取工作流状态：

```bash
# 获取工作流状态
curl http://localhost:3001/api/workflows/{id}
```

---

## 终端操作

### 连接终端

使用路径参数格式连接终端：

```javascript
// 使用路径参数格式
const ws = new WebSocket('ws://localhost:3001/api/terminal/{terminal_id}');

ws.onopen = () => {
  console.log('Terminal connected');
};

ws.onmessage = (event) => {
  console.log('Terminal output:', event.data);
};
```

### 发送输入

```javascript
ws.send(JSON.stringify({
  type: 'input',
  data: 'your command here'
}));
```

### 终端状态

- `idle`: 空闲
- `waiting`: 等待输入
- `running`: 正在执行
- `completed`: 已完成
- `failed`: 执行失败

---

## 常见问题

### Q: 工作流创建失败，提示 "CLI type not found"

**A**: 确保已正确配置 CLI 类型。检查数据库中的 `cli_types` 表是否有数据。

### Q: 终端连接超时

**A**: 检查以下几点：
1. 服务器是否正常运行
2. 防火墙是否阻止 WebSocket 连接
3. 终端 ID 是否正确

### Q: API 密钥加密失败

**A**: 确保 `SOLODAWN_ENCRYPTION_KEY` 环境变量已设置且为 32 字符。

### Q: Git 分支创建失败

**A**: 检查：
1. 项目路径是否正确
2. Git 仓库是否初始化
3. 是否有足够的文件系统权限

### Q: 工作流状态卡在 "starting"

**A**: 可能原因：
1. 终端启动失败
2. CLI 工具未正确安装
3. 查看服务器日志获取详细错误信息

### Q: 如何重置工作流状态？

**A**: 可以通过更新工作流状态来重置，或删除后重新创建：

```bash
# 方法1: 更新工作流状态（推荐）
curl -X PATCH http://localhost:3001/api/workflows/{id}/status \
  -H "Content-Type: application/json" \
  -d '{"status": "created"}'

# 方法2: 删除工作流后重新创建
curl -X DELETE http://localhost:3001/api/workflows/{id}

# 然后重新创建
curl -X POST http://localhost:3001/api/workflows ...
```

---

## 获取帮助

- **文档**: https://docs.solodawn.io
- **问题反馈**: https://github.com/your-org/solodawn/issues
- **社区讨论**: https://discord.gg/solodawn

---

*本文档最后更新于 2026-01-30*
