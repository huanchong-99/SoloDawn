<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
</p>

# GitCortex

**Complete complex, production-grade projects through a simple conversation.**

GitCortex is an upper-layer orchestration Agent that automatically commands multiple professional AI CLIs (Claude Code, Gemini CLI, Codex, Amp, Cursor Agent, etc.) to develop software in parallel. It does not write code itself — it acts as a fully autonomous project manager: assigning tasks, monitoring progress, coordinating Git branches, handling errors, and merging results, until the entire project is delivered.

> Think of it this way: you describe what you want to build, and GitCortex dispatches 5–10 AI terminals working simultaneously on different features, each on its own Git branch, with a central orchestrator keeping everything on track — automatically.

---

## Core Design Philosophy

**"1 senior engineer + GitCortex = the output of 1 senior + 3 mid-level + 10 junior engineers."**

- **Upper-layer orchestration, not code generation.** The orchestrator Agent never writes code — it commands the best professional tools (Claude Code, Gemini CLI, Codex, etc.) to do the work.
- **Non-invasive by design.** GitCortex does not replace any CLI, modify any configuration file, or define new tools. It inherits the full native ecosystem of every CLI it orchestrates — all slash commands, plugins, skills, and MCP servers work unchanged, forever.
- **Git-driven event loop.** The orchestrator only consumes LLM tokens when a Git commit event occurs. Between events, it sleeps at zero cost. This saves 98%+ tokens compared to polling-based approaches.

---

## Why GitCortex

### The Problem

During continuous AI-assisted coding, several pain points remain unsolved:

- You cannot use different providers' models within the same AI CLI session.
- Multi-CLI collaboration solutions (MCP, skills, etc.) have limitations and break with every update.
- Workflow plugins become obsolete within months as the ecosystem evolves.
- Single-terminal AI coding is inherently sequential — one task at a time.

### The Solution

GitCortex takes a fundamentally different approach: **one orchestrator Agent commanding all professional CLIs**.

| Capability | Description |
|---|---|
| **Upper-Layer Orchestration** | A central Agent automatically dispatches instructions, monitors task progress, handles branch merging and error recovery — zero human intervention required during execution. |
| **5–10× Development Efficiency** | Multi-task parallelism: the orchestrator runs 5–10 tasks simultaneously, each on its own Git branch. Tasks are serial internally (quality gates), but parallel across the workflow. |
| **Non-Invasive Ecosystem Compatibility** | Calls native CLI terminals directly. Any slash command, plugin, skill, or MCP that works in your terminal works here — forever. Switch from one AI workflow to another (e.g. Superpower, SDD) with zero migration cost. |
| **Mixed CLI & Model per Task** | Different CLIs and different providers' models can work within the same task. Claude Code with Sonnet for coding, Gemini for review, GPT for fixing — all orchestrated automatically. |
| **Git-Driven Event Loop** | Terminals signal completion via Git commits. The orchestrator sleeps between events, consuming near-zero tokens when idle. Saves 98%+ tokens compared to polling. |
| **Chat-to-Ship** | Connect to a chat platform (Telegram, Feishu/Lark), describe your project in conversation, and GitCortex handles everything — task decomposition, terminal allocation, execution, and delivery. |

---

## How It Differs from CCG / OMO / CCW

GitCortex is **not** another multi-CLI collaboration tool. The core design goal is fundamentally different:

| Aspect | Multi-CLI Tools (CCG, OMO, CCW) | GitCortex |
|---|---|---|
| Focus | CLI-to-CLI communication | Upper-layer Agent commanding all CLIs |
| Execution | Manual or semi-automated | Fully autonomous orchestration |
| Parallelism | Limited | 5–10 tasks in parallel by design |
| Plugin Ecosystem | Often builds its own | Inherits all native CLI ecosystems |
| Longevity | Tied to specific tool versions | Non-invasive — survives ecosystem churn |
| Goal | Better CLI interop | "Developer not present" long-running autonomous development |

