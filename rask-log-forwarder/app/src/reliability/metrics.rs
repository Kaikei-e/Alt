use crate::buffer::{MetricsError, safe_metrics_operation};
#[cfg(feature = "metrics")]
use prometheus::{Counter, CounterVec, Encoder, Gauge, HistogramVec, Registry, TextEncoder};
use std::sync::Arc;
#[cfg(feature = "metrics")]
use std::sync::OnceLock;
use std::time::Duration;
use tokio::sync::RwLock;

#[cfg(feature = "metrics")]
use warp::{Filter, Reply};

// MetricsError is now imported from crate::buffer::error

#[derive(Debug, Clone)]
pub struct MetricsConfig {
    pub enabled: bool,
    pub export_port: u16,
    pub export_path: String,
    pub collection_interval: Duration,
}

impl Default for MetricsConfig {
    fn default() -> Self {
        Self {
            enabled: true,
            export_port: 9090,
            export_path: "/metrics".to_string(),
            collection_interval: Duration::from_secs(15),
        }
    }
}

#[derive(Debug, Clone)]
pub struct MetricsSnapshot {
    pub total_batches_sent: u64,
    pub successful_batches: u64,
    pub failed_batches: u64,
    pub total_entries_sent: u64,
    pub disk_fallback_count: u64,
    pub retry_attempts: u64,
    pub health_check_total: u64,
    pub health_check_success: u64,
    pub health_check_failure: u64,
    pub average_transmission_latency: Duration,
    pub current_memory_usage: u64,
    pub active_connections: u64,
}

#[derive(Clone)]
pub struct MetricsCollector {
    #[allow(dead_code)]
    config: MetricsConfig,

    #[cfg(feature = "metrics")]
    prometheus: Option<PrometheusMetrics>,

    // Internal state
    state: Arc<RwLock<MetricsState>>,
}

struct MetricsState {
    total_batches_sent: u64,
    successful_batches: u64,
    failed_batches: u64,
    retry_count: u64,
    health_check_total: u64,
    health_check_success: u64,
    health_check_failure: u64,
}

/// Prometheus metric handles for `MetricsCollector`. Also used for
/// `MetricsCollector::new_legacy`'s fallback path, cached via `OnceLock`.
#[cfg(feature = "metrics")]
#[derive(Clone)]
struct PrometheusMetrics {
    registry: Arc<Registry>,
    batches_sent: CounterVec,
    entries_sent: Counter,
    transmission_latency: HistogramVec,
    disk_fallback_counter: Counter,
    retry_attempts: CounterVec,
    health_checks: CounterVec,
    memory_usage: Gauge,
    active_connections: Gauge,
}

#[cfg(feature = "metrics")]
struct PrometheusMetricNames<'a> {
    batches: &'a str,
    entries: &'a str,
    latency: &'a str,
    latency_label: &'a str,
    disk: &'a str,
    retry: &'a str,
    retry_label: &'a str,
    health: &'a str,
    memory: &'a str,
    connections: &'a str,
    register_in_registry: bool,
}

