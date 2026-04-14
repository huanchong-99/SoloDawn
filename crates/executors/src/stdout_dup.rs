//! Cross-platform stdout duplication utility for child processes
//!
//! Provides a single function to duplicate a child process's stdout stream.
//! Supports Unix and Windows platforms.

#[cfg(unix)]
use std::os::unix::io::{FromRawFd, IntoRawFd, OwnedFd};
#[cfg(windows)]
use std::os::windows::io::{FromRawHandle, IntoRawHandle, OwnedHandle};

use command_group::AsyncGroupChild;
use futures::{StreamExt, stream::BoxStream};
use tokio::io::{AsyncWrite, AsyncWriteExt};
use tokio_stream::wrappers::ReceiverStream;
use tokio_util::io::ReaderStream;

use crate::executors::ExecutorError;

/// Bound for duplicate-stdout and injector channels (W2-30-06).
const STDOUT_DUP_CHANNEL_BOUND: usize = 512;

/// Duplicate stdout from AsyncGroupChild.
///
/// Creates a stream that mirrors stdout of child process without consuming it.
///
/// # Returns
/// A stream of `io::Result<String>` that receives a copy of all stdout data.
pub fn duplicate_stdout(
    child: &mut AsyncGroupChild,
) -> Result<BoxStream<'static, std::io::Result<String>>, ExecutorError> {
    // The implementation strategy is:
    // 1. create a new file descriptor.
    // 2. read the original stdout file descriptor.
    // 3. write the data to both the new file descriptor and a duplicate stream.

    // Take the original stdout
    let original_stdout = child.inner().stdout.take().ok_or_else(|| {
        ExecutorError::Io(std::io::Error::new(
            std::io::ErrorKind::NotFound,
            "Child process has no stdout",
        ))
    })?;

    // Create a new file descriptor in a cross-platform way (using os_pipe crate)
    let (pipe_reader, pipe_writer) = os_pipe::pipe().map_err(|e| {
        ExecutorError::Io(std::io::Error::other(format!("Failed to create pipe: {e}")))
    })?;
    // Use fd as new child stdout
    child.inner().stdout = Some(wrap_fd_as_child_stdout(pipe_reader)?);

    // Obtain writer from fd
    let mut fd_writer = wrap_fd_as_tokio_writer(pipe_writer);

    // Create the duplicate stdout stream.
    // W2-30-06: bounded channel + `send().await` backpressure so a slow
    // consumer cannot cause unbounded memory growth here.
    let (dup_writer, dup_reader) = tokio::sync::mpsc::channel::<std::io::Result<String>>(
        STDOUT_DUP_CHANNEL_BOUND,
    );

    // Read original stdout and write to both new ChildStdout and duplicate stream
    tokio::spawn(async move {
        let mut stdout_stream = ReaderStream::new(original_stdout);

        while let Some(res) = stdout_stream.next().await {
            match res {
                Ok(data) => {
                    let _ = fd_writer.write_all(&data).await;

                    let string_chunk = String::from_utf8_lossy(&data).into_owned();
                    // W2-30-06: async context -> apply backpressure via send().await.
                    if dup_writer.send(Ok(string_chunk)).await.is_err() {
                        // Receiver dropped; stop forwarding.
                        break;
                    }
                }
                Err(err) => {
                    tracing::error!("Error reading from child stdout: {}", err);
                    // W2-30-06: async context -> apply backpressure via send().await.
                    if dup_writer.send(Err(err)).await.is_err() {
                        break;
                    }
                }
            }
        }
    });

    // Return the channel receiver as a boxed stream
    Ok(Box::pin(ReceiverStream::new(dup_reader)))
}

/// Handle to append additional lines into the child's stdout stream.
#[derive(Clone)]
pub struct StdoutAppender {
    tx: tokio::sync::mpsc::Sender<String>,
}

impl StdoutAppender {
    pub fn append_line<S: Into<String>>(&self, line: S) {
        // Best-effort; ignore send errors if writer task ended
        let mut line = line.into();
        while line.ends_with('\n') || line.ends_with('\r') {
            line.pop();
        }
        // W2-30-06: sync context -> use try_send and warn on full/closed
        // instead of an unbounded channel that could grow without limit.
        match self.tx.try_send(line) {
            Ok(()) => {}
            Err(tokio::sync::mpsc::error::TrySendError::Full(_)) => {
                tracing::warn!("stdout_dup: injector channel full; dropping line");
            }
            Err(tokio::sync::mpsc::error::TrySendError::Closed(_)) => {
                // Writer task has ended; silently drop.
            }
        }
    }
}

