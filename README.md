<p align="center">
  <img src="installer/assets/solodawn.ico" alt="SoloDawn" width="120" />
</p>

<h1 align="center">SoloDawn</h1>

<p align="center">
  <strong>Give it one sentence. It builds the entire project. You just wait for delivery.</strong>
</p>

<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
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

## Quick Start

### Prerequisites

| Tool | Version | Check |
|---|---|---|
| Rust | nightly-2025-12-04 | `rustc --version` |
| Node.js | ≥ 18 (recommend 20) | `node --version` |
| pnpm | 10.13.1 | `pnpm --version` |
| Git | Any recent | `git --version` |

### Local Development

```bash
# 1. Install dependencies
pnpm install

# 2. Set encryption key (required, exactly 32 characters)
# Linux/macOS:
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
# Windows PowerShell:
$env:SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"

# 3. Initialize database
pnpm run prepare-db

# 4. Start dev servers (frontend + backend)
pnpm run dev
```

Default URLs:
- Frontend: `http://localhost:23457`
- Backend API: `http://localhost:23456/api`

On first launch, the **Setup Wizard** guides you through environment detection, AI model configuration, and project setup.

### Production Build

```bash
# 1. Build backend
cargo build --release -p server

# 2. Build frontend (static assets, embedded into backend)
cd frontend && pnpm build && cd ..

# 3. Set encryption key (required)
export SOLODAWN_ENCRYPTION_KEY="your-32-character-secret-key-here"

# 4. Run
./target/release/server
```

Production mode serves both frontend and API on a single port: `http://localhost:23456`

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
