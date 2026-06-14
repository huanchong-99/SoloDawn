// Test for terminal timeout functionality
// This test ensures that terminal operations properly handle timeout scenarios

use std::time::Duration;

use services::services::terminal::process::{ProcessManager, SpawnCommand};
#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_terminal_timeout_cleanup() {
        // Test that ProcessManager properly tracks and cleans up processes
        let manager = ProcessManager::new();
        let temp_dir = tempfile::tempdir().unwrap();

        #[cfg(unix)]
        let shell = "sh";
        #[cfg(windows)]
        let shell = "cmd.exe";

        // Spawn a process, kill by PID, then verify cleanup removes dead tracking.
        let handle = manager
            .spawn_pty_with_config("test-terminal", &SpawnCommand::new(shell, temp_dir.path()), 80, 24)
            .await
            .expect("Spawn should succeed");

        // Verify process is tracked
        let running_before = manager.list_running().await;
        assert_eq!(running_before.len(), 1, "Process should be tracked");

        manager
            .kill(handle.pid)
            .await
            .expect("Process kill by PID should succeed");

        // Wait for process to exit after SIGTERM/taskkill
        tokio::time::sleep(Duration::from_millis(200)).await;

        // Cleanup should remove dead processes
        manager.cleanup().await;

        let running_after = manager.list_running().await;
        assert_eq!(running_after.len(), 0, "Dead process should be removed");
    }

    #[tokio::test]
    async fn test_terminal_is_running_detection() {
        // Test that is_running correctly detects active and dead processes
        let manager = ProcessManager::new();
        let temp_dir = tempfile::tempdir().unwrap();

        #[cfg(unix)]
        let shell = "sh";
        #[cfg(windows)]
        let shell = "cmd.exe";

        // Spawn process
        let _handle = manager
            .spawn_pty_with_config("long-running", &SpawnCommand::new(shell, temp_dir.path()), 80, 24)
            .await;

        // Should be running
        assert!(
            manager.is_running("long-running").await,
            "Process should be running"
        );
        assert!(
            !manager.is_running("non-existent").await,
            "Non-existent process should not be running"
        );

        manager
            .kill_terminal("long-running")
            .await
            .expect("Terminal kill should succeed");
    }

    #[tokio::test]
    async fn test_multiple_process_cleanup() {
        // Test cleanup of multiple processes
        let manager = ProcessManager::new();
        let temp_dir = tempfile::tempdir().unwrap();

        #[cfg(unix)]
        let shell = "sh";
        #[cfg(windows)]
        let shell = "cmd.exe";

        let mut pids = Vec::new();
        // Spawn multiple processes
        for i in 0..3 {
            let handle = manager
                .spawn_pty_with_config(&format!("terminal-{}", i), &SpawnCommand::new(shell, temp_dir.path()), 80, 24)
                .await;
            assert!(handle.is_ok(), "Spawn {} should succeed", i);
            pids.push(handle.unwrap().pid);
        }

        assert_eq!(
            manager.list_running().await.len(),
            3,
            "All processes should be tracked"
        );

        for pid in pids {
            manager
                .kill(pid)
                .await
                .expect("Process kill by PID should succeed");
        }

        // Wait for processes to exit after SIGTERM/taskkill
        tokio::time::sleep(Duration::from_millis(200)).await;

        // Cleanup should remove all dead processes
        manager.cleanup().await;

        assert_eq!(
            manager.list_running().await.len(),
            0,
            "All dead processes should be removed"
        );
    }
}
