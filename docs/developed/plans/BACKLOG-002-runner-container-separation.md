# BACKLOG-002: Runner 容器分离 — 详细实施计划

> 创建时间：2026-03-15
> 优先级：中
> 预计影响文件：~20 个文件（新建 + 修改）

## 1. 目标

将当前单容器架构拆分为 **Server 容器**（API / 编排 / 前端）和 **Runner 容器**（PTY 终端执行），实现：

- 终端进程与服务器进程隔离，Runner 崩溃不影响 Server
- Runner 可独立横向扩展（多 Runner 实例并行执行）
- 安全边界清晰：Runner 仅拥有 workspace 访问权限，不暴露 API 端口

## 2. 现状分析

### 当前架构（单进程）

```
┌─────────────────────────────────────────┐
│              gitcortex 容器              │
│  ┌──────────────┐  ┌──────────────────┐ │
│  │ Axum Server  │  │ ProcessManager   │ │
│  │ (API/WS/前端)│  │ (PTY 终端)       │ │
│  │              │──│ TerminalBridge   │ │
│  │ Orchestrator │  │ PromptWatcher    │ │
│  │ MessageBus   │  │ CCSwitchService  │ │
│  └──────────────┘  └──────────────────┘ │
│         ↕ Arc<MessageBus> (内存)         │
└─────────────────────────────────────────┘
```

### 关键耦合点

| 组件 | 耦合方式 | 分离难度 |
|------|---------|---------|
| MessageBus | Arc\<MessageBus\> 内存共享 | 高 — 需替换为网络 IPC |
| ProcessManager | Arc 直接引用 | 中 — Runner 独立持有 |
| TerminalBridge | 订阅 MessageBus topic | 中 — 跟随 MessageBus 迁移 |
| PromptWatcher | 读取 ProcessManager 输出 | 低 — 跟随 ProcessManager |
| CCSwitchService | 读数据库 | 低 — Runner 独立连接 DB |
| WebSocket (terminal_ws) | 直接读 ProcessManager handle | 高 — 需代理到 Runner |

## 3. 目标架构

```
┌────────────────────────┐         ┌────────────────────────┐
│     Server 容器         │         │     Runner 容器         │
│  ┌──────────────────┐  │  gRPC   │  ┌──────────────────┐  │
│  │ Axum Server      │◄─┼────────►┼──│ Runner Service   │  │
│  │ (API/WS/前端)    │  │         │  │ ProcessManager   │  │
│  │                  │  │  Redis  │  │ TerminalBridge   │  │
│  │ Orchestrator     │◄─┼────────►┼──│ PromptWatcher    │  │
│  │                  │  │ PubSub  │  │ CCSwitchService  │  │
│  └──────────────────┘  │         │  └──────────────────┘  │
│         ↕              │         │         ↕              │
│    SQLite (主库)        │         │    workspace mount     │
└────────────────────────┘         └────────────────────────┘
```

## 4. 分阶段实施

### Phase A: MessageBus 网络化（核心前置）

**目标**：将 MessageBus 从内存 broadcast 替换为 Redis PubSub，保持现有 API 不变。

**修改文件**：
- `crates/services/src/services/orchestrator/message_bus.rs` — 添加 `NetworkMessageBus` 实现
- `crates/services/Cargo.toml` — 添加 `redis` 依赖
- `crates/local-deployment/src/lib.rs` — MessageBus 初始化逻辑切换

**实施步骤**：

1. **定义 MessageBus trait**
   ```rust
   #[async_trait]
   pub trait MessageBusBackend: Send + Sync + 'static {
       async fn publish(&self, topic: &str, message: &BusMessage) -> anyhow::Result<()>;
       async fn subscribe(&self, topic: &str) -> anyhow::Result<BusReceiver>;
       async fn broadcast(&self, message: &BusMessage) -> anyhow::Result<()>;
       async fn subscribe_broadcast(&self) -> anyhow::Result<BroadcastReceiver>;
   }
   ```