#[cfg(feature = "metrics")]
fn build_prometheus_metrics(
    names: &PrometheusMetricNames<'_>,
) -> Result<PrometheusMetrics, MetricsError> {
    let registry = Arc::new(Registry::new());

    let batches_sent = CounterVec::new(
        prometheus::Opts::new(names.batches, "Total number of batches sent"),
        &["status"],
    )
    .map_err(|e| MetricsError::InitializationFailed {
        reason: format!("Failed to create batches_sent counter: {e}"),
    })?;
    let entries_sent =
        Counter::new(names.entries, "Total number of log entries sent").map_err(|e| {
            MetricsError::InitializationFailed {
                reason: format!("Failed to create entries_sent counter: {e}"),
            }
        })?;
    let transmission_latency = HistogramVec::new(
        prometheus::HistogramOpts::new(names.latency, "Transmission latency in seconds"),
        &[names.latency_label],
    )
    .map_err(|e| MetricsError::InitializationFailed {
        reason: format!("Failed to create transmission_latency histogram: {e}"),
    })?;
    let disk_fallback_counter = Counter::new(names.disk, "Total number of batches stored to disk")
        .map_err(|e| MetricsError::InitializationFailed {
            reason: format!("Failed to create disk_fallback_counter: {e}"),
        })?;
    let retry_attempts = CounterVec::new(
        prometheus::Opts::new(names.retry, "Total number of retry attempts"),
        &[names.retry_label],
    )
    .map_err(|e| MetricsError::InitializationFailed {
        reason: format!("Failed to create retry_attempts counter: {e}"),
    })?;
    let health_checks = CounterVec::new(
        prometheus::Opts::new(names.health, "Total number of health checks"),
        &["status"],
    )
    .map_err(|e| MetricsError::InitializationFailed {
        reason: format!("Failed to create health_checks counter: {e}"),
    })?;
    let memory_usage = Gauge::new(names.memory, "Current memory usage in bytes").map_err(|e| {
        MetricsError::InitializationFailed {
            reason: format!("Failed to create memory_usage gauge: {e}"),
        }
    })?;
    let active_connections = Gauge::new(names.connections, "Number of active HTTP connections")
        .map_err(|e| MetricsError::InitializationFailed {
            reason: format!("Failed to create active_connections gauge: {e}"),
        })?;

    if names.register_in_registry {
        let collectors: [(&str, Box<dyn prometheus::core::Collector>); 8] = [
            ("batches_sent", Box::new(batches_sent.clone())),
            ("entries_sent", Box::new(entries_sent.clone())),
            (
                "transmission_latency",
                Box::new(transmission_latency.clone()),
            ),
            (
                "disk_fallback_counter",
                Box::new(disk_fallback_counter.clone()),
            ),
            ("retry_attempts", Box::new(retry_attempts.clone())),
            ("health_checks", Box::new(health_checks.clone())),
            ("memory_usage", Box::new(memory_usage.clone())),
            ("active_connections", Box::new(active_connections.clone())),
        ];
        for (label, collector) in collectors {
            registry
                .register(collector)
                .map_err(|e| MetricsError::RegistrationFailed {
                    details: format!("Failed to register {label} metric: {e}"),
                })?;
        }
    }

    Ok(PrometheusMetrics {
        registry,
        batches_sent,
        entries_sent,
        transmission_latency,
        disk_fallback_counter,
        retry_attempts,
        health_checks,
        memory_usage,
        active_connections,
    })
}

#[cfg(feature = "metrics")]
fn build_disabled_metrics_template() -> Result<PrometheusMetrics, MetricsError> {
    build_prometheus_metrics(&PrometheusMetricNames {
        batches: "disabled_batches",
        entries: "disabled_entries",
        latency: "disabled_latency",
        latency_label: "range",
        disk: "disabled_disk",
        retry: "disabled_retry",
        retry_label: "attempt",
        health: "disabled_health",
        memory: "disabled_memory",
        connections: "disabled_connections",
        register_in_registry: false,
    })
}

/// Returns the process-wide disabled-metrics template, building it at most once.
#[cfg(feature = "metrics")]
fn disabled_metrics_template() -> Option<&'static PrometheusMetrics> {
    static TEMPLATE: OnceLock<Option<PrometheusMetrics>> = OnceLock::new();
    TEMPLATE
        .get_or_init(|| match build_disabled_metrics_template() {
            Ok(template) => Some(template),
            Err(err) => {
                tracing::error!("Failed to build disabled-metrics template: {err}");
                None
            }
        })
        .as_ref()
}

fn empty_metrics_state() -> Arc<RwLock<MetricsState>> {
    Arc::new(RwLock::new(MetricsState {
        total_batches_sent: 0,
        successful_batches: 0,
        failed_batches: 0,
        retry_count: 0,
        health_check_total: 0,
        health_check_success: 0,
        health_check_failure: 0,
    }))
}

impl MetricsCollector {
    /// Memory-safe metrics initialization with zero expect() calls.
    pub fn new(config: &MetricsConfig) -> Result<Self, MetricsError> {
        #[cfg(feature = "metrics")]
        {
            let config_clone = config.clone();
            safe_metrics_operation(move || {
                let prometheus = build_prometheus_metrics(&PrometheusMetricNames {
                    batches: "rask_batches_sent_total",
                    entries: "rask_entries_sent_total",
                    latency: "rask_transmission_latency_seconds",
                    latency_label: "batch_size_range",
                    disk: "rask_disk_fallback_total",
                    retry: "rask_retry_attempts_total",
                    retry_label: "attempt_number",
                    health: "rask_health_checks_total",
                    memory: "rask_memory_usage_bytes",
                    connections: "rask_active_connections",
                    register_in_registry: true,
                })?;

                Ok(Self {
                    config: config_clone.clone(),
                    prometheus: Some(prometheus),
                    state: empty_metrics_state(),
                })
            })
        }

        #[cfg(not(feature = "metrics"))]
        {
            Ok(Self {
                config: config.clone(),
                state: empty_metrics_state(),
            })
        }
    }

