# GitCortex TODO — 项目状态总览

> 更新时间：2026-03-15
> 目的：本文件是当前唯一的未完成总计划入口。
> 所有已完成阶段（Phase 0-29 + 全量审计修复）已归档至 `docs/developed/`。

## 当前状态

- Phase 0-29 全部交付完成。
- 全量代码审计（36组6轮）已完成，报告归档至 `docs/developed/issues/2026-03-14-full-code-audit-master.md`。
- 审计修复全部完成 ✅ — 373 个 G-ID 全部修复，63 个文件，~5681 行新增 / ~976 行删除。
- P1/P2 生产就绪修复已完成 ✅ — 11 项关键问题已修复（详见下方）。
- CI 状态：✅ 全绿（Basic Checks / Quality Gate / Docker Build 均 success）
- SonarCloud：0 bugs, 0 vulnerabilities, 0 code smells, 0 security hotspots — A rating

## P1/P2 生产就绪修复（2026-03-15）

以下 P1/P2 问题已全部修复：

| 问题 | 修复内容 | 状态 |
|------|----------|------|
| Pause/Resume 逻辑 | 暂停时 terminal 状态重置为 not_started（而非 cancelled），resume 可自动 re-prepare | ✅ |
| Workflow 完成 vs Merge 竞态 | auto_sync 使用 CAS (running→completed)，不覆盖并发状态变更 | ✅ |
| WaitingForApproval 无超时 | 添加 5 分钟超时，状态机自动 reset 到 Idle | ✅ |
| GitHub CI 安全漏洞 | 添加输入验证（SHA 格式、conclusion 白名单） | ✅ |
| Worktree 停止后未清理 | stop_workflow 后自动清理 worktree 目录 | ✅ |
| 消息总线错误静默丢弃 | 所有 `let _ =` 替换为 `tracing::warn!` 日志 | ✅ |
| Merge 缺少回滚点 | merge 前记录 HEAD SHA，失败时日志输出回滚命令 | ✅ |
| CORS 过于宽松 | 支持 GITCORTEX_CORS_ORIGINS 环境变量限制来源 | ✅ |
| CI Webhook 缺少签名验证 | 支持 GITCORTEX_CI_WEBHOOK_SECRET HMAC-SHA256 签名 | ✅ |

## 既有 Backlog

| ID | 描述 | 优先级 |
|----|------|--------|
| BACKLOG-002 | Runner 容器分离 | 中 |
| BACKLOG-003 | CLI 安装状态 API | 中 |

## 文档入口

- 已完成任务清单：`docs/developed/misc/TODO-completed.md`
- 全量代码审计报告：`docs/developed/issues/2026-03-14-full-code-audit-master.md`
- 审计修复状态跟踪：`docs/developed/plans/2026-03-14-audit-fix-status.md`
- Phase 0 详细计划：`docs/developed/plans/XX-phase-0-backend-foundation.md`
- Phase 1 详细计划：`docs/developed/plans/XX-phase-1-frontend-core.md`
- Phase 2 详细计划：`docs/developed/plans/XX-phase-2-integration.md`

## 维护规则

1. 新完成事项从本文件移除，同步更新 `docs/developed/misc/TODO-completed.md`。
2. 本文件仅保留当前未完成计划、风险和 backlog。
3. 里程碑完成后，归档到 `docs/developed/`。
