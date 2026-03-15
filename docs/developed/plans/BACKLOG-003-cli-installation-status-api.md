# BACKLOG-003: CLI 安装状态 API 增强 — 详细实施计划

> 创建时间：2026-03-15
> 优先级：中
> 预计影响文件：~15 个文件（新建 + 修改）

## 1. 目标

增强现有 CLI 检测和安装系统，提供：

- **单独安装/卸载**：按 CLI 粒度独立安装，而非一次性全量安装
- **实时安装进度**：通过 WebSocket 推送安装日志和进度
- **安装状态持久化**：记录安装历史，避免重复检测
- **自动健康监控**：后台定期检测 CLI 可用性变化，主动通知前端

## 2. 现状分析

### 已实现的能力

| 能力 | 端点 / 组件 | 状态 |
|------|------------|------|
| 批量检测所有 CLI | `GET /api/cli_types/detect` → `CliDetector::detect_all()` | ✅ 完整 |
| 单 Agent 可用性检查 | `GET /api/config/agents/check-availability` | ✅ 完整 |
| 全量安装脚本 | `POST /api/config/agents/install-ai-clis` | ✅ 但只有全量 |
| 前端检测展示 | `useCliDetection` hook + `AgentAvailabilityIndicator` | ✅ 完整 |
| 前端可用性状态 | `useAgentAvailability` hook | ✅ 完整 |

### 缺失的能力

| 能力 | 说明 |
|------|------|
| 单独安装某个 CLI | 目前只有 `install-ai-clis.sh` 全量安装脚本 |
| 安装进度实时推送 | 当前 POST 30 分钟超时同步返回，前端无中间状态 |
| 安装/卸载历史 | 无持久化记录，每次都要重新检测 |
| CLI 健康监控 | 无后台监控，仅用户触发时检测 |
| 安装锁 | 无并发安装保护 |

## 3. 详细设计

### Phase A: 单独安装/卸载 API

**目标**：支持按 CLI 粒度独立安装和卸载。

**新建文件**：
- `scripts/docker/install/install-single-cli.sh` — 单 CLI 安装脚本

**修改文件**：
- `crates/server/src/routes/cli_types.rs` — 添加安装/卸载端点
- `crates/services/src/services/terminal/detector.rs` — 添加单 CLI 安装逻辑
- `crates/db/migrations/` — 新迁移：cli_install_history 表

**API 设计**：

```
POST /api/cli_types/{cli_type_id}/install
  → 触发单个 CLI 安装，返回 install_job_id

DELETE /api/cli_types/{cli_type_id}/install
  → 卸载指定 CLI

GET /api/cli_types/{cli_type_id}/install/status
  → 查询安装任务状态
```

**数据库新增表**：

```sql
CREATE TABLE cli_install_history (
    id TEXT PRIMARY KEY,
    cli_type_id TEXT NOT NULL REFERENCES cli_type(id),
    action TEXT NOT NULL CHECK(action IN ('install', 'uninstall')),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK(status IN ('pending', 'running', 'success', 'failed')),
    started_at TEXT NOT NULL,
    completed_at TEXT,
    exit_code INTEGER,
    output TEXT,
    error_message TEXT,
    FOREIGN KEY (cli_type_id) REFERENCES cli_type(id)
);
```

**安装脚本设计**：
- `install-single-cli.sh <cli_name>` — 接受 CLI 名称参数
- 复用现有 `install-ai-clis.sh` 中的单个 CLI 安装逻辑，提取为函数
- 卸载：`npm uninstall -g <package>` 或 `gh extension remove` (Copilot)

**实施步骤**：

1. 创建 `cli_install_history` 数据库迁移
2. 在 `detector.rs` 中添加 `CliInstaller` struct：
   ```rust
   impl CliInstaller {
       pub async fn install_cli(&self, cli_type: &CliType) -> anyhow::Result<InstallJob>;
       pub async fn uninstall_cli(&self, cli_type: &CliType) -> anyhow::Result<InstallJob>;
   }
   ```
3. 添加安装锁（per-CLI `tokio::sync::Mutex`）防止并发安装同一 CLI
4. 创建 `install-single-cli.sh` 脚本
5. 在 `cli_types.rs` 中添加 3 个新 API 端点

---

### Phase B: WebSocket 实时安装进度

**目标**：安装过程中实时推送日志到前端。

**修改文件**：
- `crates/server/src/routes/cli_types.rs` — 添加 WebSocket 端点
- `crates/services/src/services/terminal/detector.rs` — 安装输出流式传递
- `frontend/src/hooks/useCliTypes.ts` — 添加安装进度 hook
- `frontend/src/components/CliInstallProgress.tsx` — 新建安装进度组件

**WebSocket 端点**：

```
WS /api/cli_types/{cli_type_id}/install/ws?job_id={id}
```

**消息格式**：

```typescript
// Server → Client
type InstallMessage =
  | { type: "log"; line: string; timestamp: number }
  | { type: "progress"; stage: string; percent: number }
  | { type: "completed"; success: boolean; exit_code: number }
  | { type: "error"; message: string }
```

**实施步骤**：

1. 安装进程输出通过 `tokio::io::BufReader` 逐行读取
2. 每行通过 `tokio::sync::broadcast` channel 发送
3. WebSocket handler 订阅 channel 并推送到前端
4. 前端 `useCliInstallProgress` hook 管理 WebSocket 连接
5. `CliInstallProgress` 组件渲染实时日志终端

---

### Phase C: 安装状态缓存与持久化