2. **实现 `InMemoryBus`**（重命名现有实现）
   - 保持完全兼容现有行为
   - 用于单容器模式和测试

3. **实现 `RedisBus`**
   - topic 消息 → Redis PubSub channel `gitcortex:topic:{name}`
   - broadcast 消息 → Redis PubSub channel `gitcortex:broadcast`
   - BusMessage 序列化用 serde_json（已实现 Serialize/Deserialize）

4. **环境变量控制**
   - `GITCORTEX_MESSAGE_BUS=memory` (默认) | `redis`
   - `GITCORTEX_REDIS_URL=redis://localhost:6379`

**验证**：现有 642 个后端测试全部通过，MessageBus 行为无变化。

---

### Phase B: Runner gRPC 服务

**目标**：将终端管理能力封装为 gRPC 服务，Runner 独立运行。

**新建文件**：
- `crates/runner/Cargo.toml` — 新 crate
- `crates/runner/src/lib.rs` — Runner 服务主体
- `crates/runner/src/grpc.rs` — gRPC 服务定义
- `crates/runner/src/main.rs` — Runner 二进制入口
- `proto/runner.proto` — gRPC 协议定义

**修改文件**：
- `Cargo.toml` — workspace members 添加 runner
- `crates/server/Cargo.toml` — 添加 gRPC client 依赖

**gRPC 接口设计**：

```protobuf
syntax = "proto3";
package gitcortex.runner;

service RunnerService {
  // 终端生命周期
  rpc SpawnTerminal(SpawnTerminalRequest) returns (SpawnTerminalResponse);
  rpc KillTerminal(KillTerminalRequest) returns (KillTerminalResponse);
  rpc IsRunning(IsRunningRequest) returns (IsRunningResponse);
  rpc ResizeTerminal(ResizeRequest) returns (ResizeResponse);

  // 终端 I/O（双向流）
  rpc TerminalStream(stream TerminalInput) returns (stream TerminalOutput);

  // Runner 健康
  rpc Health(HealthRequest) returns (HealthResponse);
}

message SpawnTerminalRequest {
  string terminal_id = 1;
  string command = 2;
  repeated string args = 3;
  string working_dir = 4;
  map<string, string> env_set = 5;
  repeated string env_unset = 6;
  uint32 cols = 7;
  uint32 rows = 8;
}
```

**实施步骤**：

1. 创建 `crates/runner/` crate，依赖 `services` crate（复用 ProcessManager, CCSwitchService 等）
2. 实现 gRPC server，将 `SpawnTerminalRequest` 转换为 `SpawnCommand` 调用 `ProcessManager`
3. `TerminalStream` 双向流：客户端发送输入，服务端推送 PTY 输出
4. 在 Server 端实现 gRPC client wrapper，实现与本地 ProcessManager 相同的接口
5. `terminal_ws.rs` WebSocket handler 通过 gRPC stream 代理到 Runner

**环境变量**：
- `GITCORTEX_RUNNER_MODE=local` (默认，进程内) | `remote`
- `GITCORTEX_RUNNER_ADDR=http://runner:50051`

---

### Phase C: Server 端适配

**目标**：Server 透明切换本地/远程 Runner。

**修改文件**：
- `crates/server/src/routes/terminals.rs` — 终端启动/停止走 Runner 抽象层
- `crates/server/src/routes/terminal_ws.rs` — WebSocket 代理到 Runner gRPC stream
- `crates/deployment/src/lib.rs` — Deployment trait 添加 runner client
- `crates/local-deployment/src/lib.rs` — 支持 local / remote Runner 初始化

**实施步骤**：

