# Checkpoint Metadata & Quality Gate Protocol

## Overview

When the quality gate is enabled, terminal commits use a **checkpoint** status instead of the traditional `completed` status. This document describes the metadata protocol and how the git watcher and orchestrator interact.

## Commit Metadata Format

### Standard (Quality Gate Disabled)

```
METADATA:
workflow_id: <uuid>
task_id: <uuid>
terminal_id: <uuid>
status: completed
next_action: continue
```

### Checkpoint (Quality Gate Enabled)

```
METADATA:
workflow_id: <uuid>
task_id: <uuid>
terminal_id: <uuid>
status: checkpoint
next_action: continue
```

The only difference is `status: checkpoint` instead of `status: completed`.

## Processing Flow

### GitWatcher (`git_watcher.rs`)

1. Detects new commits on watched branches
2. Parses the `METADATA:` block from commit messages
3. Recognizes `status: checkpoint` as a valid status variant
4. Publishes `BusMessage::GitEvent` with the parsed metadata
5. The `Checkpoint` status is preserved in the event — it is NOT converted to `Completed`

### Orchestrator (`agent.rs`)

1. Receives `BusMessage::GitEvent` with `status: Checkpoint`
2. Instead of calling `handle_terminal_completed()`, calls `handle_checkpoint_quality_gate()`
3. Runs `QualityEngine::run()` against the terminal's working directory
4. Publishes `BusMessage::TerminalQualityGateResult` with pass/fail result

### Quality Gate Result Handling

**On Pass:**
- Terminal status promoted from `working` to `completed`
- Normal flow resumes: next terminal dispatched, or task enters review/merge

**On Fail:**
- Terminal remains in `working` status
- Orchestrator injects structured fix instructions to terminal via PTY stdin
- Terminal fixes issues and commits again with `status: checkpoint`
- Cycle repeats

## Backward Compatibility

- Workflows created before the quality gate feature continue to use `status: completed`
- The orchestrator handles both `completed` and `checkpoint` statuses
- When `QUALITY_GATE_MODE=off`, `checkpoint` commits are treated as `completed` (promoted immediately)
- No-metadata commits (commits without the `METADATA:` block) are unaffected — they use the existing heuristic matching logic

## No-Metadata Commit Handling

When a commit lacks the `METADATA:` block, the git watcher falls back to:

1. Extract task hint from commit message via regex: `task[_\s-]*([0-9a-f-]{8,36})`
2. Filter tasks by branch name
3. Find working terminals (status = "working")
4. Disambiguate by checking terminal logs for commit hash
5. Fall back to deterministic selection (lowest task/terminal order)

These commits are always treated as `completed` (not checkpoint), regardless of quality gate mode.
