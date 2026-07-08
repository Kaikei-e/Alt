use anyhow::{Context, Error, Result};
use opentelemetry::{KeyValue, global, trace::TracerProvider};
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::{
    Resource,
    trace::{RandomIdGenerator, Sampler, SdkTracer, SdkTracerProvider},
};
use std::sync::{Once, OnceLock};
use tracing::info;
use tracing_subscriber::{EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};

use super::structured_log::Adr98JsonFormat;

static TRACING_INIT: Once = Once::new();

/// Retains the `SdkTracerProvider` built by `init_tracer` so `shutdown()` can
/// flush its batch exporter. `global::set_tracer_provider` only stores a
/// type-erased `dyn TracerProvider`, which has no `shutdown` in its trait
/// object surface — without this, in-flight spans in the batch exporter were
/// silently dropped on every process exit.
static TRACER_PROVIDER: OnceLock<SdkTracerProvider> = OnceLock::new();

/// Tracing サブスクライバを一度だけ初期化する。
///
/// OTEL_EXPORTER_OTLP_ENDPOINT環境変数が設定されている場合、
/// OTLPエクスポーターを使用してトレースを送信します。
/// 設定がない場合は、標準のfmtレイヤーのみを使用します。
///
/// StructuredLogLayerは常に有効化され、ADR 98準拠の
/// alt.* プレフィックス付きフィールドを出力します。
///
/// # Errors
/// サブスクライバの初期化に失敗した場合はエラーを返す。
pub fn init() -> Result<()> {
    let mut result = Ok(());
    TRACING_INIT.call_once(|| {
        if let Err(e) = do_init() {
            result = Err(e);
        }
    });
    result
}

fn do_init() -> Result<()> {
    let env_filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));

    // `Adr98JsonFormat` is the sole JSON formatter: it *is* the fmt layer's
    // event formatter (not a second, separately-registered layer), so every
    // event is written exactly once, in ADR 98's alt.*-prefixed shape.
    let fmt_layer = tracing_subscriber::fmt::layer()
        .with_target(false)
        .event_format(Adr98JsonFormat);

    // Check if OTel is enabled via environment variable
    let otel_endpoint = std::env::var("OTEL_EXPORTER_OTLP_ENDPOINT").ok();

    if let Some(endpoint) = otel_endpoint {
        // Initialize with OpenTelemetry
        // When OTel is enabled, use OTel layer for trace context propagation
        match init_tracer(&endpoint) {
            Ok(tracer) => {
                let otel_layer = tracing_opentelemetry::layer().with_tracer(tracer);
                tracing_subscriber::registry()
                    .with(env_filter)
                    .with(fmt_layer)
                    .with(otel_layer)
                    .try_init()
                    .map_err(|e: tracing_subscriber::util::TryInitError| {
                        Error::msg(e.to_string())
                    })?;
                info!(
                    otel_enabled = true,
                    endpoint = %endpoint,
                    "alt.ai.pipeline" = "recap-processing",
                    "Tracing initialized with OpenTelemetry"
                );
            }
            Err(e) => {
                // Fall back to standard tracing; `fmt_layer` already applies
                // the ADR 98 formatting via `Adr98JsonFormat`.
                tracing_subscriber::registry()
                    .with(env_filter)
                    .with(fmt_layer)
                    .try_init()
                    .map_err(|e: tracing_subscriber::util::TryInitError| {
                        Error::msg(e.to_string())
                    })?;
                info!(
                    otel_enabled = false,
                    error = %e,
                    "Tracing initialized without OpenTelemetry (init failed)"
                );
            }
        }
    } else {
        // Standard tracing; `fmt_layer` already applies the ADR 98
        // alt.*-prefixed formatting via `Adr98JsonFormat`.
        tracing_subscriber::registry()
            .with(env_filter)
            .with(fmt_layer)
            .try_init()
            .map_err(|e: tracing_subscriber::util::TryInitError| Error::msg(e.to_string()))?;
        info!(otel_enabled = false, "Standard tracing initialized");
    }

    Ok(())
}

/// OTLPエクスポーター経由でOpenTelemetryトレーサーを初期化する。
///
/// サンプリング比率はOTEL_SAMPLING_RATIO環境変数で制御（デフォルト1.0 = 全トレース）。
///
/// # Errors
/// トレーサーの初期化に失敗した場合はエラーを返す。
fn init_tracer(endpoint: &str) -> Result<SdkTracer> {
    let sampling_ratio = std::env::var("OTEL_SAMPLING_RATIO")
        .ok()
        .and_then(|s| s.parse::<f64>().ok())
        .unwrap_or(1.0);

    let exporter = opentelemetry_otlp::SpanExporter::builder()
        .with_tonic()
        .with_endpoint(endpoint)
        .build()
        .context("failed to build OTLP span exporter")?;

    let resource = Resource::builder()
        .with_attributes([
            KeyValue::new("service.name", "recap-worker"),
            KeyValue::new("service.version", env!("CARGO_PKG_VERSION")),
        ])
        .build();

    let tracer_provider = SdkTracerProvider::builder()
        .with_batch_exporter(exporter)
        .with_sampler(Sampler::TraceIdRatioBased(sampling_ratio))
        .with_id_generator(RandomIdGenerator::default())
        .with_resource(resource)
        .build();

    let tracer = tracer_provider.tracer("recap-worker");

    // グローバルトレーサープロバイダーを設定
    global::set_tracer_provider(tracer_provider.clone());
    // Retain our own typed handle too — `global::set_tracer_provider` only
    // stores it behind `dyn TracerProvider`, which can't be shut down.
    let _ = TRACER_PROVIDER.set(tracer_provider);

    Ok(tracer)
}

/// OpenTelemetryのグローバルシャットダウンを実行し、未送信のスパンをフラッシュする。
///
/// アプリケーション終了時に呼び出してください。
pub fn shutdown() {
    if let Some(provider) = TRACER_PROVIDER.get() {
        if let Err(e) = provider.shutdown() {
            tracing::warn!(error = ?e, "failed to shut down OTel tracer provider cleanly");
        }
    }
}
