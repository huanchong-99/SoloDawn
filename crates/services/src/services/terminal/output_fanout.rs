//! PTY output fanout (single-reader -> multi-subscriber).
//!
//! Design goals:
//! - Exactly one PTY reader per terminal process.
//! - Broadcast live output to multiple subscribers (WS, PromptWatcher, etc.).
//! - Retain bounded replay history to prevent "first-screen prompt loss".
//! - Provide monotonic sequence numbers for dedupe and resume.

use std::{
    collections::VecDeque,
    sync::{Arc, Mutex},
    time::SystemTime,
};

use tokio::sync::broadcast;

/// One decoded output chunk with metadata.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct OutputChunk {
    /// Monotonic sequence per terminal output stream, starting at 1.
    pub seq: u64,
    /// Decoded terminal text.
    pub text: String,
    /// Invalid UTF-8 bytes dropped during decode for this chunk.
    pub dropped_invalid_bytes: usize,
    /// Emission timestamp.
    pub timestamp: SystemTime,
}

/// Runtime configuration for fanout buffering.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct OutputFanoutConfig {
    /// `broadcast` channel capacity for live subscribers.
    pub broadcast_capacity: usize,
    /// Max replayed chunks retained in memory.
    pub replay_max_chunks: usize,
    /// Max replayed text bytes retained in memory.
    pub replay_max_bytes: usize,
}

impl Default for OutputFanoutConfig {
    fn default() -> Self {
        Self {
            broadcast_capacity: 512,
            // [G09-005] replay capacity is 2x broadcast capacity to absorb bursts
            // without losing chunks that are still in the broadcast channel.
            replay_max_chunks: 1024,
            replay_max_bytes: 1024 * 1024, // 1 MiB
        }
    }
}

#[derive(Debug, Default)]
struct OutputFanoutInner {
    next_seq: u64,
    replay: VecDeque<OutputChunk>,
    replay_bytes: usize,
}

/// Shared fanout hub for one terminal output stream.
#[derive(Debug, Clone)]
pub struct OutputFanout {
    tx: broadcast::Sender<OutputChunk>,
    inner: Arc<Mutex<OutputFanoutInner>>,
    config: OutputFanoutConfig,
}

/// Subscriber handle with replay + live stream and built-in dedupe by `seq`.
///
/// [G09-003] TODO: Support sequence-based resume for WebSocket reconnections.
/// When a WS client reconnects, it should pass its last-seen `seq` to
/// `OutputFanout::subscribe(Some(last_seq))` so only missed chunks are replayed.
/// This requires the frontend to track and send `last_seq` on reconnect.
///
/// [G09-004] TODO: Make replay buffer limits configurable at runtime (e.g., via
/// environment variables or server config) instead of compile-time constants.
/// This would allow operators to tune memory usage per deployment.
pub struct OutputSubscription {
    replay: VecDeque<OutputChunk>,
    rx: broadcast::Receiver<OutputChunk>,
    last_seq: u64,
}

impl OutputFanout {
    /// Create a new fanout hub.
    pub fn new(config: OutputFanoutConfig) -> Self {
        let config = OutputFanoutConfig {
            broadcast_capacity: config.broadcast_capacity.max(1),
            replay_max_chunks: config.replay_max_chunks.max(1),
            replay_max_bytes: config.replay_max_bytes.max(1),
        };
        let (tx, _) = broadcast::channel(config.broadcast_capacity);
        Self {
            tx,
            inner: Arc::new(Mutex::new(OutputFanoutInner::default())),
            config,
        }
    }

    /// Publish one decoded chunk.
    ///
    /// Returns `None` for fully empty/no-op chunks.
    pub fn publish(&self, text: String, dropped_invalid_bytes: usize) -> Option<OutputChunk> {
        if text.is_empty() && dropped_invalid_bytes == 0 {
            return None;
        }

        let mut inner = match self.inner.lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };

        inner.next_seq = inner.next_seq.saturating_add(1);
        let chunk = OutputChunk {
            seq: inner.next_seq,
            text,
            dropped_invalid_bytes,
            timestamp: SystemTime::now(),
        };

        inner.replay_bytes = inner.replay_bytes.saturating_add(chunk.text.len());
        inner.replay.push_back(chunk.clone());

        // Evict old chunks if limits exceeded
        while inner.replay.len() > self.config.replay_max_chunks
            || inner.replay_bytes > self.config.replay_max_bytes
        {
            if let Some(evicted) = inner.replay.pop_front() {
                inner.replay_bytes = inner.replay_bytes.saturating_sub(evicted.text.len());
            } else {
                break;
            }
        }

        drop(inner);
        let _ = self.tx.send(chunk.clone());
        Some(chunk)
    }

    /// Latest published sequence.
    pub fn latest_seq(&self) -> u64 {
        let inner = match self.inner.lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };
        inner.next_seq
    }

    /// Subscribe to output stream with replay.
    ///
    /// - `from_seq = None`: replay all retained chunks.
    /// - `from_seq = Some(n)`: replay chunks where `seq > n`.
    ///
    /// Race-safety note:
    /// We subscribe to live channel first, then snapshot replay. Overlap can happen
    /// but is deduped by sequence in `OutputSubscription::recv`.
    pub fn subscribe(&self, from_seq: Option<u64>) -> OutputSubscription {
        let rx = self.tx.subscribe();
        let mut replay = VecDeque::new();
        let mut last_seq = from_seq.unwrap_or(0);

        let inner = match self.inner.lock() {
            Ok(guard) => guard,
            Err(poisoned) => poisoned.into_inner(),
        };
        for chunk in &inner.replay {
            if chunk.seq > last_seq {
                replay.push_back(chunk.clone());
            }
        }
        if let Some(last) = replay.back() {
            last_seq = last.seq;
        }

        OutputSubscription {
            replay,
            rx,
            last_seq,
        }
    }
}

