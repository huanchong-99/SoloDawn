//! 原子写入工具
//!
//! 使用临时文件 + 重命名的方式实现原子写入，
//! 防止写入过程中断导致配置文件损坏。

use std::{
    path::{Path, PathBuf},
    sync::atomic::{AtomicU64, Ordering},
    time::{SystemTime, UNIX_EPOCH},
};

use crate::{
    config_path::ensure_parent_dir_exists,
    error::{CCSwitchError, Result},
};

static TEMP_FILE_COUNTER: AtomicU64 = AtomicU64::new(0);

fn build_temp_path(path: &Path) -> PathBuf {
    let parent = path.parent().unwrap_or(Path::new("."));
    let file_name = path.file_name().map_or_else(
        || "config".to_string(),
        |name| name.to_string_lossy().to_string(),
    );
    let nonce = TEMP_FILE_COUNTER.fetch_add(1, Ordering::Relaxed);
    let timestamp_ns = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map_or(0, |duration| duration.as_nanos());

    parent.join(format!(
        ".{file_name}.tmp.{}.{}.{}",
        std::process::id(),
        timestamp_ns,
        nonce
    ))
}

async fn create_unique_temp_file(path: &Path) -> Result<(PathBuf, tokio::fs::File)> {
    const MAX_ATTEMPTS: usize = 16;

    for _ in 0..MAX_ATTEMPTS {
        let temp_path = build_temp_path(path);
        match tokio::fs::OpenOptions::new()
            .create_new(true)
            .write(true)
            .open(&temp_path)
            .await
        {
            Ok(file) => return Ok((temp_path, file)),
            Err(error) if error.kind() == std::io::ErrorKind::AlreadyExists => {}
            Err(error) => {
                return Err(CCSwitchError::AtomicWriteError(format!(
                    "Failed to create temp file for {}: {}",
                    path.display(),
                    error
                )));
            }
        }
    }

    Err(CCSwitchError::AtomicWriteError(format!(
        "Failed to create unique temp file for {} after {MAX_ATTEMPTS} attempts",
        path.display()
    )))
}

/// 原子写入文件
///
/// 流程：
/// 1. 写入临时文件
/// 2. 同步到磁盘
/// 3. 重命名为目标文件（原子操作）
///
/// # 参数
/// - `path`: 目标文件路径
/// - `data`: 要写入的数据
///
/// # 示例
/// ```rust,ignore
/// atomic_write(&path, b"content").await?;
/// ```
pub async fn atomic_write(path: &Path, data: &[u8]) -> Result<()> {
    use tokio::io::AsyncWriteExt;

    ensure_parent_dir_exists(path).await?;

    let (temp_path, mut file) = create_unique_temp_file(path).await?;
    file.write_all(data).await?;
    file.sync_all().await?;
    drop(file);

    #[cfg(not(windows))]
    {
        tokio::fs::rename(&temp_path, path).await.map_err(|error| {
            let _ = std::fs::remove_file(&temp_path);
            CCSwitchError::AtomicWriteError(format!(
                "Failed to rename {} to {}: {}",
                temp_path.display(),
                path.display(),
                error
            ))
        })?;
    }

    #[cfg(windows)]
    {
        if let Err(first_error) = tokio::fs::rename(&temp_path, path).await {
            if matches!(
                first_error.kind(),
                std::io::ErrorKind::AlreadyExists | std::io::ErrorKind::PermissionDenied
            ) {
                if let Err(remove_error) = tokio::fs::remove_file(path).await {
                    if remove_error.kind() != std::io::ErrorKind::NotFound {
                        let _ = std::fs::remove_file(&temp_path);
                        return Err(CCSwitchError::AtomicWriteError(format!(
                            "Failed to remove existing target {} before rename: {}",
                            path.display(),
                            remove_error
                        )));
                    }
                }

                tokio::fs::rename(&temp_path, path)
                    .await
                    .map_err(|rename_error| {
                        let _ = std::fs::remove_file(&temp_path);
                        CCSwitchError::AtomicWriteError(format!(
                            "Failed to rename {} to {} after removing existing target: {}",
                            temp_path.display(),
                            path.display(),
                            rename_error
                        ))
                    })?;
            } else {
                let _ = std::fs::remove_file(&temp_path);
                return Err(CCSwitchError::AtomicWriteError(format!(
                    "Failed to rename {} to {}: {}",
                    temp_path.display(),
                    path.display(),
                    first_error
                )));
            }
        }
    }

    Ok(())
}

/// 原子写入 JSON 文件
pub async fn atomic_write_json<T: serde::Serialize>(path: &Path, value: &T) -> Result<()> {
    let json = serde_json::to_string_pretty(value)?;
    atomic_write(path, json.as_bytes()).await
}

/// 原子写入文本文件
pub async fn atomic_write_text(path: &Path, text: &str) -> Result<()> {
    atomic_write(path, text.as_bytes()).await
}

#[cfg(test)]
mod tests {
    use tempfile::tempdir;

    use super::*;

    #[tokio::test]
    async fn test_atomic_write() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");

        atomic_write(&path, b"hello world").await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        assert_eq!(content, "hello world");
    }

    #[tokio::test]
    async fn test_atomic_write_overwrites_existing_file() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("existing.txt");

        tokio::fs::write(&path, "old").await.unwrap();
        atomic_write(&path, b"new").await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        assert_eq!(content, "new");
    }

    #[tokio::test]
    async fn test_atomic_write_json() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.json");

        let data = serde_json::json!({"key": "value"});
        atomic_write_json(&path, &data).await.unwrap();

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        let parsed: serde_json::Value = serde_json::from_str(&content).unwrap();
        assert_eq!(parsed["key"], "value");
    }

    #[cfg(not(windows))]
    #[tokio::test]
    async fn test_atomic_write_concurrent_calls_do_not_conflict() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("concurrent.txt");

        let tasks: Vec<_> = (0..16)
            .map(|index| {
                let path = path.clone();
                tokio::spawn(async move {
                    let content = format!("value-{index}");
                    atomic_write(&path, content.as_bytes()).await
                })
            })
            .collect();

        for task in tasks {
            task.await.unwrap().unwrap();
        }

        let content = tokio::fs::read_to_string(&path).await.unwrap();
        assert!(content.starts_with("value-"));
    }
}
