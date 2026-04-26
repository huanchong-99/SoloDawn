//! Resilient LLM client with multi-provider failover and circuit breaking.

use std::{
    sync::{
        Arc,
        atomic::{AtomicUsize, Ordering},
    },
    time::{Duration, Instant},
};

use async_trait::async_trait;
use serde::{Deserialize, Serialize};
use tokio::sync::RwLock;

use super::{
    llm::LLMClient,
    types::{LLMMessage, LLMResponse},
};

/// Number of consecutive failures before a provider is marked dead.
const CIRCUIT_BREAKER_THRESHOLD: u32 = 5;

/// Seconds to wait before probing a dead provider again.
const PROBE_INTERVAL_SECS: u64 = 60;

/// Per-provider health tracking state.
#[derive(Default)]
struct ProviderState {
    consecutive_failures: u32,
    is_dead: bool,
    last_failure: Option<Instant>,
    last_probe: Option<Instant>,
    total_requests: u64,
    total_failures: u64,
}

/// A single provider entry: client + mutable health state.
struct ProviderEntry {
    name: String,
    client: Box<dyn LLMClient>,
    state: Arc<RwLock<ProviderState>>,
}

/// Status report for a single provider (for monitoring / API exposure).
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProviderStatusReport {
    pub name: String,
    pub is_active: bool,
    pub is_dead: bool,
    pub consecutive_failures: u32,
    pub total_requests: u64,
    pub total_failures: u64,
}

/// Event emitted when provider state changes during a `chat` call.
#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(tag = "kind", rename_all = "snake_case")]
pub enum ProviderEvent {
    /// Active provider switched to a different one after failure.
    Switched {
        from_provider: String,
        to_provider: String,
    },
    /// All providers have been exhausted (all dead or failed).
    Exhausted { provider_count: usize },
    /// A previously dead provider recovered via successful probe.
    Recovered { provider_name: String },
}

/// An LLM client that wraps multiple providers with automatic circuit-breaking
/// and round-robin failover.
///
/// On each `chat` call the active provider is tried first. If it fails, the
/// failure counter is incremented; once it reaches [`CIRCUIT_BREAKER_THRESHOLD`]
/// the provider is marked dead and the next healthy provider is selected.
///
/// Dead providers are periodically probed (every [`PROBE_INTERVAL_SECS`]
/// seconds) and revived on success.
pub struct ResilientLLMClient {
    providers: Vec<ProviderEntry>,
    active_index: AtomicUsize,
    /// Events collected during the last `chat` call.
    last_events: RwLock<Vec<ProviderEvent>>,
}

impl ResilientLLMClient {
    /// Build a new resilient client from a list of `(name, client)` pairs.
    ///
    /// The first entry is treated as the primary provider; subsequent entries
    /// are fallbacks tried in order.  Panics if `providers` is empty.
    pub fn new(providers: Vec<(String, Box<dyn LLMClient>)>) -> Self {
        assert!(
            !providers.is_empty(),
            "ResilientLLMClient requires at least one provider"
        );

        let entries = providers
            .into_iter()
            .map(|(name, client)| ProviderEntry {
                name,
                client,
                state: Arc::new(RwLock::new(ProviderState::default())),
            })
            .collect();

        Self {
            providers: entries,
            active_index: AtomicUsize::new(0),
            last_events: RwLock::new(Vec::new()),
        }
    }

    /// Returns the name of the currently active provider.
    pub fn active_provider_name(&self) -> String {
        let idx = self.active_index.load(Ordering::Relaxed);
        self.providers[idx].name.clone()
    }

    /// Reset the circuit breaker for a provider by name.
    ///
    /// Returns `true` if the provider was found and reset, `false` otherwise.
    pub async fn reset_provider(&self, provider_name: &str) -> bool {
        for entry in &self.providers {
            if entry.name == provider_name {
                let mut state = entry.state.write().await;
                state.consecutive_failures = 0;
                state.is_dead = false;
                tracing::info!(
                    "ResilientLLMClient: circuit breaker reset for provider '{}'",
                    provider_name,
                );
                return true;
            }
        }
        false
    }

    /// Take and drain events collected during the last `chat` call.
    pub async fn take_last_events(&self) -> Vec<ProviderEvent> {
        let mut events = self.last_events.write().await;
        std::mem::take(&mut *events)
    }

