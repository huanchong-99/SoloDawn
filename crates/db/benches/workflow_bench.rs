//! Workflow Performance Benchmarks
//!
//! Run with: cargo bench --bench workflow_bench
//
// NOTE: Known bench limitations (W2-05-01..08) — these are caveats of the
// harness itself, not production defects. Benchmarks are not production code;
// the following issues are intentionally left in place but should be
// considered when interpreting results:
//
// - W2-05-01: FIXED. A single `tokio::runtime::Runtime` is shared across all
//   `bench_*` functions via a `OnceLock` (see `shared_runtime`). Don't copy
//   the old per-function pattern into production code.
// - W2-05-02: FIXED (partially). `bench_find_by_id` now runs against both an
//   in-memory SQLite pool (fast baseline) and a file-backed pool via
//   `tempfile::NamedTempFile` so disk I/O, WAL, and page-cache effects are
//   actually exercised. Other benches still use in-memory only.
// - W2-05-03: FIXED. `bench_find_by_project` now cycles through multiple
//   project ids per iteration so index selectivity and plan-cache variance
//   are no longer hidden by a fixed hot key.
// - W2-05-04: Schemas inlined here are a subset of the real migrations
//   (missing columns, constraints, and some indexes) — drift between this
//   file and `crates/db/migrations/` is silent and will not fail CI.
// - W2-05-05: TODO — Data distribution is uniform (`i % 5`,
//   `i % statuses.len()`) and small (<=500 rows); production distributions
//   are skewed and much larger. Replace the uniform generator with a
//   zipfian/long-tail distribution and scale row counts into the 10k+ range
//   before using these numbers to set SLOs.
// - W2-05-06: TODO — `find_by_project_with_status` hardcodes the `IN (...)`
//   list that matches the partial index predicate. Add bench variants that
//   filter by non-indexed statuses (`completed`, `failed`, `cancelled`) to
//   measure full-scan fallback and EXPLAIN QUERY PLAN regressions.
// - W2-05-07: TODO — Workflow responses re-parse JSON payloads (agent
//   metadata, orchestrator config) on every row decode. Add a bench that
//   measures `serde_json::from_str` cost separately from row fetch so that
//   row-decoding regressions can be distinguished from query-planner ones.
// - W2-05-08: TODO — Paginated endpoints use `LIMIT/OFFSET` which is O(n)
//   in offset. Add a bench that walks deep offsets (e.g. offset 0, 500,
//   5000, 50000) so keyset-pagination migrations can be validated against
//   a concrete baseline.

use std::sync::OnceLock;

use chrono::Utc;
use criterion::{BenchmarkId, Criterion, black_box, criterion_group, criterion_main};
use db::models::workflow::{Workflow, WorkflowStatus};
use sqlx::{Row, SqlitePool};
use uuid::Uuid;

/// Shared Tokio runtime reused across all benchmarks (W2-05-01). Building a
/// new runtime per bench function is expensive and leaks worker threads for
/// the lifetime of the process.
fn shared_runtime() -> &'static tokio::runtime::Runtime {
    static RT: OnceLock<tokio::runtime::Runtime> = OnceLock::new();
    RT.get_or_init(|| tokio::runtime::Runtime::new().unwrap())
}

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
        pause_reason: None,
    }
}

/// Build a workflow table + one row, returning (pool, workflow_id).
async fn setup_single_workflow_pool(url: &str) -> (SqlitePool, String) {
    let pool = SqlitePool::connect(url).await.unwrap();

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
            updated_at TEXT NOT NULL,
            pause_reason TEXT
        )
        "#,
    )
    .execute(&pool)
    .await
    .unwrap();

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
        "#,
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

    let row = sqlx::query("SELECT id FROM workflow LIMIT 1")
        .fetch_one(&pool)
        .await
        .unwrap();
    let id: String = row.get("id");
    (pool, id)
}

/// Benchmark: Find workflow by ID
fn bench_find_by_id(c: &mut Criterion) {
    let rt = shared_runtime();

    // In-memory variant — fast baseline.
    let (mem_pool, mem_id) = rt.block_on(setup_single_workflow_pool("sqlite::memory:"));

    c.bench_function("find_by_id_memory", |b| {
        b.iter(|| {
            let pool = mem_pool.clone();
            let id = mem_id.clone();
            rt.block_on(async move {
                let result = Workflow::find_by_id(&pool, black_box(&id)).await;
                let _ = black_box(result);
            });
        })
    });

    // File-backed variant (W2-05-02) — exercises real disk I/O and WAL.
    let tmp = tempfile::NamedTempFile::new().expect("create tempfile");
    let file_url = format!("sqlite://{}?mode=rwc", tmp.path().display());
    let (file_pool, file_id) = rt.block_on(setup_single_workflow_pool(&file_url));

    c.bench_function("find_by_id_file", |b| {
        b.iter(|| {
            let pool = file_pool.clone();
            let id = file_id.clone();
            rt.block_on(async move {
                let result = Workflow::find_by_id(&pool, black_box(&id)).await;
                let _ = black_box(result);
            });
        })
    });

    // Keep `tmp` alive until the bench finishes.
    drop(tmp);
}

#[allow(dead_code)]
fn _unused_keep_old_find_by_id_setup(c: &mut Criterion) {
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
                updated_at TEXT NOT NULL,
                pause_reason TEXT
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
    let rt = shared_runtime();

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
            // TODO (W2-05-05): data distribution is uniform (`i % 5`) with only
            // ~500 rows max. Replace with zipfian/long-tail distribution and
            // scale into the 10k+ range before using this for SLOs.
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

        // W2-05-03: cycle through all 5 seeded projects instead of pinning
        // `project-0`, so index selectivity and plan-cache variance are not
        // hidden by a single hot key.
        group.bench_with_input(BenchmarkId::from_parameter(size), size, |b, _| {
            let mut iter_idx: usize = 0;
            b.iter(|| {
                let pool = pool.clone();
                let project = project_uuid(&format!("project-{}", iter_idx % 5));
                iter_idx = iter_idx.wrapping_add(1);
                rt.block_on(async move {
                    let result = Workflow::find_by_project(&pool, black_box(project)).await;
                    let _ = black_box(result);
                });
            });
        });
    }

    group.finish();
}

/// Benchmark: Find workflows by project with status filter
fn bench_find_by_project_with_status(c: &mut Criterion) {
    let rt = shared_runtime();

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
                updated_at TEXT NOT NULL,
                pause_reason TEXT
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

    // TODO (W2-05-06): this query hardcodes the `IN (...)` list that matches
    // the partial index predicate. Add variants that filter by non-indexed
    // statuses (`completed`, `failed`, `cancelled`) to measure full-scan
    // fallback and catch EXPLAIN QUERY PLAN regressions.
    // TODO (W2-05-07): Workflow rows re-parse JSON payloads (agent metadata,
    // orchestrator config) on every decode. Add a bench that isolates
    // `serde_json::from_str` cost from row fetch so row-decode regressions
    // can be distinguished from query-planner ones.
    // TODO (W2-05-08): Paginated endpoints use `LIMIT/OFFSET`, which is O(n)
    // in offset. Add a bench that walks deep offsets (0, 500, 5000, 50000)
    // to baseline a future keyset-pagination migration.
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
