#[cfg(feature = "metrics")]
use prometheus::{Counter, CounterVec, Encoder, Gauge, HistogramVec, Registry, TextEncoder};
use std::sync::Arc;
use std::time::Duration;
use thiserror::Error;
use tokio::sync::RwLock;

#[cfg(feature = "metrics")]
use warp::{Filter, Reply};

#[derive(Error, Debug)]
pub enum MetricsError {
    #[cfg(feature = "metrics")]
    #[error("Prometheus error: {0}")]
    PrometheusError(#[from] prometheus::Error),
    #[error("HTTP server error: {0}")]
    HttpError(String),
    #[error("Metrics collection error: {0}")]
    CollectionError(String),
}

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
    registry: Arc<Registry>,

    // Prometheus metrics
    #[cfg(feature = "metrics")]
    batches_sent: CounterVec,
    #[cfg(feature = "metrics")]
    entries_sent: Counter,
    #[cfg(feature = "metrics")]
    transmission_latency: HistogramVec,
    #[cfg(feature = "metrics")]
    disk_fallback_counter: Counter,
    #[cfg(feature = "metrics")]
    retry_attempts: CounterVec,
    #[cfg(feature = "metrics")]
    health_checks: CounterVec,
    #[cfg(feature = "metrics")]
    memory_usage: Gauge,
    #[cfg(feature = "metrics")]
    active_connections: Gauge,

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

impl MetricsCollector {
    pub fn new(config: MetricsConfig) -> Self {
        #[cfg(feature = "metrics")]
        {
            let registry = Arc::new(Registry::new());

            // Initialize Prometheus metrics
            let batches_sent = CounterVec::new(
                prometheus::Opts::new("rask_batches_sent_total", "Total number of batches sent"),
                &["status"], // success, failure
            )
            .unwrap();
            registry.register(Box::new(batches_sent.clone())).unwrap();

            let entries_sent = Counter::new(
                "rask_entries_sent_total",
                "Total number of log entries sent",
            )
            .unwrap();
            registry.register(Box::new(entries_sent.clone())).unwrap();

            let transmission_latency = HistogramVec::new(
                prometheus::HistogramOpts::new(
                    "rask_transmission_latency_seconds",
                    "Transmission latency in seconds",
                ),
                &["batch_size_range"], // small, medium, large
            )
            .unwrap();
            registry
                .register(Box::new(transmission_latency.clone()))
                .unwrap();

            let disk_fallback_counter = Counter::new(
                "rask_disk_fallback_total",
                "Total number of batches stored to disk",
            )
            .unwrap();
            registry
                .register(Box::new(disk_fallback_counter.clone()))
                .unwrap();

            let retry_attempts = CounterVec::new(
                prometheus::Opts::new(
                    "rask_retry_attempts_total",
                    "Total number of retry attempts",
                ),
                &["attempt_number"],
            )
            .unwrap();
            registry.register(Box::new(retry_attempts.clone())).unwrap();

            let health_checks = CounterVec::new(
                prometheus::Opts::new("rask_health_checks_total", "Total number of health checks"),
                &["status"], // success, failure
            )
            .unwrap();
            registry.register(Box::new(health_checks.clone())).unwrap();

            let memory_usage =
                Gauge::new("rask_memory_usage_bytes", "Current memory usage in bytes").unwrap();
            registry.register(Box::new(memory_usage.clone())).unwrap();

            let active_connections = Gauge::new(
                "rask_active_connections",
                "Number of active HTTP connections",
            )
            .unwrap();
            registry
                .register(Box::new(active_connections.clone()))
                .unwrap();

            Self {
                config,
                registry,
                batches_sent,
                entries_sent,
                transmission_latency,
                disk_fallback_counter,
                retry_attempts,
                health_checks,
                memory_usage,
                active_connections,
                state: Arc::new(RwLock::new(MetricsState {
                    total_batches_sent: 0,
                    successful_batches: 0,
                    failed_batches: 0,
                    retry_count: 0,
                    health_check_total: 0,
                    health_check_success: 0,
                    health_check_failure: 0,
                })),
            }
        }

        #[cfg(not(feature = "metrics"))]
        {
            Self {
                config,
                state: Arc::new(RwLock::new(MetricsState {
                    total_batches_sent: 0,
                    successful_batches: 0,
                    failed_batches: 0,
                    retry_count: 0,
                    health_check_total: 0,
                    health_check_success: 0,
                    health_check_failure: 0,
                })),
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
        {
            let status = if success { "success" } else { "failure" };
            self.batches_sent.with_label_values(&[status]).inc();
            self.entries_sent.inc_by(entry_count as f64);

            let size_range = match entry_count {
                0..=100 => "small",
                101..=1000 => "medium",
                _ => "large",
            };

            self.transmission_latency
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
        {
            self.disk_fallback_counter.inc();
        }

        tracing::info!("Batch with {} entries stored to disk fallback", entry_count);
    }

    pub async fn record_retry_attempt(&mut self, _batch_id: &str, attempt_number: u32) {
        #[cfg(feature = "metrics")]
        {
            self.retry_attempts
                .with_label_values(&[&attempt_number.to_string()])
                .inc();
        }

        let mut state = self.state.write().await;
        state.retry_count += 1;
    }

    pub async fn record_health_check_async(&self, success: bool) {
        #[cfg(feature = "metrics")]
        {
            let status = if success { "success" } else { "failure" };
            self.health_checks.with_label_values(&[status]).inc();
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
        {
            let status = if success { "success" } else { "failure" };
            self.health_checks.with_label_values(&[status]).inc();
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
        {
            self.active_connections.set(active as f64);
        }
    }

    pub fn update_memory_usage(&self, bytes: u64) {
        #[cfg(feature = "metrics")]
        {
            self.memory_usage.set(bytes as f64);
        }
    }

    pub async fn snapshot(&self) -> MetricsSnapshot {
        let state = self.state.read().await;

        MetricsSnapshot {
            total_batches_sent: state.total_batches_sent,
            successful_batches: state.successful_batches,
            failed_batches: state.failed_batches,
            total_entries_sent: {
                #[cfg(feature = "metrics")]
                {
                    self.entries_sent.get() as u64
                }
                #[cfg(not(feature = "metrics"))]
                {
                    0
                }
            },
            disk_fallback_count: {
                #[cfg(feature = "metrics")]
                {
                    self.disk_fallback_counter.get() as u64
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
            current_memory_usage: {
                #[cfg(feature = "metrics")]
                {
                    self.memory_usage.get() as u64
                }
                #[cfg(not(feature = "metrics"))]
                {
                    0
                }
            },
            active_connections: {
                #[cfg(feature = "metrics")]
                {
                    self.active_connections.get() as u64
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
            _ => format!("rask_{}", base_name),
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
        let encoder = TextEncoder::new();
        let metric_families = self.registry.gather();

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
    pub async fn new(
        config: MetricsConfig,
        collector: MetricsCollector,
    ) -> Result<Self, MetricsError> {
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
