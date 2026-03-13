# GitCortex TODO

> 更新时间：2026-03-13
> 目的：把 `docs/undeveloped/current/TODO.md` 升级为当前唯一的未完成总计划入口。
> 当前最高优先级：Phase 29 — 全栈代码质量门 / Sonar 化改造（P0）

## 当前状态

- 新增 P0 Epic：把“AI terminal 提交代码 -> 自动质量分析 -> 结果回流原 terminal 修复 -> 通过后才允许进入下一个 terminal / review / merge”固化为内建编排能力。
- 当前仓库已经有 `cargo check` / `cargo test` / `clippy` / `frontend eslint+tsc+vitest` / 手工 SonarCloud 报告沉淀，但没有仓库内可复制的 Sonar 配置、质量规则版本化资产、terminal 级质量门状态机、UI 报告回传链路。
- 当前环境未提供 `zread` MCP；本计划基于仓库现状与 Sonar 官方文档制定。后续实施时如接入 zread，可用于补齐 Sonar Web API、Quality Profile、SARIF 导入的细节对照。
- 现有 orchestrator 语义是“terminal commit 完成 -> 直接推进 `handle_terminal_completed()` / ReviewCode / FixIssues”；这与“同一 terminal 接收质量结果继续修”直接冲突，必须做编排层改造。

## 战略决策

| ID | 决策 | 说明 |
|----|------|------|
| D1 | 采用“本地质量门 + Sonar 聚合”的双层架构 | 本地质量门负责 terminal 级快速阻断；SonarQube/SonarCloud 负责统一规则、趋势、PR/主分支门禁与可视化 |
| D2 | 不把质量状态硬塞进 `terminal.status` | `terminal.status` 保持 PTY 生命周期；新增独立 `quality_run / quality_gate / quality_state` 维度，UI 再做聚合展示 |
| D3 | terminal 提交从“最终完成”改为“checkpoint” | 质量门启用的 workflow 中，代码终端提交的是 checkpoint commit；只有质量门通过后，orchestrator 才把它视为真正 completed |
| D4 | 质量失败默认回流原 terminal 修复 | 静态质量问题不再默认新建 fixer terminal；原 terminal 继续拥有该任务上下文与文件写集 |
| D5 | `ReviewCode / FixIssues` 保留，但语义后移 | 先过静态质量门，再进入语义 review/fix 循环；避免 reviewer/fixer 去修一堆本该自动拦截的低级问题 |
| D6 | 规则必须版本化进仓库 | 新增 `quality/` 目录，保存质量门策略、Sonar 质量配置导出、报告模板、baseline/豁免策略 |
| D7 | 先拦“新代码”，再治理历史债务 | Sonar 质量门初期只阻断新代码；历史 debt 以基线报告管理，防止第一天就把整个项目锁死 |
| D8 | 支持 provider 可插拔 | 首发落地 `local` + `sonar`，保留 `semgrep` / `codeql` / 其他 SARIF provider 的扩展位 |
| D9 | UI 必须双通道回传 | 质量门结果同时进入 workflow events 和 execution-process/log streams；只做一条链路会丢监控面 |

## 目标架构

### 1. 新的任务推进语义

`terminal working -> checkpoint commit -> quality_pending -> quality_passed / quality_failed -> (通过后) next terminal / review / merge`

关键要求：
- terminal 发出 checkpoint commit 后，workflow 不能立即推进到下一个 terminal。
- 若质量门失败，orchestrator 通过 PTY 注入结构化修复指令，把报告摘要与文件/行号回传给原 terminal。
- 若质量门通过，orchestrator 才把当前 checkpoint 升格为“终端完成”，然后再决定是否进入下一个 terminal、reviewer terminal 或 merge terminal。
- merge 前还要再跑一次 branch 级/仓库级完整质量门，防止 terminal 级只跑 changed scope 时漏网。

### 2. 质量门分层

