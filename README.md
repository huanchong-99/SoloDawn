<p align="center">
  <img src="installer/assets/solodawn.png" alt="SoloDawn" width="120" />
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

SoloDawn is an open-source web app that runs on your own machine (Rust backend + React frontend): an upper-layer orchestrator Agent (the primary Agent) commands the **AI CLIs actually installed on your computer** (Claude Code, Codex — 8 in total) to carry out fully automated development inside a Git repository — requirement clarification → technical spec generation → task decomposition → parallel development on isolated branches → three-layer quality gates → scored acceptance review → automatic merge.

SoloDawn's ultimate design goal is to **complete complex, production-grade products through a simple conversation on a social platform** — not toy demos, but real, complex, production-ready software.

> **One-liner:** Whether you're a programmer or not, just describe what you need. Everything else is automatic.

> **About AI hallucination, honestly:** SoloDawn cannot stop a model from hallucinating at the moment it emits tokens — nobody can. Through architecture and workflow design it catches and repairs hallucination artifacts before delivery: 31 built-in quality rules + three quality gates + a self-healing loop + a 90-point acceptance bar, so hallucinations, security holes, and integration conflicts each get intercepted on a different dimension. See [Quality System in Depth](#quality-system-in-depth).

---

## Two Core Highlights

### Highlight 1: Fully Automated AI Development

The Orchestrated Workspace uses a **two-layer Agent architecture**: a primary Agent plus child Agents — not three layers, and no child Agent is preconfigured. Every child Agent is dynamically spawned and terminated by the primary Agent: whenever the work needs more hands (additional tasks, code review, defect fixing, integration repair), the primary Agent creates new child Agents — exactly as many as required, closed when done. This design is fully implemented.

Why abandon fixed workflow definitions? Because rigid workflows introduce hard constraints: they're overly cumbersome for simple tasks yet lack capacity for complex ones, and the conditional-judgment logic inside a workflow inevitably misjudges. So the two-layer architecture grants full authority to the primary Agent — it alone decides what to spawn, how many, and when to close them.

AI-assisted coding today has a fundamental contradiction: programmers still have to build workflows, configure skills, wire up MCP servers, and write plans themselves — the human remains the driver; non-programmers can't even turn a requirement into a technical spec. SoloDawn takes the driving over from the human:

| Who you are | What you do | What the system does |
|---|---|---|
| **Programmer** | Throw in a precise task goal | Your input is used directly as the technical spec and executed — no follow-up questions. No commands to issue, no "Continue" to click; just wait for acceptance |
| **Non-programmer** | Describe what you want in plain language | Plain-language follow-up questions close the gaps → a technical spec + acceptance scoring rubric are generated in the background → fully automated development begins |

Delivery isn't "it ran, so it's done": every commit passes quality gates, and every completed task is scored item-by-item against a **rubric generated from your requirements** — **below 90 points it is automatically sent back for rework**. The rubric stays in force for the entire workflow, ensuring the final delivery matches what you originally asked for.

**The best partner for a one-person company:** with this project, you essentially have an entire professional development team.

### Highlight 2: Native Skill / MCP / Plugin Support — in Both Manual Workflows and Fully Automated AI Development

Whichever mode you use, SoloDawn launches the **real CLI processes installed on your machine** (native PTY — not an API re-wrap), and prompts reach the CLI verbatim — zero interception, zero rewriting. Whatever tools you have configured are exactly the tools available here — and the inheritance goes beyond your own tools: **the CLI's official built-in commands are inherited too, including UltraCode mode**.

- **Zero migration for skills, plugins, MCP servers, and slash commands** — no re-configuration inside SoloDawn; they're inherited as-is, and you're free to use and change them however you like;
- **All official built-in commands inherited (including UltraCode)** — in a manual workflow, configure a dedicated prompt for a task to enable UltraCode: it generates standardized workflow scripts that hardcode clear capability boundaries for each Agent; afterwards, invoking that script reuses the entire workflow;
- **Full automation or manual control — both work** — the Orchestrated Workspace hands everything to AI; the Manual Workflow lets you customize every detail of the workflow graph: how many terminals, what roles, which models, which slash commands;
- **8 AI CLIs collaborating inside one workflow** — e.g. a Claude Code terminal running a GLM model as the developer and a Codex terminal running a GPT model as the auditor.

**This project offers extremely high flexibility with nearly unlimited room for exploration: 8 AI CLIs × your skills × your MCP servers × any plugins, freely combined — ten different users will come up with a hundred unique ways to use it. That is a critical advantage for every user.**

Step-by-step instructions for both modes are at the bottom of this file: [Usage Guide: Orchestrated Workspace](#usage-guide-orchestrated-workspace) · [Usage Guide: Manual Workflow](#usage-guide-manual-workflow).

---

## Feature Overview

### Orchestration & Execution
- ✅ **Two-layer Agent architecture**: the primary Agent holds full decision authority; child Agents are zero-preconfigured — dynamically spawned and closed on demand, with more created whenever review / fixes / additional tasks need more hands
- ✅ Primary (orchestrator) Agent commanding the full workflow lifecycle; Git-driven event loop saves 98%+ tokens vs polling
- ✅ Two work modes: Orchestrated Workspace (Agent-planned, fully automated) / Manual Workflow (DIY)
- ✅ Orchestrated Workspace sub-modes: Direct Execution (precise input) / Guided Conversation (plain-language clarification)
- ✅ Multi-task parallel execution: per-task Git branch + isolated worktree, up to 10 concurrent workflows by default
- ✅ Planning Draft lifecycle: gathering → spec_ready → confirmed → materialized
- ✅ **Continuation rounds** (landed post-1.0): a delivered conversation continues in place — round N+1 plans only the delta, prior rounds fold into the same thread, one active round per project
- ✅ **Requirement ledger (评分点)**: acceptance criteria become project-scoped points (`RP-001`, …) settled at scoring time — delivered points store a compressed context capsule, regressions are flagged per point
- ✅ **Architecture-aware planning** (landed post-1.0): an architecture-thinking checklist plus reference-architecture digests, keyword-matched from a locally synced knowledge base (built-in source: awesome-architecture), are injected into the orchestrator when a round materializes; sources are user-extensible GitHub repos with automatic background refresh
- ✅ **Design styles** (landed post-1.0): pick a visual direction per round in the workspace, or set a global default in Settings — 6 built-in presets adapted from high-rated open-source design skills, plus full custom style create / edit / delete; the chosen style is carried into every UI-related terminal instruction
- ✅ Cross-terminal context handoff (each terminal's work passed to the next)
- ✅ Automatic branch merging + a designated conflict-resolution terminal; optional "run tests before merge" and "pause on conflict"

### Quality & Reliability
- ✅ **Three-layer quality gates**: Terminal (every commit, 16 blocking conditions) → Branch (task completion, 18) → Repo (pre-merge, 23)
- ✅ Built-in rules engine with **31 rules** (13 Rust / 11 TS / 7 common), zero dependencies, optional SonarQube integration
- ✅ Four enforcement modes off / shadow / warn / enforce (1.0 ships with enforce); 11 independently toggleable analyzers
- ✅ Self-healing loop: gate fails → structured fix instructions sent back → terminal self-corrects and re-commits → automatic re-check (max 10 stall recoveries per terminal)
- ✅ Acceptance review: per-project scoring rubric, 90-point pass bar (70 for foundation tasks), up to 5 automatic rework rounds
- ✅ Custom quality rules + AI natural-language rule generation (adversarial validation / sample testing / human confirmation)
- ✅ Secret-leak detection (11 pattern classes), weak-default-credential detection, test-authenticity checks (catches fake/missing tests)
- ✅ Policy snapshots and issue tracking; LLM fault tolerance and graceful degradation; multi-provider circuit breaker with failover; state persistence with crash recovery

### CLI & Model Support
- ✅ 8 AI CLIs; mixed CLI types within one task, each terminal taking a different role (developer / auditor / …)
- ✅ Provider/model switching within one CLI via CC-Switch
- ✅ Five model interface types: Anthropic / Google / OpenAI / Anthropic-compatible / OpenAI-compatible (custom base URLs, relays work)
- ✅ Per-terminal environment variable injection; MCP server config adapted per CLI
- ✅ Claude Code billing guarantee: subscription users consume only their plan quota (see [the billing deep-dive](#claude-code-no--p-interactive-transport--billing-guarantee))

### Experience & Integrations
- ✅ Web pseudo-terminal (xterm.js + native PTY) for real-time debugging and interaction
- ✅ Slash commands: 6 built-in presets + custom commands, delivered to the primary Agent which recognizes and forwards them to the right terminal — adapting third-party plugins
- ✅ All official built-in CLI commands inherited (including UltraCode mode): enable via a dedicated prompt in manual workflows to generate reusable, capability-bounded standardized workflow scripts
- ✅ Setup Wizard for first-run onboarding; automatic runtime-environment and installed-CLI detection
- ✅ Internationalization: 6 languages (English, 简体中文, 繁體中文, 日本語, Español, 한국어)
- ✅ Telegram connector; Feishu (Lark) long-lived WebSocket connector (not re-tested for 1.0, see [Current Limitations & Roadmap](#current-limitations--roadmap))

### Deployment & Operations
- ✅ Docker one-click deployment (interactive installer script); split deployment architecture (Server + Runner + Redis, over gRPC)
- ✅ Provider health monitoring API; Sentry error tracking + PostHog analytics
- ✅ AES-256-GCM encryption for API keys at rest

---

## How It Works

Four core design principles:

- **Upper-layer orchestration, not code generation.** The orchestrator Agent never writes code — it commands the best professional AI CLIs (Claude Code, Codex, Amp, Cursor Agent, etc.) to do the work.
- **Two layers of Agents, no fixed workflows.** Just a primary Agent and child Agents — none preconfigured, no rigid workflow definitions. Fixed workflows are too heavy for simple tasks, too small for complex ones, and their conditional logic inevitably misjudges — so full decision authority lives with the primary Agent, which spawns and closes child Agents dynamically.
- **Non-invasive by design.** SoloDawn doesn't replace any CLI, modify any config, or define new tools. It inherits the full native ecosystem of every CLI — all slash commands, plugins, skills, and MCP servers work unchanged.
- **Git-driven event loop.** The orchestrator only consumes LLM tokens when a Git commit event occurs; between events it sleeps at zero cost — saving 98%+ tokens compared to polling.

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

**Two-layer Agent architecture (Orchestrated Workspace):**

- **Primary Agent (orchestrator)** → one per workflow, holding full decision authority: decomposes the spec, dynamically creates/starts/closes child Agents, parses Git events, routes review and fix cycles
- **Child Agents** → zero preconfigured; all created and terminated by the primary Agent at runtime. Each one materializes as a terminal — a native AI CLI process (PTY) on your machine. New ones can be spawned at any point mid-run: code review, defect fixing, integration repair, and follow-up tasks all get fresh child Agents (concurrency is capped by a global terminal limit; excess dispatches queue and drain as slots free)

**Git-side execution structure:**

- **Workflow** → the primary Agent manages the entire lifecycle (up to 10 concurrent workflows by default)
- **Task** → independent Git branch (`workflow/{workflow-id}/{task-name}`) + isolated worktree, runs in parallel with other tasks
- **Terminal** → the runtime vehicle of a child Agent, runs serially within its task, gated by quality checks

**Code passes four checkpoints between generation and main:** every commit passes the Terminal Gate (changed-files scope) → a finished task passes the Branch Gate (branch scope) → the acceptance review scores it against the rubric (≥ 90 proceeds, otherwise sent back for self-repair) → the Repo Gate runs before merge (whole-repository scope). Whenever a checkpoint fails, structured fix instructions go back to the same terminal automatically — it repairs, re-commits, and gets re-checked with no human in the loop. Details in [Quality System in Depth](#quality-system-in-depth) and [Acceptance Review & Scoring Rubric](#acceptance-review--scoring-rubric).

**Key components:**

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
| Codex | ✅ Supported | ✅ Via CC-Switch | Codex adapter |
| Amp | ✅ Supported | — | Passthrough |
| Cursor Agent | ✅ Supported | — | Cursor adapter |
| Qwen Code | ✅ Supported | — | — |
| GitHub Copilot | ✅ Supported | — | Copilot adapter |
| Droid | ✅ Supported | — | Passthrough |
| Opencode | ✅ Supported | — | Opencode adapter |

> Gemini CLI support was removed after Google deprecated it for consumers in favor of the [Antigravity CLI](https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/).

Any CLI that runs in a terminal and supports slash commands can be integrated.

> The 1.0 48-hour acceptance run used Claude Code throughout; the testing status of the other CLIs and multi-CLI collaboration is in [Current Limitations & Roadmap](#current-limitations--roadmap). Claude Code subscription users should read the [billing guarantee](#claude-code-no--p-interactive-transport--billing-guarantee).

---

## V1.0 Test Results

SoloDawn V1.0 was validated by a **48-hour fully-autonomous, self-healing end-to-end test** (2026-06-27 → 06-30): seven real tasks executed serially through the browser UI, with a Stop-hook driver that diagnosed and fixed orchestrator root causes *during* the run — zero human intervention.

The seven tasks span five development forms — **greenfield builds, extending an existing codebase, cross-language migration, legacy refactoring, and security/performance/monitoring hardening** — a full-chain validation covering the core working scenarios of web/backend developers.

**Final grade: 88.85 / A — 7 of 7 tasks delivered.**

| # | Task | Form | Repo | Score | Grade |
|---|------|------|------|:---:|:---:|
| 1 | Knowledge-base app | greenfield | `knowledge-base-demo` | 81 | B |
| 2 | Hoppscotch load-testing module | existing-repo extension | `hoppscotch-demo` | 88 | A |
| 3 | Express → Rust rewrite | cross-language migration | `express-to-rust-demo` | 93 | A |
| 4 | Refactor + test backfill | legacy refactor | `refactor-test-demo` | 92 | A |
| 5 | Microservice e-commerce | greenfield | `ecommerce-demo` | 91 | A |
| 6 | Security + performance + monitoring | hardening | `kutt-security-demo` | 86 | A |
| 7 | Feishu memo app | greenfield | `web-memo-demo` | 91 | A |

Scoring dimensions: buildability (20) / functionality (25) / code quality (30) / tests (15) / engineering (10), weighted by task complexity. Grades: S≥95 · A≥85 · B≥70.

The test itself was a stress test of SoloDawn's self-healing. The 48-hour run surfaced and fixed **21 orchestrator deadlock / stall root causes** (the "§8" chain, #1–#21) — each one a scenario where a naive implementation loops forever. Every fix was deployed and re-validated live before continuing. Full report: `docs/undeveloped/current/V1.0-质量验收-2026-06-30-48h自修复测试.md`.

> Model: `glm-5.2[1m]` via solodawn.cloud (Anthropic protocol). Test harness: browser-MCP UI, serial execution, /goal + Stop-hook self-repair.

---

## Quick Start

### 🤖 Prompt for AI Assistants — "Run It For Me"

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
6. When the Setup Wizard appears, STOP there and hand the model configuration over to me — I will set up the AI model myself in the UI.

If a build step fails, read the error, install the missing prerequisite, and retry. Only edit SoloDawn's own files if a step truly requires it.
```

**Run vs Develop — which one are you?**

| | Run SoloDawn (use it) | Develop SoloDawn (change its code) |
|---|---|---|
| What you do | `git clone` → `pnpm run dev` → open the web UI | everything at left, plus edit Rust/TS source |
| Prerequisites | the build toolchain below (backend compiles once) | + `sqlx-cli` 0.8.6, linter, full test tooling |
| Cache footprint | modest `target/` (server binary + deps only) | large `target/` (whole-workspace test/clippy/codegen) |
| See | the rest of this section | [Contributing](#contributing) |

> **1.0 ships source-run** — there is **no installer of any kind**. Clone and run with `pnpm run dev` (or build a release binary). The `installer/` directory and the `Build Windows Installer` workflow are retained from earlier development; 1.0 does not publish a Windows installer.

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

> If unset, SoloDawn auto-generates and reuses a persistent per-machine key file (`~/.enckey`). That's fine for single-machine use; for multi-host or container deployments, set the variable explicitly — encrypted data can't be decrypted across machines otherwise.

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
- **Health checks:** `GET /healthz` (liveness) · `GET /readyz` (readiness, incl. DB/dirs/Feishu status)

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

> ⚠️ Release binaries **require `SOLODAWN_API_TOKEN`** by default (API authentication, preventing unauthenticated access) and refuse to start without it; for localhost-only use, set `SOLODAWN_LOCAL_MODE=1` to skip the check.

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

> **📖 Supplementary material below** — step-by-step usage guides for both modes, full breakdowns of the quality system and acceptance review, and reference tables. Read the core sections above first, then dip in as needed.

---

## Usage Guide: Orchestrated Workspace

The flow in one sentence: configure a model → bind a Git repository → (optionally) tune the quality gates → describe what you need → click Confirm in the top-right → wait for delivery.

**Step 0: Configure a model.** First launch opens the Setup Wizard (everything is also editable later in Settings); a model must be configured before any work can start. Two ways:

- **Native mode**: auto-detects your local Claude Code subscription login (Pro or Max), works with zero configuration, and guarantees only your plan quota is consumed (see the [billing guarantee](#claude-code-no--p-interactive-transport--billing-guarantee));
- **Manual mode**: enter an API key. Five interface types are supported — **Anthropic / Google / OpenAI / Anthropic-compatible / OpenAI-compatible** (the compatible types accept a custom base URL, so relay endpoints work). All keys are stored AES-256-GCM encrypted.

**Step 1: Pick a repository.** The screen auto-detects existing Git repositories on your machine; if you have none, it can create one for you. After selecting, you must **click a second time to confirm** — it counts only once "bound" is shown. The project must live in a Git repository: task branches, worktree isolation, quality gates, and auto-merge are all built on Git.

**Step 2 (optional): Customize the quality gates.** From this screen you can open the "Quality Gate Rules" panel: the four enforcement modes, every blocking condition of the three gates (metric + threshold), and 11 analyzer toggles are all editable — you can even have AI generate a rule from natural language. Confused by it all? That's fine — **our advice: don't touch it unless you have special needs.** The defaults already cover most scenarios, and AI-generated rule patterns can be too broad or too narrow — the result may well be worse than stock.

**Step 3: Describe what you need.** In the conversation screen, pick the terminal and model for this run (Claude Code, Codex, …), then proceed according to who you are:

- **Programmer**: throw in a precise, specific task goal. The system uses your input directly as the technical spec, asks nothing, and only generates the acceptance scoring rubric in the background.
- **Non-technical user**: skip the jargon entirely; describe the need in plain language from a user's perspective — "I want a local memo app; I'd like to view it in my browser, or have it be a desktop app that stays pinned on top…" The system asks plain-language follow-ups to close the gaps (you mention 3 features, it may ask "want to add these other two?"), and once the final requirement is confirmed it generates a technical spec + acceptance scoring rubric.

Building something with a UI? Pick a **design style** from the toolbar next to the model selector — or leave it on the system default from Settings → Design Styles (see [Architecture Knowledge & Design Styles](#architecture-knowledge--design-styles)).

Want your own acceptance criteria? Upload an audit document **before** confirming (Builtin / Merged / Custom modes — see [Acceptance Review & Scoring Rubric](#acceptance-review--scoring-rubric)); after confirmation the rubric panel becomes read-only.

**Step 4: Click Confirm in the top-right.** Confirmation is two-step: first confirm the technical spec (the scoring rubric is generated and locked at that instant), then confirm the quality gate configuration. The generated rubric immediately appears as a card in the conversation — every acceptance criterion tagged with its requirement-point code (`RP-001`, …) — and the requirement ledger opens as a side panel next to the audit-doc panel. **Remember to click Confirm when the conversation is done — nothing executes otherwise.** This is a hard block in the code: only after both confirmations does the workflow materialize and start automatically.

**Step 5: Wait for delivery.** Everything from here is automatic: the primary Agent decomposes the spec into tasks and spawns child Agents on demand (nothing is preconfigured — when review, fixes, or integration repair need more hands it simply creates more, closing them when done), opens branches, and drives them in parallel; every commit passes the Terminal Gate, every finished task passes the Branch Gate, and the acceptance review releases a task at ≥ 90 points (otherwise it's sent back automatically, up to 5 rounds); when all tasks complete, branches merge back to main automatically, with conflicts handled by your designated merge terminal — or configure "pause on conflict" for manual intervention.

**Step 6: Keep iterating — rounds.** Delivery no longer ends the conversation: once this round's workflow settles, a **Continue** button appears in the same thread. Round N+1 clarifies only the new requirement — the planner starts from the requirement ledger (delivered points + their context capsules) instead of re-reading the project, previously delivered features are protected by regression assertions in the new rubric, and only one round per project runs at a time. Prior rounds stay in the thread as collapsible dividers, their rubric snapshots included.

## Usage Guide: Manual Workflow

For users who want full control over the workflow graph. The creation wizard has 7 steps:

1. **Pick a repository** (Step 0): auto-detects local Git repositories; if it says "none detected", just hit refresh.
2. **Basics** (Step 1): name and describe this workflow, and set how many tasks to run in parallel this time.
3. **Define tasks** (Step 2): split the work into tasks; each task will get its own branch and worktree.
4. **Configure models** (Step 3): maintain the model library (provider / base URL / key). Anything configured in global Settings is read directly here; you can also configure models just for this workflow.
5. **Configure terminals** (Step 4): the page detects your machine's runtime environment and installed AI CLIs (scroll down on this page to see it). Then, for each task, choose which terminal (CLI) to use, which model that terminal runs, and a role description for it. **This is where multi-CLI collaboration happens** — e.g., a Claude Code terminal running a GLM model as the developer, and a Codex terminal running a GPT model as the auditor.
6. **Slash commands** (Step 5): enable slash commands for the workflow. In an Agent-Planned run, commands are not sent to a terminal directly — they are delivered to the primary Agent, which recognizes them and forwards them to the right terminal on its own as the run proceeds (the entire workflow is under the primary Agent's control anyway). In a DIY run there is no primary Agent — each enabled command is rendered and typed into every task's terminal automatically, right after that task's description. Six presets ship built-in (write-code / review / fix-issues / test / refactor / document) and you can add your own — plugin marketplaces are full of plugins, each with its own command; add yours here and the primary Agent can invoke your plugin during the run.
7. **Advanced** (Step 6): choose which AI coordinates the multi-task run (the orchestrator model), and which terminal + model resolves conflicts and completes the merge when branches come together; you can also toggle "run tests before merge" and "pause on conflict".

> **Power move: UltraCode.** A manual workflow launches your native terminal, so beyond your skills / MCP servers / plugins, the CLI's official built-in commands are inherited too — including UltraCode mode. Enable it by configuring a dedicated prompt for a task (the task description is typed into that task's terminal verbatim): UltraCode generates standardized workflow scripts that hardcode clear capability boundaries for each Agent, and invoking that script afterwards reuses the entire workflow.

Manual workflow video demo (recorded February 2026 when the project was still called GitCortex; it covers only the manual workflow and the UI has since changed): [GitCortex minimal MVP demo — bilibili](https://www.bilibili.com/video/BV1yxfMBCEFh/)

## Architecture Knowledge & Design Styles

Two additions that shape *how* the orchestrated workspace plans and builds.

### Architecture-aware planning

When a round materializes, the planner's goal is enriched with an **Architecture Guidance** section built from two parts:

- **A self-answered architecture checklist** (adapted from [study8677/architecture-copilot](https://github.com/study8677/architecture-copilot), MIT): system boundaries → data model → sync/async flows → capacity honesty. The orchestrator answers it while decomposing, so the plan states its architecture assumptions instead of hiding them.
- **Matched reference digests**: SoloDawn keeps a local knowledge base synced from GitHub — the built-in source is [study8677/awesome-architecture](https://github.com/study8677/awesome-architecture) (MIT), a set of uniformly structured reference architectures. At confirm time, the requirement text is keyword-matched against the entries and the top digests (key decisions / tradeoffs / scaling / anti-patterns) are attached.

**Settings → Architecture Knowledge** lets you toggle the guidance, add your own GitHub repositories as knowledge sources (markdown files under chosen path prefixes), trigger a manual sync, and see per-source sync status. Background sync checks roughly every 6 hours and refreshes any source more than 24 hours stale; only changed files are fetched (blob-SHA diff). Setting `SOLODAWN_GITHUB_TOKEN` (or `GITHUB_TOKEN`) raises the GitHub API rate limit for private or heavily synced sources.

### Design styles

Pick a **design style** in the workspace conversation toolbar (per round), or set a global default in **Settings → Design Styles**. When a round materializes with a style, its directives are appended to the goal as a **Design Direction** section, and the orchestrator contract requires carrying it into **every UI-related terminal instruction** — including the foundation task that lays down base styles, so the visual language stays consistent across parallel terminals.

Six presets ship built-in, condensed from high-rated open-source design skills (each file carries source and license attribution — see `LICENSE`):

| Preset | Adapted from | License |
|---|---|---|
| Anthropic Frontend Design | anthropics/skills — frontend-design | Apache-2.0 |
| Minimalist Editorial | Leonxlnx/taste-skill — minimalist-ui | MIT |
| Industrial Brutalist | Leonxlnx/taste-skill — industrial-brutalist-ui | MIT |
| Soft Premium | Leonxlnx/taste-skill — soft-skill | MIT |
| Impeccable Design Language | pbakaus/impeccable | Apache-2.0 |
| Emil Design Engineering | emilkowalski/skills — emil-design-eng | MIT |

Built-in presets are read-only — duplicate one to make it yours; custom styles support full create / edit / delete, and disabled styles are never injected.

## Quality System in Depth

The core principle: **fix errors at the earliest possible moment, in the smallest possible scope.** Changed files are checked at commit time (Terminal Gate), the whole branch at task completion (Branch Gate), and the whole repository before merge (Repo Gate) — hallucinations, security holes, and integration conflicts each get intercepted on a different dimension, and the self-healing loop eliminates them before they reach the main branch.

### Built-in rules engine (31 rules, zero dependencies)

The rules engine is modeled on SonarQube's quality model (the quality gate model is ported from SonarQube, LGPL-3.0) and **runs without installing SonarQube**; the Repo Gate can optionally hook up a real SonarQube for deep analysis. The 31 built-in rules come in three groups:

| Group | Count | Coverage |
|---|---|---|
| **Rust** | 13 | cyclomatic complexity, cognitive complexity, function/file length, nesting depth, error handling (`unwrap`/`expect`/`panic!` outside tests), `unsafe` usage, `clone` abuse, naming conventions, missing docs on public items, type complexity, leftover TODOs, magic numbers |
| **TypeScript / JavaScript** | 11 | cyclomatic complexity, function/file length, nesting depth, `any` abuse, `as` type assertions, leftover `console` calls, naming conventions, React Hooks misuse, import order, leftover TODOs |
| **Language-agnostic** | 7 | code duplication, hardcoded secret detection (11 pattern classes: AWS / GitHub / Slack / Google / Stripe / DB connection strings / npm tokens …), weak default credentials (`admin` / `password` / `changeme` / `123456` / weak JWT_SECRET), oversized files, overlong lines, trailing whitespace, file encoding |

Four severity levels: **Blocker / Critical / Major / Minor**. The classic AI hallucination artifacts — hardcoded secrets, weak passwords, type bypasses (`any` / `as` / `unsafe`), spaghetti code (complexity / nesting) — are all covered.

Beyond the rules engine, the gates also drive a set of independent analyzers directly: `cargo check` / `clippy` / `tsc` / test runs, coverage measurement, secret scanning, ReDoS risk detection, **test-authenticity checks** (built to catch "no tests written", "fake tests", and "hollow tests"), project-convention checks, runtime security smells, and more.

### The three quality gates

| Gate | Trigger | Scope | Blocking conditions |
|------|---------|-------|---------------------|
| **Terminal Gate** | Every checkpoint commit | Changed files only | 16 — zero compile/type/test errors, zero Critical built-in rule hits, secret leaks, missing test files, fake tests, ReDoS risks, … |
| **Branch Gate** | Last terminal in a task completes | Full task branch | 18 — everything in the Terminal Gate + clippy warnings, formatting, line coverage ≥ 60%, cyclomatic complexity ≤ 25, duplicated blocks ≤ 5, TODO density capped |
| **Repo Gate** | Before merge to main / CI | Whole repository | 23 — Branch Gate tightened across the board + zero SonarQube Blocker/Critical issues, line coverage ≥ 80%, duplicated blocks ≤ 3 |

**Four enforcement modes:** `off` → `shadow` (analyze and log, never block) → `warn` (feed issues back, don't block) → `enforce` (hard gate). **1.0 ships with `enforce`.** The 11 analyzers (Rust / frontend / repo / security / Sonar / built-in rules ×3 / coverage / completeness / delivery-readiness) can be toggled independently.

### The self-healing loop

When a gate fails, structured fix instructions are automatically sent back to **the same terminal**: it fixes the issues → re-commits → the gate re-runs automatically, no human in the loop. Hard limits prevent infinite loops: at most **10** stall-recovery attempts per terminal, and the acceptance review sends a task back at most **5** times before escalating to a human.

### Custom rules & AI-generated rules

Every blocking condition of the three gates (metric, operator, threshold) can be added, removed, or edited in the UI — and you can have **AI generate a custom rule from natural language**. Generated rules pass through adversarial validation, empirical testing against sample snippets, and mandatory human confirmation before taking effect, and they execute as pure declarative data (never executable code).

> **Our advice: don't touch the default rules unless you have special needs.** The defaults already cover most scenarios; AI-generated rule patterns can be too broad or too narrow, and the result may well be worse than stock.

## Acceptance Review & Scoring Rubric

Quality gates catch *mechanical* errors (compile, lint, complexity, secrets). But "does this task actually deliver what I asked for?" needs deeper judgment. SoloDawn answers it with an **LLM-scored acceptance review** driven by a **project-tailored scoring rubric**.

### Why "generated per project" instead of one fixed rule set?

During development we tried shipping a single built-in scoring rule set. The seven test tasks turned it into a seesaw: push task A's score up and B's went down; fix B and A dropped again — a month was lost without ever balancing it. The final answer: **ship scoring *principles*, then generate a project-specific rubric from each user's requirement document** — ten projects, ten rubrics. That's what finally got every task over the bar.

### The rubric is generated from your spec

The moment your technical spec is confirmed, SoloDawn generates an **AuditPlan** — a scoring rubric tailored to *your specific project*. An LLM adapts the built-in 100-point, 5-dimension audit principles to your spec:

| Dimension | Max |
|---|---|
| Buildability | 20 |
| Functional completeness | 25 |
| Code quality (architecture / standards / security) | 30 |
| Test quality | 15 |
| Engineering & docs | 10 |

You can **upload your own audit document** to lead the rubric — three modes:

| Mode | What the rubric uses |
|---|---|
| **Builtin** (default) | built-in principles, tailored to your spec |
| **Merged** | your document **+** built-in; your document takes precedence |
| **Custom** | your document only |

> The rubric is generated the instant you confirm the spec. To influence it, upload your audit document **before** confirming — the panel becomes read-only afterward.

### Every task is scored against the rubric

When a task finishes, the acceptance-review LLM scores the delivered code against the rubric, dimension by dimension, citing file/line evidence.

- **score ≥ 90 (pass threshold) → APPROVED → the task proceeds**
- **score < 90 → REJECTED → fix instructions sent back → the terminal self-corrects and re-commits** (self-healing loop, up to 5 rounds)
- A **veto rule** (missing requirement, security risk, fake tests, etc.) forces the total to **0** regardless of other dimensions
- Foundation/scaffolding tasks in phased projects use a separate pass threshold (**70**), so pure setup work isn't unfairly killed by functional-completeness criteria

This is why the final delivery is trustworthy: every task had to prove — against criteria derived from *your* requirements — that it met the bar, not merely that it compiled.

### Requirement points & context capsules (评分点)

The rubric's functional-completeness criteria double as a **project-scoped requirement ledger** that outlives a single delivery:

- **Confirm time** — each criterion registers as a point with a stable server-assigned code (`RP-001`, …). The rubric card in the conversation and the ledger side panel both show them; points that are still pending can be edited or removed in the panel before work starts.
- **Scoring time (评分即结算 — settle at scoring)** — the acceptance review settles the ledger point by point: a delivered point stores a **compressed context capsule** — what was built, where it lives, key decisions & gotchas, how to extend. Capsules are word-capped pointers, never code copies; the repository stays the source of truth.
- **Follow-up rounds** — the planner and the orchestrator receive the point index + capsules as background, so round N+1 plans the delta instead of re-understanding the project. Previously delivered points enter the new rubric only as regression assertions (verify-not-broken); a red verdict marks the point regressed.

Every ledger operation is fail-open: if it ever breaks, confirm / materialize / review proceed without it.

## Claude Code: No-`-p` Interactive Transport & Billing Guarantee

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

## Quality Gate Configuration

Configure in `quality/quality-gate.yaml`:

```yaml
mode: enforce  # off | shadow | warn | enforce — 1.0 ships with enforce
```

| Mode | Behavior |
|------|----------|
| `off` | Disabled |
| `shadow` | Runs analysis, logs results, never blocks (observation) |
| `warn` | Runs analysis, feeds issues back, does not block |
| `enforce` | Hard gate — blocks on failure (1.0 factory default) |

The same file defines every blocking condition of the three gates (16 terminal / 18 branch / 23 repo — metric + operator + threshold) and the 11 analyzer toggles. In the Orchestrated Workspace, the "Quality Gate Rules" panel edits the same configuration visually (including AI rule generation) after a repository is bound; cloud CI runs the Repo Gate in shadow mode.

```bash
# Run quality gate manually
pnpm run quality

# Dry-run check
pnpm run quality:check
```

## Configuration Reference (Environment Variables)

| Variable | Purpose | Notes |
|---|---|---|
| `SOLODAWN_ENCRYPTION_KEY` | AES-256-GCM master key for sensitive data | Must be exactly 32 characters; if unset, falls back to a persistent per-machine key file `~/.enckey` (auto-generated and reused); multi-host/container deployments must set it explicitly |
| `SOLODAWN_API_TOKEN` | API authentication token | Required in release mode (waived with `SOLODAWN_LOCAL_MODE=1`) |
| `SOLODAWN_LOCAL_MODE` | Local mode | Skips the API-token check; localhost-only deployments |
| `SOLODAWN_FEISHU_ENABLED` | Feishu connector toggle | Takes precedence over the database setting |
| `SOLODAWN_NO_BROWSER` | Don't auto-open the browser on startup | — |
| `BACKEND_PORT` / `FRONTEND_PORT` | Port overrides | Defaults 23456 / 23457 |
| `SOLODAWN_ASSET_DIR` / `SOLODAWN_TEMP_DIR` / `SOLODAWN_ENC_KEY_FILE` | Data / temp / key-file path overrides | Platform defaults otherwise |

**Runtime data location** (SQLite database, `config.json`, credentials): dev mode uses `dev_assets/` inside the repo; release mode uses `%APPDATA%\solodawn\solodawn\` on Windows, `~/.local/share/solodawn/solodawn/` on Linux, `~/Library/Application Support/ai/solodawn/solodawn/` on macOS.

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Rust (Axum, SQLx, Tokio), Edition 2024 |
| Frontend | React 18, TypeScript, Tailwind CSS, Zustand, TanStack Query |
| Database | SQLite (AES-256-GCM encrypted key storage) |
| Terminal | xterm.js + native PTY (WebSocket bridge) |
| Real-time | WebSocket (workflow events + terminal streams) |
| Split deployment | tonic gRPC (Server ↔ Runner) |
| Type Safety | Rust → TypeScript auto-generation via `ts-rs` |
| Quality | Built-in rule engine + optional SonarQube |
| i18n | 6 languages (en, zh-Hans, zh-Hant, ja, es, ko) |

## Project Structure

```
SoloDawn/
├── crates/                    # Rust workspace (12 crates)
│   ├── server/                # Axum HTTP/WebSocket server + MCP Task Server
│   ├── services/              # Business logic (orchestrator, terminal, Git watcher, merge coordinator, acceptance review)
│   ├── quality/               # Three-layer quality gate engine + 31 built-in rules
│   ├── executors/             # 8 AI CLI integrations + MCP config adapters
│   ├── cc-switch/             # CLI model switching library
│   ├── feishu-connector/      # Feishu long-connection client (openlark SDK)
│   ├── db/                    # Database layer (models, migrations, DAO, AES-256-GCM encryption)
│   ├── runner/                # gRPC remote Runner for split deployment
│   ├── local-deployment/      # Local process/container management
│   ├── deployment/            # Deployment abstraction layer
│   ├── tray/                  # Windows system tray
│   └── utils/                 # Shared utilities (encryption, OAuth, Sentry, paths)
├── frontend/                  # React application (components / stores / hooks / i18n with 6 locales)
├── proto/                     # gRPC protocol definitions
├── quality/                   # Quality gate config (quality-gate.yaml) and baselines
├── scripts/                   # Dev, Docker, and deployment scripts
├── docker/                    # Docker compose and images
├── tests/                     # E2E tests
└── docs/                      # 150+ documents (phase plans, ops manuals, audit reports, the 48h test report)
```

## Current Limitations & Roadmap

### What 1.0 is good at — and not good at

SoloDawn 1.0 excels at two kinds of work: **zero-to-one greenfield projects**, and **legacy projects that need large-scale refactoring** — especially inherited spaghetti codebases; you could call it a legacy-code slayer.

1.0's biggest gap was **iterating on something it already delivered**: tasks were one-shot — once feature A shipped, extending it meant posting a brand-new task with none of the old context. This is exactly what **continuation rounds + the requirement ledger** (landed post-1.0) address: a delivered conversation now continues in place, and the new round plans the delta from the ledger's context capsules instead of restarting from zero (see [Requirement points & context capsules](#requirement-points--context-capsules-评分点)). The mechanism is fully implemented and covered by unit + live-server tests, but it hasn't yet been through a 48-hour-scale acceptance run — treat it as new.

### What 1.0 was actually tested with (full disclosure)

- The 48-hour acceptance run used the **Claude Code** terminal throughout. Multi-AI-CLI collaboration (supported in both modes) hasn't been re-tested since the February MVP demo — it should work, but no promises; multi-terminal adaptation and testing is the next work item.
- The **Feishu connector** code is complete and wired into the main server, and passed testing earlier; however, it hasn't been re-tested after the changes of the past month-plus, so it's not guaranteed to work until re-verified.
- The author only has two machines with very similar environments and can't enumerate every environment difference; if it won't start on yours, have your Claude Code / Codex fix it directly (see [Contributing](#contributing)).

### Roadmap

- Battle-test continuation rounds (landed post-1.0) at 48-hour-run scale; keep raising context-capsule quality
- Built-in reusable architecture design templates
- Built-in general-purpose skills and system prompts to push output quality further
- Generic fully-automated testing (the pre-release acceptance run *was* fully automated, but that harness isn't generic yet, so it isn't built in)
- Re-test the Feishu connector; adapt and regression-test multi-AI-CLI collaboration
- Kubernetes deployment support; container image size optimization

## Project History

- **2026-01-17** — Project started.
- **2026-02-12** — Minimal MVP released under the name **GitCortex**: [original LinuxDo post](https://linux.do/t/topic/1606779) (manual workflow mode only at the time).
- **2026-07-01** — **SoloDawn 1.0** released: Orchestrated Workspace (fully automated AI development), three-layer quality gates, and the acceptance review system all in place; 7 real-world tasks delivered at 6 A / 1 B.

## Contributing

- Issues and PRs welcome — **AI-generated code is entirely fine** — but a PR must pass cloud CI or it cannot be merged.
- Environment issues: everyone's machine is different and the author can't guarantee a clean run everywhere. If it won't start, hand the error to your Claude Code / Codex and let it fix things until it runs — then please contribute the fix back (PR or issue, either works).
- Open an issue first for large changes.
- Before submitting a PR, run the same checks cloud CI runs:

```bash
cargo clippy --workspace --exclude solodawn-tray --all-targets --all-features -- -D warnings
cargo nextest run --workspace --exclude solodawn-tray --cargo-profile ci --lib
cd frontend && pnpm test:run && pnpm run lint && pnpm run check && cd ..
```

## License

- SoloDawn: Apache-2.0
- Vibe Kanban derived parts: Apache-2.0
- CC-Switch derived parts: MIT
- Quality Gate models (ported from SonarQube): LGPL-3.0
- shadcn/ui components: MIT
- Design style presets & architecture methodology (adapted from open-source skills): MIT / Apache-2.0
- See `LICENSE` for full details and per-source attribution.

## Blogroll

- [LINUX DO](https://linux.do/)

---

*Formerly known as **GitCortex**.*
