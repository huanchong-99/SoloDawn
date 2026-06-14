# Census: rs-services-githost

Unit: `crates/services/src/services/git_host/`
Branch: `refactor/streamline-quality-gates`
Date: 2026-06-14

## Module Map

| File | Purpose | Public Surface | Relations | Notes |
|------|---------|----------------|-----------|-------|
| `mod.rs` | Facade: exports the `GitHostProvider` trait, `GitHostService` enum-dispatch type, and re-exports key types. Entry point for callers. | `GitHostProvider` (trait), `GitHostService` (enum), `GitHostService::from_url`, re-exports: `CreatePrRequest`, `GitHostError`, `PrComment`, `PrCommentAuthor`, `PrReviewComment`, `ProviderKind`, `ReviewCommentUser`, `UnifiedPrComment` | Calls `detection::detect_provider_from_url`; constructs `GitHubProvider` or `AzureDevOpsProvider`. Called by: `pr_monitor.rs`, `server/routes/task_attempts/pr.rs`, `services/share.rs` (error type only). | `enum_dispatch` macro eliminates vtable; trait is `Send + Sync`. |
| `types.rs` | Shared domain types: request/response structs, error enum, unified comment model. | `ProviderKind` (enum, TS-exported), `CreatePrRequest`, `GitHostError`, `PrCommentAuthor`, `PrComment`, `ReviewCommentUser`, `PrReviewComment`, `UnifiedPrComment` (enum, TS-exported), `UnifiedPrComment::created_at()` | Imported by all other files in this module and by `server/bin/generate_types.rs` (TS bindings). `GitHostError` propagated into `server/error.rs::ApiError` and `services/share.rs::ShareError`. | `GitHostError::should_retry()` drives retry policy in both provider impls. |
| `detection.rs` | URL-to-provider detection logic. Parses HTTPS, SSH, SCP-style, GHE, legacy VisualStudio.com URLs. | `detect_provider_from_url(url: &str) -> ProviderKind` (pub within crate) | Called only by `mod.rs::GitHostService::from_url`. `detect_provider_from_pr_url` is `#[cfg(test)]`-only (used by unit tests, not production). | `extract_host` is private. `detect_provider_from_pr_url` is test-only — not exported to production paths. |
| `github/mod.rs` | GitHub provider: async `GitHostProvider` impl wrapping `GhCli`. Handles cross-fork PR head formatting, auth check, retry logic. | `GitHubProvider` (struct, `pub`), `GhCli` (re-exported via `pub use cli::GhCli`) | Calls `github/cli.rs` via `spawn_blocking`. Registered in `mod.rs` enum. `GhCli` re-export consumed by nothing outside this module (no external callers found). | Retry via `backon` ExponentialBuilder (1s–30s, 3 retries, jitter). Auth check done before every `create_pr`. |
| `github/cli.rs` | Low-level `gh` CLI wrapper. Spawns `gh` subprocess, parses JSON/text output. | `GhCli` (struct), `GitHubRepoInfo`, `GhCliError`, methods: `get_repo_info`, `create_pr`, `check_auth`, `view_pr`, `list_prs_for_branch`, `get_pr_comments`, `get_pr_review_comments` | Used exclusively by `github/mod.rs`. Depends on `utils::shell::resolve_executable_path_blocking`, `db::models::merge`, types from `git_host::types`. | Uses temp file for PR body to avoid shell escaping/length issues. Auth detection uses both exit code 4 and stderr string matching. |
| `azure/mod.rs` | Azure DevOps provider: async `GitHostProvider` impl wrapping `AzCli`. Cross-fork PRs explicitly rejected. | `AzureDevOpsProvider` (struct, `pub`), `AzCli` (re-exported via `pub use cli::AzCli`) | Calls `azure/cli.rs` via `spawn_blocking`. Registered in `mod.rs` enum. `AzCli` re-export consumed by nothing outside this module. | Same retry pattern as GitHub provider. `create_pr` returns early error for cross-fork scenarios. |
| `azure/cli.rs` | Low-level `az` CLI wrapper. Spawns `az repos`/`az devops invoke` subprocesses, parses JSON. | `AzCli` (struct), `AzureRepoInfo`, `AzCliError`, methods: `get_repo_info`, `create_pr`, `check_auth`, `view_pr`, `list_prs_for_branch`, `get_pr_threads`, `parse_pr_url` (pub) | Used exclusively by `azure/mod.rs`. Depends on `utils::shell::resolve_executable_path_blocking`, `db::models::merge`, types from `git_host::types`. | `get_pr_threads` uses `az devops invoke` REST API passthrough (not a direct `az repos pr` command). `parse_pr_url` is pub and tested. Legacy `visualstudio.com` format handled throughout. |

## External Callers Summary

| Caller | What it uses |
|--------|-------------|
| `crates/services/src/services/pr_monitor.rs` | `GitHostService::from_url`, `GitHostProvider::get_pr_status`, `GitHostError` |
| `crates/server/src/routes/task_attempts/pr.rs` | `GitHostService::from_url`, `create_pr`, `list_prs_for_branch`, `get_pr_comments`, `provider_kind`, `CreatePrRequest`, `GitHostError`, `ProviderKind`, `UnifiedPrComment` |
| `crates/services/src/services/share.rs` | `GitHostError` (error propagation only) |
| `crates/server/src/error.rs` | `GitHostError` (via `ApiError::GitHost`) |
| `crates/server/src/bin/generate_types.rs` | `UnifiedPrComment::decl()`, `ProviderKind::decl()` (TypeScript type generation) |

## Candidates

| # | Path | Lines | Kind | Evidence | Disposition | Confidence |
|---|------|--------|------|----------|-------------|------------|
| 1 | `detection.rs` | 69-88 | dead | `detect_provider_from_pr_url` is `#[cfg(test)]`-gated; used only in tests (lines 177-200). No production caller exists. | investigate | medium |
| 2 | `github/mod.rs` | 9 | redundant | `pub use cli::GhCli` re-exports `GhCli` publicly, but grep finds zero external callers consuming `git_host::github::GhCli` — only `github/mod.rs` itself uses it internally. | investigate | medium |
| 3 | `azure/mod.rs` | 9 | redundant | Same as above: `pub use cli::AzCli` re-exported but no external caller imports `git_host::azure::AzCli`. | investigate | medium |