    /// Legacy constructor for backward compatibility - logs errors instead of panicking.
    pub fn new_legacy(config: MetricsConfig) -> Self {
        match Self::new(&config) {
            Ok(collector) => collector,
            Err(e) => {
                tracing::error!(
                    "Failed to initialize metrics collector: {}, disabling metrics",
                    e
                );
                #[cfg(feature = "metrics")]
                {
                    Self {
                        config: MetricsConfig {
                            enabled: false,
                            ..config
                        },
                        prometheus: disabled_metrics_template().cloned(),
                        state: empty_metrics_state(),
                    }
                }
                #[cfg(not(feature = "metrics"))]
                {
                    Self {
                        config: MetricsConfig {
                            enabled: false,
                            ..config
                        },
                        state: empty_metrics_state(),
                    }
                }
            }
        }
    }

    pub async fn record_batch_sent(
        &mut self,
        entry_count: usize,
        success: bool,
        latency: Duration,
    ) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            let status = if success { "success" } else { "failure" };
            prom.batches_sent.with_label_values(&[status]).inc();
            prom.entries_sent.inc_by(entry_count as f64);

            let size_range = match entry_count {
                0..=100 => "small",
                101..=1000 => "medium",
                _ => "large",
            };

            prom.transmission_latency
                .with_label_values(&[size_range])
                .observe(latency.as_secs_f64());
        }

        let mut state = self.state.write().await;
        state.total_batches_sent += 1;
        if success {
            state.successful_batches += 1;
        } else {
            state.failed_batches += 1;
        }
    }

    pub fn record_disk_fallback(&self, entry_count: usize) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            prom.disk_fallback_counter.inc();
        }

        tracing::info!("Batch with {} entries stored to disk fallback", entry_count);
    }

    pub async fn record_retry_attempt(&mut self, _batch_id: &str, attempt_number: u32) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            prom.retry_attempts
                .with_label_values(&[&attempt_number.to_string()])
                .inc();
        }

        let mut state = self.state.write().await;
        state.retry_count += 1;
    }

    pub async fn record_health_check_async(&self, success: bool) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            let status = if success { "success" } else { "failure" };
            prom.health_checks.with_label_values(&[status]).inc();
        }

        let mut state = self.state.write().await;
        state.health_check_total += 1;
        if success {
            state.health_check_success += 1;
        } else {
            state.health_check_failure += 1;
        }
    }

    pub fn record_health_check(&self, success: bool) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            let status = if success { "success" } else { "failure" };
            prom.health_checks.with_label_values(&[status]).inc();
        }

        // Use non-async updates for immediate recording
        let state = self.state.clone();
        tokio::spawn(async move {
            let mut state = state.write().await;
            state.health_check_total += 1;
            if success {
                state.health_check_success += 1;
            } else {
                state.health_check_failure += 1;
            }
        });
    }

    pub fn record_connection_stats(&self, active: usize, _max: usize) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            prom.active_connections.set(active as f64);
        }
    }

    pub fn update_memory_usage(&self, bytes: u64) {
        #[cfg(feature = "metrics")]
        if let Some(prom) = &self.prometheus {
            prom.memory_usage.set(bytes as f64);
        }
    }

    pub async fn snapshot(&self) -> MetricsSnapshot {
        let state = self.state.read().await;

        MetricsSnapshot {
            total_batches_sent: state.total_batches_sent,
            successful_batches: state.successful_batches,
            failed_batches: state.failed_batches,
            // Safety: Prometheus counters/gauges return f64 but contain non-negative integers
            #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
            total_entries_sent: {
                #[cfg(feature = "metrics")]
                {
                    self.prometheus
                        .as_ref()
                        .map_or(0, |p| p.entries_sent.get() as u64)
                }
                #[cfg(not(feature = "metrics"))]
                {
                    0
                }
            },
            #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
            disk_fallback_count: {
                #[cfg(feature = "metrics")]
                {
                    self.prometheus
                        .as_ref()
                        .map_or(0, |p| p.disk_fallback_counter.get() as u64)
                }
                #[cfg(not(feature = "metrics"))]
                {
                    0
                }
            },
            retry_attempts: state.retry_count,
            health_check_total: state.health_check_total,
            health_check_success: state.health_check_success,
            health_check_failure: state.health_check_failure,
            average_transmission_latency: Duration::ZERO, // Would be calculated from histogram
            #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
            current_memory_usage: {
                #[cfg(feature = "metrics")]
                {
                    self.prometheus
                        .as_ref()
                        .map_or(0, |p| p.memory_usage.get() as u64)
                }
                #[cfg(not(feature = "metrics"))]
                {
                    0
                }
            },
            #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
            active_connections: {
                #[cfg(feature = "metrics")]
                {
                    self.prometheus
                        .as_ref()
                        .map_or(0, |p| p.active_connections.get() as u64)
                }
                #[cfg(not(feature = "metrics"))]
                {
                    0
                }
            },
        }
    }

    pub fn get_metric_name(&self, base_name: &str) -> String {
        match base_name {
            "batches_sent" => "rask_batches_sent_total".to_string(),
            "latency" => "rask_latency_seconds".to_string(),
            _ => format!("rask_{base_name}"),
        }
    }

    pub async fn reset_metrics(&mut self) {
        let mut state = self.state.write().await;
        state.total_batches_sent = 0;
        state.successful_batches = 0;
        state.failed_batches = 0;
        state.retry_count = 0;
        state.health_check_total = 0;
        state.health_check_success = 0;
        state.health_check_failure = 0;
    }

    #[cfg(feature = "metrics")]
    pub fn export_metrics(&self) -> Result<String, MetricsError> {
        let Some(prom) = &self.prometheus else {
            return Ok("# Metrics disabled\n".to_string());
        };
        let encoder = TextEncoder::new();
        let metric_families = prom.registry.gather();

        let mut buffer = Vec::new();
        encoder.encode(&metric_families, &mut buffer)?;

        Ok(String::from_utf8_lossy(&buffer).to_string())
    }

    #[cfg(not(feature = "metrics"))]
    pub fn export_metrics(&self) -> Result<String, MetricsError> {
        Ok("# Metrics disabled\n".to_string())
    }
}

