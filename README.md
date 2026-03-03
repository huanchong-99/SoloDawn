<p align="center">
  <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <strong>AI Agent Cross-Terminal Task Coordination Platform</strong>
</p>

<p align="center">
  Based on <a href="https://github.com/BloopAI/vibe-kanban">Vibe Kanban</a>, integrated with <a href="https://github.com/farion1231/cc-switch">CC-Switch</a> model switching capabilities
</p>

---

## Overview

GitCortex is an AI-driven multi-terminal task coordination platform that enables multiple AI coding agents (Claude Code, Gemini CLI, Codex, etc.) to collaborate in parallel on complex software development tasks.

### Core Features

| Feature | Description |
|---------|-------------|
| **Main Agent Coordination** | AI-driven central controller responsible for task distribution, progress monitoring, and result review |
| **Multi-Task Parallelism** | Multiple Tasks execute simultaneously, each with independent Git branches |
| **Intra-Task Serial Execution** | Terminals within each Task execute sequentially (coding→review→fix) |
| **cc-switch Integration** | One-click model configuration switching for any CLI |
| **Event-Driven** | Workflow progression based on Git commits and message bus events, reducing unnecessary polling and context repetition |
| **Terminal Debug View** | Native terminal access for environment configuration verification after startup |
| **Workflow Persistence** | Complete three-layer data model: Workflow/Task/Terminal |
| **Slash Command System** | Reusable prompt presets with template variable substitution |
| **Multi-Model Support** | Support for Claude, Gemini, OpenAI, and other AI models |
| **Git Integration** | Deep Git integration with automatic branch and merge management |

### Architecture Overview

```
╔═══════════════════════════════════════════════════════════════════╗
║                     Orchestrator (Main Agent)                      ║
║      User Config: API Type + Base URL + API Key + Model           ║
╚═══════════════════════════════════════════════════════════════════╝
         │                      │                      │
         ▼                      ▼                      ▼
  ┌─────────────┐       ┌─────────────┐       ┌─────────────┐
  │   Task 1    │       │   Task 2    │       │   Task 3    │
  │ branch:login│       │ branch:i18n │       │ branch:theme│
  │  T1→T2→T3   │       │   TA→TB     │       │   TX→TY     │
  └─────────────┘       └─────────────┘       └─────────────┘
         ║                      ║                      ║
         ╚══════════════════════╩══════════════════════╝
                    Parallel Task Execution
                              │
                              ▼
  ┌─────────────────────────────────────────────────────────────────┐
  │                   Global Merge Terminal                          │
  └─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
                          [ main ]
```

### Key Collaboration Mechanisms (Critical)

> This section explains GitCortex's core value: **a single Orchestrator Agent coordinating multiple CLI terminals to complete complex tasks collaboratively**.

#### 1) Why "One Orchestrator" Instead of "Multiple Orchestrators"

In GitCortex, the Orchestrator is the sole global scheduler responsible for unified decision-making and progression, avoiding conflicts from multiple controllers issuing simultaneous commands.

It performs four main functions:

1. **Task Decomposition & Distribution**: Allocate workflow goals to different tasks.
2. **Terminal Serial Progression**: Launch next terminal within each task by `orderIndex`.
3. **State Machine Convergence**: Maintain unified three-layer state (workflow/task/terminal).
4. **Event Loop Closure**: Consume Git events, Prompt events, WS events and decide subsequent actions.

This means the "multi-terminal collaboration" you see is not chaotic concurrency, but **centralized orchestration + observable state machine**.

#### 2) Multi-CLI Collaboration Model (Horizontal)

GitCortex supports placing different CLIs in the same workflow:

- `claude-code` handles main development
- `codex` handles audit/fix suggestions
- `gemini-cli` handles documentation or test completion

They can operate in:

- **Task-level parallelism** (Task A / B / C run simultaneously)
- **Intra-task serial execution** (Terminal 1 → Terminal 2 → Terminal 3)

Achieving a combination strategy of "parallel acceleration + serial quality control".

#### 3) Multi-Model Collaboration with Same CLI (Vertical)

GitCortex doesn't require "one CLI corresponds to one model only".

You can use **the same AI CLI + different models** within the same task for role division, for example all using `claude-code`:

