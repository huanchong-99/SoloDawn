//! Database Performance Tests
//!
//! Tests for database query performance and throughput.
//!
//! These tests measure:
//! - Workflow list query performance
//! - Workflow detail query performance
//! - Concurrent write performance
//! - Index effectiveness
//!
//! IMPORTANT: Set SOLODAWN_TEST_DATABASE_URL to a test database before running.
//! Run with: cargo test --test performance_database -- --nocapture

use std::sync::atomic::{AtomicUsize, Ordering};
use std::sync::Arc;
use std::time::{Duration, Instant};
use sqlx::sqlite::SqlitePoolOptions;
use sqlx::SqlitePool;
use uuid::Uuid;

/// Query performance statistics
#[derive(Debug, Default)]
struct QueryStats {
    queries_executed: AtomicUsize,
    latencies_us: parking_lot::Mutex<Vec<u64>>,
    errors: AtomicUsize,
}

impl QueryStats {
    fn new() -> Self {
        Self::default()
    }

    fn record_query(&self, latency_us: u64) {
        self.queries_executed.fetch_add(1, Ordering::SeqCst);
        self.latencies_us.lock().push(latency_us);
    }

    fn record_error(&self) {
        self.errors.fetch_add(1, Ordering::SeqCst);
    }

    fn qps(&self, duration: Duration) -> f64 {
        let queries = self.queries_executed.load(Ordering::SeqCst) as f64;
        queries / duration.as_secs_f64()
    }

    fn p95_latency_us(&self) -> Option<u64> {
        let mut latencies = self.latencies_us.lock().clone();
        if latencies.is_empty() {
            return None;
        }
        latencies.sort();
        let idx = (latencies.len() as f64 * 0.95) as usize;
        Some(latencies[idx.min(latencies.len() - 1)])
    }

    fn p99_latency_us(&self) -> Option<u64> {
        let mut latencies = self.latencies_us.lock().clone();
        if latencies.is_empty() {
            return None;
        }
        latencies.sort();
        let idx = (latencies.len() as f64 * 0.99) as usize;
        Some(latencies[idx.min(latencies.len() - 1)])
    }

    fn avg_latency_us(&self) -> Option<f64> {
        let latencies = self.latencies_us.lock();
        if latencies.is_empty() {
            return None;
        }
        Some(latencies.iter().sum::<u64>() as f64 / latencies.len() as f64)
    }

    fn min_latency_us(&self) -> Option<u64> {
        self.latencies_us.lock().iter().min().copied()
    }

    fn max_latency_us(&self) -> Option<u64> {
        self.latencies_us.lock().iter().max().copied()
    }
}

/// Create test database pool
/// Fixed: Use dedicated test database environment variable to prevent production data mutation
async fn create_test_pool() -> Result<SqlitePool, sqlx::Error> {
    let database_url = std::env::var("SOLODAWN_TEST_DATABASE_URL").map_err(|_| {
        sqlx::Error::Configuration(Box::new(std::io::Error::new(
            std::io::ErrorKind::Other,
            "SOLODAWN_TEST_DATABASE_URL must be set for performance tests. \
             Example: SOLODAWN_TEST_DATABASE_URL=sqlite:./test_data/perf_test.db",
        )))
    })?;

    SqlitePoolOptions::new()
        .max_connections(10)
        .acquire_timeout(Duration::from_secs(30))
        .connect(&database_url)
        .await
}

