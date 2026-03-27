# Phase 19: Docker 部署实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 将 SoloDawn 从本机运行形态迁移为 Docker 可运行、可发布、可回滚的形态，容器内原生安装 AI CLI。

**Architecture:** MVP 单容器架构——Rust server + 嵌入前端 + Node.js/CLI 工具链运行在同一容器内。保留 `LocalDeployment` 不变，通过环境变量覆盖路径使其适配容器环境。不引入新的 `DockerDeployment` 抽象。

**Tech Stack:** Docker multi-stage build, docker-compose, Node.js 22, Rust nightly, SQLite, pnpm, bash

---

## 0. 核心约束与设计决策

### 0.1 硬约束

1. Docker 容器是隔离环境，**不可穿透到宿主机**调用 CLI。
2. 所有 AI CLI 必须在**容器构建时**安装并固化版本。
3. 迁移后**必须保留本地开发模式**（向后兼容）。
4. `LocalDeployment` 在容器内**直接复用**——容器内就是"本地"。

### 0.2 关键发现（基于代码审计）

| 发现 | 代码位置 | 影响 |
|---|---|---|
| 服务器默认绑定 `127.0.0.1` | `crates/server/src/main.rs:152` | Docker 端口映射不可达，**MVP 必改** |
| `/api/health` 仅返回 `"OK"`，无 DB/CLI 检查 | `crates/server/src/routes/health.rs:4` | 编排工具无法判断就绪状态 |
| `/api/health` 被 API token 中间件保护 | `crates/server/src/routes/mod.rs:72` | Docker 健康检查无法通过 |
| `asset_dir()` debug 用相对路径，release 用系统目录 | `crates/utils/src/assets.rs:6-23` | 容器内需环境变量覆盖 |
| `get_solodawn_temp_dir()` Linux 下用 `/var/tmp/solodawn` | `crates/utils/src/path.rs:102-119` | 需挂卷持久化。**注意：** `get_worktree_base_dir()` = `get_solodawn_temp_dir().join("worktrees")`，所以 env 设为 `/var/lib/solodawn` 即可，**不要**设为 `/var/lib/solodawn/worktrees`（否则双重嵌套） |
| Executor 大量走 `npx -y <pkg>@version` 而非全局命令 | `crates/executors/src/executors/claude.rs` 等 | 全局安装 CLI 是**补充**而非替代 |
| 前端通过 `rust_embed` 编译时嵌入 `frontend/dist` | `crates/server/src/routes/frontend.rs:10` | Docker 构建必须先产出前端 |
| Release 模式下自动打开浏览器 | `crates/server/src/main.rs:163-174` | 容器内无浏览器，需跳过 |

### 0.3 AI CLI 官方安装命令（2026 年 2 月核实）

| CLI | 安装命令 | 验证命令 | 运行时依赖 |
|---|---|---|---|
| Claude Code | `npm i -g @anthropic-ai/claude-code` | `claude --version` | Node.js 18+ |
| Codex CLI | `npm i -g @openai/codex` | `codex --version` | Node.js 20+ |
| Gemini CLI | `npm i -g @google/gemini-cli` | `gemini --version` | Node.js 20+ |
| Qwen Code | `npm i -g @qwen-code/qwen-code@latest` | `qwen --version` | Node.js 20+ |
| Amp | `npm i -g @sourcegraph/amp@latest` | `amp --version` | Node.js 18+ |
| OpenCode | `npm i -g opencode-ai@latest` | `opencode --version` | Node.js 22+ |
| Kilo CLI | `npm i -g @kilocode/cli` | `kilocode --version` | Node.js 18+ |
| GitHub Copilot | `gh extension install github/gh-copilot` | `gh copilot --version` | gh CLI |

---

## 实施任务清单

### Task 1: 路径函数环境变量覆盖

**Files:**
- Modify: `crates/utils/src/assets.rs:6-23`
- Modify: `crates/utils/src/path.rs:102-119`
- Test: `crates/utils/src/assets.rs` (inline tests)
- Test: `crates/utils/src/path.rs` (inline tests)

**Step 1: Write failing tests for `asset_dir` env override**

