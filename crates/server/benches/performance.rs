//! Performance Benchmarks
//!
//! Run with: cargo bench --package server
//
// NOTE: Known bench limitations (W2-05-01..08). These benches are not
// production code; the caveats below are intentional but should frame how
// results are read:
//
// - W2-05-01: `bench_db_queries` constructs a `tokio::runtime::Runtime` that
//   is never used (the `_rt` binding is leaked for the lifetime of the bench
//   process). Cosmetic, but don't mirror this in real code.
// - W2-05-02: No real SQLite (not even in-memory) is exercised here. Any DB
//   numbers reported are synthetic and will not catch regressions in sqlx,
//   the query planner, or index usage.
// - W2-05-03: `workflow_list_query` and the `result_size` sweep are
//   `Vec<u8>` allocations, not queries — they measure allocator throughput,
//   not database selectivity. Fixed parameters mean the branch predictor and
//   cache are always hot.
// - W2-05-04: `Vec<u8>` simulations are not representative of row decoding
//   cost (row-to-struct mapping, TEXT→Uuid parsing, Option handling) that
//   dominates real workflow queries.
// - W2-05-05: JSON/UUID/string/SHA256 micro-benches use a single fixed input
//   per function; no input-size variance beyond the hardcoded `10000` byte
//   hash case.
// - W2-05-06: `bench_async_tasks` measures `tokio::spawn` of trivial
//   futures, which is dominated by scheduler bookkeeping and tells us
//   nothing about request-handling latency.
// - W2-05-07: No HTTP/axum layer is exercised — despite the file name,
//   end-to-end server performance is not benchmarked here.
// - W2-05-08: Results are not compared against a baseline or regression
//   threshold in CI; drift is invisible unless someone reads the report.

use std::time::Duration;

use criterion::{BenchmarkId, Criterion, Throughput, black_box, criterion_group, criterion_main};

/// Benchmark database query performance.
///
/// W2-05-05: the previous version built a `tokio::runtime::Runtime` it never
/// used. These benches are placeholders that measure allocator throughput, not
/// database work; the runtime has been removed and the whole module should be
/// replaced with real benches when the perf harness is set up.
fn bench_db_queries(c: &mut Criterion) {
    let mut group = c.benchmark_group("database_queries");
    group.measurement_time(Duration::from_secs(10));
    group.sample_size(50);

    // Benchmark workflow list query simulation
    group.bench_function("workflow_list_query", |b| {
        b.iter(|| {
            // Simulate query overhead
            let _result: Vec<u8> = (0..100).map(|i| i as u8).collect();
            black_box(_result)
        })
    });

    // Benchmark with different result sizes
    for size in [10, 50, 100, 500].iter() {
        group.throughput(Throughput::Elements(*size as u64));
        group.bench_with_input(BenchmarkId::new("result_size", size), size, |b, &size| {
            b.iter(|| {
                let _result: Vec<u8> = (0..size).map(|i| i as u8).collect();
                black_box(_result)
            })
        });
    }

    group.finish();
}

/// Benchmark JSON serialization/deserialization
fn bench_json_serde(c: &mut Criterion) {
    use serde::{Deserialize, Serialize};

    #[derive(Serialize, Deserialize, Clone)]
    struct WorkflowResponse {
        id: String,
        name: String,
        description: String,
        status: String,
        project_id: String,
        target_branch: String,
        created_at: String,
        updated_at: String,
    }

    let sample = WorkflowResponse {
        id: "550e8400-e29b-41d4-a716-446655440000".to_string(),
        name: "Test Workflow".to_string(),
        description: "A test workflow for benchmarking".to_string(),
        status: "created".to_string(),
        project_id: "660e8400-e29b-41d4-a716-446655440001".to_string(),
        target_branch: "main".to_string(),
        created_at: "2026-01-30T10:00:00Z".to_string(),
        updated_at: "2026-01-30T10:00:00Z".to_string(),
    };

    let mut group = c.benchmark_group("json_serde");

    // Serialize benchmark
    group.bench_function("serialize_workflow", |b| {
        b.iter(|| {
            let json = serde_json::to_string(&sample).unwrap();
            black_box(json)
        })
    });

    // Deserialize benchmark
    let json_str = serde_json::to_string(&sample).unwrap();
    group.bench_function("deserialize_workflow", |b| {
        b.iter(|| {
            let workflow: WorkflowResponse = serde_json::from_str(&json_str).unwrap();
            black_box(workflow)
        })
    });

    // Batch serialization
    let batch: Vec<_> = (0..100)
        .map(|i| {
            let mut w = sample.clone();
            w.id = format!("id-{}", i);
            w
        })
        .collect();

    group.bench_function("serialize_batch_100", |b| {
        b.iter(|| {
            let json = serde_json::to_string(&batch).unwrap();
            black_box(json)
        })
    });

    group.finish();
}

