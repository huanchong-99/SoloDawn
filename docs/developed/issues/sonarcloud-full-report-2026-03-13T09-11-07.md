启用team模式，拉起多个Agent并行修复（每一个都是全栈开发），推送
# SonarCloud 代码质量完整报告

**生成时间**: 2026/03/13 17:11
**项目**: huanchong-99_GitCortex

---

# SonarCloud Issues 报告

**生成时间**: 2026/03/13 17:11
**问题总数**: 50
**已加载**: 50
**收集数量**: 50

---

## 统计信息

### 按严重程度分类

- **Major**: 27 个
- **Minor**: 12 个
- **Critical**: 10 个
- **Blocker**: 1 个

### 按类型分类

- **Code Smell**: 47 个
- **Bug**: 2 个
- **Vulnerability**: 1 个

### 按影响分类

- **Maintainability**: 29 个
- **Reliability**: 20 个
- **Security**: 1 个

### 按属性分类

- **Consistency**: 27 个
- **Intentionality**: 21 个
- **Adaptability**: 2 个

### 按文件统计 (Top 20)

- **frontend/src/components/ui/shadcn-io/kanban.tsx**: 6 个问题
- **docker/scripts/init-sonar.sh**: 5 个问题
- **frontend/src/components/quality/QualityTimeline.tsx**: 5 个问题
- **docker/scripts/cleanup-quality-data.sh**: 4 个问题
- **docker/scripts/upgrade-sonar.sh**: 4 个问题
- **frontend/src/components/quality/QualityIssueList.tsx**: 3 个问题
- **scripts/quality/run-terminal-gate.sh**: 3 个问题
- **docker/Dockerfile**: 2 个问题
- **frontend/src/components/terminal/TerminalDebugView.tsx**: 2 个问题
- **frontend/.../components/workflow/__tests__/QualityBadge.test.tsx**: 2 个问题
- **.github/workflows/ci-notify.yml**: 1 个问题
- **crates/db/migrations/20260312130000_create_quality_gates.sql**: 1 个问题
- **frontend/src/components/board/TerminalActivityPanel.test.tsx**: 1 个问题
- **frontend/src/components/quality/QualityReportPanel.tsx**: 1 个问题
- **frontend/src/components/tasks/BranchSelector.tsx**: 1 个问题
- **frontend/src/components/terminal/TerminalEmulator.tsx**: 1 个问题
- **frontend/src/components/workflow/QualityBadge.tsx**: 1 个问题
- **frontend/src/components/workflow/steps/Step4Terminals.tsx**: 1 个问题
- **frontend/.../components/workflow/validators/step5Commands.ts**: 1 个问题
- **frontend/src/pages/settings/FeishuSettings.tsx**: 1 个问题

---

## 问题列表（按文件分组）

## 1. .github/workflows/ci-notify.yml

> 该文件共有 **1** 个问题

### 1.1 Change this workflow to not use user-controlled data directly in a run block.

- **问题ID**: `AZzmVhtWhjBidydR36xc`
- **行号**: L351
- **类型**: Vulnerability
- **严重程度**: Blocker
- **属性**: Intentionality
- **影响**: Security
- **标签**: cwe

**问题代码片段**:
```
1: # ============================================================================
2: # CI Workflow: Notify Orchestrator
3: # Triggers: workflow_run completion of ci-basic, ci-quality, ci-docker
4: # Purpose: Posts CI results to GitCortex API for orchestrator consumption
5: # Required check for: None (notification-only)
6: # ============================================================================
7: name: CI Notify Orchestrator
9: on:
10: workflow_run:
11: workflows:
12: - "Basic Checks"
13: - "Quality Gate Check"
14: - "Docker Build Check"
15: types:
16: - completed
18: jobs:
19: notify:
20: name: Notify GitCortex API
21: runs-on: ubuntu-latest
22: timeout-minutes: 5
24: steps:
25: - name: Post result to GitCortex orchestrator
26: env:
27: GITCORTEX_API_URL: ${{ vars.GITCORTEX_API_URL || 'http://localhost:23456' }}
28: GITCORTEX_API_TOKEN: ${{ secrets.GITCORTEX_API_TOKEN }}
29: run: |
30: PAYLOAD=$(cat <<EOF
31: {
32: "workflow": "${{ github.event.workflow_run.name }}",
33: "conclusion": "${{ github.event.workflow_run.conclusion }}",
34: "sha": "${{ github.event.workflow_run.head_sha }}",
35: "branch": "${{ }}",
36: "run_id": ${{ github.event.workflow_run.id }},
37: "run_url": "${{ github.event.workflow_run.html_url }}"
38: }
39: EOF
40: )
42: echo "Sending CI notification:"
43: echo "$PAYLOAD" | jq .
45: # POST to GitCortex API (best-effort, non-blocking)
46: HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
47: -X POST "${GITCORTEX_API_URL}/api/ci/webhook" \
48: -H "Content-Type: application/json" \
49: -H "Authorization: Bearer ${GITCORTEX_API_TOKEN}" \
50: -d "$PAYLOAD" \
51: --connect-timeout 10 \
52: --max-time 30 \
53: ) || true
55: echo "API response status: ${HTTP_STATUS:-timeout}"
57: # Log but don't fail - this is notification-only
```

---

## 2. crates/db/migrations/20260312130000_create_quality_gates.sql

> 该文件共有 **1** 个问题

### 2.1 Define a constant instead of duplicating this literal 3 times.

- **问题ID**: `AZzg0KKBVeL0hYeMa3Nq`
- **行号**: L184
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 184min effort
- **创建时间**: 1 day ago
- **标签**: design

**问题代码片段**:
```
13: mode TEXT NOT NULL -- 'off', 'shadow', 'warn', 'enforce'
14: CHECK (mode IN ('off','shadow','warn','enforce')),
15: gate_name TEXT NOT NULL,
16: duration_ms INTEGER,
17: summary TEXT, -- JSON summary of results
18: created_at TEXT NOT NULL DEFAULT (datetime('now', )),
19: updated_at TEXT NOT NULL DEFAULT (datetime('now',)),
20: FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
21: FOREIGN KEY (workflow_id) REFERENCES workflows(id) ON DELETE CASCADE,
22: FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
23: FOREIGN KEY (terminal_id) REFERENCES terminals(id) ON DELETE CASCADE
24: );
26: CREATE TABLE quality_issues (
27: id BLOB PRIMARY KEY,
28: run_id BLOB NOT NULL,
29: provider TEXT NOT NULL,
30: rule_id TEXT NOT NULL,
31: severity TEXT NOT NULL
32: CHECK (severity IN ('info','minor','major','critical','blocker')),
33: message TEXT NOT NULL,
34: file_path TEXT,
35: line_start INTEGER,
36: line_end INTEGER,
37: column_start INTEGER,
38: column_end INTEGER,
39: created_at TEXT NOT NULL DEFAULT (datetime('now',)),
40: FOREIGN KEY (run_id) REFERENCES quality_runs(id) ON DELETE CASCADE
41: );
```

---

## 3. docker/Dockerfile

> 该文件共有 **2** 个问题

### 3.1 Merge this RUN instruction with the consecutive ones.

- **问题ID**: `AZzTCVgJ6d2gHPl5SzRQ`
- **行号**: L1155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1155min effort
- **创建时间**: 3 days ago

**问题代码片段**:
```
110: FROM debian:trixie-slim AS runtime
111: ARG GITCORTEX_BUILD_NETWORK_PROFILE
112: ARG INSTALL_AI_CLIS=0
113: ARG NODE_MAJOR=22
115: set -eux; \
116: if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
117: sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/*.sources 2>/dev/null || \
118: sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list 2>/dev/null || true; \
119: fi; \
120: printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
121: > /etc/apt/apt.conf.d/80gitcortex-retries; \
122: apt-get update && apt-get install -y --no-install-recommends \
123: libsqlite3-0 libgit2-1.9 git curl ca-certificates bash gnupg xz-utils \
124: && rm -rf /var/lib/apt/lists/*
126: consecutive RUN instruction set -eux; \
127: if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
128: NODE_DIST_URL="https://npmmirror.com/mirrors/node"; \
129: else \
130: NODE_DIST_URL="https://nodejs.org/dist"; \
131: fi; \
132: ARCH="$(dpkg --print-architecture)"; \
133: case "$ARCH" in \
134: amd64) NODE_ARCH="x64" ;; \
135: arm64) NODE_ARCH="arm64" ;; \
136: *) echo "Unsupported arch: $ARCH" >&2; exit 1 ;; \
137: esac; \
138: NODE_VERSION="$(curl -fsSL "${NODE_DIST_URL}/latest-v${NODE_MAJOR}.x/" | grep -oP 'node-v\K[0-9]+\.[0-9]+\.[0-9]+' | head -1)"; \
139: curl -fsSL "${NODE_DIST_URL}/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-${NODE_ARCH}.tar.xz" \
140: | tar -xJ --strip-components=1 -C /usr/local; \
141: node --version && npm --version
143: consecutive RUN instruction set -eux; \
144: npm config --global set fetch-retries 5; \
145: npm config --global set fetch-retry-mintimeout 20000; \
146: npm config --global set fetch-retry-maxtimeout 120000; \
147: npm config --global set fetch-timeout 300000; \
148: if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
149: npm config --global set registry https://registry.npmmirror.com; \
150: fi
152: consecutive RUN instruction set -eux; \
153: if [ "$INSTALL_AI_CLIS" = "1" ]; then \
154: mkdir -p /etc/apt/keyrings; \
155: if curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg \
156: | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
157: && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list \
```

### 3.2 Line is too long. Split it into multiple lines using backslash continuations.

- **问题ID**: `AZzTCVgJ6d2gHPl5SzRS`
- **行号**: L1382
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1382min effort
- **创建时间**: 3 days ago

**问题代码片段**:
```
1: # syntax=docker/dockerfile:1.7
3: ARG GITCORTEX_BUILD_NETWORK_PROFILE=official
4: ARG PNPM_VERSION=10.13.1
5: ARG RUST_TOOLCHAIN=nightly-2025-12-04
6: ARG NODE_MAJOR=22
8: # ============================================================
9: # Stage 1: Frontend build
10: # ============================================================
11: FROM node:${NODE_MAJOR}-slim AS frontend-builder
12: ARG GITCORTEX_BUILD_NETWORK_PROFILE
13: ARG PNPM_VERSION
15: WORKDIR /build
17: # Enable corepack pnpm — zero network cost, no npm install -g
18: RUN corepack enable && corepack prepare "pnpm@${PNPM_VERSION}" --activate
20: # China mirror for npm/pnpm (only affects pnpm install, not corepack)
21: RUN set -eux; \
22: if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
23: npm config --global set registry https://registry.npmmirror.com; \
24: pnpm config set registry https://registry.npmmirror.com --global; \
25: fi
27: # Layer 1: dependency manifest only → cached unless lockfile changes
28: COPY package.json pnpm-lock.yaml pnpm-workspace.yaml .npmrc ./
29: COPY frontend/package.json frontend/
31: RUN --mount=type=cache,id=gitcortex-pnpm-store,target=/pnpm/store,sharing=locked \
32: pnpm config set store-dir /pnpm/store && \
33: cd frontend && \
34: pnpm install --frozen-lockfile --prefer-offline
36: # Layer 2: source code → only rebuilds when src changes
37: COPY frontend/ frontend/
38: COPY shared/ shared/
40: ENV BROWSERSLIST_IGNORE_OLD_DATA=1 \
41: GITCORTEX_BUILD_SOURCEMAP=0
43: RUN cd frontend && pnpm run build:docker
45: # ============================================================
46: # Stage 2: Rust toolchain + system deps (heavily cached layer)
47: # ============================================================
48: FROM rust:slim-trixie AS rust-base
49: ARG GITCORTEX_BUILD_NETWORK_PROFILE
50: ARG RUST_TOOLCHAIN
52: RUN set -eux; \
53: if [ "$GITCORTEX_BUILD_NETWORK_PROFILE" = "china" ]; then \
54: sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list.d/*.sources 2>/dev/null || \
55: sed -i 's|deb.debian.org|mirrors.aliyun.com|g' /etc/apt/sources.list 2>/dev/null || true; \
56: fi; \
57: printf 'Acquire::Retries "5";\nAcquire::http::Timeout "30";\nAcquire::https::Timeout "30";\n' \
58: > /etc/apt/apt.conf.d/80gitcortex-retries; \
59: apt-get update && apt-get install -y --no-install-recommends \
60: git pkg-config libsqlite3-dev libgit2-dev zlib1g-dev \
61: cmake make ninja-build clang libclang-dev perl nasm libssl-dev \
62: && rm -rf /var/lib/apt/lists/*
```

---

## 4. docker/scripts/cleanup-quality-data.sh

> 该文件共有 **4** 个问题

