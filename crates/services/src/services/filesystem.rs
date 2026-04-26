#[cfg(not(feature = "qa-mode"))]
use std::collections::HashSet;
use std::{
    fs,
    path::{Path, PathBuf},
};

#[cfg(not(feature = "qa-mode"))]
use ignore::WalkBuilder;
use serde::Serialize;
use thiserror::Error;
#[cfg(not(feature = "qa-mode"))]
use tokio_util::sync::CancellationToken;
use ts_rs::TS;

#[derive(Clone)]
pub struct FilesystemService {
    allowed_roots: Vec<PathBuf>,
}

#[cfg(test)]
mod tests {
    use super::FilesystemService;

    #[test]
    fn list_directory_prefers_workspace_root_when_allowed() {
        let workspace = tempfile::tempdir().expect("workspace tempdir");
        std::fs::create_dir_all(workspace.path().join("repo-a")).expect("create repo-a");

        let service = FilesystemService::new_with_roots(vec![workspace.path().to_path_buf()]);

        unsafe {
            std::env::set_var(
                "SOLODAWN_WORKSPACE_ROOT",
                workspace.path().to_string_lossy().to_string(),
            );
        }

        let result = service
            .list_directory(None)
            .expect("workspace root should be browsable");

        let canonical_workspace =
            dunce::canonicalize(workspace.path()).expect("canonical workspace");
        assert_eq!(
            result.current_path,
            canonical_workspace.to_string_lossy().to_string()
        );

        unsafe {
            std::env::remove_var("SOLODAWN_WORKSPACE_ROOT");
        }
    }

    #[test]
    fn list_directory_falls_back_to_allowed_root_when_workspace_root_outside_boundary() {
        let allowed = tempfile::tempdir().expect("allowed tempdir");
        std::fs::create_dir_all(allowed.path().join("repo-b")).expect("create repo-b");

        let outside = tempfile::tempdir().expect("outside tempdir");
        let service = FilesystemService::new_with_roots(vec![allowed.path().to_path_buf()]);

        unsafe {
            std::env::set_var(
                "SOLODAWN_WORKSPACE_ROOT",
                outside.path().to_string_lossy().to_string(),
            );
        }

        let result = service
            .list_directory(None)
            .expect("should fall back to allowed root");

        let canonical_allowed = dunce::canonicalize(allowed.path()).expect("canonical allowed");
        assert_eq!(
            result.current_path,
            canonical_allowed.to_string_lossy().to_string()
        );

        unsafe {
            std::env::remove_var("SOLODAWN_WORKSPACE_ROOT");
        }
    }
}

#[derive(Debug, Error)]
pub enum FilesystemError {
    #[error("Directory does not exist")]
    DirectoryDoesNotExist,
    #[error("Path is not a directory")]
    PathIsNotDirectory,
    #[error("Path is outside allowed roots")]
    PathOutsideAllowedRoots,
    #[error("Failed to read directory: {0}")]
    Io(#[from] std::io::Error),
}
#[derive(Debug, Serialize, TS)]
pub struct DirectoryListResponse {
    pub entries: Vec<DirectoryEntry>,
    pub current_path: String,
}

#[derive(Debug, Serialize, TS)]
pub struct DirectoryEntry {
    pub name: String,
    pub path: PathBuf,
    pub is_directory: bool,
    pub is_git_repo: bool,
    pub last_modified: Option<u64>,
}

impl Default for FilesystemService {
    fn default() -> Self {
        Self::new()
    }
}

impl FilesystemService {
    pub fn new() -> Self {
        let mut allowed_roots = vec![Self::get_home_directory()];
        if let Ok(cwd) = std::env::current_dir() {
            allowed_roots.push(cwd);
        }
        allowed_roots.extend(Self::get_env_allowed_roots());
        allowed_roots.extend(Self::get_windows_drive_roots(&allowed_roots));
        Self::new_with_roots(allowed_roots)
    }

    fn get_env_allowed_roots() -> Vec<PathBuf> {
        let mut roots = Vec::new();
        if let Ok(raw) =
            utils::env_compat::var_with_compat("SOLODAWN_ALLOWED_ROOTS", "GITCORTEX_ALLOWED_ROOTS")
        {
            for item in raw.split([',', ';']) {
                let trimmed = item.trim();
                if trimmed.is_empty() {
                    continue;
                }
                roots.push(PathBuf::from(trimmed));
            }
        }
        roots
    }

