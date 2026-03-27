# Phase 25: Auto-Confirm Reliability Fix - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix "working but no output" issue by ensuring auto-confirm works without frontend WebSocket connection.

**Architecture:**
- P0: Fix frontend/backend auto_confirm defaults + historical data migration
- P1: Decouple PromptWatcher from WebSocket by adding PTY output fanout
- P2: Fix UTF-8 decoding black screen + add comprehensive E2E tests

**Tech Stack:** Rust (Tokio, SQLx), TypeScript (React, Zustand), SQLite

**Codex Review Findings:**
1. ✅ Historical data migration needed (DEFAULT 0 in DB)
2. ✅ Manual terminal start path missing PromptWatcher
3. ✅ Fanout race condition on first prompt
4. ✅ UTF-8 decoder needs sharing between WS and Watcher
5. ✅ Test baseline weak (terminal_ws placeholder, type mismatches)

---

## P0: Auto-Confirm Parameter Delivery (Immediate Fix)

### Task 25.1: Frontend - Add autoConfirm Field

**Files:**
- Modify: `frontend/src/components/workflow/types.ts:272`
- Modify: `frontend/src/hooks/useWorkflows.ts:58`
- Modify: `frontend/src/hooks/useWorkflows.test.tsx:55`

**Step 1: Add autoConfirm to Terminal type**

```typescript
// frontend/src/components/workflow/types.ts
export interface Terminal {
  name: string;
  cliTypeId: string;
  modelId: string;
  autoConfirm?: boolean; // Add this field
}
```

**Step 2: Set autoConfirm in wizard mapping**

```typescript
// frontend/src/components/workflow/types.ts:wizardConfigToCreateRequest
terminals: config.terminals.map((t, idx) => ({
  name: t.name,
  cliTypeId: t.cliTypeId,
  modelId: t.modelId,
  orderIndex: idx,
  autoConfirm: true, // Add this line
})),
```

**Step 3: Update test to verify field**

```typescript
// frontend/src/hooks/useWorkflows.test.tsx
expect(mockFetch).toHaveBeenCalledWith(
  expect.stringContaining('/api/workflows'),
  expect.objectContaining({
    body: expect.stringContaining('"autoConfirm":true'),
  })
);
```

**Step 4: Run frontend tests**

```bash
cd frontend && pnpm test useWorkflows
```

Expected: PASS

**Step 5: Commit**

```bash
git add frontend/src/components/workflow/types.ts frontend/src/hooks/useWorkflows.ts frontend/src/hooks/useWorkflows.test.tsx
git commit -m "feat(phase25): Add autoConfirm field to terminal creation payload"
```

---

### Task 25.2: Backend - Default auto_confirm to true

**Files:**
- Modify: `crates/db/src/models/terminal.rs:227`
- Modify: `crates/db/src/models/workflow.rs:407`
- Test: `crates/db/src/models/terminal.rs` (add test)

**Step 1: Write test for default value**

```rust
// crates/db/src/models/terminal.rs
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_auto_confirm_defaults_to_true() {
        let json = r#"{"name":"T1","cli_type_id":"1","model_id":"m1","order_index":0}"#;
        let terminal: Terminal = serde_json::from_str(json).unwrap();
        assert_eq!(terminal.auto_confirm, true);
    }

    #[test]
    fn test_auto_confirm_explicit_false() {
        let json = r#"{"name":"T1","cli_type_id":"1","model_id":"m1","order_index":0,"auto_confirm":false}"#;
        let terminal: Terminal = serde_json::from_str(json).unwrap();
        assert_eq!(terminal.auto_confirm, false);
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cd crates/db && cargo test test_auto_confirm_defaults_to_true
```

Expected: FAIL (defaults to false)

**Step 3: Add default function**

```rust
// crates/db/src/models/terminal.rs
fn default_auto_confirm() -> bool {
    true
}

#[derive(Debug, Clone, Serialize, Deserialize, FromRow)]
#[serde(rename_all = "camelCase")]
pub struct Terminal {
    // ... other fields
    #[serde(default = "default_auto_confirm")]
    pub auto_confirm: bool,
}
```