| 层级 | 触发时机 | 目标 | 结果 |
|----|------|------|------|
| Terminal Gate | 每次 checkpoint commit | 快速阻断当前 terminal 产出的低级错误 | 回传原 terminal 修复，不允许 handoff |
| Task/Branch Gate | 任务最后一个 terminal 通过后 | 覆盖整个 task branch 的完整检查 | 决定能否进入 review/merge |
| Repo Gate | 合并主分支前 / GitHub Actions | 统一 Sonar 质量门、全量测试、构建、部署检查 | 决定能否 push / merge / 发布 |

### 3. 质量引擎组成

必须纳入首发：
- Rust: `cargo fmt --check`, `cargo clippy --workspace --all-targets --all-features`, `cargo check --workspace`, 受影响测试/最小回归
- Frontend: `pnpm --dir frontend lint`, `pnpm --dir frontend check`, `pnpm --dir frontend test:run`
- Repo: `pnpm generate-types:check`, `pnpm prepare-db:check`
- CI/Infra: `actionlint`, Dockerfile/compose 规则检查
- Secrets/Security: 复用并升级 `scripts/audit-security.sh`，统一输出到质量报告模型
- Sonar: 项目级分析、Quality Gate、issue 聚合、规则中心、PR/branch 分析

建议二期纳入：
- `semgrep`
- `gitleaks`
- `hadolint`
- `trivy` / 依赖漏洞扫描
- SARIF 聚合导入

### 4. 仓库内规则资产布局

规划新增目录：
- `quality/quality-gate.yaml`
- `quality/providers/`
- `quality/sonar/sonar-project.properties`
- `quality/sonar/profiles/`
- `quality/baselines/`
- `quality/reports/templates/`
- `quality/docs/`

规则治理原则：
- 远端 Sonar Quality Profile 不是唯一真相，必须定期导出到仓库备份。
- 本地 gate 配置优先面向“terminal 级快速反馈”，Sonar 优先面向“分支/仓库级统一门禁”。
- 所有豁免、阈值、路径排除必须有仓库内文件，不允许只存在于网页控制台。

## Phase 29 全量实施计划

### 29A. 架构与协议定型

- [x] P29-A01 编写质量门 ADR，明确 `checkpoint -> quality gate -> handoff` 的新状态语义。
- [x] P29-A02 设计新的 commit metadata 协议。
说明：质量门开启时，代码 terminal 改用 `status: checkpoint`；保留对历史 `completed` metadata 的兼容解析。
- [x] P29-A03 设计 `QualityRun`, `QualityIssue`, `QualityGateDecision`, `QualityProvider`, `QualitySummary` 数据模型。
- [x] P29-A04 设计仓库级 `quality/quality-gate.yaml` 结构。
- [x] P29-A05 设计本地 gate 与 Sonar gate 的先后顺序、超时、重试与降级策略。
- [x] P29-A06 设计“质量失败回流原 terminal”的系统 prompt 模板、终端停等契约与重提交流程。

### 29B. 数据库与领域模型

- [x] P29-B01 新增质量运行表：`quality_run`
- [x] P29-B02 新增质量问题表：`quality_issue`
- [ ] P29-B03 新增质量策略/配置快照表：`quality_policy_snapshot`
- [x] P29-B04 为 workflow/task/terminal 增加 quality 聚合字段或视图。
- [ ] P29-B05 设计旧 workflow 的兼容迁移与回填逻辑。
- [ ] P29-B06 增加数据清理/归档策略，避免历史质量报告无限膨胀。

### 29C. Orchestrator 核心改造

