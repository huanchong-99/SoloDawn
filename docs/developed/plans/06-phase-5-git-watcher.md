# Phase 5: Git 事件驱动系统

> **状态:** ✅ 完成
> **进度追踪:** 查看 `TODO.md`
> **前置条件:** Phase 4 完成
> **完成日期:** 2024-01-18

## 概述

实现 Git 事件驱动系统，监听提交事件并触发 Orchestrator 响应。

---

## Phase 5: Git 事件驱动系统

### Task 5.1: 实现 GitWatcher

**状态:** ✅ 完成

**前置条件:**
- Phase 4 已完成

**目标:**
实现 Git 仓库监控，监听 .git/refs/heads 目录变化，检测新提交。

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/mod.rs`
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/watcher.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/mod.rs`

---

**Step 5.1.1: 创建 git_watcher/mod.rs**

```rust
//! Git 事件监控模块

pub mod watcher;
pub mod parser;

pub use watcher::GitWatcher;
pub use parser::CommitParser;
```

---

**Step 5.1.2: 创建 watcher.rs**

```rust
//! Git 仓库监控

use std::path::PathBuf;
use std::sync::Arc;
use tokio::sync::mpsc;
use notify::{Watcher, RecursiveMode, Event, EventKind};
use crate::services::orchestrator::{MessageBus, BusMessage};

/// Git 事件
#[derive(Debug, Clone)]
pub struct GitEvent {
    pub commit_hash: String,
    pub branch: String,
    pub message: String,
    pub author: String,
    pub timestamp: String,
}

/// Git 监控器
pub struct GitWatcher {
    repo_path: PathBuf,
    message_bus: Arc<MessageBus>,
    workflow_id: String,
}

impl GitWatcher {
    pub fn new(repo_path: PathBuf, message_bus: Arc<MessageBus>, workflow_id: String) -> Self {
        Self { repo_path, message_bus, workflow_id }
    }

    /// 启动监控
    pub async fn start(&self) -> anyhow::Result<()> {
        let refs_path = self.repo_path.join(".git/refs/heads");

        if !refs_path.exists() {
            return Err(anyhow::anyhow!("Not a git repository: {}", self.repo_path.display()));
        }

        let (tx, mut rx) = mpsc::channel(100);
        let message_bus = self.message_bus.clone();
        let workflow_id = self.workflow_id.clone();
        let repo_path = self.repo_path.clone();

        // 启动文件监控
        let mut watcher = notify::recommended_watcher(move |res: Result<Event, _>| {
            if let Ok(event) = res {
                if matches!(event.kind, EventKind::Create(_) | EventKind::Modify(_)) {
                    let _ = tx.blocking_send(event);
                }
            }
        })?;

        watcher.watch(&refs_path, RecursiveMode::Recursive)?;

        tracing::info!("Git watcher started for {}", refs_path.display());

        // 事件处理循环
        tokio::spawn(async move {
            while let Some(event) = rx.recv().await {
                for path in event.paths {
                    if let Some(branch) = Self::extract_branch_name(&path) {
                        if let Ok(git_event) = Self::get_latest_commit(&repo_path, &branch).await {
                            message_bus.publish_git_event(
                                &workflow_id,
                                &git_event.commit_hash,
                                &git_event.branch,
                                &git_event.message,
                            ).await;
                        }
                    }
                }
            }
        });

        // 保持 watcher 存活
        std::mem::forget(watcher);

        Ok(())
    }

    /// 从路径提取分支名
    fn extract_branch_name(path: &std::path::Path) -> Option<String> {
        path.file_name()
            .and_then(|n| n.to_str())
            .map(|s| s.to_string())
    }

    /// 获取最新提交信息
    async fn get_latest_commit(repo_path: &PathBuf, branch: &str) -> anyhow::Result<GitEvent> {
        use tokio::process::Command;

        let output = Command::new("git")
            .current_dir(repo_path)
            .args(["log", "-1", "--format=%H|%s|%an|%aI", branch])
            .output()
            .await?;

        if !output.status.success() {
            return Err(anyhow::anyhow!("git log failed"));
        }

        let line = String::from_utf8_lossy(&output.stdout);
        let parts: Vec<&str> = line.trim().split('|').collect();

        if parts.len() >= 4 {
            Ok(GitEvent {
                commit_hash: parts[0].to_string(),
                message: parts[1].to_string(),
                author: parts[2].to_string(),
                timestamp: parts[3].to_string(),
                branch: branch.to_string(),
            })
        } else {
            Err(anyhow::anyhow!("Invalid git log output"))
        }
    }
}
```