/// Seed test data for performance testing
async fn seed_test_data(pool: &SqlitePool, count: usize) -> Result<String, sqlx::Error> {
    // Fixed: Check cli_types BEFORE creating project to avoid orphan records on failure
    let cli_type: Option<(String,)> = sqlx::query_as("SELECT id FROM cli_types LIMIT 1")
        .fetch_optional(pool)
        .await?;

    let cli_type_id = cli_type
        .map(|c| c.0)
        .ok_or(sqlx::Error::RowNotFound)?;

    let project_id = Uuid::new_v4().to_string();

    // Create test project
    sqlx::query(
        r#"
        INSERT INTO projects (id, name, path, created_at, updated_at)
        VALUES (?, ?, ?, datetime('now'), datetime('now'))
        "#,
    )
    .bind(&project_id)
    .bind(format!("Perf Test Project {}", count))
    .bind("/tmp/perf-test")
    .execute(pool)
    .await?;

    // Create test workflows
    for i in 0..count {
        let workflow_id = Uuid::new_v4().to_string();

        sqlx::query(
            r#"
            INSERT INTO workflows (id, project_id, name, description, status, target_branch, created_at, updated_at)
            VALUES (?, ?, ?, ?, 'created', 'main', datetime('now'), datetime('now'))
            "#,
        )
        .bind(&workflow_id)
        .bind(&project_id)
        .bind(format!("Perf Test Workflow {}", i))
        .bind(format!("Performance test workflow number {}", i))
        .execute(pool)
        .await?;

        // Create tasks for each workflow
        for j in 0..3 {
            let task_id = Uuid::new_v4().to_string();

            sqlx::query(
                r#"
                INSERT INTO workflow_tasks (id, workflow_id, name, description, order_index, status, created_at, updated_at)
                VALUES (?, ?, ?, ?, ?, 'pending', datetime('now'), datetime('now'))
                "#,
            )
            .bind(&task_id)
            .bind(&workflow_id)
            .bind(format!("Task {}", j))
            .bind(format!("Task {} for workflow {}", j, i))
            .bind(j as i32)
            .execute(pool)
            .await?;

            // Create terminal for each task
            let terminal_id = Uuid::new_v4().to_string();

            sqlx::query(
                r#"
                INSERT INTO workflow_terminals (id, workflow_task_id, cli_type_id, order_index, status, created_at, updated_at)
                VALUES (?, ?, ?, 0, 'idle', datetime('now'), datetime('now'))
                "#,
            )
            .bind(&terminal_id)
            .bind(&task_id)
            .bind(&cli_type_id)
            .execute(pool)
            .await?;
        }
    }

    Ok(project_id)
}

/// Clean up test data
async fn cleanup_test_data(pool: &SqlitePool, project_id: &str) -> Result<(), sqlx::Error> {
    // Delete in reverse order of foreign key dependencies
    sqlx::query(
        r#"
        DELETE FROM workflow_terminals WHERE workflow_task_id IN (
            SELECT wt.id FROM workflow_tasks wt
            JOIN workflows w ON wt.workflow_id = w.id
            WHERE w.project_id = ?
        )
        "#,
    )
    .bind(project_id)
    .execute(pool)
    .await?;

    sqlx::query(
        r#"
        DELETE FROM workflow_tasks WHERE workflow_id IN (
            SELECT id FROM workflows WHERE project_id = ?
        )
        "#,
    )
    .bind(project_id)
    .execute(pool)
    .await?;

    sqlx::query("DELETE FROM workflows WHERE project_id = ?")
        .bind(project_id)
        .execute(pool)
        .await?;

    sqlx::query("DELETE FROM projects WHERE id = ?")
        .bind(project_id)
        .execute(pool)
        .await?;

    Ok(())
}

