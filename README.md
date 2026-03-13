<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
</p>

# GitCortex

**Complete complex, production-grade projects through a simple conversation.**

GitCortex is an upper-layer orchestration Agent that automatically commands multiple professional AI CLIs (Claude Code, Gemini CLI, Codex, Amp, Cursor Agent, etc.) to develop software in parallel. It does not write code itself — it acts as a fully autonomous project manager: assigning tasks, monitoring progress, coordinating Git branches, handling errors, and merging results, until the entire project is delivered.

> Think of it this way: you describe what you want to build, and GitCortex dispatches 5–10 AI terminals working simultaneously on different features, each on its own Git branch, with a central orchestrator keeping everything on track — automatically.

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
| **Chat-to-Ship (Vision)** | The ultimate goal: connect to a chat platform (Telegram, Feishu/Lark), describe your project in conversation, and GitCortex handles everything — task decomposition, terminal allocation, execution, and delivery. Not a toy demo, but real production-grade output. |

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
| `ChatConnector` | Unified outbound messaging trait (Telegram, Feishu/Lark) |

---

## Features

### Implemented

- ✅ Upper-layer orchestrator Agent commanding full workflow lifecycle
- ✅ Multi-task parallel execution (5–10 tasks simultaneously)
- ✅ **Built-in Quality Gates** with three-layer verification (Terminal → Task → Repo)
- ✅ Local **SonarQube integration** for deep code analysis (zero external API dependencies)
- ✅ Serial quality gates within each task (code → review → fix)
- ✅ Mixed CLI types within the same task (Claude Code + Gemini + Codex + more)
- ✅ Mixed providers/models within the same CLI via CC-Switch integration
- ✅ Native slash command system — supports all official and custom commands
- ✅ Full native plugin/skill/MCP compatibility (whatever your CLI supports, GitCortex supports)
- ✅ Git-driven event loop (98%+ token savings vs polling)
- ✅ Web-based pseudo-terminal for real-time debugging and interaction
- ✅ Cross-terminal context handoff (previous terminal's work passed to the next)
- ✅ Automatic branch merging on workflow completion
- ✅ ReviewCode / FixIssues / MergeBranch instruction execution
- ✅ LLM fault tolerance with graceful degradation (agent survives provider outages)
- ✅ State persistence with crash recovery (agent resumes from DB after restart)
- ✅ Multi-provider circuit breaker with automatic failover
- ✅ Terminal-level provider failover (auto-spawns replacement terminal)
- ✅ Telegram connector with conversation binding
- ✅ Feishu (Lark) long-lived WebSocket connector
- ✅ Planning Draft with multi-turn LLM conversation
- ✅ Docker one-click deployment with installer/update scripts
- ✅ Provider health monitoring API with WebSocket events

### Roadmap

- 🔜 Full conversational task decomposition (Agent decides task count and terminal allocation)
- 🔜 Deeper chat platform integration (describe project → auto-execute → deliver)
- 📋 Kubernetes deployment support
- 📋 Container image size optimization

---

## Supported AI CLIs

| CLI | Status | Model Switching |
|---|---|---|
| Claude Code | ✅ Supported | ✅ Via CC-Switch |
| Gemini CLI | ✅ Supported | ✅ Via CC-Switch |
| Codex | ✅ Supported | ✅ Via CC-Switch |
| Amp | ✅ Supported | — |
| Cursor Agent | ✅ Supported | — |
| Qwen Code | ✅ Supported | — |
| GitHub Copilot | ✅ Supported | — |

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

### Docker (One-Click Install)

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\install-docker.ps1
```

The installer supports interactive setup (mount path, keys, port, optional AI CLI install), `.env` reuse, and automatic handoff to update flow.

**Optional: SonarQube Integration**
GitCortex features an integrated, three-layer Quality Gate system using the SonarQube code analysis engine running locally.
To spin up the local SonarQube instance, navigate to the docker directory:
```bash
cd docker/compose
docker-compose -f docker-compose.dev.yml up -d sonarqube
```

### Docker Update

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest
```

Options: `-AllowDirty`, `-PullBaseImages`, `-SkipBuild`, `-SkipReadyCheck`

---

## How It Works

### 1. Create a Workflow

Through the web UI wizard, you:
- Select a Git repository
- Define parallel tasks (e.g. "auth module", "i18n", "dark theme")
- Assign terminals to each task (choose CLI type + model for each)
- Optionally configure slash commands for execution order
- Configure the orchestrator Agent's LLM

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
- Merges all task branches when the workflow completes

The orchestrator sleeps between Git events — it only wakes and consumes tokens when there's actual work to process.

### 4. Deliver

All task branches are automatically merged to the target branch. The workflow is complete.

---

## Data Safety

- **Deleting a project** only removes database records. Repository files on disk are never touched.
- **Unbinding a repository** only removes the database link. The Git repository remains intact.
- **Project-repo binding** stores a reference path only. No file system operations on bind/unbind.

---

## Health Check

```bash
curl http://localhost:23456/readyz
curl http://localhost:23456/api/health
```

With API token enabled:

```bash
curl http://localhost:23456/api/health -H "Authorization: Bearer <token>"
```

---

## Quality Gate

GitCortex includes a built-in quality gate engine that automatically verifies code quality at three levels:

| Gate | Trigger | Scope |
|------|---------|-------|
| **Terminal Gate** | Each checkpoint commit | Changed files only — cargo check, clippy, tsc, tests |
| **Branch Gate** | Last terminal in a task passes | Full task branch — all checks + lint |
| **Repo Gate** | Before merge to main / CI | Full repo — all checks + SonarQube analysis |

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

---

## Project Structure

```
GitCortex/
├── crates/                    # Rust workspace
│   ├── db/                    # Database layer (models, migrations, DAO)
│   ├── server/                # Axum HTTP/WebSocket server
│   ├── services/              # Business logic
│   │   ├── orchestrator/      # Agent, Runtime, State, Error handling
│   │   ├── terminal/          # Launcher, Bridge, Prompt watcher
│   │   ├── git_watcher.rs     # Git commit monitoring
│   │   ├── cc_switch.rs       # CLI/model configuration switching
│   │   ├── message_bus.rs     # Event routing
│   │   ├── feishu.rs          # Feishu service integration
│   │   └── chat_connector.rs  # Unified chat trait
│   ├── executors/             # CLI integrations
│   └── feishu-connector/      # Feishu WebSocket client
├── frontend/                  # React application
│   ├── src/
│   │   ├── components/        # UI components (board, workflow, ui-new)
│   │   ├── hooks/             # React Query hooks
│   │   ├── stores/            # Zustand stores (WebSocket, UI state)
│   │   └── pages/             # Route components
│   └── CLAUDE.md              # Frontend design guidelines
├── shared/                    # Auto-generated TypeScript types
├── scripts/                   # Dev and Docker scripts
└── docs/                      # Documentation
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
