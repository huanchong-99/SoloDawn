use std::{
    collections::VecDeque,
    sync::{Arc, RwLock},
};

use axum::response::sse::Event;
use futures::{StreamExt, TryStreamExt, future};
use tokio::{sync::broadcast, task::JoinHandle};
use tokio_stream::wrappers::BroadcastStream;

use crate::{log_msg::LogMsg, stream_lines::LinesStreamExt};

// G33-005: Reduced history cap from 100 MB to 50 MB
const HISTORY_BYTES: usize = 50_000 * 1_024;

/// Warn threshold: emit a tracing::warn! when usage exceeds 80% of the cap.
const HISTORY_WARN_THRESHOLD: usize = HISTORY_BYTES / 5 * 4;

#[derive(Clone)]
struct StoredMsg {
    msg: LogMsg,
    bytes: usize,
}

struct Inner {
    history: VecDeque<StoredMsg>,
    total_bytes: usize,
}

pub struct MsgStore {
    inner: RwLock<Inner>,
    sender: broadcast::Sender<LogMsg>,
}

impl Default for MsgStore {
    fn default() -> Self {
        Self::new()
    }
}

impl MsgStore {
    pub fn new() -> Self {
        let (sender, _) = broadcast::channel(10000);
        Self {
            inner: RwLock::new(Inner {
                history: VecDeque::with_capacity(32),
                total_bytes: 0,
            }),
            sender,
        }
    }

    pub fn push(&self, msg: LogMsg) {
        if self.sender.send(msg.clone()).is_err() {
            tracing::debug!("msg_store broadcast has no listeners");
        }
        let bytes = msg.approx_bytes();

        let mut inner = self
            .inner
            .write()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        while inner.total_bytes.saturating_add(bytes) > HISTORY_BYTES {
            if let Some(front) = inner.history.pop_front() {
                inner.total_bytes = inner.total_bytes.saturating_sub(front.bytes);
            } else {
                break;
            }
        }
        inner.history.push_back(StoredMsg { msg, bytes });
        inner.total_bytes = inner.total_bytes.saturating_add(bytes);

        // G33-005: warn when history usage exceeds 80% of the cap
        if inner.total_bytes >= HISTORY_WARN_THRESHOLD {
            tracing::warn!(
                used_bytes = inner.total_bytes,
                cap_bytes = HISTORY_BYTES,
                "MsgStore history usage exceeds 80% of cap; consider increasing throughput or reducing history"
            );
        }
    }

    // Convenience
    pub fn push_stdout<S: Into<String>>(&self, s: S) {
        self.push(LogMsg::Stdout(s.into()));
    }

    pub fn push_stderr<S: Into<String>>(&self, s: S) {
        self.push(LogMsg::Stderr(s.into()));
    }
    pub fn push_patch(&self, patch: json_patch::Patch) {
        self.push(LogMsg::JsonPatch(patch));
    }

    pub fn push_session_id(&self, session_id: String) {
        self.push(LogMsg::SessionId(session_id));
    }

    pub fn push_finished(&self) {
        self.push(LogMsg::Finished);
    }

    /// Subscribe to the live broadcast channel.
    ///
    /// E25-09: We intentionally do not add an explicit buffer-overrun guard
    /// here. The underlying `tokio::sync::broadcast` channel already signals
    /// overrun to slow subscribers via `RecvError::Lagged(n)`, which callers
    /// are expected to handle (typically by resyncing from `get_history()` or
    /// logging and continuing). Dropped messages on lag are an accepted
    /// trade-off for back-pressure-free producers; the history buffer acts as
    /// the recovery path for subscribers who miss live messages.
    pub fn get_receiver(&self) -> broadcast::Receiver<LogMsg> {
        self.sender.subscribe()
    }

    pub fn get_history(&self) -> Vec<LogMsg> {
        self.inner
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .history
            .iter()
            .map(|s| s.msg.clone())
            .collect()
    }

    /// History then live, as `LogMsg`.
    ///
    /// G33-006: to close the race window between taking the history snapshot and
    /// subscribing to the live broadcast channel, we subscribe to the channel
    /// *first* and then snapshot history while we are already a subscriber.
    /// Any message pushed concurrently will either already be in the history
    /// snapshot (pushed before subscribe) or will appear in the live channel
    /// (pushed after subscribe).  Neither case produces a gap.
    pub fn history_plus_stream(
        &self,
    ) -> futures::stream::BoxStream<'static, Result<LogMsg, std::io::Error>> {
        // 1. Subscribe to the broadcast channel first so we don't miss anything.
        let rx = self.sender.subscribe();
        // 2. Only then take the history snapshot.  Messages pushed between these
        //    two steps will appear in both the history and the live stream (a
        //    harmless duplicate), but no message will be silently lost.
        let history: Vec<LogMsg> = self
            .inner
            .read()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .history
            .iter()
            .map(|s| s.msg.clone())
            .collect();

        let hist = futures::stream::iter(history.into_iter().map(Ok::<_, std::io::Error>));
        let live = BroadcastStream::new(rx)
            .filter_map(|res| async move { res.ok().map(Ok::<_, std::io::Error>) });

        Box::pin(hist.chain(live))
    }

    pub fn stdout_chunked_stream(
        &self,
    ) -> futures::stream::BoxStream<'static, Result<String, std::io::Error>> {
        self.history_plus_stream()
            .take_while(|res| future::ready(!matches!(res, Ok(LogMsg::Finished))))
            .filter_map(|res| async move {
                match res {
                    Ok(LogMsg::Stdout(s)) => Some(Ok(s)),
                    _ => None,
                }
            })
            .boxed()
    }

    pub fn stdout_lines_stream(
        &self,
    ) -> futures::stream::BoxStream<'static, std::io::Result<String>> {
        self.stdout_chunked_stream().lines()
    }

    pub fn stderr_chunked_stream(
        &self,
    ) -> futures::stream::BoxStream<'static, Result<String, std::io::Error>> {
        self.history_plus_stream()
            .take_while(|res| future::ready(!matches!(res, Ok(LogMsg::Finished))))
            .filter_map(|res| async move {
                match res {
                    Ok(LogMsg::Stderr(s)) => Some(Ok(s)),
                    _ => None,
                }
            })
            .boxed()
    }

    pub fn stderr_lines_stream(
        &self,
    ) -> futures::stream::BoxStream<'static, std::io::Result<String>> {
        self.stderr_chunked_stream().lines()
    }

    /// Same stream but mapped to `Event` for SSE handlers.
    pub fn sse_stream(&self) -> futures::stream::BoxStream<'static, Result<Event, std::io::Error>> {
        self.history_plus_stream()
            .map_ok(|m| m.to_sse_event())
            .boxed()
    }

    /// Forward a stream of typed log messages into this store.
    pub fn spawn_forwarder<S, E>(self: Arc<Self>, stream: S) -> JoinHandle<()>
    where
        S: futures::Stream<Item = Result<LogMsg, E>> + Send + 'static,
        E: std::fmt::Display + Send + 'static,
    {
        tokio::spawn(async move {
            tokio::pin!(stream);

            while let Some(next) = stream.next().await {
                match next {
                    Ok(msg) => self.push(msg),
                    Err(e) => self.push(LogMsg::Stderr(format!("stream error: {e}"))),
                }
            }
        })
    }
}