- [x] P29-C01 在 `git_watcher` 中新增 checkpoint metadata 解析与路由。
- [x] P29-C02 在 orchestrator 中引入 `quality_pending / quality_pass / quality_fail` 分支处理。
- [x] P29-C03 阻断 `handle_terminal_completed()` 直接推进逻辑，改为“先等 quality gate 决策”。
- [x] P29-C04 将“质量失败 -> 新建 fixer terminal”改成“质量失败 -> 回写原 terminal”。
- [x] P29-C05 保留 `ReviewCode / FixIssues`，但调整触发点为“质量门通过之后”。
- [x] P29-C06 改造 `prompt_watcher` 与 terminal completion 模板，禁止在质量门未通过前 handoff。
- [ ] P29-C07 增加 checkpoint 去重、重放、幂等、防乱序保护。
- [x] P29-C08 增加 quality timeout、provider down、scanner crash、partial report 的恢复机制。
- [x] P29-C09 增加 feature flag：`QUALITY_GATE_MODE=off|shadow|warn|enforce`
- [x] P29-C10 保证关闭质量门时，旧 workflow 语义完全不变。

### 29D. 质量执行引擎

- [x] P29-D01 新建 `quality` 服务模块，统一封装 provider 接口。
- [ ] P29-D02 实现 changed-files 采样与 analyzer 选择策略。
- [x] P29-D03 实现 Rust analyzer adapter。
- [x] P29-D04 实现 Frontend analyzer adapter。
- [x] P29-D05 实现 Repo/Infra analyzer adapter。
- [x] P29-D06 实现 Sonar provider adapter。
- [x] P29-D07 实现报告聚合器，把 stdout/stderr/SARIF/JSON 统一转成内部报告模型。
- [x] P29-D08 实现 severity 映射与 blocking/non-blocking 判定。
- [ ] P29-D09 实现 baseline 对比与“只阻断新问题”策略。
- [ ] P29-D10 实现手动重跑与自动重跑策略。

### 29E. SonarQube / SonarCloud 集成

- [x] P29-E01 选定首发模式：`SonarQube Server`、`SonarQube Community Build` 或 `SonarCloud`。
说明：实施前必须先核对 Rust 覆盖边界；若远端 Sonar 对 Rust 规则不足，本地 `clippy` 仍作为 Rust 阻断真相源。
- [x] P29-E02 新增仓库内 `sonar-project.properties` 与路径包含/排除策略。
- [x] P29-E03 建立 Sonar token / host / project key / org key 的环境变量规范。
- [x] P29-E04 打通 branch / PR / main 三类分析模式。
- [x] P29-E05 设计并实现 Quality Profile 导出备份机制，定期同步到 `quality/sonar/profiles/`。
- [ ] P29-E06 设计并实现 Sonar 规则变更的 code review 流程。
- [ ] P29-E07 实现外部问题/SARIF 导入方案，用于统一展示本地 analyzer 结果。
- [x] P29-E08 明确 Quality Gate 阈值。
说明：首发先拦 new code 的 blocker/critical/coverage/duplication，不直接用全量历史债务卡死仓库。
- [x] P29-E09 设计 Sonar 故障时的降级路径。
说明：`shadow/warn` 模式可放行但必须落日志；`enforce` 模式仅在配置明确允许时才降级到本地 gate 真相源。
- [ ] P29-E10 规范旧 `docs/developed/issues/sonarcloud-*.md` 的归档与迁移。

### 29F. 脚本与命令入口

- [x] P29-F01 统一根 `package.json` 的 `lint/check/test/quality` 命名。
- [ ] P29-F02 为 frontend 增加稳定的 `quality` 聚合命令。
- [x] P29-F03 新增 `scripts/quality/run-quality-gate.*`
- [ ] P29-F04 新增 `scripts/quality/run-terminal-gate.*`
- [ ] P29-F05 新增 `scripts/quality/run-branch-gate.*`
- [ ] P29-F06 新增 `scripts/quality/collect-report.*`
- [x] P29-F07 新增 `scripts/quality/sync-sonar-profile.*`
- [ ] P29-F08 把 `scripts/verify-baseline.sh` 与 `scripts/audit-security.sh` 并入统一质量入口。
- [x] P29-F09 为 Windows/PowerShell 与 Linux/macOS 都提供等价入口。
- [x] P29-F10 保证所有脚本都能在 orchestrator 调用链中被安全复用。