### 4.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoThjBidydR36xO`
- **行号**: L222
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 222min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # cleanup-quality-data.sh — Retention cleanup for quality data, logs, and disk
3: # Usage: ./cleanup-quality-data.sh [db_path] [log_dir]
4: #
5: # Schedule via cron (host) or a one-shot container:
6: # 0 3 * * 0 /path/to/cleanup-quality-data.sh /var/lib/gitcortex/data.db /var/log/gitcortex
7: set -euo pipefail
9: DB_PATH="${1:-/var/lib/gitcortex/data.db}"
10: LOG_DIR="${2:-/var/log/gitcortex}"
11: RETENTION_DAYS=90
12: LOG_COMPRESS_DAYS=7
14: echo "=== Quality Data Cleanup ==="
15: echo " DB path : ${DB_PATH}"
16: echo " Log dir : ${LOG_DIR}"
17: echo " DB retention : ${RETENTION_DAYS} days"
18: echo " Log compress : ${LOG_COMPRESS_DAYS} days"
19: echo ""
21: # ── Step 1: SQLite DB cleanup ─────────────────────────────────────
22: if -f "${DB_PATH}" ]; then
23: echo "[cleanup] Cleaning quality_run records older than ${RETENTION_DAYS} days ..."
24: cutoff=$(date -u -d "-${RETENTION_DAYS} days" +"%Y-%m-%dT%H:%M:%S" 2>/dev/null \
25: || date -u -v-${RETENTION_DAYS}d +"%Y-%m-%dT%H:%M:%S")
27: deleted=$(sqlite3 "${DB_PATH}" <<SQL
28: DELETE FROM quality_run WHERE created_at < '${cutoff}';
29: SELECT changes();
30: SQL
31: )
32: echo "[cleanup] Deleted ${deleted} quality_run rows."
34: # Also clean old terminal_log entries (keep 90 days)
35: deleted_logs=$(sqlite3 "${DB_PATH}" <<SQL
36: DELETE FROM terminal_log WHERE created_at < '${cutoff}';
37: SELECT changes();
38: SQL
39: )
40: echo "[cleanup] Deleted ${deleted_logs} terminal_log rows."
42: # Reclaim space
43: sqlite3 "${DB_PATH}" "VACUUM;"
44: echo "[cleanup] Database vacuumed."
45: else
46: echo "[cleanup] WARN: Database not found at ${DB_PATH}, skipping."
47: fi
49: # ── Step 2: Log rotation ──────────────────────────────────────────
50: if -d "${LOG_DIR}" ]; then
51: echo "[cleanup] Compressing logs older than ${LOG_COMPRESS_DAYS} days ..."
52: compressed=0
53: while IFS= read -r -d '' logfile; do
54: gzip "${logfile}"
55: compressed=$((compressed + 1))
56: done < <(find "${LOG_DIR}" -name "*.log" -mtime +${LOG_COMPRESS_DAYS} -print0 2>/dev/null)
57: echo "[cleanup] Compressed ${compressed} log files."
```

### 4.2 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoThjBidydR36xP`
- **行号**: L502
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 502min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # cleanup-quality-data.sh — Retention cleanup for quality data, logs, and disk
3: # Usage: ./cleanup-quality-data.sh [db_path] [log_dir]
4: #
5: # Schedule via cron (host) or a one-shot container:
6: # 0 3 * * 0 /path/to/cleanup-quality-data.sh /var/lib/gitcortex/data.db /var/log/gitcortex
7: set -euo pipefail
9: DB_PATH="${1:-/var/lib/gitcortex/data.db}"
10: LOG_DIR="${2:-/var/log/gitcortex}"
11: RETENTION_DAYS=90
12: LOG_COMPRESS_DAYS=7
14: echo "=== Quality Data Cleanup ==="
15: echo " DB path : ${DB_PATH}"
16: echo " Log dir : ${LOG_DIR}"
17: echo " DB retention : ${RETENTION_DAYS} days"
18: echo " Log compress : ${LOG_COMPRESS_DAYS} days"
19: echo ""
21: # ── Step 1: SQLite DB cleanup ─────────────────────────────────────
22: if -f "${DB_PATH}" ]; then
23: echo "[cleanup] Cleaning quality_run records older than ${RETENTION_DAYS} days ..."
24: cutoff=$(date -u -d "-${RETENTION_DAYS} days" +"%Y-%m-%dT%H:%M:%S" 2>/dev/null \
25: || date -u -v-${RETENTION_DAYS}d +"%Y-%m-%dT%H:%M:%S")
27: deleted=$(sqlite3 "${DB_PATH}" <<SQL
28: DELETE FROM quality_run WHERE created_at < '${cutoff}';
29: SELECT changes();
30: SQL
31: )
32: echo "[cleanup] Deleted ${deleted} quality_run rows."
34: # Also clean old terminal_log entries (keep 90 days)
35: deleted_logs=$(sqlite3 "${DB_PATH}" <<SQL
36: DELETE FROM terminal_log WHERE created_at < '${cutoff}';
37: SELECT changes();
38: SQL
39: )
40: echo "[cleanup] Deleted ${deleted_logs} terminal_log rows."
42: # Reclaim space
43: sqlite3 "${DB_PATH}" "VACUUM;"
44: echo "[cleanup] Database vacuumed."
45: else
46: echo "[cleanup] WARN: Database not found at ${DB_PATH}, skipping."
47: fi
49: # ── Step 2: Log rotation ──────────────────────────────────────────
50: if -d "${LOG_DIR}" ]; then
51: echo "[cleanup] Compressing logs older than ${LOG_COMPRESS_DAYS} days ..."
52: compressed=0
53: while IFS= read -r -d '' logfile; do
54: gzip "${logfile}"
55: compressed=$((compressed + 1))
56: done < <(find "${LOG_DIR}" -name "*.log" -mtime +${LOG_COMPRESS_DAYS} -print0 2>/dev/null)
57: echo "[cleanup] Compressed ${compressed} log files."
```

### 4.3 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoThjBidydR36xQ`
- **行号**: L732
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 732min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # cleanup-quality-data.sh — Retention cleanup for quality data, logs, and disk
3: # Usage: ./cleanup-quality-data.sh [db_path] [log_dir]
4: #
5: # Schedule via cron (host) or a one-shot container:
6: # 0 3 * * 0 /path/to/cleanup-quality-data.sh /var/lib/gitcortex/data.db /var/log/gitcortex
7: set -euo pipefail
9: DB_PATH="${1:-/var/lib/gitcortex/data.db}"
10: LOG_DIR="${2:-/var/log/gitcortex}"
11: RETENTION_DAYS=90
12: LOG_COMPRESS_DAYS=7
14: echo "=== Quality Data Cleanup ==="
15: echo " DB path : ${DB_PATH}"
16: echo " Log dir : ${LOG_DIR}"
17: echo " DB retention : ${RETENTION_DAYS} days"
18: echo " Log compress : ${LOG_COMPRESS_DAYS} days"
19: echo ""
21: # ── Step 1: SQLite DB cleanup ─────────────────────────────────────
22: if -f "${DB_PATH}" ]; then
23: echo "[cleanup] Cleaning quality_run records older than ${RETENTION_DAYS} days ..."
24: cutoff=$(date -u -d "-${RETENTION_DAYS} days" +"%Y-%m-%dT%H:%M:%S" 2>/dev/null \
25: || date -u -v-${RETENTION_DAYS}d +"%Y-%m-%dT%H:%M:%S")
27: deleted=$(sqlite3 "${DB_PATH}" <<SQL
28: DELETE FROM quality_run WHERE created_at < '${cutoff}';
29: SELECT changes();
30: SQL
31: )
32: echo "[cleanup] Deleted ${deleted} quality_run rows."
34: # Also clean old terminal_log entries (keep 90 days)
35: deleted_logs=$(sqlite3 "${DB_PATH}" <<SQL
36: DELETE FROM terminal_log WHERE created_at < '${cutoff}';
37: SELECT changes();
38: SQL
39: )
40: echo "[cleanup] Deleted ${deleted_logs} terminal_log rows."
42: # Reclaim space
43: sqlite3 "${DB_PATH}" "VACUUM;"
44: echo "[cleanup] Database vacuumed."
45: else
46: echo "[cleanup] WARN: Database not found at ${DB_PATH}, skipping."
47: fi
49: # ── Step 2: Log rotation ──────────────────────────────────────────
50: if -d "${LOG_DIR}" ]; then
51: echo "[cleanup] Compressing logs older than ${LOG_COMPRESS_DAYS} days ..."
52: compressed=0
53: while IFS= read -r -d '' logfile; do
54: gzip "${logfile}"
55: compressed=$((compressed + 1))
56: done < <(find "${LOG_DIR}" -name "*.log" -mtime +${LOG_COMPRESS_DAYS} -print0 2>/dev/null)
57: echo "[cleanup] Compressed ${compressed} log files."
```

### 4.4 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoThjBidydR36xR`
- **行号**: L772
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 772min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # cleanup-quality-data.sh — Retention cleanup for quality data, logs, and disk
3: # Usage: ./cleanup-quality-data.sh [db_path] [log_dir]
4: #
5: # Schedule via cron (host) or a one-shot container:
6: # 0 3 * * 0 /path/to/cleanup-quality-data.sh /var/lib/gitcortex/data.db /var/log/gitcortex
7: set -euo pipefail
9: DB_PATH="${1:-/var/lib/gitcortex/data.db}"
10: LOG_DIR="${2:-/var/log/gitcortex}"
11: RETENTION_DAYS=90
12: LOG_COMPRESS_DAYS=7
14: echo "=== Quality Data Cleanup ==="
15: echo " DB path : ${DB_PATH}"
16: echo " Log dir : ${LOG_DIR}"
17: echo " DB retention : ${RETENTION_DAYS} days"
18: echo " Log compress : ${LOG_COMPRESS_DAYS} days"
19: echo ""
21: # ── Step 1: SQLite DB cleanup ─────────────────────────────────────
22: if -f "${DB_PATH}" ]; then
23: echo "[cleanup] Cleaning quality_run records older than ${RETENTION_DAYS} days ..."
24: cutoff=$(date -u -d "-${RETENTION_DAYS} days" +"%Y-%m-%dT%H:%M:%S" 2>/dev/null \
25: || date -u -v-${RETENTION_DAYS}d +"%Y-%m-%dT%H:%M:%S")
27: deleted=$(sqlite3 "${DB_PATH}" <<SQL
28: DELETE FROM quality_run WHERE created_at < '${cutoff}';
29: SELECT changes();
30: SQL
31: )
32: echo "[cleanup] Deleted ${deleted} quality_run rows."
34: # Also clean old terminal_log entries (keep 90 days)
35: deleted_logs=$(sqlite3 "${DB_PATH}" <<SQL
36: DELETE FROM terminal_log WHERE created_at < '${cutoff}';
37: SELECT changes();
38: SQL
39: )
40: echo "[cleanup] Deleted ${deleted_logs} terminal_log rows."
42: # Reclaim space
43: sqlite3 "${DB_PATH}" "VACUUM;"
44: echo "[cleanup] Database vacuumed."
45: else
46: echo "[cleanup] WARN: Database not found at ${DB_PATH}, skipping."
47: fi
49: # ── Step 2: Log rotation ──────────────────────────────────────────
50: if -d "${LOG_DIR}" ]; then
51: echo "[cleanup] Compressing logs older than ${LOG_COMPRESS_DAYS} days ..."
52: compressed=0
53: while IFS= read -r -d '' logfile; do
54: gzip "${logfile}"
55: compressed=$((compressed + 1))
56: done < <(find "${LOG_DIR}" -name "*.log" -mtime +${LOG_COMPRESS_DAYS} -print0 2>/dev/null)
57: echo "[cleanup] Compressed ${compressed} log files."
```

---

## 5. docker/scripts/init-sonar.sh

> 该文件共有 **5** 个问题

### 5.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoqhjBidydR36xT`
- **行号**: L182
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 182min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # init-sonar.sh — Initialize SonarQube for GitCortex
3: # Usage: ./init-sonar.sh [sonar_url] [admin_password]
4: set -euo pipefail
6: SONAR_URL="${1:-http://localhost:9000}"
7: ADMIN_PASS="${2:-admin}"
8: PROJECT_KEY="gitcortex"
9: PROJECT_NAME="GitCortex"
10: MAX_WAIT=300
11: POLL_INTERVAL=5
13: # ── Step 1: Wait for SonarQube to be healthy ──────────────────────
14: echo "[init-sonar] Waiting for SonarQube at ${SONAR_URL} ..."
15: elapsed=0
16: while true; do
17: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
18: if "$status" = "UP" ]; then
19: echo "[init-sonar] SonarQube is UP (${elapsed}s)"
20: break
21: fi
22: if "$elapsed" -ge "$MAX_WAIT" ]; then
24: exit 1
25: fi
26: sleep "$POLL_INTERVAL"
27: elapsed=$((elapsed + POLL_INTERVAL))
28: done
30: # ── Step 2: Create project if it doesn't exist ────────────────────
31: echo "[init-sonar] Checking project '${PROJECT_KEY}' ..."
32: project_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
33: "${SONAR_URL}/api/projects/search?projects=${PROJECT_KEY}" \
34: | grep -c "\"key\":\"${PROJECT_KEY}\"" || true)
36: if "$project_exists" -eq 0 ]; then
37: echo "[init-sonar] Creating project '${PROJECT_KEY}' ..."
38: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
39: "${SONAR_URL}/api/projects/create" \
40: -d "project=${PROJECT_KEY}&name=${PROJECT_NAME}" \
41: > /dev/null
42: echo "[init-sonar] Project created."
43: else
44: echo "[init-sonar] Project already exists, skipping."
45: fi
47: # ── Step 3: Set up quality profiles ───────────────────────────────
48: echo "[init-sonar] Configuring quality profiles ..."
49: for lang in ts js; do
50: # Use the built-in "Sonar way" profile as default
51: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
52: "${SONAR_URL}/api/qualityprofiles/set_default" \
53: -d "language=${lang}&qualityProfile=Sonar%20way" \
54: > /dev/null 2>&1 || echo "[init-sonar] WARN: Could not set default profile for ${lang}"
55: done
56: echo "[init-sonar] Quality profiles configured."
```

### 5.2 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoqhjBidydR36xU`
- **行号**: L222
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 222min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # init-sonar.sh — Initialize SonarQube for GitCortex
3: # Usage: ./init-sonar.sh [sonar_url] [admin_password]
4: set -euo pipefail
6: SONAR_URL="${1:-http://localhost:9000}"
7: ADMIN_PASS="${2:-admin}"
8: PROJECT_KEY="gitcortex"
9: PROJECT_NAME="GitCortex"
10: MAX_WAIT=300
11: POLL_INTERVAL=5
13: # ── Step 1: Wait for SonarQube to be healthy ──────────────────────
14: echo "[init-sonar] Waiting for SonarQube at ${SONAR_URL} ..."
15: elapsed=0
16: while true; do
17: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
18: if "$status" = "UP" ]; then
19: echo "[init-sonar] SonarQube is UP (${elapsed}s)"
20: break
21: fi
22: if "$elapsed" -ge "$MAX_WAIT" ]; then
24: exit 1
25: fi
26: sleep "$POLL_INTERVAL"
27: elapsed=$((elapsed + POLL_INTERVAL))
28: done
30: # ── Step 2: Create project if it doesn't exist ────────────────────
31: echo "[init-sonar] Checking project '${PROJECT_KEY}' ..."
32: project_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
33: "${SONAR_URL}/api/projects/search?projects=${PROJECT_KEY}" \
34: | grep -c "\"key\":\"${PROJECT_KEY}\"" || true)
36: if "$project_exists" -eq 0 ]; then
37: echo "[init-sonar] Creating project '${PROJECT_KEY}' ..."
38: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
39: "${SONAR_URL}/api/projects/create" \
40: -d "project=${PROJECT_KEY}&name=${PROJECT_NAME}" \
41: > /dev/null
42: echo "[init-sonar] Project created."
43: else
44: echo "[init-sonar] Project already exists, skipping."
45: fi
47: # ── Step 3: Set up quality profiles ───────────────────────────────
48: echo "[init-sonar] Configuring quality profiles ..."
49: for lang in ts js; do
50: # Use the built-in "Sonar way" profile as default
51: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
52: "${SONAR_URL}/api/qualityprofiles/set_default" \
53: -d "language=${lang}&qualityProfile=Sonar%20way" \
54: > /dev/null 2>&1 || echo "[init-sonar] WARN: Could not set default profile for ${lang}"
55: done
56: echo "[init-sonar] Quality profiles configured."
```

### 5.3 Redirect this error message to stderr (>&2).

- **问题ID**: `AZzmVhoqhjBidydR36xS`
- **行号**: L235
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 235min effort
- **标签**: output, posix, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # init-sonar.sh — Initialize SonarQube for GitCortex
3: # Usage: ./init-sonar.sh [sonar_url] [admin_password]
4: set -euo pipefail
6: SONAR_URL="${1:-http://localhost:9000}"
7: ADMIN_PASS="${2:-admin}"
8: PROJECT_KEY="gitcortex"
9: PROJECT_NAME="GitCortex"
10: MAX_WAIT=300
11: POLL_INTERVAL=5
13: # ── Step 1: Wait for SonarQube to be healthy ──────────────────────
14: echo "[init-sonar] Waiting for SonarQube at ${SONAR_URL} ..."
15: elapsed=0
16: while true; do
17: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
18: if "$status" = "UP" ]; then
19: echo "[init-sonar] SonarQube is UP (${elapsed}s)"
20: break
21: fi
22: if "$elapsed" -ge "$MAX_WAIT" ]; then
24: exit 1
25: fi
26: sleep "$POLL_INTERVAL"
27: elapsed=$((elapsed + POLL_INTERVAL))
28: done
30: # ── Step 2: Create project if it doesn't exist ────────────────────
31: echo "[init-sonar] Checking project '${PROJECT_KEY}' ..."
32: project_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
33: "${SONAR_URL}/api/projects/search?projects=${PROJECT_KEY}" \
34: | grep -c "\"key\":\"${PROJECT_KEY}\"" || true)
36: if "$project_exists" -eq 0 ]; then
37: echo "[init-sonar] Creating project '${PROJECT_KEY}' ..."
38: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
39: "${SONAR_URL}/api/projects/create" \
40: -d "project=${PROJECT_KEY}&name=${PROJECT_NAME}" \
41: > /dev/null
42: echo "[init-sonar] Project created."
43: else
44: echo "[init-sonar] Project already exists, skipping."
45: fi
47: # ── Step 3: Set up quality profiles ───────────────────────────────
48: echo "[init-sonar] Configuring quality profiles ..."
49: for lang in ts js; do
50: # Use the built-in "Sonar way" profile as default
51: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
52: "${SONAR_URL}/api/qualityprofiles/set_default" \
53: -d "language=${lang}&qualityProfile=Sonar%20way" \
54: > /dev/null 2>&1 || echo "[init-sonar] WARN: Could not set default profile for ${lang}"
55: done
56: echo "[init-sonar] Quality profiles configured."
```