**Step 4: Run test to verify it passes**

```bash
cargo test test_auto_confirm
```

Expected: PASS

**Step 5: Commit**

```bash
git add crates/db/src/models/terminal.rs
git commit -m "fix(phase25): Default auto_confirm to true with explicit false support"
```

---

### Task 25.3: Historical Data Migration Script

**Files:**
- Create: `scripts/migrate_auto_confirm.sql`
- Create: `scripts/migrate_auto_confirm.sh`

**Step 1: Create dry-run SQL**

```sql
-- scripts/migrate_auto_confirm.sql
-- Dry run: Show terminals that would be updated
SELECT
    terminal_id,
    name,
    workflow_id,
    auto_confirm,
    created_at
FROM terminal
WHERE auto_confirm = 0
ORDER BY created_at DESC;

-- Apply: Update terminals to auto_confirm=1
-- UPDATE terminal SET auto_confirm = 1 WHERE auto_confirm = 0;
```

**Step 2: Create migration script**

```bash
#!/bin/bash
# scripts/migrate_auto_confirm.sh

DB_PATH="${1:-./solodawn.db}"

echo "=== Dry Run: Terminals with auto_confirm=0 ==="
sqlite3 "$DB_PATH" < scripts/migrate_auto_confirm.sql

read -p "Apply migration? (yes/no): " confirm
if [ "$confirm" = "yes" ]; then
    sqlite3 "$DB_PATH" "UPDATE terminal SET auto_confirm = 1 WHERE auto_confirm = 0;"
    echo "✅ Migration applied"
else
    echo "❌ Migration cancelled"
fi
```

**Step 3: Test dry-run**

```bash
chmod +x scripts/migrate_auto_confirm.sh
./scripts/migrate_auto_confirm.sh ./solodawn.db
```

Expected: Shows affected terminals, prompts for confirmation

**Step 4: Commit**

```bash
git add scripts/migrate_auto_confirm.sql scripts/migrate_auto_confirm.sh
git commit -m "feat(phase25): Add historical data migration for auto_confirm"
```

---

## P1: PromptWatcher Backend Decoupling (Root Cause Fix)

### Task 25.4: PTY Output Fanout Architecture

**Files:**
- Modify: `crates/services/src/services/terminal/process.rs:150`
- Create: `crates/services/src/services/terminal/output_fanout.rs`
- Test: `crates/services/src/services/terminal/output_fanout.rs`

**Step 1: Write fanout test**

```rust
// crates/services/src/services/terminal/output_fanout.rs
#[cfg(test)]
mod tests {
    use super::*;
    use tokio::sync::mpsc;

    #[tokio::test]
    async fn test_multiple_subscribers_receive_output() {
        let (tx, rx) = mpsc::unbounded_channel();
        let fanout = OutputFanout::new(rx);

        let mut sub1 = fanout.subscribe();
        let mut sub2 = fanout.subscribe();

        tx.send(b"test".to_vec()).unwrap();

        assert_eq!(sub1.recv().await.unwrap(), b"test");
        assert_eq!(sub2.recv().await.unwrap(), b"test");
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cargo test test_multiple_subscribers_receive_output
```

Expected: FAIL (module not found)

**Step 3: Implement OutputFanout**

```rust
// crates/services/src/services/terminal/output_fanout.rs
use tokio::sync::broadcast;
use tokio::sync::mpsc;

pub struct OutputFanout {
    tx: broadcast::Sender<Vec<u8>>,
}

impl OutputFanout {
    pub fn new(mut pty_rx: mpsc::UnboundedReceiver<Vec<u8>>) -> Self {
        let (tx, _) = broadcast::channel(1024);
        let tx_clone = tx.clone();

        tokio::spawn(async move {
            while let Some(data) = pty_rx.recv().await {
                let _ = tx_clone.send(data);
            }
        });

        Self { tx }
    }

    pub fn subscribe(&self) -> broadcast::Receiver<Vec<u8>> {
        self.tx.subscribe()
    }
}
```