```rust
// In crates/utils/src/assets.rs, add to #[cfg(test)] mod tests
#[test]
fn test_asset_dir_env_override() {
    let test_dir = std::env::temp_dir().join("solodawn-test-assets");
    std::fs::create_dir_all(&test_dir).unwrap();
    unsafe { std::env::set_var("SOLODAWN_ASSET_DIR", &test_dir); }
    let result = asset_dir().unwrap();
    assert_eq!(result, test_dir);
    unsafe { std::env::remove_var("SOLODAWN_ASSET_DIR"); }
    std::fs::remove_dir_all(&test_dir).ok();
}
```

**Step 2: Run test to verify it fails**

Run: `cargo test -p utils -- test_asset_dir_env_override`
Expected: FAIL — env var not yet read

**Step 3: Implement env override in `asset_dir()`**

```rust
pub fn asset_dir() -> std::io::Result<std::path::PathBuf> {
    let path = if let Ok(override_dir) = std::env::var("SOLODAWN_ASSET_DIR") {
        std::path::PathBuf::from(override_dir)
    } else if cfg!(debug_assertions) {
        std::path::PathBuf::from(PROJECT_ROOT).join("../../dev_assets")
    } else {
        let dirs = ProjectDirs::from("ai", "bloop", "solodawn").ok_or_else(|| {
            std::io::Error::new(std::io::ErrorKind::NotFound, "OS didn't give us a home directory")
        })?;
        dirs.data_dir().to_path_buf()
    };
    std::fs::create_dir_all(&path)?;
    Ok(path)
}
```

**Step 4: Write failing test for `get_solodawn_temp_dir` env override**

```rust
// In crates/utils/src/path.rs, add to #[cfg(test)] mod tests
#[test]
fn test_temp_dir_env_override() {
    let test_dir = std::env::temp_dir().join("solodawn-test-tmp");
    unsafe { std::env::set_var("SOLODAWN_TEMP_DIR", &test_dir); }
    let result = get_solodawn_temp_dir();
    assert_eq!(result, test_dir);
    unsafe { std::env::remove_var("SOLODAWN_TEMP_DIR"); }
}
```

**Step 5: Implement env override in `get_solodawn_temp_dir()`**

```rust
pub fn get_solodawn_temp_dir() -> std::path::PathBuf {
    if let Ok(override_dir) = std::env::var("SOLODAWN_TEMP_DIR") {
        return std::path::PathBuf::from(override_dir);
    }
    // ... existing logic unchanged ...
}
```

**Step 6: Run all tests**

Run: `cargo test -p utils`
Expected: ALL PASS

**Step 7: Commit**

```bash
git add crates/utils/src/assets.rs crates/utils/src/path.rs
git commit -m "feat(utils): add SOLODAWN_ASSET_DIR and SOLODAWN_TEMP_DIR env overrides for Docker"
```

---

### Task 2: 健康检查路由（无鉴权）

**Files:**
- Modify: `crates/server/src/routes/health.rs`
- Modify: `crates/server/src/routes/mod.rs:42-80`
- Modify: `crates/server/src/main.rs:152` (HOST 绑定)

**Step 1: 增强 health.rs — 新增 `/healthz` 和 `/readyz`**

`/healthz` 是 liveness 探针（进程存活即可），`/readyz` 检查 DB 可连接 + 工作目录可写。

```rust
use axum::{extract::State, response::Json};
use serde_json::{json, Value};
use utils::response::ApiResponse;
use crate::DeploymentImpl;
use deployment::Deployment;

pub async fn health_check() -> Json<ApiResponse<String>> {
    Json(ApiResponse::success("OK".to_string()))
}

pub async fn healthz() -> Json<Value> {
    Json(json!({"status": "alive"}))
}

/// readyz 使用 State<DeploymentImpl> 而非 Extension<SqlitePool>，
/// 因为 router 使用 .with_state(deployment) 注入状态。
pub async fn readyz(
    State(deployment): State<DeploymentImpl>,
) -> (axum::http::StatusCode, Json<Value>) {
    let db_ok = sqlx::query("SELECT 1").fetch_one(&deployment.db().pool).await.is_ok();
    let asset_ok = utils::assets::asset_dir().map(|p| p.exists()).unwrap_or(false);
    let temp_ok = utils::path::get_solodawn_temp_dir().exists()
        || std::fs::create_dir_all(utils::path::get_solodawn_temp_dir()).is_ok();

    let all_ok = db_ok && asset_ok && temp_ok;
    let status = if all_ok {
        axum::http::StatusCode::OK
    } else {
        axum::http::StatusCode::SERVICE_UNAVAILABLE
    };

    (status, Json(json!({
        "ready": all_ok,
        "checks": { "database": db_ok, "asset_dir": asset_ok, "temp_dir": temp_ok }
    })))
}
```

