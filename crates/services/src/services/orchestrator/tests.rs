//! Orchestrator unit tests
//!
//! Comprehensive test suite for LLM client, message bus, and Agent core functionality.

#[cfg(test)]
mod orchestrator_tests {
    use std::sync::Arc;

    use wiremock::{
        Mock, MockServer, ResponseTemplate,
        matchers::{method, path},
    };

    use crate::services::orchestrator::{
        BusMessage, CommitMetadata, LLMMessage, MessageBus, MockLLMClient, OrchestratorAgent,
        OrchestratorConfig, OrchestratorInstruction, OrchestratorRunState, OrchestratorState,
        RuntimeActionService, TerminalCompletionEvent, TerminalCompletionStatus,
        constants::DEFAULT_LLM_RATE_LIMIT_PER_SECOND, create_llm_client,
    };

    // Tests will be added in subsequent tasks

    // =========================================================================
    // Test Suite 1: Types Serialization
    // =========================================================================

    #[test]
    fn test_orchestrator_instruction_serialization() {
        let instruction = OrchestratorInstruction::SendToTerminal {
            terminal_id: "terminal-1".to_string(),
            message: "Implement login feature".to_string(),
        };

        let json = serde_json::to_string(&instruction).unwrap();
        let parsed: OrchestratorInstruction = serde_json::from_str(&json).unwrap();

        match parsed {
            OrchestratorInstruction::SendToTerminal {
                terminal_id,
                message,
            } => {
                assert_eq!(terminal_id, "terminal-1");
                assert_eq!(message, "Implement login feature");
            }
            _ => panic!("Wrong instruction type"),
        }
    }

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

    #[test]
    fn test_all_instruction_variants() {
        let variants = vec![
            OrchestratorInstruction::StartTask {
                task_id: "task-1".to_string(),
                instruction: "Build API".to_string(),
            },
            OrchestratorInstruction::CreateTask {
                task_id: Some("task-runtime".to_string()),
                name: "Runtime task".to_string(),
                description: Some("Create work at runtime".to_string()),
                branch: None,
                order_index: Some(0),
            },
            OrchestratorInstruction::CreateTerminal {
                terminal_id: Some("term-runtime".to_string()),
                task_id: "task-runtime".to_string(),
                cli_type_id: "cli-claude-code".to_string(),
                model_config_id: "model-claude-sonnet".to_string(),
                custom_base_url: None,
                custom_api_key: None,
                role: Some("coder".to_string()),
                role_description: None,
                order_index: Some(0),
                auto_confirm: Some(true),
            },
            OrchestratorInstruction::StartTerminal {
                terminal_id: "term-runtime".to_string(),
                instruction: "Implement runtime feature".to_string(),
            },
            OrchestratorInstruction::CloseTerminal {
                terminal_id: "term-runtime".to_string(),
                final_status: Some("completed".to_string()),
            },
            OrchestratorInstruction::CompleteTask {
                task_id: "task-runtime".to_string(),
                summary: "Task is fully planned".to_string(),
            },
            OrchestratorInstruction::SetWorkflowPlanningComplete {
                summary: Some("No more tasks required".to_string()),
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
            let _parsed: OrchestratorInstruction = serde_json::from_str(&json).unwrap();

            // Verify type tag is correctly serialized
            let json_obj = serde_json::from_str::<serde_json::Value>(&json).unwrap();
            assert!(json_obj.get("type").is_some());
        }
    }

    // =========================================================================
    // Test Suite 2: Configuration
    // =========================================================================

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

    // =========================================================================
    // Test Suite 3: State Management
    // =========================================================================

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

    #[tokio::test]
    async fn test_task_init_and_tracking() {
        let mut state = OrchestratorState::new("workflow-1".to_string());

        state.init_task(
            "task-1".to_string(),
            vec![
                "terminal-1".to_string(),
                "terminal-2".to_string(),
                "terminal-3".to_string(),
            ],
        );

        assert!(state.task_states.contains_key("task-1"));
        let task_state = state.task_states.get("task-1").unwrap();
        assert_eq!(task_state.task_id, "task-1");
        assert_eq!(task_state.total_terminals, 3);
        assert_eq!(task_state.current_terminal_index, 0);
        assert!(task_state.completed_terminals.is_empty());
        assert!(task_state.failed_terminals.is_empty());
        assert!(!task_state.is_completed);
    }

    #[tokio::test]
    async fn test_terminal_completion_marking() {
        let mut state = OrchestratorState::new("workflow-1".to_string());
        state.init_task(
            "task-1".to_string(),
            vec![
                "terminal-1".to_string(),
                "terminal-2".to_string(),
                "terminal-3".to_string(),
            ],
        );

        // Mark first terminal as completed
        state.mark_terminal_completed("task-1", "terminal-1", true);

        {
            let task_state = state.task_states.get("task-1").unwrap();
            assert_eq!(task_state.completed_terminals.len(), 1);
            assert!(
                task_state
                    .completed_terminals
                    .contains(&"terminal-1".to_string())
            );
            assert!(!task_state.is_completed);
        }

        // Mark second as failed
        state.mark_terminal_completed("task-1", "terminal-2", false);
        {
            let task_state = state.task_states.get("task-1").unwrap();
            assert_eq!(task_state.failed_terminals.len(), 1);
        }

        // Mark third as completed - should complete the task
        state.mark_terminal_completed("task-1", "terminal-3", true);
        {
            let task_state = state.task_states.get("task-1").unwrap();
            assert!(task_state.is_completed);
        }
    }

    #[tokio::test]
    async fn test_conversation_history() {
        let mut state = OrchestratorState::new("workflow-1".to_string());
        let config = OrchestratorConfig::default();

        state.add_message("system", "You are a helpful assistant", &config);
        state.add_message("user", "Hello", &config);
        state.add_message("assistant", "Hi there!", &config);

        assert_eq!(state.conversation_history.len(), 3);
        assert_eq!(state.conversation_history[0].role, "system");
        assert_eq!(state.conversation_history[1].content, "Hello");
    }

    #[tokio::test]
    async fn test_conversation_history_pruning() {
        let mut state = OrchestratorState::new("workflow-1".to_string());
        let config = OrchestratorConfig::default();

        // Add system message
        state.add_message("system", "System prompt", &config);

        // Add 60 user messages (exceeds max_conversation_history of 50)
        for i in 0..60 {
            state.add_message("user", &format!("Message {i}"), &config);
            state.add_message("assistant", &format!("Response {i}"), &config);
        }

        // History should be pruned to max_conversation_history, keeping system messages
        assert!(state.conversation_history.len() <= 51); // 1 system + 50 recent
        assert_eq!(state.conversation_history[0].role, "system");
    }

    #[tokio::test]
    async fn test_all_tasks_completed() {
        let mut state = OrchestratorState::new("workflow-1".to_string());

        state.init_task(
            "task-1".to_string(),
            vec!["terminal-1".to_string(), "terminal-2".to_string()],
        );
        state.init_task("task-2".to_string(), vec!["terminal-1".to_string()]);

        assert!(!state.all_tasks_completed());

        // Complete task-2
        state.mark_terminal_completed("task-2", "terminal-1", true);
        assert!(!state.all_tasks_completed());

        // Complete task-1
        state.mark_terminal_completed("task-1", "terminal-1", true);
        state.mark_terminal_completed("task-1", "terminal-2", true);
        assert!(state.all_tasks_completed());
    }

    // =========================================================================
    // Test Suite 4: LLM Client
    // =========================================================================

    #[tokio::test]
    async fn test_llm_client_basic_request() {
        // Install crypto provider for reqwest (ignore if already installed)
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

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
        let messages = vec![LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }];

        let response = client.chat(messages).await.unwrap();

        assert!(response.content.contains("Hello"));
        assert!(response.usage.is_some());
        let usage = response.usage.unwrap();
        assert_eq!(usage.total_tokens, 19);
    }

    #[tokio::test]
    async fn test_llm_client_error_handling() {
        // Install crypto provider for reqwest (ignore if already installed)
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

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
        let messages = vec![LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }];