| Terminal | CLI | Model | Typical Role |
|---|---|---|---|
| T1 | `claude-code` | `glm-4.7` | Frontend implementation |
| T2 | `claude-code` | `claude-opus-4.6` | Backend implementation |
| T3 | `codex` | `gpt-5.3-codex-xhigh` | Code audit/convergence |

The value of this approach:

- Retain the same CLI's operational habits and context style
- Leverage different models' strengths in code generation, reasoning depth, and audit capabilities
- Ensure handoff order and state consistency through Orchestrator

#### 4) cc-switch's Role in Collaboration

`cc-switch` decouples "terminal instances" from "model configurations", allowing flexible model switching within the same CLI ecosystem:

- Write target model configuration before startup
- Maintain consistent model semantics for that terminal session after startup
- Support different terminals binding different models without mutual contamination

Therefore GitCortex supports two types of collaboration:

- **Cross-CLI collaboration** (Claude + Codex + Gemini)
- **Same-CLI multi-model collaboration** (e.g., multiple Claude Code terminals each bound to different models)

#### 5) How Complex Tasks Are Stably Advanced

In real development scenarios, a "complex task" is usually not generated in one pass, but through multiple closed loops:

1. Terminal A implements main logic first
2. Terminal B reviews and adds tests
3. Terminal C performs audit and risk convergence
4. Merge Terminal unifies merge to target branch

GitCortex's focus is not "single response quality", but making this closed-loop process repeatable, monitorable, replayable, and recoverable.

In other words, GitCortex provides **Agent collaboration pipeline capabilities**, not just "calling a certain model".

---

## Tech Stack

### Backend

- **Language & Runtime**: Rust + Tokio
- **Web Framework**: Axum (REST + WebSocket)
- **Data Layer**: SQLx + SQLite
- **Project Structure**: Rust Workspace (`crates/server`, `crates/services`, `crates/db`, `crates/cc-switch`, etc.)

### Frontend

- **Framework**: React 18 + TypeScript
- **Build Tool**: Vite
- **State & Data**: TanStack Query + WebSocket Store
- **Terminal Rendering**: xterm.js (terminal debugging and output display)

### Collaboration Runtime Components (Core)

- `OrchestratorRuntime`: Unified workflow lifecycle scheduling
- `OrchestratorAgent`: Execute orchestration decisions and state progression
- `MessageBus`: Cross-terminal/cross-module event bus
- `TerminalCoordinator`: Terminal preparation and serial progression coordination
- `TerminalLauncher`: Terminal process startup and lifecycle management
- `GitWatcher`: Monitor Git commits and trigger events
- `CCSwitchService`: CLI/model configuration switching and isolation

The above components can be found in `crates/services/src/services/` and `crates/server/src/routes/`.

---

## Deployment Guide

### Deployment Modes

- **Development Mode (Dual Service)**: Frontend dev server + backend API service run separately
  - Frontend: `23457`
  - Backend: `23456`
- **Production Mode (Single Service)**: Run backend binary only, backend serves both `/api` and frontend static resources

### Development Mode Deployment (Recommended for Local Development)

```bash
pnpm install

# Required: Set 32-character encryption key
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Prepare SQLx query cache as needed
npm run prepare-db

# Start frontend and backend
pnpm run dev
```

Access URLs:

- Frontend: `http://localhost:23457`
- Backend: `http://localhost:23456/api`

### Production Mode Deployment (Single Machine)

```bash
# 1) Install dependencies
pnpm install

# 2) Build frontend (for backend static resource embedding)
cd frontend && pnpm install && pnpm build && cd ..

# 3) Build backend
cargo build --release -p server

# 4) Set runtime environment variables
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="your-32-character-key"
$env:BACKEND_PORT="23456"   # Optional
$env:HOST="127.0.0.1"       # Optional, set to 0.0.0.0 for external access

# 5) Start service
# Windows
.\target\release\server.exe

# Linux/macOS
./target/release/server
```

Health check:

```bash
# When GITCORTEX_API_TOKEN is not enabled
curl http://127.0.0.1:23456/api/health

# When GITCORTEX_API_TOKEN is enabled (all /api routes require Bearer)
curl http://127.0.0.1:23456/api/health \
  -H "Authorization: Bearer <your-token>"
```

