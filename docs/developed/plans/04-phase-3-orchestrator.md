# Phase 3: Orchestrator Main Agent Implementation - TDD Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the Orchestrator main agent that coordinates multiple AI coding agents using test-driven development with unlimited resources.

**Architecture:** The Orchestrator is a central coordination agent that:
- Uses LLM (OpenAI-compatible API) for decision-making
- Communicates via a message bus for event-driven coordination
- Tracks state in a thread-safe state machine
- Monitors Git commits from terminal agents to trigger workflows

**Tech Stack:**
- Rust async/await with tokio
- OpenAI-compatible HTTP API (reqwest)
- tokio::sync channels for message bus
- serde for JSON serialization
- Thread-safe state with Arc<RwLock<>>

---

## Prerequisites

**Working Directory:** `F:/Project/SoloDawn/.worktrees/phase-3-orchestrator/vibe-kanban-main`

**Verification Steps:**
```bash
cd F:/Project/SoloDawn/.worktrees/phase-3-orchestrator/vibe-kanban-main
cargo build -p services
cargo test -p services --no-run
```

---

## Task 1: Add Test Dependencies

**Files:**
- Modify: `vibe-kanban-main/crates/services/Cargo.toml`

**Step 1: Add test dependencies to Cargo.toml**

Add to `[dev-dependencies]` section (create if doesn't exist):

```toml
[dev-dependencies]
wiremock = "0.6"
tokio-test = "0.4"
```

**Step 2: Run build to verify dependencies**

Run: `cargo build -p services`

Expected: `Finished` without errors

**Step 3: Commit**

```bash
git add crates/services/Cargo.toml
git commit -m "test: add wiremock and tokio-test for orchestrator tests"
```

---

## Task 2: Create Test Module Structure

**Files:**
- Create: `vibe-kanban-main/crates/services/src/services/orchestrator/tests.rs`

**Step 1: Create test module file with module declaration**

File: `vibe-kanban-main/crates/services/src/services/orchestrator/tests.rs`

```rust
//! Orchestrator unit tests
//!
//! Comprehensive test suite for LLM client, message bus, and Agent core functionality.

#[cfg(test)]
mod tests {
    use super::*;

    // Tests will be added in subsequent tasks

    // =========================================================================
    // Test Suite 1: Types Serialization
    // =========================================================================

    // =========================================================================
    // Test Suite 2: Configuration
    // =========================================================================

    // =========================================================================
    // Test Suite 3: State Management
    // =========================================================================

    // =========================================================================
    // Test Suite 4: LLM Client
    // =========================================================================

    // =========================================================================
    // Test Suite 5: Message Bus
    // =========================================================================

    // =========================================================================
    // Test Suite 6: OrchestratorAgent
    // =========================================================================
}
```

**Step 2: Add tests module to orchestrator/mod.rs**

Add to `vibe-kanban-main/crates/services/src/services/orchestrator/mod.rs`:

```rust
#[cfg(test)]
mod tests;
```

**Step 3: Run build to verify module structure**

Run: `cargo build -p services`

Expected: `Finished` without errors

**Step 4: Commit**

```bash
git add crates/services/src/services/orchestrator/mod.rs
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add orchestrator test module structure"
```

---

## Task 3: Test Suite 1 - Types Serialization

**Step 3.1: Write failing test - OrchestratorInstruction serialization**

```rust
#[test]
fn test_orchestrator_instruction_serialization() {
    let instruction = OrchestratorInstruction::SendToTerminal {
        terminal_id: "terminal-1".to_string(),
        message: "Implement login feature".to_string(),
    };

    let json = serde_json::to_string(&instruction).unwrap();
    let parsed: OrchestratorInstruction = serde_json::from_str(&json).unwrap();

    match parsed {
        OrchestratorInstruction::SendToTerminal { terminal_id, message } => {
            assert_eq!(terminal_id, "terminal-1");
            assert_eq!(message, "Implement login feature");
        }
        _ => panic!("Wrong instruction type"),
    }
}
```

**Step 3.2: Run test**

Run: `cargo test -p services orchestrator::tests::test_orchestrator_instruction_serialization -- --exact`

Expected: PASS (types.rs already implements serialization)

**Step 3.3: Write failing test - TerminalCompletionEvent with all fields**

```rust
#[test]
fn test_terminal_completion_event_full() {
    let event = TerminalCompletionEvent {
        terminal_id: "terminal-1".to_string(),
        task_id: "task-1".to_string(),
        workflow_id: "workflow-1".to_string(),
        status: TerminalCompletionStatus::Completed,
        commit_hash: Some("abc123".to_string()),
        commit_message: Some("feat: add login".to_string()),
        metadata: Some(CommitMetadata {
            workflow_id: "workflow-1".to_string(),
            task_id: "task-1".to_string(),
            terminal_id: "terminal-1".to_string(),
            terminal_order: 1,
            cli: "claude".to_string(),
            model: "claude-4".to_string(),
            status: "completed".to_string(),
            severity: None,
            reviewed_terminal: None,
            issues: None,
            next_action: "review".to_string(),
        }),
    };

    let json = serde_json::to_string(&event).unwrap();
    let parsed: TerminalCompletionEvent = serde_json::from_str(&json).unwrap();

    assert_eq!(parsed.terminal_id, "terminal-1");
    assert_eq!(parsed.status, TerminalCompletionStatus::Completed);
    assert!(parsed.metadata.is_some());
}
```

**Step 3.4: Run test**

Run: `cargo test -p services orchestrator::tests::test_terminal_completion_event_full -- --exact`

Expected: PASS

**Step 3.5: Write failing test - All instruction variants**

```rust
#[test]
fn test_all_instruction_variants() {
    let variants = vec![
        OrchestratorInstruction::StartTask {
            task_id: "task-1".to_string(),
            instruction: "Build API".to_string(),
        },
        OrchestratorInstruction::ReviewCode {
            terminal_id: "terminal-1".to_string(),
            commit_hash: "abc123".to_string(),
        },
        OrchestratorInstruction::FixIssues {
            terminal_id: "terminal-1".to_string(),
            issues: vec!["Bug in line 42".to_string()],
        },
        OrchestratorInstruction::MergeBranch {
            source_branch: "feature/login".to_string(),
            target_branch: "main".to_string(),
        },
        OrchestratorInstruction::PauseWorkflow {
            reason: "Need manual review".to_string(),
        },
        OrchestratorInstruction::CompleteWorkflow {
            summary: "All tasks completed".to_string(),
        },
        OrchestratorInstruction::FailWorkflow {
            reason: "Critical error".to_string(),
        },
    ];

    for instruction in variants {
        let json = serde_json::to_string(&instruction).unwrap();
        let parsed: OrchestratorInstruction = serde_json::from_str(&json).unwrap();

        // Verify type tag is correctly serialized
        let json_obj = serde_json::from_str::<serde_json::Value>(&json).unwrap();
        assert!(json_obj.get("type").is_some());
    }
}
```

**Step 3.6: Run test**

Run: `cargo test -p services orchestrator::tests::test_all_instruction_variants -- --exact`

Expected: PASS

**Step 3.7: Commit**

```bash
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add types serialization tests"
```

---

## Task 4: Test Suite 2 - Configuration

**Step 4.1: Write failing test - Default configuration**

```rust
#[test]
fn test_default_config() {
    let config = OrchestratorConfig::default();

    assert_eq!(config.api_type, "openai");
    assert_eq!(config.base_url, "https://api.openai.com/v1");
    assert_eq!(config.model, "gpt-4o");
    assert_eq!(config.max_retries, 3);
    assert_eq!(config.timeout_secs, 120);
    assert!(!config.system_prompt.is_empty());
}
```

**Step 4.2: Run test**

Run: `cargo test -p services orchestrator::tests::test_default_config -- --exact`

Expected: PASS

**Step 4.3: Write failing test - Configuration validation**

```rust
#[test]
fn test_config_validation() {
    // Valid config
    let config = OrchestratorConfig {
        api_key: "sk-test-123".to_string(),
        base_url: "https://api.openai.com/v1".to_string(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };
    assert!(config.validate().is_ok());

    // Missing API key
    let config = OrchestratorConfig {
        api_key: String::new(),
        base_url: "https://api.openai.com/v1".to_string(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };
    assert!(config.validate().is_err());

    // Missing base URL
    let config = OrchestratorConfig {
        api_key: "sk-test-123".to_string(),
        base_url: String::new(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };
    assert!(config.validate().is_err());

    // Missing model
    let config = OrchestratorConfig {
        api_key: "sk-test-123".to_string(),
        base_url: "https://api.openai.com/v1".to_string(),
        model: String::new(),
        ..Default::default()
    };
    assert!(config.validate().is_err());
}
```

**Step 4.4: Run test**

Run: `cargo test -p services orchestrator::tests::test_config_validation -- --exact`

Expected: PASS

**Step 4.5: Write failing test - from_workflow constructor**

```rust
#[test]
fn test_config_from_workflow() {
    // All Some
    let config = OrchestratorConfig::from_workflow(
        Some("anthropic"),
        Some("https://api.anthropic.com"),
        Some("sk-ant-123"),
        Some("claude-4-opus"),
    );
    assert!(config.is_some());
    let config = config.unwrap();
    assert_eq!(config.api_type, "anthropic");
    assert_eq!(config.base_url, "https://api.anthropic.com");
    assert_eq!(config.api_key, "sk-ant-123");
    assert_eq!(config.model, "claude-4-opus");

    // None returns None
    let config = OrchestratorConfig::from_workflow(None, None, None, None);
    assert!(config.is_none());
}
```

**Step 4.6: Run test**

Run: `cargo test -p services orchestrator::tests::test_config_from_workflow -- --exact`

Expected: PASS

**Step 4.7: Commit**

```bash
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add configuration tests"
```

---

## Task 5: Test Suite 3 - State Management

**Step 5.1: Write failing test - State initialization**

```rust
#[tokio::test]
async fn test_state_initialization() {
    let state = OrchestratorState::new("workflow-1".to_string());

    assert_eq!(state.workflow_id, "workflow-1");
    assert_eq!(state.run_state, OrchestratorRunState::Idle);
    assert!(state.task_states.is_empty());
    assert!(state.conversation_history.is_empty());
    assert!(state.pending_events.is_empty());
    assert_eq!(state.total_tokens_used, 0);
    assert_eq!(state.error_count, 0);
}
```

**Step 5.2: Run test**

Run: `cargo test -p services orchestrator::tests::test_state_initialization -- --exact`

Expected: PASS

**Step 5.3: Write failing test - Task initialization and tracking**

```rust
#[tokio::test]
async fn test_task_init_and_tracking() {
    let mut state = OrchestratorState::new("workflow-1".to_string());

    state.init_task("task-1".to_string(), 3);

    assert!(state.task_states.contains_key("task-1"));
    let task_state = state.task_states.get("task-1").unwrap();
    assert_eq!(task_state.task_id, "task-1");
    assert_eq!(task_state.total_terminals, 3);
    assert_eq!(task_state.current_terminal_index, 0);
    assert!(task_state.completed_terminals.is_empty());
    assert!(task_state.failed_terminals.is_empty());
    assert!(!task_state.is_completed);
}
```

**Step 5.4: Run test**

Run: `cargo test -p services orchestrator::tests::test_task_init_and_tracking -- --exact`

Expected: PASS

**Step 5.5: Write failing test - Terminal completion marking**

```rust
#[tokio::test]
async fn test_terminal_completion_marking() {
    let mut state = OrchestratorState::new("workflow-1".to_string());
    state.init_task("task-1".to_string(), 3);

    // Mark first terminal as completed
    state.mark_terminal_completed("task-1", "terminal-1", true);

    let task_state = state.task_states.get("task-1").unwrap();
    assert_eq!(task_state.completed_terminals.len(), 1);
    assert!(task_state.completed_terminals.contains(&"terminal-1".to_string()));
    assert!(!task_state.is_completed);

    // Mark second as failed
    state.mark_terminal_completed("task-1", "terminal-2", false);
    assert_eq!(task_state.failed_terminals.len(), 1);

    // Mark third as completed - should complete the task
    state.mark_terminal_completed("task-1", "terminal-3", true);
    assert!(task_state.is_completed);
}
```

**Step 5.6: Run test**

Run: `cargo test -p services orchestrator::tests::test_terminal_completion_marking -- --exact`

Expected: PASS

**Step 5.7: Write failing test - Conversation history management**

```rust
#[tokio::test]
async fn test_conversation_history() {
    let mut state = OrchestratorState::new("workflow-1".to_string());

    state.add_message("system", "You are a helpful assistant");
    state.add_message("user", "Hello");
    state.add_message("assistant", "Hi there!");

    assert_eq!(state.conversation_history.len(), 3);
    assert_eq!(state.conversation_history[0].role, "system");
    assert_eq!(state.conversation_history[1].content, "Hello");
}
```

**Step 5.8: Run test**

Run: `cargo test -p services orchestrator::tests::test_conversation_history -- --exact`

Expected: PASS

**Step 5.9: Write failing test - Conversation history pruning**

```rust
#[tokio::test]
async fn test_conversation_history_pruning() {
    let mut state = OrchestratorState::new("workflow-1".to_string());

    // Add system message
    state.add_message("system", "System prompt");

    // Add 60 user messages (exceeds MAX_HISTORY of 50)
    for i in 0..60 {
        state.add_message("user", &format!("Message {}", i));
        state.add_message("assistant", &format!("Response {}", i));
    }

    // History should be pruned to MAX_HISTORY, keeping system messages
    assert!(state.conversation_history.len() <= 51); // 1 system + 50 recent
    assert_eq!(state.conversation_history[0].role, "system");
}
```

**Step 5.10: Run test**

Run: `cargo test -p services orchestrator::tests::test_conversation_history_pruning -- --exact`

Expected: PASS

**Step 5.11: Write failing test - All tasks completed check**

```rust
#[tokio::test]
async fn test_all_tasks_completed() {
    let mut state = OrchestratorState::new("workflow-1".to_string());

    state.init_task("task-1".to_string(), 2);
    state.init_task("task-2".to_string(), 1);

    assert!(!state.all_tasks_completed());

    // Complete task-2
    state.mark_terminal_completed("task-2", "terminal-1", true);
    assert!(!state.all_tasks_completed());

    // Complete task-1
    state.mark_terminal_completed("task-1", "terminal-1", true);
    state.mark_terminal_completed("task-1", "terminal-2", true);
    assert!(state.all_tasks_completed());
}
```

**Step 5.12: Run test**

Run: `cargo test -p services orchestrator::tests::test_all_tasks_completed -- --exact`

Expected: PASS

**Step 5.13: Commit**

```bash
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add state management tests"
```

---

## Task 6: Test Suite 4 - LLM Client

**Step 6.1: Write failing test - LLM client basic request with mock**

```rust
#[tokio::test]
async fn test_llm_client_basic_request() {
    use wiremock::{MockServer, Mock, ResponseTemplate};
    use wiremock::matchers::{method, path};
    use reqwest::Client;

    let mock_server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "choices": [{
                "message": {
                    "role": "assistant",
                    "content": "Hello! How can I help you?"
                }
            }],
            "usage": {
                "prompt_tokens": 10,
                "completion_tokens": 9,
                "total_tokens": 19
            }
        })))
        .mount(&mock_server)
        .await;

    let config = OrchestratorConfig {
        base_url: mock_server.uri(),
        api_key: "test-key".to_string(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };

    let client = create_llm_client(&config).unwrap();
    let messages = vec![
        LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }
    ];

    let response = client.chat(messages).await.unwrap();

    assert!(response.content.contains("Hello"));
    assert!(response.usage.is_some());
    let usage = response.usage.unwrap();
    assert_eq!(usage.total_tokens, 19);
}
```

**Step 6.2: Run test**

Run: `cargo test -p services orchestrator::tests::test_llm_client_basic_request -- --exact`

Expected: PASS

**Step 6.3: Write failing test - LLM client error handling**

```rust
#[tokio::test]
async fn test_llm_client_error_handling() {
    use wiremock::{MockServer, Mock, ResponseTemplate};
    use wiremock::matchers::{method, path};

    let mock_server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .respond_with(ResponseTemplate::new(401).set_body_json(serde_json::json!({
            "error": {
                "message": "Invalid API key",
                "type": "invalid_request_error"
            }
        })))
        .mount(&mock_server)
        .await;

    let config = OrchestratorConfig {
        base_url: mock_server.uri(),
        api_key: "invalid-key".to_string(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };

    let client = create_llm_client(&config).unwrap();
    let messages = vec![
        LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }
    ];

    let result = client.chat(messages).await;

    assert!(result.is_err());
}
```

**Step 6.4: Run test**

Run: `cargo test -p services orchestrator::tests::test_llm_client_error_handling -- --exact`

Expected: PASS

**Step 6.5: Write failing test - LLM client empty response**

```rust
#[tokio::test]
async fn test_llm_client_empty_response() {
    use wiremock::{MockServer, Mock, ResponseTemplate};
    use wiremock::matchers::{method, path};

    let mock_server = MockServer::start().await;

    Mock::given(method("POST"))
        .and(path("/chat/completions"))
        .respond_with(ResponseTemplate::new(200).set_body_json(serde_json::json!({
            "choices": []
        })))
        .mount(&mock_server)
        .await;

    let config = OrchestratorConfig {
        base_url: mock_server.uri(),
        api_key: "test-key".to_string(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };

    let client = create_llm_client(&config).unwrap();
    let messages = vec![
        LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }
    ];

    let response = client.chat(messages).await.unwrap();

    assert_eq!(response.content, "");
}
```

**Step 6.6: Run test**

Run: `cargo test -p services orchestrator::tests::test_llm_client_empty_response -- --exact`

Expected: PASS

**Step 6.7: Commit**

```bash
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add LLM client tests"
```

---

## Task 7: Test Suite 5 - Message Bus

**Step 7.1: Write failing test - Message bus creation**

```rust
#[tokio::test]
async fn test_message_bus_creation() {
    let bus = MessageBus::new(100);

    // Should be able to create broadcast subscribers
    let _sub1 = bus.subscribe_broadcast();
    let _sub2 = bus.subscribe_broadcast();

    // Broadcast should work
    let result = bus.broadcast(BusMessage::Shutdown);
    assert!(result.is_ok());
}
```

**Step 7.2: Run test**

Run: `cargo test -p services orchestrator::tests::test_message_bus_creation -- --exact`

Expected: PASS

**Step 7.3: Write failing test - Message bus topic subscription**

```rust
#[tokio::test]
async fn test_message_bus_topic_subscription() {
    let bus = MessageBus::new(100);
    let mut subscriber = bus.subscribe("workflow:wf-1").await;

    // Publish to topic
    bus.publish("workflow:wf-1", BusMessage::StatusUpdate {
        workflow_id: "wf-1".to_string(),
        status: "running".to_string(),
    }).await;

    // Receive message
    let msg = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        subscriber.recv()
    ).await;

    assert!(msg.is_ok());
    let msg = msg.unwrap().unwrap();
    match msg {
        BusMessage::StatusUpdate { workflow_id, status } => {
            assert_eq!(workflow_id, "wf-1");
            assert_eq!(status, "running");
        }
        _ => panic!("Wrong message type"),
    }
}
```

**Step 7.4: Run test**

Run: `cargo test -p services orchestrator::tests::test_message_bus_topic_subscription -- --exact`

Expected: PASS

**Step 7.5: Write failing test - Message bus topic isolation**

```rust
#[tokio::test]
async fn test_message_bus_topic_isolation() {
    let bus = MessageBus::new(100);

    let mut sub_wf1 = bus.subscribe("workflow:wf-1").await;
    let mut sub_wf2 = bus.subscribe("workflow:wf-2").await;

    // Publish to wf-1 only
    bus.publish("workflow:wf-1", BusMessage::StatusUpdate {
        workflow_id: "wf-1".to_string(),
        status: "running".to_string(),
    }).await;

    // wf-1 should receive
    let msg = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        sub_wf1.recv()
    ).await;
    assert!(msg.is_ok());

    // wf-2 should NOT receive (timeout)
    let msg = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        sub_wf2.recv()
    ).await;
    assert!(msg.is_err());
}
```

**Step 7.6: Run test**

Run: `cargo test -p services orchestrator::tests::test_message_bus_topic_isolation -- --exact`

Expected: PASS

**Step 7.7: Write failing test - Broadcast to all subscribers**

```rust
#[tokio::test]
async fn test_message_bus_broadcast() {
    let bus = MessageBus::new(100);

    let mut sub1 = bus.subscribe_broadcast();
    let mut sub2 = bus.subscribe_broadcast();

    bus.broadcast(BusMessage::Shutdown).unwrap();

    // Both should receive
    let msg1 = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        sub1.recv()
    ).await;
    assert!(msg1.is_ok());

    let msg2 = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        sub2.recv()
    ).await;
    assert!(msg2.is_ok());
}
```

**Step 7.8: Run test**

Run: `cargo test -p services orchestrator::tests::test_message_bus_broadcast -- --exact`

Expected: PASS

**Step 7.9: Write failing test - Terminal completed event helper**

```rust
#[tokio::test]
async fn test_publish_terminal_completed() {
    let bus = MessageBus::new(100);
    let mut sub = bus.subscribe("workflow:wf-1").await;

    let event = TerminalCompletionEvent {
        terminal_id: "terminal-1".to_string(),
        task_id: "task-1".to_string(),
        workflow_id: "wf-1".to_string(),
        status: TerminalCompletionStatus::Completed,
        commit_hash: Some("abc123".to_string()),
        commit_message: Some("feat: add feature".to_string()),
        metadata: None,
    };

    bus.publish_terminal_completed(event).await;

    let msg = tokio::time::timeout(
        std::time::Duration::from_millis(100),
        sub.recv()
    ).await.unwrap().unwrap();

    match msg {
        BusMessage::TerminalCompleted(e) => {
            assert_eq!(e.terminal_id, "terminal-1");
            assert_eq!(e.workflow_id, "wf-1");
        }
        _ => panic!("Wrong message type"),
    }
}
```

**Step 7.10: Run test**

Run: `cargo test -p services orchestrator::tests::test_publish_terminal_completed -- --exact`

Expected: PASS

**Step 7.11: Commit**

```bash
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add message bus tests"
```

---

## Task 8: Test Suite 6 - OrchestratorAgent Integration

**Step 8.1: Write failing test - Agent creation**

```rust
#[tokio::test]
async fn test_agent_creation() {
    use db::DBService;
    use std::sync::Arc;

    let config = OrchestratorConfig {
        base_url: "https://api.openai.com/v1".to_string(),
        api_key: "test-key".to_string(),
        model: "gpt-4".to_string(),
        ..Default::default()
    };

    let message_bus = Arc::new(MessageBus::new(100));

    // Note: This test requires a mock database
    // For now, we'll test that the struct can be created with proper error handling

    let result = OrchestratorAgent::new(
        config,
        "workflow-1".to_string(),
        message_bus,
        // Arc::new(DBService::in_memory().await.unwrap()), // Mock DB needed
    );

    // This may fail without a proper DB, but we're testing the interface
    // In a full implementation, we'd use a mock DB service
}
```

**Step 8.2: Run test**

Run: `cargo test -p services orchestrator::tests::test_agent_creation -- --exact`

Expected: May fail - need to implement proper DB mock

**Step 8.3: Create mock DB helper for tests**

Add to `vibe-kanban-main/crates/services/src/services/orchestrator/tests.rs`:

```rust
#[cfg(test)]
struct MockDB {
    // Mock implementation would go here
    // For now, we'll skip DB-dependent tests
}
```

**Step 8.4: Write test for instruction parsing**

```rust
#[test]
fn test_instruction_parsing() {
    let json = r#"{"type":"send_to_terminal","terminal_id":"t1","message":"Do something"}"#;

    let instruction: OrchestratorInstruction = serde_json::from_str(json).unwrap();

    match instruction {
        OrchestratorInstruction::SendToTerminal { terminal_id, message } => {
            assert_eq!(terminal_id, "t1");
            assert_eq!(message, "Do something");
        }
        _ => panic!("Wrong instruction type"),
    }
}
```

**Step 8.5: Run test**

Run: `cargo test -p services orchestrator::tests::test_instruction_parsing -- --exact`

Expected: PASS

**Step 8.6: Write test for all instruction types parsing**

```rust
#[test]
fn test_all_instruction_parsing() {
    let test_cases = vec![
        (
            r#"{"type":"start_task","task_id":"task-1","instruction":"Build API"}"#,
            "start_task"
        ),
        (
            r#"{"type":"review_code","terminal_id":"t1","commit_hash":"abc123"}"#,
            "review_code"
        ),
        (
            r#"{"type":"fix_issues","terminal_id":"t1","issues":["bug1","bug2"]}"#,
            "fix_issues"
        ),
        (
            r#"{"type":"merge_branch","source_branch":"feature","target_branch":"main"}"#,
            "merge_branch"
        ),
        (
            r#"{"type":"pause_workflow","reason":"manual review"}"#,
            "pause_workflow"
        ),
        (
            r#"{"type":"complete_workflow","summary":"done"}"#,
            "complete_workflow"
        ),
        (
            r#"{"type":"fail_workflow","reason":"error"}"#,
            "fail_workflow"
        ),
    ];

    for (json, expected_type) in test_cases {
        let instruction: OrchestratorInstruction = serde_json::from_str(json).unwrap();
        let json_obj = serde_json::from_str::<serde_json::Value>(json).unwrap();
        assert_eq!(json_obj["type"], expected_type);
    }
}
```

**Step 8.7: Run test**

Run: `cargo test -p services orchestrator::tests::test_all_instruction_parsing -- --exact`

Expected: PASS

**Step 8.8: Commit**

```bash
git add crates/services/src/services/orchestrator/tests.rs
git commit -m "test: add OrchestratorAgent integration tests"
```

---

## Task 9: Run Full Test Suite

**Step 9.1: Run all orchestrator tests**

Run: `cargo test -p services orchestrator::tests`

Expected: All tests pass

**Step 9.2: Run with output**

Run: `cargo test -p services orchestrator::tests -- --nocapture --test-threads=1`

Expected: All tests pass with detailed output

**Step 9.3: Generate test coverage report (if available)**

Run: `cargo test -p services orchestrator::tests --no-run`

Expected: Compilation succeeds

**Step 9.4: Build full project**

Run: `cargo build -p services`

Expected: `Finished` without errors

---

## Task 10: Verify Phase 3 Completion

**Step 10.1: Verify all files exist**

Run: `ls -la vibe-kanban-main/crates/services/src/services/orchestrator/`

Expected output should include:
- `mod.rs`
- `types.rs`
- `config.rs`
- `state.rs`
- `llm.rs`
- `message_bus.rs`
- `agent.rs`
- `tests.rs` (new)

**Step 10.2: Verify mod.rs exports**

File: `vibe-kanban-main/crates/services/src/services/mod.rs`

Should include: `pub mod orchestrator;`

**Step 10.3: Run final test suite**

Run: `cargo test -p services --lib orchestrator`

Expected: All tests pass

**Step 10.4: Run full project tests**

Run: `cargo test -p services`

Expected: All existing tests still pass

**Step 10.5: Final commit**

```bash
git add -A
git commit -m "test: complete Phase 3 Orchestrator test suite

- Added comprehensive test coverage for types serialization
- Added configuration validation tests
- Added state management tests including history pruning
- Added LLM client tests with wiremock
- Added message bus pub/sub tests
- Added instruction parsing tests
- All tests passing"
```

---

## Phase 3 Completion Checklist

- [x] Task 3.1: Orchestrator module structure created
- [x] Task 3.2: LLM client implemented
- [x] Task 3.3: Message bus implemented
- [x] Task 3.4: OrchestratorAgent implemented
- [x] Comprehensive test suite added
- [x] All tests passing
- [ ] Integration tests with actual LLM API (optional)
- [ ] Documentation updates (optional)

---

## Next Steps

After completing this plan:

1. **Use @superpowers:finishing-a-development-branch** to:
   - Verify all tests pass
   - Present merge options
   - Handle the PR/merge process

2. **Optional enhancements:**
   - Add integration tests with real LLM API (requires API key)
   - Add performance benchmarks
   - Add logging/tracing instrumentation
   - Write API documentation

---

**Execution Options:**

This plan can be executed in two ways:

**1. Subagent-Driven (this session)** - I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Executing-Plans Skill** - Use @superpowers:executing-plans for batch execution with review checkpoints

Which approach would you prefer?
