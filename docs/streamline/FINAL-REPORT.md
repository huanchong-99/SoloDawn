# SoloDawn 精简 + 质量门规则改造 — 最终报告

**分支**：`refactor/streamline-and-quality-gate-rules`（off `9995e65eb`，未推送、未触碰 main）
**日期**：2026-06-14
**方式**：ultracode 多 agent 编排（P0–P6，11 个 workflow，约 600+ subagent）

## 最终门禁状态（全绿）

| 门 | 结果 |
|----|------|
| `cargo check --workspace --all-targets` | ✅ 0 错误 |
| `cargo clippy --workspace --all-targets` (`RUST_MIN_STACK` + `-j1`) | ✅ 0 错误，2 预存警告（baseline 已有） |
| `cargo test --workspace --no-run` (`-j1`) | ✅ 0 错误 |
| G2/G3 后端测试 (6) | ✅ 全过 |
| frontend `tsc --noEmit` | ✅ 0 错误 |
| frontend `vitest run` | ✅ 458 测试全过（453 基线 + 5 新功能） |
| frontend `eslint` | ✅ 10（= 基线，全是预存 `.cjs` 配置文件） |
| `generate_types` 幂等 | ✅ shared/types.ts 一致 |

构建前置：`RUST_MIN_STACK=268435456` + `-j 1`（重负载 codegen 否则 ICE/OOM，详见 memory）。

## 规模

349 文件变更，+15429 / −11510 行（含审计文档）。删除 **57 文件 / ~58 死符号集**；前端真实代码 −5700 行（26928→21229）。

## 三大目标

### G1 — 精简（审计→评委→对抗裁决，非预删）
- 201 候选 → 75+ 确认删除（对抗验证拦下 5 个误删：RB-23/FE-18/FE-37/FE-69/FE-79）。
- **IDE 外部编辑器功能**整条删除（前后端+i18n+类型重生成）。**保留** `EditorConfig`/`EditorType`（配置 schema 骨架）、`vscode/bridge.ts`（剪贴板被所有聊天框 wysiwyg 使用，**非** IDE 功能）、`check_editor_availability`（RB-64 延后）。
- 顺带修真实 bug：RB-37 三 CLI 临时密钥残留泄漏（P0 安全）、RB-38/39、FE-54/56/57 等。

### G2 — confirm→materialize 间质量门二次确认（硬门禁）
- 新列 `planning_draft.gates_confirmed_at` + `materialize_draft` 未确认返回 **400**（在 `Workflow::create` 之前，前端绕不过）+ `POST /confirm-gates`。

### G3 — 按项目可编辑质量门规则（System A）
- 新表 `project_quality_policy`（`project_id` BLOB FK）；services 解析器 `resolve_quality_config`（DB→repo yaml→内置）；`QualityEngine::from_config`；CRUD 路由；ts-rs 类型；**一个共享 `QualityGateRulesEditor`** 同时服务二次确认弹窗与新设置页（MetricKey 闭枚举选择器 + 11 provider + GT/LT）。

## 已记录的延后项（未做，含理由）
- FE-22 shared-tasks(ElectricSQL) 子图删除：风险大，保守跳过。
- FollowUpConflictSection/ConflictBanner、openTaskForm.ts：孤儿死代码，未渲染，留待。
- ~115 个 P2 低优先/高风险项（legacy task-detail UI 簇、RB-D18 runner 保留、RB-K02 redis 保留、RB-K03 审计回退保留等）。
- container.rs Site3 advisory gate 仍走 from_project（两个强制门已走 DB 优先级）。
- qualityGates.* i18n 用 defaultValue 兜底（nav key 已加 en/zh-Hans）。

## UNAVAILABLE（如实标注，未伪装）
- 实时 app 端到端：未做（依赖 DB+服务全栈启动）；以编译门 + 自动化测试覆盖替代。
- fast-context MCP 在 P1/P2 高并发期被 Windsurf 配额限流，部分 agent 退化为穷举 grep 并已标注（精确符号检索 grep 本即金标准）。

## 审计轨迹
`docs/streamline/`：census/*（普查）、ledger-*.md、P2-candidate-ledger.md、P4-FINAL.md（执行计划）、P3-design-spec.md（功能设计）、各门禁日志。