**Step 2: 在 mod.rs 中注册无鉴权路由**

在 `router()` 函数中，将 `/healthz` 和 `/readyz` 放在 API token 中间件**之外**。
`/healthz` 无状态；`/readyz` 需要 `State<DeploymentImpl>` 所以必须在 `.with_state()` 之后：

```rust
// 在 Router::new() 最外层，token 中间件之前
Router::new()
    .route("/healthz", get(health::healthz))
    .route("/readyz", get(health::readyz))
    .route("/", get(frontend::serve_frontend_root))
    .route("/{*path}", get(frontend::serve_frontend))
    .nest("/api", base_routes)
    .with_state(deployment.clone())  // readyz 需要 State<DeploymentImpl>
    .into_make_service()
```

> **注意：** 原 `base_routes` 已有 `.with_state(deployment)`，外层再加一次是为了让 `/readyz` 也能提取 `State`。`/healthz` 不需要状态，不受影响。

**Step 3: 修改 HOST 默认值支持 Docker**

在 `crates/server/src/main.rs:152`，当检测到容器环境时默认绑定 `0.0.0.0`：

```rust
let host = std::env::var("HOST").unwrap_or_else(|_| {
    // 容器内默认绑定所有接口；本地开发默认 127.0.0.1
    if std::env::var("SOLODAWN_ASSET_DIR").is_ok() || std::path::Path::new("/.dockerenv").exists() {
        "0.0.0.0".to_string()
    } else {
        "127.0.0.1".to_string()
    }
});
```

**Step 4: 跳过容器内浏览器打开**

在 `crates/server/src/main.rs:163`，增加容器检测：

```rust
if !cfg!(debug_assertions) && !std::path::Path::new("/.dockerenv").exists() {
```

**Step 5: Run tests and verify compilation**

Run: `cargo check -p server`
Expected: PASS

**Step 6: Commit**

```bash
git add crates/server/src/routes/health.rs crates/server/src/routes/mod.rs crates/server/src/main.rs
git commit -m "feat(server): add /healthz /readyz probes, bind 0.0.0.0 in Docker, skip browser open"
```

---

### Task 3: 多阶段 Dockerfile

**Files:**
- Create: `docker/Dockerfile`
- Create: `.dockerignore` (仓库根目录，因为 build context 是 repo root)

**Step 1: 创建 `.dockerignore`（仓库根目录）**

```
target/
node_modules/
.git/
dev_assets/
.worktrees/
*.log
.tmp/
tmp/
```

**Step 2: 创建多阶段 Dockerfile**