> For more complete operations, backup, upgrade, and rollback procedures, see: `docs/developed/ops/runbook.md` and `docs/developed/ops/troubleshooting.md`.

---

## Quick Start

### Prerequisites

| Tool | Version Requirement | Description |
|------|---------------------|-------------|
| **Rust** | nightly-2025-12-04 | Defined in `rust-toolchain.toml` |
| **Node.js** | >= 18 (recommend 20) | Frontend runtime |
| **pnpm** | 10.13.1 | Package manager |
| **CMake** | Latest | Build tool (required on some systems) |
| **SQLite** | 3.x | Database (usually built-in) |

### Installation

#### 1. Install Rust Toolchain

```bash
# Install Rustup
# Download: https://rustup.rs/ or use winget
winget install Rustlang.Rustup

# Install project-specified version
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04

# Install Cargo tools
cargo install cargo-watch
cargo install sqlx-cli --features sqlite

# Verify installation
rustc --version
# Should output: rustc 1.85.0-nightly (2025-12-04)
```

#### 2. Install Node.js and pnpm

```bash
# Recommend using nvm-windows
# Download: https://github.com/coreybutler/nvm-windows
nvm install 20
nvm use 20

# Install specified pnpm version
npm install -g pnpm@10.13.1

# Verify installation
pnpm --version
# Should output: 10.13.1
```

#### 3. Clone and Start Project

```bash
# Clone repository
git clone <your-repo-url>
cd GitCortex

# Install dependencies
pnpm install

# Set environment variable (required)
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Generate/verify SQLx query cache (as needed)
npm run prepare-db

# Build backend (Rust)
cargo build --release

# Start development servers (frontend + backend)
pnpm run dev
```

Access:
- Frontend: http://localhost:23457
- Backend API: http://localhost:23456/api

**Detailed Operations Manual:** See [Operations Manual](docs/developed/ops/runbook.md) for production deployment, monitoring, upgrades, and other detailed operations.

### Docker Deployment (Recommended for Clean Machines)

```bash
# Clone repository
git clone <your-repo-url>
cd GitCortex

# Optional: install AI CLIs during image build (default is 0 for build stability)
export INSTALL_AI_CLIS=0
# Optional: map host repositories into container workspace
export HOST_WORKSPACE_ROOT=../..
# Optional: enable API bearer auth in Docker only
export GITCORTEX_DOCKER_API_TOKEN=

# Build and start
docker compose -f docker/compose/docker-compose.yml build
docker compose -f docker/compose/docker-compose.yml up -d
```

PowerShell example:

```powershell
$env:INSTALL_AI_CLIS="0"
$env:HOST_WORKSPACE_ROOT="../.."
$env:GITCORTEX_DOCKER_API_TOKEN=""
docker compose -f docker/compose/docker-compose.yml build
docker compose -f docker/compose/docker-compose.yml up -d
```

Verify:

```bash
curl http://localhost:23456/readyz
docker compose -f docker/compose/docker-compose.yml ps
```

For detailed operations, backup, and troubleshooting, see:
- [Docker Deployment Guide](docs/developed/ops/docker-deployment.md)
- [Operations Manual](docs/developed/ops/runbook.md)

### Restore from Existing Repository

If you've already cloned the repository, just ensure tool versions are correct and reinstall dependencies:

```bash
cd GitCortex

# Check Rust version
rustc --version
# If version is incorrect, run:
rustup default nightly-2025-12-04

# Reinstall dependencies
pnpm install

# Set environment variable and start
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
pnpm run dev
```

---

## Development Environment Configuration

### IDE Recommendations

- **VS Code** + Extensions:
  - `rust-analyzer` (Rust language server)
  - `ESLint` (Frontend linting)
  - `Prettier` (Code formatting)

### Environment Variables

Create `.env` file or set system environment variables:

```bash
# Required: Encryption key (32-character string)
GITCORTEX_ENCRYPTION_KEY=your-32-character-key-here

# Optional
BACKEND_PORT=23456           # Backend port (default)
HOST=127.0.0.1               # Backend listen address (default)
GITCORTEX_API_TOKEN=your-api-token   # Enable API Bearer auth (optional)
```