**Step 4: Run test to verify it passes**

```bash
cargo test test_multiple_subscribers_receive_output
```

Expected: PASS

**Step 5: Integrate into ProcessManager**

```rust
// crates/services/src/services/terminal/process.rs
pub struct TrackedProcess {
    // ... existing fields
    pub output_fanout: Arc<OutputFanout>,
}

// In spawn_pty_with_config:
let (output_tx, output_rx) = mpsc::unbounded_channel();
let fanout = Arc::new(OutputFanout::new(output_rx));

// Background reader task:
tokio::spawn(async move {
    let mut buf = [0u8; 8192];
    while let Ok(n) = pty_reader.read(&mut buf).await {
        if n == 0 { break; }
        let _ = output_tx.send(buf[..n].to_vec());
    }
});
```

**Step 6: Commit**

```bash
git add crates/services/src/services/terminal/output_fanout.rs crates/services/src/services/terminal/process.rs
git commit -m "feat(phase25): Add PTY output fanout for multiple subscribers"
```

---

### Task 25.5: PromptWatcher Background Task

**Files:**
- Modify: `crates/services/src/services/terminal/prompt_watcher.rs:177`
- Modify: `crates/services/src/services/terminal/launcher.rs:317`
- Test: `crates/services/src/services/terminal/prompt_watcher.rs`

**Step 1: Write background watcher test**

```rust
#[tokio::test]
async fn test_watcher_works_without_websocket() {
    let (bus, _) = MessageBus::new();
    let process = create_mock_process_with_output(b"Continue? (y/n): ");

    let watcher = PromptWatcher::spawn_background(
        "session-1".to_string(),
        "terminal-1".to_string(),
        process.output_fanout.clone(),
        bus.clone(),
        true, // auto_confirm
    );

    tokio::time::sleep(Duration::from_millis(100)).await;

    // Verify prompt_detected event was published
    let events = bus.get_events_for_topic("terminal-1");
    assert!(events.iter().any(|e| matches!(e, BusMessage::TerminalPromptDetected(_))));
}
```

**Step 2: Run test to verify it fails**

```bash
cargo test test_watcher_works_without_websocket
```

Expected: FAIL

**Step 3: Implement spawn_background**

```rust
// crates/services/src/services/terminal/prompt_watcher.rs
impl PromptWatcher {
    pub fn spawn_background(
        session_id: String,
        terminal_id: String,
        output_fanout: Arc<OutputFanout>,
        bus: Arc<MessageBus>,
        auto_confirm: bool,
    ) -> tokio::task::JoinHandle<()> {
        tokio::spawn(async move {
            let mut watcher = Self::new(session_id, terminal_id, bus, auto_confirm);
            let mut subscriber = output_fanout.subscribe();

            while let Ok(data) = subscriber.recv().await {
                if let Ok(text) = String::from_utf8(data) {
                    watcher.process_output(&text).await;
                }
            }
        })
    }
}
```

**Step 4: Run test to verify it passes**

```bash
cargo test test_watcher_works_without_websocket
```

Expected: PASS

**Step 5: Register in launcher**

```rust
// crates/services/src/services/terminal/launcher.rs
pub async fn launch_terminal(&self, terminal: &Terminal) -> Result<String> {
    // ... existing launch code

    // Register PromptWatcher background task
    if terminal.auto_confirm {
        PromptWatcher::spawn_background(
            session_id.clone(),
            terminal.terminal_id.clone(),
            process.output_fanout.clone(),
            self.bus.clone(),
            true,
        );
    }

    Ok(session_id)
}
```

**Step 6: Commit**

```bash
git add crates/services/src/services/terminal/prompt_watcher.rs crates/services/src/services/terminal/launcher.rs
git commit -m "feat(phase25): PromptWatcher as independent background task"
```

---

### Task 25.6: Unify Manual Terminal Start Path