---

**交付物:** `git_watcher/mod.rs`, `git_watcher/watcher.rs`

---

### Task 5.2: 实现提交信息解析器

**状态:** ✅ 完成

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/parser.rs`

---

**Step 5.2.1: 创建 parser.rs**

```rust
//! Git 提交信息解析器
//!
//! 解析强制 Git 提交规范中的 METADATA 部分。

use serde::{Deserialize, Serialize};
use crate::services::orchestrator::types::CommitMetadata;

/// 提交解析器
pub struct CommitParser;

impl CommitParser {
    /// 解析提交信息
    ///
    /// 格式:
    /// ```text
    /// [Terminal:xxx] [Status:xxx] summary
    ///
    /// {body}
    ///
    /// ---METADATA---
    /// workflow_id: xxx
    /// task_id: xxx
    /// terminal_id: xxx
    /// ...
    /// ```
    pub fn parse(message: &str) -> Option<ParsedCommit> {
        let lines: Vec<&str> = message.lines().collect();

        if lines.is_empty() {
            return None;
        }

        // 解析标题行
        let title = lines[0];
        let (terminal_id, status, summary) = Self::parse_title(title)?;

        // 查找 METADATA 部分
        let metadata_start = lines.iter().position(|l| l.trim() == "---METADATA---")?;
        let metadata_lines = &lines[metadata_start + 1..];

        // 解析元数据
        let metadata = Self::parse_metadata(metadata_lines)?;

        Some(ParsedCommit {
            terminal_id,
            status,
            summary: summary.to_string(),
            metadata,
        })
    }

    /// 解析标题行
    fn parse_title(title: &str) -> Option<(String, String, &str)> {
        // [Terminal:xxx] [Status:xxx] summary
        let re = regex::Regex::new(r"\[Terminal:([^\]]+)\]\s*\[Status:([^\]]+)\]\s*(.+)").ok()?;
        let caps = re.captures(title)?;

        Some((
            caps.get(1)?.as_str().to_string(),
            caps.get(2)?.as_str().to_string(),
            caps.get(3)?.as_str(),
        ))
    }

    /// 解析元数据
    fn parse_metadata(lines: &[&str]) -> Option<CommitMetadata> {
        let mut map = std::collections::HashMap::new();

        for line in lines {
            if let Some(pos) = line.find(':') {
                let key = line[..pos].trim().to_string();
                let value = line[pos + 1..].trim().to_string();
                map.insert(key, value);
            }
        }

        Some(CommitMetadata {
            workflow_id: map.get("workflow_id")?.clone(),
            task_id: map.get("task_id")?.clone(),
            terminal_id: map.get("terminal_id")?.clone(),
            terminal_order: map.get("terminal_order").and_then(|s| s.parse().ok()).unwrap_or(0),
            cli: map.get("cli").cloned().unwrap_or_default(),
            model: map.get("model").cloned().unwrap_or_default(),
            status: map.get("status").cloned().unwrap_or_default(),
            severity: map.get("severity").cloned(),
            reviewed_terminal: map.get("reviewed_terminal").cloned(),
            issues: None,
            next_action: map.get("next_action").cloned().unwrap_or_else(|| "continue".to_string()),
        })
    }

    /// 生成提交信息
    pub fn generate(
        terminal_id: &str,
        status: &str,
        summary: &str,
        metadata: &CommitMetadata,
    ) -> String {
        format!(
            "[Terminal:{}] [Status:{}] {}\n\n---METADATA---\nworkflow_id: {}\ntask_id: {}\nterminal_id: {}\nterminal_order: {}\ncli: {}\nmodel: {}\nstatus: {}\nnext_action: {}",
            terminal_id, status, summary,
            metadata.workflow_id, metadata.task_id, metadata.terminal_id,
            metadata.terminal_order, metadata.cli, metadata.model,
            metadata.status, metadata.next_action
        )
    }
}