/// Benchmark UUID generation
fn bench_uuid_generation(c: &mut Criterion) {
    let mut group = c.benchmark_group("uuid");

    group.bench_function("generate_v4", |b| {
        b.iter(|| {
            let id = uuid::Uuid::new_v4();
            black_box(id)
        })
    });

    group.bench_function("parse_uuid", |b| {
        let uuid_str = "550e8400-e29b-41d4-a716-446655440000";
        b.iter(|| {
            let id: uuid::Uuid = uuid_str.parse().unwrap();
            black_box(id)
        })
    });

    group.bench_function("uuid_to_string", |b| {
        let id = uuid::Uuid::new_v4();
        b.iter(|| {
            let s = id.to_string();
            black_box(s)
        })
    });

    group.finish();
}

/// Benchmark string operations common in the codebase
fn bench_string_ops(c: &mut Criterion) {
    let mut group = c.benchmark_group("string_ops");

    // Branch name generation
    group.bench_function("branch_name_generation", |b| {
        b.iter(|| {
            let workflow_id = uuid::Uuid::new_v4();
            let name = "Test Workflow Name";
            let slug: String = name
                .chars()
                .map(|c| {
                    if c.is_alphanumeric() {
                        c.to_ascii_lowercase()
                    } else {
                        '-'
                    }
                })
                .collect();
            let branch = format!("workflow/{}/{}", workflow_id, slug);
            black_box(branch)
        })
    });

    // Path joining
    group.bench_function("path_joining", |b| {
        b.iter(|| {
            let base = "/home/user/projects";
            let project = "my-project";
            let file = "src/main.rs";
            let path = format!("{}/{}/{}", base, project, file);
            black_box(path)
        })
    });

    group.finish();
}

/// Benchmark encryption operations (if applicable)
fn bench_encryption(c: &mut Criterion) {
    use sha2::{Digest, Sha256};

    let mut group = c.benchmark_group("encryption");

    // SHA256 hashing
    group.bench_function("sha256_hash", |b| {
        let data = b"test data for hashing benchmark";
        b.iter(|| {
            let mut hasher = Sha256::new();
            hasher.update(data);
            let result = hasher.finalize();
            black_box(result)
        })
    });

    // Hash larger data
    let large_data: Vec<u8> = (0..10000).map(|i| (i % 256) as u8).collect();
    group.bench_function("sha256_hash_10kb", |b| {
        b.iter(|| {
            let mut hasher = Sha256::new();
            hasher.update(&large_data);
            let result = hasher.finalize();
            black_box(result)
        })
    });

    group.finish();
}

/// Benchmark async task spawning
fn bench_async_tasks(c: &mut Criterion) {
    let rt = tokio::runtime::Runtime::new().unwrap();

    let mut group = c.benchmark_group("async_tasks");
    group.measurement_time(Duration::from_secs(5));

    group.bench_function("spawn_task", |b| {
        b.to_async(&rt).iter(|| async {
            let handle = tokio::spawn(async { 42 });
            let result = handle.await.unwrap();
            black_box(result)
        })
    });

    group.bench_function("spawn_10_tasks", |b| {
        b.to_async(&rt).iter(|| async {
            let handles: Vec<_> = (0..10)
                .map(|i| tokio::spawn(async move { i * 2 }))
                .collect();

            let mut results = Vec::with_capacity(10);
            for handle in handles {
                results.push(handle.await.unwrap());
            }
            black_box(results)
        })
    });

    group.finish();
}

criterion_group!(
    benches,
    bench_db_queries,
    bench_json_serde,
    bench_uuid_generation,
    bench_string_ops,
    bench_encryption,
    bench_async_tasks,
);

criterion_main!(benches);
