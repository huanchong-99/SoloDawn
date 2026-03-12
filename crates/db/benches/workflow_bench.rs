//! Workflow Performance Benchmarks
//!
//! Run with: cargo bench --bench workflow_bench

use chrono::Utc;
use criterion::{BenchmarkId, Criterion, black_box, criterion_group, criterion_main};
use db::models::workflow::{Workflow, WorkflowStatus};
use sqlx::{Row, SqlitePool};
use uuid::Uuid;

/// Create a deterministic UUID from a project name string (for benchmarking)
fn project_uuid(name: &str) -> Uuid {
    // Pad or truncate name bytes to fill 16 bytes for a deterministic UUID
    let mut bytes = [0u8; 16];
    for (i, b) in name.as_bytes().iter().enumerate().take(16) {
        bytes[i] = *b;
    }
    Uuid::from_bytes(bytes)
}

/// Create a test workflow for benchmarking
fn create_test_workflow(project_id: &str) -> Workflow {
    Workflow {
        id: Uuid::new_v4().to_string(),
        project_id: project_uuid(project_id),
        name: format!("Benchmark Workflow {}", Uuid::new_v4()),
        description: Some("A test workflow for performance benchmarking".to_string()),
        status: WorkflowStatus::Created.to_string(),
        execution_mode: "diy".to_string(),
        initial_goal: None,
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
        ready_at: None,
        started_at: None,
        completed_at: None,
        created_at: Utc::now(),
        updated_at: Utc::now(),
    }
}

/// Benchmark: Find workflow by ID
fn bench_find_by_id(c: &mut Criterion) {
    let rt = tokio::runtime::Runtime::new().unwrap();

    // Setup: Create in-memory database and test data
    let pool = rt.block_on(async {
        let pool = SqlitePool::connect("sqlite::memory:").await.unwrap();

        // Create workflow table
        sqlx::query(
            r#"
            CREATE TABLE workflow (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                name TEXT NOT NULL,
                description TEXT,
                status TEXT NOT NULL DEFAULT 'created',
                execution_mode TEXT NOT NULL DEFAULT 'diy',
                initial_goal TEXT,
                use_slash_commands INTEGER NOT NULL DEFAULT 0,
                orchestrator_enabled INTEGER NOT NULL DEFAULT 1,
                orchestrator_api_type TEXT,
                orchestrator_base_url TEXT,
                orchestrator_api_key TEXT,
                orchestrator_model TEXT,
                error_terminal_enabled INTEGER NOT NULL DEFAULT 0,
                error_terminal_cli_id TEXT,
                error_terminal_model_id TEXT,
                merge_terminal_cli_id TEXT NOT NULL,
                merge_terminal_model_id TEXT NOT NULL,
                target_branch TEXT NOT NULL DEFAULT 'main',
                git_watcher_enabled INTEGER NOT NULL DEFAULT 1,
                ready_at TEXT,
                started_at TEXT,
                completed_at TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            )
            "#
        )
        .execute(&pool)
        .await
        .unwrap();

        // Create test workflow
        let workflow = create_test_workflow("test-project");
        sqlx::query(
            r#"
            INSERT INTO workflow (
                id, project_id, name, description, status,
                execution_mode, initial_goal,
                use_slash_commands, orchestrator_enabled,
                orchestrator_api_type, orchestrator_base_url,
                orchestrator_api_key, orchestrator_model,
                error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
                merge_terminal_cli_id, merge_terminal_model_id,
                target_branch, ready_at, started_at, completed_at,
                created_at, updated_at
            ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, ?24)
            "#
        )
        .bind(&workflow.id)
        .bind(workflow.project_id)
        .bind(&workflow.name)
        .bind(&workflow.description)
        .bind(&workflow.status)
        .bind(&workflow.execution_mode)
        .bind(&workflow.initial_goal)
        .bind(workflow.use_slash_commands)
        .bind(workflow.orchestrator_enabled)
        .bind(&workflow.orchestrator_api_type)
        .bind(&workflow.orchestrator_base_url)
        .bind(&workflow.orchestrator_api_key)
        .bind(&workflow.orchestrator_model)
        .bind(workflow.error_terminal_enabled)
        .bind(&workflow.error_terminal_cli_id)
        .bind(&workflow.error_terminal_model_id)
        .bind(&workflow.merge_terminal_cli_id)
        .bind(&workflow.merge_terminal_model_id)
        .bind(&workflow.target_branch)
        .bind(workflow.ready_at)
        .bind(workflow.started_at)
        .bind(workflow.completed_at)
        .bind(workflow.created_at)
        .bind(workflow.updated_at)
        .execute(&pool)
        .await
        .unwrap();

        pool
    });

    let workflow_id: String = rt.block_on(async {
        // Get the workflow ID we just created
        let row = sqlx::query("SELECT id FROM workflow LIMIT 1")
            .fetch_one(&pool)
            .await
            .unwrap();
        row.get("id")
    });

    c.bench_function("find_by_id", |b| {
        b.iter(|| {
            let pool = pool.clone();
            let id = workflow_id.clone();
            rt.block_on(async move {
                let result = Workflow::find_by_id(&pool, black_box(&id)).await;
                let _ = black_box(result);
            });
        })
    });
}