### 29G. API、事件总线与共享类型

- [x] P29-G01 新增 `quality.*` workflow event 类型。
建议事件：`quality.run.started`, `quality.run.updated`, `quality.run.completed`, `quality.gate.blocked`, `quality.gate.passed`
- [x] P29-G02 新增 REST API：查询 workflow/task/terminal 的最新质量报告、重跑质量门、获取详细 issue 列表。
- [x] P29-G03 扩展 shared DTO：`WorkflowQualityGateDto`, `QualityRunDto`, `QualityIssueDto`, `TerminalQualitySummaryDto`
- [ ] P29-G04 明确 execution process 是否要承载质量扫描进程；若承载，扩展 run reason 和前端标签映射。
- [x] P29-G05 保证 workflow event 与 execution process log 双写不冲突、可追踪。
- [x] P29-G06 为 API/WS 设计版本兼容层，避免前端部署顺序导致崩溃。

### 29H. 前端 Workflow / Debug / 日志 UI

- [x] P29-H01 在工作流详情页显示“当前质量门状态、最近一次报告、阻断原因、重跑入口”。
- [x] P29-H02 在 `WorkflowDebug` / `TerminalDebugView` 中增加质量摘要面板。
- [x] P29-H03 在 pipeline/board 视图中给 terminal 增加质量徽标、问题数和阻断态。
- [x] P29-H04 在伪终端界面可见“分析已开始 / 已完成 / 已回传 terminal”状态。
- [x] P29-H05 在日志面板复用 `tool` 内容或新增 `quality report` 视图，保证详细报告可展开阅读。
- [x] P29-H06 扩展 `wsStore` 的 payload normalize 与事件订阅。
- [x] P29-H07 保持 React Query + workflow events 的现有数据刷新模式，不额外引入新的全局 workflow store。
- [x] P29-H08 提供失败回流链路的时间线。
说明：用户要看到“checkpoint 提交 -> 质量失败 -> 回传终端 -> 再次提交 -> 通过”的全过程。
- [x] P29-H09 支持历史终端查看对应的最后一次质量报告。
- [ ] P29-H10 处理前后端版本不一致时的 UI 容错与 placeholder。

### 29I. GitHub Actions / 分支保护 / 合并策略

- [x] P29-I01 拆分现有 `baseline-check.yml`，形成基础检查、质量扫描、Docker 检查三层 workflow。
- [x] P29-I02 补齐前端 lint/check/test 到 GitHub Actions。
- [x] P29-I03 增加 Sonar 分析 job 与 Quality Gate 等待逻辑。
- [ ] P29-I04 为 PR、main、release 分别定义 required checks。
- [ ] P29-I05 将 GitHub workflow 结果回写到 orchestrator 可消费的状态源。
- [ ] P29-I06 明确“合并到主分支后主动 push 并等待 workflow 完成”的自动化流程如何与新质量门联动。
- [ ] P29-I07 定义失败自动回修策略。
说明：CI 失败要能回到对应 agent/terminal，而不是只在 GitHub 页面失败后人工处理。
- [ ] P29-I08 为私有仓库/自托管 runner 场景补齐 secret 管理与缓存策略。

### 29J. Docker / 部署 / 运维

- [x] P29-J01 决定 Sonar 服务部署方式：外部托管 or 内嵌 compose profile。
- [ ] P29-J02 若自托管，补齐 Docker Compose、持久化卷、初始化脚本、升级脚本。
- [ ] P29-J03 将 Sonar/quality env 写入 Docker 安装与更新流程。
- [ ] P29-J04 补齐启动顺序、健康检查、重试与 readiness。
- [ ] P29-J05 增加报告保留、日志轮转、数据库清理策略。
- [ ] P29-J06 为离线/受限网络环境设计 fallback 模式。
- [ ] P29-J07 为 Windows 开发环境和 Linux 部署环境分别验证。