        let result = client.chat(messages).await;

        assert!(result.is_err());
    }

    #[tokio::test]
    async fn test_llm_client_empty_response() {
        // Install crypto provider for reqwest (ignore if already installed)
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

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
        let messages = vec![LLMMessage {
            role: "user".to_string(),
            content: "Hello".to_string(),
        }];

        let response = client.chat(messages).await.unwrap();

        assert_eq!(response.content, "");
    }

    // =========================================================================
    // Test Suite 5: Message Bus
    // =========================================================================

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

    #[tokio::test]
    async fn test_message_bus_topic_subscription() {
        let bus = MessageBus::new(100);
        let mut subscriber = bus.subscribe("workflow:wf-1").await;

        // Publish to topic
        bus.publish(
            "workflow:wf-1",
            BusMessage::StatusUpdate {
                workflow_id: "wf-1".to_string(),
                status: "running".to_string(),
            },
        )
        .await
        .unwrap();

        // Receive message
        let msg =
            tokio::time::timeout(std::time::Duration::from_millis(100), subscriber.recv()).await;

        assert!(msg.is_ok());
        let msg = msg.unwrap().unwrap();
        match msg {
            BusMessage::StatusUpdate {
                workflow_id,
                status,
            } => {
                assert_eq!(workflow_id, "wf-1");
                assert_eq!(status, "running");
            }
            _ => panic!("Wrong message type"),
        }
    }

    #[tokio::test]
    async fn test_message_bus_topic_isolation() {
        let bus = MessageBus::new(100);

        let mut sub_wf1 = bus.subscribe("workflow:wf-1").await;
        let mut sub_wf2 = bus.subscribe("workflow:wf-2").await;

        // Publish to wf-1 only
        bus.publish(
            "workflow:wf-1",
            BusMessage::StatusUpdate {
                workflow_id: "wf-1".to_string(),
                status: "running".to_string(),
            },
        )
        .await
        .unwrap();

        // wf-1 should receive
        let msg = tokio::time::timeout(std::time::Duration::from_millis(100), sub_wf1.recv()).await;
        assert!(msg.is_ok());

        // wf-2 should NOT receive (timeout)
        let msg = tokio::time::timeout(std::time::Duration::from_millis(100), sub_wf2.recv()).await;
        assert!(msg.is_err());
    }

    #[tokio::test]
    async fn test_message_bus_broadcast() {
        let bus = MessageBus::new(100);

        let mut sub1 = bus.subscribe_broadcast();
        let mut sub2 = bus.subscribe_broadcast();

        bus.broadcast(BusMessage::Shutdown).unwrap();

        // Both should receive
        let msg1 = tokio::time::timeout(std::time::Duration::from_millis(100), sub1.recv()).await;
        assert!(msg1.is_ok());

        let msg2 = tokio::time::timeout(std::time::Duration::from_millis(100), sub2.recv()).await;
        assert!(msg2.is_ok());
    }

    #[tokio::test]
    async fn test_publish_workflow_event_fanout_to_topic_and_broadcast() {
        let bus = MessageBus::new(100);
        let mut topic_sub = bus.subscribe("workflow:wf-1").await;
        let mut broadcast_sub = bus.subscribe_broadcast();

        let delivered = bus
            .publish_workflow_event(
                "wf-1",
                BusMessage::StatusUpdate {
                    workflow_id: "wf-1".to_string(),
                    status: "running".to_string(),
                },
            )
            .await
            .unwrap();

        assert_eq!(delivered, 1);

        let topic_msg =
            tokio::time::timeout(std::time::Duration::from_millis(100), topic_sub.recv())
                .await
                .unwrap()
                .unwrap();
        match topic_msg {
            BusMessage::StatusUpdate {
                workflow_id,
                status,
            } => {
                assert_eq!(workflow_id, "wf-1");
                assert_eq!(status, "running");
            }
            _ => panic!("Expected StatusUpdate on workflow topic"),
        }

        let broadcast_msg =
            tokio::time::timeout(std::time::Duration::from_millis(100), broadcast_sub.recv())
                .await
                .unwrap()
                .unwrap();
        match broadcast_msg {
            BusMessage::StatusUpdate {
                workflow_id,
                status,
            } => {
                assert_eq!(workflow_id, "wf-1");
                assert_eq!(status, "running");
            }
            _ => panic!("Expected StatusUpdate on broadcast"),
        }
    }

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

        let msg = tokio::time::timeout(std::time::Duration::from_millis(100), sub.recv())
            .await
            .unwrap()
            .unwrap();

        match msg {
            BusMessage::TerminalCompleted(e) => {
                assert_eq!(e.terminal_id, "terminal-1");
                assert_eq!(e.workflow_id, "wf-1");
            }
            _ => panic!("Wrong message type"),
        }
    }

    // =========================================================================
    // Test Suite 6: OrchestratorAgent
    // =========================================================================

    #[test]
    fn test_instruction_parsing() {
        let json = r#"{"type":"send_to_terminal","terminal_id":"t1","message":"Do something"}"#;

        let instruction: OrchestratorInstruction = serde_json::from_str(json).unwrap();

        match instruction {
            OrchestratorInstruction::SendToTerminal {
                terminal_id,
                message,
            } => {
                assert_eq!(terminal_id, "t1");
                assert_eq!(message, "Do something");
            }
            _ => panic!("Wrong instruction type"),
        }
    }

    #[test]
    fn test_all_instruction_parsing() {
        let test_cases = vec![
            (
                r#"{"type":"start_task","task_id":"task-1","instruction":"Build API"}"#,
                "start_task",
            ),
            (
                r#"{"type":"create_task","task_id":"task-runtime","name":"Runtime task","description":"Create work","branch":null,"order_index":0}"#,
                "create_task",
            ),
            (
                r#"{"type":"create_terminal","terminal_id":"term-runtime","task_id":"task-runtime","cli_type_id":"cli-claude-code","model_config_id":"model-claude-sonnet","custom_base_url":null,"custom_api_key":null,"role":"coder","role_description":null,"order_index":0,"auto_confirm":true}"#,
                "create_terminal",
            ),
            (
                r#"{"type":"start_terminal","terminal_id":"term-runtime","instruction":"Implement feature"}"#,
                "start_terminal",
            ),
            (
                r#"{"type":"close_terminal","terminal_id":"term-runtime","final_status":"completed"}"#,
                "close_terminal",
            ),
            (
                r#"{"type":"complete_task","task_id":"task-runtime","summary":"done"}"#,
                "complete_task",
            ),
            (
                r#"{"type":"set_workflow_planning_complete","summary":"graph closed"}"#,
                "set_workflow_planning_complete",
            ),
            (
                r#"{"type":"review_code","terminal_id":"t1","commit_hash":"abc123"}"#,
                "review_code",
            ),
            (
                r#"{"type":"fix_issues","terminal_id":"t1","issues":["bug1","bug2"]}"#,
                "fix_issues",
            ),
            (
                r#"{"type":"merge_branch","source_branch":"feature","target_branch":"main"}"#,
                "merge_branch",
            ),
            (
                r#"{"type":"pause_workflow","reason":"manual review"}"#,
                "pause_workflow",
            ),
            (
                r#"{"type":"complete_workflow","summary":"done"}"#,
                "complete_workflow",
            ),
            (
                r#"{"type":"fail_workflow","reason":"error"}"#,
                "fail_workflow",
            ),
        ];

        for (json, expected_type) in test_cases {
            let _instruction: OrchestratorInstruction = serde_json::from_str(json).unwrap();
            let json_obj = serde_json::from_str::<serde_json::Value>(json).unwrap();
            assert_eq!(json_obj["type"], expected_type);
        }
    }

    #[tokio::test]
    async fn test_agent_creation() {
        use std::{path::PathBuf, sync::Arc};

        use db::DBService;
        use sqlx::sqlite::SqlitePoolOptions;

        // Create in-memory database with migrations
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool });

        // Create message bus with capacity (SharedMessageBus = Arc<MessageBus>)
        let message_bus = Arc::new(MessageBus::new(100));

        // Create orchestrator config
        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "test-key".to_string(),
            model: "gpt-4o".to_string(),
            ..Default::default()
        };

        // Verify config validation works
        assert!(
            config.validate().is_ok(),
            "Config with api_key should be valid"
        );

        // Create mock LLM client for testing
        let mock_llm_client = Box::new(MockLLMClient::new());

        // Create agent using mock LLM client (full agent creation test)
        let result = OrchestratorAgent::with_llm_client(
            config,
            "test-workflow".to_string(),
            message_bus,
            db,
            mock_llm_client,
        );

        // Verify agent creation succeeds
        assert!(
            result.is_ok(),
            "Agent creation with mock LLM client should succeed"
        );

        let _agent = result.unwrap();

        // Agent creation succeeded - this verifies:
        // 1. Mock LLM client infrastructure works
        // 2. Agent can be created with mock client
        // 3. All dependencies (DB, MessageBus, State) are properly initialized
        // Note: state field is private, so we can't inspect it directly
    }

    #[tokio::test]
    async fn test_execute_instruction_supports_runtime_planning_array() {
        use std::{path::PathBuf, sync::Arc};

        use crate::services::terminal::{PromptWatcher, process::ProcessManager};
        use chrono::Utc;
        use db::{DBService, models::Workflow};
        use sqlx::sqlite::SqlitePoolOptions;
        use uuid::Uuid;

        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");
        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });
        let message_bus = Arc::new(MessageBus::new(100));

        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("runtime-project")
        .bind(Utc::now())
        .bind(Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        let workflow = Workflow {
            id: Uuid::new_v4().to_string(),
            project_id,
            name: "Runtime Planning Workflow".to_string(),
            description: Some("Runtime planning test".to_string()),
            status: "running".to_string(),
            execution_mode: "agent_planned".to_string(),
            initial_goal: Some("Decide whether any work is required".to_string()),
            use_slash_commands: false,
            orchestrator_enabled: false,
            orchestrator_api_type: None,
            orchestrator_base_url: None,
            orchestrator_api_key: None,
            orchestrator_model: None,
            error_terminal_enabled: false,
            error_terminal_cli_id: None,
            error_terminal_model_id: None,
            merge_terminal_cli_id: "cli-claude-code".to_string(),
            merge_terminal_model_id: "model-claude-sonnet".to_string(),
            target_branch: "main".to_string(),
            git_watcher_enabled: true,
            ready_at: Some(Utc::now()),
            started_at: Some(Utc::now()),
            completed_at: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
        };
        Workflow::create(&pool, &workflow).await.unwrap();

        let mut agent = OrchestratorAgent::with_llm_client(
            OrchestratorConfig {
                api_key: "test-key".to_string(),
                ..Default::default()
            },
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            Box::new(MockLLMClient::new()),
        )
        .expect("agent should be created");

        let process_manager = Arc::new(ProcessManager::new());
        let prompt_watcher = PromptWatcher::new(message_bus.clone(), process_manager.clone());
        agent.attach_runtime_actions(Arc::new(RuntimeActionService::new(
            db.clone(),
            message_bus.clone(),
            process_manager,
            prompt_watcher,
        )));

        agent
            .execute_instruction(
                r#"[
                    {"type":"create_task","task_id":"task-runtime","name":"Runtime Task","description":"Nothing to execute","order_index":0},
                    {"type":"complete_task","task_id":"task-runtime","summary":"No terminals needed"},
                    {"type":"set_workflow_planning_complete","summary":"Planning is finished"}
                ]"#,
            )
            .await
            .expect("runtime planning array should execute");

        let task = db::models::WorkflowTask::find_by_id(&pool, "task-runtime")
            .await
            .unwrap()
            .expect("task should be created");
        assert_eq!(task.status, "completed");

        let workflow = Workflow::find_by_id(&pool, &workflow.id)
            .await
            .unwrap()
            .expect("workflow should still exist");
        assert_eq!(workflow.status, "completed");
    }

    #[tokio::test]
    async fn test_execute_instruction_send_to_terminal() {
        use std::{path::PathBuf, sync::Arc};

        use db::DBService;
        use sqlx::sqlite::SqlitePoolOptions;
        use uuid::Uuid;

        // Create in-memory database with migrations
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });

        // Create workflow, task and terminal in database
        let workflow_id = Uuid::new_v4().to_string();
        let task_id = Uuid::new_v4().to_string();
        let terminal_id = Uuid::new_v4().to_string();
        let pty_session_id = Uuid::new_v4().to_string();

        // First, we need to insert a project (required by workflow)
        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("test-project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow
        sqlx::query(
            r"
            INSERT INTO workflow (
                id, project_id, name, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            ",
        )
        .bind(&workflow_id)
        .bind(&project_id)
        .bind("test-workflow")
        .bind("main")
        .bind("cli-claude-code") // From migration
        .bind("model-claude-sonnet") // From migration
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow_task
        sqlx::query(
            r"
            INSERT INTO workflow_task (
                id, workflow_id, name, branch, order_index,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
            ",
        )
        .bind(&task_id)
        .bind(&workflow_id)
        .bind("test-task")
        .bind("feature/test")
        .bind(0)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert terminal record
        sqlx::query(
            r"
            INSERT INTO terminal (
                id, workflow_task_id, cli_type_id, model_config_id,
                order_index, status, pty_session_id, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ",
        )
        .bind(&terminal_id)
        .bind(&task_id)
        .bind("cli-claude-code") // From migration
        .bind("model-claude-sonnet") // From migration
        .bind(0)
        .bind("working")
        .bind(&pty_session_id)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config.clone(),
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        // Subscribe to terminal topic to verify message sent
        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;

        // Execute instruction
        let instruction_json = format!(
            r#"{{"type":"send_to_terminal","terminal_id":"{terminal_id}","message":"echo test"}}"#
        );

        // Run in task to allow async message propagation
        tokio::spawn(async move {
            let _ = agent.execute_instruction(instruction_json.as_str()).await;
        });

        // Verify message received on terminal topic
        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(500), terminal_rx.recv()).await;

        assert!(timeout.is_ok(), "Should receive message within timeout");

        let msg = timeout.unwrap();
        assert!(msg.is_some(), "Should receive a message");

        match msg.as_ref().unwrap() {
            BusMessage::TerminalMessage { message } => {
                assert_eq!(message, "echo test");
            }
            other => panic!("Expected TerminalMessage, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn test_execute_instruction_complete_workflow_success() {
        use std::{path::PathBuf, sync::Arc};

        use db::{DBService, models::Workflow};
        use sqlx::sqlite::SqlitePoolOptions;
        use uuid::Uuid;

        // Create in-memory database with migrations
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });

        // Create workflow in database
        let workflow_id = Uuid::new_v4().to_string();

        // First, we need to insert a project (required by workflow)
        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("test-project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow
        sqlx::query(
            r"
            INSERT INTO workflow (
                id, project_id, name, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            ",
        )
        .bind(&workflow_id)
        .bind(&project_id)
        .bind("test-workflow")
        .bind("main")
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config.clone(),
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        // Subscribe to workflow topic
        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{workflow_id}"))
            .await;

        // Execute instruction
        let instruction_json =
            r#"{"type":"complete_workflow","summary":"All tasks completed successfully"}"#;

        tokio::spawn(async move {
            let _ = agent.execute_instruction(instruction_json).await;
        });

        // Verify workflow status updated
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;

        let updated_workflow = Workflow::find_by_id(&pool, &workflow_id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(updated_workflow.status, "completed");

        // Verify status update event published
        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(500), workflow_rx.recv()).await;

        assert!(timeout.is_ok(), "Should receive status update event");

        let msg = timeout.unwrap().unwrap();
        match msg {
            BusMessage::StatusUpdate {
                workflow_id: wf_id,
                status,
            } => {
                assert_eq!(wf_id, workflow_id);
                assert_eq!(status, "completed");
            }
            _ => panic!("Expected StatusUpdate, got {msg:?}"),
        }
    }

    #[tokio::test]
    async fn test_execute_instruction_fail_workflow() {
        use std::{path::PathBuf, sync::Arc};

        use db::{DBService, models::Workflow};
        use sqlx::sqlite::SqlitePoolOptions;
        use uuid::Uuid;

        // Create in-memory database with migrations
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });

        // Create workflow in database
        let workflow_id = Uuid::new_v4().to_string();

        // First, we need to insert a project (required by workflow)
        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("test-project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow
        sqlx::query(
            r"
            INSERT INTO workflow (
                id, project_id, name, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            ",
        )
        .bind(&workflow_id)
        .bind(&project_id)
        .bind("test-workflow")
        .bind("main")
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config.clone(),
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        // Execute instruction with failure
        let instruction_json =
            r#"{"type":"fail_workflow","reason":"Critical error in task execution"}"#;

        agent.execute_instruction(instruction_json).await.unwrap();

        // Verify workflow status is failed
        let updated_workflow = Workflow::find_by_id(&pool, &workflow_id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(updated_workflow.status, "failed");
    }

    // =========================================================================
    // Test Suite 6.5: StartTask and Auto-Dispatch (Phase 20)
    // =========================================================================

    /// Helper function to set up workflow with multiple terminals for testing
    async fn setup_workflow_with_terminals(
        terminal_count: usize,
        include_pty: bool,
    ) -> (
        Arc<db::DBService>,
        String,
        String,
        Vec<(String, Option<String>)>,
    ) {
        use std::path::PathBuf;

        use db::DBService;
        use sqlx::sqlite::SqlitePoolOptions;
        use uuid::Uuid;

        // Create in-memory database with migrations
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });

        let workflow_id = Uuid::new_v4().to_string();
        let task_id = Uuid::new_v4().to_string();

        // Insert project
        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("test-project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow
        sqlx::query(
            r"
            INSERT INTO workflow (
                id, project_id, name, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            ",
        )
        .bind(&workflow_id)
        .bind(&project_id)
        .bind("test-workflow")
        .bind("main")
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow_task
        sqlx::query(
            r"
            INSERT INTO workflow_task (
                id, workflow_id, name, branch, order_index,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
            ",
        )
        .bind(&task_id)
        .bind(&workflow_id)
        .bind("test-task")
        .bind("feature/test")
        .bind(0)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        let mut terminals = Vec::new();
        for index in 0..terminal_count {
            let terminal_id = Uuid::new_v4().to_string();
            let pty_session_id = if include_pty {
                Some(Uuid::new_v4().to_string())
            } else {
                None
            };

            sqlx::query(
                r"
                INSERT INTO terminal (
                    id, workflow_task_id, cli_type_id, model_config_id,
                    order_index, status, pty_session_id, created_at, updated_at
                ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
                ",
            )
            .bind(&terminal_id)
            .bind(&task_id)
            .bind("cli-claude-code")
            .bind("model-claude-sonnet")
            .bind(index as i32)
            .bind("waiting")
            .bind(pty_session_id.as_deref())
            .bind(chrono::Utc::now())
            .bind(chrono::Utc::now())
            .execute(&pool)
            .await
            .unwrap();

            terminals.push((terminal_id, pty_session_id));
        }

        (db, workflow_id, task_id, terminals)
    }

    #[tokio::test]
    async fn test_execute_instruction_start_task() {
        use db::models::{Terminal, WorkflowTask};

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(1, true).await;
        let (terminal_id, pty_session_id) = terminals.first().cloned().unwrap();
        let pty_session_id = pty_session_id.expect("PTY should be present");

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;

        let instruction_json = format!(
            r#"{{"type":"start_task","task_id":"{}","instruction":"echo start"}}"#,
            task_id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(result.is_ok(), "StartTask should succeed");

        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(500), terminal_rx.recv()).await;

        assert!(timeout.is_ok(), "Should receive terminal message");
        let msg = timeout.unwrap().unwrap();
        match msg {
            BusMessage::TerminalMessage { message } => {
                assert_eq!(message, "echo start");
            }
            other => panic!("Expected TerminalMessage, got {other:?}"),
        }

        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "working");

        let task = WorkflowTask::find_by_id(&db.pool, &task_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(task.status, "running");
    }

    #[tokio::test]
    async fn test_execute_instruction_start_task_skips_dispatch_when_terminal_not_waiting() {
        use db::models::{Terminal, WorkflowTask};

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(1, true).await;
        let (terminal_id, pty_session_id) = terminals.first().cloned().unwrap();
        let pty_session_id = pty_session_id.expect("PTY should be present");

        sqlx::query("UPDATE terminal SET status = 'working' WHERE id = ?")
            .bind(&terminal_id)
            .execute(&db.pool)
            .await
            .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{}", workflow_id))
            .await;
        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;

        let instruction_json = format!(
            r#"{{"type":"start_task","task_id":"{}","instruction":"echo start"}}"#,
            task_id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(
            result.is_ok(),
            "StartTask should skip safely when terminal is not waiting"
        );

        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), terminal_rx.recv()).await;
        assert!(timeout.is_err(), "Should not dispatch any terminal message");

        let workflow_timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), workflow_rx.recv()).await;
        assert!(
            workflow_timeout.is_err(),
            "Should not publish workflow status updates when dispatch is skipped"
        );

        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "working");

        let task = WorkflowTask::find_by_id(&db.pool, &task_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(task.status, "pending");
    }

    #[tokio::test]
    async fn test_execute_instruction_start_task_uses_latest_pty_after_cas() {
        use db::models::Terminal;

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(1, true).await;
        let (terminal_id, original_pty) = terminals.first().cloned().unwrap();
        let original_pty = original_pty.expect("PTY should be present");
        let fresh_pty = format!("{}-fresh", original_pty);

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        sqlx::query("UPDATE terminal SET pty_session_id = ?1 WHERE id = ?2")
            .bind(&fresh_pty)
            .bind(&terminal_id)
            .execute(&db.pool)
            .await
            .unwrap();

        let mut old_session_rx = message_bus.subscribe(&original_pty).await;
        let mut new_session_rx = message_bus.subscribe(&fresh_pty).await;

        let instruction_json = format!(
            r#"{{"type":"start_task","task_id":"{}","instruction":"echo latest-pty"}}"#,
            task_id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(
            result.is_ok(),
            "StartTask should succeed with refreshed PTY binding"
        );

        let old_timeout = tokio::time::timeout(
            tokio::time::Duration::from_millis(250),
            old_session_rx.recv(),
        )
        .await;
        assert!(
            old_timeout.is_err(),
            "Legacy PTY topic should not receive dispatch after metadata refresh"
        );

        let new_msg = tokio::time::timeout(
            tokio::time::Duration::from_millis(500),
            new_session_rx.recv(),
        )
        .await
        .expect("Expected message on refreshed PTY topic")
        .expect("PTY subscriber should remain active");

        match new_msg {
            BusMessage::TerminalMessage { message } => {
                assert_eq!(message, "echo latest-pty");
            }
            other => panic!("Expected TerminalMessage, got {other:?}"),
        }

        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "working");
        assert_eq!(terminal.pty_session_id.as_deref(), Some(fresh_pty.as_str()));
    }

    #[tokio::test]
    async fn test_execute_instruction_send_to_terminal_requires_working_status() {
        use db::models::{Terminal, WorkflowTask};

        let (db, workflow, terminal) = setup_test_workflow().await;
        let pty_session_id = terminal
            .pty_session_id
            .clone()
            .expect("PTY should be present");

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;
        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{}", workflow.id))
            .await;

        let instruction_json = format!(
            r#"{{"type":"send_to_terminal","terminal_id":"{}","message":"echo gated"}}"#,
            terminal.id
        );

        // setup_test_workflow creates terminal in waiting state: should skip dispatch.
        let first_result = agent.execute_instruction(&instruction_json).await;
        assert!(
            first_result.is_ok(),
            "SendToTerminal should return ok when skipping non-working terminal"
        );

        let first_timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), terminal_rx.recv()).await;
        assert!(
            first_timeout.is_err(),
            "Non-working terminal should not receive dispatched message"
        );

        let first_workflow_timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), workflow_rx.recv()).await;
        assert!(
            first_workflow_timeout.is_err(),
            "Skipped SendToTerminal should not publish workflow status updates"
        );

        let terminal_after_skip = Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal_after_skip.status, "waiting");

        let task_after_skip = WorkflowTask::find_by_id(&db.pool, &terminal.workflow_task_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(task_after_skip.status, "pending");

        sqlx::query("UPDATE terminal SET status = 'working' WHERE id = ?")
            .bind(&terminal.id)
            .execute(&db.pool)
            .await
            .unwrap();

        let second_result = agent.execute_instruction(&instruction_json).await;
        assert!(
            second_result.is_ok(),
            "SendToTerminal should dispatch after status becomes working"
        );

        let msg = tokio::time::timeout(tokio::time::Duration::from_millis(500), terminal_rx.recv())
            .await
            .expect("expected terminal message after legal status transition")
            .expect("terminal channel should remain open");

        match msg {
            BusMessage::TerminalMessage { message } => {
                assert_eq!(message, "echo gated");
            }
            other => panic!("Expected TerminalMessage, got {other:?}"),
        }

        let second_workflow_timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), workflow_rx.recv()).await;
        assert!(
            second_workflow_timeout.is_err(),
            "SendToTerminal dispatch should not publish workflow status updates"
        );

        let terminal_after_dispatch = Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal_after_dispatch.status, "working");

        let task_after_dispatch = WorkflowTask::find_by_id(&db.pool, &terminal.workflow_task_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(task_after_dispatch.status, "pending");
    }

    #[tokio::test]
    async fn test_execute_instruction_send_to_terminal_skips_non_working_without_pty() {
        use db::models::Terminal;

        let (db, workflow, terminal) = setup_test_workflow().await;

        // Simulate a completed terminal after teardown: status is non-working and PTY is gone.
        sqlx::query("UPDATE terminal SET status = 'completed', pty_session_id = NULL WHERE id = ?")
            .bind(&terminal.id)
            .execute(&db.pool)
            .await
            .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus,
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let instruction_json = format!(
            r#"{{"type":"send_to_terminal","terminal_id":"{}","message":"echo should-skip"}}"#,
            terminal.id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(
            result.is_ok(),
            "SendToTerminal should skip non-working terminal even when PTY is missing"
        );

        let terminal_after = Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal_after.status, "completed");
        assert!(terminal_after.pty_session_id.is_none());
    }

    #[tokio::test]
    async fn test_execute_instruction_start_task_claude_code_retries_submit_once_on_first_dispatch()
    {
        use db::models::Terminal;

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(1, true).await;
        let (terminal_id, pty_session_id) = terminals.first().cloned().unwrap();
        let pty_session_id = pty_session_id.expect("PTY should be present");

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id,
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut terminal_rx = message_bus.subscribe(&pty_session_id).await;
        let terminal_input_topic = format!("terminal.input.{terminal_id}");
        let mut input_rx = message_bus.subscribe(&terminal_input_topic).await;

        let instruction_json = format!(
            r#"{{"type":"start_task","task_id":"{}","instruction":"echo start"}}"#,
            task_id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(
            result.is_ok(),
            "StartTask should succeed for claude-code terminal"
        );

        let first_dispatch =
            tokio::time::timeout(tokio::time::Duration::from_millis(500), terminal_rx.recv())
                .await
                .expect("expected first terminal message")
                .expect("terminal message channel should stay open");

        match first_dispatch {
            BusMessage::TerminalMessage { message } => {
                assert_eq!(message, "echo start");
            }
            other => panic!("Expected TerminalMessage for first dispatch, got {other:?}"),
        }

        let submit = tokio::time::timeout(tokio::time::Duration::from_millis(900), input_rx.recv())
            .await
            .expect("expected claude-code submit retry")
            .expect("terminal input channel should stay open");

        match submit {
            BusMessage::TerminalInput {
                terminal_id: tid,
                session_id,
                input,
                ..
            } => {
                assert_eq!(tid, terminal_id);
                assert_eq!(session_id, pty_session_id);
                assert_eq!(input, "");
            }
            other => panic!("Expected TerminalInput submit retry, got {other:?}"),
        }

        let no_second_submit =
            tokio::time::timeout(tokio::time::Duration::from_millis(700), input_rx.recv()).await;
        assert!(
            no_second_submit.is_err(),
            "claude-code first dispatch should submit exactly once"
        );

        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "working");
    }

    #[tokio::test]
    async fn test_execute_instruction_start_task_codex_uses_terminal_input_with_submit() {
        use db::models::Terminal;

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(1, true).await;
        let (terminal_id, pty_session_id) = terminals.first().cloned().unwrap();
        let pty_session_id = pty_session_id.expect("PTY should be present");

        sqlx::query("UPDATE terminal SET cli_type_id = ? WHERE id = ?")
            .bind("cli-codex")
            .bind(&terminal_id)
            .execute(&db.pool)
            .await
            .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id,
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let terminal_input_topic = format!("terminal.input.{terminal_id}");
        let mut input_rx = message_bus.subscribe(&terminal_input_topic).await;

        let instruction_json = format!(
            r#"{{"type":"start_task","task_id":"{}","instruction":"echo start"}}"#,
            task_id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(
            result.is_ok(),
            "StartTask should succeed for codex terminal"
        );

        let first = tokio::time::timeout(tokio::time::Duration::from_millis(700), input_rx.recv())
            .await
            .expect("expected first terminal input for codex")
            .expect("terminal input channel should stay open");

        match first {
            BusMessage::TerminalInput {
                terminal_id: tid,
                session_id,
                input,
                ..
            } => {
                assert_eq!(tid, terminal_id);
                assert_eq!(session_id, pty_session_id);
                assert_eq!(input, "echo start");
            }
            other => panic!("Expected TerminalInput for codex instruction, got {other:?}"),
        }

        let submit_1 =
            tokio::time::timeout(tokio::time::Duration::from_millis(700), input_rx.recv())
                .await
                .expect("expected first codex submit keystroke")
                .expect("terminal input channel should stay open");

        match submit_1 {
            BusMessage::TerminalInput { input, .. } => {
                assert_eq!(input, "");
            }
            other => panic!("Expected first codex submit keystroke, got {other:?}"),
        }

        let submit_2 =
            tokio::time::timeout(tokio::time::Duration::from_millis(900), input_rx.recv())
                .await
                .expect("expected second codex submit keystroke")
                .expect("terminal input channel should stay open");

        match submit_2 {
            BusMessage::TerminalInput { input, .. } => {
                assert_eq!(input, "");
            }
            other => panic!("Expected second codex submit keystroke, got {other:?}"),
        }

        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "working");
    }

    #[tokio::test]
    async fn test_execute_instruction_start_task_no_pty() {
        use db::models::{Terminal, WorkflowTask};

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(1, false).await;
        let (terminal_id, _) = terminals.first().cloned().unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient::new());

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus,
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let instruction_json = format!(
            r#"{{"type":"start_task","task_id":"{}","instruction":"echo start"}}"#,
            task_id
        );

        let result = agent.execute_instruction(&instruction_json).await;
        assert!(result.is_err(), "StartTask should fail without PTY");

        let terminal = Terminal::find_by_id(&db.pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(terminal.status, "failed");

        let task = WorkflowTask::find_by_id(&db.pool, &task_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(task.status, "failed");
    }

    // =========================================================================
    // Test Suite 7: handle_git_event (Task 8)
    // =========================================================================

    /// Helper function to set up test workflow with terminal
    async fn setup_test_workflow() -> (
        Arc<db::DBService>,
        db::models::Workflow,
        db::models::Terminal,
    ) {
        use std::{path::PathBuf, sync::Arc};

        use db::DBService;
        use sqlx::sqlite::SqlitePoolOptions;
        use uuid::Uuid;

        // Create in-memory database with migrations
        let pool = SqlitePoolOptions::new().connect(":memory:").await.unwrap();

        // Run migrations
        let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
        let migration_dir = manifest_dir
            .ancestors()
            .nth(1)
            .unwrap()
            .join("db")
            .join("migrations");

        let migrator = sqlx::migrate::Migrator::new(migration_dir).await.unwrap();
        migrator.run(&pool).await.unwrap();

        let db = Arc::new(DBService { pool: pool.clone() });

        // Create workflow, task and terminal in database
        let workflow_id = Uuid::new_v4().to_string();
        let task_id = Uuid::new_v4().to_string();
        let terminal_id = Uuid::new_v4().to_string();
        let pty_session_id = Uuid::new_v4().to_string();

        // First, we need to insert a project (required by workflow)
        let project_id = Uuid::new_v4();
        sqlx::query(
            "INSERT INTO projects (id, name, created_at, updated_at) VALUES (?1, ?2, ?3, ?4)",
        )
        .bind(project_id)
        .bind("test-project")
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow
        sqlx::query(
            r"
            INSERT INTO workflow (
                id, project_id, name, target_branch,
                merge_terminal_cli_id, merge_terminal_model_id,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)
            ",
        )
        .bind(&workflow_id)
        .bind(&project_id)
        .bind("test-workflow")
        .bind("main")
        .bind("cli-claude-code") // From migration
        .bind("model-claude-sonnet") // From migration
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert workflow_task
        sqlx::query(
            r"
            INSERT INTO workflow_task (
                id, workflow_id, name, branch, order_index,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
            ",
        )
        .bind(&task_id)
        .bind(&workflow_id)
        .bind("test-task")
        .bind("feature/test")
        .bind(0)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Insert terminal record
        sqlx::query(
            r"
            INSERT INTO terminal (
                id, workflow_task_id, cli_type_id, model_config_id,
                order_index, status, pty_session_id, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)
            ",
        )
        .bind(&terminal_id)
        .bind(&task_id)
        .bind("cli-claude-code") // From migration
        .bind("model-claude-sonnet") // From migration
        .bind(0)
        .bind("waiting")
        .bind(&pty_session_id)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&pool)
        .await
        .unwrap();

        // Fetch and return workflow and terminal
        let workflow = db::models::Workflow::find_by_id(&pool, &workflow_id)
            .await
            .unwrap()
            .unwrap();
        let terminal = db::models::Terminal::find_by_id(&pool, &terminal_id)
            .await
            .unwrap()
            .unwrap();

        (db, workflow, terminal)
    }

    #[tokio::test]
    async fn test_handle_git_event_terminal_completed() {
        let (db, workflow, terminal) = setup_test_workflow().await;

        // Simulate a terminal that has been quiet for >40s so completion can proceed immediately.
        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config.clone(),
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        // Subscribe to workflow topic
        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{}", workflow.id))
            .await;

        // Create valid commit message (KV format, not JSON)
        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow.id, terminal.workflow_task_id, terminal.id
        );

        // Handle git event
        agent
            .handle_git_event(&workflow.id, "abc123", "main", commit_message.as_str())
            .await
            .unwrap();

        // Verify terminal status updated
        let updated_terminal = db::models::Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(updated_terminal.status, "completed");
        assert!(updated_terminal.pty_session_id.is_none());
        assert!(updated_terminal.process_id.is_none());
        assert!(updated_terminal.session_id.is_none());
        assert!(updated_terminal.execution_process_id.is_none());

        // Verify event published
        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(500), workflow_rx.recv()).await;

        assert!(timeout.is_ok());
    }

    #[tokio::test]
    async fn test_handle_git_event_terminal_completed_defers_when_not_quiet() {
        let (db, workflow, terminal) = setup_test_workflow().await;

        // Terminal is currently executing but still within quiet window.
        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now())
        .bind(&terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{}", workflow.id))
            .await;

        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow.id, terminal.workflow_task_id, terminal.id
        );

        // Terminal has recent activity (updated_at from setup), so completion should be deferred.
        agent
            .handle_git_event(&workflow.id, "abc123", "main", commit_message.as_str())
            .await
            .unwrap();

        let updated_terminal = db::models::Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(updated_terminal.status, "working");
        assert!(updated_terminal.completed_at.is_none());

        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), workflow_rx.recv()).await;
        assert!(
            timeout.is_err(),
            "Quiet-window deferral should not emit completion status updates"
        );
    }

    #[tokio::test]
    async fn test_handle_git_event_terminal_completed_race_keeps_terminal_unfinished_within_quiet_window()
     {
        let (db, workflow, terminal) = setup_test_workflow().await;

        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now())
        .bind(&terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{}", workflow.id))
            .await;

        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow.id, terminal.workflow_task_id, terminal.id
        );

        let first_call = agent.handle_git_event(
            &workflow.id,
            "race_completion_hash_1",
            "main",
            commit_message.as_str(),
        );
        let second_call = agent.handle_git_event(
            &workflow.id,
            "race_completion_hash_2",
            "main",
            commit_message.as_str(),
        );

        let (first_result, second_result) = tokio::join!(first_call, second_call);
        assert!(
            first_result.is_ok(),
            "First completion event should be handled safely"
        );
        assert!(
            second_result.is_ok(),
            "Second completion event should be handled safely"
        );

        let updated_terminal = db::models::Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(updated_terminal.status, "working");
        assert!(updated_terminal.completed_at.is_none());

        let timeout =
            tokio::time::timeout(tokio::time::Duration::from_millis(250), workflow_rx.recv()).await;
        assert!(
            timeout.is_err(),
            "Concurrent in-window completions should not emit finalized status updates"
        );
    }

    #[tokio::test]
    async fn test_handle_git_event_terminal_completed_ignores_non_working_terminal() {
        let (db, workflow, terminal) = setup_test_workflow().await;

        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'waiting', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus,
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow.id, terminal.workflow_task_id, terminal.id
        );

        agent
            .handle_git_event(&workflow.id, "abc123", "main", commit_message.as_str())
            .await
            .unwrap();

        let updated_terminal = db::models::Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(updated_terminal.status, "waiting");
        assert!(updated_terminal.completed_at.is_none());
    }

    #[tokio::test]
    async fn test_handle_git_event_terminal_completed_ignores_out_of_order_terminal() {
        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(2, true).await;
        let first_terminal_id = terminals[0].0.clone();
        let second_terminal_id = terminals[1].0.clone();

        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'waiting', started_at = ?1, updated_at = ?1
            WHERE workflow_task_id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&task_id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus,
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow_id, task_id, second_terminal_id
        );

        agent
            .handle_git_event(&workflow_id, "def456", "main", commit_message.as_str())
            .await
            .unwrap();

        let first_terminal = db::models::Terminal::find_by_id(&db.pool, &first_terminal_id)
            .await
            .unwrap()
            .unwrap();
        let second_terminal = db::models::Terminal::find_by_id(&db.pool, &second_terminal_id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(first_terminal.status, "waiting");
        assert_eq!(second_terminal.status, "waiting");
        assert!(first_terminal.completed_at.is_none());
        assert!(second_terminal.completed_at.is_none());
    }

    #[tokio::test]
    async fn test_handle_git_event_terminal_completed_dispatches_next_terminal_without_preinitialized_task_state()
     {
        use db::models::Terminal;

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(2, true).await;
        let (first_terminal_id, _) = terminals[0].clone();
        let (second_terminal_id, second_pty_session_id) = terminals[1].clone();
        let second_pty_session_id = second_pty_session_id.expect("Second terminal should have PTY");

        // Make first terminal actively working and old enough to satisfy quiet-window checks.
        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&first_terminal_id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut second_terminal_rx = message_bus.subscribe(&second_pty_session_id).await;

        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow_id, task_id, first_terminal_id
        );

        let result = agent
            .handle_git_event(
                &workflow_id,
                "metadata_handoff_without_state_1",
                "main",
                &commit_message,
            )
            .await;
        assert!(
            result.is_ok(),
            "Metadata completion event should be handled"
        );

        let first_terminal = Terminal::find_by_id(&db.pool, &first_terminal_id)
            .await
            .unwrap()
            .unwrap();
        let second_terminal = Terminal::find_by_id(&db.pool, &second_terminal_id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(first_terminal.status, "completed");
        assert_eq!(second_terminal.status, "working");

        let dispatched = tokio::time::timeout(
            tokio::time::Duration::from_millis(500),
            second_terminal_rx.recv(),
        )
        .await;
        assert!(
            dispatched.is_ok(),
            "Second terminal should receive dispatch message after metadata handoff"
        );
    }

    #[tokio::test]
    async fn test_handle_git_event_review_pass_publishes_terminal_status_update() {
        let (db, workflow, terminal) = setup_test_workflow().await;

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut workflow_rx = message_bus
            .subscribe(&format!("workflow:{}", workflow.id))
            .await;

        let commit_message = format!(
            r#"Review passed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: reviewer-1
status: review_pass
reviewed_terminal: {}"#,
            workflow.id, terminal.workflow_task_id, terminal.id
        );

        agent
            .handle_git_event(&workflow.id, "abc123", "main", commit_message.as_str())
            .await
            .unwrap();

        let msg = tokio::time::timeout(tokio::time::Duration::from_millis(500), workflow_rx.recv())
            .await
            .expect("Should receive workflow message")
            .expect("Message should not be None");

        match msg {
            BusMessage::TerminalStatusUpdate {
                workflow_id,
                terminal_id,
                status,
            } => {
                assert_eq!(workflow_id, workflow.id);
                assert_eq!(terminal_id, terminal.id);
                assert_eq!(status, "review_passed");
            }
            _ => panic!("Expected TerminalStatusUpdate, got {msg:?}"),
        }
    }

    #[tokio::test]
    async fn test_handle_git_event_workflow_mismatch() {
        let (db, workflow, _terminal) = setup_test_workflow().await;

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config.clone(),
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        // Create commit with different workflow ID (KV format)
        let commit_message = r#"Terminal completed

---METADATA---
workflow_id: different-workflow
task_id: task-1
terminal_id: term-1
status: completed
next_action: handoff"#;

        // Should succeed but do nothing (workflow mismatch)
        let result = agent
            .handle_git_event(&workflow.id, "abc123", "main", commit_message)
            .await;

        assert!(result.is_ok());
    }

    // =========================================================================
    // Test Suite 8: LLM Retry with Backoff (Task 12)
    // =========================================================================

    #[tokio::test]
    async fn test_llm_retry_with_backoff() {
        use std::sync::atomic::{AtomicUsize, Ordering};

        struct RetryResponder {
            counter: AtomicUsize,
        }

        impl wiremock::Respond for RetryResponder {
            fn respond(&self, _request: &wiremock::Request) -> wiremock::ResponseTemplate {
                let count = self.counter.fetch_add(1, Ordering::SeqCst) + 1;
                if count < 3 {
                    ResponseTemplate::new(500).set_body_json(serde_json::json!({
                        "error": "Internal server error"
                    }))
                } else {
                    ResponseTemplate::new(200).set_body_json(serde_json::json!({
                        "choices": [{
                            "message": {
                                "role": "assistant",
                                "content": "Success after retries"
                            }
                        }],
                        "usage": {
                            "prompt_tokens": 10,
                            "completion_tokens": 20,
                            "total_tokens": 30
                        }
                    }))
                }
            }
        }

        // Install crypto provider for reqwest
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

        let mock_server = MockServer::start().await;

        // Create a custom responder that tracks calls
        // Mount the responder - it will be moved
        let responder = RetryResponder {
            counter: AtomicUsize::new(0),
        };

        Mock::given(method("POST"))
            .and(path("/chat/completions"))
            .respond_with(responder)
            .mount(&mock_server)
            .await;

        let config = OrchestratorConfig {
            base_url: mock_server.uri(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            ..Default::default()
        };

        let client = create_llm_client(&config).unwrap();

        let messages = vec![LLMMessage {
            role: "user".to_string(),
            content: "Test".to_string(),
        }];

        let result = client.chat(messages).await;

        // Test passes if retry logic is implemented (succeeds after retries)
        // Test fails if no retry logic (fails on first 500 error)
        assert!(result.is_ok());
        assert_eq!(result.unwrap().content, "Success after retries");
    }

    // =========================================================================
    // Test Suite 9: Git Event-Driven Integration (Phase 21)
    // =========================================================================

    #[tokio::test]
    async fn test_publish_git_event() {
        let bus = MessageBus::new(100);
        let mut sub = bus.subscribe("workflow:wf-1").await;

        // Publish git event
        bus.publish_git_event(
            "wf-1",
            "abc123def456",
            "feature/test",
            "feat: add new feature",
        )
        .await;

        // Verify message received on topic
        let msg = tokio::time::timeout(std::time::Duration::from_millis(100), sub.recv()).await;

        assert!(msg.is_ok(), "Should receive git event within timeout");
        let msg = msg.unwrap().unwrap();

        match msg {
            BusMessage::GitEvent {
                workflow_id,
                commit_hash,
                branch,
                message,
            } => {
                assert_eq!(workflow_id, "wf-1");
                assert_eq!(commit_hash, "abc123def456");
                assert_eq!(branch, "feature/test");
                assert_eq!(message, "feat: add new feature");
            }
            _ => panic!("Expected GitEvent, got {:?}", msg),
        }
    }

    #[tokio::test]
    async fn test_git_event_broadcast() {
        let bus = MessageBus::new(100);
        let mut broadcast_sub = bus.subscribe_broadcast();

        // Publish git event
        bus.publish_git_event("wf-1", "abc123", "main", "fix: bug fix")
            .await;

        // Verify broadcast received
        let msg =
            tokio::time::timeout(std::time::Duration::from_millis(100), broadcast_sub.recv()).await;

        assert!(msg.is_ok(), "Should receive broadcast within timeout");
        let msg = msg.unwrap().unwrap();

        match msg {
            BusMessage::GitEvent { workflow_id, .. } => {
                assert_eq!(workflow_id, "wf-1");
            }
            _ => panic!("Expected GitEvent broadcast"),
        }
    }

    #[tokio::test]
    async fn test_commit_idempotency() {
        let (db, workflow, terminal) = setup_test_workflow().await;

        // Ensure terminal is already quiet long enough to satisfy completion gate.
        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config.clone(),
            workflow.id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        // Create valid commit message
        let commit_message = format!(
            r#"Terminal completed

---METADATA---
workflow_id: {}
task_id: {}
terminal_id: {}
status: completed
next_action: handoff"#,
            workflow.id, terminal.workflow_task_id, terminal.id
        );

        let commit_hash = "unique_commit_hash_123";

        // First call should process the commit
        let result1 = agent
            .handle_git_event(&workflow.id, commit_hash, "main", &commit_message)
            .await;
        assert!(result1.is_ok(), "First call should succeed");

        // Second call with same commit hash should be idempotent (no error, but no processing)
        let result2 = agent
            .handle_git_event(&workflow.id, commit_hash, "main", &commit_message)
            .await;
        assert!(result2.is_ok(), "Second call should succeed (idempotent)");

        // Verify terminal status is still completed (not double-processed)
        let updated_terminal = db::models::Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(updated_terminal.status, "completed");
    }

    #[tokio::test]
    async fn test_handle_git_event_no_metadata_infers_task_and_advances_handoff() {
        use db::models::Terminal;

        let (db, workflow_id, task_id, terminals) = setup_workflow_with_terminals(2, true).await;
        let (first_terminal_id, _) = terminals[0].clone();
        let (second_terminal_id, second_pty_session_id) = terminals[1].clone();
        let second_pty_session_id = second_pty_session_id.expect("Second terminal should have PTY");

        // Mark first terminal as actively working and old enough to pass quiet-window checks.
        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&first_terminal_id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow_id.clone(),
            message_bus.clone(),
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let mut second_terminal_rx = message_bus.subscribe(&second_pty_session_id).await;
        let task_prefix = task_id
            .split('-')
            .next()
            .expect("Task ID should have prefix");
        let commit_message = format!("chore: no-op advance for task {task_prefix}");

        let result = agent
            .handle_git_event(
                &workflow_id,
                "no_metadata_commit_456",
                "main",
                &commit_message,
            )
            .await;
        assert!(result.is_ok(), "No-metadata commit should be handled");

        let first_terminal = Terminal::find_by_id(&db.pool, &first_terminal_id)
            .await
            .unwrap()
            .unwrap();
        let second_terminal = Terminal::find_by_id(&db.pool, &second_terminal_id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(first_terminal.status, "completed");
        assert_eq!(second_terminal.status, "working");

        let dispatched = tokio::time::timeout(
            tokio::time::Duration::from_millis(500),
            second_terminal_rx.recv(),
        )
        .await;
        assert!(
            dispatched.is_ok(),
            "Second terminal should receive dispatch message after inferred handoff"
        );

        let git_events = db::models::git_event::GitEvent::find_by_workflow(&db.pool, &workflow_id)
            .await
            .unwrap();
        let event = git_events
            .iter()
            .find(|event| event.commit_hash == "no_metadata_commit_456")
            .expect("Git event should be persisted");
        assert_eq!(event.process_status, "processed");
    }

    #[tokio::test]
    async fn test_handle_git_event_no_metadata_no_hint_commits_do_not_stall_parallel_tasks() {
        use db::models::{Terminal, WorkflowTask};
        use uuid::Uuid;

        let (db, workflow, first_terminal) = setup_test_workflow().await;

        let second_task_id = Uuid::new_v4().to_string();
        let second_terminal_id = Uuid::new_v4().to_string();
        let second_pty_session_id = Uuid::new_v4().to_string();
        let started_at = chrono::Utc::now() - chrono::Duration::seconds(90);

        sqlx::query(
            r#"
            INSERT INTO workflow_task (
                id, workflow_id, name, branch, order_index,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)
            "#,
        )
        .bind(&second_task_id)
        .bind(&workflow.id)
        .bind("parallel-task")
        .bind("feature/parallel")
        .bind(1)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            INSERT INTO terminal (
                id, workflow_task_id, cli_type_id, model_config_id,
                order_index, status, pty_session_id, started_at, created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)
            "#,
        )
        .bind(&second_terminal_id)
        .bind(&second_task_id)
        .bind("cli-claude-code")
        .bind("model-claude-sonnet")
        .bind(0)
        .bind("working")
        .bind(&second_pty_session_id)
        .bind(started_at)
        .bind(chrono::Utc::now())
        .bind(chrono::Utc::now())
        .execute(&db.pool)
        .await
        .unwrap();

        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(started_at)
        .bind(&first_terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus,
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let commit_message = "chore: advance orchestrator (no changes needed)";
        let first_result = agent
            .handle_git_event(
                &workflow.id,
                "no_metadata_parallel_commit_1",
                "main",
                commit_message,
            )
            .await;
        assert!(
            first_result.is_ok(),
            "First no-metadata commit should be handled"
        );

        let second_result = agent
            .handle_git_event(
                &workflow.id,
                "no_metadata_parallel_commit_2",
                "main",
                commit_message,
            )
            .await;
        assert!(
            second_result.is_ok(),
            "Second no-metadata commit should be handled"
        );

        let first_terminal_after = Terminal::find_by_id(&db.pool, &first_terminal.id)
            .await
            .unwrap()
            .unwrap();
        let second_terminal_after = Terminal::find_by_id(&db.pool, &second_terminal_id)
            .await
            .unwrap()
            .unwrap();

        assert_eq!(first_terminal_after.status, "completed");
        assert_eq!(second_terminal_after.status, "completed");

        let first_task_after = WorkflowTask::find_by_id(&db.pool, &first_terminal.workflow_task_id)
            .await
            .unwrap()
            .unwrap();
        let second_task_after = WorkflowTask::find_by_id(&db.pool, &second_task_id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(first_task_after.status, "completed");
        assert_eq!(second_task_after.status, "completed");

        let git_events = db::models::git_event::GitEvent::find_by_workflow(&db.pool, &workflow.id)
            .await
            .unwrap();
        let first_event = git_events
            .iter()
            .find(|event| event.commit_hash == "no_metadata_parallel_commit_1")
            .expect("First git event should be persisted");
        let second_event = git_events
            .iter()
            .find(|event| event.commit_hash == "no_metadata_parallel_commit_2")
            .expect("Second git event should be persisted");
        assert_eq!(first_event.process_status, "processed");
        assert_eq!(second_event.process_status, "processed");
    }

    #[tokio::test]
    async fn test_handle_git_event_no_metadata_marks_failed_when_task_cannot_be_inferred() {
        let (db, workflow, terminal) = setup_test_workflow().await;

        sqlx::query(
            r#"
            UPDATE terminal
            SET status = 'working', started_at = ?1, updated_at = ?1
            WHERE id = ?2
            "#,
        )
        .bind(chrono::Utc::now() - chrono::Duration::seconds(90))
        .bind(&terminal.id)
        .execute(&db.pool)
        .await
        .unwrap();

        let config = OrchestratorConfig {
            api_type: "openai".to_string(),
            base_url: "https://api.openai.com/v1".to_string(),
            api_key: "sk-test".to_string(),
            model: "gpt-4".to_string(),
            max_retries: 3,
            timeout_secs: 120,
            retry_delay_ms: 1000,
            rate_limit_requests_per_second: DEFAULT_LLM_RATE_LIMIT_PER_SECOND,
            max_conversation_history: 50,
            system_prompt: String::new(),
        };

        let message_bus = Arc::new(MessageBus::new(100));
        let mock_llm = Box::new(MockLLMClient {
            should_fail: false,
            response_content: String::new(),
        });

        let agent = OrchestratorAgent::with_llm_client(
            config,
            workflow.id.clone(),
            message_bus,
            db.clone(),
            mock_llm,
        )
        .unwrap();

        let commit_message = "chore: no-op advance for task deadbeef";
        let result = agent
            .handle_git_event(
                &workflow.id,
                "no_metadata_commit_789",
                "main",
                commit_message,
            )
            .await;
        assert!(
            result.is_ok(),
            "No-metadata inference failure should not crash agent"
        );

        let refreshed_terminal = db::models::Terminal::find_by_id(&db.pool, &terminal.id)
            .await
            .unwrap()
            .unwrap();
        assert_eq!(refreshed_terminal.status, "working");

        let git_events = db::models::git_event::GitEvent::find_by_workflow(&db.pool, &workflow.id)
            .await
            .unwrap();
        let event = git_events
            .iter()
            .find(|event| event.commit_hash == "no_metadata_commit_789")
            .expect("Git event should be persisted");
        assert_eq!(event.process_status, "failed");
    }

    #[tokio::test]
    async fn test_processed_commits_tracking() {
        let mut state = OrchestratorState::new("workflow-1".to_string());

        // Initially empty
        assert!(state.processed_commits.is_empty());

        // Add a commit
        state.processed_commits.insert("commit_hash_1".to_string());
        assert!(state.processed_commits.contains("commit_hash_1"));
        assert!(!state.processed_commits.contains("commit_hash_2"));

        // Add another commit
        state.processed_commits.insert("commit_hash_2".to_string());
        assert_eq!(state.processed_commits.len(), 2);

        // Duplicate insert should not increase count
        state.processed_commits.insert("commit_hash_1".to_string());
        assert_eq!(state.processed_commits.len(), 2);
    }

    #[tokio::test]
    async fn test_git_event_topic_isolation() {
        let bus = MessageBus::new(100);

        let mut sub_wf1 = bus.subscribe("workflow:wf-1").await;
        let mut sub_wf2 = bus.subscribe("workflow:wf-2").await;

        // Publish to wf-1 only
        bus.publish_git_event("wf-1", "abc123", "main", "commit message")
            .await;

        // wf-1 should receive
        let msg = tokio::time::timeout(std::time::Duration::from_millis(100), sub_wf1.recv()).await;
        assert!(msg.is_ok(), "wf-1 should receive git event");

        // wf-2 should NOT receive (timeout)
        let msg = tokio::time::timeout(std::time::Duration::from_millis(100), sub_wf2.recv()).await;
        assert!(msg.is_err(), "wf-2 should not receive wf-1's git event");
    }
}
