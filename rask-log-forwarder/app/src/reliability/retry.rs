#![deny(warnings, rust_2018_idioms)]

use rand::RngExt;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::time::{Duration, Instant};
use thiserror::Error;

#[derive(Error, Debug)]
pub enum RetryError {
    #[error("Maximum retry attempts exceeded")]
    MaxAttemptsExceeded,
    #[error("Retry not found for batch: {0}")]
    RetryNotFound(String),
    #[error("Invalid retry configuration: {0}")]
    InvalidConfig(String),
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
pub enum RetryStrategy {
    ExponentialBackoff,
    LinearBackoff,
    FixedDelay,
}

#[derive(Debug, Clone)]
pub struct RetryConfig {
    pub max_attempts: u32,
    pub base_delay: Duration,
    pub max_delay: Duration,
    pub strategy: RetryStrategy,
    pub jitter: bool,
}

impl Default for RetryConfig {
    fn default() -> Self {
        Self {
            max_attempts: 5,
            base_delay: Duration::from_millis(500),
            max_delay: Duration::from_secs(60),
            strategy: RetryStrategy::ExponentialBackoff,
            jitter: true,
        }
    }
}

#[derive(Debug, Clone)]
struct RetryState {
    batch_id: String,
    attempt_count: u32,
    first_attempt_time: Instant,
    last_attempt_time: Option<Instant>,
    next_retry_time: Option<Instant>,
}

pub struct RetryManager {
    config: RetryConfig,
    retry_states: HashMap<String, RetryState>,
}

impl RetryManager {
    #[must_use]
    pub fn new(config: RetryConfig) -> Self {
        Self {
            config,
            retry_states: HashMap::new(),
        }
    }

    pub fn start_retry(&mut self, batch_id: &str) {
        let state = RetryState {
            batch_id: batch_id.to_string(),
            attempt_count: 0,
            first_attempt_time: Instant::now(),
            last_attempt_time: None,
            next_retry_time: None,
        };

        self.retry_states.insert(batch_id.to_string(), state);
    }

    pub fn increment_attempt(&mut self, batch_id: &str) {
        // First get the current attempt count
        let current_attempt = self
            .retry_states
            .get(batch_id)
            .map_or(0, |state| state.attempt_count);

        let new_attempt_count = current_attempt + 1;
        let next_retry_time = if new_attempt_count < self.config.max_attempts {
            let delay = self.calculate_delay(new_attempt_count);
            Some(Instant::now() + delay)
        } else {
            None
        };

        // Now update the state
        if let Some(state) = self.retry_states.get_mut(batch_id) {
            state.attempt_count = new_attempt_count;
            state.last_attempt_time = Some(Instant::now());
            state.next_retry_time = next_retry_time;
        }
    }

    #[must_use]
    pub fn should_give_up(&self, batch_id: &str) -> bool {
        self.retry_states
            .get(batch_id)
            .is_none_or(|state| state.attempt_count >= self.config.max_attempts)
    }

    #[must_use]
    pub fn get_attempt_count(&self, batch_id: &str) -> u32 {
        self.retry_states
            .get(batch_id)
            .map_or(0, |state| state.attempt_count)
    }

    #[must_use]
    pub fn is_ready_for_retry(&self, batch_id: &str) -> bool {
        self.retry_states
            .get(batch_id)
            .and_then(|state| state.next_retry_time)
            .is_some_and(|next_time| Instant::now() >= next_time)
    }

    #[must_use]
    pub fn calculate_delay(&self, attempt: u32) -> Duration {
        // Configured base_delay in ms cannot exceed u64::MAX (~584M years).
        let base_ms = u64::try_from(self.config.base_delay.as_millis()).unwrap_or(u64::MAX);
        let base_delay = match self.config.strategy {
            RetryStrategy::ExponentialBackoff => match 2_u64.checked_pow(attempt) {
                Some(multiplier) => Duration::from_millis(base_ms.saturating_mul(multiplier)),
                // `attempt` is large enough that 2^attempt overflows u64;
                // the delay would be capped to `max_delay` below anyway.
                None => self.config.max_delay,
            },
            RetryStrategy::LinearBackoff => {
                Duration::from_millis(base_ms.saturating_mul(u64::from(attempt).saturating_add(1)))
            }
            RetryStrategy::FixedDelay => self.config.base_delay,
        };

        // Apply maximum delay cap
        let capped_delay = std::cmp::min(base_delay, self.config.max_delay);

        // Apply jitter if enabled
        if self.config.jitter {
            Self::apply_jitter(capped_delay)
        } else {
            capped_delay
        }
    }

    fn apply_jitter(delay: Duration) -> Duration {
        let mut rng = rand::rng();
        let jitter_permille = rng.random_range(500_u64..1500_u64); // ±50% jitter
        // Delay is capped by max_delay (60s default); saturate to u64::MAX (~584M years).
        let jittered_millis = delay
            .as_millis()
            .saturating_mul(u128::from(jitter_permille))
            / 1000;
        Duration::from_millis(u64::try_from(jittered_millis).unwrap_or(u64::MAX))
    }

    pub fn remove_retry(&mut self, batch_id: &str) {
        self.retry_states.remove(batch_id);
    }

    #[must_use]
    pub fn get_pending_retries(&self) -> Vec<String> {
        self.retry_states
            .iter()
            .filter(|(_, state)| {
                !self.should_give_up(&state.batch_id)
                    && state.next_retry_time.is_none_or(|t| Instant::now() >= t)
            })
            .map(|(batch_id, _)| batch_id.clone())
            .collect()
    }

    pub fn cleanup_old_retries(&mut self, max_age: Duration) {
        let now = Instant::now();
        self.retry_states
            .retain(|_, state| now.duration_since(state.first_attempt_time) <= max_age);
    }
}
