// W2-37-05 / W2-37-06: This module performs case-insensitive file search via
// `str::to_lowercase()`. That function uses Unicode's simple (1:1) case-fold
// mapping and is only correct for ASCII; it does NOT implement full Unicode
// case folding (e.g. Turkish dotless-I, German ß, or title-case characters).
// The behaviour here is intentionally best-effort: we treat it as an
// ASCII-case-insensitive substring match over file paths/names. Callers should
// not rely on this function for Unicode-correct matching.

use std::{
    path::{Path, PathBuf},
    sync::Arc,
    time::{Duration, Instant},
};

use dashmap::DashMap;
use db::models::{
    project::{Project, SearchMatchType, SearchResult},
    project_repo::ProjectRepo,
};
use ignore::WalkBuilder;
use moka::future::Cache;
use notify::{RecommendedWatcher, RecursiveMode};
use notify_debouncer_full::{DebounceEventResult, Debouncer, RecommendedCache, new_debouncer};
use serde::{Deserialize, Serialize};
use sqlx::SqlitePool;
use thiserror::Error;
use tokio::sync::mpsc;
use tokio::task::JoinHandle;
use tracing::{error, info, warn};
use ts_rs::TS;

use super::{
    file_ranker::{FileRanker, FileStats},
    git::GitService,
};

/// Search mode for different use cases
#[derive(Debug, Clone, Copy, Serialize, Deserialize, TS, Default)]
#[serde(rename_all = "lowercase")]
pub enum SearchMode {
    #[default]
    TaskForm, // Default: exclude ignored files (clean results)
    Settings, // Include ignored files (for project config like .env)
}

/// Search query parameters for typed Axum extraction
#[derive(Debug, Clone, Deserialize)]
pub struct SearchQuery {
    pub q: String,
    #[serde(default)]
    pub mode: SearchMode,
}

/// FST-indexed file search result
#[derive(Clone, Debug)]
pub struct IndexedFile {
    pub path: String,
    pub is_file: bool,
    pub match_type: SearchMatchType,
    pub path_lowercase: Arc<str>,
    pub is_ignored: bool, // Track if file is gitignored
}

