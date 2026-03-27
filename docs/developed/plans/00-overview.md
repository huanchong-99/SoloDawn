# SoloDawn 实现计划概览

> **For Claude:** 使用 `superpowers-automation` skill 自动执行此计划。
> 进度追踪在 `TODO.md`，详细任务在各 phase 文件中。

## 项目目标

基于 Vibe Kanban 改造并集成 CC-Switch，实现主 Agent 跨终端任务协调系统。

## 技术栈

- **后端:** Rust (axum 0.8.4, sqlx, tokio)
- **前端:** React 18 + TypeScript + Tailwind CSS
- **数据库:** SQLite
- **终端:** xterm.js + WebSocket

## 源项目位置

- **Vibe Kanban:** `F:\Project\SoloDawn\vibe-kanban-main`
- **CC-Switch:** `F:\Project\SoloDawn\cc-switch-main`

## 设计文档

详细架构设计请参考: `2026-01-16-orchestrator-design.md`

---

## 阶段概览

| Phase | 文件 | 内容 | 任务数 | 依赖 |
|-------|------|------|--------|------|
| 0 | `01-phase-0-docs.md` | 项目文档重写 | 2 | 无 |
| 1 | `02-phase-1-database.md` | 数据库模型扩展 | 4 | Phase 0 |
| 2 | `03-phase-2-cc-switch.md` | CC-Switch 核心提取与集成 | 5 | Phase 1 |
| 3 | `04-phase-3-orchestrator.md` | Orchestrator 主 Agent 实现 | 4 | Phase 2 |
| 4 | `05-phase-4-terminal.md` | 终端管理与启动机制 | 3 | Phase 3 |
| 5 | `06-phase-5-git-watcher.md` | Git 事件驱动系统 | 3 | Phase 4 |
| 6 | `07-phase-6-frontend.md` | 前端界面改造 (7步向导) | 5 | Phase 5 |
| 7 | `08-phase-7-terminal-debug.md` | 终端调试视图 | 3 | Phase 6 |
| 8 | `09-phase-8-testing.md` | 集成测试与文档 | 3 | Phase 7 |

**总计: 32 个任务**

---

## 里程碑

| 里程碑 | 覆盖 Phase | 关键产出 | 验收标准 |
|--------|-----------|---------|----------|
| M1 数据层就绪 | 0-1 | 数据库 + API | `cargo sqlx migrate run` 成功 |
| M2 模型切换 | 2 | cc-switch crate | 配置切换测试通过 |
| M3 协调引擎 | 3-4 | Orchestrator + Terminal | 可启动多终端 |
| M4 Git驱动 | 5 | GitWatcher | 检测到 commit 触发事件 |
| M5 用户界面 | 6-7 | 7步向导 + 调试视图 | 前端可创建工作流 |
| M6 生产就绪 | 8 | 测试 + 文档 | E2E 测试全部通过 |

---

## 架构依赖图

```
Phase 0 (文档)
    ↓
Phase 1 (数据库)
    ↓
Phase 2 (CC-Switch) ←── cc-switch-main 源码
    ↓
Phase 3 (Orchestrator)
    ↓
Phase 4 (终端管理)
    ↓
Phase 5 (Git监听)
    ↓
Phase 6 (前端向导) ←── vibe-kanban-main/frontend
    ↓
Phase 7 (终端调试)
    ↓
Phase 8 (测试文档)
```

---

## 关键路径

1. **数据库优先:** Phase 1 必须先完成，后续所有功能依赖数据模型
2. **CC-Switch 提取:** Phase 2 需要仔细分析依赖，剥离 Tauri 相关代码
3. **前端改造:** Phase 6 是最大的改动，需要遵循 vibe-kanban 的设计规范

## 风险点

| 风险 | 影响 Phase | 缓解措施 |
|------|-----------|----------|
| CC-Switch 有 Tauri 依赖 | 2 | 分析并替换为纯 Rust 实现 |
| Windows 文件监听性能 | 5 | 使用 notify crate 优化 |
| xterm.js WebSocket 集成 | 7 | 参考现有实现示例 |

---

## 如何使用此计划

### 自动执行 (推荐)

```
/superpowers-automation
```

Skill 会自动：
1. 读取 `TODO.md` 找到下一个待完成任务
2. 加载对应的 phase 文件
3. 派发子代理执行任务
4. 更新 `TODO.md` 进度
5. 上下文压缩时自动重启继续

### 手动执行

1. 查看 `TODO.md` 确定当前进度
2. 打开对应的 phase 文件
3. 按照 Task 步骤执行
4. 完成后更新 `TODO.md`
