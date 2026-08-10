pub(crate) mod metrics;
pub(crate) mod structured_log;
pub(crate) mod tracing;

use std::sync::Arc;

use anyhow::Result;
use prometheus::{Encoder, Registry, TextEncoder};

use self::metrics::Metrics;

/// Telemetry（メトリクスとトレーシング）を管理する構造体。
#[derive(Debug, Clone)]
pub struct Telemetry {
    metrics: Arc<Metrics>,
    /// Held because it is the only place this service's instruments live:
    /// `Metrics::new` registers into it rather than into the crate-global
    /// default registry, so dropping it would leave the exposition empty.
    registry: Arc<Registry>,
}

impl Telemetry {
    /// 新しいTelemetryインスタンスを作成し、トレーシングとメトリクスを初期化する。
    pub fn new() -> Result<Self> {
        tracing::init()?;
        let registry = Arc::new(Registry::new());
        let metrics = Arc::new(Metrics::new(Arc::clone(&registry))?);
        Ok(Self { metrics, registry })
    }

    /// メトリクスへのアクセスを提供する。
    pub fn metrics(&self) -> &Metrics {
        &self.metrics
    }

    pub(crate) fn metrics_arc(&self) -> Arc<Metrics> {
        Arc::clone(&self.metrics)
    }

    /// 準備完了プローブを記録する。
    pub fn record_ready_probe(&self) {
        ::tracing::info!("service ready probe recorded");
    }

    /// ライブプローブを記録する。
    pub fn record_live_probe(&self) {
        ::tracing::debug!("service live probe");
    }

    /// 管理者リトライ呼び出しを記録する。
    pub fn record_admin_retry_invocation(&self) {
        ::tracing::warn!("admin retry invoked");
    }

    /// 手動生成呼び出しを記録する。
    pub fn record_manual_generate_invocation(&self) {
        ::tracing::info!("manual generation invoked");
    }

    /// アプリケーション終了時にOTLPトレースをフラッシュする。
    ///
    /// SIGTERM/SIGINT を受けたグレースフルシャットダウンの最終ステップとして
    /// `main.rs` から一度だけ呼び出す。
    pub fn shutdown(&self) {
        tracing::shutdown();
    }

    /// Prometheusメトリクスをレンダリングする。
    ///
    /// `prometheus::gather()` reads the crate-global default registry, which
    /// this service registers nothing into. Gathering from the owned registry
    /// is what makes the exposition non-empty.
    pub fn render_prometheus(&self) -> String {
        let encoder = TextEncoder::new();
        let metric_families = self.registry.gather();
        let mut buffer = Vec::new();
        encoder.encode(&metric_families, &mut buffer).ok();
        String::from_utf8(buffer).unwrap_or_default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The instruments are registered into a registry this struct owns, so
    /// rendering has to read that registry. Reading the crate-global default
    /// one instead returns an exposition that parses, scrapes and alerts
    /// cleanly while carrying none of this service's metrics — which is how
    /// the notification relay's gauges were absent from Prometheus while the
    /// target reported healthy.
    #[test]
    fn render_prometheus_exposes_the_registered_instruments() {
        let registry = Arc::new(Registry::new());
        let metrics = Arc::new(Metrics::new(Arc::clone(&registry)).expect("register metrics"));
        let telemetry = Telemetry {
            metrics,
            registry: Arc::clone(&registry),
        };

        let rendered = telemetry.render_prometheus();

        assert!(
            rendered.contains("notification_outbox_oldest_pending_age_seconds"),
            "the relay gauge the push alerts guard on must be exposed, got:\n{rendered}"
        );
        assert!(
            rendered.contains("notification_outbox_last_tick_timestamp_seconds"),
            "the relay liveness gauge must be exposed, got:\n{rendered}"
        );
    }

    /// Every instrument, not just the two the push alerts need. A partial
    /// exposition is the same failure in a smaller costume.
    #[test]
    fn render_prometheus_exposes_every_registered_family() {
        let registry = Arc::new(Registry::new());
        let metrics = Arc::new(Metrics::new(Arc::clone(&registry)).expect("register metrics"));
        let telemetry = Telemetry {
            metrics,
            registry: Arc::clone(&registry),
        };

        let registered = registry.gather().len();
        assert!(registered > 0, "the fixture registers nothing");

        let rendered = telemetry.render_prometheus();
        let exposed = rendered
            .lines()
            .filter(|line| line.starts_with("# TYPE "))
            .count();

        assert_eq!(
            exposed, registered,
            "every registered family has to reach the exposition"
        );
    }
}