### 29K. 文档与知识资产全量更新

- [x] P29-K01 更新 `README.md`
- [x] P29-K02 更新 `README.zh-CN.md`
- [x] P29-K03 更新 `docs/undeveloped/README.md`
- [x] P29-K04 新增/更新用户如何启用质量门、查看报告、处理失败的用户文档。
- [x] P29-K05 新增/更新运维文档：Sonar 部署、token 配置、profile 同步、故障排查、数据清理。
- [x] P29-K06 更新 `git-watcher` 相关文档，写清 checkpoint metadata 与质量门语义。
- [x] P29-K07 更新 orchestrator/manual/runbook，写清终端质量回流机制。
- [x] P29-K08 更新历史 SonarCloud 报告归档策略与命名规范。
- [x] P29-K09 写一份“质量门设计说明 + 故障降级矩阵 + 回滚方案”。
- [x] P29-K10 所有新增 env、脚本、CI job、UI 状态都必须在双语文档可查。

### 29L. 测试、灰度与发布

- [x] P29-L01 为 checkpoint -> quality gate -> original terminal fix loop 写端到端集成测试。
- [x] P29-L02 为 legacy workflow 写兼容回归测试。
- [x] P29-L03 为 WebSocket `quality.*` 事件写合同测试。
- [ ] P29-L04 为前端 `WorkflowDebug` / `Workflows` / logs 面板补齐质量门 UI 测试。
- [ ] P29-L05 为 Sonar 故障、token 错误、scanner timeout、部分结果、重复提交写异常场景测试。
- [ ] P29-L06 先上线 `shadow` 模式，记录一周基线。
- [ ] P29-L07 再切 `warn` 模式，验证终端回流体验。
- [ ] P29-L08 最后切 `enforce` 模式，并开启 required checks。
- [ ] P29-L09 写清发布/回滚清单与 on-call 操作步骤。

## Subagent 开发编排

### 并发原则

- 允许高并发，但不允许多个 agent 同时触碰同一簇强耦合文件。
- “同一个 agent 负责相关联内容”是硬约束：后续修复、测试补丁、回归问题也必须回到原 owner agent。
- 建议长期固定 12 个开发 agent + 4 个短时验证 agent。
- 超过 12 个长期 agent 的收益会迅速低于冲突成本；宁可多轮并行，也不要让 3 个 agent 同时改 `orchestrator/agent.rs`、`wsStore.ts`、`README.md` 这种中心文件。

### 长期 Agent 所有权矩阵

| Agent | 永久负责范围 | 典型文件写集 | 备注 |
|----|------|------|------|
| A01 | 编排协议与 checkpoint 语义 | `crates/services/src/services/orchestrator/**`, `crates/services/src/services/git_watcher.rs`, `crates/services/src/services/terminal/prompt_watcher.rs` | 所有质量门编排语义统一归 A01 |
| A02 | 质量执行引擎 | `crates/services/src/services/quality/**`, `scripts/quality/**` | provider、report 聚合、命令执行都归 A02 |
| A03 | 质量持久化 | `crates/db/migrations/*`, `crates/db/src/models/quality*.rs`, `crates/db/src/models/mod.rs` | 只负责 DB 与 model 层 |
| A04 | Server API / WS / DTO | `crates/server/src/routes/quality*.rs`, `crates/server/src/routes/workflow_events.rs`, `crates/server/src/routes/mod.rs`, `crates/server/src/routes/workflows_dto.rs`, `shared/**` | API 合同由 A04 统一收口 |
| A05 | 前端事件与状态 | `frontend/src/stores/wsStore.ts`, `frontend/src/stores/terminalStore.ts`, `frontend/src/hooks/useWorkflows.ts`, `frontend/src/hooks/useExecutionProcesses.ts` | 所有 `quality.*` 前端事件归一化都归 A05 |
| A06 | WorkflowDebug / Terminal UI | `frontend/src/pages/WorkflowDebug*.tsx`, `frontend/src/components/terminal/**`, `frontend/src/components/workflow/**` | 伪终端与 debug 体验统一归 A06 |
| A07 | Board / Pipeline / 报告视图 | `frontend/src/pages/Workflows.tsx`, `frontend/src/pages/Pipeline.tsx`, `frontend/src/components/board/**`, `frontend/src/components/pipeline/**`, `frontend/src/components/quality/**` | 工作流概览与报告卡片归 A07 |
| A08 | CI 与质量配置 | `.github/workflows/**`, `quality/sonar/**`, `quality/quality-gate.yaml`, `package.json`, `frontend/package.json`, `clippy.toml` | 规则与入口统一归 A08 |
| A09 | Docker / 部署集成 | `docker/**`, `scripts/docker/**` | Sonar 部署与安装更新只归 A09 |
| A10 | README / 入口文档 | `README.md`, `README.zh-CN.md`, `docs/undeveloped/current/TODO.md`, `docs/undeveloped/README.md` | 文档入口只归 A10 |
| A11 | 运维 / 用户手册 | `docs/developed/ops/**`, `docs/developed/misc/**`, `quality/docs/**` | runbook、manual、guide 只归 A11 |
| A12 | 集成测试与回归 | `crates/services/tests/**`, `crates/server/tests/**`, `frontend/src/**/*.test.*`, `tests/**` | 仅在 owner agent 提供稳定接口后介入 |