```dockerfile
# ============================================================
# Stage 1: Frontend build
# ============================================================
FROM node:22-slim AS frontend-builder
WORKDIR /build
# pnpm-lock.yaml 和 pnpm-workspace.yaml 在仓库根目录
COPY pnpm-lock.yaml pnpm-workspace.yaml ./
COPY frontend/package.json frontend/
RUN corepack enable && cd frontend && pnpm install --frozen-lockfile
COPY frontend/ frontend/
COPY shared/ shared/
RUN cd frontend && pnpm build

# ============================================================
# Stage 2: Rust build
# ============================================================
FROM rust:nightly-slim AS rust-builder
RUN apt-get update && apt-get install -y \
    pkg-config libsqlite3-dev libgit2-dev zlib1g-dev \
    cmake ninja-build clang libclang-dev perl nasm libssl-dev \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /build
COPY Cargo.toml Cargo.lock rust-toolchain.toml ./
COPY crates/ crates/
COPY assets/ assets/
COPY shared/ shared/
COPY --from=frontend-builder /build/frontend/dist frontend/dist
COPY crates/db/.sqlx crates/db/.sqlx
ENV SQLX_OFFLINE=true
RUN cargo build --release -p server

# ============================================================
# Stage 3: Runtime
# ============================================================
FROM debian:bookworm-slim AS runtime

# System deps
RUN apt-get update && apt-get install -y \
    libsqlite3-0 libgit2-1.5 git curl ca-certificates bash \
    && rm -rf /var/lib/apt/lists/*

# Node.js 22 for CLI tools
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# gh CLI for GitHub Copilot
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
    | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" \
    | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update && apt-get install -y gh && rm -rf /var/lib/apt/lists/*

# Install AI CLIs (core set)
COPY scripts/docker/install/ /opt/solodawn/install/
RUN bash /opt/solodawn/install/install-ai-clis.sh

# Copy binary and assets
COPY --from=rust-builder /build/target/release/server /usr/local/bin/solodawn-server
COPY assets/scripts/ /opt/solodawn/assets/scripts/
COPY assets/sounds/ /opt/solodawn/assets/sounds/

# Non-root user for security
RUN groupadd -r solodawn && useradd -r -g solodawn -m solodawn

# Volumes — SOLODAWN_TEMP_DIR=/var/lib/solodawn（不是 /worktrees，因为
# get_worktree_base_dir() 会自动 .join("worktrees")）
RUN mkdir -p /var/lib/solodawn/assets /var/lib/solodawn \
    && chown -R solodawn:solodawn /var/lib/solodawn
VOLUME ["/var/lib/solodawn"]

# Environment
ENV SOLODAWN_ASSET_DIR=/var/lib/solodawn/assets \
    SOLODAWN_TEMP_DIR=/var/lib/solodawn \
    GH_EXTENSIONS_DIR=/opt/solodawn/gh-extensions \
    HOST=0.0.0.0 \
    PORT=23456 \
    RUST_LOG=info

EXPOSE 23456

# Entrypoint
COPY scripts/docker/entrypoint.sh /opt/solodawn/entrypoint.sh
RUN chmod +x /opt/solodawn/entrypoint.sh

# Switch to non-root user
USER solodawn

ENTRYPOINT ["/opt/solodawn/entrypoint.sh"]
CMD ["solodawn-server"]
```

**Step 3: Commit**

```bash
git add docker/Dockerfile .dockerignore
git commit -m "feat(docker): add multi-stage Dockerfile with frontend+rust+runtime"
```

---

### Task 4: CLI 安装脚本体系

**Files:**
- Create: `scripts/docker/install/lib/common.sh`
- Create: `scripts/docker/install/install-ai-clis.sh`
- Create: `scripts/docker/install/verify-all-clis.sh`

**Step 1: 创建公共函数库 `lib/common.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

log_info()  { echo "[INFO]  $(date +%H:%M:%S) $*"; }
log_warn()  { echo "[WARN]  $(date +%H:%M:%S) $*"; }
log_error() { echo "[ERROR] $(date +%H:%M:%S) $*" >&2; }

# Install npm package with retry
npm_install_global() {
    local pkg="$1"
    local max_retries="${2:-3}"
    local attempt=0
    while [ $attempt -lt $max_retries ]; do
        attempt=$((attempt + 1))
        log_info "Installing $pkg (attempt $attempt/$max_retries)..."
        if npm install -g "$pkg" 2>&1; then
            log_info "$pkg installed successfully"
            return 0
        fi
        log_warn "$pkg install attempt $attempt failed, retrying..."
        sleep 2
    done
    log_error "Failed to install $pkg after $max_retries attempts"
    return 1
}

# Verify a CLI is available
verify_cli() {
    local name="$1"
    local cmd="$2"
    if eval "$cmd" > /dev/null 2>&1; then
        local version
        version=$(eval "$cmd" 2>&1 | head -1)
        log_info "✅ $name: $version"
        echo "{\"name\":\"$name\",\"installed\":true,\"version\":\"$version\"}"
        return 0
    else
        log_warn "❌ $name: not available"
        echo "{\"name\":\"$name\",\"installed\":false}"
        return 1
    fi
}
```