    /// Returns a status snapshot for every registered provider.
    pub async fn provider_status(&self) -> Vec<ProviderStatusReport> {
        let active = self.active_index.load(Ordering::Relaxed);
        let mut reports = Vec::with_capacity(self.providers.len());

        for (i, entry) in self.providers.iter().enumerate() {
            let state = entry.state.read().await;
            reports.push(ProviderStatusReport {
                name: entry.name.clone(),
                is_active: i == active,
                is_dead: state.is_dead,
                consecutive_failures: state.consecutive_failures,
                total_requests: state.total_requests,
                total_failures: state.total_failures,
            });
        }

        reports
    }

    /// Advance `active_index` from `current` to the next provider (wrapping).
    ///
    /// Uses CAS so only one thread performs the switch when racing.
    fn switch_to_next(&self, current: usize) {
        let next = (current + 1) % self.providers.len();
        // Best-effort CAS; if another thread already switched, that's fine.
        let _ =
            self.active_index
                .compare_exchange(current, next, Ordering::AcqRel, Ordering::Relaxed);
        tracing::warn!(
            "ResilientLLMClient: switching from provider {} ({}) to {} ({})",
            current,
            self.providers[current].name,
            next,
            self.providers[next].name,
        );
    }

    /// Whether a dead provider should be probed again.
    fn should_probe(state: &ProviderState) -> bool {
        if !state.is_dead {
            return false;
        }
        let probe_interval = Duration::from_secs(PROBE_INTERVAL_SECS);
        match state.last_probe.or(state.last_failure) {
            Some(t) => t.elapsed() >= probe_interval,
            None => true,
        }
    }

    /// Record a successful request for the given provider.
    ///
    /// G24-008: Release the state lock before acquiring the events lock to
    /// avoid nested lock acquisition which could cause deadlocks under
    /// contention.
    async fn record_success(&self, idx: usize) {
        let was_dead = {
            let mut state = self.providers[idx].state.write().await;
            state.total_requests += 1;
            state.consecutive_failures = 0;
            let was_dead = state.is_dead;
            if state.is_dead {
                tracing::info!(
                    "ResilientLLMClient: provider {} ({}) revived after successful probe",
                    idx,
                    self.providers[idx].name,
                );
                state.is_dead = false;
            }
            was_dead
            // state lock released here
        };
        if was_dead {
            // Record recovery event with state lock already released
            let mut events = self.last_events.write().await;
            events.push(ProviderEvent::Recovered {
                provider_name: self.providers[idx].name.clone(),
            });
        }
    }

    /// Record a failed request; returns `true` if the provider was just marked dead.
    async fn record_failure(&self, idx: usize) -> bool {
        let mut state = self.providers[idx].state.write().await;
        state.total_requests += 1;
        state.total_failures += 1;
        state.consecutive_failures += 1;
        state.last_failure = Some(Instant::now());

        if !state.is_dead && state.consecutive_failures >= CIRCUIT_BREAKER_THRESHOLD {
            state.is_dead = true;
            tracing::error!(
                "ResilientLLMClient: provider {} ({}) marked DEAD after {} consecutive failures",
                idx,
                self.providers[idx].name,
                state.consecutive_failures,
            );
            return true;
        }
        false
    }
}

#[async_trait]
impl LLMClient for ResilientLLMClient {
    async fn chat(&self, messages: Vec<LLMMessage>) -> anyhow::Result<LLMResponse> {
        // Clear events from previous call.
        {
            let mut events = self.last_events.write().await;
            events.clear();
        }

        let provider_count = self.providers.len();
        let start_index = self.active_index.load(Ordering::Relaxed);

        // Try each provider at most once per call.
        for offset in 0..provider_count {
            let idx = (start_index + offset) % provider_count;
            let entry = &self.providers[idx];

            // Check circuit breaker and mark probe timestamp atomically
            // to prevent TOCTOU race where multiple threads probe simultaneously.
            {
                let mut state = entry.state.write().await;
                if state.is_dead {
                    if !Self::should_probe(&state) {
                        tracing::debug!(
                            "ResilientLLMClient: skipping dead provider {} ({})",
                            idx,
                            entry.name,
                        );
                        continue;
                    }
                    tracing::info!(
                        "ResilientLLMClient: probing dead provider {} ({})",
                        idx,
                        entry.name,
                    );
                    state.last_probe = Some(Instant::now());
                }
            }

            match entry.client.chat(messages.clone()).await {
                Ok(response) => {
                    self.record_success(idx).await;
                    // If we drifted away from the active index, update it.
                    if idx != start_index {
                        let _ = self.active_index.compare_exchange(
                            start_index,
                            idx,
                            Ordering::AcqRel,
                            Ordering::Relaxed,
                        );
                    }
                    return Ok(response);
                }
                Err(e) => {
                    tracing::warn!(
                        "ResilientLLMClient: provider {} ({}) failed: {}",
                        idx,
                        entry.name,
                        e,
                    );
                    let just_died = self.record_failure(idx).await;
                    if offset + 1 < provider_count {
                        let next_idx = (idx + 1) % provider_count;
                        if just_died {
                            self.switch_to_next(idx);
                        }
                        // G24-003: emit Switched event on every actual provider switch,
                        // not only when the provider transitions to "dead".
                        let mut events = self.last_events.write().await;
                        events.push(ProviderEvent::Switched {
                            from_provider: self.providers[idx].name.clone(),
                            to_provider: self.providers[next_idx].name.clone(),
                        });
                    }
                    // Continue to next provider.
                }
            }
        }

        // Record exhausted event
        {
            let mut events = self.last_events.write().await;
            events.push(ProviderEvent::Exhausted { provider_count });
        }

        Err(anyhow::anyhow!(
            "All LLM providers exhausted ({provider_count} providers tried)",
        ))
    }