### 5.4 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoqhjBidydR36xV`
- **行号**: L362
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 362min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # init-sonar.sh — Initialize SonarQube for GitCortex
3: # Usage: ./init-sonar.sh [sonar_url] [admin_password]
4: set -euo pipefail
6: SONAR_URL="${1:-http://localhost:9000}"
7: ADMIN_PASS="${2:-admin}"
8: PROJECT_KEY="gitcortex"
9: PROJECT_NAME="GitCortex"
10: MAX_WAIT=300
11: POLL_INTERVAL=5
13: # ── Step 1: Wait for SonarQube to be healthy ──────────────────────
14: echo "[init-sonar] Waiting for SonarQube at ${SONAR_URL} ..."
15: elapsed=0
16: while true; do
17: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
18: if "$status" = "UP" ]; then
19: echo "[init-sonar] SonarQube is UP (${elapsed}s)"
20: break
21: fi
22: if "$elapsed" -ge "$MAX_WAIT" ]; then
24: exit 1
25: fi
26: sleep "$POLL_INTERVAL"
27: elapsed=$((elapsed + POLL_INTERVAL))
28: done
30: # ── Step 2: Create project if it doesn't exist ────────────────────
31: echo "[init-sonar] Checking project '${PROJECT_KEY}' ..."
32: project_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
33: "${SONAR_URL}/api/projects/search?projects=${PROJECT_KEY}" \
34: | grep -c "\"key\":\"${PROJECT_KEY}\"" || true)
36: if "$project_exists" -eq 0 ]; then
37: echo "[init-sonar] Creating project '${PROJECT_KEY}' ..."
38: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
39: "${SONAR_URL}/api/projects/create" \
40: -d "project=${PROJECT_KEY}&name=${PROJECT_NAME}" \
41: > /dev/null
42: echo "[init-sonar] Project created."
43: else
44: echo "[init-sonar] Project already exists, skipping."
45: fi
47: # ── Step 3: Set up quality profiles ───────────────────────────────
48: echo "[init-sonar] Configuring quality profiles ..."
49: for lang in ts js; do
50: # Use the built-in "Sonar way" profile as default
51: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
52: "${SONAR_URL}/api/qualityprofiles/set_default" \
53: -d "language=${lang}&qualityProfile=Sonar%20way" \
54: > /dev/null 2>&1 || echo "[init-sonar] WARN: Could not set default profile for ${lang}"
55: done
56: echo "[init-sonar] Quality profiles configured."
```

### 5.5 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhoqhjBidydR36xW`
- **行号**: L672
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 672min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # init-sonar.sh — Initialize SonarQube for GitCortex
3: # Usage: ./init-sonar.sh [sonar_url] [admin_password]
4: set -euo pipefail
6: SONAR_URL="${1:-http://localhost:9000}"
7: ADMIN_PASS="${2:-admin}"
8: PROJECT_KEY="gitcortex"
9: PROJECT_NAME="GitCortex"
10: MAX_WAIT=300
11: POLL_INTERVAL=5
13: # ── Step 1: Wait for SonarQube to be healthy ──────────────────────
14: echo "[init-sonar] Waiting for SonarQube at ${SONAR_URL} ..."
15: elapsed=0
16: while true; do
17: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
18: if "$status" = "UP" ]; then
19: echo "[init-sonar] SonarQube is UP (${elapsed}s)"
20: break
21: fi
22: if "$elapsed" -ge "$MAX_WAIT" ]; then
24: exit 1
25: fi
26: sleep "$POLL_INTERVAL"
27: elapsed=$((elapsed + POLL_INTERVAL))
28: done
30: # ── Step 2: Create project if it doesn't exist ────────────────────
31: echo "[init-sonar] Checking project '${PROJECT_KEY}' ..."
32: project_exists=$(curl -sf -u "admin:${ADMIN_PASS}" \
33: "${SONAR_URL}/api/projects/search?projects=${PROJECT_KEY}" \
34: | grep -c "\"key\":\"${PROJECT_KEY}\"" || true)
36: if "$project_exists" -eq 0 ]; then
37: echo "[init-sonar] Creating project '${PROJECT_KEY}' ..."
38: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
39: "${SONAR_URL}/api/projects/create" \
40: -d "project=${PROJECT_KEY}&name=${PROJECT_NAME}" \
41: > /dev/null
42: echo "[init-sonar] Project created."
43: else
44: echo "[init-sonar] Project already exists, skipping."
45: fi
47: # ── Step 3: Set up quality profiles ───────────────────────────────
48: echo "[init-sonar] Configuring quality profiles ..."
49: for lang in ts js; do
50: # Use the built-in "Sonar way" profile as default
51: curl -sf -u "admin:${ADMIN_PASS}" -X POST \
52: "${SONAR_URL}/api/qualityprofiles/set_default" \
53: -d "language=${lang}&qualityProfile=Sonar%20way" \
54: > /dev/null 2>&1 || echo "[init-sonar] WARN: Could not set default profile for ${lang}"
55: done
56: echo "[init-sonar] Quality profiles configured."
```

---

## 6. docker/scripts/upgrade-sonar.sh

> 该文件共有 **4** 个问题

### 6.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhn8hjBidydR36xL`
- **行号**: L592
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 592min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # upgrade-sonar.sh — Upgrade SonarQube in Docker Compose
3: # Usage: ./upgrade-sonar.sh [new_image_tag]
4: # Run from the docker/compose/ directory (or pass -f path).
5: set -euo pipefail
7: COMPOSE_DIR="$(cd "$(dirname "$0")/../compose" && pwd)"
8: NEW_TAG="${1:-latest}"
9: BACKUP_DIR="${COMPOSE_DIR}/../backups/sonar-$(date +%Y%m%d-%H%M%S)"
10: SONAR_URL="http://localhost:9000"
11: MAX_WAIT=300
12: POLL_INTERVAL=5
14: echo "=== SonarQube Upgrade ==="
15: echo " Compose dir : ${COMPOSE_DIR}"
16: echo " New tag : ${NEW_TAG}"
17: echo " Backup dir : ${BACKUP_DIR}"
18: echo ""
20: # ── Step 1: Back up current configuration ─────────────────────────
21: echo "[upgrade] Backing up current config ..."
22: mkdir -p "${BACKUP_DIR}"
24: # Export docker volume data
25: for vol in sonarqube-data sonarqube-extensions sonarqube-logs; do
26: full_vol="compose_${vol}"
27: if docker volume inspect "${full_vol}" > /dev/null 2>&1; then
28: echo "[upgrade] Backing up volume ${full_vol} ..."
29: docker run --rm \
30: -v "${full_vol}:/source:ro" \
31: -v "${BACKUP_DIR}:/backup" \
32: alpine tar czf "/backup/${vol}.tar.gz" -C /source .
33: fi
34: done
36: # Save current compose config
37: cp "${COMPOSE_DIR}/docker-compose.yml" "${BACKUP_DIR}/docker-compose.yml.bak"
38: echo "[upgrade] Backup complete -> ${BACKUP_DIR}"
40: # ── Step 2: Pull new image ────────────────────────────────────────
41: echo "[upgrade] Pulling sonarqube:${NEW_TAG} ..."
42: docker pull "sonarqube:${NEW_TAG}"
44: # ── Step 3: Stop SonarQube and run database migration ─────────────
45: echo "[upgrade] Stopping SonarQube ..."
46: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" stop sonarqube
48: echo "[upgrade] Starting SonarQube with new image (DB migration runs automatically) ..."
49: # SonarQube runs migrations on startup automatically
50: SONARQUBE_IMAGE="sonarqube:${NEW_TAG}" \
51: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" up -d sonarqube
53: # ── Step 4: Verify health ─────────────────────────────────────────
54: echo "[upgrade] Waiting for SonarQube to become healthy ..."
55: elapsed=0
56: while true; do
57: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null \
58: | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
59: if "$status" = "UP" ]; then
```

### 6.2 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhn8hjBidydR36xM`
- **行号**: L632
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 632min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # upgrade-sonar.sh — Upgrade SonarQube in Docker Compose
3: # Usage: ./upgrade-sonar.sh [new_image_tag]
4: # Run from the docker/compose/ directory (or pass -f path).
5: set -euo pipefail
7: COMPOSE_DIR="$(cd "$(dirname "$0")/../compose" && pwd)"
8: NEW_TAG="${1:-latest}"
9: BACKUP_DIR="${COMPOSE_DIR}/../backups/sonar-$(date +%Y%m%d-%H%M%S)"
10: SONAR_URL="http://localhost:9000"
11: MAX_WAIT=300
12: POLL_INTERVAL=5
14: echo "=== SonarQube Upgrade ==="
15: echo " Compose dir : ${COMPOSE_DIR}"
16: echo " New tag : ${NEW_TAG}"
17: echo " Backup dir : ${BACKUP_DIR}"
18: echo ""
20: # ── Step 1: Back up current configuration ─────────────────────────
21: echo "[upgrade] Backing up current config ..."
22: mkdir -p "${BACKUP_DIR}"
24: # Export docker volume data
25: for vol in sonarqube-data sonarqube-extensions sonarqube-logs; do
26: full_vol="compose_${vol}"
27: if docker volume inspect "${full_vol}" > /dev/null 2>&1; then
28: echo "[upgrade] Backing up volume ${full_vol} ..."
29: docker run --rm \
30: -v "${full_vol}:/source:ro" \
31: -v "${BACKUP_DIR}:/backup" \
32: alpine tar czf "/backup/${vol}.tar.gz" -C /source .
33: fi
34: done
36: # Save current compose config
37: cp "${COMPOSE_DIR}/docker-compose.yml" "${BACKUP_DIR}/docker-compose.yml.bak"
38: echo "[upgrade] Backup complete -> ${BACKUP_DIR}"
40: # ── Step 2: Pull new image ────────────────────────────────────────
41: echo "[upgrade] Pulling sonarqube:${NEW_TAG} ..."
42: docker pull "sonarqube:${NEW_TAG}"
44: # ── Step 3: Stop SonarQube and run database migration ─────────────
45: echo "[upgrade] Stopping SonarQube ..."
46: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" stop sonarqube
48: echo "[upgrade] Starting SonarQube with new image (DB migration runs automatically) ..."
49: # SonarQube runs migrations on startup automatically
50: SONARQUBE_IMAGE="sonarqube:${NEW_TAG}" \
51: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" up -d sonarqube
53: # ── Step 4: Verify health ─────────────────────────────────────────
54: echo "[upgrade] Waiting for SonarQube to become healthy ..."
55: elapsed=0
56: while true; do
57: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null \
58: | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
59: if "$status" = "UP" ]; then
```

### 6.3 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhn8hjBidydR36xN`
- **行号**: L662
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 662min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # upgrade-sonar.sh — Upgrade SonarQube in Docker Compose
3: # Usage: ./upgrade-sonar.sh [new_image_tag]
4: # Run from the docker/compose/ directory (or pass -f path).
5: set -euo pipefail
7: COMPOSE_DIR="$(cd "$(dirname "$0")/../compose" && pwd)"
8: NEW_TAG="${1:-latest}"
9: BACKUP_DIR="${COMPOSE_DIR}/../backups/sonar-$(date +%Y%m%d-%H%M%S)"
10: SONAR_URL="http://localhost:9000"
11: MAX_WAIT=300
12: POLL_INTERVAL=5
14: echo "=== SonarQube Upgrade ==="
15: echo " Compose dir : ${COMPOSE_DIR}"
16: echo " New tag : ${NEW_TAG}"
17: echo " Backup dir : ${BACKUP_DIR}"
18: echo ""
20: # ── Step 1: Back up current configuration ─────────────────────────
21: echo "[upgrade] Backing up current config ..."
22: mkdir -p "${BACKUP_DIR}"
24: # Export docker volume data
25: for vol in sonarqube-data sonarqube-extensions sonarqube-logs; do
26: full_vol="compose_${vol}"
27: if docker volume inspect "${full_vol}" > /dev/null 2>&1; then
28: echo "[upgrade] Backing up volume ${full_vol} ..."
29: docker run --rm \
30: -v "${full_vol}:/source:ro" \
31: -v "${BACKUP_DIR}:/backup" \
32: alpine tar czf "/backup/${vol}.tar.gz" -C /source .
33: fi
34: done
36: # Save current compose config
37: cp "${COMPOSE_DIR}/docker-compose.yml" "${BACKUP_DIR}/docker-compose.yml.bak"
38: echo "[upgrade] Backup complete -> ${BACKUP_DIR}"
40: # ── Step 2: Pull new image ────────────────────────────────────────
41: echo "[upgrade] Pulling sonarqube:${NEW_TAG} ..."
42: docker pull "sonarqube:${NEW_TAG}"
44: # ── Step 3: Stop SonarQube and run database migration ─────────────
45: echo "[upgrade] Stopping SonarQube ..."
46: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" stop sonarqube
48: echo "[upgrade] Starting SonarQube with new image (DB migration runs automatically) ..."
49: # SonarQube runs migrations on startup automatically
50: SONARQUBE_IMAGE="sonarqube:${NEW_TAG}" \
51: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" up -d sonarqube
53: # ── Step 4: Verify health ─────────────────────────────────────────
54: echo "[upgrade] Waiting for SonarQube to become healthy ..."
55: elapsed=0
56: while true; do
57: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null \
58: | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
59: if "$status" = "UP" ]; then
```

### 6.4 Redirect this error message to stderr (>&2).

- **问题ID**: `AZzmVhn8hjBidydR36xK`
- **行号**: L675
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 675min effort
- **标签**: output, posix, ...

**问题代码片段**:
```
1: #!/usr/bin/env bash
2: # upgrade-sonar.sh — Upgrade SonarQube in Docker Compose
3: # Usage: ./upgrade-sonar.sh [new_image_tag]
4: # Run from the docker/compose/ directory (or pass -f path).
5: set -euo pipefail
7: COMPOSE_DIR="$(cd "$(dirname "$0")/../compose" && pwd)"
8: NEW_TAG="${1:-latest}"
9: BACKUP_DIR="${COMPOSE_DIR}/../backups/sonar-$(date +%Y%m%d-%H%M%S)"
10: SONAR_URL="http://localhost:9000"
11: MAX_WAIT=300
12: POLL_INTERVAL=5
14: echo "=== SonarQube Upgrade ==="
15: echo " Compose dir : ${COMPOSE_DIR}"
16: echo " New tag : ${NEW_TAG}"
17: echo " Backup dir : ${BACKUP_DIR}"
18: echo ""
20: # ── Step 1: Back up current configuration ─────────────────────────
21: echo "[upgrade] Backing up current config ..."
22: mkdir -p "${BACKUP_DIR}"
24: # Export docker volume data
25: for vol in sonarqube-data sonarqube-extensions sonarqube-logs; do
26: full_vol="compose_${vol}"
27: if docker volume inspect "${full_vol}" > /dev/null 2>&1; then
28: echo "[upgrade] Backing up volume ${full_vol} ..."
29: docker run --rm \
30: -v "${full_vol}:/source:ro" \
31: -v "${BACKUP_DIR}:/backup" \
32: alpine tar czf "/backup/${vol}.tar.gz" -C /source .
33: fi
34: done
36: # Save current compose config
37: cp "${COMPOSE_DIR}/docker-compose.yml" "${BACKUP_DIR}/docker-compose.yml.bak"
38: echo "[upgrade] Backup complete -> ${BACKUP_DIR}"
40: # ── Step 2: Pull new image ────────────────────────────────────────
41: echo "[upgrade] Pulling sonarqube:${NEW_TAG} ..."
42: docker pull "sonarqube:${NEW_TAG}"
44: # ── Step 3: Stop SonarQube and run database migration ─────────────
45: echo "[upgrade] Stopping SonarQube ..."
46: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" stop sonarqube
48: echo "[upgrade] Starting SonarQube with new image (DB migration runs automatically) ..."
49: # SonarQube runs migrations on startup automatically
50: SONARQUBE_IMAGE="sonarqube:${NEW_TAG}" \
51: docker compose -f "${COMPOSE_DIR}/docker-compose.yml" up -d sonarqube
53: # ── Step 4: Verify health ─────────────────────────────────────────
54: echo "[upgrade] Waiting for SonarQube to become healthy ..."
55: elapsed=0
56: while true; do
57: status=$(curl -sf "${SONAR_URL}/api/system/status" 2>/dev/null \
58: | grep -o '"status":"[^"]*"' | cut -d'"' -f4 || true)
59: if "$status" = "UP" ]; then
```

---

## 7. frontend/.../components/workflow/__tests__/QualityBadge.test.tsx

> 该文件共有 **2** 个问题

### 7.1 'opts.count' may use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZzmVg8WhjBidydR36xI`
- **行号**: L165
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 165min effort
- **标签**: object, string, ...

