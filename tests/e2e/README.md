# End-to-End Tests for SoloDawn Workflow System

This directory contains comprehensive end-to-end tests for the SoloDawn workflow orchestration system.

## Overview

These tests verify the complete workflow system functionality by making actual HTTP requests to a running server instance. The tests cover:

- CLI type detection and management
- Model configuration retrieval
- Workflow CRUD operations (Create, Read, Update, Delete)
- Workflow lifecycle management
- Error handling and edge cases
- Integration between workflow components (slash commands, orchestrator, terminals)

## Prerequisites

### 1. Running Server

The tests require the SoloDawn server to be running on `http://localhost:3001`.

Start the server:

```bash
cargo run --bin server
```

Or using the workspace:

```bash
cargo run -p server
```

### 2. Database Initialization

The database must be initialized with seed data including:

- CLI types (e.g., `claude-code`, `aider`, `cursor`)
- Model configurations for each CLI type
- Slash command presets (system presets like `/write-code`, `/review-code`)

Seed data is typically loaded during server startup or via database migrations.

### 3. Environment Variables

Some tests may require environment variables:

- `SOLODAWN_ENCRYPTION_KEY` - 32-byte encryption key for API key storage (required for workflows with orchestrator configuration)

Example:

```bash
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
```

## Running Tests

### Run All E2E Tests

```bash
cargo test --test workflow_test
```

### Run Specific Test

```bash
cargo test --test workflow_test test_cli_detection
cargo test --test workflow_test test_workflow_lifecycle
```

### Run with Output

```bash
cargo test --test workflow_test -- --nocapture
```

### Run Specific Test with Output

```bash
cargo test --test workflow_test test_workflow_lifecycle -- --nocapture
```

## Test Coverage

### 1. `test_cli_detection`

Tests the CLI detection endpoint (`GET /api/cli_types/detect`).

**Verifies:**
- Response structure contains required fields
- Returns at least one CLI type
- Each CLI type has: `cliTypeId`, `name`, `displayName`, `installed`, `installGuideUrl`

### 2. `test_list_cli_types`

Tests listing all available CLI types (`GET /api/cli_types`).

**Verifies:**
- Response structure contains required fields
- Returns CLI types with: `id`, `name`, `displayName`, `detectCommand`

### 3. `test_list_models_for_cli`

Tests retrieving model configurations for a specific CLI type (`GET /api/cli_types/:id/models`).

**Verifies:**
- Returns models for valid CLI type ID
- Each model has: `id`, `cliTypeId`, `name`
- Models' `cliTypeId` matches the requested CLI type

### 4. `test_list_command_presets`

Tests listing available slash command presets (`GET /api/workflows/presets/commands`).

**Verifies:**
- Returns system command presets
- Each preset has: `id`, `command`, `description`, `isSystem`
- All commands start with `/` (slash)

### 5. `test_workflow_lifecycle`

Tests complete workflow CRUD operations.

**Verifies:**
1. **Create** workflow with valid configuration
2. **Get** workflow details by ID
3. **List** workflows for a project
4. **Update** workflow status
5. **Verify** status update persisted
6. **Delete** workflow
7. **Verify** deletion (workflow no longer exists)

### 6. `test_workflow_with_tasks`

Tests creating workflows with advanced configurations.

**Verifies:**
- Creating workflow with slash commands
- Creating workflow with orchestrator configuration
- Orchestrator settings: `apiType`, `baseUrl`, `model`
- Slash command associations are created correctly
- Target branch configuration
- Cleanup (deletes created workflow)

### 7. `test_workflow_error_handling`

Tests error handling and edge cases.

**Verifies:**
1. Rejects invalid CLI type ID (400/500 error)
2. Handles non-existent workflow gracefully (404 or null response)
3. Rejects status update for non-existent workflow (400/404)
4. Handles delete of non-existent workflow (idempotent or 404)
5. Rejects invalid status values (400/422)

## Test Data

### Project IDs

Each test generates a unique project ID using UUID:

