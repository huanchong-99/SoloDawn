# GitCortex 未完成任务清单（完整规划版）

> **更新时间:** 2026-03-07  
> **当前主线:** 主 Agent 对话化（Web + 社交通道）与多终端编排稳定化  
> **目标层级:** 从“可跑通 MVP”推进到“可持续长时自动开发”

---

## 0. 现状基线（截至 2026-03-06）

| 能力 | 现状 | 备注 |
|---|---|---|
| 上层 Orchestrator 编排 | 已实现 | 支持运行时动作（创建任务/终端、启动、关闭、完成） |
| 双模式工作流（`diy` / `agent_planned`） | 已实现 | 后端校验与前端向导已接入 |
| 多任务并行 + 任务内串行 | 已实现（基础） | 并发上限与基础测试存在，长稳态仍需强化 |
| 工作区会话对话（Session Chat） | 已实现 | 面向执行代理（coding agent）的 follow-up 通道 |
| 主 Agent 直接对话入口 | 已实现（基础版） | Web 侧可直发主 Agent，对话回执含 `command_id/status` |
| 社交软件对接主 Agent | 未实现 | 仅有目标方向，暂无接入层与协议 |
| 端到端复杂任务稳定性 | 未完成 | 当前验证任务偏简单，复杂场景覆盖不足 |

---

## 1. 本轮核心目标（必须完成）

1. 在 GitCortex 内建立“**主 Agent 对话通道**”，用于直接驱动编排决策，而非仅依赖 Git 事件被动唤醒。  
2. 保留并明确“**工作区对话通道**”职责，仅负责执行代理跟进，避免职责混乱。  
3. 建立“**社交通道接入层**”，使外部对话可安全映射到主 Agent 指令。  
4. 建立端到端可观测与可回放机制，保证长时间自动运行时可诊断、可恢复、可审计。  
5. 形成可发布门禁：接口稳定、权限可控、失败可回滚、关键路径自动化测试通过。

---

## 2. 范围与非目标

### 2.1 In Scope（本次必须覆盖）

- 主 Agent 对话 API（Workflow 维度）。
- 主 Agent 对话历史持久化与状态同步。
- Web 前端新增“主 Agent”对话区或工作流级面板。
- 社交软件接入网关（至少 1 个渠道先落地，其他渠道共用同一抽象）。
- 指令白名单、权限边界、速率限制、审计日志。
- 核心路径自动化测试（后端集成 + 前端关键交互 + e2e 冒烟）。

### 2.2 Out of Scope（本次不做）

- 全自动产品经理能力（主 Agent 完全自主拆解项目边界）。
- 多组织复杂权限系统重构。
- 大规模多租户部署拓扑优化（K8s 多集群级别）。

---

## 3. 架构原则（强约束）

1. **双通道隔离**：  
   - `Orchestrator Chat`：面向“编排意图”。  
   - `Session Chat`：面向“执行细节”。  
2. **单一真源**：所有主 Agent 指令统一进入 Orchestrator Runtime，不允许旁路直接改任务状态。  
3. **幂等与可回放**：外部消息必须具备去重键（`source + external_message_id`）。  
4. **安全优先**：默认 deny，指令白名单 + RBAC + 速率限制 + 审计。  
5. **可降级**：外部接入失败不影响本地 Web 主通道。

---

## 4. 里程碑计划（完整）

| 里程碑 | 目标 | 关键交付 | 退出条件 |
|---|---|---|---|
| M1 | 主 Agent 对话契约落地 | API/DTO/状态机设计文档、OpenAPI、错误码表 | 评审通过，后端/前端/接入层统一签字 |
| M2 | 后端主通道实现 | `orchestrator chat` 路由、服务层、持久化、幂等去重 | 单测 + 集成测试通过，接口可用 |
| M3 | Web 主 Agent 对话 UI | Workflow 级主 Agent 面板、历史流、发送/重试/停止 | 可在 UI 触发编排动作并可见回执 |
| M4 | 社交通道接入（首个） | Connector 网关、签名校验、消息映射、回执桥接 | 外部消息可安全驱动同一编排链路 |
| M5 | 可观测与治理 | 指标、日志、审计、速率限制、权限控制 | 故障演练通过，审计链可追踪 |
| M6 | 稳定性与回归 | 长时运行测试、并发与恢复测试、故障注入 | 连续运行门禁达到发布阈值 |
| M7 | 发布与文档 | 操作手册、运维手册、FAQ、回滚方案 | 对外发布可执行，支持交付 |

---

## 5. 分模块任务拆解（执行清单）

### 5.1 Orchestrator 后端

- [x] ORCH-001 新增工作流级主 Agent 对话入口（`POST /api/workflows/:id/orchestrator/chat`）。
- [x] ORCH-002 新增对话历史查询与分页（`GET /api/workflows/:id/orchestrator/messages`）。（支持 `cursor/limit`）
- [x] ORCH-003 新增对话命令状态（queued/running/succeeded/failed/cancelled）。（基础版：提交回执）
- [x] ORCH-004 主 Agent 对话输入接入 `OrchestratorAgent` 事件循环（非 Git 事件触发）。
- [x] ORCH-005 指令白名单校验（create_task/create_terminal/start_terminal/...）。
- [x] ORCH-006 幂等去重（source/external_message_id）。
- [x] ORCH-007 失败回执标准化（错误码 + 重试建议 + 可读摘要）。（基础版：`command_id/status/error/retryable`）

### 5.2 数据与持久化

- [x] DATA-001 新增 `workflow_orchestrator_message` 表（入站/出站消息）。
- [x] DATA-002 新增 `workflow_orchestrator_command` 表（执行状态/耗时/错误）。
- [x] DATA-003 新增 `external_conversation_binding` 表（外部会话映射）。
- [ ] DATA-004 持久化恢复策略：服务重启后可恢复未完成编排命令。
- [ ] DATA-005 脱敏策略：API key、token、敏感 prompt 片段日志脱敏。