**问题代码片段**:
```
1: import { render, screen } from '@testing-library/react';
2: import { describe, it, expect, vi } from 'vitest';
4: vi.mock('react-i18next', () => ({
5: useTranslation: () => ({
6: t: (key: string, opts?: Record<string, unknown>) => {
7: const translations: Record<string, string> = {
8: 'status.ok': 'Passed',
9: 'status.warn': 'Warnings',
10: 'status.error': 'Failed',
11: 'status.running': 'Running',
12: 'status.pending': 'Pending',
13: 'status.skipped': 'Skipped',
14: };
15: if (key === 'status.warnCount' && opts?.count !== undefined) {
16: return `${} warnings`;
17: }
18: if (key === 'status.errorCount' && opts?.count !== undefined) {
19: return `${} blocking`;
20: }
21: return translations[key] ?? key;
22: },
23: }),
24: }));
26: import { QualityBadge } from '../../workflow/QualityBadge';
28: describe('QualityBadge', () => {
29: it('renders ok status with Passed label', () => {
30: render(<QualityBadge gateStatus="ok" />);
31: expect(screen.getByText('Passed')).toBeInTheDocument();
32: });
34: it('renders error status with Failed label', () => {
35: render(<QualityBadge gateStatus="error" />);
36: expect(screen.getByText('Failed')).toBeInTheDocument();
37: });
39: it('renders error status with blocking count', () => {
40: render(<QualityBadge gateStatus="error" blockingIssues={5} />);
41: expect(screen.getByText('5 blocking')).toBeInTheDocument();
42: });
44: it('renders warn status with Warnings label', () => {
45: render(<QualityBadge gateStatus="warn" />);
46: expect(screen.getByText('Warnings')).toBeInTheDocument();
47: });
49: it('renders warn status with warning count', () => {
50: render(<QualityBadge gateStatus="warn" blockingIssues={3} />);
51: expect(screen.getByText('3 warnings')).toBeInTheDocument();
52: });
54: it('renders pending status', () => {
55: render(<QualityBadge gateStatus="pending" />);
56: expect(screen.getByText('Pending')).toBeInTheDocument();
57: });
59: it('renders running status', () => {
```

### 7.2 'opts.count' may use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZzmVg8WhjBidydR36xJ`
- **行号**: L195
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 195min effort
- **标签**: object, string, ...

**问题代码片段**:
```
1: import { render, screen } from '@testing-library/react';
2: import { describe, it, expect, vi } from 'vitest';
4: vi.mock('react-i18next', () => ({
5: useTranslation: () => ({
6: t: (key: string, opts?: Record<string, unknown>) => {
7: const translations: Record<string, string> = {
8: 'status.ok': 'Passed',
9: 'status.warn': 'Warnings',
10: 'status.error': 'Failed',
11: 'status.running': 'Running',
12: 'status.pending': 'Pending',
13: 'status.skipped': 'Skipped',
14: };
15: if (key === 'status.warnCount' && opts?.count !== undefined) {
16: return `${} warnings`;
17: }
18: if (key === 'status.errorCount' && opts?.count !== undefined) {
19: return `${} blocking`;
20: }
21: return translations[key] ?? key;
22: },
23: }),
24: }));
26: import { QualityBadge } from '../../workflow/QualityBadge';
28: describe('QualityBadge', () => {
29: it('renders ok status with Passed label', () => {
30: render(<QualityBadge gateStatus="ok" />);
31: expect(screen.getByText('Passed')).toBeInTheDocument();
32: });
34: it('renders error status with Failed label', () => {
35: render(<QualityBadge gateStatus="error" />);
36: expect(screen.getByText('Failed')).toBeInTheDocument();
37: });
39: it('renders error status with blocking count', () => {
40: render(<QualityBadge gateStatus="error" blockingIssues={5} />);
41: expect(screen.getByText('5 blocking')).toBeInTheDocument();
42: });
44: it('renders warn status with Warnings label', () => {
45: render(<QualityBadge gateStatus="warn" />);
46: expect(screen.getByText('Warnings')).toBeInTheDocument();
47: });
49: it('renders warn status with warning count', () => {
50: render(<QualityBadge gateStatus="warn" blockingIssues={3} />);
51: expect(screen.getByText('3 warnings')).toBeInTheDocument();
52: });
54: it('renders pending status', () => {
55: render(<QualityBadge gateStatus="pending" />);
56: expect(screen.getByText('Pending')).toBeInTheDocument();
57: });
59: it('renders running status', () => {
```

---

## 8. frontend/.../components/workflow/validators/step5Commands.ts

> 该文件共有 **1** 个问题

### 8.1 Remove this use of the "void" operator.