### 短时验证 Agent

| Agent | 任务 | 触发时机 |
|----|------|------|
| V01 | Terminal gate benchmark / timeout profiling | A01+A02 合并后 |
| V02 | Sonar profile sync / token / degraded mode 验证 | A08+A09 合并后 |
| V03 | UI 合同与事件回放验证 | A04+A05+A06+A07 合并后 |
| V04 | 文档一致性 / README / runbook walkthrough | A10+A11 完成后 |

### 推荐并行轮次

#### Round 1：基础骨架

- A01 编排协议
- A02 质量执行引擎
- A03 数据库模型
- A08 CI 与规则配置
- A10 README / TODO / 文档入口

#### Round 2：服务与契约

- A04 API / WS / DTO
- A05 前端事件与状态
- A09 Docker / 部署
- A11 运维与用户手册

#### Round 3：UI 与体验

- A06 WorkflowDebug / Terminal UI
- A07 Board / Pipeline / 报告视图
- A12 测试基建补齐

#### Round 4：验证与收敛

- V01 benchmark
- V02 Sonar/配置验证
- V03 UI 合同回放
- V04 文档 walkthrough

## 文件级改动清单

### 必改代码 / 配置

- `.github/workflows/baseline-check.yml`
- `package.json`
- `frontend/package.json`
- `crates/services/src/services/orchestrator/agent.rs`
- `crates/services/src/services/orchestrator/state.rs`
- `crates/services/src/services/orchestrator/types.rs`
- `crates/services/src/services/git_watcher.rs`
- `crates/services/src/services/terminal/prompt_watcher.rs`
- `crates/server/src/routes/workflow_events.rs`
- `frontend/src/stores/wsStore.ts`
- `frontend/src/pages/Workflows.tsx`
- `frontend/src/pages/WorkflowDebugPage.tsx`
- `frontend/src/components/terminal/TerminalDebugView.tsx`

### 高概率新增

- `crates/services/src/services/quality/`
- `crates/server/src/routes/quality.rs`
- `crates/db/src/models/quality_run.rs`
- `crates/db/src/models/quality_issue.rs`
- `quality/quality-gate.yaml`
- `quality/sonar/sonar-project.properties`
- `quality/sonar/profiles/`
- `scripts/quality/`
- `frontend/src/components/quality/`
- `frontend/src/hooks/useWorkflowQualityReports.ts`

### 必改文档

