use anyhow::{Context, Result};
use metrics::{Counter, counter};
use metrics_exporter_prometheus::{PrometheusBuilder, PrometheusHandle};
use once_cell::sync::OnceCell;

static PROMETHEUS_HANDLE: OnceCell<PrometheusHandle> = OnceCell::new();

#[derive(Clone)]
pub struct Telemetry {
    handle: PrometheusHandle,
    ready_counter: Counter,
    live_counter: Counter,
    admin_retry_counter: Counter,
    manual_generate_counter: Counter,
}

impl Telemetry {
    pub(crate) fn new() -> Result<Self> {
        let handle = PROMETHEUS_HANDLE
            .get_or_try_init(|| {
                let handle = PrometheusBuilder::new()
                    .install_recorder()
                    .context("failed to install Prometheus recorder")?;

                metrics::describe_counter!(
                    "recap_worker_health_ready_total",
                    "Number of readiness checks performed"
                );
                metrics::describe_counter!(
                    "recap_worker_health_live_total",
                    "Number of liveness checks performed"
                );
                metrics::describe_counter!(
                    "recap_worker_admin_retry_total",
                    "Number of admin retry invocations"
                );
                metrics::describe_counter!(
                    "recap_worker_generate_manual_total",
                    "Number of manual 7days recap generation requests"
                );

                Ok::<PrometheusHandle, anyhow::Error>(handle)
            })?
            .clone();

        let ready_counter = counter!("recap_worker_health_ready_total");
        let live_counter = counter!("recap_worker_health_live_total");
        let admin_retry_counter = counter!("recap_worker_admin_retry_total");
        let manual_generate_counter = counter!("recap_worker_generate_manual_total");

        Ok(Self {
            handle,
            ready_counter,
            live_counter,
            admin_retry_counter,
            manual_generate_counter,
        })
    }

    pub(crate) fn render_prometheus(&self) -> String {
        self.handle.render()
    }

    pub(crate) fn record_ready_probe(&self) {
        self.ready_counter.increment(1);
    }

    pub(crate) fn record_live_probe(&self) {
        self.live_counter.increment(1);
    }

    pub(crate) fn record_admin_retry_invocation(&self) {
        self.admin_retry_counter.increment(1);
    }

    pub(crate) fn record_manual_generate_invocation(&self) {
        self.manual_generate_counter.increment(1);
    }
}