- **问题ID**: `AZyVweU7Z9DOUQdEsGdP`
- **行号**: L75
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 75min effort
- **创建时间**: 1 month ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import type { WizardConfig } from '../types';
3: /**
4: * Validates command preset selection for step 5.
5: */
6: export function validateStep5Commands(config: WizardConfig): Record<string, string> {
7: config;
8: return {};
9: }
```

---

## 9. frontend/src/components/board/TerminalActivityPanel.test.tsx

> 该文件共有 **1** 个问题

### 9.1 'opts.count' may use Object's default stringification format ('[object Object]') when stringified.

- **问题ID**: `AZzh345GklupxyAQ7gxO`
- **行号**: L255
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 255min effort
- **创建时间**: 21 hours ago
- **标签**: object, string, ...

**问题代码片段**:
```
1: import { render, screen } from '@testing-library/react';
2: import { describe, it, expect, vi, beforeEach } from 'vitest';
4: // Mock hooks before imports
5: vi.mock('@/hooks/useWorkflows', () => ({
6: useWorkflow: vi.fn(),
7: }));
9: vi.mock('@/stores/terminalStore', () => ({
10: useRecentTerminalOutput: vi.fn(() => []),
11: }));
13: vi.mock('react-i18next', () => ({
14: useTranslation: () => ({
15: t: (key: string, opts?: Record<string, unknown>) => {
16: const translations: Record<string, string> = {
17: 'terminalActivity.title': 'Terminal Activity',
18: 'terminalActivity.selectWorkflow': 'Select a workflow to view terminal activity.',
19: 'terminalActivity.loading': 'Loading terminal activity...',
20: 'terminalActivity.noTerminalsYet': 'No terminals yet.',
21: 'terminalActivity.noActive': 'No active terminals.',
22: 'terminalActivity.defaultLabel': 'Terminal',
23: };
24: if (opts?.count !== undefined && key === 'terminalActivity.active') {
25: return `${} active`;
26: }
27: return translations[key] ?? key;
28: },
29: }),
30: }));
32: vi.mock('react-router-dom', () => ({
33: Link: ({ children, to, ...rest }: any) => <a href={to} {...rest}>{children}</a>,
34: }));
36: import { TerminalActivityPanel } from './TerminalActivityPanel';
37: import { useWorkflow } from '@/hooks/useWorkflows';
39: describe('TerminalActivityPanel', () => {
40: beforeEach(() => {
41: vi.clearAllMocks();
42: });
44: it('renders title and select-workflow message when no workflow selected', () => {
45: vi.mocked(useWorkflow).mockReturnValue({
46: data: undefined,
47: isLoading: false,
48: error: null,
49: } as any);
51: render(<TerminalActivityPanel workflowId={null} />);
52: expect(screen.getByText('Terminal Activity')).toBeInTheDocument();
53: expect(
54: screen.getByText('Select a workflow to view terminal activity.')
55: ).toBeInTheDocument();
56: });
57: });
```

---

## 10. frontend/src/components/quality/QualityIssueList.tsx

> 该文件共有 **3** 个问题

### 10.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZzg0Jx9VeL0hYeMa3Nc`
- **行号**: L335
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 335min effort
- **创建时间**: 1 day ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { QualityIssueRecord } from 'shared/types';
2: import { CheckCircle, AlertTriangle, StopCircle, Info, Bug, ChevronRight, ChevronDown } from 'lucide-react';
3: import { useState } from 'react';
4: import { cn } from '@/lib/utils';
5: import { Button } from '@/components/ui/button';
7: export interface QualityIssueListProps {
8: issues: QualityIssueRecord[];
9: className?: string;
10: maxHeight?: string;
11: }
13: const SeverityIcon = ({ severity, className }: { severity: string, className?: string }) => {
14: switch (severity.toLowerCase()) {
15: case 'blocker':
16: case 'critical':
17: return <StopCircle className={cn('text-red-500 w-4 h-4', className)} />;
18: case 'major':
19: return <AlertTriangle className={cn('text-amber-500 w-4 h-4', className)} />;
20: case 'minor':
21: return <Bug className={cn('text-yellow-500 w-4 h-4', className)} />;
22: case 'info':
23: default:
24: return <Info className={cn('text-blue-500 w-4 h-4', className)} />;
25: }
26: };
28: const IssueItem = ({ issue }: { issue: QualityIssueRecord }) => {
29: const [expanded, setExpanded] = useState(false);
31: return (
32: <div className="border border-slate-200 dark:border-slate-800 rounded-md overflow-hidden bg-white dark:bg-slate-900 transition-all hover:border-slate-300 dark:hover:border-slate-700">
37: <div className="mt-0.5">
38: <SeverityIcon severity={issue.severity} />
39: </div>
40: <div className="flex-1 min-w-0">
41: <div className="flex items-center justify-between gap-2">
42: <h4 className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">
43: {issue.message}
44: </h4>
45: <div className="flex-shrink-0 flex items-center gap-1.5 text-xs font-mono text-slate-500">
46: <span className="font-medium text-slate-800 dark:text-slate-200">
47: {issue.ruleId}
48: </span>
49: </div>
50: </div>
51: <div className="mt-1 text-xs text-slate-500 dark:text-slate-400 flex items-center gap-2">
52: <span className="truncate max-w-[200px] md:max-w-xs">{issue.filePath || 'Unknown file'}</span>
53: {issue.line !== null && issue.line !== undefined && (
54: <span>Line {issue.line.toString()}</span>
55: )}
56: <span className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded capitalize text-[10px]">
57: {issue.source}
58: </span>
```

### 10.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZzg0Jx9VeL0hYeMa3Nd`
- **行号**: L335
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 335min effort
- **创建时间**: 1 day ago
- **标签**: accessibility, react

**问题代码片段**:
```
1: import { QualityIssueRecord } from 'shared/types';
2: import { CheckCircle, AlertTriangle, StopCircle, Info, Bug, ChevronRight, ChevronDown } from 'lucide-react';
3: import { useState } from 'react';
4: import { cn } from '@/lib/utils';
5: import { Button } from '@/components/ui/button';
7: export interface QualityIssueListProps {
8: issues: QualityIssueRecord[];
9: className?: string;
10: maxHeight?: string;
11: }
13: const SeverityIcon = ({ severity, className }: { severity: string, className?: string }) => {
14: switch (severity.toLowerCase()) {
15: case 'blocker':
16: case 'critical':
17: return <StopCircle className={cn('text-red-500 w-4 h-4', className)} />;
18: case 'major':
19: return <AlertTriangle className={cn('text-amber-500 w-4 h-4', className)} />;
20: case 'minor':
21: return <Bug className={cn('text-yellow-500 w-4 h-4', className)} />;
22: case 'info':
23: default:
24: return <Info className={cn('text-blue-500 w-4 h-4', className)} />;
25: }
26: };
28: const IssueItem = ({ issue }: { issue: QualityIssueRecord }) => {
29: const [expanded, setExpanded] = useState(false);
31: return (
32: <div className="border border-slate-200 dark:border-slate-800 rounded-md overflow-hidden bg-white dark:bg-slate-900 transition-all hover:border-slate-300 dark:hover:border-slate-700">
37: <div className="mt-0.5">
38: <SeverityIcon severity={issue.severity} />
39: </div>
40: <div className="flex-1 min-w-0">
41: <div className="flex items-center justify-between gap-2">
42: <h4 className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">
43: {issue.message}
44: </h4>
45: <div className="flex-shrink-0 flex items-center gap-1.5 text-xs font-mono text-slate-500">
46: <span className="font-medium text-slate-800 dark:text-slate-200">
47: {issue.ruleId}
48: </span>
49: </div>
50: </div>
51: <div className="mt-1 text-xs text-slate-500 dark:text-slate-400 flex items-center gap-2">
52: <span className="truncate max-w-[200px] md:max-w-xs">{issue.filePath || 'Unknown file'}</span>
53: {issue.line !== null && issue.line !== undefined && (
54: <span>Line {issue.line.toString()}</span>
55: )}
56: <span className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded capitalize text-[10px]">
57: {issue.source}
58: </span>
```

### 10.3 Mark the props of the component as read-only.

- **问题ID**: `AZzg0Jx9VeL0hYeMa3Ne`
- **行号**: L705
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 705min effort
- **创建时间**: 1 day ago
- **标签**: react, type-dependent

**问题代码片段**:
```
1: import { QualityIssueRecord } from 'shared/types';
2: import { CheckCircle, AlertTriangle, StopCircle, Info, Bug, ChevronRight, ChevronDown } from 'lucide-react';
3: import { useState } from 'react';
4: import { cn } from '@/lib/utils';
5: import { Button } from '@/components/ui/button';
7: export interface QualityIssueListProps {
8: issues: QualityIssueRecord[];
9: className?: string;
10: maxHeight?: string;
11: }
13: const SeverityIcon = ({ severity, className }: { severity: string, className?: string }) => {
14: switch (severity.toLowerCase()) {
15: case 'blocker':
16: case 'critical':
17: return <StopCircle className={cn('text-red-500 w-4 h-4', className)} />;
18: case 'major':
19: return <AlertTriangle className={cn('text-amber-500 w-4 h-4', className)} />;
20: case 'minor':
21: return <Bug className={cn('text-yellow-500 w-4 h-4', className)} />;
22: case 'info':
23: default:
24: return <Info className={cn('text-blue-500 w-4 h-4', className)} />;
25: }
26: };
28: const IssueItem = ({ issue }: { issue: QualityIssueRecord }) => {
29: const [expanded, setExpanded] = useState(false);
31: return (
32: <div className="border border-slate-200 dark:border-slate-800 rounded-md overflow-hidden bg-white dark:bg-slate-900 transition-all hover:border-slate-300 dark:hover:border-slate-700">
37: <div className="mt-0.5">
38: <SeverityIcon severity={issue.severity} />
39: </div>
40: <div className="flex-1 min-w-0">
41: <div className="flex items-center justify-between gap-2">
42: <h4 className="text-sm font-medium text-slate-900 dark:text-slate-100 truncate">
43: {issue.message}
44: </h4>
45: <div className="flex-shrink-0 flex items-center gap-1.5 text-xs font-mono text-slate-500">
46: <span className="font-medium text-slate-800 dark:text-slate-200">
47: {issue.ruleId}
48: </span>
49: </div>
50: </div>
51: <div className="mt-1 text-xs text-slate-500 dark:text-slate-400 flex items-center gap-2">
52: <span className="truncate max-w-[200px] md:max-w-xs">{issue.filePath || 'Unknown file'}</span>
53: {issue.line !== null && issue.line !== undefined && (
54: <span>Line {issue.line.toString()}</span>
55: )}
56: <span className="px-1.5 py-0.5 bg-slate-100 dark:bg-slate-800 rounded capitalize text-[10px]">
57: {issue.source}
58: </span>
```

---

## 11. frontend/src/components/quality/QualityReportPanel.tsx

> 该文件共有 **1** 个问题

### 11.1 Mark the props of the component as read-only.

- **问题ID**: `AZzg0J09VeL0hYeMa3Nk`
- **行号**: L155
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 155min effort
- **创建时间**: 1 day ago
- **标签**: react, type-dependent

**问题代码片段**:
```
1: import { useTerminalLatestQuality, useQualityIssues } from '@/hooks/useQualityGate';
2: import { isQualityGateAvailable } from '@/lib/apiVersionCompat';
3: import { QualityBadge } from '@/components/workflow/QualityBadge';
4: import { QualityIssueList } from './QualityIssueList';
5: import { AlertTriangle, StopCircle, Bug, Loader2, ShieldOff } from 'lucide-react';
6: import { Button } from '@/components/ui/button';
7: import { useTranslation } from 'react-i18next';
9: export interface QualityReportPanelProps {
10: terminalId: string;
11: className?: string;
12: onRefresh?: () => void;
13: }
15: export function QualityReportPanel() {
16: const { t } = useTranslation('quality');
17: const { data: latestRun, isLoading, error, refetch } = useTerminalLatestQuality(terminalId);
18: const runId = latestRun?.id;
19: const { data: issuesData, isLoading: issuesLoading } = useQualityIssues(runId);
21: if (isLoading) {
22: return (
23: <div className="flex items-center justify-center p-8 text-slate-500">
24: <Loader2 className="w-6 h-6 animate-spin mr-2" />
25: <span className="text-sm">{t('panel.loading')}</span>
26: </div>
27: );
28: }
30: // Fallback when backend doesn't support quality gate (404 / version mismatch)
31: if (error && !isQualityGateAvailable(error)) {
32: return (
33: <div className="flex flex-col items-center justify-center p-8 text-slate-400 text-sm border border-dashed rounded-md">
34: <ShieldOff className="w-8 h-8 mb-2 text-slate-300" />
35: <span>{t('panel.notAvailable', 'Quality gate not available')}</span>
36: <span className="text-xs mt-1 text-slate-300">
37: {t('panel.notAvailableHint', 'The backend does not support this feature yet.')}
38: </span>
39: </div>
40: );
41: }
43: if (error) {
44: return (
45: <div className="p-4 bg-red-50 text-red-600 rounded-md text-sm border border-red-100">
46: {t('panel.error')}
47: </div>
48: );
49: }
51: if (!isLoading && !latestRun) {
52: return (
53: <div className="text-center p-8 text-slate-500 text-sm border border-dashed rounded-md">
54: {t('panel.empty')}
55: </div>
56: );
```

---

## 12. frontend/src/components/quality/QualityTimeline.tsx

> 该文件共有 **5** 个问题

### 12.1 Mark the props of the component as read-only.

- **问题ID**: `AZzg0J0sVeL0hYeMa3Nf`
- **行号**: L105
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 105min effort
- **创建时间**: 1 day ago
- **标签**: react, type-dependent

**问题代码片段**:
```
1: import { CheckCircle, Circle, RotateCcw } from 'lucide-react';
2: import { cn } from '@/lib/utils';
3: import { QualityRun } from 'shared/types';
5: export interface QualityTimelineProps {
6: runs?: QualityRun[];
7: className?: string;
8: }
10: export function QualityTimeline() {
11: const steps = [
12: { id: 'checkpoint', label: 'Checkpoint' },
13: { id: 'analysis', label: 'Analysis' },
14: { id: 'feedback', label: 'Feedback' },
15: { id: 'passed', label: 'Passed' },
16: ];
18: let currentStep = 'checkpoint';
20: if (runs && runs.length > 0) {
21: const latestRun = runs[0];
22: if (latestRun.gateStatus === 'running' || latestRun.gateStatus === 'pending') {
23: currentStep = 'analysis';
24: } else if (latestRun.gateStatus === 'error' || latestRun.gateStatus === 'warn') {
25: currentStep = 'feedback';
26: } else if (latestRun.gateStatus === 'ok') {
27: currentStep = 'passed';
28: }
29: }
31: const getStepStatus = (stepId: string) => {
32: const currentIndex = steps.findIndex(s => s.id === currentStep);
33: const stepIndex = steps.findIndex(s => s.id === stepId);
35: if (stepIndex < currentIndex) return 'completed';
36: if (stepIndex === currentIndex) return 'current';
37: return 'pending';
38: };
40: return (
41: <div className={cn("relative", className)}>
42: <div className="absolute top-1/2 left-0 w-full h-0.5 bg-slate-100 dark:bg-slate-800 -translate-y-1/2 rounded" />
43: <div className="relative flex justify-between items-center w-full">
44: {steps.map((step) => {
45: const status = getStepStatus(step.id);
47: return (
48: <div key={step.id} className="relative z-10 flex flex-col items-center">
49: <div
50: className={cn(
51: "w-8 h-8 rounded-full flex items-center justify-center border-2 transition-colors bg-white dark:bg-slate-950",
52: status === 'completed' ? "border-green-500 text-green-500" :
56: )}
57: >
58: {status === 'completed' && <CheckCircle className="w-4 h-4" />}
59: {status === 'current' && step.id === 'feedback' && <RotateCcw className="w-4 h-4" />}
60: {status === 'current' && step.id !== 'feedback' && <Circle className="w-3 h-3 fill-current" />}
61: {status === 'pending' && <Circle className="w-3 h-3" />}
```

### 12.2 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZzg0J0sVeL0hYeMa3Ng`
- **行号**: L535
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 535min effort
- **创建时间**: 1 day ago
- **标签**: confusing

**问题代码片段**:
```
1: import { CheckCircle, Circle, RotateCcw } from 'lucide-react';
2: import { cn } from '@/lib/utils';
3: import { QualityRun } from 'shared/types';
5: export interface QualityTimelineProps {
6: runs?: QualityRun[];
7: className?: string;
8: }
10: export function QualityTimeline() {
11: const steps = [
12: { id: 'checkpoint', label: 'Checkpoint' },
13: { id: 'analysis', label: 'Analysis' },
14: { id: 'feedback', label: 'Feedback' },
15: { id: 'passed', label: 'Passed' },
16: ];
18: let currentStep = 'checkpoint';
20: if (runs && runs.length > 0) {
21: const latestRun = runs[0];
22: if (latestRun.gateStatus === 'running' || latestRun.gateStatus === 'pending') {
23: currentStep = 'analysis';
24: } else if (latestRun.gateStatus === 'error' || latestRun.gateStatus === 'warn') {
25: currentStep = 'feedback';
26: } else if (latestRun.gateStatus === 'ok') {
27: currentStep = 'passed';
28: }
29: }
31: const getStepStatus = (stepId: string) => {
32: const currentIndex = steps.findIndex(s => s.id === currentStep);
33: const stepIndex = steps.findIndex(s => s.id === stepId);
35: if (stepIndex < currentIndex) return 'completed';
36: if (stepIndex === currentIndex) return 'current';
37: return 'pending';
38: };
40: return (
41: <div className={cn("relative", className)}>
42: <div className="absolute top-1/2 left-0 w-full h-0.5 bg-slate-100 dark:bg-slate-800 -translate-y-1/2 rounded" />
43: <div className="relative flex justify-between items-center w-full">
44: {steps.map((step) => {
45: const status = getStepStatus(step.id);
47: return (
48: <div key={step.id} className="relative z-10 flex flex-col items-center">
49: <div
50: className={cn(
51: "w-8 h-8 rounded-full flex items-center justify-center border-2 transition-colors bg-white dark:bg-slate-950",
52: status === 'completed' ? "border-green-500 text-green-500" :
56: )}
57: >
58: {status === 'completed' && <CheckCircle className="w-4 h-4" />}
59: {status === 'current' && step.id === 'feedback' && <RotateCcw className="w-4 h-4" />}
60: {status === 'current' && step.id !== 'feedback' && <Circle className="w-3 h-3 fill-current" />}
61: {status === 'pending' && <Circle className="w-3 h-3" />}
```

### 12.3 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZzg0J0sVeL0hYeMa3Nh`
- **行号**: L545
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 545min effort
- **创建时间**: 1 day ago
- **标签**: confusing

**问题代码片段**:
```
1: import { CheckCircle, Circle, RotateCcw } from 'lucide-react';
2: import { cn } from '@/lib/utils';
3: import { QualityRun } from 'shared/types';
5: export interface QualityTimelineProps {
6: runs?: QualityRun[];
7: className?: string;
8: }
10: export function QualityTimeline() {
11: const steps = [
12: { id: 'checkpoint', label: 'Checkpoint' },
13: { id: 'analysis', label: 'Analysis' },
14: { id: 'feedback', label: 'Feedback' },
15: { id: 'passed', label: 'Passed' },
16: ];
18: let currentStep = 'checkpoint';
20: if (runs && runs.length > 0) {
21: const latestRun = runs[0];
22: if (latestRun.gateStatus === 'running' || latestRun.gateStatus === 'pending') {
23: currentStep = 'analysis';
24: } else if (latestRun.gateStatus === 'error' || latestRun.gateStatus === 'warn') {
25: currentStep = 'feedback';
26: } else if (latestRun.gateStatus === 'ok') {
27: currentStep = 'passed';
28: }
29: }
31: const getStepStatus = (stepId: string) => {
32: const currentIndex = steps.findIndex(s => s.id === currentStep);
33: const stepIndex = steps.findIndex(s => s.id === stepId);
35: if (stepIndex < currentIndex) return 'completed';
36: if (stepIndex === currentIndex) return 'current';
37: return 'pending';
38: };
40: return (
41: <div className={cn("relative", className)}>
42: <div className="absolute top-1/2 left-0 w-full h-0.5 bg-slate-100 dark:bg-slate-800 -translate-y-1/2 rounded" />
43: <div className="relative flex justify-between items-center w-full">
44: {steps.map((step) => {
45: const status = getStepStatus(step.id);
47: return (
48: <div key={step.id} className="relative z-10 flex flex-col items-center">
49: <div
50: className={cn(
51: "w-8 h-8 rounded-full flex items-center justify-center border-2 transition-colors bg-white dark:bg-slate-950",
52: status === 'completed' ? "border-green-500 text-green-500" :
56: )}
57: >
58: {status === 'completed' && <CheckCircle className="w-4 h-4" />}
59: {status === 'current' && step.id === 'feedback' && <RotateCcw className="w-4 h-4" />}
60: {status === 'current' && step.id !== 'feedback' && <Circle className="w-3 h-3 fill-current" />}
61: {status === 'pending' && <Circle className="w-3 h-3" />}
```

### 12.4 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZzg0J0sVeL0hYeMa3Ni`
- **行号**: L675
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 675min effort
- **创建时间**: 1 day ago
- **标签**: confusing

**问题代码片段**:
```
1: import { CheckCircle, Circle, RotateCcw } from 'lucide-react';
2: import { cn } from '@/lib/utils';
3: import { QualityRun } from 'shared/types';
5: export interface QualityTimelineProps {
6: runs?: QualityRun[];
7: className?: string;
8: }
10: export function QualityTimeline() {
11: const steps = [
12: { id: 'checkpoint', label: 'Checkpoint' },
13: { id: 'analysis', label: 'Analysis' },
14: { id: 'feedback', label: 'Feedback' },
15: { id: 'passed', label: 'Passed' },
16: ];
18: let currentStep = 'checkpoint';
20: if (runs && runs.length > 0) {
21: const latestRun = runs[0];
22: if (latestRun.gateStatus === 'running' || latestRun.gateStatus === 'pending') {
23: currentStep = 'analysis';
24: } else if (latestRun.gateStatus === 'error' || latestRun.gateStatus === 'warn') {
25: currentStep = 'feedback';
26: } else if (latestRun.gateStatus === 'ok') {
27: currentStep = 'passed';
28: }
29: }
31: const getStepStatus = (stepId: string) => {
32: const currentIndex = steps.findIndex(s => s.id === currentStep);
33: const stepIndex = steps.findIndex(s => s.id === stepId);
35: if (stepIndex < currentIndex) return 'completed';
36: if (stepIndex === currentIndex) return 'current';
37: return 'pending';
38: };
40: return (
41: <div className={cn("relative", className)}>
42: <div className="absolute top-1/2 left-0 w-full h-0.5 bg-slate-100 dark:bg-slate-800 -translate-y-1/2 rounded" />
43: <div className="relative flex justify-between items-center w-full">
44: {steps.map((step) => {
45: const status = getStepStatus(step.id);
47: return (
48: <div key={step.id} className="relative z-10 flex flex-col items-center">
49: <div
50: className={cn(
51: "w-8 h-8 rounded-full flex items-center justify-center border-2 transition-colors bg-white dark:bg-slate-950",
52: status === 'completed' ? "border-green-500 text-green-500" :
56: )}
57: >
58: {status === 'completed' && <CheckCircle className="w-4 h-4" />}
59: {status === 'current' && step.id === 'feedback' && <RotateCcw className="w-4 h-4" />}
60: {status === 'current' && step.id !== 'feedback' && <Circle className="w-3 h-3 fill-current" />}
61: {status === 'pending' && <Circle className="w-3 h-3" />}
```

### 12.5 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZzg0J0sVeL0hYeMa3Nj`
- **行号**: L685
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 685min effort
- **创建时间**: 1 day ago
- **标签**: confusing

**问题代码片段**:
```
1: import { CheckCircle, Circle, RotateCcw } from 'lucide-react';
2: import { cn } from '@/lib/utils';
3: import { QualityRun } from 'shared/types';
5: export interface QualityTimelineProps {
6: runs?: QualityRun[];
7: className?: string;
8: }
10: export function QualityTimeline() {
11: const steps = [
12: { id: 'checkpoint', label: 'Checkpoint' },
13: { id: 'analysis', label: 'Analysis' },
14: { id: 'feedback', label: 'Feedback' },
15: { id: 'passed', label: 'Passed' },
16: ];
18: let currentStep = 'checkpoint';
20: if (runs && runs.length > 0) {
21: const latestRun = runs[0];
22: if (latestRun.gateStatus === 'running' || latestRun.gateStatus === 'pending') {
23: currentStep = 'analysis';
24: } else if (latestRun.gateStatus === 'error' || latestRun.gateStatus === 'warn') {
25: currentStep = 'feedback';
26: } else if (latestRun.gateStatus === 'ok') {
27: currentStep = 'passed';
28: }
29: }
31: const getStepStatus = (stepId: string) => {
32: const currentIndex = steps.findIndex(s => s.id === currentStep);
33: const stepIndex = steps.findIndex(s => s.id === stepId);
35: if (stepIndex < currentIndex) return 'completed';
36: if (stepIndex === currentIndex) return 'current';
37: return 'pending';
38: };
40: return (
41: <div className={cn("relative", className)}>
42: <div className="absolute top-1/2 left-0 w-full h-0.5 bg-slate-100 dark:bg-slate-800 -translate-y-1/2 rounded" />
43: <div className="relative flex justify-between items-center w-full">
44: {steps.map((step) => {
45: const status = getStepStatus(step.id);
47: return (
48: <div key={step.id} className="relative z-10 flex flex-col items-center">
49: <div
50: className={cn(
51: "w-8 h-8 rounded-full flex items-center justify-center border-2 transition-colors bg-white dark:bg-slate-950",
52: status === 'completed' ? "border-green-500 text-green-500" :
56: )}
57: >
58: {status === 'completed' && <CheckCircle className="w-4 h-4" />}
59: {status === 'current' && step.id === 'feedback' && <RotateCcw className="w-4 h-4" />}
60: {status === 'current' && step.id !== 'feedback' && <Circle className="w-3 h-3 fill-current" />}
61: {status === 'pending' && <Circle className="w-3 h-3" />}
```

---

## 13. frontend/src/components/tasks/BranchSelector.tsx

> 该文件共有 **1** 个问题

### 13.1 Expected a `for-of` loop instead of a `for` loop with this simple iteration.

- **问题ID**: `AZzjW8nztUTrntRZSM18`
- **行号**: L1675
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 1675min effort
- **创建时间**: 14 hours ago
- **标签**: clumsy

**问题代码片段**:
```
1: import { useState, useMemo, useRef, useEffect, useCallback, memo } from 'react';
2: import { Virtuoso, VirtuosoHandle } from 'react-virtuoso';
3: import { useTranslation } from 'react-i18next';
4: import { Button } from '@/components/ui/button.tsx';
5: import { ArrowDown, GitBranch as GitBranchIcon, Search } from 'lucide-react';
6: import {
7: DropdownMenu,
8: DropdownMenuContent,
9: DropdownMenuItem,
10: DropdownMenuSeparator,
11: DropdownMenuTrigger,
12: } from '@/components/ui/dropdown-menu.tsx';
13: import {
14: Tooltip,
15: TooltipContent,
16: TooltipProvider,
17: TooltipTrigger,
18: } from '@/components/ui/tooltip.tsx';
19: import { Input } from '@/components/ui/input.tsx';
20: import type { GitBranch } from 'shared/types';
22: type Props = Readonly<{
23: branches: GitBranch[];
24: selectedBranch: string | null;
25: onBranchSelect: (branch: string) => void;
26: placeholder?: string;
27: className?: string;
28: excludeCurrentBranch?: boolean;
29: disabledTooltip?: string;
30: }>;
32: type RowProps = {
33: branch: GitBranch;
34: isSelected: boolean;
35: isHighlighted: boolean;
36: isDisabled: boolean;
37: onHover: () => void;
38: onSelect: () => void;
39: disabledTooltip?: string;
40: };
42: const BranchRow = memo(function BranchRow({
43: branch,
44: isSelected,
45: isHighlighted,
46: isDisabled,
47: onHover,
48: onSelect,
49: disabledTooltip,
50: }: Readonly<RowProps>) {
51: const { t } = useTranslation(['common']);
52: const classes =
53: (isSelected ? 'bg-accent text-accent-foreground ' : '') +
```

---

## 14. frontend/src/components/terminal/TerminalDebugView.tsx

> 该文件共有 **2** 个问题

### 14.1 Avoid non-native interactive elements. If using native HTML is not possible, add an appropriate role and support for tabbing, mouse, keyboard, and touch inputs to an interactive content element.

- **问题ID**: `AZzg0J4oVeL0hYeMa3No`
- **行号**: L4655
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 4655min effort
- **创建时间**: 1 day ago
- **标签**: accessibility, react

**问题代码片段**:
```
216: terminalRef.current?.clear();
217: };
219: const resetAutoStart = useCallback((terminalId: string) => {
220: autoStartedRef.current.delete(terminalId);
221: }, []);
223: const startTerminal = useCallback(async (terminalId: string, retryAfterStop = false) => {
224: // Allow multiple terminals to start in parallel
225: if (startingTerminalIdsRef.current.has(terminalId)) return;
226: startingTerminalIdsRef.current.add(terminalId);
227: // Mark as auto-started only after confirming we can start
228: autoStartedRef.current.add(terminalId);
229: try {
230: const response = await fetch(`/api/terminals/${terminalId}/start`, {
231: method: 'POST',
232: });
234: if (response.ok) {
235: console.log('Terminal started successfully');
236: // Mark this terminal as ready and clear restart flag
237: needsRestartRef.current.delete(terminalId);
238: readyTerminalIdsRef.current.add(terminalId);
239: // Note: Don't reset restart attempts here - only reset on manual restart
240: // This prevents infinite loops when API succeeds but process doesn't actually start
241: } else {
242: const error = await response.json().catch(() => null);
244: // Handle 409 Conflict by stopping first, then retrying
245: if (response.status === 409 && !retryAfterStop) {
246: console.log('Terminal conflict, stopping and retrying...');
247: startingTerminalIdsRef.current.delete(terminalId);
248: try {
249: await fetch(`/api/terminals/${terminalId}/stop`, { method: 'POST' });
250: } catch {
251: // Ignore stop errors
252: }
253: // Retry start after stop
254: return startTerminal(terminalId, true);
255: }
257: console.error('Failed to start terminal:', error);
258: resetAutoStart(terminalId);
259: // Clear ready state on failure
260: readyTerminalIdsRef.current.delete(terminalId);
261: }
262: } catch (error) {
263: console.error('Failed to start terminal:', error);
264: resetAutoStart(terminalId);
265: // Clear ready state on failure
266: readyTerminalIdsRef.current.delete(terminalId);
267: } finally {
268: startingTerminalIdsRef.current.delete(terminalId);
269: }
270: }, [resetAutoStart]);
```

### 14.2 Visible, non-interactive elements with click handlers must have at least one keyboard listener.

- **问题ID**: `AZzg0J4oVeL0hYeMa3Np`
- **行号**: L4655
- **类型**: Bug
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 4655min effort
- **创建时间**: 1 day ago
- **标签**: accessibility, react

**问题代码片段**:
```
216: terminalRef.current?.clear();
217: };
219: const resetAutoStart = useCallback((terminalId: string) => {
220: autoStartedRef.current.delete(terminalId);
221: }, []);
223: const startTerminal = useCallback(async (terminalId: string, retryAfterStop = false) => {
224: // Allow multiple terminals to start in parallel
225: if (startingTerminalIdsRef.current.has(terminalId)) return;
226: startingTerminalIdsRef.current.add(terminalId);
227: // Mark as auto-started only after confirming we can start
228: autoStartedRef.current.add(terminalId);
229: try {
230: const response = await fetch(`/api/terminals/${terminalId}/start`, {
231: method: 'POST',
232: });
234: if (response.ok) {
235: console.log('Terminal started successfully');
236: // Mark this terminal as ready and clear restart flag
237: needsRestartRef.current.delete(terminalId);
238: readyTerminalIdsRef.current.add(terminalId);
239: // Note: Don't reset restart attempts here - only reset on manual restart
240: // This prevents infinite loops when API succeeds but process doesn't actually start
241: } else {
242: const error = await response.json().catch(() => null);
244: // Handle 409 Conflict by stopping first, then retrying
245: if (response.status === 409 && !retryAfterStop) {
246: console.log('Terminal conflict, stopping and retrying...');
247: startingTerminalIdsRef.current.delete(terminalId);
248: try {
249: await fetch(`/api/terminals/${terminalId}/stop`, { method: 'POST' });
250: } catch {
251: // Ignore stop errors
252: }
253: // Retry start after stop
254: return startTerminal(terminalId, true);
255: }
257: console.error('Failed to start terminal:', error);
258: resetAutoStart(terminalId);
259: // Clear ready state on failure
260: readyTerminalIdsRef.current.delete(terminalId);
261: }
262: } catch (error) {
263: console.error('Failed to start terminal:', error);
264: resetAutoStart(terminalId);
265: // Clear ready state on failure
266: readyTerminalIdsRef.current.delete(terminalId);
267: } finally {
268: startingTerminalIdsRef.current.delete(terminalId);
269: }
270: }, [resetAutoStart]);
```

---

## 15. frontend/src/components/terminal/TerminalEmulator.tsx

> 该文件共有 **1** 个问题

### 15.1 Elements with ARIA roles must use a valid, non-abstract ARIA role.

- **问题ID**: `AZyVwepzZ9DOUQdEsGjk`
- **行号**: L3485
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Reliability
- **工作量**: 3485min effort
- **创建时间**: 1 month ago
- **标签**: accessibility, react

**问题代码片段**:
```
100: reconnectTimerRef.current = globalThis.setTimeout(() => {
101: reconnectTimerRef.current = null;
102: if (!wsUrl) {
103: return;
104: }
105: markConnecting();
106: setWsKey((k) => k + 1);
107: }, delay);
108: }, [markConnecting, wsUrl]);
110: // Expose methods via ref
111: useImperativeHandle(ref, () => ({
112: write: (data: string) => {
113: terminalRef.current?.write(data);
114: },
115: clear: () => {
116: terminalRef.current?.clear();
117: },
118: reconnect: () => {
119: // Close existing connection and trigger reconnect
120: skipNextAutoReconnectRef.current = true;
121: reconnectAttemptsRef.current = 0;
122: hasReportedTransportErrorRef.current = false;
123: clearReconnectTimer();
124: clearKeepAliveTimer();
125: if (wsUrl) {
126: markConnecting();
127: } else {
128: setConnectionState('idle');
129: setDisconnectHint(null);
130: }
131: if (wsRef.current) {
132: wsRef.current.close();
133: wsRef.current = null;
134: }
135: setWsKey((k) => k + 1);
136: },
137: }), [clearKeepAliveTimer, clearReconnectTimer, markConnecting, wsUrl]);
139: // Stable handlers
140: const handleData = useCallback((data: string) => {
141: onData?.(data);
142: if (wsRef.current?.readyState === WebSocket.OPEN) {
143: wsRef.current.send(JSON.stringify({ type: 'input', data }));
144: } else {
145: pendingInputRef.current.push(data);
146: }
147: }, [onData]);
149: const handleResize = useCallback((cols: number, rows: number) => {
150: onResize?.(cols, rows);
151: if (wsRef.current?.readyState === WebSocket.OPEN) {
152: wsRef.current.send(JSON.stringify({ type: 'resize', cols, rows }));
```

---

## 16. frontend/src/components/ui/shadcn-io/kanban.tsx

> 该文件共有 **6** 个问题

### 16.1 Remove this use of the "void" operator.

- **问题ID**: `AZzjW8c0tUTrntRZSM11`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import * as React from 'react';
2: import { cn } from '@/lib/utils';
4: interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
5: children: React.ReactNode;
6: id?: string;
7: name?: string;
8: index?: number;
9: parent?: string;
10: isOpen?: boolean;
11: forwardedRef?: React.RefObject<HTMLDivElement>;
12: dragDisabled?: boolean;
13: }
15: export function KanbanCard({
16: children,
17: className,
18: id: _id,
19: name: _name,
20: index: _index,
21: parent: _parent,
22: isOpen: _isOpen,
23: forwardedRef,
24: dragDisabled: _dragDisabled,
25: ...props
26: }: Readonly<KanbanCardProps>) {
27: // These destructured props are consumed by the drag-and-drop library at a higher level
28: // and must be extracted here to avoid passing them to the DOM element via ...props.
29: _id; _name; _index; _parent; _isOpen; _dragDisabled;
30: return (
31: <div
32: ref={forwardedRef}
33: className={cn(
34: 'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
35: className
36: )}
37: {...props}
38: >
39: {children}
40: </div>
41: );
42: }
```

### 16.2 Remove this use of the "void" operator.

- **问题ID**: `AZzjW8c0tUTrntRZSM12`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import * as React from 'react';
2: import { cn } from '@/lib/utils';
4: interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
5: children: React.ReactNode;
6: id?: string;
7: name?: string;
8: index?: number;
9: parent?: string;
10: isOpen?: boolean;
11: forwardedRef?: React.RefObject<HTMLDivElement>;
12: dragDisabled?: boolean;
13: }
15: export function KanbanCard({
16: children,
17: className,
18: id: _id,
19: name: _name,
20: index: _index,
21: parent: _parent,
22: isOpen: _isOpen,
23: forwardedRef,
24: dragDisabled: _dragDisabled,
25: ...props
26: }: Readonly<KanbanCardProps>) {
27: // These destructured props are consumed by the drag-and-drop library at a higher level
28: // and must be extracted here to avoid passing them to the DOM element via ...props.
29: _id; _name; _index; _parent; _isOpen; _dragDisabled;
30: return (
31: <div
32: ref={forwardedRef}
33: className={cn(
34: 'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
35: className
36: )}
37: {...props}
38: >
39: {children}
40: </div>
41: );
42: }
```

### 16.3 Remove this use of the "void" operator.

- **问题ID**: `AZzjW8c0tUTrntRZSM13`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import * as React from 'react';
2: import { cn } from '@/lib/utils';
4: interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
5: children: React.ReactNode;
6: id?: string;
7: name?: string;
8: index?: number;
9: parent?: string;
10: isOpen?: boolean;
11: forwardedRef?: React.RefObject<HTMLDivElement>;
12: dragDisabled?: boolean;
13: }
15: export function KanbanCard({
16: children,
17: className,
18: id: _id,
19: name: _name,
20: index: _index,
21: parent: _parent,
22: isOpen: _isOpen,
23: forwardedRef,
24: dragDisabled: _dragDisabled,
25: ...props
26: }: Readonly<KanbanCardProps>) {
27: // These destructured props are consumed by the drag-and-drop library at a higher level
28: // and must be extracted here to avoid passing them to the DOM element via ...props.
29: _id; _name; _index; _parent; _isOpen; _dragDisabled;
30: return (
31: <div
32: ref={forwardedRef}
33: className={cn(
34: 'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
35: className
36: )}
37: {...props}
38: >
39: {children}
40: </div>
41: );
42: }
```

### 16.4 Remove this use of the "void" operator.

- **问题ID**: `AZzjW8c0tUTrntRZSM14`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import * as React from 'react';
2: import { cn } from '@/lib/utils';
4: interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
5: children: React.ReactNode;
6: id?: string;
7: name?: string;
8: index?: number;
9: parent?: string;
10: isOpen?: boolean;
11: forwardedRef?: React.RefObject<HTMLDivElement>;
12: dragDisabled?: boolean;
13: }
15: export function KanbanCard({
16: children,
17: className,
18: id: _id,
19: name: _name,
20: index: _index,
21: parent: _parent,
22: isOpen: _isOpen,
23: forwardedRef,
24: dragDisabled: _dragDisabled,
25: ...props
26: }: Readonly<KanbanCardProps>) {
27: // These destructured props are consumed by the drag-and-drop library at a higher level
28: // and must be extracted here to avoid passing them to the DOM element via ...props.
29: _id; _name; _index; _parent; _isOpen; _dragDisabled;
30: return (
31: <div
32: ref={forwardedRef}
33: className={cn(
34: 'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
35: className
36: )}
37: {...props}
38: >
39: {children}
40: </div>
41: );
42: }
```

