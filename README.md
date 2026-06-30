<p align="center">
  <img src="installer/assets/solodawn.ico" alt="SoloDawn" width="120" />
</p>

<h1 align="center">SoloDawn</h1>

<p align="center">
  <strong>Give it one sentence. It builds the entire project. You just wait for delivery.</strong>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
  &nbsp;·&nbsp;
  <a href="https://linux.do/">Community</a>
</p>

---

## What Is SoloDawn?

SoloDawn's ultimate design goal is to **complete complex, production-grade products through a simple conversation on a social platform** — not toy demos, but real, complex, production-ready software.

> **One-liner:** Whether you're a programmer or not, just describe what you need. Everything else is automatic.

---

## The Problem We Solve

There's a fundamental contradiction in AI-assisted coding today:

**For programmers** — you use vibe coding, but you still have to set up workflows, configure skills, wire up MCP servers, write plans, organize docs… a mountain of prerequisite work before you get satisfying results. At its core, the human is still the driver.

**For non-programmers** — say, a project manager — they can't even translate a requirement into a technical spec. The reality is: AI can only boost your productivity if you already know how to do the work yourself.

**SoloDawn solves both problems:**

| Who you are | What you do | What happens |
|---|---|---|
| **Programmer** | Throw in the task goal | Fully automatic development. No commands to issue, no "Continue" to click. Just wait for delivery. Your existing workflows, skills, MCP servers? All compatible. Zero migration cost. |
| **Non-programmer** | Describe what you want in plain language | The system proactively asks follow-up questions in plain language until everything is clear, then automatically generates a formal technical spec on the backend. After that — same deal — just wait for delivery. |

**The best partner for a one-person company.** With this project, you essentially have an entire professional development team.

---

## Two Work Modes

### 1. Manual Workflow (Advanced)

For power users who want full control:

- Choose how many terminals to use and their roles
- Select the model for each terminal
- Configure which skills, MCP servers, and slash commands each terminal uses
- Full customization over every aspect of the workflow graph

### 2. Orchestrated Workspace

The orchestrated workspace has two sub-modes:

#### Direct Execution Mode
For technical users — provide a detailed, specific requirement and the system executes it directly. No questions asked.

#### Guided Conversation Mode
For non-technical users — provide a vague idea and the system will:
1. **Proactively ask follow-up questions in plain language** until the requirement is fully understood
2. **Automatically generate a formal technical specification** on the backend
3. **The upper-level orchestrator Agent takes over**, autonomously commanding all terminals until the product is delivered

---

## Built-in Quality Gates — Solving AI Hallucination at Scale

AI-generated code has a quality problem. SoloDawn addresses this head-on with a **three-layer quality gate system** that maximally mitigates AI hallucination in code output:

| Gate | Trigger | Scope |
|------|---------|-------|
| **Terminal Gate** | Every checkpoint commit | Changed files only — cargo check, clippy, tsc, tests, secret detection |
| **Branch Gate** | Last terminal in a task completes | Full task branch — all checks + lint + coverage + complexity |
| **Repo Gate** | Before merge to main / CI | Full repo — all checks + SonarQube analysis + security scan |

**Four enforcement modes:** `off` → `shadow` → `warn` → `enforce`

When a quality gate fails, structured fix instructions are automatically sent back to the same terminal. The terminal fixes the issues and re-commits — no human intervention required. This creates a **self-healing development loop** that catches and corrects hallucination-induced errors before they reach your codebase.

---

## Core Design Philosophy

- **Upper-layer orchestration, not code generation.** The orchestrator Agent never writes code — it commands the best professional AI CLIs (Claude Code, Gemini CLI, Codex, Amp, Cursor Agent, etc.) to do the work.
- **Non-invasive by design.** SoloDawn doesn't replace any CLI, modify any config, or define new tools. It inherits the full native ecosystem of every CLI — all slash commands, plugins, skills, and MCP servers work unchanged. Your existing setup? Zero migration cost.
- **Git-driven event loop.** The orchestrator only consumes LLM tokens when a Git commit event occurs. Between events, it sleeps at zero cost — saving 98%+ tokens compared to polling.