```rust
fn test_project_id() -> String {
    Uuid::new_v4().to_string()
}
```

This ensures tests don't interfere with each other.

### CLI Types and Models

Tests dynamically fetch CLI types and models from the API, ensuring tests work with whatever data is seeded:

```rust
// Get CLI types
let cli_response = client.get("/api/cli_types").send().await;
let cli_types: Vec<Value> = cli_response.json().await;

// Get models for CLI type
let models_response = client.get(format!("/api/cli_types/{}/models", cli_type_id)).send().await;
let models: Vec<Value> = models_response.json().await;
```

### Cleanup

Tests automatically clean up created resources:

```rust
// Cleanup: delete the workflow
let _ = client.delete(format!("/workflows/{}", workflow_id)).send().await;
```

## API Endpoints Tested

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/api/cli_types` | List all CLI types |
| GET | `/api/cli_types/detect` | Detect installed CLIs |
| GET | `/api/cli_types/:id/models` | List models for CLI type |
| GET | `/api/workflows/presets/commands` | List slash command presets |
| GET | `/api/workflows?project_id=xxx` | List workflows for project |
| GET | `/api/workflows/:id` | Get workflow details |
| POST | `/api/workflows` | Create workflow |
| PUT | `/api/workflows/:id/status` | Update workflow status |
| DELETE | `/api/workflows/:id` | Delete workflow |

## Troubleshooting

### Connection Refused

If tests fail with "connection refused", ensure the server is running:

```bash
# Check if server is running on port 3001
curl http://localhost:3001/api/cli_types
```

### Missing CLI Types or Models

If tests fail due to missing data, ensure the database is seeded:

```bash
# Run database migrations
cargo run --bin migrate

# Or seed data via server CLI
cargo run --bin server -- --seed
```

### Encryption Key Errors

If tests fail with encryption key errors, set the environment variable:

```bash
export SOLODAWN_ENCRYPTION_KEY="12345678901234567890123456789012"
```

The key must be exactly 32 bytes (characters).

### Timeouts

Tests have a 30-second timeout. If tests timeout:

1. Check server performance
2. Verify database queries are optimized
3. Check network connectivity

## File Structure

```
tests/e2e/
├── README.md              # This file
└── workflow_test.rs       # E2E test implementations
```

## Adding New Tests

When adding new E2E tests:

1. Follow the existing test structure
2. Use the `client()` helper for HTTP requests
3. Generate unique IDs using `Uuid::new_v4()`
4. Clean up created resources
5. Add descriptive assertions with helpful error messages
6. Update this README with the new test description

Example:

```rust
#[tokio::test]
async fn test_new_feature() {
    let client = client();
    let project_id = test_project_id();

    // Arrange: Set up test data
    let response = client
        .post("/api/endpoint")
        .json(&json!({"projectId": project_id}))
        .send()
        .await
        .expect("Failed to send request");

    // Assert: Verify response
    assert_eq!(response.status(), 200);

    // Cleanup: Remove test data
    let _ = client.delete(format!("/api/endpoint/{}", id)).send().await;
}
```

## CI/CD Integration

These tests can be integrated into CI/CD pipelines:

```yaml
# .github/workflows/e2e-tests.yml
name: E2E Tests

on: [push, pull_request]

jobs:
  e2e:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions-rust-lang/setup-rust-toolchain@v1

      - name: Start server
        run: |
          cargo run --bin server &
          sleep 10  # Wait for server to start

      - name: Run E2E tests
        run: cargo test --test workflow_test
```

## Related Documentation

- [Workflow API Documentation](../../../docs/workflow-api.md)
- [Database Schema](../../../docs/database-schema.md)
- [Integration Tests](../integration/README.md)
- [Unit Tests](../../crates/db/src/models/workflow.rs)

## Support

For issues or questions about E2E tests:

1. Check the troubleshooting section above
2. Review server logs for errors
3. Verify database has required seed data
4. Check network connectivity to localhost:3001
5. Review test output with `-- --nocapture` flag