### 16.5 Remove this use of the "void" operator.

- **问题ID**: `AZzjW8c0tUTrntRZSM15`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import * as React from 'react';
2: import { cn } from '@/lib/utils';
4: interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
5: children: React.ReactNode;
6: id?: string;
7: name?: string;
8: index?: number;
9: parent?: string;
10: isOpen?: boolean;
11: forwardedRef?: React.RefObject<HTMLDivElement>;
12: dragDisabled?: boolean;
13: }
15: export function KanbanCard({
16: children,
17: className,
18: id: _id,
19: name: _name,
20: index: _index,
21: parent: _parent,
22: isOpen: _isOpen,
23: forwardedRef,
24: dragDisabled: _dragDisabled,
25: ...props
26: }: Readonly<KanbanCardProps>) {
27: // These destructured props are consumed by the drag-and-drop library at a higher level
28: // and must be extracted here to avoid passing them to the DOM element via ...props.
29: _id; _name; _index; _parent; _isOpen; _dragDisabled;
30: return (
31: <div
32: ref={forwardedRef}
33: className={cn(
34: 'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
35: className
36: )}
37: {...props}
38: >
39: {children}
40: </div>
41: );
42: }
```

### 16.6 Remove this use of the "void" operator.

- **问题ID**: `AZzjW8c0tUTrntRZSM16`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 295min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
1: import * as React from 'react';
2: import { cn } from '@/lib/utils';
4: interface KanbanCardProps extends React.HTMLAttributes<HTMLDivElement> {
5: children: React.ReactNode;
6: id?: string;
7: name?: string;
8: index?: number;
9: parent?: string;
10: isOpen?: boolean;
11: forwardedRef?: React.RefObject<HTMLDivElement>;
12: dragDisabled?: boolean;
13: }
15: export function KanbanCard({
16: children,
17: className,
18: id: _id,
19: name: _name,
20: index: _index,
21: parent: _parent,
22: isOpen: _isOpen,
23: forwardedRef,
24: dragDisabled: _dragDisabled,
25: ...props
26: }: Readonly<KanbanCardProps>) {
27: // These destructured props are consumed by the drag-and-drop library at a higher level
28: // and must be extracted here to avoid passing them to the DOM element via ...props.
29: _id; _name; _index; _parent; _isOpen; _dragDisabled;
30: return (
31: <div
32: ref={forwardedRef}
33: className={cn(
34: 'rounded-lg border bg-card p-3 shadow-sm transition-colors hover:bg-accent/50',
35: className
36: )}
37: {...props}
38: >
39: {children}
40: </div>
41: );
42: }
```