/// 解析后的提交
#[derive(Debug, Clone)]
pub struct ParsedCommit {
    pub terminal_id: String,
    pub status: String,
    pub summary: String,
    pub metadata: CommitMetadata,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_parse_commit() {
        let message = r#"[Terminal:t1] [Status:completed] Implement login feature

Added user authentication

---METADATA---
workflow_id: wf1
task_id: task1
terminal_id: t1
terminal_order: 0
cli: claude-code
model: sonnet
status: completed
next_action: continue"#;

        let parsed = CommitParser::parse(message).unwrap();
        assert_eq!(parsed.terminal_id, "t1");
        assert_eq!(parsed.status, "completed");
        assert_eq!(parsed.metadata.workflow_id, "wf1");
    }
}
```

---

**交付物:** `git_watcher/parser.rs`

---

### Task 5.3: 连接 Git 事件到 Orchestrator

**状态:** ✅ 完成

**涉及文件:**
- 创建: `vibe-kanban-main/crates/services/src/services/git_watcher/handler.rs`
- 修改: `vibe-kanban-main/crates/services/src/services/git_watcher/mod.rs`

---

**Step 5.3.1: 创建 handler.rs**

```rust
//! Git 事件处理器

use std::sync::Arc;
use db::DBService;
use db::models::{terminal_dao, git_event_dao};
use crate::services::orchestrator::{MessageBus, BusMessage, TerminalCompletionEvent, TerminalCompletionStatus};
use super::parser::{CommitParser, ParsedCommit};
use super::watcher::GitEvent;

/// Git 事件处理器
pub struct GitEventHandler {
    db: Arc<DBService>,
    message_bus: Arc<MessageBus>,
}

impl GitEventHandler {
    pub fn new(db: Arc<DBService>, message_bus: Arc<MessageBus>) -> Self {
        Self { db, message_bus }
    }

    /// 处理 Git 事件
    pub async fn handle(&self, event: GitEvent) -> anyhow::Result<()> {
        tracing::info!("Handling git event: {} on {}", event.commit_hash, event.branch);

        // 解析提交信息
        let parsed = match CommitParser::parse(&event.message) {
            Some(p) => p,
            None => {
                tracing::debug!("Commit message not in expected format, skipping");
                return Ok(());
            }
        };

        // 保存到数据库
        let event_id = terminal_dao::create_git_event(
            &self.db.pool,
            &parsed.metadata.workflow_id,
            Some(&parsed.terminal_id),
            &event.commit_hash,
            &event.branch,
            &event.message,
            Some(&serde_json::to_string(&parsed.metadata)?),
        ).await?;

        // 更新终端状态
        terminal_dao::update_terminal_last_commit(
            &self.db.pool,
            &parsed.terminal_id,
            &event.commit_hash,
            &event.message,
        ).await?;

        // 转换为终端完成事件
        let completion_status = match parsed.status.as_str() {
            "completed" => TerminalCompletionStatus::Completed,
            "review_pass" => TerminalCompletionStatus::ReviewPass,
            "review_reject" => TerminalCompletionStatus::ReviewReject,
            "failed" => TerminalCompletionStatus::Failed,
            _ => TerminalCompletionStatus::Completed,
        };

        let completion_event = TerminalCompletionEvent {
            terminal_id: parsed.terminal_id.clone(),
            task_id: parsed.metadata.task_id.clone(),
            workflow_id: parsed.metadata.workflow_id.clone(),
            status: completion_status,
            commit_hash: Some(event.commit_hash),
            commit_message: Some(parsed.summary),
            metadata: Some(parsed.metadata),
        };

        // 发布到消息总线
        self.message_bus.publish_terminal_completed(completion_event).await;

        // 更新事件处理状态
        terminal_dao::update_git_event_status(&self.db.pool, &event_id, "processed", None).await?;

        Ok(())
    }
}
```

---

**Step 5.3.2: 更新 git_watcher/mod.rs**

```rust
pub mod watcher;
pub mod parser;
pub mod handler;