### Database

Project uses SQLite (embedded), no database server installation required:
- Development default location: `dev_assets/db.sqlite`
- Migration files: `crates/db/migrations/`

### Verify Installation

```bash
# Backend compilation check
cargo check --workspace

# Frontend compilation check
cd frontend && npm run check && cd ..

# Run tests
cargo test --workspace
cd frontend && npm run test:run && cd ..
```

---

## Project Structure

```
GitCortex/
├── crates/                    # Rust workspace
│   ├── db/                    # Database layer (models + DAO + migrations)
│   ├── server/                # Axum backend server
│   ├── services/              # Business logic layer
│   │   ├── orchestrator/      # Main Agent orchestration logic
│   │   ├── terminal/          # Terminal process management
│   │   └── ...                # git_watcher.rs / cc_switch.rs etc.
│   └── utils/                 # Utility functions
├── frontend/                  # React + TypeScript frontend
│   ├── src/
│   │   ├── components/        # UI components
│   │   │   ├── workflow/      # Workflow wizard components
│   │   │   └── terminal/      # Terminal debug components
│   │   ├── hooks/             # React Hooks
│   │   ├── pages/             # Page components
│   │   └── i18n/              # Internationalization config
│   └── package.json
├── shared/                    # Frontend-backend shared types (auto-generated)
├── docs/                      # Documentation
│   ├── 已开发/                # Completed and stable documentation
│   └── 未开发/                # Pending and in-progress documentation
├── Cargo.toml                 # Workspace configuration
├── rust-toolchain.toml        # Rust version lock
├── package.json               # Root package.json
└── pnpm-workspace.yaml        # pnpm workspace config
```

---

## Development Progress

> **Data Source:** `docs/undeveloped/current/TODO.md` (README progress aligns with it)
> **Overall Status:** Completion rate **97.3%** (**288/296**), In Progress **0**, Not Started **8** (Phase 21: 2, Phase 27: 6), Optional Optimizations **5**.
> **Current Audit Score:** **100/100 (S-tier)**
> **Next Step:** Complete deferred/optional items in Phase 18.5 and Phase 21, and keep TODO milestones aligned with current implementation.

| Phase | Status (Aligned with TODO) | Notes |
|-------|----------------------------|-------|
| Phase 0 - Phase 18 | ✅ Completed | Core pipeline established |
| Phase 18.1 | ✅ Completed | Test technical debt cleanup complete |
| Phase 18.5 | 🚧 In Progress | P0 complete, P1/P2 have deferred items (including optional optimizations) |
| Phase 20 | ✅ Completed | Automated coordination core (auto-dispatch) |
| Phase 21 | ✅ Completed (with 2 not started) | 21.10 deferred, 21.12 optional |
| Phase 22 | ✅ Completed | WebSocket event broadcast improvements |
| Phase 23 | ✅ Completed | Terminal process isolation fix |
| Phase 24 | ✅ Completed | Terminal auto-confirm and message bridging |
| Phase 25 | ✅ Completed | Auto-confirm reliability fix |
| Phase 26 | ✅ Completed | Joint audit issue full fix |
| Phase 27 | 📋 To Implement | 6 tasks not started |

**Overall Progress:** 288/296 tasks completed (97.3%, per `docs/undeveloped/current/TODO.md`)

Detailed progress tracking: [docs/undeveloped/current/TODO.md](docs/undeveloped/current/TODO.md)

**Quality Status:** Per `docs/undeveloped/current/TODO.md`, currently recorded as S-tier (100/100).

---

## Current Real-World Validation Status

### Completed Development (Verified)

- The main agent can orchestrate and call multiple AI CLIs in one workflow.
- The current verified test setup uses three AI terminals:
  - Claude Code terminal with `glm-4.7`
  - Claude Code terminal with `claude-opus-4.6`
  - Codex terminal with `gpt-5.3-codex-xhigh`
- Docker single-container deployment is available and verified (`docker compose build --no-cache` + `up -d` + `readyz`).

### Not Completed / Not Fully Verified Yet