---

## 17. frontend/src/components/workflow/QualityBadge.tsx

> 该文件共有 **1** 个问题

### 17.1 "running" | "pending" | "error" | "skipped" | "ok" | "warn" is overridden by string in this union type.

- **问题ID**: `AZzhtQFpLoKQwZWpemVE`
- **行号**: L85
- **类型**: Code Smell
- **严重程度**: Minor
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 85min effort
- **创建时间**: 22 hours ago
- **标签**: redundant, type-dependent

**问题代码片段**:
```
1: import { Shield, ShieldCheck, ShieldAlert, ShieldX, Loader2 } from 'lucide-react';
2: import { StatusPill } from '@/components/ui-new/primitives/StatusPill';
3: import { useTranslation } from 'react-i18next';
5: type GateStatus = 'pending' | 'running' | 'ok' | 'warn' | 'error' | 'skipped';
7: interface QualityBadgeProps {
8: readonly gateStatus: | string;
9: readonly totalIssues?: number;
10: readonly blockingIssues?: number;
11: readonly mode?: string;
12: readonly className?: string;
13: }
15: function statusToTone(status: string) {
16: switch (status) {
17: case 'ok':
18: return 'success' as const;
19: case 'warn':
20: return 'warning' as const;
21: case 'error':
22: return 'danger' as const;
23: case 'running':
24: case 'pending':
25: return 'info' as const;
26: case 'skipped':
27: default:
28: return 'neutral' as const;
29: }
30: }
32: function statusToIcon(status: string) {
33: switch (status) {
34: case 'ok':
35: return ShieldCheck;
36: case 'warn':
37: return ShieldAlert;
38: case 'error':
39: return ShieldX;
40: case 'running':
41: case 'pending':
42: return Loader2;
43: default:
44: return Shield;
45: }
46: }
48: export function QualityBadge({
49: gateStatus,
50: blockingIssues,
51: className,
52: }: Readonly<QualityBadgeProps>) {
53: const { t } = useTranslation('quality');
55: function statusToLabel(status: string, blocking?: number) {
56: switch (status) {
```

---

## 18. frontend/src/components/workflow/steps/Step4Terminals.tsx

> 该文件共有 **1** 个问题

### 18.1 Refactor this code to not nest functions more than 4 levels deep.

- **问题ID**: `AZzjW8kctUTrntRZSM17`
- **行号**: L19520
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Adaptability
- **影响**: Maintainability
- **工作量**: 19520min effort
- **创建时间**: 14 hours ago
- **标签**: brain-overload

**问题代码片段**:
```
123: export const Step4Terminals: React.FC<Step4TerminalsProps> = ({
124: config,
125: errors,
126: onUpdate,
127: onError,
128: }) Nesting +1 {
129: const { notifyError } = useErrorNotification({ onError, context: 'Step4Terminals' });
130: const { t } = useTranslation('workflow');
131: const [currentTaskIndex, setCurrentTaskIndex] = useState(0);
132: const [cliTypes, setCliTypes] = useState<CliType[]>([]);
133: const [isLoading, setIsLoading] = useState(true);
150: setCurrentTaskIndex(config.tasks.length - 1);
151: }
152: }, [currentTaskIndex, config.tasks.length]);
154: // Initialize/normalize terminals for all tasks
155: useEffect(() Nesting +1 {
156: if (!hasTasks) {
157: return;
158: }
160: // Helper: Create normalized terminal config
185: // Helper: Normalize terminals for a single task
186: const normalizeTerminalsForTask = (
187: task: { id: string; terminalCount: number },
188: existingTerminals: TerminalConfig[]
189: ): TerminalConfig[] Nesting +1 {
190: const sortedExisting = existingTerminals
191: .filter((terminal) => terminal.taskId === task.id)
192: .sort((a, b) => a.orderIndex - b.orderIndex);
194: return Array.from({ length: task.terminalCount }, (_, orderIndex) Nesting +1 {
195: const byOrderIndex = sortedExisting.find((terminal) terminal.orderIndex === orderIndex);
196: return createNormalizedTerminal(task, orderIndex, byOrderIndex);
197: });
198: };
200: const normalizedTerminals: TerminalConfig[] = config.tasks.flatMap((task) =>
201: normalizeTerminalsForTask(task, config.terminals)
202: );
204: if (!terminalConfigListEquals(config.terminals, normalizedTerminals)) {
```

---

## 19. frontend/src/pages/settings/FeishuSettings.tsx

> 该文件共有 **1** 个问题

### 19.1 Extract this nested ternary operation into an independent statement.

- **问题ID**: `AZzl1fG1M0YAVBIOcxHB`
- **行号**: L1355
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 1355min effort
- **创建时间**: 2 hours ago
- **标签**: confusing

**问题代码片段**:
```
1: import { useState, useEffect, useCallback } from 'react';
2: import { useTranslation } from 'react-i18next';
3: import { RefreshCw, CheckCircle2, XCircle, Loader2 } from 'lucide-react';
4: import { feishuApi } from '@/lib/api';
5: import {
6: Card,
7: CardContent,
8: CardDescription,
9: CardHeader,
10: CardTitle,
11: } from '@/components/ui/card';
12: import { Button } from '@/components/ui/button';
13: import { Input } from '@/components/ui/input';
14: import { Label } from '@/components/ui/label';
15: import { Alert, AlertDescription } from '@/components/ui/alert';
17: type FeishuStatus = Awaited<ReturnType<typeof feishuApi.getStatus>>;
19: export function FeishuSettings() {
20: const { t } = useTranslation(['settings']);
22: const [status, setStatus] = useState<FeishuStatus | null>(null);
23: const [loading, setLoading] = useState(true);
24: const [saving, setSaving] = useState(false);
25: const [reconnecting, setReconnecting] = useState(false);
26: const [error, setError] = useState<string | null>(null);
27: const [success, setSuccess] = useState<string | null>(null);
29: const [appId, setAppId] = useState('');
30: const [appSecret, setAppSecret] = useState('');
31: const [tenantKey, setTenantKey] = useState('');
32: const [baseUrl, setBaseUrl] = useState('https://open.feishu.cn');
34: const fetchStatus = useCallback(async () => {
35: try {
36: setLoading(true);
37: const data = await feishuApi.getStatus();
38: setStatus(data);
39: if (data.configSummary) {
40: setAppId(data.configSummary.appId);
41: setBaseUrl(data.configSummary.baseUrl);
42: setTenantKey(data.configSummary.tenantKey || '');
43: }
44: } catch {
45: setError(t('settings.feishu.loadError'));
46: } finally {
47: setLoading(false);
48: }
49: }, [t]);
51: useEffect(() => {
52: fetchStatus();
53: }, [fetchStatus]);
55: const handleSave = async () => {
56: if (!appId.trim() || !appSecret.trim()) {
57: setError(t('settings.feishu.form.requiredFields'));
```

---

## 20. frontend/src/stores/wizardStore.ts

> 该文件共有 **1** 个问题

### 20.1 Remove this use of the "void" operator.

- **问题ID**: `AZzjW9A6tUTrntRZSM19`
- **行号**: L2975
- **类型**: Code Smell
- **严重程度**: Critical
- **属性**: Intentionality
- **影响**: Maintainability
- **工作量**: 2975min effort
- **创建时间**: 14 hours ago
- **标签**: confusing, type-dependent

**问题代码片段**:
```
48: addModel: (model: ModelConfig) => void;
49: updateModel: (id: string, updates: Partial<ModelConfig>) => void;
50: removeModel: (id: string) => void;
52: // Terminal management
53: setTerminals: (terminals: TerminalConfig[]) => void;
54: addTerminal: (terminal: TerminalConfig) => void;
55: updateTerminal: (id: string, updates: Partial<TerminalConfig>) => void;
56: removeTerminal: (id: string) => void;
58: // Validation
59: setErrors: (errors: Record<string, string>) => void;
60: setError: (key: string, message: string) => void;
61: clearError: (key: string) => void;
62: clearAllErrors: () => void;
63: hasErrors: () => boolean;
65: // Submission
66: setSubmitting: (submitting: boolean) => void;
68: // Reset
69: reset: () => void;
70: }
72: const TOTAL_STEPS = 7; // Steps 0-6
74: export const useWizardStore = create<WizardStoreState>((set, get) => ({
75: // Initial state
76: currentStep: 0,
77: config: getDefaultWizardConfig(),
78: errors: {},
79: isDirty: false,
80: isSubmitting: false,
82: // Navigation
83: setStep: (step) => {
84: if (step >= 0 && step < TOTAL_STEPS) {
85: set({ currentStep: step });
86: }
87: },
89: nextStep: () => {
90: const { currentStep } = get();
91: if (currentStep < TOTAL_STEPS - 1) {
92: set({ currentStep: currentStep + 1 });
93: }
94: },
96: prevStep: () => {
97: const { currentStep } = get();
98: if (currentStep > 0) {
99: set({ currentStep: currentStep - 1 });
100: }
101: },
103: canGoNext: () => {
104: const { currentStep, errors } = get();
105: return currentStep < TOTAL_STEPS - 1 && Object.keys(errors).length === 0;
106: },
108: canGoPrev: () => {
```

---

## 21. scripts/quality/run-branch-gate.sh

> 该文件共有 **1** 个问题