/// Tee the child's stdout and provide both a duplicate stream and an appender to write additional
/// lines into the child's stdout. This keeps the original stdout functional and mirrors output to
/// the returned duplicate stream.
pub fn tee_stdout_with_appender(
    child: &mut AsyncGroupChild,
) -> Result<(BoxStream<'static, std::io::Result<String>>, StdoutAppender), ExecutorError> {
    // Take original stdout
    let original_stdout = child.inner().stdout.take().ok_or_else(|| {
        ExecutorError::Io(std::io::Error::new(
            std::io::ErrorKind::NotFound,
            "Child process has no stdout",
        ))
    })?;

    // Create replacement pipe and set as new child stdout
    let (pipe_reader, pipe_writer) = os_pipe::pipe().map_err(|e| {
        ExecutorError::Io(std::io::Error::other(format!("Failed to create pipe: {e}")))
    })?;
    child.inner().stdout = Some(wrap_fd_as_child_stdout(pipe_reader)?);

    // Single shared writer for both original stdout forwarding and injected lines
    let writer = wrap_fd_as_tokio_writer(pipe_writer);
    let shared_writer = std::sync::Arc::new(tokio::sync::Mutex::new(writer));

    // Create duplicate stream publisher.
    // W2-30-06: bounded channels + `send().await` backpressure so a slow
    // consumer cannot cause unbounded memory growth here.
    let (dup_tx, dup_rx) = tokio::sync::mpsc::channel::<std::io::Result<String>>(
        STDOUT_DUP_CHANNEL_BOUND,
    );
    // Create injector channel (bounded; producers use try_send via StdoutAppender).
    let (inj_tx, mut inj_rx) =
        tokio::sync::mpsc::channel::<String>(STDOUT_DUP_CHANNEL_BOUND);

    // Clone dup_tx for Task 2 before Task 1 moves it
    let dup_tx2 = dup_tx.clone();

    // Task 1: forward original stdout to child stdout and duplicate stream
    {
        let shared_writer = shared_writer.clone();
        tokio::spawn(async move {
            let mut stdout_stream = ReaderStream::new(original_stdout);
            while let Some(res) = stdout_stream.next().await {
                match res {
                    Ok(data) => {
                        // forward to child stdout
                        let mut w = shared_writer.lock().await;
                        let _ = w.write_all(&data).await;
                        drop(w);
                        // publish duplicate
                        let string_chunk = String::from_utf8_lossy(&data).into_owned();
                        // W2-30-06: async -> send().await provides backpressure.
                        if dup_tx.send(Ok(string_chunk)).await.is_err() {
                            break;
                        }
                    }
                    Err(err) => {
                        // W2-30-06: async -> send().await provides backpressure.
                        if dup_tx.send(Err(err)).await.is_err() {
                            break;
                        }
                    }
                }
            }
        });
    }

    // Task 2: write injected lines to child stdout and duplicate stream
    {
        let shared_writer = shared_writer.clone();
        tokio::spawn(async move {
            while let Some(line) = inj_rx.recv().await {
                let mut data = line.into_bytes();
                data.push(b'\n');
                let mut w = shared_writer.lock().await;
                let _ = w.write_all(&data).await;
                drop(w);
                let string_chunk = String::from_utf8_lossy(&data).into_owned();
                // W2-30-06: async -> send().await provides backpressure.
                if dup_tx2.send(Ok(string_chunk)).await.is_err() {
                    break;
                }
            }
        });
    }

    Ok((
        Box::pin(ReceiverStream::new(dup_rx)),
        StdoutAppender { tx: inj_tx },
    ))
}

/// Create a fresh stdout pipe for the child process and return an async writer
/// that writes directly to the child's new stdout.
///
/// This helper does not read or duplicate any existing stdout; it simply
/// replaces the child's stdout with a new pipe reader and returns the
/// corresponding async writer for the caller to write into.
pub fn create_stdout_pipe_writer<'b>(
    child: &mut AsyncGroupChild,
) -> Result<impl AsyncWrite + 'b, ExecutorError> {
    // Create replacement pipe and set as new child stdout
    let (pipe_reader, pipe_writer) = os_pipe::pipe().map_err(|e| {
        ExecutorError::Io(std::io::Error::other(format!("Failed to create pipe: {e}")))
    })?;
    child.inner().stdout = Some(wrap_fd_as_child_stdout(pipe_reader)?);

    // Return async writer to the caller
    Ok(wrap_fd_as_tokio_writer(pipe_writer))
}

// =========================================
// OS file descriptor helper functions
// =========================================

/// Convert os_pipe::PipeReader to tokio::process::ChildStdout
fn wrap_fd_as_child_stdout(
    pipe_reader: os_pipe::PipeReader,
) -> Result<tokio::process::ChildStdout, ExecutorError> {
    #[cfg(unix)]
    {
        // On Unix: PipeReader -> raw fd -> OwnedFd -> std::process::ChildStdout -> tokio::process::ChildStdout
        let raw_fd = pipe_reader.into_raw_fd();
        let owned_fd = unsafe { OwnedFd::from_raw_fd(raw_fd) };
        let std_stdout = std::process::ChildStdout::from(owned_fd);
        tokio::process::ChildStdout::from_std(std_stdout).map_err(ExecutorError::Io)
    }

    #[cfg(windows)]
    {
        // On Windows: PipeReader -> raw handle -> OwnedHandle -> std::process::ChildStdout -> tokio::process::ChildStdout
        let raw_handle = pipe_reader.into_raw_handle();
        let owned_handle = unsafe { OwnedHandle::from_raw_handle(raw_handle) };
        let std_stdout = std::process::ChildStdout::from(owned_handle);
        tokio::process::ChildStdout::from_std(std_stdout).map_err(ExecutorError::Io)
    }
}

/// Convert os_pipe::PipeWriter to a tokio file for async writing
fn wrap_fd_as_tokio_writer(pipe_writer: os_pipe::PipeWriter) -> impl AsyncWrite {
    #[cfg(unix)]
    {
        // On Unix: PipeWriter -> raw fd -> OwnedFd -> std::fs::File -> tokio::fs::File
        let raw_fd = pipe_writer.into_raw_fd();
        let owned_fd = unsafe { OwnedFd::from_raw_fd(raw_fd) };
        let std_file = std::fs::File::from(owned_fd);
        tokio::fs::File::from_std(std_file)
    }

    #[cfg(windows)]
    {
        // On Windows: PipeWriter -> raw handle -> OwnedHandle -> std::fs::File -> tokio::fs::File
        let raw_handle = pipe_writer.into_raw_handle();
        let owned_handle = unsafe { OwnedHandle::from_raw_handle(raw_handle) };
        let std_file = std::fs::File::from(owned_handle);
        tokio::fs::File::from_std(std_file)
    }
}