- `README.md`
- `README.zh-CN.md`
- `docs/undeveloped/README.md`
- `docs/developed/ops/docker-deployment.md`
- `docs/developed/ops/runbook.md`
- `docs/developed/ops/troubleshooting.md`
- `docs/developed/misc/OPERATIONS_MANUAL.md`
- `docs/developed/misc/USER_GUIDE.md`
- `docs/developed/misc/git-watcher-usage.md`

## 验收标准

- [ ] Terminal checkpoint 提交后，不会直接推进到下一个 terminal。
- [ ] 质量门失败时，问题会以结构化摘要回到原 terminal，并在 UI 上可见。
- [ ] 原 terminal 修复后可再次 checkpoint，直到质量门通过。
- [ ] 质量门通过后，才允许进入下一 terminal / review / merge。
- [ ] Workflow events 与 execution process logs 都能看到质量运行过程。
- [ ] GitHub Actions 有前端 + 后端 + Sonar 的统一 required checks。
- [ ] Sonar 规则/质量配置可在仓库中 versioned 与 review。
- [ ] README / README.zh-CN / runbook / troubleshooting / user guide 全部更新。
- [ ] `QUALITY_GATE_MODE=off` 时，旧流程完全兼容。
- [ ] `shadow -> warn -> enforce` 三阶段 rollout 可验证、可回滚。

## 风险与应对

- 风险：Sonar 对 Rust 的规则覆盖或许可模式不满足预期。
应对：本地 `clippy` 继续作为 Rust 阻断真相源，Sonar 先承担聚合与趋势展示。
- 风险：把质量状态混进 `terminal.status` 会污染 PTY 生命周期。
应对：单独建 `quality_run/quality_state` 模型，并在 UI 做聚合显示。
- 风险：质量门太慢导致 terminal 卡住体验差。
应对：terminal gate 只跑 changed scope；branch/repo gate 再跑 full scope。
- 风险：多个 agent 同改核心文件导致冲突。
应对：执行上文 owner matrix，核心文件只能单 owner。
- 风险：Sonar 故障导致整个 workflow 阻塞。
应对：通过 feature flag 和 degrade matrix 明确 `shadow/warn/enforce` 行为。
- 风险：旧文档/旧手工 Sonar 报告继续误导使用者。
应对：统一归档旧报告，README 与运维文档明确新入口。

## 参考依据

- Sonar 官方文档已确认存在以下能力：
- 语言支持与 Rust 文档
- GitHub Actions / CI 分析集成
- 外部问题 / SARIF 导入
- Quality Profile 备份与恢复
- 实施时必须再次对照官方当前文档，确认版本、许可与参数名没有变化

## 文档入口

- 已完成任务清单：`docs/developed/misc/TODO-completed.md`
- 历史全量快照（只读）：`docs/developed/misc/TODO-legacy-full-2026-02-23.md`
- 历史执行看板归档：`docs/developed/misc/TODO-pending-archived-2026-03-11.md`
- Phase 28 计划文档：`docs/developed/plans/2026-03-11-phase-28-orchestrator-evolution.md`

## 维护规则

1. 新完成事项从“当前未完成计划”移动到 `docs/developed/misc/TODO-completed.md`，并同步更新时间。
2. 本文件保留当前最高优先级 Epic、未完成计划、风险、owner matrix 和验收标准。
3. 里程碑完成后，把稳定沉淀内容归档到 `docs/developed/`，避免 `docs/undeveloped/current/` 膨胀。
4. 若 Phase 29 后续继续拆 Phase 30+，必须继续在本文件保留总入口，不再回退到多个平行 TODO。

## 既有 Backlog（保留）

| ID | 描述 | 优先级 |
|----|------|--------|
| BACKLOG-001 | Docker Deployment 抽象 | 低 |
| BACKLOG-002 | Runner 容器分离 | 低 |
| BACKLOG-003 | CLI 安装状态 API | 中 |
| BACKLOG-004 | K8s 部署支持 | 低 |
| BACKLOG-005 | 镜像体积优化 | 中 |
