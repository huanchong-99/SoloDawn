# GitCortex TODO

> 更新时间：2026-03-14
> 目的：`docs/undeveloped/current/TODO.md` 是当前唯一的未完成总计划入口。
> 所有已完成阶段（Phase 0-29）已归档至 `docs/developed/`。

## 当前状态

- Phase 0-29 全部交付完成。
- 全量代码审计（36组6轮）已完成，报告归档至 `docs/developed/issues/2026-03-14-full-code-audit-master.md`。
- 审计修复进行中，部分 Batch 尚未完成，详见下方。

## 审计修复 — 未完成项

> 详细状态跟踪：`docs/undeveloped/current/2026-03-14-audit-fix-status.md`

### 优先级 1：CI 修复 ✅ 已完成

- [x] 修复 2 个 Rust 测试失败
- [x] 推送并确认 CI 全绿

### 优先级 2：前端核心文件剩余问题（Batch 8 未完成部分）

- [ ] `Workflows.tsx`: G07-004/007/011, G08-003/006, G26-004-009, G27-003-008
- [ ] `useWorkflows.ts`: G02-004/007, G05-009, G26-003/006/012, G30-004/006
- [ ] `wsStore.ts`: G08-007, G12-001/003/009, G27-001, G30-007/008

### 优先级 3：前端组件（Batch 9 未完成部分）

- [ ] `WorkflowDebugPage.tsx`: G28-001/003/008 (mapTerminalStatus 修复)
- [ ] `TerminalDebugView.tsx`: G09-002/012, G28-002/006/007/009
- [ ] `TerminalEmulator.tsx`: G28-005/011, G09-009
- [ ] `DisplayConversationEntry.tsx`: G09-011 (lucide→phosphor 迁移)
- [ ] `OrchestratorChatPanel.tsx`: G28-004 (轮询→WS TODO)
- [ ] Wizard 验证器: G25-003/004/005/006/009
- [ ] Wizard 步骤: G25-007/008/010/013/014/016/018

### 优先级 4：辅助模块与集成（Batch 10）

- [ ] Feishu: G32-001 到 G32-018
- [ ] Quality Gate: G31-001 到 G31-009
- [ ] Events/SSE: G33-001 到 G33-010
- [ ] Task Attempts: G34-001 到 G34-012
- [ ] api.ts: G30-005/009/010/011
- [ ] contexts: G36-009/010
- [ ] i18n: G36-001/002/012
- [ ] generate_types.rs: G17-001, G36-003
- [ ] utils: G36-007/008
- [ ] CLAUDE.md: G08-004

## 既有 Backlog

| ID | 描述 | 优先级 |
|----|------|--------|
| BACKLOG-001 | Docker Deployment 抽象 | 低 |
| BACKLOG-002 | Runner 容器分离 | 低 |
| BACKLOG-003 | CLI 安装状态 API | 中 |
| BACKLOG-004 | K8s 部署支持 | 低 |
| BACKLOG-005 | 镜像体积优化 | 中 |

## 文档入口

- 已完成任务清单：`docs/developed/misc/TODO-completed.md`
- 历史全量快照（只读）：`docs/developed/misc/TODO-legacy-full-2026-02-23.md`
- 历史执行看板归档：`docs/developed/misc/TODO-pending-archived-2026-03-11.md`
- 全量代码审计报告：`docs/developed/issues/2026-03-14-full-code-audit-master.md`
- 审计修复状态跟踪：`docs/undeveloped/current/2026-03-14-audit-fix-status.md`
- Phase 29 计划文档：`docs/developed/plans/2026-03-13-phase-29-quality-gate-design.md`

## 维护规则

1. 新完成事项从本文件移除，同步更新 `docs/developed/misc/TODO-completed.md`。
2. 本文件仅保留当前未完成计划、风险和 backlog。
3. 里程碑完成后，把稳定沉淀内容归档到 `docs/developed/`，避免 `docs/undeveloped/current/` 膨胀。