**Files:**
- Modify: `crates/server/src/routes/terminals.rs:254`
- Test: `crates/server/tests/terminal_start_test.rs`

**Step 1: Write test for manual start**

```rust
// crates/server/tests/terminal_start_test.rs
#[tokio::test]
async fn test_manual_start_registers_prompt_watcher() {
    let app = create_test_app().await;

    // Create terminal with auto_confirm=true
    let terminal_id = create_test_terminal(&app, true).await;

    // Start terminal manually
    let response = app
        .oneshot(
            Request::builder()
                .uri(format!("/api/terminals/{}/start", terminal_id))
                .method("POST")
                .body(Body::empty())
                .unwrap(),
        )
        .await
        .unwrap();

    assert_eq!(response.status(), StatusCode::OK);

    // Verify PromptWatcher is active (check for prompt detection)
    // ... verification logic
}
```

**Step 2: Run test to verify it fails**

```bash
cargo test test_manual_start_registers_prompt_watcher
```

Expected: FAIL

**Step 3: Add watcher registration**

```rust
// crates/server/src/routes/terminals.rs
pub async fn start_terminal(
    State(state): State<Arc<AppState>>,
    Path(terminal_id): Path<String>,
) -> Result<Json<TerminalStartResponse>, AppError> {
    // ... existing start logic

    // Register PromptWatcher if auto_confirm enabled
    if terminal.auto_confirm {
        let process = state.process_manager.get_process(&session_id)?;
        PromptWatcher::spawn_background(
            session_id.clone(),
            terminal_id.clone(),
            process.output_fanout.clone(),
            state.bus.clone(),
            true,
        );
    }

    Ok(Json(response))
}
```

**Step 4: Run test to verify it passes**

```bash
cargo test test_manual_start_registers_prompt_watcher
```

Expected: PASS

**Step 5: Commit**

```bash
git add crates/server/src/routes/terminals.rs crates/server/tests/terminal_start_test.rs
git commit -m "fix(phase25): Register PromptWatcher in manual terminal start path"
```

---

## P2: Stability & Testing (Black Screen Fix + E2E)

### Task 25.8: Shared UTF-8 Stream Decoder

**Files:**
- Create: `crates/services/src/services/terminal/utf8_decoder.rs`
- Modify: `crates/server/src/routes/terminal_ws.rs:497`
- Test: `crates/services/src/services/terminal/utf8_decoder.rs`

**Step 1: Write decoder tests**

```rust
// crates/services/src/services/terminal/utf8_decoder.rs
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_incomplete_utf8_sequence() {
        let mut decoder = Utf8StreamDecoder::new();

        // First part of 2-byte UTF-8 (incomplete)
        let result = decoder.decode(&[0xC3]);
        assert_eq!(result, "");

        // Second part completes the sequence (ã)
        let result = decoder.decode(&[0xA3]);
        assert_eq!(result, "ã");
    }

    #[test]
    fn test_invalid_middle_bytes_recovered() {
        let mut decoder = Utf8StreamDecoder::new();

        // Valid + Invalid + Valid
        let result = decoder.decode(b"Hello\xFF\xFEWorld");
        assert!(result.contains("Hello"));
        assert!(result.contains("World"));
    }
}
```

**Step 2: Run test to verify it fails**

```bash
cargo test test_incomplete_utf8_sequence
```

Expected: FAIL

**Step 3: Implement Utf8StreamDecoder**