    #[cfg(windows)]
    fn get_windows_drive_roots(existing_roots: &[PathBuf]) -> Vec<PathBuf> {
        let mut drive_roots = Vec::new();

        for root in existing_roots {
            if let Some(drive_root) = Self::extract_windows_drive_root(root)
                && !drive_roots.iter().any(|existing| existing == &drive_root)
            {
                drive_roots.push(drive_root);
            }
        }

        // Include mounted common drive roots to support browsing from drive root.
        for drive in 'C'..='Z' {
            let candidate = PathBuf::from(format!("{drive}:\\"));
            if candidate.is_dir() && !drive_roots.iter().any(|existing| existing == &candidate) {
                drive_roots.push(candidate);
            }
        }

        drive_roots
    }

    #[cfg(not(windows))]
    fn get_windows_drive_roots(_existing_roots: &[PathBuf]) -> Vec<PathBuf> {
        Vec::new()
    }

    #[cfg(windows)]
    fn extract_windows_drive_root(path: &Path) -> Option<PathBuf> {
        match path.components().next() {
            Some(std::path::Component::Prefix(prefix)) => match prefix.kind() {
                std::path::Prefix::Disk(drive) | std::path::Prefix::VerbatimDisk(drive) => {
                    let drive_letter = char::from(drive).to_ascii_uppercase();
                    Some(PathBuf::from(format!("{drive_letter}:\\")))
                }
                _ => None,
            },
            _ => None,
        }
    }

    pub fn new_with_roots(allowed_roots: Vec<PathBuf>) -> Self {
        let normalized_roots = Self::normalize_allowed_roots(allowed_roots);
        FilesystemService {
            allowed_roots: normalized_roots,
        }
    }

    fn normalize_allowed_roots(allowed_roots: Vec<PathBuf>) -> Vec<PathBuf> {
        let mut normalized = Vec::new();
        for root in allowed_roots {
            if root.as_os_str().is_empty() {
                continue;
            }
            let canonical = dunce::canonicalize(&root).unwrap_or(root);
            if !normalized.iter().any(|existing| existing == &canonical) {
                normalized.push(canonical);
            }
        }
        normalized
    }

    #[cfg(not(feature = "qa-mode"))]
    fn get_directories_to_skip() -> HashSet<String> {
        let mut skip_dirs = HashSet::from(
            [
                "node_modules",
                "target",
                "build",
                "dist",
                ".next",
                ".nuxt",
                ".cache",
                ".npm",
                ".yarn",
                ".pnpm-store",
                "Library",
                "AppData",
                "Applications",
            ]
            .map(String::from),
        );

        [
            dirs::executable_dir(),
            dirs::data_dir(),
            dirs::download_dir(),
            dirs::picture_dir(),
            dirs::video_dir(),
            dirs::audio_dir(),
        ]
        .into_iter()
        .flatten()
        .filter_map(|path| path.file_name()?.to_str().map(String::from))
        .for_each(|name| {
            skip_dirs.insert(name);
        });

        skip_dirs
    }

    #[cfg_attr(feature = "qa-mode", allow(unused_variables))]
    pub async fn list_git_repos(
        &self,
        path: Option<String>,
        timeout_ms: u64,
        hard_timeout_ms: u64,
        max_depth: Option<usize>,
    ) -> Result<Vec<DirectoryEntry>, FilesystemError> {
        #[cfg(feature = "qa-mode")]
        {
            tracing::info!("QA mode: returning hardcoded QA repos instead of scanning filesystem");
            super::qa_repos::get_qa_repos()
        }

        #[cfg(not(feature = "qa-mode"))]
        {
            let base_path = path.map_or_else(|| self.default_browse_root(), PathBuf::from);
            Self::verify_directory(&base_path)?;
            let base_path = self.resolve_path(&base_path)?;
            self.list_git_repos_with_timeout(
                vec![base_path],
                timeout_ms,
                hard_timeout_ms,
                max_depth,
            )
            .await
        }
    }

    #[cfg(not(feature = "qa-mode"))]
    async fn list_git_repos_with_timeout(
        &self,
        paths: Vec<PathBuf>,
        timeout_ms: u64,
        hard_timeout_ms: u64,
        max_depth: Option<usize>,
    ) -> Result<Vec<DirectoryEntry>, FilesystemError> {
        let cancel_token = CancellationToken::new();
        let cancel_after_delay = cancel_token.clone();
        tokio::spawn(async move {
            tokio::time::sleep(std::time::Duration::from_millis(timeout_ms)).await;
            cancel_after_delay.cancel();
        });
        let cancel_for_scan = cancel_token.clone();
        let mut scan_handle = tokio::spawn(async move {
            Self::list_git_repos_inner(&paths, max_depth, Some(&cancel_for_scan))
        });

        let hard_timeout = tokio::time::sleep(std::time::Duration::from_millis(hard_timeout_ms));
        tokio::pin!(hard_timeout);

        tokio::select! {
            res = &mut scan_handle => {
                match res {
                    Ok(repos) => Ok(repos),
                    Err(join_err) => Err(FilesystemError::Io(
                        std::io::Error::other(join_err.to_string()))),
                }
            }
            () = &mut hard_timeout => {
                scan_handle.abort();
                tracing::warn!("list_git_repos_with_timeout: hard timeout reached after {}ms", hard_timeout_ms);
                Err(FilesystemError::Io(std::io::Error::new(
                    std::io::ErrorKind::TimedOut,
                    "Operation forcibly terminated due to hard timeout",
                )))
            }
        }
    }