pub struct PrometheusExporter {
    config: MetricsConfig,
    collector: MetricsCollector,
}

impl PrometheusExporter {
    pub fn new(config: MetricsConfig, collector: MetricsCollector) -> Result<Self, MetricsError> {
        Ok(Self { config, collector })
    }

    pub fn export_metrics(&self) -> Result<String, MetricsError> {
        self.collector.export_metrics()
    }

    #[cfg(feature = "metrics")]
    pub async fn start_server(&self) -> Result<(), MetricsError> {
        if !self.config.enabled {
            return Ok(());
        }

        let collector = self.collector.clone();

        let metrics =
            warp::path!("metrics")
                .and(warp::get())
                .map(move || match collector.export_metrics() {
                    Ok(metrics_text) => warp::reply::with_header(
                        metrics_text,
                        "content-type",
                        "text/plain; version=0.0.4",
                    )
                    .into_response(),
                    Err(_) => warp::reply::with_status(
                        "Internal Server Error",
                        warp::http::StatusCode::INTERNAL_SERVER_ERROR,
                    )
                    .into_response(),
                });

        let health = warp::path!("health").and(warp::get()).map(|| "OK");

        let routes = metrics.or(health);

        tracing::info!(
            "Starting Prometheus metrics server on port {}",
            self.config.export_port
        );

        warp::serve(routes)
            .run(([0, 0, 0, 0], self.config.export_port))
            .await;

        Ok(())
    }

    #[cfg(not(feature = "metrics"))]
    pub async fn start_server(&self) -> Result<(), MetricsError> {
        tracing::warn!("Metrics feature is disabled");
        Ok(())
    }
}

#[cfg(test)]
mod new_legacy_tests {
    use super::*;

    /// Pins down that the fixed metric names/opts of the fallback template
    /// build, so the `None` branch of `disabled_metrics_template()` is not
    /// reachable in practice.
    #[cfg(feature = "metrics")]
    #[test]
    fn disabled_metrics_template_builds_successfully() {
        assert!(build_disabled_metrics_template().is_ok());
    }

    #[cfg(feature = "metrics")]
    #[test]
    fn disabled_metrics_template_is_cached_across_calls() {
        let first = disabled_metrics_template().expect("template should build");
        let second = disabled_metrics_template().expect("template should build");
        // Same process-wide instance: cloning per-call no longer re-runs
        // fallible construction (and therefore can no longer panic per-call).
        assert!(std::ptr::eq(first, second));
    }

    /// Exercises exactly the struct-literal construction `new_legacy`'s
    /// fallback branch performs (cloning the cached template), without
    /// needing to force `MetricsCollector::new` itself to fail.
    #[cfg(feature = "metrics")]
    #[test]
    fn disabled_collector_can_be_built_from_template_without_panicking() {
        let template = disabled_metrics_template()
            .expect("template should build")
            .clone();
        let collector = MetricsCollector {
            config: MetricsConfig {
                enabled: false,
                ..MetricsConfig::default()
            },
            prometheus: Some(template),
            state: empty_metrics_state(),
        };
        assert!(!collector.config.enabled);
    }

    /// `new_legacy` must never panic. In this environment `MetricsCollector::new`
    /// succeeds via its primary path (fixed valid metric names), so this
    /// mainly guards against a regression reintroducing an unconditional
    /// panic; the fallback branch itself is covered by the two tests above.
    #[test]
    fn new_legacy_never_panics() {
        for _ in 0..10 {
            let _collector = MetricsCollector::new_legacy(MetricsConfig::default());
        }
    }
}
