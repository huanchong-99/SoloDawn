//! Analysis utilities
//!
//! Shared infrastructure for built-in quality analysis.

pub mod coverage_parser;

use std::path::Path;

/// Check if a file path is a Rust source file
pub fn is_rust_file(path: &Path) -> bool {
    path.extension().is_some_and(|ext| ext == "rs")
}

/// Check if a file path is a TypeScript/JavaScript source file
pub fn is_ts_file(path: &Path) -> bool {
    path.extension()
        .is_some_and(|ext| ext == "ts" || ext == "tsx" || ext == "js" || ext == "jsx")
}

/// Check if a file path should be excluded from analysis.
///
/// Matches on path *components* (platform-independent) so excludes apply on
/// both POSIX (`/`) and Windows (`\`). The previous `contains("dist/")` form
/// matched nothing on Windows — where paths use `\` (e.g. `...\dist\assets\…`)
/// — silently leaking build artifacts (minified bundles under `dist/`,
/// `build/`, `target/`, coverage output, …) into security/quality scans and
/// producing false blocker issues that final-repair rounds could never clear.
pub fn is_excluded(path: &Path) -> bool {
    path.components().any(|c| {
        matches!(
            c.as_os_str().to_str(),
            Some(
                "node_modules" | "target" | "dist" | ".git" | "vendor" | ".next" | "build"
                    | "coverage" | ".turbo" | "out" | ".cache" | "__pycache__"
            )
        )
    })
}

/// Collect all source files from a directory recursively
pub fn collect_files(root: &Path, filter: fn(&Path) -> bool) -> Vec<std::path::PathBuf> {
    let mut files = Vec::new();
    collect_files_recursive(root, filter, &mut files);
    files
}

fn collect_files_recursive(
    dir: &Path,
    filter: fn(&Path) -> bool,
    files: &mut Vec<std::path::PathBuf>,
) {
    let entries = match std::fs::read_dir(dir) {
        Ok(e) => e,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        if is_excluded(&path) {
            continue;
        }
        if path.is_dir() {
            collect_files_recursive(&path, filter, files);
        } else if filter(&path) {
            files.push(path);
        }
    }
}
