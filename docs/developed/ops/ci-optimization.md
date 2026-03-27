# CI Pipeline Optimization

## Overview

This document describes the CI pipeline optimizations applied to SoloDawn. The guiding principles are:

1. **Zero check removal** -- every lint, test, and quality gate that existed before the optimization continues to run. Nothing is skipped.
2. **Faster feedback** -- reduce wall-clock time by eliminating redundant work, caching compilation artifacts, and parallelizing where possible.
3. **Lower maintenance cost** -- extract repeated setup logic into reusable Composite Actions so changes propagate from a single source of truth.

## Optimization Items

### 1. Remove Redundant `cargo check`

`cargo clippy` is a strict superset of `cargo check`: it runs the same compiler front-end and then applies additional lint passes. Running both `cargo check` and `cargo clippy` in the same job compiles the workspace twice for no additional coverage. The optimization removes the standalone `cargo check` step and relies solely on `cargo clippy`.

### 2. sccache Compilation Cache

[sccache](https://github.com/mozilla/sccache) intercepts `rustc` invocations and stores the resulting object files in a cache. On subsequent runs with identical inputs (source, flags, toolchain), it returns the cached artifact instead of recompiling.

**How it works in this project:**

| Setting | Value | Purpose |
|---------|-------|---------|
| `RUSTC_WRAPPER` | `sccache` | Tells Cargo to invoke sccache instead of rustc directly |
| `SCCACHE_GHA_ENABLED` | `true` | Uses the GitHub Actions cache as the storage backend |

The GitHub Actions cache backend stores artifacts keyed by a hash of the compiler invocation. Because GHA caches are scoped to the repository and branch, cache entries are shared across jobs within the same workflow run and across runs on the same branch. Pull request branches also read from the base branch cache, providing warm caches even on first PR runs.

**Cache granularity:** sccache operates at the individual compilation unit (crate/module) level, not at the workspace level. Changing one crate only invalidates that crate's cache entries; all other crates continue to hit the cache.

### 3. cargo-chef Docker Layer Caching

Docker rebuilds every layer after the first changed layer. In a naive Dockerfile, copying source code invalidates the dependency compilation layer, forcing a full rebuild on every code change.

[cargo-chef](https://github.com/LukeMathWalker/cargo-chef) solves this by splitting the build into two stages:

1. **Planner stage** (`chef-planner`): Scans `Cargo.toml` / `Cargo.lock` and produces a `recipe.json` that captures the dependency graph without any project source code.
2. **Builder stage** (`chef-builder`): First compiles all dependencies from `recipe.json` (the "cook" step). Then copies project source and compiles only the project crates.

Because `recipe.json` only changes when dependencies change, the cook layer is cached across builds where only source code changed. This turns a 10+ minute full rebuild into a fast incremental compile of project crates only.

The reference template is in `docker/Dockerfile.chef-stage`. Key details:

- Both stages inherit from the `rust-base` stage (toolchain + system deps).
- The cook step uses `--profile docker` (opt-level=1, codegen-units=256) for faster CI builds.
- BuildKit cache mounts (`--mount=type=cache`) keep the cargo registry warm across builds.

### 4. Composite Actions

Three reusable Composite Actions eliminate duplicated setup steps across workflow jobs:

| Action | Location | Purpose |
|--------|----------|---------|
| `setup-rust` | `.github/actions/setup-rust/action.yml` | System deps, Rust toolchain, build env vars, optional sccache env, optional nextest |
| `setup-frontend` | `.github/actions/setup-frontend/action.yml` | pnpm, Node.js, frontend dependency install |
| `setup-sccache` | `.github/actions/setup-sccache/action.yml` | sccache binary install, GHA cache backend config, stats reporting |

Before this optimization, each job duplicated 15-20 lines of setup YAML. With Composite Actions, a job's setup section is reduced to a few `uses:` lines, and any change (e.g., upgrading a system dependency) is made in one place.

### 5. cargo-nextest Test Acceleration

[cargo-nextest](https://nexte.st/) is a drop-in replacement for `cargo test` that runs each test binary in parallel with better scheduling. Benefits:

- Tests from different crates run concurrently (cargo test runs them sequentially by default).
- Faster test discovery and execution.
- Structured output with per-test timing.

nextest is installed conditionally via the `setup-rust` action's `install-nextest: true` input.

### 6. Cross-Job Cache Sharing

sccache with the GHA cache backend automatically shares compilation artifacts across jobs within the same workflow and across workflow runs on the same branch. This means:

- A "build" job that compiles the workspace populates the cache.
- A subsequent "test" job reuses those cached artifacts instead of recompiling.
- The next push to the same branch starts with a warm cache from the previous run.

No explicit `actions/cache` configuration is needed for Rust artifacts -- sccache handles it transparently.

## Composite Actions Reference

### setup-rust

**Location:** `.github/actions/setup-rust/action.yml`

**Inputs:**

| Input | Default | Description |
|-------|---------|-------------|
| `toolchain` | `nightly-2025-12-04` | Rust toolchain version to install |
| `components` | `rustfmt, clippy` | Comma-separated list of rustup components |
| `sccache-enabled` | `true` | Set `RUSTC_WRAPPER` and `SCCACHE_GHA_ENABLED` env vars |
| `install-nextest` | `false` | Download and install cargo-nextest binary |
| `extra-packages` | `''` | Additional apt packages to install (space-separated) |

**What it does:**

1. Installs system dependencies (pkg-config, libsqlite3-dev, libgit2-dev, zlib1g-dev, cmake, ninja-build, clang, libclang-dev, perl, nasm, libssl-dev, protobuf-compiler).
2. Installs any extra system packages specified via `extra-packages`.
3. Installs the Rust toolchain via `dtolnay/rust-toolchain`.
4. Sets build environment variables (`CARGO_TERM_COLOR`, `SQLX_OFFLINE`, `AWS_LC_SYS_STATIC`, `AWS_LC_SYS_NO_PREGENERATED_SRC`, `LIBGIT2_SYS_USE_PKG_CONFIG`).
5. Optionally sets sccache environment variables.
6. Optionally installs cargo-nextest.

**Usage example:**

```yaml
- uses: ./.github/actions/setup-rust
  with:
    install-nextest: true
```

### setup-frontend

**Location:** `.github/actions/setup-frontend/action.yml`

**Inputs:**

| Input | Default | Description |
|-------|---------|-------------|
| `node-version` | `20` | Node.js version |
| `pnpm-version` | `10` | pnpm version |
| `working-directory` | `frontend` | Directory for `pnpm install` |
| `frozen-lockfile` | `true` | Use `--frozen-lockfile` flag |

**What it does:**

1. Installs pnpm via `pnpm/action-setup`.
2. Sets up Node.js via `actions/setup-node` with pnpm caching enabled.
3. Runs `pnpm install` (with `--frozen-lockfile` by default).

**Usage example:**

```yaml
- uses: ./.github/actions/setup-frontend
```

### setup-sccache

**Location:** `.github/actions/setup-sccache/action.yml`

**Inputs:**

| Input | Default | Description |
|-------|---------|-------------|
| `version` | `0.8.2` | sccache version to install |
| `enabled` | `true` | Set to `false` to skip all sccache steps |

**What it does:**

1. Installs sccache via `mozilla-actions/sccache-action`.
2. Sets `SCCACHE_GHA_ENABLED=true` and `RUSTC_WRAPPER=sccache` in the environment.
3. Prints sccache configuration for debugging.
4. Shows sccache cache statistics at the end of the job (runs even if previous steps fail).

**Usage example:**

```yaml
- uses: ./.github/actions/setup-sccache
  with:
    version: '0.8.2'
```

## Troubleshooting

### sccache cache misses (compilation is not faster)

**Symptoms:** sccache stats show zero cache hits; build times are not improving.

**Checklist:**

1. Verify `RUSTC_WRAPPER` is set to `sccache`:
   ```bash
   echo $RUSTC_WRAPPER  # should print "sccache"
   ```
2. Verify `SCCACHE_GHA_ENABLED` is `true`:
   ```bash
   echo $SCCACHE_GHA_ENABLED  # should print "true"
   ```
3. Check sccache stats after a build:
   ```bash
   sccache --show-stats
   ```
   If "Compile requests" is 0, sccache is not intercepting rustc calls. Confirm the `setup-sccache` action ran before `setup-rust`, or that `setup-rust` has `sccache-enabled: true`.
4. Cache may be cold on a new branch. The first run always compiles from scratch; subsequent runs on the same branch will hit the cache.

### sccache failure (build errors mentioning sccache)

**Symptoms:** Build fails with errors referencing sccache, or sccache returns non-zero exit codes.

**Resolution:** Disable sccache as a fallback by setting `sccache-enabled: false` in the `setup-rust` action and `enabled: false` in the `setup-sccache` action. This removes sccache from the compilation pipeline entirely, falling back to direct rustc invocation. Then investigate the sccache issue separately.

```yaml
- uses: ./.github/actions/setup-sccache
  with:
    enabled: false

- uses: ./.github/actions/setup-rust
  with:
    sccache-enabled: false
```

### cargo-chef recipe changes (dependency layer cache miss)

The `recipe.json` produced by `cargo chef prepare` changes when:

- Any `Cargo.toml` in the workspace is modified (added/removed dependencies, changed features).
- `Cargo.lock` changes (dependency version updates).
- The crate directory structure changes (new crate added, crate renamed/removed).

When `recipe.json` changes, the `cargo chef cook` layer is invalidated and all dependencies are recompiled. This is expected and unavoidable -- it only happens when dependencies actually change.

**Minimizing impact:** Batch dependency updates into fewer commits. Avoid unnecessary `Cargo.toml` formatting changes.

### Docker cache invalidation (full image rebuild)

The Docker build caches layers sequentially. Changes to these files cause the corresponding layer and all subsequent layers to rebuild:

| Changed file | Invalidated layers |
|---|---|
| Base image (e.g., Debian update) | Everything |
| System dependency list in `rust-base` | Toolchain install + all Rust compilation |
| `Cargo.toml` / `Cargo.lock` | `chef-planner` + `chef-builder` cook + project build |
| Any file in `crates/` | `chef-planner` + project build (cook layer is preserved if recipe unchanged) |
| Frontend source (`frontend/`) | `frontend-builder` + final COPY in chef-builder |

BuildKit cache mounts (`--mount=type=cache,id=solodawn-cargo-registry,...`) persist the cargo registry across builds regardless of layer invalidation, so even a full rebuild avoids re-downloading crates.

### cargo-nextest compatibility issues

If nextest fails on a specific test (e.g., tests that rely on shared global state or non-standard test harnesses), fall back to `cargo test` for that specific crate:

```yaml
# Run most tests with nextest
- run: cargo nextest run --workspace --exclude problematic-crate

# Fall back to cargo test for the problematic crate
- run: cargo test -p problematic-crate
```

To disable nextest entirely, set `install-nextest: false` in the `setup-rust` action and replace `cargo nextest run` with `cargo test` in the workflow steps.

## CI Timing Monitoring

### benchmark-ci.sh

**Location:** `scripts/ci/benchmark-ci.sh`

A lightweight timing utility that records start/end timestamps for named CI steps and produces a JSON summary.

**Commands:**

```bash
# Record the start of a step
./scripts/ci/benchmark-ci.sh start <step_name>

# Record the end of a step (prints duration)
./scripts/ci/benchmark-ci.sh end <step_name>

# Output JSON summary of all recorded steps
./scripts/ci/benchmark-ci.sh report
```

**Example workflow usage:**

```yaml
- run: ./scripts/ci/benchmark-ci.sh start rust-build
- run: cargo build --workspace
- run: ./scripts/ci/benchmark-ci.sh end rust-build

- run: ./scripts/ci/benchmark-ci.sh start rust-test
- run: cargo nextest run --workspace
- run: ./scripts/ci/benchmark-ci.sh end rust-test

- run: ./scripts/ci/benchmark-ci.sh report
```

**Output format:**

```json
{
  "steps": [
    {"name": "rust-build", "duration_seconds": 142},
    {"name": "rust-test", "duration_seconds": 38}
  ],
  "total_seconds": 180
}
```

When running in GitHub Actions, the script automatically appends timing data to `$GITHUB_STEP_SUMMARY` as a Markdown table.

The benchmark data directory defaults to `/tmp/ci-benchmark` and can be overridden via the `CI_BENCHMARK_DIR` environment variable.

### test-sccache.sh

**Location:** `scripts/ci/test-sccache.sh`

A diagnostic script that verifies sccache integration is working correctly. Run it in CI or locally to confirm that sccache is intercepting rustc calls and using the cache.

**What it checks (5 steps):**

1. sccache binary is available in PATH.
2. `RUSTC_WRAPPER` is set to `sccache` and `SCCACHE_GHA_ENABLED` is `true`.
3. Captures initial sccache statistics (compile requests, cache hits).
4. Runs `cargo check -p utils` to trigger compilation through sccache.
5. Compares sccache statistics before and after -- compile requests should increase. Cache hits increasing indicates a warm cache.

**Usage:**

```bash
./scripts/ci/test-sccache.sh
```

**Exit codes:**

- `0` -- all checks passed; sccache is working.
- `1` -- one or more checks failed; see output for details.

**Example output (healthy):**

```
[1/5  Checking sccache binary]
  PASS  sccache found at /usr/local/bin/sccache (sccache 0.8.2)

[2/5  Checking environment variables]
  PASS  RUSTC_WRAPPER is set to 'sccache'
  PASS  SCCACHE_GHA_ENABLED is 'true'

[3/5  Capturing initial sccache stats]
  INFO  Compile requests before: 0
  INFO  Cache hits before:       0
  PASS  Initial stats captured

[4/5  Running compile test (cargo check -p utils)]
  PASS  cargo check -p utils succeeded

[5/5  Comparing sccache stats after compilation]
  PASS  Compile requests increased (0 -> 12)
  INFO  Cache hits did not increase (0 -> 0) -- this is normal on a cold cache

  Summary: 6 checks, 6 passed, 0 failed

RESULT: PASS -- sccache integration is working correctly.
```