**目标**：减少重复检测开销，提供安装历史记录。

**修改文件**：
- `crates/services/src/services/terminal/detector.rs` — 添加缓存层
- `crates/server/src/routes/cli_types.rs` — 添加历史查询端点
- `crates/db/src/models/cli_type.rs` — 添加 CliInstallHistory model

**API 新增**：

```
GET /api/cli_types/{cli_type_id}/install/history
  → 返回安装/卸载历史记录

GET /api/cli_types/status/cached
  → 返回缓存的检测结果（不重新执行检测命令）
```

**缓存策略**：
- 内存缓存：`Arc<RwLock<HashMap<String, CachedDetectionResult>>>`
- TTL：5 分钟（与前端 React Query stale time 对齐）
- 安装/卸载操作后立即失效对应 CLI 的缓存
- 手动刷新：`POST /api/cli_types/detect/refresh` 强制重新检测

**实施步骤**：

1. 添加 `CliInstallHistory` model（CRUD）
2. `CliDetector` 添加 `DetectionCache` 内层
3. `detect_all()` 优先返回缓存，过期则重新检测
4. 安装/卸载完成后调用 `cache.invalidate(cli_type_id)`
5. 新增历史查询端点

---

### Phase D: 后台健康监控

**目标**：后台定期检测 CLI 状态变化，主动通知前端。

**修改文件**：
- `crates/services/src/services/terminal/detector.rs` — 添加后台监控任务
- `crates/local-deployment/src/lib.rs` — 启动监控任务
- `crates/server/src/routes/cli_types.rs` — SSE 端点推送状态变化
- `frontend/src/hooks/useCliTypes.ts` — 订阅 SSE 事件

**SSE 端点**：

```
GET /api/cli_types/status/stream
  → Server-Sent Events 流，推送 CLI 状态变化
```

**事件格式**：

```
event: cli_status_change
data: {"cli_type_id": "cli-claude-code", "previous": "installed", "current": "not_found", "detected_at": "..."}
```

**实施步骤**：

1. `CliHealthMonitor` struct，包含 `tokio::time::interval` (默认 5 分钟)
2. 每轮执行 `detect_all()`，与缓存比较差异
3. 差异通过 `tokio::sync::broadcast` 推送
4. SSE handler 订阅 broadcast 推送到前端
5. 前端 `useCliStatusStream` hook 接收 SSE 事件，自动更新 React Query 缓存
6. 环境变量 `GITCORTEX_CLI_HEALTH_INTERVAL_SECS` 控制检测间隔（默认 300）

---

### Phase E: 前端集成

**目标**：统一 CLI 管理界面。

**新建文件**：
- `frontend/src/components/CliManagementPanel.tsx` — CLI 管理面板
- `frontend/src/hooks/useCliInstall.ts` — 安装操作 hook
- `frontend/src/hooks/useCliStatusStream.ts` — SSE 状态流 hook

**修改文件**：
- `frontend/src/components/AgentAvailabilityIndicator.tsx` — 添加安装/卸载按钮
- `frontend/src/hooks/useCliTypes.ts` — 集成缓存 API

**界面设计**：

```
┌──────────────────────────────────────────────────────┐
│  CLI Management                                       │
├──────────────────────────────────────────────────────┤
│  ┌─────────────┬──────────┬─────────┬──────────────┐ │
│  │ CLI         │ Status   │ Version │ Action       │ │
│  ├─────────────┼──────────┼─────────┼──────────────┤ │
│  │ Claude Code │ ✅ Ready │ 1.2.3   │ [Uninstall]  │ │
│  │ Codex       │ ⚠ Found  │ 0.9.1   │ [Uninstall]  │ │
│  │ Gemini CLI  │ ❌ None  │ —       │ [Install]    │ │
│  │ Amp         │ 🔄 Installing... (45%)            │ │
│  │             │ > npm install -g @sourcegraph/amp  │ │
│  │             │ > Downloading...                   │ │
│  └─────────────┴──────────┴─────────┴──────────────┘ │
│  [Install All] [Refresh Detection]                    │
└──────────────────────────────────────────────────────┘
```

**实施步骤**：

1. 创建 `useCliInstall` hook（调用 install/uninstall API + WebSocket 进度）
2. 创建 `useCliStatusStream` hook（SSE 连接 + React Query 缓存同步）
3. 创建 `CliManagementPanel` 组件（表格 + 安装进度内嵌终端）
4. 在 `AgentAvailabilityIndicator` 中添加快捷安装按钮
5. 将 `CliManagementPanel` 集成到设置页面

## 4. 实施顺序与依赖

```
Phase A (单独安装/卸载 API)
    ↓
Phase B (WebSocket 实时进度)  ←─ 依赖 Phase A
    ↓
Phase C (缓存与持久化)        ←─ 可与 Phase B 并行
    ↓
Phase D (后台健康监控)        ←─ 依赖 Phase C 的缓存层
    ↓
Phase E (前端集成)           ←─ 依赖 Phase A-D
```

## 5. 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| npm install 需要网络且耗时 | 超时设置 10 分钟；进度推送让用户知道状态 |
| 安装脚本需要 root/sudo | Docker 容器内以 gitcortex 用户运行，npm global 使用 `--prefix` |
| CLI 版本不兼容 | 安装时记录版本号，卸载时精确匹配 |
| SSE 连接过多 | 所有 CLI 状态共享一个 SSE 流，不按 CLI 分连接 |
| 并发安装冲突 | per-CLI Mutex 锁 + 前端 disable 已锁定 CLI 的安装按钮 |