    #[cfg_attr(feature = "qa-mode", allow(unused_variables))]
    pub async fn list_common_git_repos(
        &self,
        timeout_ms: u64,
        hard_timeout_ms: u64,
        max_depth: Option<usize>,
    ) -> Result<Vec<DirectoryEntry>, FilesystemError> {
        #[cfg(feature = "qa-mode")]
        {
            tracing::info!(
                "QA mode: returning hardcoded QA repos instead of scanning common directories"
            );
            super::qa_repos::get_qa_repos()
        }

        #[cfg(not(feature = "qa-mode"))]
        {
            let search_strings = ["repos", "dev", "work", "code", "projects"];
            let mut paths = Vec::new();
            for root in &self.allowed_roots {
                if !root.exists() || !root.is_dir() {
                    continue;
                }
                paths.push(root.clone());
                for name in &search_strings {
                    let candidate = root.join(name);
                    if candidate.exists() && candidate.is_dir() {
                        paths.push(candidate);
                    }
                }
            }

            let mut resolved_paths = Vec::new();
            for path in paths {
                match self.resolve_path(&path) {
                    Ok(resolved) => {
                        if !resolved_paths.iter().any(|existing| existing == &resolved) {
                            resolved_paths.push(resolved);
                        }
                    }
                    Err(FilesystemError::PathOutsideAllowedRoots) => {}
                    Err(e) => return Err(e),
                }
            }

            if resolved_paths.is_empty() {
                return Err(FilesystemError::PathOutsideAllowedRoots);
            }

            self.list_git_repos_with_timeout(resolved_paths, timeout_ms, hard_timeout_ms, max_depth)
                .await
        }
    }

    #[cfg(not(feature = "qa-mode"))]
    fn list_git_repos_inner(
        path: &[PathBuf],
        max_depth: Option<usize>,
        cancel: Option<&CancellationToken>,
    ) -> Vec<DirectoryEntry> {
        let Some(base_dir) = path.first() else {
            return vec![];
        };
        let skip_dirs = Self::get_directories_to_skip();
        let solodawn_temp_dir = utils::path::get_solodawn_temp_dir();
        let mut walker_builder = WalkBuilder::new(base_dir);
        walker_builder
            .follow_links(false)
            .hidden(true) // true to skip hidden files
            .git_ignore(true)
            .filter_entry({
                let cancel = cancel.cloned();
                move |entry| {
                    if let Some(token) = cancel.as_ref()
                        && token.is_cancelled()
                    {
                        tracing::trace!("Cancellation token triggered");
                        return false;
                    }

                    let path = entry.path();
                    if !path.is_dir() {
                        return false;
                    }

                    // Skip solodawn temp directory and all subdirectories
                    // Normalize to handle macOS /private/var vs /var aliasing
                    if utils::path::normalize_macos_private_alias(path)
                        .starts_with(&solodawn_temp_dir)
                    {
                        return false;
                    }

                    // Skip common non-git folders
                    if let Some(name) = path.file_name().and_then(|n| n.to_str())
                        && skip_dirs.contains(name)
                    {
                        return false;
                    }

                    true
                }
            })
            .max_depth(max_depth)
            .git_exclude(true);
        for p in path.iter().skip(1) {
            walker_builder.add(p);
        }
        let mut seen_dirs = HashSet::new();
        let mut git_repos: Vec<DirectoryEntry> = walker_builder
            .build()
            .filter_map(|entry| {
                let entry = entry.ok()?;
                if seen_dirs.contains(entry.path()) {
                    return None;
                }
                seen_dirs.insert(entry.path().to_owned());
                let name = entry.file_name().to_str()?;
                if !entry.path().join(".git").exists() {
                    return None;
                }
                let last_modified = entry
                    .metadata()
                    .ok()
                    .and_then(|m| m.modified().ok())
                    .map(|t| t.elapsed().unwrap_or_default().as_secs());
                Some(DirectoryEntry {
                    name: name.to_string(),
                    path: entry.into_path(),
                    is_directory: true,
                    is_git_repo: true,
                    last_modified,
                })
            })
            .collect();
        git_repos.sort_by_key(|entry| entry.last_modified.unwrap_or(0));
        git_repos
    }

