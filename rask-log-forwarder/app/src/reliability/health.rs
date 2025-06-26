use std::collections::{HashMap, VecDeque};
use std::time::{Duration, Instant};
use serde::{Serialize, Deserialize};
use tokio::sync::RwLock;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum HealthStatus {
    Healthy,
    Degraded,
    Unhealthy,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub enum ComponentHealth {
    Healthy,
    Degraded(String), // reason
    Unhealthy(String), // reason
}

#[derive(Debug, Clone)]
pub struct HealthConfig {
    pub check_interval: Duration,
    pub unhealthy_threshold: u32,
    pub recovery_threshold: u32,
}

impl Default for HealthConfig {
    fn default() -> Self {
        Self {
            check_interval: Duration::from_secs(30),
            unhealthy_threshold: 3, // 3 consecutive failures = unhealthy
            recovery_threshold: 2,  // 2 consecutive successes = recovered
        }
    }
}

#[derive(Debug)]
struct ComponentState {
    health: ComponentHealth,
    recent_checks: VecDeque<bool>,
    consecutive_failures: u32,
    consecutive_successes: u32,
    last_check: Option<Instant>,
}

impl ComponentState {
    fn new() -> Self {
        Self {
            health: ComponentHealth::Healthy,
            recent_checks: VecDeque::with_capacity(100),
            consecutive_failures: 0,
            consecutive_successes: 0,
            last_check: None,
        }
    }
}

pub struct HealthMonitor {
    config: HealthConfig,
    components: RwLock<HashMap<String, ComponentState>>,
}

impl HealthMonitor {
    pub fn new(config: HealthConfig) -> Self {
        Self {
            config,
            components: RwLock::new(HashMap::new()),
        }
    }
    
    pub async fn update_component_health(&self, component: &str, health: ComponentHealth) {
        let mut components = self.components.write().await;
        let state = components.entry(component.to_string()).or_insert_with(ComponentState::new);
        
        state.health = health;
        state.last_check = Some(Instant::now());
        
        tracing::debug!("Updated health for component '{}': {:?}", component, state.health);
    }
    
    pub async fn record_health_check(&self, component: &str, success: bool) {
        let mut components = self.components.write().await;
        let state = components.entry(component.to_string()).or_insert_with(ComponentState::new);
        
        // Update check history
        state.recent_checks.push_back(success);
        if state.recent_checks.len() > 100 {
            state.recent_checks.pop_front();
        }
        
        state.last_check = Some(Instant::now());
        
        // Update consecutive counters
        if success {
            state.consecutive_successes += 1;
            state.consecutive_failures = 0;
        } else {
            state.consecutive_failures += 1;
            state.consecutive_successes = 0;
        }
        
        // Update health status based on consecutive results
        let new_health = if state.consecutive_failures >= self.config.unhealthy_threshold {
            ComponentHealth::Unhealthy(format!(
                "{} consecutive health check failures", 
                state.consecutive_failures
            ))
        } else if state.consecutive_successes >= self.config.recovery_threshold &&
                  matches!(state.health, ComponentHealth::Unhealthy(_)) {
            ComponentHealth::Healthy
        } else {
            state.health.clone()
        };
        
        if new_health != state.health {
            tracing::info!(
                "Component '{}' health changed from {:?} to {:?}",
                component, state.health, new_health
            );
            state.health = new_health;
        }
    }
    
    pub async fn get_overall_health(&self) -> HealthStatus {
        let components = self.components.read().await;
        
        if components.is_empty() {
            return HealthStatus::Unhealthy;
        }
        
        let mut has_unhealthy = false;
        let mut has_degraded = false;
        
        for state in components.values() {
            match &state.health {
                ComponentHealth::Unhealthy(_) => has_unhealthy = true,
                ComponentHealth::Degraded(_) => has_degraded = true,
                ComponentHealth::Healthy => {}
            }
        }
        
        if has_unhealthy {
            HealthStatus::Unhealthy
        } else if has_degraded {
            HealthStatus::Degraded
        } else {
            HealthStatus::Healthy
        }
    }
    
    pub async fn get_component_health(&self, component: &str) -> ComponentHealth {
        let components = self.components.read().await;
        components
            .get(component)
            .map(|state| state.health.clone())
            .unwrap_or(ComponentHealth::Unhealthy("Component not found".to_string()))
    }
    
    pub async fn get_component_history(&self, component: &str) -> Vec<bool> {
        let components = self.components.read().await;
        components
            .get(component)
            .map(|state| state.recent_checks.iter().cloned().collect())
            .unwrap_or_default()
    }
    
    pub async fn get_all_component_status(&self) -> HashMap<String, ComponentHealth> {
        let components = self.components.read().await;
        components
            .iter()
            .map(|(name, state)| (name.clone(), state.health.clone()))
            .collect()
    }
    
    pub async fn start_periodic_checks<F>(&self, mut check_fn: F)
    where
        F: FnMut() -> HashMap<String, bool> + Send + 'static,
    {
        let mut interval = tokio::time::interval(self.config.check_interval);
        
        loop {
            interval.tick().await;
            
            let check_results = check_fn();
            
            for (component, success) in check_results {
                self.record_health_check(&component, success).await;
            }
        }
    }
    
    pub async fn cleanup_stale_components(&self, max_age: Duration) {
        let now = Instant::now();
        let mut components = self.components.write().await;
        
        components.retain(|component, state| {
            if let Some(last_check) = state.last_check {
                let age = now.duration_since(last_check);
                if age > max_age {
                    tracing::warn!("Removing stale component '{}' (last check: {:?} ago)", component, age);
                    false
                } else {
                    true
                }
            } else {
                false // Remove components that have never been checked
            }
        });
    }
}

#[derive(Serialize)]
pub struct HealthReport {
    pub overall_status: HealthStatus,
    pub components: HashMap<String, ComponentHealth>,
    pub timestamp: String,
    pub uptime: Duration,
}

impl HealthReport {
    pub async fn generate(monitor: &HealthMonitor, start_time: Instant) -> Self {
        Self {
            overall_status: monitor.get_overall_health().await,
            components: monitor.get_all_component_status().await,
            timestamp: chrono::Utc::now().to_rfc3339(),
            uptime: start_time.elapsed(),
        }
    }
}