1. Multi-task parallel execution has not been fully tested.
2. Terminal count stress limit has not been tested yet (custom terminal count is supported).
3. Frontend UI issues are known and not fixed yet.
4. Merge terminal and error-resolution terminal flow has not been fully tested.
5. Task completion notification has not been developed.
6. Communication software integration has not been developed.
7. Slash command system has not been fully tested (currently likely unavailable or unstable).

### Tested End-to-End Demo Task

The validated demo task is a very simple local mini message board with separated frontend and backend:

- Backend: Python
- Frontend: Single HTML file
- Behavior: users input text on the page, click save, content is written into a local `.json` file, and the saved messages are displayed in a live-updated list below.

### Project Positioning and Collaboration

- This demo is intentionally simple and mainly proves that the full workflow can run through end-to-end.
- Complex tasks and long-run stability still need more validation.
- Current codebase is fully AI-generated.
- Community PRs are welcome. To avoid duplicated work, please open an issue before submitting a PR to claim the feature/task.

---

## Architecture Design

### Data Model

GitCortex uses a three-layer model:

1. **Workflow** - Top-level container
   - Contains multiple Tasks
   - Configures Orchestrator (main Agent)
   - Configures Merge Terminal
   - Optional Error Terminal

2. **WorkflowTask** - Mid-level unit
   - Each Task corresponds to a Git branch
   - Contains multiple Terminals
   - Independent state: pending → running → completed

3. **Terminal** - Bottom-level execution unit
   - Bound to specific CLI type (Claude/Gemini/Codex)
   - Bound to specific model configuration
   - Serial execution: not_started → starting → waiting → working → completed (can reach failed/cancelled on exception)

### State Machine

**Workflow State Transitions:**
```
created → starting → ready → running → (paused) → merging → completed/failed
                                              ↓
                                          cancelled
```

**Terminal State Transitions:**
```
not_started → starting → waiting → working → completed
                                         ↓
                                      failed/cancelled
```

### Core Services

| Service | Responsibility |
|---------|----------------|
| **OrchestratorAgent** | Main Agent, responsible for task distribution, progress monitoring, result review |
| **MessageBus** | Cross-terminal message routing |
| **TerminalLauncher** | Terminal process startup and management |
| **GitWatcher** | Monitor Git events (.git/refs/heads changes) |
| **CCSwitchService** | Model configuration switching (atomic config file writes) |
| **Workflow API + DB Models** | Workflow CRUD and state management (`routes/workflows.rs` + `db/models/workflow*.rs`) |

---

## Supported CLIs

| CLI | Name | Detection Command | Config File Path |
|-----|------|-------------------|------------------|
| Claude Code | Claude Code | `claude --version` | `~/.claude/settings.json` |
| Gemini CLI | Gemini | `gemini --version` | `~/.gemini/.env` |
| Codex | Codex | `codex --version` | `~/.codex/auth.json`, `~/.codex/config.toml` |
| Amp | Amp | `amp --version` | - |
| Cursor Agent | Cursor | `cursor --version` | - |
| Qwen Code | Qwen | `qwen --version` | - |
| GitHub Copilot | Copilot | `gh copilot --version` | - |
| Droid | Droid | `droid --version` | - |
| Opencode | Opencode | `opencode --version` | - |

### Model Switching

CC-Switch provides atomic write mechanism for safe CLI model configuration switching:

- ✅ Support configuring multiple CLIs simultaneously
- ✅ Temporary switching (single workflow)
- ✅ Permanent switching (modify config files)
- ✅ Automatic config backup
- ✅ Model availability verification

---

## Usage Guide

### Create Workflow

1. Click "New Workflow"
2. Select project
3. Configure basic information
4. Add tasks and terminals
5. Select models and CLIs
6. Start workflow

### Operations

For production deployment, database management, monitoring, and troubleshooting, see:

- **Operations Manual:** [docs/developed/ops/runbook.md](docs/developed/ops/runbook.md)
  - Start server (development/production mode)
  - Database management (backup/restore/migration)
  - Monitoring and performance tuning
  - Upgrade and rollback procedures

- **Troubleshooting:** [docs/developed/ops/troubleshooting.md](docs/developed/ops/troubleshooting.md)
  - Server won't start
  - Workflow stuck
  - API key issues
  - Terminal no output
  - Database locked