**Step 2: 创建主安装入口 `install-ai-clis.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

log_info "=== SoloDawn AI CLI Installation ==="

# Core CLIs (must succeed)
CORE_CLIS=(
    "@anthropic-ai/claude-code"
    "@openai/codex"
    "@google/gemini-cli"
)

# Extended CLIs (best-effort)
EXTENDED_CLIS=(
    "@qwen-code/qwen-code@latest"
    "@sourcegraph/amp@latest"
    "opencode-ai@latest"
    "@kilocode/cli"
)

FAILED=0

log_info "--- Installing core CLIs ---"
for pkg in "${CORE_CLIS[@]}"; do
    npm_install_global "$pkg" 3 || FAILED=$((FAILED + 1))
done

log_info "--- Installing extended CLIs (best-effort) ---"
for pkg in "${EXTENDED_CLIS[@]}"; do
    npm_install_global "$pkg" 2 || log_warn "Skipping optional: $pkg"
done

# GitHub Copilot CLI (requires gh)
# gh extensions are user-scoped — install to shared dir so non-root user can access
if command -v gh > /dev/null 2>&1; then
    log_info "Installing GitHub Copilot CLI extension..."
    export GH_EXTENSIONS_DIR="/opt/solodawn/gh-extensions"
    mkdir -p "$GH_EXTENSIONS_DIR"
    gh extension install github/gh-copilot 2>&1 || log_warn "Skipping gh-copilot"
fi

log_info "=== Installation complete (failures: $FAILED) ==="

# Run verification
bash "$SCRIPT_DIR/verify-all-clis.sh"

exit $FAILED
```

**Step 3: 创建验证脚本 `verify-all-clis.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib/common.sh"

log_info "=== CLI Verification ==="

TOTAL=0
OK=0

check() {
    TOTAL=$((TOTAL + 1))
    if verify_cli "$1" "$2"; then
        OK=$((OK + 1))
    fi
}

check "Claude Code"  "claude --version"
check "Codex CLI"    "codex --version"
check "Gemini CLI"   "gemini --version"
check "Qwen Code"    "qwen --version"
check "Amp"          "amp --version"
check "OpenCode"     "opencode --version"
check "Kilo CLI"     "kilocode --version"
check "GH Copilot"   "gh copilot --version"

log_info "=== Verification complete: $OK/$TOTAL CLIs available ==="
```

**Step 4: Commit**

```bash
git add scripts/docker/install/
git commit -m "feat(docker): add CLI installation and verification scripts"
```

---

### Task 5: 容器启动入口脚本

**Files:**
- Create: `scripts/docker/entrypoint.sh`

**Step 1: 创建 entrypoint.sh**

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "=== SoloDawn Container Starting ==="

# Ensure data directories exist with correct permissions
mkdir -p "${SOLODAWN_ASSET_DIR:-/var/lib/solodawn/assets}"
mkdir -p "${SOLODAWN_TEMP_DIR:-/var/lib/solodawn}"

# Verify critical dependencies
if ! command -v git > /dev/null 2>&1; then
    echo "FATAL: git not found" >&2
    exit 1
fi

if ! command -v node > /dev/null 2>&1; then
    echo "FATAL: node not found" >&2
    exit 1
fi

echo "Node.js: $(node --version)"
echo "npm: $(npm --version)"
echo "git: $(git --version)"
echo "Asset dir: ${SOLODAWN_ASSET_DIR:-/var/lib/solodawn/assets}"
echo "Temp dir: ${SOLODAWN_TEMP_DIR:-/var/lib/solodawn}"

