# GitCortex TODO — 项目状态总览

> 更新时间：2026-03-15
> 目的：本文件是当前唯一的未完成总计划入口。
> 所有已完成阶段（Phase 0-29 + 全量审计修复 + BACKLOG-002/003）已归档至 `docs/developed/`。

## 当前状态

- Phase 0-29 全部交付完成 ✅
- 全量代码审计（36组6轮）已完成 ✅ — 报告归档至 `docs/developed/issues/2026-03-14-full-code-audit-master.md`
- 审计修复全部完成 ✅ — 373 个 G-ID 全部修复，63 个文件，~5681 行新增 / ~976 行删除
- P1/P2 生产就绪修复已完成 ✅ — 11 项关键问题已修复
- BACKLOG-002 Runner 容器分离 ✅ — 架构实现完成（详见下方剩余 stub）
- BACKLOG-003 CLI 安装状态 API 增强 ✅ — 架构实现完成（详见下方剩余 stub）
- CI 状态：✅ 全绿（Basic Checks / Quality Gate / Docker Build 均 success）
- SonarCloud：0 bugs, 0 vulnerabilities, 0 code smells, 0 security hotspots — A rating

## 剩余收尾项（Stub / 注释代码）

以下为已完成功能中遗留的少量 stub，不影响现有功能运行：

| 项目 | 说明 | 优先级 |
|------|------|--------|
| RemoteRunner gRPC client | `crates/services/src/services/runner_client.rs` 中 `RemoteRunner` 为 placeholder，所有方法返回 `bail!`。需集成 tonic gRPC client 代码 | 低 — 仅远程 Runner 模式需要 |
| WebSocket 安装进度 | `crates/server/src/routes/cli_types.rs` 中 `install_progress_ws()` 为 skeleton，需订阅 broadcast channel | 低 — 安装仍可通过 REST 轮询 |
| SSE 路由未启用 | `crates/server/src/routes/mod.rs` 中 SSE 路由被注释，需添加 `SharedCliHealthMonitor` Extension 层后启用 | 低 — 手动刷新仍可用 |

## 文档入口

- 已完成任务清单：`docs/developed/misc/TODO-completed.md`
- 全量代码审计报告：`docs/developed/issues/2026-03-14-full-code-audit-master.md`
- 审计修复状态跟踪：`docs/developed/plans/2026-03-14-audit-fix-status.md`
- Runner 容器分离计划：`docs/developed/plans/BACKLOG-002-runner-container-separation.md`
- CLI 安装状态 API 计划：`docs/developed/plans/BACKLOG-003-cli-installation-status-api.md`
- Phase 0 详细计划：`docs/developed/plans/XX-phase-0-backend-foundation.md`
- Phase 1 详细计划：`docs/developed/plans/XX-phase-1-frontend-core.md`
- Phase 2 详细计划：`docs/developed/plans/XX-phase-2-integration.md`

## 维护规则

1. 新完成事项从本文件移除，同步更新 `docs/developed/misc/TODO-completed.md`。
2. 本文件仅保留当前未完成计划、风险和 backlog。
3. 里程碑完成后，归档到 `docs/developed/`。
