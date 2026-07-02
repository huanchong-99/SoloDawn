use command_group::AsyncGroupChild;
#[cfg(unix)]
use nix::{
    sys::signal::{Signal, killpg},
    unistd::{Pid, getpgid},
};
use services::services::container::ContainerError;
#[cfg(unix)]
use tokio::time::Duration;

pub async fn kill_process_group(child: &mut AsyncGroupChild) -> Result<(), ContainerError> {
    // hit the whole process group, not just the leader
    #[cfg(unix)]
    {
        if let Some(pid) = child.inner().id() {
            #[allow(clippy::cast_possible_wrap)]
            match getpgid(Some(Pid::from_raw(pid as i32))) {
                Ok(pgid) => {
                    for sig in [Signal::SIGINT, Signal::SIGTERM, Signal::SIGKILL] {
                        if let Err(e) = killpg(pgid, sig) {
                            tracing::warn!(
                                "Failed to send signal {:?} to process group {}: {}",
                                sig,
                                pgid,
                                e
                            );
                        }
                        tokio::time::sleep(Duration::from_secs(2)).await;
                        if child
                            .inner()
                            .try_wait()
                            .map_err(ContainerError::Io)?
                            .is_some()
                        {
                            break;
                        }
                    }
                }
                Err(e) => {
                    // Group lookup can fail (e.g. ESRCH) when the child has already
                    // exited; do not hard-fail — fall through to child.kill()/wait()
                    // below so the child is always reaped.
                    tracing::warn!("getpgid failed for pid {}: {}", pid, e);
                }
            }
        }
    }

    let _ = child.kill().await;
    let _ = child.wait().await;
    Ok(())
}