# Execute the main command
exec "$@"
```

**Step 2: Commit**

```bash
git add scripts/docker/entrypoint.sh
git commit -m "feat(docker): add container entrypoint with pre-flight checks"
```

---

### Task 6: Docker Compose 编排文件

**Files:**
- Create: `docker/compose/docker-compose.yml`
- Create: `docker/compose/docker-compose.dev.yml`
- Create: `docker/compose/.env.example`

**Step 1: 创建生产编排文件 `docker-compose.yml`**

```yaml
services:
  solodawn:
    build:
      context: ../..
      dockerfile: docker/Dockerfile
    ports:
      - "${PORT:-23456}:23456"
    volumes:
      - solodawn-data:/var/lib/solodawn
    environment:
      - SOLODAWN_ASSET_DIR=/var/lib/solodawn/assets
      - SOLODAWN_TEMP_DIR=/var/lib/solodawn
      - HOST=0.0.0.0
      - PORT=23456
      - RUST_LOG=${RUST_LOG:-info}
      - SOLODAWN_ENCRYPTION_KEY=${SOLODAWN_ENCRYPTION_KEY}
      - SOLODAWN_API_TOKEN=${SOLODAWN_API_TOKEN:-}
      # AI CLI API Keys (inject via .env)
      - ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY:-}
      - OPENAI_API_KEY=${OPENAI_API_KEY:-}
      - GOOGLE_API_KEY=${GOOGLE_API_KEY:-}
    healthcheck:
      test: ["CMD", "curl", "-sf", "http://localhost:23456/readyz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
    restart: unless-stopped

volumes:
  solodawn-data:
```

**Step 2: 创建 `.env.example`**

```env
# Required
SOLODAWN_ENCRYPTION_KEY=your-32-char-encryption-key-here

# Optional: API token for /api routes
SOLODAWN_API_TOKEN=

# AI CLI API Keys
ANTHROPIC_API_KEY=
OPENAI_API_KEY=
GOOGLE_API_KEY=

# Server
PORT=23456
RUST_LOG=info
```

**Step 3: 创建开发编排文件 `docker-compose.dev.yml`**

```yaml
services:
  solodawn:
    build:
      context: ../..
      dockerfile: docker/Dockerfile
    ports:
      - "23456:23456"
    volumes:
      - solodawn-dev-data:/var/lib/solodawn
    environment:
      - SOLODAWN_ASSET_DIR=/var/lib/solodawn/assets
      - SOLODAWN_TEMP_DIR=/var/lib/solodawn
      - HOST=0.0.0.0
      - PORT=23456
      - RUST_LOG=debug
      - SOLODAWN_ENCRYPTION_KEY=12345678901234567890123456789012
    env_file:
      - .env

volumes:
  solodawn-dev-data:
```

**Step 4: Commit**

```bash
git add docker/compose/
git commit -m "feat(docker): add docker-compose production and dev configs"
```

---

### Task 7: CI/CD — GitHub Actions Docker 构建

**Files:**
- Modify: `.github/workflows/baseline-check.yml`

**Step 1: 在现有 workflow 中增加 Docker build 验证 job**

```yaml
  docker-build:
    name: Docker Build Check
    runs-on: ubuntu-latest
    needs: check
    steps:
    - uses: actions/checkout@v4
    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v3
    - name: Build Docker image
      uses: docker/build-push-action@v6
      with:
        context: .
        file: docker/Dockerfile
        push: false
        tags: solodawn:ci-test
        cache-from: type=gha
        cache-to: type=gha,mode=max
```

**Step 2: Commit**

```bash
git add .github/workflows/baseline-check.yml
git commit -m "ci: add Docker build verification to baseline check"
```

---

### Task 8: E2E 冒烟测试脚本

**Files:**
- Create: `scripts/docker/e2e-smoke.sh`

**Step 1: 创建冒烟测试**

```bash
#!/usr/bin/env bash
set -euo pipefail

echo "=== SoloDawn Docker E2E Smoke Test ==="

COMPOSE_FILE="docker/compose/docker-compose.dev.yml"
SERVICE="solodawn"
BASE_URL="http://localhost:23456"
MAX_WAIT=120

# Build and start
echo "Building and starting container..."
docker compose -f "$COMPOSE_FILE" up -d --build

# Wait for healthz
echo "Waiting for service to be healthy (max ${MAX_WAIT}s)..."
elapsed=0
while [ $elapsed -lt $MAX_WAIT ]; do
    if curl -sf "$BASE_URL/healthz" > /dev/null 2>&1; then
        echo "✅ /healthz OK after ${elapsed}s"
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
done

if [ $elapsed -ge $MAX_WAIT ]; then
    echo "❌ Timeout waiting for /healthz"
    docker compose -f "$COMPOSE_FILE" logs
    docker compose -f "$COMPOSE_FILE" down
    exit 1
fi

# Test readyz — must assert ready=true
echo "Checking /readyz..."
READYZ=$(curl -s "$BASE_URL/readyz" || true)
echo "readyz response: $READYZ"
READY_VAL=$(echo "$READYZ" | grep -o '"ready":true' || true)
if [ -z "$READY_VAL" ]; then
    echo "❌ /readyz did not return ready:true"
    docker compose -f "$COMPOSE_FILE" logs
    docker compose -f "$COMPOSE_FILE" down
    exit 1
fi
echo "✅ /readyz ready=true"

# Test frontend — must return 200
echo "Checking frontend..."
HTTP_CODE=$(curl -so /dev/null -w "%{http_code}" "$BASE_URL/")
if [ "$HTTP_CODE" != "200" ]; then
    echo "❌ Frontend returned $HTTP_CODE (expected 200)"
    docker compose -f "$COMPOSE_FILE" logs
    docker compose -f "$COMPOSE_FILE" down
    exit 1
fi
echo "✅ Frontend serving OK"

# Test API health (with token if set)
echo "Checking /api/health..."
API_CODE=$(curl -so /dev/null -w "%{http_code}" "$BASE_URL/api/health" \
    -H "Authorization: Bearer ${SOLODAWN_API_TOKEN:-test}")
echo "API health: $API_CODE"

# Restart persistence test
echo "Testing restart persistence..."
docker compose -f "$COMPOSE_FILE" restart
sleep 5
elapsed=0
while [ $elapsed -lt 30 ]; do
    if curl -sf "$BASE_URL/healthz" > /dev/null 2>&1; then
        echo "✅ Service recovered after restart (${elapsed}s)"
        break
    fi
    sleep 2
    elapsed=$((elapsed + 2))
done

if [ $elapsed -ge 30 ]; then
    echo "❌ Service did not recover after restart"
    docker compose -f "$COMPOSE_FILE" logs
    docker compose -f "$COMPOSE_FILE" down
    exit 1
fi

# Cleanup
echo "Cleaning up..."
docker compose -f "$COMPOSE_FILE" down

echo "=== Smoke test complete ==="
```

**Step 2: Commit**

```bash
git add scripts/docker/e2e-smoke.sh
git commit -m "test(docker): add E2E smoke test script"
```

---

### Task 9: 运维文档

**Files:**
- Create: `docs/developed/ops/docker-deployment.md`

**Step 1: 编写部署运维文档**

内容要点：
- 快速启动命令（`docker compose up -d`）
- 环境变量说明表
- 卷挂载说明（assets = DB + config，worktrees = 工作区）
- API Key 注入方式
- 健康检查端点说明
- 日志查看（`docker compose logs -f`）
- 备份恢复（`docker cp` SQLite 文件）
- 回滚步骤（停容器 → 切回本地模式）
- 常见问题排查

**Step 2: Commit**

```bash
git add docs/developed/ops/docker-deployment.md
git commit -m "docs: add Docker deployment operations guide"
```

---

## 验收矩阵

### 功能验收

- [ ] `docker compose up` 后服务从宿主机可访问 `http://localhost:23456`
- [ ] `/healthz` 返回 `{"status":"alive"}`（无需 token）
- [ ] `/readyz` 在 DB + 目录就绪后返回 `{"ready":true}`（无需 token）
- [ ] 前端页面正常加载（SPA 路由正常）
- [ ] 容器内至少 Claude Code / Codex / Gemini CLI 可检测到
- [ ] 重启容器后 DB 数据和工作区仍在（卷持久化）
- [ ] 本地开发模式（`cargo run`）不受影响

### 质量验收

- [ ] Docker 镜像构建成功（CI 绿灯）
- [ ] 冒烟测试脚本全部通过
- [ ] 无安全警告（无明文密钥、无 root 运行建议）
- [ ] 日志中无持续重试或卡死

---

## 回滚策略

1. `LocalDeployment` 始终保留，`SOLODAWN_ASSET_DIR` 不设则走原逻辑。
2. 停止容器 → 直接 `cargo run` 回到本地模式。
3. SQLite 文件可从 Docker volume `docker cp` 出来恢复。
4. 所有新增代码通过环境变量开关控制，不设变量 = 零影响。

---

## 与 TODO 的关系

- 本文定义完整实施蓝图（9 个任务）。
- 执行进度跟踪在 `docs/undeveloped/current/TODO-pending.md`。
- 已完成任务归档在 `docs/developed/misc/TODO-completed.md`。
