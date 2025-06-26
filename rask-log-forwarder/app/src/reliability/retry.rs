#![deny(warnings, rust_2018_idioms)]

use std::collections::HashMap;
use std::time::{Duration, Instant};
use rand::Rng;
use serde::{Serialize, Deserialize};
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
        let current_attempt = self.retry_states
            .get(batch_id)
            .map(|state| state.attempt_count)
            .unwrap_or(0);
        
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
    
    pub fn should_give_up(&self, batch_id: &str) -> bool {
        self.retry_states
            .get(batch_id)
            .map(|state| state.attempt_count >= self.config.max_attempts)
            .unwrap_or(true)
    }
    
    pub fn get_attempt_count(&self, batch_id: &str) -> u32 {
        self.retry_states
            .get(batch_id)
            .map(|state| state.attempt_count)
            .unwrap_or(0)
    }
    
    pub fn is_ready_for_retry(&self, batch_id: &str) -> bool {
        self.retry_states
            .get(batch_id)
            .and_then(|state| state.next_retry_time)
            .map(|next_time| Instant::now() >= next_time)
            .unwrap_or(false)
    }
    
    pub fn calculate_delay(&self, attempt: u32) -> Duration {
        let base_delay = match self.config.strategy {
            RetryStrategy::ExponentialBackoff => {
                let multiplier = 2_u64.pow(attempt);
                Duration::from_millis(
                    self.config.base_delay.as_millis() as u64 * multiplier
                )
            }
            RetryStrategy::LinearBackoff => {
                Duration::from_millis(
                    self.config.base_delay.as_millis() as u64 * (attempt as u64 + 1)
                )
            }
            RetryStrategy::FixedDelay => self.config.base_delay,
        };
        
        // Apply maximum delay cap
        let capped_delay = std::cmp::min(base_delay, self.config.max_delay);
        
        // Apply jitter if enabled
        if self.config.jitter {
            self.apply_jitter(capped_delay)
        } else {
            capped_delay
        }
    }
    
    fn apply_jitter(&self, delay: Duration) -> Duration {
        let mut rng = rand::rng();
        let jitter_factor = rng.random_range(0.5..1.5); // ±50% jitter
        let jittered_millis = (delay.as_millis() as f64 * jitter_factor) as u64;
        Duration::from_millis(jittered_millis)
    }
    
    pub fn remove_retry(&mut self, batch_id: &str) {
        self.retry_states.remove(batch_id);
    }
    
    pub fn get_pending_retries(&self) -> Vec<String> {
        self.retry_states
            .iter()
            .filter(|(_, state)| {
                !self.should_give_up(&state.batch_id) &&
                state.next_retry_time.map(|t| Instant::now() >= t).unwrap_or(true)
            })
            .map(|(batch_id, _)| batch_id.clone())
            .collect()
    }
    
    pub fn cleanup_old_retries(&mut self, max_age: Duration) {
        let now = Instant::now();
        self.retry_states.retain(|_, state| {
            now.duration_since(state.first_attempt_time) <= max_age
        });
    }
}