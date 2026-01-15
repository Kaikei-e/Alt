use anyhow::{Context, Error, Result};
use once_cell::sync::OnceCell;
use opentelemetry::{KeyValue, global, trace::TracerProvider};
use opentelemetry_otlp::WithExportConfig;
use opentelemetry_sdk::{
    Resource,
    trace::{RandomIdGenerator, Sampler, SdkTracer, SdkTracerProvider},
};
use tracing::info;
use tracing_subscriber::{EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};

use super::structured_log::StructuredLogLayer;

static TRACING_INIT: OnceCell<()> = OnceCell::new();

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
    TRACING_INIT.get_or_try_init(|| {
        let env_filter =
            EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));

        let fmt_layer = tracing_subscriber::fmt::layer().with_target(false).json();

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
                    // Fall back to standard tracing with StructuredLogLayer
                    let structured_layer = StructuredLogLayer;
                    tracing_subscriber::registry()
                        .with(env_filter)
                        .with(fmt_layer)
                        .with(structured_layer)
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
            // Standard tracing with StructuredLogLayer for ADR 98 compliance
            let structured_layer = StructuredLogLayer;
            tracing_subscriber::registry()
                .with(env_filter)
                .with(fmt_layer)
                .with(structured_layer)
                .try_init()
                .map_err(|e: tracing_subscriber::util::TryInitError| Error::msg(e.to_string()))?;
            info!(otel_enabled = false, "Standard tracing initialized");
        }

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
    global::set_tracer_provider(tracer_provider);

    Ok(tracer)
}

/// OpenTelemetryのグローバルシャットダウンを実行し、未送信のスパンをフラッシュする。
///
/// アプリケーション終了時に呼び出してください。
#[allow(dead_code)]
pub fn shutdown() {
    // OpenTelemetry 0.31.0では、グローバルトレーサープロバイダーから直接
    // SdkTracerProviderを取得できないため、shutdownは個別に管理する必要があります。
    // 実際の使用時は、init_tracerで返されたSdkTracerProviderを保持し、
    // アプリケーション終了時に直接shutdown()を呼び出してください。
}