/// Benchmark: Find workflows by project with varying dataset sizes
fn bench_find_by_project(c: &mut Criterion) {
    let rt = tokio::runtime::Runtime::new().unwrap();

    let mut group = c.benchmark_group("find_by_project");

    for size in [10, 50, 100, 500].iter() {
        let pool = rt.block_on(async {
            let pool = SqlitePool::connect("sqlite::memory:").await.unwrap();

            // Create workflow table with index
            sqlx::query(
                r#"
                CREATE TABLE workflow (
                    id TEXT PRIMARY KEY,
                    project_id TEXT NOT NULL,
                    name TEXT NOT NULL,
                    description TEXT,
                    status TEXT NOT NULL DEFAULT 'created',
                    use_slash_commands INTEGER NOT NULL DEFAULT 0,
                    orchestrator_enabled INTEGER NOT NULL DEFAULT 1,
                    orchestrator_api_type TEXT,
                    orchestrator_base_url TEXT,
                    orchestrator_api_key TEXT,
                    orchestrator_model TEXT,
                    error_terminal_enabled INTEGER NOT NULL DEFAULT 0,
                    error_terminal_cli_id TEXT,
                    error_terminal_model_id TEXT,
                    merge_terminal_cli_id TEXT NOT NULL,
                    merge_terminal_model_id TEXT NOT NULL,
                    target_branch TEXT NOT NULL DEFAULT 'main',
                    ready_at TEXT,
                    started_at TEXT,
                    completed_at TEXT,
                    created_at TEXT NOT NULL,
                    updated_at TEXT NOT NULL
                )
                "#
            )
            .execute(&pool)
            .await
            .unwrap();

            // Create composite index (like in production)
            sqlx::query(
                "CREATE INDEX idx_workflow_project_status ON workflow(project_id, status) WHERE status IN ('created', 'ready', 'running')"
            )
            .execute(&pool)
            .await
            .unwrap();

            // Insert test data
            for i in 0..*size {
                let workflow = create_test_workflow(&format!("project-{}", i % 5)); // Distribute across 5 projects
                sqlx::query(
                    r#"
                    INSERT INTO workflow (
                        id, project_id, name, description, status,
                        execution_mode, initial_goal,
                        use_slash_commands, orchestrator_enabled,
                        orchestrator_api_type, orchestrator_base_url,
                        orchestrator_api_key, orchestrator_model,
                        error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
                        merge_terminal_cli_id, merge_terminal_model_id,
                        target_branch, ready_at, started_at, completed_at,
                        created_at, updated_at
                    ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, ?24)
                    "#
                )
                .bind(&workflow.id)
                .bind(workflow.project_id)
                .bind(&workflow.name)
                .bind(&workflow.description)
                .bind(&workflow.status)
                .bind(&workflow.execution_mode)
                .bind(&workflow.initial_goal)
                .bind(workflow.use_slash_commands)
                .bind(workflow.orchestrator_enabled)
                .bind(&workflow.orchestrator_api_type)
                .bind(&workflow.orchestrator_base_url)
                .bind(&workflow.orchestrator_api_key)
                .bind(&workflow.orchestrator_model)
                .bind(workflow.error_terminal_enabled)
                .bind(&workflow.error_terminal_cli_id)
                .bind(&workflow.error_terminal_model_id)
                .bind(&workflow.merge_terminal_cli_id)
                .bind(&workflow.merge_terminal_model_id)
                .bind(&workflow.target_branch)
                .bind(workflow.ready_at)
                .bind(workflow.started_at)
                .bind(workflow.completed_at)
                .bind(workflow.created_at)
                .bind(workflow.updated_at)
                .execute(&pool)
                .await
                .unwrap();
            }

            pool
        });

        group.bench_with_input(BenchmarkId::from_parameter(size), size, |b, _| {
            b.iter(|| {
                let pool = pool.clone();
                rt.block_on(async move {
                    let result = Workflow::find_by_project(&pool, black_box(project_uuid("project-0"))).await;
                    let _ = black_box(result);
                });
            });
        });
    }

    group.finish();
}