    async fn provider_status(&self) -> Vec<ProviderStatusReport> {
        // Delegate to the inherent method.
        ResilientLLMClient::provider_status(self).await
    }

    async fn reset_provider(&self, provider_name: &str) -> bool {
        ResilientLLMClient::reset_provider(self, provider_name).await
    }

    async fn take_provider_events(&self) -> Vec<ProviderEvent> {
        self.take_last_events().await
    }
}

// SAFETY: ProviderEntry contains Box<dyn LLMClient> which is Send + Sync
// (trait bound on LLMClient), and Arc<RwLock<ProviderState>> is Send + Sync.
// AtomicUsize is Send + Sync. Therefore ResilientLLMClient is Send + Sync.

#[cfg(test)]
mod tests {
    use super::{super::llm::MockLLMClient, *};

    fn msg() -> Vec<LLMMessage> {
        vec![LLMMessage {
            role: "user".to_string(),
            content: "hello".to_string(),
        }]
    }

    #[tokio::test]
    async fn single_provider_success() {
        let client =
            ResilientLLMClient::new(vec![("primary".into(), Box::new(MockLLMClient::new()))]);
        let result = client.chat(msg()).await;
        assert!(result.is_ok());
        assert_eq!(client.active_provider_name(), "primary");
    }

    #[tokio::test]
    async fn failover_to_second_provider() {
        let client = ResilientLLMClient::new(vec![
            ("primary".into(), Box::new(MockLLMClient::that_fails())),
            ("fallback".into(), Box::new(MockLLMClient::new())),
        ]);
        let result = client.chat(msg()).await;
        assert!(result.is_ok());
    }

    #[tokio::test]
    async fn all_providers_fail_returns_error() {
        let client = ResilientLLMClient::new(vec![
            ("p1".into(), Box::new(MockLLMClient::that_fails())),
            ("p2".into(), Box::new(MockLLMClient::that_fails())),
        ]);
        let result = client.chat(msg()).await;
        assert!(result.is_err());
        assert!(
            result
                .unwrap_err()
                .to_string()
                .contains("All LLM providers exhausted")
        );
    }

    #[tokio::test]
    async fn circuit_breaker_marks_provider_dead() {
        // Use two failing providers so the primary accumulates failures
        // without a healthy fallback stealing the active index.
        let client = ResilientLLMClient::new(vec![
            ("primary".into(), Box::new(MockLLMClient::that_fails())),
            ("fallback".into(), Box::new(MockLLMClient::that_fails())),
        ]);

        // Each chat() call fails both providers (2 failures each round).
        // After enough rounds the primary hits the threshold.
        for _ in 0..CIRCUIT_BREAKER_THRESHOLD {
            let _ = client.chat(msg()).await;
        }

        let status = client.provider_status().await;
        assert!(
            status[0].is_dead,
            "primary should be dead after threshold failures"
        );
    }

    #[tokio::test]
    async fn provider_status_reports_correct_counts() {
        let client = ResilientLLMClient::new(vec![("only".into(), Box::new(MockLLMClient::new()))]);

        client.chat(msg()).await.unwrap();
        client.chat(msg()).await.unwrap();

        let status = client.provider_status().await;
        assert_eq!(status.len(), 1);
        assert_eq!(status[0].total_requests, 2);
        assert_eq!(status[0].total_failures, 0);
        assert!(status[0].is_active);
    }
}