### Testing & Building

```bash
# Run tests
cargo test --workspace
cd frontend && npm run test:run && cd ..

# Build production version (frontend + backend)
cd frontend && npm run build && cd ..
cargo build --release -p server

# Type generation
pnpm run generate-types
pnpm run generate-types:check
```

---

## Workspace Create Page Manual (`/workspaces/create`)

### What this page is for

This page is the **quick-start entry for execution**.  
You provide task intent, bind repositories/branches, and create a runnable workspace directly.

### How to read the page layout (left / middle / right)

1. **Left: Workspaces**
   - Existing workspaces list (active/history).
   - Used for context switching, not for creation config.
2. **Middle: Task input box**
   - Input the task description (chat-style form).
   - Select executor/model variant and optionally attach images.
   - Click create to start a new workspace.
3. **Right: Project / Repositories / Add repositories**
   - Select project.
   - Add repositories for this run.
   - Select target branch per repository.

### Does it conflict with `/wizard`?

No. They are parallel entry points with different goals:

- `/workspaces/create`: fast single-workspace creation and immediate execution.
- `/wizard`: workflow orchestration and staged pipeline management.

### Minimal operation flow

1. Select project on the right panel.
2. Add at least one repository and confirm target branch.
3. Enter task description in the middle input.
4. Click **Create Workspace**.

### Common confusion and clarification

- “Bottom dialog”: it is not a popup; it is the main creation input area.
- “Left vs right panels”: left is workspace switching, right is creation context binding.
- If create button is unavailable, usually task text or repository selection is missing.

---

## Documentation

### Implementation Plans

- [Overall Overview](docs/developed/plans/00-overview.md)
- [未开发目录](docs/未开发)
- [Latest Progress Tracking (authoritative)](docs/undeveloped/current/TODO.md)

### Core Design Documents

- [Orchestrator Architecture Design](docs/developed/plans/2026-01-16-orchestrator-design.md)
- [GitCortex Detailed Implementation Plan](docs/developed/plans/2026-01-16-gitcortex-implementation.md)

### Progress Tracking

- [Development Progress Tracker](docs/undeveloped/current/TODO.md)

---

## FAQ

### Q: Compilation fails, can't find nightly version?

Ensure correct Rust version is installed:

```bash
rustup install nightly-2025-12-04
rustup default nightly-2025-12-04
```

### Q: Workflow creation fails, encryption key error?

Ensure environment variable is set:

```bash
# Windows PowerShell
$env:GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"

# Linux/macOS
export GITCORTEX_ENCRYPTION_KEY="12345678901234567890123456789012"
```

### Q: CLI detection fails, shows not installed?

Ensure CLI is installed and findable in PATH:

```bash
claude --version
gemini --version
codex --version
```

### Q: Browserslist warning during testing?

Update Browserslist database:

```bash
pnpm dlx browserslist@latest --update-db
```

---

## Contributing

Issues and Pull Requests are welcome!

### Development Standards

- **Rust Code**: Follow `cargo fmt` and `cargo clippy` standards
- **Frontend Code**: Use ESLint + Prettier, strict mode
- **Commit Messages**: Use Conventional Commits

### Code Quality Standards

Current quality status per `docs/undeveloped/current/TODO.md`: **100/100 (S-tier)**.

Recommended before each release:

- `cargo check --workspace`
- `cargo test --workspace`
- `npm run check`
- `cd frontend && npm run test:run`

---

## Acknowledgments

This project is based on the following excellent open source projects:

- **[Vibe Kanban](https://github.com/BloopAI/vibe-kanban)** - AI coding agent task management platform (Apache 2.0)
- **[CC-Switch](https://github.com/farion1231/cc-switch)** - Claude Code/Codex/Gemini CLI configuration switching tool (MIT)

Thanks to the authors and contributors of these projects!

---

## License

This project follows the open source licenses of upstream projects:

- Vibe Kanban portion: Apache License 2.0
- CC-Switch portion: MIT License

See [LICENSE](LICENSE) file for details.

---

<p align="center">
  <em>GitCortex - Making AI Agents Work Together</em>
</p>