GitCortex doesn't define tools — it commands the best tools to complete tasks most efficiently.

---

## Two Execution Modes

### Manual Orchestration (DIY)

You define the workflow graph upfront through a wizard:
- Set tasks, Git branches, terminal assignments, CLI types, and models
- Full control over task granularity and terminal roles
- Best for well-understood projects with clear decomposition

### AI Auto-Orchestration (Agent-Planned)

You describe a project goal, and the orchestrator LLM autonomously:
- Decomposes the project into tasks with dependency analysis
- Creates terminals, assigns CLIs and models per task
- Manages multi-phase execution (infrastructure → features → integration → finalization)
- Merges completed branches and dispatches follow-up tasks
- Best for complex projects where task decomposition itself requires intelligence

### WorkspacePlanning: Multi-Turn Conversation

Before creating a workflow, you can engage the AI in multi-turn planning:

1. **Gathering** — You describe your project; the AI asks clarifying questions
2. **Spec Ready** — Technical specification generated and presented for review
3. **Confirmed** — You approve the spec
4. **Materialized** — The spec is converted into a runnable agent_planned Workflow

The Planning Draft captures both `requirement_summary` and `technical_spec`, which are combined into the workflow's initial goal for the orchestrator.

### Intelligent Requirement Assessment

The Workspace Planner automatically evaluates the clarity of your input:

- **Vague requirements** (e.g., "build a knowledge management tool") trigger the Gathering phase: the AI asks focused follow-up questions about scope, features, auth, deployment, etc.
- **Precise requirements** (e.g., 5+ specific technical requirements with clear scope) skip directly to spec generation, respecting your expertise.

This eliminates both the friction of unnecessary interrogation for experienced users and the risk of building the wrong thing from ambiguous prompts.

### V1.0 Feature Highlights

| Feature | Description |
|---|---|
| **Quality Gate System** | Three-layer verification (terminal/branch/repo) with shadow, warn, and enforce modes. Works in both Workflow and Workspace modes. |
| **Send Mode Toggle** | Switch between Enter and Ctrl+Enter for message sending via a single click in the chat footer. |
| **Responsive Layout** | Workspace UI adapts to narrow viewports (800px+) with collapsible sidebars. |
| **Repo Auto-Fill** | When creating a workflow, bound repositories are auto-populated from the selected project. |
| **Shallow Clone Handling** | Automatically unshallows `--depth 1` clones during workspace preparation to prevent commit failures. |
| **Model Selection Reliability** | Improved model config persistence ensures the selected model is always used, even across workflow restarts. |
| **Paginated Process Loading** | Execution process history loads incrementally via REST API with database-level indexing. |
| **API Fault Recovery** | Circuit breaker auto-pauses workflows on provider exhaustion; tracks pause reason for future auto-resume. |

---

## Architecture

```
                    ┌─────────────────────────────────┐
                    │   Orchestrator Agent (LLM-driven)│
                    │   Dispatches · Monitors · Merges │
                    └──────────┬──────────────────────┘
                               │
              ┌────────────────┼────────────────┐
              ▼                ▼                ▼
     ┌──────────────┐ ┌──────────────┐ ┌──────────────┐
     │   Task 1     │ │   Task 2     │ │   Task 3     │
     │ branch: auth │ │ branch: i18n │ │ branch: theme│
     │              │ │              │ │              │
     │ T1 → T2 → T3│ │ TA → TB     │ │ TX → TY     │
     │  (serial)    │ │  (serial)    │ │  (serial)    │
     └──────────────┘ └──────────────┘ └──────────────┘
              │                │                │
              └────────────────┼────────────────┘
                               ▼
                         Auto-Merge → main
```

**Three-Layer Execution Model:**

- **Workflow** → The orchestrator Agent manages the entire lifecycle
- **Task** → Independent Git branch, runs in parallel with other tasks
- **Terminal** → A native AI CLI process (PTY), runs serially within its task

**Key Components:**