#[tokio::test]
#[ignore = "requires test database"]
async fn test_db_workflow_list_performance() {
    let pool = match create_test_pool().await {
        Ok(p) => p,
        Err(e) => {
            eprintln!("Failed to connect to database: {}", e);
            return;
        }
    };

    // Seed test data
    let project_id = match seed_test_data(&pool, 100).await {
        Ok(id) => id,
        Err(e) => {
            eprintln!("Failed to seed test data: {}", e);
            return;
        }
    };

    let stats = QueryStats::new();
    let iterations = 100;

    println!("\n=== Workflow List Query Performance ===");

    // Fixed: Track total wall time for accurate QPS
    let total_start = Instant::now();

    for _ in 0..iterations {
        let start = Instant::now();

        let result: Result<Vec<(String, String, String)>, _> = sqlx::query_as(
            r#"
            SELECT w.id, w.name, w.status
            FROM workflows w
            WHERE w.project_id = ?
            ORDER BY w.created_at DESC
            LIMIT 50
            "#,
        )
        .bind(&project_id)
        .fetch_all(&pool)
        .await;

        let latency = start.elapsed().as_micros() as u64;

        match result {
            Ok(_) => stats.record_query(latency),
            Err(_) => stats.record_error(),
        }
    }

    let total_duration = total_start.elapsed();

    println!("Iterations: {}", iterations);
    println!("QPS: {:.2}", stats.qps(total_duration));
    println!("P95 latency: {:?} us ({:.2} ms)", stats.p95_latency_us(), stats.p95_latency_us().unwrap_or(0) as f64 / 1000.0);
    println!("P99 latency: {:?} us ({:.2} ms)", stats.p99_latency_us(), stats.p99_latency_us().unwrap_or(0) as f64 / 1000.0);
    println!("Avg latency: {:.2} us ({:.2} ms)", stats.avg_latency_us().unwrap_or(0.0), stats.avg_latency_us().unwrap_or(0.0) / 1000.0);
    println!("Min latency: {:?} us", stats.min_latency_us());
    println!("Max latency: {:?} us", stats.max_latency_us());
    println!("Errors: {}", stats.errors.load(Ordering::SeqCst));

    // P95 should be under 50ms
    if let Some(p95) = stats.p95_latency_us() {
        assert!(
            p95 <= 50_000,
            "P95 latency {} us exceeds 50ms threshold",
            p95
        );
    }

    // Cleanup
    let _ = cleanup_test_data(&pool, &project_id).await;
}

#[tokio::test]
#[ignore = "requires test database"]
async fn test_db_workflow_detail_performance() {
    let pool = match create_test_pool().await {
        Ok(p) => p,
        Err(e) => {
            eprintln!("Failed to connect to database: {}", e);
            return;
        }
    };

    // Seed test data
    let project_id = match seed_test_data(&pool, 10).await {
        Ok(id) => id,
        Err(e) => {
            eprintln!("Failed to seed test data: {}", e);
            return;
        }
    };

    // Get a workflow ID for testing
    let workflow: Option<(String,)> = sqlx::query_as(
        "SELECT id FROM workflows WHERE project_id = ? LIMIT 1",
    )
    .bind(&project_id)
    .fetch_optional(&pool)
    .await
    .unwrap_or(None);

    let workflow_id = match workflow {
        Some((id,)) => id,
        None => {
            eprintln!("No workflow found for testing");
            return;
        }
    };

    let stats = QueryStats::new();
    let iterations = 100;

    println!("\n=== Workflow Detail Query Performance ===");

    let total_start = Instant::now();

    for _ in 0..iterations {
        let start = Instant::now();

        // Simulate fetching workflow with tasks and terminals
        // Fixed: Track query results for error handling
        let workflow_result: Result<Option<(String, String, String, String)>, _> = sqlx::query_as(
            r#"
            SELECT w.id, w.name, w.description, w.status
            FROM workflows w
            WHERE w.id = ?
            "#,
        )
        .bind(&workflow_id)
        .fetch_optional(&pool)
        .await;

        let tasks_result: Result<Vec<(String, String, i32)>, _> = sqlx::query_as(
            r#"
            SELECT id, name, order_index
            FROM workflow_tasks
            WHERE workflow_id = ?
            ORDER BY order_index
            "#,
        )
        .bind(&workflow_id)
        .fetch_all(&pool)
        .await;

        let latency = start.elapsed().as_micros() as u64;

        // Fixed: Only record success if both queries succeeded
        if workflow_result.is_ok() && tasks_result.is_ok() {
            stats.record_query(latency);
        } else {
            stats.record_error();
        }
    }

    let total_duration = total_start.elapsed();

    println!("Iterations: {}", iterations);
    println!("QPS: {:.2}", stats.qps(total_duration));
    println!("P95 latency: {:?} us ({:.2} ms)", stats.p95_latency_us(), stats.p95_latency_us().unwrap_or(0) as f64 / 1000.0);
    println!("P99 latency: {:?} us ({:.2} ms)", stats.p99_latency_us(), stats.p99_latency_us().unwrap_or(0) as f64 / 1000.0);
    println!("Avg latency: {:.2} us ({:.2} ms)", stats.avg_latency_us().unwrap_or(0.0), stats.avg_latency_us().unwrap_or(0.0) / 1000.0);
    println!("Min latency: {:?} us", stats.min_latency_us());
    println!("Max latency: {:?} us", stats.max_latency_us());
    println!("Errors: {}", stats.errors.load(Ordering::SeqCst));

    // P95 should be under 30ms
    if let Some(p95) = stats.p95_latency_us() {
        assert!(
            p95 <= 30_000,
            "P95 latency {} us exceeds 30ms threshold",
            p95
        );
    }

    // Cleanup
    let _ = cleanup_test_data(&pool, &project_id).await;
}