### 5.3 前端 Web（Workflow 级）

- [x] FE-001 工作流详情页新增“主 Agent”对话面板。
- [ ] FE-002 主通道消息流：用户指令、系统回执、执行摘要。
- [x] FE-003 状态呈现：排队中、执行中、失败、可重试。（基础版：提交回执状态）
- [x] FE-004 与现有 Session Chat 明确区分文案与视觉层级，避免误用。
- [ ] FE-005 在 `agent_planned` 模式默认展示主 Agent 面板入口。
- [ ] FE-006 关键交互测试（发送、失败重试、权限报错、断线恢复）。

### 5.4 社交接入层

- [ ] CHAT-001 设计统一 Connector 接口（Webhook In / Callback Out）。
- [ ] CHAT-002 实现首个渠道接入（待定：企业微信/Telegram/Discord 任选其一）。
- [ ] CHAT-003 外部消息签名校验、时戳校验、重放攻击防护。
- [ ] CHAT-004 外部会话到工作流映射（绑定、解绑、权限校验）。
- [ ] CHAT-005 外部回执模板（成功、失败、需确认、不可执行）。

### 5.5 治理与运维

- [ ] GOV-001 权限模型：谁可以给主 Agent 下达编排命令。
- [ ] GOV-002 速率限制：每 workflow / 每用户 / 每外部会话。
- [ ] GOV-003 审计日志：指令来源、执行动作、结果、操作者。
- [ ] GOV-004 熔断策略：连续失败阈值触发自动暂停与告警。
- [ ] GOV-005 回滚手册：功能开关 + 数据迁移回退步骤。

---

## 6. 接口与契约草案（第一版）

### 6.1 主 Agent 对话发送

- `POST /api/workflows/{workflow_id}/orchestrator/chat`
- 请求体（草案）：

```json
{
  "message": "请把任务拆成 3 个并行子任务并立即启动第一个终端",
  "source": "web",
  "externalMessageId": null,
  "metadata": {
    "operatorId": "user-123",
    "clientTs": "2026-03-06T20:00:00+08:00"
  }
}
```

- 响应体（草案）：返回 `command_id` 与当前状态。

### 6.2 主 Agent 对话历史

- `GET /api/workflows/{workflow_id}/orchestrator/messages?cursor=&limit=`
- 返回统一消息流（user/system/agent/tool-summary）。

### 6.3 社交通道入站

- `POST /api/integrations/chat/{provider}/events`
- 必须字段：`provider_message_id`、`conversation_id`、`sender_id`、`text`、`signature`。

---

## 7. 测试与验收计划（发布门禁）

### 7.1 自动化测试门禁

- [ ] 单元测试：主通道服务层、幂等、权限、限流逻辑。
- [ ] 集成测试：`chat -> orchestrator -> runtime action` 全链路。
- [ ] 前端测试：主 Agent 面板交互与错误态。
- [ ] e2e 测试：`agent_planned` 模式下从对话驱动到任务状态变更。
- [ ] 回归测试：现有 Session Chat 与 Workflow API 不回归。

### 7.2 长稳态验证门禁

- [ ] 8 小时持续运行无死锁/无内存异常增长。
- [ ] 并发 workflow 压测达到配置上限后行为可预期。
- [ ] 重启恢复后，未完成命令可继续或明确失败并可重试。
- [ ] 社交通道重复消息不会触发重复执行。

---

## 8. 风险与缓解

| 风险 | 影响 | 缓解策略 |
|---|---|---|
| 主/执行通道职责混淆 | 用户误操作，状态失真 | UI 强区分 + API 强校验 + 文案引导 |
| 外部消息重放/伪造 | 安全风险，重复执行 | 签名校验 + 去重键 + 时窗校验 |
| 指令过强导致误编排 | 任务污染 | 指令白名单 + 高危动作二次确认 |
| 长时运行状态漂移 | 难排查，易卡死 | 审计日志 + 结构化指标 + 自动恢复 |
| 兼容多渠道复杂度过高 | 交付延误 | 先做单渠道 MVP，抽象稳定后扩展 |

---

## 9. 发布策略

1. **灰度开关**：主 Agent 对话能力使用 feature flag，默认仅开发环境开启。  
2. **分阶段放量**：先 Web 内测，再开放首个社交通道，再扩大。  
3. **回滚优先**：出现编排异常可快速关闭新入口，保留原有工作流能力。  
4. **文档同步**：发布前更新 README、运维手册、故障排查手册。

---

## 10. Definition of Done（完成定义）

- [ ] Web 内可直接与主 Agent 对话并驱动编排动作。  
- [ ] 工作区 Session 对话不受影响，职责边界清晰。  
- [ ] 至少 1 个社交通道可稳定接入同一主编排链路。  
- [ ] 全链路可审计、可限流、可恢复、可回滚。  
- [ ] 自动化测试与长稳态门禁通过，发布文档齐全。

---

## 11. 低优先级保留项（原清单延续）

| 目标 | 描述 | 优先级 |
|---|---|---|
| DockerDeployment 抽象 | 新建 `crates/docker-deployment` 实现 `Deployment` trait，支持容器级隔离执行 | 低 |
| Runner 容器分离 | 控制面与执行面解耦，CLI 执行在独立 Runner 容器 | 低 |
| CLI 安装状态 API | `/api/cli_install` 查询/重试安装状态 | 中 |
| K8s 部署支持 | Helm chart、多副本、高可用 | 低 |
| 镜像体积优化 | 分层缓存、CLI 按需安装、distroless 基础镜像 | 中 |
