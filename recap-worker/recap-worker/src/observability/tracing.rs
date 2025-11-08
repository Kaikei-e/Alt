use anyhow::{Context, Error, Result};
use once_cell::sync::OnceCell;
use opentelemetry::{KeyValue, global, trace::TracerProvider};
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::{
    Resource,
    trace::{RandomIdGenerator, Sampler, Tracer},
};
use tracing::info;
use tracing_subscriber::{EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};

static TRACING_INIT: OnceCell<()> = OnceCell::new();

/// Tracing サブスクライバを一度だけ初期化する。
///
/// OTel設定が提供されている場合、OTLPエクスポーターを使用してトレースを送信します。
/// 設定がない場合は、標準のfmtレイヤーのみを使用します。
///
/// # Errors
/// サブスクライバの初期化に失敗した場合はエラーを返す。
pub fn init() -> Result<()> {
    TRACING_INIT.get_or_try_init(|| {
        let env_filter =
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));

        let fmt_layer = tracing_subscriber::fmt::layer().with_target(false).json();

        // Note: OpenTelemetryは現在バージョンミスマッチのため無効化
        tracing_subscriber::registry()
            .with(env_filter)
            .with(fmt_layer)
            .try_init()
            .map_err(|error| Error::msg(error.to_string()))?;

        info!("Standard tracing initialized");

        Ok::<(), Error>(())
    })?;
    Ok(())
}

/// OTLPエクスポーター経由でOpenTelemetryトレーサーを初期化する。
///
/// サンプリング比率はOTEL_SAMPLING_RATIO環境変数で制御（デフォルト1.0 = 全トレース）。
///
/// # Errors
/// トレーサーの初期化に失敗した場合はエラーを返す。
#[allow(dead_code)]
fn init_tracer(endpoint: &str) -> Result<Tracer> {
    let sampling_ratio = std::env::var("OTEL_SAMPLING_RATIO")
        .ok()
        .and_then(|s| s.parse::<f64>().ok())
        .unwrap_or(1.0);

    let exporter = opentelemetry_otlp::SpanExporter::builder()
        .with_tonic()
        .with_endpoint(endpoint)
        .build()
        .context("failed to build OTLP span exporter")?;

    let tracer = opentelemetry_sdk::trace::TracerProvider::builder()
        .with_batch_exporter(exporter, opentelemetry_sdk::runtime::Tokio)
        .with_sampler(Sampler::TraceIdRatioBased(sampling_ratio))
        .with_id_generator(RandomIdGenerator::default())
        .with_resource(Resource::new(vec![
            KeyValue::new("service.name", "recap-worker"),
            KeyValue::new("service.version", env!("CARGO_PKG_VERSION")),
        ]))
        .build()
        .tracer("recap-worker");

    Ok(tracer)
}

/// OpenTelemetryのグローバルシャットダウンを実行し、未送信のスパンをフラッシュする。
///
/// アプリケーション終了時に呼び出してください。
#[allow(dead_code)]
pub fn shutdown() {
    global::shutdown_tracer_provider();
}
