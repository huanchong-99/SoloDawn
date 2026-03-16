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
    path.extension().is_some_and(|ext| {
        ext == "ts" || ext == "tsx" || ext == "js" || ext == "jsx"
    })
}

/// Check if a file path should be excluded from analysis
pub fn is_excluded(path: &Path) -> bool {
    let path_str = path.to_string_lossy();
    path_str.contains("node_modules")
        || path_str.contains("target/")
        || path_str.contains("dist/")
        || path_str.contains(".git/")
        || path_str.contains("vendor/")
        || path_str.contains(".next/")
        || path_str.contains("build/")
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