pub use watcher::{GitWatcher, GitEvent};
pub use parser::{CommitParser, ParsedCommit};
pub use handler::GitEventHandler;
```

---

**交付物:** `git_watcher/handler.rs`

**验收标准:**
1. 编译通过
2. Git 事件可以正确解析并转发到 Orchestrator

---

### Phase 5 单元测试用例

> 在 `vibe-kanban-main/crates/services/src/services/git_watcher/tests.rs` 创建以下测试

```rust
//! Git Watcher 单元测试
//!
//! 测试 Git 监听、提交解析、事件处理等功能

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::TempDir;
    use tokio::sync::mpsc;

    // =========================================================================
    // 测试 1: 解析标准提交消息
    // =========================================================================
    #[test]
    fn test_parse_standard_commit() {
        let commit_msg = "feat: implement user login\n\nAdded JWT authentication";
        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.commit_type, Some("feat".to_string()));
        assert_eq!(result.subject, "implement user login");
        assert!(result.body.is_some());
    }

    // =========================================================================
    // 测试 2: 解析带状态标记的提交消息
    // =========================================================================
    #[test]
    fn test_parse_commit_with_status() {
        let commit_msg = r#"fix: resolve database connection issue

Status: COMPLETED
Terminal: T1
NextAction: CONTINUE

Fixed connection pooling timeout."#;

        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.status, Some(TaskStatus::Completed));
        assert_eq!(result.terminal_id, Some("T1".to_string()));
        assert_eq!(result.next_action, Some(NextAction::Continue));
    }

    // =========================================================================
    // 测试 3: 解析带帮助请求的提交消息
    // =========================================================================
    #[test]
    fn test_parse_commit_with_help_request() {
        let commit_msg = r#"wip: stuck on API integration

Status: NEED_HELP
Terminal: T2
HelpType: TECHNICAL
HelpContext: Cannot figure out how to handle rate limiting

Tried exponential backoff but still hitting limits."#;

        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.status, Some(TaskStatus::NeedHelp));
        assert_eq!(result.help_type, Some(HelpType::Technical));
        assert!(result.help_context.is_some());
    }

    // =========================================================================
    // 测试 4: Git refs 文件变更检测
    // =========================================================================
    #[tokio::test]
    async fn test_detect_ref_change() {
        let temp_dir = TempDir::new().unwrap();
        let git_dir = temp_dir.path().join(".git");
        let refs_dir = git_dir.join("refs/heads");
        fs::create_dir_all(&refs_dir).unwrap();

        // 创建初始 ref
        let main_ref = refs_dir.join("main");
        fs::write(&main_ref, "abc123").unwrap();

        let (tx, mut rx) = mpsc::channel(10);
        let watcher = GitWatcher::new(temp_dir.path().to_path_buf(), tx);

        // 启动监听（后台）
        let watcher_handle = tokio::spawn(async move {
            watcher.start().await
        });

        // 模拟新提交
        tokio::time::sleep(tokio::time::Duration::from_millis(100)).await;
        fs::write(&main_ref, "def456").unwrap();

        // 等待事件
        let event = tokio::time::timeout(
            tokio::time::Duration::from_secs(2),
            rx.recv()
        ).await;

        assert!(event.is_ok());
        let event = event.unwrap().unwrap();
        assert_eq!(event.branch, "main");
        assert_eq!(event.new_commit, "def456");

        watcher_handle.abort();
    }

    // =========================================================================
    // 测试 5: 事件处理器路由
    // =========================================================================
    #[tokio::test]
    async fn test_event_handler_routing() {
        let (orchestrator_tx, mut orchestrator_rx) = mpsc::channel(10);
        let handler = GitEventHandler::new(orchestrator_tx);

        let event = GitEvent {
            branch: "feature/login".to_string(),
            old_commit: Some("abc123".to_string()),
            new_commit: "def456".to_string(),
            timestamp: chrono::Utc::now(),
        };

        handler.handle(event).await.unwrap();

        // 验证消息被发送到 Orchestrator
        let msg = orchestrator_rx.recv().await.unwrap();
        assert!(matches!(msg, OrchestratorMessage::GitCommitDetected { .. }));
    }

    // =========================================================================
    // 测试 6: 多分支并发监听
    // =========================================================================
    #[tokio::test]
    async fn test_multi_branch_watch() {
        let temp_dir = TempDir::new().unwrap();
        let git_dir = temp_dir.path().join(".git");
        let refs_dir = git_dir.join("refs/heads");
        fs::create_dir_all(&refs_dir).unwrap();

        // 创建多个分支 refs
        fs::write(refs_dir.join("main"), "commit1").unwrap();
        fs::write(refs_dir.join("feature-a"), "commit2").unwrap();
        fs::write(refs_dir.join("feature-b"), "commit3").unwrap();

        let (tx, mut rx) = mpsc::channel(10);
        let watcher = GitWatcher::new(temp_dir.path().to_path_buf(), tx);

        // 验证所有分支都被监听
        let branches = watcher.get_watched_branches();
        assert!(branches.contains(&"main".to_string()));
        assert!(branches.contains(&"feature-a".to_string()));
        assert!(branches.contains(&"feature-b".to_string()));
    }

    // =========================================================================
    // 测试 7: 解析无状态标记的普通提交
    // =========================================================================
    #[test]
    fn test_parse_plain_commit() {
        let commit_msg = "docs: update README with installation instructions";
        let result = CommitParser::parse(commit_msg);

        assert_eq!(result.commit_type, Some("docs".to_string()));
        assert_eq!(result.subject, "update README with installation instructions");
        assert!(result.status.is_none());
        assert!(result.terminal_id.is_none());
    }

    // =========================================================================
    // 测试 8: 事件去重（防止重复触发）
    // =========================================================================
    #[tokio::test]
    async fn test_event_deduplication() {
        let (tx, mut rx) = mpsc::channel(10);
        let handler = GitEventHandler::new(tx);

        let event = GitEvent {
            branch: "main".to_string(),
            old_commit: Some("abc".to_string()),
            new_commit: "def".to_string(),
            timestamp: chrono::Utc::now(),
        };

        // 发送相同事件两次
        handler.handle(event.clone()).await.unwrap();
        handler.handle(event.clone()).await.unwrap();

        // 应该只收到一个事件（去重）
        let first = rx.recv().await;
        assert!(first.is_some());

        // 第二个应该被过滤
        let second = tokio::time::timeout(
            tokio::time::Duration::from_millis(100),
            rx.recv()
        ).await;
        assert!(second.is_err()); // 超时，说明没有第二个事件
    }
}
```

**运行测试:**
```bash
cd F:\Project\SoloDawn\vibe-kanban-main
cargo test -p services git_watcher -- --nocapture
```

---

## Phase 5 完成检查清单

- [x] Task 5.1: GitWatcher 实现完成
- [x] Task 5.2: CommitParser 实现完成
- [x] Task 5.3: GitEventHandler 实现完成

---

## 实现完成

**完成日期:** 2024-01-18

**提交数量:** 10 commits

**测试覆盖:** 12 单元测试，全部通过

**实现的模块:**
1. `git_watcher/mod.rs` - 模块声明
2. `git_watcher/parser.rs` - CommitParser 解析器
3. `git_watcher/watcher.rs` - GitWatcher 文件监控
4. `git_watcher/handler.rs` - GitEventHandler 事件处理
5. `git_watcher/tests.rs` - 完整测试套件

**集成点:**
- MessageBus: 新增 `publish_git_event()` 方法
- Database: 使用现有 `GitEvent` 和 `Terminal` 模型
- Orchestrator: 发布 `TerminalCompletionEvent`

**验收标准:**
- [x] 编译通过
- [x] Git 事件可以正确解析
- [x] 事件转发到 Orchestrator
- [x] 数据库记录正确创建
- [x] 终端状态正确更新
- [x] 所有测试通过