---

## Architecture

```
           ┌──────────────────────────────────────────────┐
           │        Orchestrator Agent (LLM-driven)       │
           │      Dispatches · Monitors · Merges          │
           └─────────────────────┬────────────────────────┘
                                 │
            ┌────────────────────┼────────────────────┐
            ▼                    ▼                    ▼
   ┌────────────────┐  ┌────────────────┐  ┌────────────────┐
   │    Task 1      │  │    Task 2      │  │    Task 3      │
   │  branch: auth  │  │  branch: i18n  │  │  branch: theme │
   │                │  │                │  │                │
   │  T1 → T2 → T3 │  │  TA → TB      │  │  TX → TY      │
   │   (serial +    │  │   (serial +   │  │   (serial +   │
   │  quality gate) │  │  quality gate) │  │  quality gate) │
   └────────────────┘  └────────────────┘  └────────────────┘
            │                    │                    │
            └────────────────────┼────────────────────┘
                                 ▼
                      Quality Gate Check
                                 ▼
                        Auto-Merge → main
```

**Three-Layer Execution Model:**

- **Workflow** → The orchestrator Agent manages the entire lifecycle
- **Task** → Independent Git branch, runs in parallel with other tasks
- **Terminal** → A native AI CLI process (PTY), runs serially within its task, gated by quality checks

**Key Components:**

| Component | Role |
|---|---|
| `OrchestratorAgent` | LLM-driven decision core: dispatches terminals, parses Git events, routes review/fix cycles |
| `OrchestratorRuntime` | Workflow lifecycle management, slot reservation, crash recovery |
| `QualityGateEngine` | Three-layer verification engine (terminal/branch/repo) with configurable enforcement modes |
| `MessageBus` | Event routing across all modules (workflow-scoped topics) |
| `TerminalLauncher` | Spawns native PTY processes with per-terminal environment isolation |
| `GitWatcher` | Detects Git commits → publishes events → wakes the orchestrator |
| `ResilientLLMClient` | Multi-provider round-robin with 5-failure circuit breaker and 60s probe recovery |
| `MergeCoordinator` | Centralized merge handling with conflict detection and partial-failure tracking |
| `ChatConnector` | Unified outbound messaging trait (Telegram, Feishu/Lark) |

---

## Supported AI CLIs

| CLI | Status | Model Switching | MCP Config |
|---|---|---|---|
| Claude Code | ✅ Supported | ✅ Via CC-Switch | Passthrough |
| Gemini CLI | ✅ Supported | ✅ Via CC-Switch | Gemini adapter |
| Codex | ✅ Supported | ✅ Via CC-Switch | Codex adapter |
| Amp | ✅ Supported | — | Passthrough |
| Cursor Agent | ✅ Supported | — | Cursor adapter |
| Qwen Code | ✅ Supported | — | — |
| GitHub Copilot | ✅ Supported | — | Copilot adapter |
| Droid | ✅ Supported | — | Passthrough |
| Opencode | ✅ Supported | — | Opencode adapter |

Any CLI that runs in a terminal and supports slash commands can be integrated.

### Claude Code: No-`-p` Interactive Transport & Billing Guarantee

Every Claude Code run in SoloDawn — initial requests, follow-ups, **and** reviews —
goes through **interactive Claude Code (no `-p`/`--print`)**, the same way you run
`claude` by hand in a terminal. The transport tails the on-disk session transcript
JSONL instead of consuming a `--print` stream. This exists for one reason: **billing
correctness.**

| Auth mode | How it's detected | What it bills | How it's wired |
|---|---|---|---|
| **Native (subscription)** | no stored API key | **only** your Pro/Max plan quota — never the Agent SDK credit | OAuth `~/.claude/.credentials.json` copied into an isolated home; billing env vars scrubbed |
| **Official key** | API key, no custom base URL | the key's pay-as-you-go account | `ANTHROPIC_API_KEY` |
| **Relay** | API key **and** custom base URL | the relay endpoint | `ANTHROPIC_AUTH_TOKEN` + `ANTHROPIC_BASE_URL` |