    fn get_home_directory() -> PathBuf {
        dirs::home_dir()
            .or_else(dirs::desktop_dir)
            .or_else(dirs::document_dir)
            .unwrap_or_else(|| {
                if cfg!(windows) {
                    std::env::var("USERPROFILE")
                        .map_or_else(|_| PathBuf::from("C:\\"), PathBuf::from)
                } else {
                    PathBuf::from("/")
                }
            })
    }

    fn default_browse_root(&self) -> PathBuf {
        let mut preferred_paths: Vec<PathBuf> = Vec::new();

        if let Ok(workspace_root) = utils::env_compat::var_with_compat(
            "SOLODAWN_WORKSPACE_ROOT",
            "GITCORTEX_WORKSPACE_ROOT",
        ) {
            let trimmed = workspace_root.trim();
            if !trimmed.is_empty() {
                preferred_paths.push(PathBuf::from(trimmed));
            }
        }

        preferred_paths.extend(Self::get_env_allowed_roots());

        if let Ok(cwd) = std::env::current_dir() {
            preferred_paths.push(cwd);
        }

        preferred_paths.push(Self::get_home_directory());

        for candidate in preferred_paths {
            if !candidate.exists() || !candidate.is_dir() {
                continue;
            }

            if let Ok(resolved) = self.resolve_path(&candidate) {
                return resolved;
            }
        }

        self.allowed_roots
            .iter()
            .find(|path| path.exists() && path.is_dir())
            .cloned()
            .unwrap_or_else(Self::get_home_directory)
    }

    fn verify_directory(path: &Path) -> Result<(), FilesystemError> {
        if !path.exists() {
            return Err(FilesystemError::DirectoryDoesNotExist);
        }
        if !path.is_dir() {
            return Err(FilesystemError::PathIsNotDirectory);
        }
        Ok(())
    }

    fn resolve_path(&self, path: &Path) -> Result<PathBuf, FilesystemError> {
        let canonical_path = dunce::canonicalize(path)?;
        let allowed = self
            .allowed_roots
            .iter()
            .any(|root| canonical_path.starts_with(root));
        if !allowed {
            return Err(FilesystemError::PathOutsideAllowedRoots);
        }
        Ok(canonical_path)
    }

    /// Async version of list_directory for use in async contexts
    ///
    /// This uses `tokio::task::spawn_blocking` to offload the blocking
    /// filesystem I/O to a dedicated thread pool, preventing the async runtime
    /// from being blocked.
    pub async fn list_directory_async(
        &self,
        path: Option<String>,
    ) -> Result<DirectoryListResponse, FilesystemError> {
        let self_clone = self.clone();
        tokio::task::spawn_blocking(move || self_clone.list_directory(path))
            .await
            .map_err(|e| FilesystemError::Io(std::io::Error::other(e)))?
    }

    pub fn list_directory(
        &self,
        path: Option<String>,
    ) -> Result<DirectoryListResponse, FilesystemError> {
        let path = path.map_or_else(|| self.default_browse_root(), PathBuf::from);
        Self::verify_directory(&path)?;
        let path = self.resolve_path(&path)?;

        let entries = fs::read_dir(&path)?;
        let mut directory_entries = Vec::new();

        for entry in entries.flatten() {
            let path = entry.path();
            let metadata = entry.metadata().ok();
            if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
                // Skip hidden files/directories
                if name.starts_with('.') && name != ".." {
                    continue;
                }

                let is_directory = metadata.is_some_and(|m| m.is_dir());
                let is_git_repo = if is_directory {
                    path.join(".git").exists()
                } else {
                    false
                };

                directory_entries.push(DirectoryEntry {
                    name: name.to_string(),
                    path,
                    is_directory,
                    is_git_repo,
                    last_modified: None,
                });
            }
        }
        // Sort: directories first, then files, both alphabetically
        directory_entries.sort_by(|a, b| match (a.is_directory, b.is_directory) {
            (true, false) => std::cmp::Ordering::Less,
            (false, true) => std::cmp::Ordering::Greater,
            _ => a.name.to_lowercase().cmp(&b.name.to_lowercase()),
        });

        Ok(DirectoryListResponse {
            entries: directory_entries,
            current_path: path.to_string_lossy().to_string(),
        })
    }
}