/// Benchmark: Find workflows by project with status filter
fn bench_find_by_project_with_status(c: &mut Criterion) {
    let rt = tokio::runtime::Runtime::new().unwrap();

    // Setup: Create in-memory database with test data
    let pool = rt.block_on(async {
        let pool = SqlitePool::connect("sqlite::memory:").await.unwrap();

        // Create workflow table
        sqlx::query(
            r#"
            CREATE TABLE workflow (
                id TEXT PRIMARY KEY,
                project_id TEXT NOT NULL,
                name TEXT NOT NULL,
                description TEXT,
                status TEXT NOT NULL DEFAULT 'created',
                execution_mode TEXT NOT NULL DEFAULT 'diy',
                initial_goal TEXT,
                use_slash_commands INTEGER NOT NULL DEFAULT 0,
                orchestrator_enabled INTEGER NOT NULL DEFAULT 1,
                orchestrator_api_type TEXT,
                orchestrator_base_url TEXT,
                orchestrator_api_key TEXT,
                orchestrator_model TEXT,
                error_terminal_enabled INTEGER NOT NULL DEFAULT 0,
                error_terminal_cli_id TEXT,
                error_terminal_model_id TEXT,
                merge_terminal_cli_id TEXT NOT NULL,
                merge_terminal_model_id TEXT NOT NULL,
                target_branch TEXT NOT NULL DEFAULT 'main',
                git_watcher_enabled INTEGER NOT NULL DEFAULT 1,
                ready_at TEXT,
                started_at TEXT,
                completed_at TEXT,
                created_at TEXT NOT NULL,
                updated_at TEXT NOT NULL
            )
            "#
        )
        .execute(&pool)
        .await
        .unwrap();

        // Create partial index (like in production)
        sqlx::query(
            "CREATE INDEX idx_workflow_project_status ON workflow(project_id, status) WHERE status IN ('created', 'ready', 'running')"
        )
        .execute(&pool)
        .await
        .unwrap();

        // Insert test data with various statuses
        let statuses = ["created", "ready", "running", "completed", "failed", "cancelled"];
        for i in 0..100 {
            let mut workflow = create_test_workflow("test-project");
            workflow.status = statuses[i % statuses.len()].to_string();

            sqlx::query(
                r#"
                INSERT INTO workflow (
                    id, project_id, name, description, status,
                    execution_mode, initial_goal,
                    use_slash_commands, orchestrator_enabled,
                    orchestrator_api_type, orchestrator_base_url,
                    orchestrator_api_key, orchestrator_model,
                    error_terminal_enabled, error_terminal_cli_id, error_terminal_model_id,
                    merge_terminal_cli_id, merge_terminal_model_id,
                    target_branch, ready_at, started_at, completed_at,
                    created_at, updated_at
                ) VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20, ?21, ?22, ?23, ?24)
                "#
            )
            .bind(&workflow.id)
            .bind(workflow.project_id)
            .bind(&workflow.name)
            .bind(&workflow.description)
            .bind(&workflow.status)
            .bind(&workflow.execution_mode)
            .bind(&workflow.initial_goal)
            .bind(workflow.use_slash_commands)
            .bind(workflow.orchestrator_enabled)
            .bind(&workflow.orchestrator_api_type)
            .bind(&workflow.orchestrator_base_url)
            .bind(&workflow.orchestrator_api_key)
            .bind(&workflow.orchestrator_model)
            .bind(workflow.error_terminal_enabled)
            .bind(&workflow.error_terminal_cli_id)
            .bind(&workflow.error_terminal_model_id)
            .bind(&workflow.merge_terminal_cli_id)
            .bind(&workflow.merge_terminal_model_id)
            .bind(&workflow.target_branch)
            .bind(workflow.ready_at)
            .bind(workflow.started_at)
            .bind(workflow.completed_at)
            .bind(workflow.created_at)
            .bind(workflow.updated_at)
            .execute(&pool)
            .await
            .unwrap();
        }

        pool
    });

    c.bench_function("find_by_project_with_status", |b| {
        b.iter(|| {
            let pool = pool.clone();
            rt.block_on(async move {
                // Query with status filter (uses partial index)
                let result = sqlx::query_as::<_, Workflow>(
                    r"
                    SELECT * FROM workflow
                    WHERE project_id = ? AND status IN ('created', 'ready', 'running')
                    ORDER BY created_at DESC
                    ",
                )
                .bind(black_box(project_uuid("test-project")))
                .fetch_all(&pool)
                .await;
                let _ = black_box(result);
            });
        })
    });
}

criterion_group!(
    benches,
    bench_find_by_id,
    bench_find_by_project,
    bench_find_by_project_with_status
);
criterion_main!(benches);