- **Subscription users consume ONLY their plan quota (Pro/Max) and NEVER the Agent
  SDK pay-as-you-go credit.** The interactive transport is the only thing that makes
  this guarantee hold; `-p` would draw from the SDK credit pool.
- The credential precedence mirrors the legacy `-p` path exactly — *which* credential
  you get is unchanged; only the transport changes.
- **`-p` is a dormant fallback.** Set `SOLODAWN_NO_POOL=1` to opt back into the proven
  `-p` path (e.g. for debugging); it accepts the pool draw and is off by default.
- **Tier-2 interactive approvals** (auto-answering Claude's per-tool permission dialog
  over the PTY) are **off by default** and gated behind
  `SOLODAWN_INTERACTIVE_APPROVALS_TIER2=1`. Unset, the default tier-1 path is untouched.

> Note: native subscription and official-key modes are covered by unit/argv-env tests
> plus a live re-probe at startup. Full live end-to-end coverage for **relay** and
> **api-key** modes requires real credentials and is a manual check.

---

## Features

### Orchestration & Execution
- ✅ Upper-layer orchestrator Agent commanding full workflow lifecycle
- ✅ Two work modes: Manual Workflow (DIY) and Orchestrated Workspace (Agent-Planned)
- ✅ Orchestrated Workspace sub-modes: Direct Execution and Guided Conversation
- ✅ Intelligent requirement assessment — vague inputs trigger plain-language follow-ups; precise inputs skip to execution
- ✅ Multi-task parallel execution (5–10 tasks simultaneously)
- ✅ WorkspacePlanning multi-turn LLM conversation for project scoping
- ✅ Planning Draft lifecycle: gathering → spec_ready → confirmed → materialized
- ✅ Cross-terminal context handoff (previous terminal's work passed to the next)
- ✅ Automatic branch merging with conflict auto-resolution

### Quality & Reliability
- ✅ **Three-layer Quality Gate System** (Terminal → Branch → Repo) to combat AI hallucination
- ✅ Built-in rule engine (runs without SonarQube), with optional SonarQube integration
- ✅ Four enforcement modes: off / shadow / warn / enforce
- ✅ Self-healing loop: failed gates → structured fix instructions → terminal auto-corrects → re-check
- ✅ Policy snapshots and issue tracking per terminal and per workflow
- ✅ Secret detection to prevent credential leaks
- ✅ Cyclomatic complexity and code duplication checks
- ✅ LLM fault tolerance with graceful degradation
- ✅ State persistence with crash recovery
- ✅ Multi-provider circuit breaker with automatic failover

### CLI & Model Support
- ✅ 9 AI CLIs supported
- ✅ Mixed CLI types within the same task
- ✅ Mixed providers/models within the same CLI via CC-Switch
- ✅ Per-terminal environment variable injection
- ✅ MCP server integration with per-CLI adapter

### Developer Experience
- ✅ Web-based pseudo-terminal for real-time debugging and interaction
- ✅ Full native plugin/skill/MCP compatibility — zero migration cost
- ✅ Git-driven event loop (98%+ token savings vs polling)
- ✅ Setup Wizard for first-run configuration
- ✅ Internationalization: 6 languages (English, 简体中文, 繁體中文, 日本語, Español, 한국어)

### Chat Platform Integration
- ✅ Telegram connector with conversation binding
- ✅ Feishu (Lark) long-lived WebSocket connector with session binding

### Deployment & Operations
- ✅ Docker one-click deployment with interactive installer
- ✅ Split deployment architecture (Server + Runner + Redis)
- ✅ Provider health monitoring API
- ✅ Sentry error tracking + PostHog analytics
- ✅ AES-256-GCM encryption for API keys at rest

### Roadmap
- 📋 Kubernetes deployment support
- 📋 Container image size optimization

---

## V1.0 Tested — Real-World Delivery Results

SoloDawn V1.0 was validated by a **48-hour fully-autonomous, self-healing end-to-end test** (2026-06-27 → 06-30): seven real tasks executed serially through the browser UI, with a Stop-hook driver that diagnosed and fixed orchestrator root causes *during* the run — zero human intervention.

**Final grade: 88.85 / A — 7 of 7 tasks delivered.**

| # | Task | Repo | Score | Grade |
|---|------|------|:---:|:---:|
| 1 | Knowledge-base app (from scratch) | `knowledge-base-demo` | 81 | B |
| 2 | Hoppscotch load-testing module | `hoppscotch-demo` | 88 | A |
| 3 | Express → Rust rewrite | `express-to-rust-demo` | 93 | A |
| 4 | Refactor + test backfill | `refactor-test-demo` | 92 | A |
| 5 | Microservice e-commerce (from scratch) | `ecommerce-demo` | 91 | A |
| 6 | Security + performance + monitoring | `kutt-security-demo` | 86 | A |
| 7 | Feishu memo app | `web-memo-demo` | 91 | A |

Scoring dimensions: buildability (20) / functionality (25) / code quality (30) / tests (15) / engineering (10), weighted by task complexity. Grades: S≥95 · A≥85 · B≥70.

The test itself was a stress test of SoloDawn's self-healing. The 48-hour run surfaced and fixed **21 orchestrator deadlock / stall root causes** (the "§8" chain, #1–#21) — each one a scenario where a naive implementation loops forever. Every fix was deployed and re-validated live before continuing. Full report: `docs/undeveloped/current/V1.0-质量验收-2026-06-30-48h自修复测试.md`.

> Model: `glm-5.2[1m]` via solodawn.cloud (Anthropic protocol). Test harness: browser-MCP UI, serial execution, /goal + Stop-hook self-repair.

---

## 🤖 Prompt for AI Assistants — "Run It For Me"

> **For end users.** You're not here to modify SoloDawn's source — you just want it running so you can use it. Paste the block below to any coding AI (Claude Code, Cursor, Codex, …) and it will get SoloDawn running on your machine.

```markdown
Help me install and RUN SoloDawn locally. I want to USE it, not modify its source.

Repository (clone source): https://github.com/huanchong-99/SoloDawn
Clone command: git clone https://github.com/huanchong-99/SoloDawn.git

Detect my OS (Windows / Linux / macOS) and do whatever is needed to get the web UI open at http://localhost:23457:

1. Install prerequisites if missing, verifying each:
   - Rust toolchain nightly-2025-12-04 (rustup install nightly-2025-12-04)
   - Node.js >= 18 and pnpm 10.13.1
   - Git
   - Build toolchain the Rust backend needs: a C/C++ compiler, protoc 31.1, LLVM/libclang, and (on x86-64) cmake + nasm + perl (for aws-lc-rs)
2. cd SoloDawn && pnpm install
3. Set a 32-character SOLODAWN_ENCRYPTION_KEY environment variable.
4. pnpm run dev  —  first launch compiles the Rust backend (several minutes), then serves frontend :23457 / backend :23456.
5. Poll http://localhost:23456/readyz until it returns {"ready":true}, then open http://localhost:23457.
6. Confirm the Setup Wizard appears, then help me configure an AI model and verify it.

If a build step fails, read the error, install the missing prerequisite, and retry. Only edit SoloDawn's own files if a step truly requires it.
```

**Run vs Develop — which one are you?**

| | Run SoloDawn (use it) | Develop SoloDawn (change its code) |
|---|---|---|
| What you do | `git clone` → `pnpm run dev` → open the web UI | everything at left, plus edit Rust/TS source |
| Prerequisites | the build toolchain above (backend compiles once) | + `sqlx-cli` 0.8.6, linter, full test tooling |
| Cache footprint | modest `target/` (server binary + deps only) | large `target/` (whole-workspace test/clippy/codegen) |
| See | [Quick Start](#quick-start) below | [Contributing](#contributing) |

> **1.0 ships source-run** — there is **no installer of any kind**. Clone and run with `pnpm run dev` (or build a release binary). The `installer/` directory and the `Build Windows Installer` workflow are retained from earlier development; 1.0 does not publish a Windows installer.

---

## Quick Start

### Prerequisites

| Tool | Version | Check |
|---|---|---|
| Rust | nightly-2025-12-04 | `rustc --version` |
| C/C++ toolchain | MSVC Build Tools (Windows) · gcc/clang (Linux/macOS) | — |
| protoc | 31.1 | `protoc --version` |
| LLVM / libclang | recent (needed by bindgen) | `clang --version` |
| cmake · nasm · perl | recent (needed by `aws-lc-rs` on x86-64) | `cmake --version` · `nasm --version` |
| Node.js | ≥ 18 (recommend 20) | `node --version` |
| pnpm | 10.13.1 | `pnpm --version` |
| Git | Any recent | `git --version` |

> ⚠️ **`protoc`, `LLVM/libclang`, and the `aws-lc-rs` build tools (`cmake`, `nasm`, `perl`) are required to build but are NOT installed by `scripts/setup-windows.ps1`** — install them manually:
>
> **Windows:** download [`protoc-31.1-win64.zip`](https://github.com/protocolbuffers/protobuf/releases/tag/v31.1), extract it, and add its `bin` to `PATH`; then install LLVM, NASM, CMake, and Perl:
> ```powershell
> winget install LLVM.LLVM
> winget install NASM.NASM
> winget install Kitware.CMake
> winget install StrawberryPerl.StrawberryPerl
> [Environment]::SetEnvironmentVariable("PROTOC", "C:\path\to\protoc\bin\protoc.exe", "User")
> [Environment]::SetEnvironmentVariable("LIBCLANG_PATH", "$env:ProgramFiles\LLVM\bin", "User")
> ```
> **Linux (apt):** `sudo apt-get install -y protobuf-compiler clang libclang-dev cmake nasm perl`
> **macOS (brew):** `brew install protobuf llvm cmake nasm` (Perl ships with macOS)

### Getting Started After Cloning

> 中文版请参阅 [README.zh-CN.md](README.zh-CN.md)

#### 1. Install Rust Toolchain

```bash
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04
```

#### 2. Install Required Cargo Tools

```bash
cargo install cargo-watch
# Pin sqlx-cli to 0.8.x — the latest 0.9.0 needs rustc ≥ 1.94, but the pinned
# nightly-2025-12-04 is rustc 1.93, so an unpinned install fails.
cargo install sqlx-cli --version 0.8.6 --no-default-features --features rustls,sqlite
```

#### 3. Install Node.js Dependencies

```bash
pnpm install
```

#### 4. Set Environment Variables

**Linux / macOS:**

```bash
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"  # Must be exactly 32 characters
```

**Windows PowerShell:**

```powershell
$env:SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
```

#### 5. Initialize Database

```bash
pnpm run prepare-db
```

#### 6. Start Development Servers

```bash
pnpm run dev
```

This starts both the backend (Rust/Axum) and frontend (Vite/React) dev servers.

- **Frontend:** http://localhost:23457
- **Backend API:** http://localhost:23456/api

On first launch, the **Setup Wizard** will guide you through environment detection, AI model configuration, and project setup.

#### 7. (Optional) Production Build

```bash
# Build backend
cargo build --release -p server

# Build frontend
cd frontend && pnpm build && cd ..

# Set encryption key and run
export SOLODAWN_ENCRYPTION_KEY="your-32-character-secret-key-here"
./target/release/server
```

Production mode serves both frontend and API on a single port: http://localhost:23456

### ⚠️ Common Pitfalls

These trip up first-time setup, especially on Windows:

- **`protoc` and `libclang` are required but are NOT installed by `scripts/setup-windows.ps1`.** Without `protoc`, `crates/services`, `crates/runner`, and `crates/feishu-connector` fail to build (there is no vendored protoc in the lockfile). Without `libclang`, `libsqlite3-sys` fails when bindgen runs (the `sqlite-preupdate-hook` sqlx feature triggers it). Install commands are in [Prerequisites](#prerequisites).
- **Pin `sqlx-cli` to 0.8.x.** The latest 0.9.0 requires rustc ≥ 1.94, but the pinned `nightly-2025-12-04` is rustc 1.93, so an unpinned `cargo install sqlx-cli` fails.
- **No database is needed to build.** `.cargo/config.toml` sets `SQLX_OFFLINE=true`, so builds use the committed `crates/db/.sqlx/` query cache. You only need `sqlx-cli` / `pnpm run prepare-db` when you change SQL queries or migrations.
- **Windows: restart your terminal after installing tools** so it picks up the updated `PATH`, `PROTOC`, and `LIBCLANG_PATH`.
- **`cmake`, `nasm`, and `perl` ARE required for a local build on x86-64.** Since the Feishu connector migrated to the `openlark` SDK, the dependency tree uses `aws-lc-rs` (AWS-LC) instead of `ring`; its `aws-lc-sys` build compiles AWS-LC's optimized assembly from source (needs `nasm` + `cmake`; `perl` on some platforms). Install commands are in [Prerequisites](#prerequisites). (`libgit2-sys` itself still builds via the `cc` crate.)

### Docker (One-Click Install)

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

### Split Deployment (Server + Runner)

```bash
cd docker/compose
docker-compose -f docker-compose.split.yml up -d
```

---

## Quality Gate Configuration

Configure in `quality/quality-gate.yaml`:

```yaml
mode: shadow  # off | shadow | warn | enforce
```

| Mode | Behavior |
|------|----------|
| `off` | Disabled |
| `shadow` | Runs analysis, logs results, never blocks (default) |
| `warn` | Runs analysis, notifies via UI, does not block |
| `enforce` | Hard gate — blocks terminal handoff on failure |

```bash
# Run quality gate manually
pnpm run quality

# Dry-run check
pnpm run quality:check
```

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Rust (Axum, SQLx, Tokio) |
| Frontend | React 18, TypeScript, Tailwind CSS, Zustand, React Query |
| Database | SQLite (AES-256-GCM encrypted key storage) |
| Terminal | xterm.js + native PTY (WebSocket bridge) |
| Real-time | WebSocket (workflow events + terminal streams) |
| Type Safety | Rust → TypeScript auto-generation via `ts-rs` |
| Quality | Built-in rule engine + optional SonarQube |
| i18n | 6 languages (en, zh-Hans, zh-Hant, ja, es, ko) |

---

## Project Structure

```
SoloDawn/
├── crates/                    # Rust workspace
│   ├── server/                # Axum HTTP/WebSocket server + MCP Task Server
│   ├── services/              # Business logic
│   │   ├── orchestrator/      # Agent, Runtime, State, Error handling
│   │   ├── terminal/          # Launcher, Bridge, Prompt watcher
│   │   ├── git_watcher.rs     # Git commit monitoring
│   │   ├── merge_coordinator.rs # Centralized merge handling
│   │   └── chat_connector.rs  # Unified chat trait
│   ├── quality/               # Three-layer quality gate engine
│   │   └── src/gate/          # Ported from SonarQube quality gate model
│   ├── cc-switch/             # CLI model switching library
│   ├── executors/             # CLI integrations + MCP config adapters
│   ├── feishu-connector/      # Feishu WebSocket client
│   ├── db/                    # Database layer (models, migrations, DAO)
│   ├── runner/                # gRPC remote Runner for split deployment
│   └── utils/                 # Shared utilities (encryption, OAuth, Sentry)
├── frontend/                  # React application
│   └── src/
│       ├── components/        # UI components
│       ├── stores/            # Zustand stores
│       └── i18n/              # 6 locales
├── quality/                   # Quality gate profiles and baselines
├── scripts/                   # Dev, Docker, and deployment scripts
└── docs/                      # Documentation
```

---

## Contributing

- Open an issue first for large changes.
- Run checks before PR:

```bash
cargo check --workspace
cargo test --workspace
cd frontend && pnpm test:run && cd ..
```

---

## License

- SoloDawn: Apache-2.0
- Vibe Kanban derived parts: Apache-2.0
- CC-Switch derived parts: MIT
- Quality Gate models (ported from SonarQube): LGPL-3.0
- See `LICENSE` for full details.

## Blogroll

- [LINUX DO](https://linux.do/)
