# No-`-p` Interactive Transport — keep subscription users off the Agent SDK credit pool

**Status:** implementation complete — B/C/C+/D + follow-up-builder fix + all-3-mode auth + all-entry-point routing + S7 done; live smoke GREEN (branch `feat/no-p-interactive-transport`, off `main`)
**Date:** 2026-06-15
**Driver:** Anthropic's Agent SDK credit pool (eff. 2026-06-15) meters `subscription-OAuth × non-interactive(-p) surface`. SoloDawn's executor always pushes `-p` → subscription users get metered. Goal: keep **all** users functional with **zero pool draw and no extra billing**, especially official subscription users.

## Why this works (verified)

- Pool membership is **surface-keyed, not behavior-keyed** (Anthropic support art. 15036540 + headless docs). The four metered surfaces are: Agent SDK, `claude -p`, GitHub Actions, third-party-apps-via-SDK. **"Interactive Claude Code in the terminal/IDE" is explicitly on the subscription side.** No documented server-side human-vs-script detection.
- So: **drop `-p`, run the genuine `claude` binary interactively** → subscription metering restored, pool avoided. PTY-driven interactive presents the identical client surface + credential as a human.
- **ToS-clean basis:** we drive the *unmodified genuine binary* on the user's own machine; cc_switch only copies `.credentials.json` into an isolated home for that binary to consume — we do NOT extract the OAuth token into SoloDawn's own auth path. (The one existing violation — the orchestrator impersonation client — is removed; see Stage 1.)
- Interactive mode has no stdout stream-json (`--output-format stream-json` requires `-p`). **Structured capture = tail the on-disk session transcript JSONL**, whose nested message bodies are byte-identical to what `ClaudeLogProcessor` already parses.

### Verified facts (live, claude 2.1.177)
| Item | Value |
|---|---|
| Transcript path | `$CLAUDE_HOME/projects/<slug>/<uuid>.jsonl` |
| Slug rule | working_dir, drive-colon dropped, separators→`-` (`E:\SoloDawn`→`E--SoloDawn`) |
| Session flags | `--session-id <uuid>`, `--resume <uuid>`, `--fork-session` all exist in 2.1.177 |
| Completion | **No `type=result` ever**; use `type=system,subtype=turn_duration` (preceded by `stop_hook_summary`) + idle debounce |
| Parser reuse | Outer envelope camelCase (9 top-level types); **nested `message.content` blocks match `ClaudeContentItem` snake_case exactly** → only outer-envelope alias + existing `Unknown` catch-all needed |
| `toolUseResult` sideband | supplementary; status/output already derived from nested `ClaudeContentItem::ToolResult` (claude.rs:1004-1017) — ignore it |

## PROBE corrections (live, claude 2.1.177 — supersede earlier assumptions)