### 21.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhqvhjBidydR36xX`
- **行号**: L552
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 552min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/bin/bash
2: set -euo pipefail
4: SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
5: PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
7: BRANCH=""
9: while [[ $# -gt 0 ]]; do
10: case "$1" in
11: --branch)
12: BRANCH="$2"
13: shift 2
14: ;;
15: *)
16: echo "Unknown argument: $1"
17: echo "Usage: $0 [--branch <branch>]"
18: exit 1
19: ;;
20: esac
21: done
23: cd "$PROJECT_ROOT"
25: # Default to current branch
26: if [[ -z "$BRANCH" ]]; then
27: BRANCH="$(git rev-parse --abbrev-ref HEAD)"
28: fi
30: echo "=== GitCortex Branch Quality Gate ==="
31: echo "Project root: $PROJECT_ROOT"
32: echo "Branch: $BRANCH"
33: echo ""
35: # Compute changed files vs main
36: CHANGED_FILES="$(git diff --name-only "main...$BRANCH" | tr '\n' ',' | sed 's/,$//')"
38: if [[ -z "$CHANGED_FILES" ]]; then
39: echo "No changed files detected between main and $BRANCH."
40: echo "Branch quality gate passed (nothing to check)."
41: exit 0
42: fi
44: echo "Changed files: $CHANGED_FILES"
45: echo ""
47: cargo run --package quality -- \
48: --tier branch \
49: --config quality/quality-gate.yaml \
50: --working-dir "$PROJECT_ROOT" \
51: --changed-files "$CHANGED_FILES"
53: EXIT_CODE=$?
55: if $EXIT_CODE -eq 0 ]; then
56: echo ""
57: echo "Branch quality gate passed."
58: else
59: echo ""
60: echo "Branch quality gate failed."
61: fi
63: exit $EXIT_CODE
```

---

## 22. scripts/quality/run-quality-gate.sh

> 该文件共有 **1** 个问题

### 22.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhrIhjBidydR36xY`
- **行号**: L532
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 532min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/bin/bash
2: set -euo pipefail
4: SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
5: PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
7: TIER="${1:-repo}"
8: MODE="${2:-shadow}"
9: shift 2 2>/dev/null || true
11: INCLUDE_BASELINE=false
12: INCLUDE_SECURITY=false
14: while [[ $# -gt 0 ]]; do
15: case "$1" in
16: --include-baseline)
17: INCLUDE_BASELINE=true
18: shift
19: ;;
20: --include-security)
21: INCLUDE_SECURITY=true
22: shift
23: ;;
24: --all)
25: INCLUDE_BASELINE=true
26: INCLUDE_SECURITY=true
27: shift
28: ;;
29: *)
30: echo "Unknown flag: $1"
31: echo "Usage: $0 [tier] [mode] [--include-baseline] [--include-security] [--all]"
32: exit 1
33: ;;
34: esac
35: done
37: echo "=== GitCortex Quality Gate ==="
38: echo "Project root: $PROJECT_ROOT"
39: echo "Tier: $TIER"
40: echo "Mode: $MODE"
41: echo ""
43: cd "$PROJECT_ROOT"
45: # Run the quality engine via cargo
46: cargo run --package quality -- \
47: --project-root "$PROJECT_ROOT" \
48: --tier "$TIER" \
49: --mode "$MODE"
51: EXIT_CODE=$?
53: if $EXIT_CODE -ne 0 ]; then
54: echo ""
55: echo "Quality gate failed with exit code $EXIT_CODE."
56: exit $EXIT_CODE
57: fi
59: echo ""
60: echo "Quality gate completed successfully."
```

---

## 23. scripts/quality/run-sonar-scanner.sh

> 该文件共有 **1** 个问题

### 23.1 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzg0KMIVeL0hYeMa3Nr`
- **行号**: L82
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 82min effort
- **创建时间**: 1 day ago
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/bin/bash
2: set -e
4: SCANNER_VERSION="5.0.1.3006"
5: SCANNER_DIR="$(cd "$(dirname "$0")/../../quality/sonar" && pwd)/scanner"
6: SCANNER_BIN="$SCANNER_DIR/sonar-scanner-$SCANNER_VERSION-linux/bin/sonar-scanner"
8: if ! -f "$SCANNER_BIN" ]; then
9: echo "Downloading SonarScanner..."
10: mkdir -p "$SCANNER_DIR"
11: curl -sSLo "$SCANNER_DIR/sonar-scanner.zip" "https://binaries.sonarsource.com/Distribution/sonar-scanner-cli/sonar-scanner-cli-$SCANNER_VERSION-linux.zip"
12: unzip -q -o "$SCANNER_DIR/sonar-scanner.zip" -d "$SCANNER_DIR"
13: rm "$SCANNER_DIR/sonar-scanner.zip"
14: fi
16: echo "Running SonarScanner..."
17: "$SCANNER_BIN" -D"sonar.projectBaseDir=$(cd "$(dirname "$0")/../.." && pwd)" "$@"
```

---

## 24. scripts/quality/run-terminal-gate.sh

> 该文件共有 **3** 个问题

### 24.1 Redirect this error message to stderr (>&2).

- **问题ID**: `AZzmVhrlhjBidydR36xZ`
- **行号**: L295
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 295min effort
- **标签**: output, posix, ...

**问题代码片段**:
```
1: #!/bin/bash
2: set -euo pipefail
4: SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
5: PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
7: WORKING_DIR=""
8: CHANGED_FILES=""
10: while [[ $# -gt 0 ]]; do
11: case "$1" in
12: --working-dir)
13: WORKING_DIR="$2"
14: shift 2
15: ;;
16: --changed-files)
17: CHANGED_FILES="$2"
18: shift 2
19: ;;
20: *)
21: echo "Unknown argument: $1"
22: echo "Usage: $0 --working-dir <dir> --changed-files <files>"
23: exit 1
24: ;;
25: esac
26: done
28: if [[ -z "$WORKING_DIR" ]]; then
30: exit 1
31: fi
33: if [[ -z "$CHANGED_FILES" ]]; then
35: exit 1
36: fi
38: echo "=== GitCortex Terminal Quality Gate ==="
39: echo "Project root: $PROJECT_ROOT"
40: echo "Working dir: $WORKING_DIR"
41: echo "Changed files: $CHANGED_FILES"
42: echo ""
44: cd "$PROJECT_ROOT"
46: cargo run --package quality -- \
47: --tier terminal \
48: --config quality/quality-gate.yaml \
49: --working-dir "$WORKING_DIR" \
50: --changed-files "$CHANGED_FILES"
52: EXIT_CODE=$?
54: if $EXIT_CODE -eq 0 ]; then
55: echo ""
56: echo "Terminal quality gate passed."
57: else
58: echo ""
59: echo "Terminal quality gate failed."
60: fi
62: exit $EXIT_CODE
```

### 24.2 Redirect this error message to stderr (>&2).

- **问题ID**: `AZzmVhrlhjBidydR36xa`
- **行号**: L345
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Maintainability
- **工作量**: 345min effort
- **标签**: output, posix, ...

**问题代码片段**:
```
1: #!/bin/bash
2: set -euo pipefail
4: SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
5: PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
7: WORKING_DIR=""
8: CHANGED_FILES=""
10: while [[ $# -gt 0 ]]; do
11: case "$1" in
12: --working-dir)
13: WORKING_DIR="$2"
14: shift 2
15: ;;
16: --changed-files)
17: CHANGED_FILES="$2"
18: shift 2
19: ;;
20: *)
21: echo "Unknown argument: $1"
22: echo "Usage: $0 --working-dir <dir> --changed-files <files>"
23: exit 1
24: ;;
25: esac
26: done
28: if [[ -z "$WORKING_DIR" ]]; then
30: exit 1
31: fi
33: if [[ -z "$CHANGED_FILES" ]]; then
35: exit 1
36: fi
38: echo "=== GitCortex Terminal Quality Gate ==="
39: echo "Project root: $PROJECT_ROOT"
40: echo "Working dir: $WORKING_DIR"
41: echo "Changed files: $CHANGED_FILES"
42: echo ""
44: cd "$PROJECT_ROOT"
46: cargo run --package quality -- \
47: --tier terminal \
48: --config quality/quality-gate.yaml \
49: --working-dir "$WORKING_DIR" \
50: --changed-files "$CHANGED_FILES"
52: EXIT_CODE=$?
54: if $EXIT_CODE -eq 0 ]; then
55: echo ""
56: echo "Terminal quality gate passed."
57: else
58: echo ""
59: echo "Terminal quality gate failed."
60: fi
62: exit $EXIT_CODE
```

### 24.3 Use '[[' instead of '[' for conditional tests. The '[[' construct is safer and more feature-rich.

- **问题ID**: `AZzmVhrlhjBidydR36xb`
- **行号**: L542
- **类型**: Code Smell
- **严重程度**: Major
- **属性**: Consistency
- **影响**: Reliability
- **工作量**: 542min effort
- **标签**: bash, best-practices, ...

**问题代码片段**:
```
1: #!/bin/bash
2: set -euo pipefail
4: SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
5: PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
7: WORKING_DIR=""
8: CHANGED_FILES=""
10: while [[ $# -gt 0 ]]; do
11: case "$1" in
12: --working-dir)
13: WORKING_DIR="$2"
14: shift 2
15: ;;
16: --changed-files)
17: CHANGED_FILES="$2"
18: shift 2
19: ;;
20: *)
21: echo "Unknown argument: $1"
22: echo "Usage: $0 --working-dir <dir> --changed-files <files>"
23: exit 1
24: ;;
25: esac
26: done
28: if [[ -z "$WORKING_DIR" ]]; then
30: exit 1
31: fi
33: if [[ -z "$CHANGED_FILES" ]]; then
35: exit 1
36: fi
38: echo "=== GitCortex Terminal Quality Gate ==="
39: echo "Project root: $PROJECT_ROOT"
40: echo "Working dir: $WORKING_DIR"
41: echo "Changed files: $CHANGED_FILES"
42: echo ""
44: cd "$PROJECT_ROOT"
46: cargo run --package quality -- \
47: --tier terminal \
48: --config quality/quality-gate.yaml \
49: --working-dir "$WORKING_DIR" \
50: --changed-files "$CHANGED_FILES"
52: EXIT_CODE=$?
54: if $EXIT_CODE -eq 0 ]; then
55: echo ""
56: echo "Terminal quality gate passed."
57: else
58: echo ""
59: echo "Terminal quality gate failed."
60: fi
62: exit $EXIT_CODE
```

---



---

# SonarCloud 重复代码报告

**生成时间**: 2026/03/13 17:11
**项目**: huanchong-99_GitCortex
**问题文件总数**: 10
**重复行总数**: 258
**重复块总数**: 63

---

## 统计信息

### 重复率分布

- **严重 (≥50%)**: 2 个文件
- **较高 (30-50%)**: 2 个文件
- **中等 (10-30%)**: 2 个文件
- **轻微 (<10%)**: 4 个文件

---

## 重复文件列表（按路径分组）

## 1. crates/db/migrations

> 该目录共有 **1** 个重复文件

### 1.1 20250716143725_add_default_templates.sql

- **路径**: `crates/db/migrations/20250716143725_add_default_templates.sql`
- **重复率**: 94.5%
- **重复行数**: 103 行
- **重复块数**: 26 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Acrates%2Fdb%2Fmigrations%2F20250716143725_add_default_templates.sql)

---

## 2. frontend/src/components/quality/__tests__

> 该目录共有 **1** 个重复文件

### 2.1 QualityIssueList.test.tsx

- **路径**: `frontend/src/components/quality/__tests__/QualityIssueList.test.tsx`
- **重复率**: 30.4%
- **重复行数**: 34 行
- **重复块数**: 2 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fcomponents%2Fquality%2F__tests__%2FQualityIssueList.test.tsx)

---

## 3. frontend/src/components/terminal

> 该目录共有 **1** 个重复文件

### 3.1 TerminalDebugView.test.tsx

- **路径**: `frontend/src/components/terminal/TerminalDebugView.test.tsx`
- **重复率**: 1.9%
- **重复行数**: 2 行
- **重复块数**: 2 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fcomponents%2Fterminal%2FTerminalDebugView.test.tsx)

---

## 4. frontend/src/components/workflow/steps

> 该目录共有 **2** 个重复文件

### 4.1 Step6Advanced.test.tsx

- **路径**: `frontend/src/components/workflow/steps/Step6Advanced.test.tsx`
- **重复率**: 23.8%
- **重复行数**: 5 行
- **重复块数**: 5 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fcomponents%2Fworkflow%2Fsteps%2FStep6Advanced.test.tsx)

### 4.2 Step4Terminals.test.tsx

- **路径**: `frontend/src/components/workflow/steps/Step4Terminals.test.tsx`
- **重复率**: 4.0%
- **重复行数**: 2 行
- **重复块数**: 1 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fcomponents%2Fworkflow%2Fsteps%2FStep4Terminals.test.tsx)

---

## 5. frontend/src/pages

> 该目录共有 **5** 个重复文件

### 5.1 SlashCommands.test.tsx

- **路径**: `frontend/src/pages/SlashCommands.test.tsx`
- **重复率**: 72.7%
- **重复行数**: 8 行
- **重复块数**: 5 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fpages%2FSlashCommands.test.tsx)

### 5.2 SlashCommands.e2e.test.tsx

- **路径**: `frontend/src/pages/SlashCommands.e2e.test.tsx`
- **重复率**: 37.7%
- **重复行数**: 57 行
- **重复块数**: 16 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fpages%2FSlashCommands.e2e.test.tsx)

### 5.3 Board.tsx

- **路径**: `frontend/src/pages/Board.tsx`
- **重复率**: 28.0%
- **重复行数**: 14 行
- **重复块数**: 1 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fpages%2FBoard.tsx)

### 5.4 Workflows.test.tsx

- **路径**: `frontend/src/pages/Workflows.test.tsx`
- **重复率**: 6.9%
- **重复行数**: 19 行
- **重复块数**: 4 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fpages%2FWorkflows.test.tsx)

### 5.5 Workflows.tsx

- **路径**: `frontend/src/pages/Workflows.tsx`
- **重复率**: 1.4%
- **重复行数**: 14 行
- **重复块数**: 1 个
- **SonarCloud 链接**: [查看详情](https://sonarcloud.io/component_measures?id=huanchong-99_GitCortex&metric=new_duplicated_lines_density&selected=huanchong-99_GitCortex%3Afrontend%2Fsrc%2Fpages%2FWorkflows.tsx)

---



---

# SonarCloud 安全热点报告

**生成时间**: 2026/03/13 17:11
**项目**: huanchong-99_GitCortex
**安全热点总数**: 6

---

## 统计信息

### 按审核优先级分布

| 优先级 | 数量 |
|--------|------|
| High | 0 |
| Medium | 0 |
| Low | 6 |

### 按类别分布

- **Others**: 6 个

---

## 安全热点列表

### 🟢 Low 优先级 (6 个)

#### 1. Use full commit SHA hash for this dependency.

| 属性 | 值 |
|------|----|
| **文件路径** | `.github/workflows/ci-basic.yml` |
| **规则ID** | [githubactions:S7637](https://sonarcloud.io/organizations/huanchong-99/rules?open=githubactions%3AS7637&rule_key=githubactions%3AS7637) |
| **类别** | Others |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_GitCortex) |

#### 2. Use full commit SHA hash for this dependency.

| 属性 | 值 |
|------|----|
| **文件路径** | `.github/workflows/ci-basic.yml` |
| **规则ID** | [githubactions:S7637](https://sonarcloud.io/organizations/huanchong-99/rules?open=githubactions%3AS7637&rule_key=githubactions%3AS7637) |
| **类别** | Others |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_GitCortex) |

#### 3. Use full commit SHA hash for this dependency.

| 属性 | 值 |
|------|----|
| **文件路径** | `.github/workflows/ci-docker.yml` |
| **规则ID** | [githubactions:S7637](https://sonarcloud.io/organizations/huanchong-99/rules?open=githubactions%3AS7637&rule_key=githubactions%3AS7637) |
| **类别** | Others |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_GitCortex) |

#### 4. Use full commit SHA hash for this dependency.

| 属性 | 值 |
|------|----|
| **文件路径** | `.github/workflows/ci-docker.yml` |
| **规则ID** | [githubactions:S7637](https://sonarcloud.io/organizations/huanchong-99/rules?open=githubactions%3AS7637&rule_key=githubactions%3AS7637) |
| **类别** | Others |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_GitCortex) |

#### 5. Use full commit SHA hash for this dependency.

| 属性 | 值 |
|------|----|
| **文件路径** | `.github/workflows/ci-quality.yml` |
| **规则ID** | [githubactions:S7637](https://sonarcloud.io/organizations/huanchong-99/rules?open=githubactions%3AS7637&rule_key=githubactions%3AS7637) |
| **类别** | Others |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_GitCortex) |

#### 6. Use full commit SHA hash for this dependency.

| 属性 | 值 |
|------|----|
| **文件路径** | `.github/workflows/ci-quality.yml` |
| **规则ID** | [githubactions:S7637](https://sonarcloud.io/organizations/huanchong-99/rules?open=githubactions%3AS7637&rule_key=githubactions%3AS7637) |
| **类别** | Others |
| **状态** | To Review |
| **SonarCloud** | [查看详情](https://sonarcloud.io/project/security_hotspots?id=huanchong-99_GitCortex) |

---
启用team模式，拉起多个Agent并行修复（每一个都是全栈开发），推送