| Component | Role |
|---|---|
| `OrchestratorAgent` | LLM-driven decision core: dispatches terminals, parses Git events, routes review/fix cycles |
| `OrchestratorRuntime` | Workflow lifecycle management, slot reservation, crash recovery |
| `MessageBus` | Event routing across all modules (workflow-scoped topics) |
| `TerminalLauncher` | Spawns native PTY processes with per-terminal environment isolation |
| `GitWatcher` | Detects Git commits → publishes events → wakes the orchestrator |
| `ResilientLLMClient` | Multi-provider round-robin with 5-failure circuit breaker and 60s probe recovery |
| `MergeCoordinator` | Centralized merge handling with conflict detection and partial-failure tracking |
| `ChatConnector` | Unified outbound messaging trait (Telegram, Feishu/Lark) |

---

## Features

### Implemented

**Orchestration & Execution**
- ✅ Upper-layer orchestrator Agent commanding full workflow lifecycle
- ✅ Two execution modes: Manual (DIY) and AI Auto-Orchestration (Agent-Planned)
- ✅ Multi-task parallel execution (5–10 tasks simultaneously)
- ✅ WorkspacePlanning multi-turn LLM conversation for project scoping
- ✅ Planning Draft lifecycle: gathering → spec_ready → confirmed → materialized
- ✅ Serial quality gates within each task (code → review → fix)
- ✅ Cross-terminal context handoff (previous terminal's work passed to the next)
- ✅ ReviewCode / FixIssues / MergeBranch instruction execution
- ✅ Automatic branch merging with conflict auto-resolution for non-overlapping changes
- ✅ Post-merge branch base refresh for pending tasks

**CLI & Model Support**
- ✅ 9 AI CLIs supported (see table below)
- ✅ Mixed CLI types within the same task (Claude Code + Gemini + Codex + more)
- ✅ Mixed providers/models within the same CLI via CC-Switch integration
- ✅ Per-terminal environment variable injection (no global config switching)
- ✅ MCP server integration with per-CLI adapter (auto-generates correct config format)

**Quality & Reliability**
- ✅ **Built-in Quality Gates** with three-layer verification (Terminal → Task → Repo)
- ✅ Built-in rule engine (runs without SonarQube), with optional local SonarQube integration
- ✅ Policy snapshots and issue tracking per terminal and per workflow
- ✅ LLM fault tolerance with graceful degradation (agent survives provider outages)
- ✅ State persistence with crash recovery (agent resumes from DB after restart)
- ✅ Multi-provider circuit breaker with automatic failover
- ✅ Terminal-level provider failover (auto-spawns replacement terminal)

**Developer Experience**
- ✅ Web-based pseudo-terminal for real-time debugging and interaction
- ✅ Native slash command system — supports all official and custom commands
- ✅ Full native plugin/skill/MCP compatibility (whatever your CLI supports, GitCortex supports)
- ✅ Git-driven event loop (98%+ token savings vs polling)
- ✅ Setup Wizard for first-run environment detection and configuration
- ✅ Internationalization: 6 languages (English, 简体中文, 繁體中文, 日本語, Español, 한국어)

**Chat Platform Integration**
- ✅ Telegram connector with conversation binding
- ✅ Feishu (Lark) long-lived WebSocket connector with session binding and bot event notifications

**Deployment & Operations**
- ✅ Docker one-click deployment with interactive installer/updater scripts
- ✅ Split deployment architecture (Server + Runner + Redis via `docker-compose.split.yml`)
- ✅ Provider health monitoring API with SSE and WebSocket events
- ✅ Health check endpoints (`/healthz`, `/readyz`, `/api/health`)
- ✅ Sentry error tracking integration
- ✅ PostHog product analytics integration
- ✅ Structured logging via `tracing` crate

**Security**
- ✅ API Token authentication (`GITCORTEX_API_TOKEN`)
- ✅ AES-256-GCM encryption for API keys at rest (random nonce per encryption)
- ✅ OAuth support for external service authentication
- ✅ Per-request token validation middleware

### Roadmap

- 📋 Kubernetes deployment support
- 📋 Container image size optimization

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
# Windows PowerShell:
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
# Linux/macOS:
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# 3. Initialize database
pnpm run prepare-db

# 4. Start dev servers (frontend + backend)
pnpm run dev
```

Default URLs:
- Frontend: `http://localhost:23457`
- Backend API: `http://localhost:23456/api`

On first launch, the **Setup Wizard** guides you through environment detection, AI model configuration, and project setup.

**Optional: SonarQube Integration**
GitCortex features an integrated, three-layer Quality Gate system. The built-in rule engine works without SonarQube, but for deep code analysis you can spin up a local instance:
```bash
cd docker/compose
docker-compose -f docker-compose.dev.yml up -d sonarqube
```

### Production Build

```bash
# 1. Build backend (release binary)
cargo build --release -p server

# 2. Build frontend (static assets, embedded into backend)
cd frontend && pnpm build && cd ..

# 3. Set encryption key (required, exactly 32 characters)
# Linux/macOS:
export GITCORTEX_ENCRYPTION_KEY="your-32-character-secret-key-here"
# Windows PowerShell:
$env:GITCORTEX_ENCRYPTION_KEY="your-32-character-secret-key-here"

# 4. Run
./target/release/server    # Linux/macOS
.\target\release\server.exe  # Windows
```

Production mode serves both frontend and API on a single port: `http://localhost:23456`

> **Note:** In development mode (`pnpm run dev`), the encryption key is optional — a default key is used automatically. In production (release build), the server will **refuse to start** if the key is not set.

### Docker (One-Click Install)

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

The installer supports interactive setup (mount path, keys, port, optional AI CLI install), `.env` reuse, and automatic handoff to update flow. The encryption key is configured automatically during the install wizard.

### Docker Update

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest
```

Options: `-AllowDirty`, `-PullBaseImages`, `-SkipBuild`, `-SkipReadyCheck`

### Split Deployment (Server + Runner)

For teams that want to separate the web server from terminal execution:

```bash
cd docker/compose
docker-compose -f docker-compose.split.yml up -d
```

This starts three services:
- **Server** — API + frontend, handles orchestration logic
- **Runner** — Executes PTY terminals with AI CLIs installed, communicates via gRPC
- **Redis** — Message broker between server and runner

Best for: CI/CD environments, multi-machine setups, or when AI CLIs need a different runtime than the web server.

---

## How It Works

### 1. Create a Workflow

Through the web UI wizard, you either:

**DIY Mode:**
- Select a Git repository
- Define parallel tasks (e.g. "auth module", "i18n", "dark theme")
- Assign terminals to each task (choose CLI type + model for each)
- Optionally configure slash commands for execution order
- Configure the orchestrator Agent's LLM

**Agent-Planned Mode:**
- Select a Git repository
- Describe the project goal in natural language
- Configure the orchestrator Agent's LLM
- The orchestrator will autonomously decompose, create tasks, and assign terminals

### 2. Prepare & Debug

GitCortex spawns all terminal PTY processes and enters a **Ready** state. You can:
- Verify CLI environments in the web-based pseudo-terminal
- Test slash commands and plugin availability
- Install missing dependencies

Zero token consumption during this phase.

### 3. Execute

Click **Start** and the orchestrator takes over:
- Dispatches instructions to each task's first terminal
- Monitors Git commits for completion signals
- Passes context from completed terminals to the next one (handoff notes)
- Handles review cycles (ReviewCode → FixIssues → re-review)
- Manages errors and retries automatically
- Auto-resolves non-overlapping merge conflicts
- Refreshes pending task branches after each merge
- Merges all task branches when the workflow completes

The orchestrator sleeps between Git events — it only wakes and consumes tokens when there's actual work to process.

### 4. Deliver

All task branches are automatically merged to the target branch. The workflow is complete.

---

## MCP Integration

GitCortex integrates with the Model Context Protocol (MCP) at two levels:

### MCP Task Server
GitCortex ships a built-in MCP Task Server (`mcp_task_server` binary) that exposes workflow and task management as MCP tools, enabling external AI agents to interact with GitCortex programmatically.

### Per-CLI MCP Configuration
When launching terminals, GitCortex auto-generates the correct MCP server configuration for each CLI type. Each CLI has its own adapter that transforms a unified server definition into the CLI-specific format:
- **Claude Code / Amp / Droid** — Passthrough (native MCP format)
- **Gemini CLI** — Gemini-specific adapter
- **Codex** — Codex-specific adapter
- **Cursor Agent** — Cursor-specific adapter
- **Opencode** — Opencode-specific adapter
- **GitHub Copilot** — Copilot-specific adapter

---

## Feishu (Lark) Integration

GitCortex includes a full Feishu bot integration:

- **WebSocket long-lived connection** for real-time event reception
- **Session binding** — conversations are linked to specific workflows
- **Bot event notifications** — workflow status changes, terminal completions, and errors are pushed to bound Feishu conversations
- **Configuration** via frontend Settings UI with i18n support (en + zh-Hans)
- **3 REST endpoints** for Feishu webhook, bot management, and session control

---

## Security

| Feature | Details |
|---|---|
| **API Token Auth** | Enable `GITCORTEX_API_TOKEN` env var; all API routes require `Authorization: Bearer <token>` |
| **AES-256-GCM Encryption** | API keys encrypted at rest with random nonce per encryption; key from `GITCORTEX_ENCRYPTION_KEY` (32 bytes) |
| **OAuth Support** | OAuth client for external service authentication |
| **Approval Gates** | Interactive prompts for destructive operations; user confirmation via WebSocket |
| **Per-Request Validation** | `require_api_token` middleware on all routes when token is configured |

---

## Observability

| Feature | Details |
|---|---|
| **Sentry** | Error tracking integration via `utils::sentry` |
| **PostHog** | Product analytics integration |
| **Structured Logging** | `tracing` crate with configurable levels via `RUST_LOG` |
| **Health Endpoints** | `/healthz` (liveness), `/readyz` (readiness: DB + assets + temp dir + Feishu), `/api/health` (legacy) |
| **CLI Health Monitoring** | SSE endpoint for real-time provider health status |

---

## Data Safety

- **Deleting a project** only removes database records. Repository files on disk are never touched.
- **Unbinding a repository** only removes the database link. The Git repository remains intact.
- **Project-repo binding** stores a reference path only. No file system operations on bind/unbind.

---

## Quality Gate

GitCortex includes a built-in quality gate engine that automatically verifies code quality at three levels:

| Gate | Trigger | Scope |
|------|---------|-------|
| **Terminal Gate** | Each checkpoint commit | Changed files only — cargo check, clippy, tsc, tests |
| **Branch Gate** | Last terminal in a task passes | Full task branch — all checks + lint |
| **Repo Gate** | Before merge to main / CI | Full repo — all checks + SonarQube analysis |

### Built-in Rule Engine

The quality gate includes a built-in rule engine that works **without requiring SonarQube**:
- Runs configurable lint checks (Rust clippy, TypeScript tsc, ESLint)
- Tracks policy snapshots per terminal and per workflow
- Issue tracking with structured fix instructions sent back to terminals
- SonarQube is optional — adds deep static analysis when available

### Modes

Configure in `quality/quality-gate.yaml`:

```yaml
mode: shadow  # off | shadow | warn | enforce
```

| Mode | Behavior |
|------|----------|
| `off` | Disabled, legacy workflow semantics |
| `shadow` | Runs analysis, logs results, never blocks (default) |
| `warn` | Runs analysis, notifies via UI, does not block |
| `enforce` | Hard gate — blocks terminal handoff on failure |

### How It Works

1. Terminal commits code → orchestrator intercepts as **checkpoint** (not final completion)
2. Quality engine runs configured checks against the terminal's working directory
3. **Pass** → terminal promoted to completed → next terminal dispatched
4. **Fail** → structured fix instructions sent back to the same terminal → terminal fixes and re-commits

### Running Manually

```bash
# Full quality gate (repo level, shadow mode)
pnpm run quality

# Dry-run check
pnpm run quality:check

# SonarCloud analysis only
pnpm run quality:sonar
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `QUALITY_GATE_MODE` | Override YAML mode (off/shadow/warn/enforce) |
| `SONAR_TOKEN` | SonarQube/SonarCloud authentication token |
| `SONAR_HOST_URL` | SonarQube server URL (default: http://localhost:9000) |

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Rust (Axum, SQLx, Tokio) |
| Frontend | React 18, TypeScript, Tailwind CSS, Zustand, React Query |
| Database | SQLite (encrypted API key storage via AES-256-GCM) |
| Terminal | xterm.js + native PTY (WebSocket bridge) |
| Real-time | WebSocket (workflow events + terminal streams) |
| Type Safety | Rust → TypeScript auto-generation via `ts-rs` |
| i18n | 6 languages (en, zh-Hans, zh-Hant, ja, es, ko) |

---

## Project Structure

```
GitCortex/
├── crates/                    # Rust workspace
│   ├── db/                    # Database layer (models, migrations, DAO)
│   ├── server/                # Axum HTTP/WebSocket server + MCP Task Server
│   ├── services/              # Business logic
│   │   ├── orchestrator/      # Agent, Runtime, State, Error handling
│   │   ├── terminal/          # Launcher, Bridge, Prompt watcher
│   │   ├── git_watcher.rs     # Git commit monitoring
│   │   ├── cc_switch.rs       # CLI/model configuration switching
│   │   ├── message_bus.rs     # Event routing
│   │   ├── merge_coordinator.rs # Centralized merge handling
│   │   ├── feishu.rs          # Feishu service integration
│   │   └── chat_connector.rs  # Unified chat trait
│   ├── cc-switch/             # CLI model switching library
│   ├── executors/             # CLI integrations + MCP config adapters
│   ├── feishu-connector/      # Feishu WebSocket client
│   ├── quality/               # Code quality gate engine
│   ├── runner/                # gRPC remote Runner for split deployment
│   ├── review/                # Code review CLI
│   ├── deployment/            # Deployment tooling
│   └── utils/                 # Shared utilities (encryption, OAuth, analytics, Sentry)
├── frontend/                  # React application
│   ├── src/
│   │   ├── components/        # UI components (board, workflow, setup wizard, ui-new)
│   │   ├── hooks/             # React Query hooks
│   │   ├── stores/            # Zustand stores (WebSocket, UI state)
│   │   ├── i18n/              # 6 locales (en, zh-Hans, zh-Hant, ja, es, ko)
│   │   └── pages/             # Route components
│   └── CLAUDE.md              # Frontend design guidelines
├── shared/                    # Auto-generated TypeScript types (Rust → TS)
├── quality/                   # Quality gate profiles and baselines
├── scripts/                   # Dev, Docker, and deployment scripts
└── docs/                      # Documentation and architecture decisions
```

---

## Health Check

```bash
curl http://localhost:23456/healthz    # Liveness (stateless, always 200)
curl http://localhost:23456/readyz     # Readiness (checks DB, assets, temp dir, Feishu)
curl http://localhost:23456/api/health # Legacy health check
```

With API token enabled:

```bash
curl http://localhost:23456/api/health -H "Authorization: Bearer <token>"
```

---

## Contributing

- Open an issue first for large changes.
- Keep commits small and reviewable.
- Run checks before PR:

```bash
cargo check --workspace
cargo test --workspace
cd frontend && pnpm test:run && cd ..
```

---

## License

- Vibe Kanban derived parts: Apache-2.0
- CC-Switch derived parts: MIT
- See `LICENSE` for details.