```rust
// crates/services/src/services/terminal/utf8_decoder.rs
pub struct Utf8StreamDecoder {
    pending: Vec<u8>,
    dropped_bytes: usize,
}

impl Utf8StreamDecoder {
    pub fn new() -> Self {
        Self {
            pending: Vec::new(),
            dropped_bytes: 0,
        }
    }

    pub fn decode(&mut self, data: &[u8]) -> String {
        self.pending.extend_from_slice(data);

        match String::from_utf8(self.pending.clone()) {
            Ok(s) => {
                self.pending.clear();
                s
            }
            Err(e) => {
                let valid_up_to = e.utf8_error().valid_up_to();
                let valid = String::from_utf8_lossy(&self.pending[..valid_up_to]).to_string();

                // Check if error is incomplete sequence at end
                if let Some(len) = e.utf8_error().error_len() {
                    // Invalid byte in middle - drop it and continue
                    self.dropped_bytes += len;
                    self.pending.drain(..valid_up_to + len);
                } else {
                    // Incomplete sequence at end - keep for next chunk
                    self.pending.drain(..valid_up_to);
                }

                valid
            }
        }
    }

    pub fn dropped_bytes(&self) -> usize {
        self.dropped_bytes
    }
}
```

**Step 4: Run test to verify it passes**

```bash
cargo test utf8_decoder
```

Expected: PASS

**Step 5: Replace terminal_ws decoder**

```rust
// crates/server/src/routes/terminal_ws.rs
let mut decoder = Utf8StreamDecoder::new();

while let Some(data) = pty_rx.recv().await {
    let text = decoder.decode(&data);
    if !text.is_empty() {
        ws_tx.send(Message::Text(text)).await?;
    }
}

if decoder.dropped_bytes() > 0 {
    tracing::warn!("Dropped {} invalid UTF-8 bytes", decoder.dropped_bytes());
}
```

**Step 6: Commit**

```bash
git add crates/services/src/services/terminal/utf8_decoder.rs crates/server/src/routes/terminal_ws.rs
git commit -m "fix(phase25): Add shared UTF-8 stream decoder with recovery"
```

---

### Task 25.9: E2E Regression Test Matrix

**Files:**
- Create: `crates/server/tests/phase25_e2e_test.rs`

**Step 1: Write E2E test matrix**

```rust
// crates/server/tests/phase25_e2e_test.rs
#[tokio::test]
async fn test_workflow_without_websocket_connection() {
    let test_dir = setup_test_workspace().await;
    let app = create_test_app().await;

    // Create workflow with auto_confirm=true
    let workflow_id = create_test_workflow(&app, &test_dir, true).await;

    // Start workflow WITHOUT opening WebSocket
    start_workflow(&app, &workflow_id).await;

    // Wait for prompt detection and auto-response
    tokio::time::sleep(Duration::from_secs(3)).await;

    // Verify files were created (prompt was auto-confirmed)
    assert!(test_dir.join("output.txt").exists());
}

#[tokio::test]
async fn test_websocket_reconnect_continues_watching() {
    // Test WS disconnect/reconnect doesn't break prompt detection
}

#[tokio::test]
async fn test_invalid_utf8_bytes_dont_block_output() {
    // Test that invalid UTF-8 in middle of stream is handled
}
```

**Step 2: Run tests**

```bash
cargo test phase25_e2e
```

Expected: PASS

**Step 3: Commit**

```bash
git add crates/server/tests/phase25_e2e_test.rs
git commit -m "test(phase25): Add E2E regression matrix for auto-confirm reliability"
```

---

## Verification Checklist

- [ ] New workflow terminals have `auto_confirm=1` in database
- [ ] CLI launch includes `--dangerously-skip-permissions` / `--yolo` when auto_confirm=true
- [ ] PromptWatcher detects prompts without WebSocket connection
- [ ] Manual terminal start path registers PromptWatcher
- [ ] UTF-8 decoding handles invalid bytes without blocking
- [ ] E2E test passes: workflow completes without frontend connection
- [ ] Historical terminals migrated to auto_confirm=1

---

## Rollback Plan

If issues arise:
1. Revert frontend changes: `git revert <commit-hash>`
2. Revert backend default: Set `auto_confirm` back to `false` default
3. Rollback DB migration: `UPDATE terminal SET auto_confirm = 0 WHERE ...`
4. Disable background watcher: Add feature flag check

---

## Post-Implementation

1. Monitor logs for `PromptWatcher` activity without WS
2. Check for UTF-8 decoder warnings in production
3. Verify no regression in existing workflows
4. Update documentation with new auto-confirm behavior