#[tokio::test]
#[ignore = "requires test database"]
async fn test_db_concurrent_write_performance() {
    let pool = match create_test_pool().await {
        Ok(p) => p,
        Err(e) => {
            eprintln!("Failed to connect to database: {}", e);
            return;
        }
    };

    let stats = Arc::new(QueryStats::new());
    let concurrent_writers = 10;
    let writes_per_writer = 20;

    println!("\n=== Concurrent Write Performance ===");

    let mut handles = Vec::new();

    for writer_id in 0..concurrent_writers {
        let pool = pool.clone();
        let stats = Arc::clone(&stats);

        let handle = tokio::spawn(async move {
            let project_id = Uuid::new_v4().to_string();

            // Create project first
            let _ = sqlx::query(
                r#"
                INSERT INTO projects (id, name, path, created_at, updated_at)
                VALUES (?, ?, ?, datetime('now'), datetime('now'))
                "#,
            )
            .bind(&project_id)
            .bind(format!("Concurrent Test Project {}", writer_id))
            .bind("/tmp/concurrent-test")
            .execute(&pool)
            .await;

            for i in 0..writes_per_writer {
                let workflow_id = Uuid::new_v4().to_string();
                let start = Instant::now();

                let result = sqlx::query(
                    r#"
                    INSERT INTO workflows (id, project_id, name, description, status, target_branch, created_at, updated_at)
                    VALUES (?, ?, ?, ?, 'created', 'main', datetime('now'), datetime('now'))
                    "#,
                )
                .bind(&workflow_id)
                .bind(&project_id)
                .bind(format!("Concurrent Workflow {}-{}", writer_id, i))
                .bind("Concurrent write test")
                .execute(&pool)
                .await;

                let latency = start.elapsed().as_micros() as u64;

                match result {
                    Ok(_) => stats.record_query(latency),
                    Err(_) => stats.record_error(),
                }
            }

            // Cleanup
            let _ = sqlx::query("DELETE FROM workflows WHERE project_id = ?")
                .bind(&project_id)
                .execute(&pool)
                .await;
            let _ = sqlx::query("DELETE FROM projects WHERE id = ?")
                .bind(&project_id)
                .execute(&pool)
                .await;
        });

        handles.push(handle);
    }

    for handle in handles {
        let _ = handle.await;
    }

    let total_writes = concurrent_writers * writes_per_writer;

    println!("Concurrent writers: {}", concurrent_writers);
    println!("Writes per writer: {}", writes_per_writer);
    println!("Total writes: {}", total_writes);
    println!("Successful writes: {}", stats.queries_executed.load(Ordering::SeqCst));
    println!("P95 latency: {:?} us ({:.2} ms)", stats.p95_latency_us(), stats.p95_latency_us().unwrap_or(0) as f64 / 1000.0);
    println!("Avg latency: {:.2} us ({:.2} ms)", stats.avg_latency_us().unwrap_or(0.0), stats.avg_latency_us().unwrap_or(0.0) / 1000.0);
    println!("Errors (deadlocks): {}", stats.errors.load(Ordering::SeqCst));

    // No deadlocks should occur
    assert_eq!(
        stats.errors.load(Ordering::SeqCst),
        0,
        "Deadlocks detected during concurrent writes"
    );
}