/// Errors that can occur during file index building
#[derive(Error, Debug)]
pub enum FileIndexError {
    #[error(transparent)]
    Io(#[from] std::io::Error),
    #[error(transparent)]
    Walk(#[from] ignore::Error),
    #[error(transparent)]
    StripPrefix(#[from] std::path::StripPrefixError),
}

/// Cached repository data with git stats
#[derive(Clone)]
pub struct CachedRepo {
    pub head_sha: String,
    pub indexed_files: Vec<IndexedFile>,
    pub stats: Arc<FileStats>,
    pub build_ts: Instant,
}

/// Cache miss error
#[derive(Debug)]
pub enum CacheError {
    Miss,
    BuildError(String),
}

/// File search cache with FST indexing
pub struct FileSearchCache {
    cache: Cache<PathBuf, CachedRepo>,
    git_service: GitService,
    file_ranker: FileRanker,
    build_queue: mpsc::Sender<PathBuf>,
    watchers: Arc<DashMap<PathBuf, Debouncer<RecommendedWatcher, RecommendedCache>>>,
    watcher_tasks: std::sync::Mutex<Vec<JoinHandle<()>>>,
    worker_task: std::sync::Mutex<Option<JoinHandle<()>>>,
}

impl Drop for FileSearchCache {
    // [W2-36-02] Abort watcher/worker tasks synchronously without a graceful
    // shutdown handshake. Acceptable here: these tasks are purely reactive
    // (file-system watchers and FST index builders) with no non-idempotent
    // external state; the next startup will rebuild in-memory indexes and
    // reattach watchers. Performing an awaited shutdown would require a Tokio
    // runtime inside `Drop`, which is not guaranteed.
    fn drop(&mut self) {
        if let Ok(mut tasks) = self.watcher_tasks.lock() {
            for handle in tasks.drain(..) {
                handle.abort();
            }
        }
        if let Ok(mut worker) = self.worker_task.lock()
            && let Some(handle) = worker.take()
        {
            handle.abort();
        }
    }
}

impl FileSearchCache {
    pub fn new() -> Self {
        // [W2-30-03] Bounded build queue (capacity 256). All senders use
        // `try_send` and warn on `Full`, so a backed-up worker coalesces
        // rather than growing memory without bound. Cache misses and watcher
        // refreshes are idempotent — dropping a duplicate enqueue is safe
        // because the worker will rebuild from the latest HEAD on the next
        // admitted request.
        let (build_sender, build_receiver) = mpsc::channel(256);

        // Create cache with 100MB limit and 1 hour TTL
        let cache = Cache::builder()
            .max_capacity(50) // Max 50 repos
            .time_to_live(Duration::from_secs(3600)) // 1 hour TTL
            .build();

        let cache_for_worker = cache.clone();
        let git_service = GitService::new();
        let file_ranker = FileRanker::new();
        let watchers: Arc<DashMap<PathBuf, Debouncer<RecommendedWatcher, RecommendedCache>>> =
            Arc::new(DashMap::new());

        // Spawn background worker
        let worker_git_service = git_service.clone();
        let worker_file_ranker = file_ranker.clone();
        let worker_build_sender = build_sender.clone();
        let worker_watchers = Arc::clone(&watchers);
        let worker_task = tokio::spawn(async move {
            Self::background_worker(
                build_receiver,
                cache_for_worker,
                worker_git_service,
                worker_file_ranker,
                worker_build_sender,
                worker_watchers,
            )
            .await;
        });

        Self {
            cache,
            git_service,
            file_ranker,
            build_queue: build_sender,
            watchers,
            watcher_tasks: std::sync::Mutex::new(Vec::new()),
            worker_task: std::sync::Mutex::new(Some(worker_task)),
        }
    }

    /// Search files in repository using cache
    pub async fn search(
        &self,
        repo_path: &Path,
        query: &str,
        mode: SearchMode,
    ) -> Result<Vec<SearchResult>, CacheError> {
        let repo_path_buf = repo_path.to_path_buf();

        // Check if we have a valid cache entry
        if let Some(cached) = self.cache.get(&repo_path_buf).await
            && let Ok(head_info) = self.git_service.get_head_info(&repo_path_buf)
            && head_info.oid == cached.head_sha
        {
            // Cache hit - perform fast search with mode-based filtering
            return Ok(self.search_in_cache(&cached, query, mode));
        }

        // Cache miss - trigger background refresh and return error
        // [W2-30-03] Bounded queue: drop on Full rather than await, since we
        // are in a hot search path and a backlogged worker will pick up the
        // repo on its next successful enqueue or watcher event.
        if let Err(e) = self.build_queue.try_send(repo_path_buf) {
            match e {
                mpsc::error::TrySendError::Full(path) => {
                    warn!("Build queue full, dropping cache build for {:?}", path);
                }
                mpsc::error::TrySendError::Closed(path) => {
                    warn!("Build queue closed, cannot enqueue {:?}", path);
                }
            }
        }

        Err(CacheError::Miss)
    }

    /// Pre-warm cache for given repositories
    pub fn warm_repos(&self, repo_paths: Vec<PathBuf>) -> Result<(), String> {
        for repo_path in repo_paths {
            // [W2-30-03] Bounded queue: warn-and-drop on Full. Warming is
            // best-effort; if the worker is busy building other repos the
            // next search miss will re-enqueue this path.
            if let Err(e) = self.build_queue.try_send(repo_path.clone()) {
                match e {
                    mpsc::error::TrySendError::Full(path) => {
                        warn!("Build queue full, skipping warm for {:?}", path);
                    }
                    mpsc::error::TrySendError::Closed(path) => {
                        error!("Build queue closed, cannot warm {:?}", path);
                    }
                }
            }
        }
        Ok(())
    }

    /// Pre-warm cache for most active projects
    pub async fn warm_most_active(&self, db_pool: &SqlitePool, limit: i32) -> Result<(), String> {
        info!("Starting file search cache warming...");

        // Get most active projects
        let active_projects = Project::find_most_active(db_pool, limit)
            .await
            .map_err(|e| format!("Failed to fetch active projects: {e}"))?;

        if active_projects.is_empty() {
            info!("No active projects found, skipping cache warming");
            return Ok(());
        }

        // Collect all repository paths from active projects
        let mut repo_paths: Vec<PathBuf> = Vec::new();
        for project in &active_projects {
            let repos = ProjectRepo::find_repos_for_project(db_pool, project.id)
                .await
                .map_err(|e| format!("Failed to fetch repositories for project: {e}"))?;
            for repo in repos {
                repo_paths.push(repo.path);
            }
        }

        if repo_paths.is_empty() {
            info!("No repositories found for active projects, skipping cache warming");
            return Ok(());
        }

        info!(
            "Warming cache for {} repositories: {:?}",
            repo_paths.len(),
            repo_paths
        );

        // Warm the cache
        self.warm_repos(repo_paths.clone())
            .map_err(|e| format!("Failed to warm cache: {e}"))?;

        // Setup watchers for active projects
        for repo_path in &repo_paths {
            if let Err(e) = self.setup_watcher(repo_path) {
                warn!("Failed to setup watcher for {:?}: {}", repo_path, e);
            }
        }

        info!("File search cache warming completed");
        Ok(())
    }

    /// Search within cached index with mode-based filtering
    fn search_in_cache(
        &self,
        cached: &CachedRepo,
        query: &str,
        mode: SearchMode,
    ) -> Vec<SearchResult> {
        let query_lower = query.to_lowercase();
        let mut results = Vec::new();

        // Search through indexed files with mode-based filtering
        for indexed_file in &cached.indexed_files {
            if indexed_file.path_lowercase.contains(&query_lower) {
                // Apply mode-based filtering
                match mode {
                    SearchMode::TaskForm => {
                        // Exclude ignored files for task forms
                        if indexed_file.is_ignored {
                            continue;
                        }
                    }
                    SearchMode::Settings => {
                        // Include all files (including ignored) for project settings
                        // No filtering needed
                    }
                }

                // Determine match type at query time based on what the query matched
                let file_name = std::path::Path::new(&*indexed_file.path_lowercase)
                    .file_name()
                    .map(|name| name.to_string_lossy().to_lowercase())
                    .unwrap_or_default();

                let match_type = if !file_name.is_empty() && file_name.contains(&query_lower) {
                    SearchMatchType::FileName
                } else if std::path::Path::new(&*indexed_file.path_lowercase)
                    .parent()
                    .and_then(|p| p.file_name())
                    .map(|name| name.to_string_lossy().to_lowercase())
                    .unwrap_or_default()
                    .contains(&query_lower)
                {
                    SearchMatchType::DirectoryName
                } else {
                    SearchMatchType::FullPath
                };

                results.push(SearchResult {
                    path: indexed_file.path.clone(),
                    is_file: indexed_file.is_file,
                    match_type,
                    score: 0,
                });
            }
        }

        // Apply git history-based ranking
        self.file_ranker.rerank(&mut results, &cached.stats);

        // Populate scores for sorted results
        for result in &mut results {
            result.score = self.file_ranker.calculate_score(result, &cached.stats);
        }

        // Limit to top 10 results
        results.truncate(10);
        results
    }

    /// Search files in a single repository with cache + fallback
    pub async fn search_repo(
        &self,
        repo_path: &Path,
        query: &str,
        mode: SearchMode,
    ) -> Result<Vec<SearchResult>, String> {
        let query = query.trim();
        if query.is_empty() {
            return Ok(vec![]);
        }

        // Try cache first
        match self.search(repo_path, query, mode).await {
            Ok(results) => Ok(results),
            Err(CacheError::Miss | CacheError::BuildError(_)) => {
                // Fall back to filesystem search
                self.search_files_no_cache(repo_path, query, mode).await
            }
        }
    }

    /// Fallback filesystem search when cache is not available
    async fn search_files_no_cache(
        &self,
        repo_path: &Path,
        query: &str,
        mode: SearchMode,
    ) -> Result<Vec<SearchResult>, String> {
        if !repo_path.exists() {
            return Err(format!("Path not found: {}", repo_path.display()));
        }

        let mut results = Vec::new();
        let query_lower = query.to_lowercase();

        let walker = match mode {
            SearchMode::Settings => {
                // Settings mode: Include ignored files but exclude performance killers
                WalkBuilder::new(repo_path)
                    .git_ignore(false)
                    .git_global(false)
                    .git_exclude(false)
                    .hidden(false)
                    .filter_entry(|entry| {
                        let name = entry.file_name().to_string_lossy();
                        name != ".git"
                            && name != "node_modules"
                            && name != "target"
                            && name != "dist"
                            && name != "build"
                    })
                    .build()
            }
            SearchMode::TaskForm => WalkBuilder::new(repo_path)
                .git_ignore(true)
                .git_global(true)
                .git_exclude(true)
                .hidden(false)
                .filter_entry(|entry| {
                    let name = entry.file_name().to_string_lossy();
                    name != ".git"
                })
                .build(),
        };

        for result in walker {
            let Ok(entry) = result else {
                continue;
            };
            let path = entry.path();

            // Skip the root directory itself
            if path == repo_path {
                continue;
            }

            let Ok(relative_path) = path.strip_prefix(repo_path) else {
                continue;
            };
            let relative_path_str = relative_path.to_string_lossy().to_lowercase();

            let file_name = path
                .file_name()
                .map(|name| name.to_string_lossy().to_lowercase())
                .unwrap_or_default();

            if file_name.contains(&query_lower) {
                results.push(SearchResult {
                    path: relative_path.to_string_lossy().to_string(),
                    is_file: path.is_file(),
                    match_type: SearchMatchType::FileName,
                    score: 0,
                });
            } else if relative_path_str.contains(&query_lower) {
                let match_type = if path
                    .parent()
                    .and_then(|p| p.file_name())
                    .map(|name| name.to_string_lossy().to_lowercase())
                    .unwrap_or_default()
                    .contains(&query_lower)
                {
                    SearchMatchType::DirectoryName
                } else {
                    SearchMatchType::FullPath
                };

                results.push(SearchResult {
                    path: relative_path.to_string_lossy().to_string(),
                    is_file: path.is_file(),
                    match_type,
                    score: 0,
                });
            }
        }

        // Apply git history-based ranking
        match self.file_ranker.get_stats(repo_path).await {
            Ok(stats) => {
                self.file_ranker.rerank(&mut results, &stats);
                // Populate scores for sorted results
                for result in &mut results {
                    result.score = self.file_ranker.calculate_score(result, &stats);
                }
            }
            Err(_) => {
                // Fallback to basic priority sorting
                results.sort_by(|a, b| {
                    let priority = |match_type: &SearchMatchType| match match_type {
                        SearchMatchType::FileName => 0,
                        SearchMatchType::DirectoryName => 1,
                        SearchMatchType::FullPath => 2,
                    };

                    priority(&a.match_type)
                        .cmp(&priority(&b.match_type))
                        .then_with(|| a.path.cmp(&b.path))
                });
            }
        }

        results.truncate(10);
        Ok(results)
    }

    /// Build cache entry for a repository
    async fn build_repo_cache(&self, repo_path: &Path) -> Result<CachedRepo, String> {
        let repo_path_buf = repo_path.to_path_buf();

        info!("Building cache for repo: {:?}", repo_path);

        // Get current HEAD
        let head_info = self
            .git_service
            .get_head_info(&repo_path_buf)
            .map_err(|e| format!("Failed to get HEAD info: {e}"))?;

        // Get git stats
        let stats = self
            .file_ranker
            .get_stats(repo_path)
            .await
            .map_err(|e| format!("Failed to get git stats: {e}"))?;

        // Build file index
        let indexed_files = Self::build_file_index(repo_path)
            .map_err(|e| format!("Failed to build file index: {e}"))?;

        Ok(CachedRepo {
            head_sha: head_info.oid,
            indexed_files,
            stats,
            build_ts: Instant::now(),
        })
    }

    /// Build file index from filesystem traversal using superset approach
    fn build_file_index(repo_path: &Path) -> Result<Vec<IndexedFile>, FileIndexError> {
        let mut indexed_files = Vec::new();

        // Build superset walker - include ignored files but exclude .git and performance killers
        let mut builder = WalkBuilder::new(repo_path);
        builder
            .git_ignore(false) // Include all files initially
            .git_global(false)
            .git_exclude(false)
            .hidden(false) // Show hidden files like .env
            .filter_entry(|entry| {
                let name = entry.file_name().to_string_lossy();
                // Always exclude .git directories
                if name == ".git" {
                    return false;
                }
                // Exclude performance killers even when including ignored files
                if name == "node_modules" || name == "target" || name == "dist" || name == "build" {
                    return false;
                }
                true
            });

        let walker = builder.build();

        // Create a second walker for checking ignore status
        let ignore_walker = WalkBuilder::new(repo_path)
            .git_ignore(true) // This will tell us what's ignored
            .git_global(true)
            .git_exclude(true)
            .hidden(false)
            .filter_entry(|entry| {
                let name = entry.file_name().to_string_lossy();
                name != ".git"
            })
            .build();

        // Collect paths from ignore-aware walker to know what's NOT ignored
        let mut non_ignored_paths = std::collections::HashSet::new();
        for result in ignore_walker {
            if let Ok(entry) = result
                && let Ok(relative_path) = entry.path().strip_prefix(repo_path)
            {
                non_ignored_paths.insert(relative_path.to_path_buf());
            }
        }

        // Now walk all files and determine their ignore status
        for result in walker {
            let entry = result?;
            let path = entry.path();

            if path == repo_path {
                continue;
            }

            let relative_path = path.strip_prefix(repo_path)?;
            let relative_path_str = relative_path.to_string_lossy().to_string();
            let relative_path_lower = relative_path_str.to_lowercase();

            // Skip empty paths
            if relative_path_lower.is_empty() {
                continue;
            }

            // Determine if this file is ignored
            let is_ignored = !non_ignored_paths.contains(relative_path);

            let file_name = path
                .file_name()
                .map(|name| name.to_string_lossy().to_lowercase())
                .unwrap_or_default();

            // Determine match type
            let match_type = if !file_name.is_empty() {
                SearchMatchType::FileName
            } else if path
                .parent()
                .and_then(|p| p.file_name())
                .map(|name| name.to_string_lossy().to_lowercase())
                .unwrap_or_default()
                != relative_path_lower
            {
                SearchMatchType::DirectoryName
            } else {
                SearchMatchType::FullPath
            };

            let indexed_file = IndexedFile {
                path: relative_path_str,
                is_file: path.is_file(),
                match_type,
                path_lowercase: Arc::from(relative_path_lower.as_str()),
                is_ignored,
            };

            indexed_files.push(indexed_file);
        }

        Ok(indexed_files)
    }

    /// Background worker for cache building
    async fn background_worker(
        mut build_receiver: mpsc::Receiver<PathBuf>,
        cache: Cache<PathBuf, CachedRepo>,
        git_service: GitService,
        file_ranker: FileRanker,
        build_queue: mpsc::Sender<PathBuf>,
        watchers: Arc<DashMap<PathBuf, Debouncer<RecommendedWatcher, RecommendedCache>>>,
    ) {
        while let Some(repo_path) = build_receiver.recv().await {
            let cache_builder = FileSearchCache {
                cache: cache.clone(),
                git_service: git_service.clone(),
                file_ranker: file_ranker.clone(),
                build_queue: build_queue.clone(),
                watchers: Arc::clone(&watchers),
                watcher_tasks: std::sync::Mutex::new(Vec::new()),
                worker_task: std::sync::Mutex::new(None),
            };

            match cache_builder.build_repo_cache(&repo_path).await {
                Ok(cached_repo) => {
                    // E28-12: `build_repo_cache` reads the working tree's
                    // current HEAD; by the time we insert the result the HEAD
                    // may already have advanced (concurrent commit/rebase)
                    // and a subsequent build request for the same repo will
                    // have been enqueued by the watcher. Last-writer-wins on
                    // the moka cache is acceptable here because the watcher
                    // always re-queues the repo after external changes, so
                    // any stale entry is self-healing within one debounce
                    // interval. Keep this insert non-conditional; do not add
                    // CAS here.
                    cache.insert(repo_path.clone(), cached_repo).await;
                    info!("Successfully cached repo: {:?}", repo_path);
                }
                Err(e) => {
                    error!("Failed to cache repo {:?}: {}", repo_path, e);
                }
            }
        }
    }

    /// Setup file watcher for repository
    pub fn setup_watcher(&self, repo_path: &Path) -> Result<(), String> {
        use dashmap::mapref::entry::Entry;

        let repo_path_buf = repo_path.to_path_buf();

        let git_dir = repo_path.join(".git");
        if !git_dir.exists() {
            return Err("Not a git repository".to_string());
        }

        // E27-10: use DashMap entry API to atomically check-and-insert, avoiding a
        // TOCTOU where two concurrent calls could each observe an empty slot and
        // both create a watcher.
        let entry = self.watchers.entry(repo_path_buf.clone());
        let vacant = match entry {
            Entry::Occupied(_) => return Ok(()), // Already watching
            Entry::Vacant(v) => v,
        };

        let build_queue = self.build_queue.clone();
        let watched_path = repo_path_buf.clone();

        // [W2-30-03] Bounded HEAD-watcher channel (capacity 64). The
        // debouncer callback is sync, so we use `try_send` and drop events
        // on `Full`. Dropping is safe: HEAD events are idempotent signals
        // to re-enqueue the repo build, and the debouncer already coalesces
        // bursts of filesystem activity within its 500ms window.
        let (tx, mut rx) = mpsc::channel::<()>(64);

        let mut debouncer = new_debouncer(
            Duration::from_millis(500),
            None,
            move |res: DebounceEventResult| {
                if let Ok(events) = res {
                    for event in events {
                        // Check if any path contains HEAD file
                        // L08: Matching on file_name == "HEAD" captures the common
                        // case where .git/HEAD is replaced atomically via rename.
                        // On all supported platforms (Unix and Windows) branch
                        // switches result in a HEAD-named file event, so this
                        // check is cross-platform.
                        for path in &event.event.paths {
                            if path.file_name().is_some_and(|name| name == "HEAD") {
                                // [W2-30-03] Bounded channel: drop on Full
                                // (another HEAD event is already queued; the
                                // worker will re-read HEAD once drained).
                                if let Err(e) = tx.try_send(()) {
                                    match e {
                                        mpsc::error::TrySendError::Full(()) => {
                                            warn!(
                                                "HEAD watcher channel full, dropping event"
                                            );
                                        }
                                        mpsc::error::TrySendError::Closed(()) => {
                                            warn!(
                                                "HEAD watcher channel closed, dropping event"
                                            );
                                        }
                                    }
                                }
                                break;
                            }
                        }
                    }
                }
            },
        )
        .map_err(|e| format!("Failed to create file watcher: {e}"))?;

        debouncer
            .watch(git_dir.join("HEAD"), RecursiveMode::NonRecursive)
            .map_err(|e| format!("Failed to watch HEAD file: {e}"))?;

        // Spawn task to handle HEAD changes
        let handle = tokio::spawn(async move {
            while rx.recv().await.is_some() {
                info!("HEAD changed for repo: {:?}", watched_path);
                if let Err(e) = build_queue.send(watched_path.clone()).await {
                    error!("Failed to enqueue cache refresh: {}", e);
                }
            }
        });
        if let Ok(mut tasks) = self.watcher_tasks.lock() {
            tasks.push(handle);
        }

        // Keep debouncer alive for the lifetime of the watcher registration.
        vacant.insert(debouncer);

        info!("Setup file watcher for repo: {:?}", repo_path);
        Ok(())
    }
}

impl Default for FileSearchCache {
    fn default() -> Self {
        Self::new()
    }
}