1. **定义 Runner trait**
   ```rust
   #[async_trait]
   pub trait RunnerClient: Send + Sync + 'static {
       async fn spawn_terminal(&self, config: SpawnCommand) -> anyhow::Result<SpawnResult>;
       async fn kill_terminal(&self, terminal_id: &str) -> anyhow::Result<()>;
       async fn is_running(&self, terminal_id: &str) -> anyhow::Result<bool>;
       async fn terminal_stream(&self, terminal_id: &str) -> anyhow::Result<TerminalStream>;
   }
   ```

2. **LocalRunner**：直接调用 ProcessManager（零开销，兼容现有行为）
3. **RemoteRunner**：gRPC client 调用远程 Runner 服务
4. 修改 `terminals.rs` 中 `start_terminal` handler，从直接调用 ProcessManager 改为通过 RunnerClient
5. 修改 `terminal_ws.rs`，通过 RunnerClient::terminal_stream() 获取 I/O 流

---

### Phase D: Docker 分离

**目标**：提供双容器 Docker Compose 配置。

**新建文件**：
- `docker/Dockerfile.server` — Server 专用镜像（无 AI CLI）
- `docker/Dockerfile.runner` — Runner 专用镜像（含 AI CLI + workspace）
- `docker/compose/docker-compose.split.yml` — 双容器编排
- `docker/compose/.env.split.example` — 配置模板

**实施步骤**：

1. **Server 镜像**：基于现有 Dockerfile，移除 AI CLI 安装步骤，仅包含 `gitcortex-server` 二进制
2. **Runner 镜像**：包含 `gitcortex-runner` 二进制 + 全部 AI CLI + git 工具
3. **Compose 配置**：
   ```yaml
   services:
     server:
       build: { dockerfile: docker/Dockerfile.server }
       ports: ["23456:23456"]
       environment:
         GITCORTEX_RUNNER_MODE: remote
         GITCORTEX_RUNNER_ADDR: http://runner:50051
         GITCORTEX_MESSAGE_BUS: redis
         GITCORTEX_REDIS_URL: redis://redis:6379
     runner:
       build: { dockerfile: docker/Dockerfile.runner }
       volumes: ["${HOST_WORKSPACE_ROOT}:/workspace"]
       environment:
         GITCORTEX_MESSAGE_BUS: redis
         GITCORTEX_REDIS_URL: redis://redis:6379
     redis:
       image: redis:7-alpine
   ```
4. 保留原有 `docker-compose.yml` 不变（单容器模式仍可用）

---

### Phase E: 测试与文档

**修改文件**：
- `crates/runner/tests/` — Runner gRPC 集成测试
- `crates/server/tests/` — Server 端远程 Runner 集成测试

**测试矩阵**：

| 场景 | MessageBus | Runner | 预期 |
|------|-----------|--------|------|
| 开发模式 | memory | local | 零配置，与现有行为一致 |
| 分离模式 | redis | remote | 双容器独立运行 |
| 混合模式 | redis | local | 调试用 |

## 5. 风险与缓解

| 风险 | 缓解措施 |
|------|---------|
| SQLite 不支持多进程并发写 | Runner 仅更新 terminal 状态；考虑迁移到 PostgreSQL 或用 Server 代理写操作 |
| WebSocket 延迟增加 | gRPC stream 使用 HTTP/2 多路复用，延迟可控 |
| Redis 单点故障 | 生产环境用 Redis Sentinel；开发环境保留 memory 模式 |
| 现有测试兼容性 | Phase A 完成后立即跑全量测试确认无回归 |

## 6. 实施顺序与依赖

```
Phase A (MessageBus 网络化)
    ↓
Phase B (Runner gRPC 服务) ←─ 依赖 Phase A
    ↓
Phase C (Server 端适配)    ←─ 依赖 Phase B
    ↓
Phase D (Docker 分离)      ←─ 依赖 Phase C
    ↓
Phase E (测试与文档)       ←─ 依赖 Phase D
```

各 Phase 完成后均可独立交付验证，不破坏现有单容器部署模式。