#[tokio::test]
#[ignore = "requires test database"]
async fn test_db_index_effectiveness() {
    let pool = match create_test_pool().await {
        Ok(p) => p,
        Err(e) => {
            eprintln!("Failed to connect to database: {}", e);
            return;
        }
    };

    println!("\n=== Index Effectiveness Analysis ===");

    // Test queries that should use indexes
    let test_queries = vec![
        (
            "Workflow by project (idx_workflow_project_created)",
            "EXPLAIN QUERY PLAN SELECT * FROM workflows WHERE project_id = 'test' ORDER BY created_at DESC",
        ),
        (
            "Tasks by workflow",
            "EXPLAIN QUERY PLAN SELECT * FROM workflow_tasks WHERE workflow_id = 'test' ORDER BY order_index",
        ),
        (
            "Terminals by task",
            "EXPLAIN QUERY PLAN SELECT * FROM workflow_terminals WHERE workflow_task_id = 'test'",
        ),
    ];

    for (name, query) in test_queries {
        let result: Result<Vec<(i32, i32, i32, String)>, _> = sqlx::query_as(query)
            .fetch_all(&pool)
            .await;

        match result {
            Ok(rows) => {
                println!("\n{}:", name);
                for (_, _, _, detail) in rows {
                    println!("  {}", detail);
                    // Check if index is being used
                    // Fixed: Use ASCII-safe output
                    let uses_index = detail.contains("USING INDEX") || detail.contains("USING COVERING INDEX");
                    if uses_index {
                        println!("  OK: index is being used");
                    } else if detail.contains("SCAN") {
                        println!("  WARN: full table scan detected");
                    }
                }
            }
            Err(e) => {
                println!("{}: Error - {}", name, e);
            }
        }
    }
}

#[tokio::test]
#[ignore = "requires test database"]
async fn test_db_large_dataset_performance() {
    let pool = match create_test_pool().await {
        Ok(p) => p,
        Err(e) => {
            eprintln!("Failed to connect to database: {}", e);
            return;
        }
    };

    println!("\n=== Large Dataset Performance ===");

    // Test with different dataset sizes
    let sizes = [100, 500, 1000];

    for size in sizes {
        let project_id = match seed_test_data(&pool, size).await {
            Ok(id) => id,
            Err(e) => {
                eprintln!("Failed to seed {} records: {}", size, e);
                continue;
            }
        };

        let stats = QueryStats::new();

        // Run list query
        for _ in 0..10 {
            let start = Instant::now();

            // Fixed: Track query result for error handling
            let result: Result<Vec<(String,)>, _> = sqlx::query_as(
                "SELECT id FROM workflows WHERE project_id = ? ORDER BY created_at DESC LIMIT 50",
            )
            .bind(&project_id)
            .fetch_all(&pool)
            .await;

            let latency = start.elapsed().as_micros() as u64;

            if result.is_ok() {
                stats.record_query(latency);
            } else {
                stats.record_error();
            }
        }

        println!(
            "Dataset size: {} workflows, Avg query time: {:.2} ms, P95: {:.2} ms",
            size,
            stats.avg_latency_us().unwrap_or(0.0) / 1000.0,
            stats.p95_latency_us().unwrap_or(0) as f64 / 1000.0
        );

        // Cleanup
        let _ = cleanup_test_data(&pool, &project_id).await;
    }
}