- **`CLAUDE_HOME` is a NO-OP** in 2.1.177; the real redirect for BOTH transcript location AND credential discovery is **`CLAUDE_CONFIG_DIR`**. Interactive home sets both (CONFIG_DIR = redirect; HOME = so RB-37 cleanup scan still finds the dir). ⚠ **Latent bug:** the existing `-p` path sets only `CLAUDE_HOME` → its RB-37 credential isolation is likely ineffective (reads real `~/.claude`). Separate from the pool goal; flagged, not fixed (don't destabilize the proven -p path).
- **No PTY needed** for one-turn: piped/`null` stdin → `claude --session-id <uuid> --dangerously-skip-permissions "<prompt>"` runs one turn, exits RC=0. (PTY only for S7 tier-2 keystroke approvals.)
- **Completion marker**: `turn_duration`/`type=result` are NEVER emitted in piped runs → drive `Finished` off **process exit** (idle is only a long safety net; 10s idle can fire mid-long-tool — fix in S6).
- **Slug rule**: every non-alphanumeric char → `-` (so `E:\SoloDawn`→`E--SoloDawn`), not "drive-colon dropped".
- `--resume <uuid>` WITHOUT `--fork-session` appends to the same `<uuid>.jsonl`.

## Scope decision (UPDATED 2026-06-15): ALL users interactive, no `-p`

~~Originally: only native-OAuth users interactive; api-key/relay keep `-p`.~~ **Superseded by product decision.** ALL ClaudeCode runs use the interactive transport (no `-p`) — native, relay, AND API-key alike. Rationale: subscription↔relay is a **frequent** switch (quota exhausted → relay; quota refreshed → back); a single unified transport avoids dual-path bugs at switch time. `-p` is retained ONLY as a dormant `SOLODAWN_NO_POOL` emergency fallback.

Per-mode billing stays correct: native→subscription plan quota (Pro→Pro/Max→Max); relay→relay endpoint; api-key→pay-as-you-go. Re-probe (claude 2.1.177, fake creds) confirms all 3 modes run interactively (no `-p`) with **NO blocking confirmation TUI** and always write the transcript.

**Consequence:** plan/approvals-mode synchronous pre-execution gate (was `-p` control-protocol only) is unavailable for ALL users; default **bypass** mode unchanged; S7 tier-2 is the only approval fallback.

**ACCEPTANCE CRITERION — transport consistency / switch-safety:** relay and subscription users MUST use the IDENTICAL interactive code path; the ONLY per-mode difference is the auth env (subscription copies creds; relay sets `ANTHROPIC_AUTH_TOKEN`+`ANTHROPIC_BASE_URL`). There must be NO active `-p` path for anyone (`-p` is reachable ONLY via the dormant `SOLODAWN_NO_POOL`). This guarantees that frequent relay↔subscription switching can NEVER route a subscription user back onto `-p` / the Agent SDK credit. Verification MUST cover EVERY ClaudeCode execution entry point — initial, follow-up, **`spawn_review`**, and any other spawn site — not just initial/follow-up; a single overlooked entry still on `-p` would leak a subscription user to Agent SDK billing. Final review greps all spawn entry points for active `-p`/stream-json and confirms zero.

**Re-probe corrections (load-bearing):**
- The legacy "Detected a custom API key" blocking prompt does NOT appear in interactive no-`-p` 2.1.177 (any auth mode).
- `primaryApiKey` in config.json is a **NO-OP for auth** now — the env key (`ANTHROPIC_API_KEY` / `ANTHROPIC_AUTH_TOKEN`) authenticates (cc_switch already sets it).
- **CRITICAL:** set a low `CLAUDE_CODE_MAX_RETRIES` (e.g. 2) in the interactive env, or relay/network errors retry with backoff for 30s+ (looks like a hang) before writing a terminator. Completion = child-exit; also treat "prefix-only transcript, no terminator" as failure.
- Auth setup per mode (reuse cc_switch): native = copy `.credentials.json` (drop `--bare`); official key = env `ANTHROPIC_API_KEY` + `--settings`; relay = env `ANTHROPIC_AUTH_TOKEN`+`ANTHROPIC_BASE_URL`, no `ANTHROPIC_API_KEY`.

## Shared contract (settle before editing)

- **Session UUID**: generated once per *logical session* (workflow_task / chat session) in `cc_switch`, threaded as `--session-id <uuid>` at first launch and `--resume <uuid>` (WITHOUT `--fork-session`) on follow-ups. Persisted to `coding_agent_turn.agent_session_id`.
- **Transcript path**: computed from `CLAUDE_HOME` + slug(working_dir) + `<uuid>.jsonl`, exposed by cc_switch to the launcher.
- **CLAUDE_HOME lifecycle**: stable per *logical session* (key on session UUID / workflow_task, **not** terminal.id), **exempt** from terminal-end delete; cleaned only at logical-session teardown (preserves RB-37 secret cleanup, just later).
- **Tailer → MsgStore**: tail JSONL, push each complete line as `LogMsg::Stdout` into the same per-execution MsgStore `track_child_msgs_in_store` uses; push `LogMsg::SessionId(uuid)` immediately (known a priori); push `LogMsg::Finished` on `turn_duration` + short idle debounce. `ClaudeLogProcessor` then normalizes unchanged.
- **Approvals**: tier-1 (default, native-OAuth) = `--dangerously-skip-permissions` + PromptWatcher residual net (NO `--bare` — it's stripped for native OAuth and breaks token loading). tier-2 (feature-flagged fallback) = PTY keystroke injection; **not** a true synchronous gate (the real `can_use_tool` gate is `-p`-only). Honest limitation.
- **Kill-switch**: keep the `-p` executor compiled behind `SOLODAWN_NO_POOL` (revert per-deploy without rebuild; accepts pool draw).

## Staged implementation (each stage must `cargo check`)

- [x] **S0 — parser shim** (claude.rs): `#[serde(alias="sessionId")]` on System/Assistant/User `session_id`; 6 kebab types → `Unknown`. *Done, `cargo check -p executors` green.*
- [x] **S1 — impersonation client removed** (orchestrator/llm.rs): `ClaudeCodeNativeClient` + `compute_cch` + hardcoded org_id + "You are Claude Code" identity POST all deleted (grep: **0 live hits**, 2 doc-comments only). Native planning rerouted via **Option 1** — new `InteractiveClaudeClient` single-turn `LLMClient` (genuine `claude`, no `-p`, reads final assistant text from the transcript JSONL); 4 call sites rewired (agent.rs:199/748, planning_drafts.rs:371/583) with key-first `.or_else()` fallback preserved (configured-key users unchanged). *Done, workspace green, 6 unit tests.*
- [x] **S2 — interactive executor mode** (claude.rs): `interactive`/`interactive_session_id` fields + `build_interactive_command_builder` (no `-p`/stream-json/permission flags; `--session-id` + `--dangerously-skip-permissions`); pub seams `build_interactive_command_parts` / `build_interactive_follow_up_command_parts` (`--resume` w/o `--fork-session`); spawn()/spawn_follow_up() guards error if interactive hits the -p path. *Done, green.*
- [x] **S3 — home stability + transcript path** (cc_switch.rs): `create_interactive_isolated_home` keyed on logical-session UUID; `slug_working_dir`; `interactive_transcript_path`; `InteractiveHome`. *Done, green, 3 unit tests.*
- [x] **S4 — home lifecycle** (terminal/process.rs): exempt `claude-isession-*` homes from terminal-end delete; `cleanup_logical_session_home` for deferred RB-37 teardown. *Done, green.*
- [x] **S5 — transcript tailer + interactive spawn** (local-deployment/container.rs): `spawn_interactive_transcript_tailer` (SessionId→Stdout→Finished) + `spawn_interactive_claude` (piped one-shot, no PTY/-p/ProtocolPeer). *Done, green.* ⚠ completion currently idle-fallback only → S6 must drive `Finished` off child-exit.
- [x] **S6 — transport selection** (local-deployment/container.rs `try_spawn_interactive_native_oauth` in start_execution_inner): native-OAuth ClaudeCode coding-agent runs (initial+follow-up) routed to interactive (creds copied into home, `CLAUDE_CONFIG_DIR`+`CLAUDE_HOME` set, `ANTHROPIC_API_KEY` unset); api-key/relay (model_config key/base_url) and `SOLODAWN_NO_POOL` → unchanged `-p`. Completion-on-exit watcher added (idle relaxed to ~120s net); redundant in-tailer normalization removed; teardown wired into cleanup_workspace. Session UUID persists via existing LogMsg::SessionId drain. *Done, workspace green.*
- [x] **S6+ — all-modes + all-entry-point routing** (local-deployment/container.rs `try_spawn_interactive_native_oauth`): generalized from native-only to ALL 3 auth modes (native/official-key/relay) via `setup_interactive_auth`; native still requires `~/.claude/.credentials.json` (else falls back to `-p`). **2026-06-15:** extended the eligible-action match to ALSO cover `ExecutorActionType::ReviewRequest` (was falling through to `-p`) — review now routes through the interactive transport too (resume when it carries a `session_id`, fresh otherwise), satisfying the "EVERY entry point — initial, follow-up, AND `spawn_review`" acceptance criterion. *Done, `cargo check -p local-deployment` green.*
- [x] **follow-up-builder fix** (claude.rs): `build_interactive_command_builder` no longer emits `--session-id`; the INITIAL path appends `--session-id <uuid>`, the FOLLOW-UP path appends ONLY `--resume <uuid>` (no `--session-id`, no `--fork-session`). Fixes claude 2.1.177 rejecting `--session-id` + `--resume` without `--fork-session` (follow-up/resume previously no-op'd). *Done; verified live (resume appends to the same transcript).*
- [x] **S7 — approvals tier-2 fallback** (terminal/prompt_watcher.rs): feature-flagged per-tool dialog detector + keystroke bridge gated by `SOLODAWN_INTERACTIVE_APPROVALS_TIER2`. OFF by default; tier-1 (`--dangerously-skip-permissions`) path untouched. *Done, 6 unit tests.*
- [ ] **S8 — contract test**: diff a live `.jsonl` against the parser per CLI version bump (schema-drift guard). *(`test_interactive_transcript_schema_contract` unit test in claude.rs is the seed; full live-diff guard still pending.)*

## Live verification results (2026-06-15, claude 2.1.177, native-OAuth creds)

Command: `RUST_MIN_STACK=268435456 cargo test -p local-deployment --test interactive_transport_smoke -- --ignored --nocapture` → **1 passed**.

| Check | Result |
|---|---|
| Initial argv | `--dangerously-skip-permissions --model claude-sonnet-4-6 --session-id <uuid>` — NO `-p`/`--print`/stream-json ✅ |
| Initial reply | assistant entry = `PONG` ✅ |
| Finished-on-exit | 9.95s (well under the 120s idle net) → completion-on-exit watcher fired, not idle ✅ |
| Transcript written | `<uuid>.jsonl`, 10197 bytes / 10 lines ✅ |
| Follow-up argv | `... --resume <uuid>` ONLY — NO `--session-id`, NO `--fork-session`, NO `-p` ✅ |
| Follow-up resume APPENDS | transcript grew 10197 → 18210 bytes on the SAME `<uuid>.jsonl` ✅ |
| Follow-up reply | resumed-session assistant entry = `PING` ✅ |

Non-live (CI-safe) coverage added in the same test file (`mod router_argv_env`, 5 tests, all pass):
mode resolution `(api_key, base_url)` → native/official-key/relay; per-mode argv has NO `-p`/`--output-format`; per-mode auth env (native scrubs all keys / official sets `ANTHROPIC_API_KEY` + `--settings` / relay sets `ANTHROPIC_AUTH_TOKEN`+`ANTHROPIC_BASE_URL` & unsets `ANTHROPIC_API_KEY`); follow-up argv `--resume`-only.

Suite status: `cargo check --workspace` green; `executors` `test_interactive_*` (4) pass; `services` `setup_interactive_auth` (4) pass; `local-deployment` `router_argv_env` (5) pass; live smoke (1) passes.

**Final entry-point grep (acceptance criterion):** active `-p`/stream-json in claude.rs exists ONLY in `build_command_builder`, gated behind `if self.interactive == Some(true) { return … interactive … }` (line ~121). With the router now forcing `interactive = Some(true)` for ClaudeCode initial+follow-up+review, the `-p` path is reachable for ClaudeCode ONLY via the dormant `SOLODAWN_NO_POOL` kill-switch. Non-ClaudeCode executors keep `-p` (the Agent SDK credit pool is Claude-specific). Zero subscription-user leak to the Agent SDK credit.

## Residual risks
- ToS reading of no-`-p` PTY automation is untested (MEDIUM); mitigate with one-time consent + `/feedback`, keep `SOLODAWN_NO_POOL` revert.
- Transcript JSONL is documented behavior, not a stable contract (MEDIUM) → S8 guard + `Unknown` catch-all (already debug-logged).
- Build is OOM-prone: full codegen needs `RUST_MIN_STACK=256MB` + `-j 1` (see memory `rust-build-stack-oom`); use `cargo check -p <crate>` during dev.