impl OutputSubscription {
    /// Receive next chunk (replay first, then live).
    pub async fn recv(&mut self) -> Result<OutputChunk, broadcast::error::RecvError> {
        // Drain replay buffer first
        if let Some(chunk) = self.replay.pop_front() {
            self.last_seq = chunk.seq;
            return Ok(chunk);
        }

        // Then receive from live stream with deduplication
        loop {
            let chunk = self.rx.recv().await?;
            if chunk.seq <= self.last_seq {
                // Skip duplicate (already seen in replay or previous live)
                continue;
            }
            self.last_seq = chunk.seq;
            return Ok(chunk);
        }
    }

    /// Last consumed sequence.
    pub fn last_seq(&self) -> u64 {
        self.last_seq
    }
}

#[cfg(test)]
mod tests {
    use tokio::time::{Duration, timeout};

    use super::*;

    #[test]
    fn test_publish_seq_increments() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());
        let c1 = fanout.publish("a".to_string(), 0).unwrap();
        let c2 = fanout.publish("b".to_string(), 0).unwrap();
        assert_eq!(c1.seq, 1);
        assert_eq!(c2.seq, 2);
    }

    #[test]
    fn test_publish_empty_returns_none() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());
        let result = fanout.publish(String::new(), 0);
        assert!(result.is_none());
    }

    #[tokio::test]
    async fn test_subscribe_with_replay() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());
        fanout.publish("first".to_string(), 0);
        fanout.publish("second".to_string(), 0);

        let mut sub = fanout.subscribe(None);
        let first = timeout(Duration::from_secs(1), sub.recv())
            .await
            .unwrap()
            .unwrap();
        let second = timeout(Duration::from_secs(1), sub.recv())
            .await
            .unwrap()
            .unwrap();
        assert_eq!(first.text, "first");
        assert_eq!(second.text, "second");
        assert_eq!(second.seq, 2);
    }

    #[tokio::test]
    async fn test_subscribe_from_seq_only_newer_chunks() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());
        fanout.publish("old-1".to_string(), 0);
        fanout.publish("old-2".to_string(), 0);

        let mut sub = fanout.subscribe(Some(2));
        fanout.publish("new-3".to_string(), 0);

        let chunk = timeout(Duration::from_secs(1), sub.recv())
            .await
            .unwrap()
            .unwrap();
        assert_eq!(chunk.seq, 3);
        assert_eq!(chunk.text, "new-3");
    }

    #[tokio::test]
    async fn test_replay_eviction_by_chunk_limit() {
        let config = OutputFanoutConfig {
            replay_max_chunks: 2,
            replay_max_bytes: 1024,
            ..OutputFanoutConfig::default()
        };
        let fanout = OutputFanout::new(config);
        fanout.publish("1".to_string(), 0);
        fanout.publish("2".to_string(), 0);
        fanout.publish("3".to_string(), 0);

        let mut sub = fanout.subscribe(None);
        let c1 = timeout(Duration::from_secs(1), sub.recv())
            .await
            .unwrap()
            .unwrap();
        let c2 = timeout(Duration::from_secs(1), sub.recv())
            .await
            .unwrap()
            .unwrap();
        // First chunk should be evicted, only last 2 retained
        assert_eq!(c1.text, "2");
        assert_eq!(c2.text, "3");
    }

    #[tokio::test]
    async fn test_multiple_subscribers_receive_same_output() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());

        let mut sub1 = fanout.subscribe(None);
        let mut sub2 = fanout.subscribe(None);

        fanout.publish("broadcast".to_string(), 0);

        let chunk1 = timeout(Duration::from_secs(1), sub1.recv())
            .await
            .unwrap()
            .unwrap();
        let chunk2 = timeout(Duration::from_secs(1), sub2.recv())
            .await
            .unwrap()
            .unwrap();

        assert_eq!(chunk1.text, "broadcast");
        assert_eq!(chunk2.text, "broadcast");
        assert_eq!(chunk1.seq, chunk2.seq);
    }

    #[test]
    fn test_latest_seq_tracking() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());
        assert_eq!(fanout.latest_seq(), 0);

        fanout.publish("a".to_string(), 0);
        assert_eq!(fanout.latest_seq(), 1);

        fanout.publish("b".to_string(), 0);
        assert_eq!(fanout.latest_seq(), 2);
    }

    #[tokio::test]
    async fn test_dropped_invalid_bytes_tracking() {
        let fanout = OutputFanout::new(OutputFanoutConfig::default());
        let chunk = fanout.publish("text".to_string(), 5).unwrap();

        assert_eq!(chunk.dropped_invalid_bytes, 5);

        let mut sub = fanout.subscribe(None);
        let received = timeout(Duration::from_secs(1), sub.recv())
            .await
            .unwrap()
            .unwrap();
        assert_eq!(received.dropped_invalid_bytes, 5);
    }
}
