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
}

impl Telemetry {
    /// 新しいTelemetryインスタンスを作成し、トレーシングとメトリクスを初期化する。
    pub fn new() -> Result<Self> {
        tracing::init()?;
        let registry = Arc::new(Registry::new());
        let metrics = Arc::new(Metrics::new(Arc::clone(&registry))?);
        Ok(Self { metrics })
    }

    /// メトリクスへのアクセスを提供する。
    pub fn metrics(&self) -> &Metrics {
        &self.metrics
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

    /// Prometheusメトリクスをレンダリングする。
    pub fn render_prometheus(&self) -> String {
        let encoder = TextEncoder::new();
        let metric_families = prometheus::gather();
        let mut buffer = Vec::new();
        encoder.encode(&metric_families, &mut buffer).ok();
        String::from_utf8(buffer).unwrap_or_default()
    }
}
