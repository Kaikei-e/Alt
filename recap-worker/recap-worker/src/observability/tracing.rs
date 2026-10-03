use anyhow::{Context, Error, Result};
use opentelemetry::{KeyValue, global, trace::TracerProvider};
use opentelemetry_otlp::{WithExportConfig, WithHttpConfig};
use opentelemetry_sdk::{
    Resource,
    trace::{RandomIdGenerator, Sampler, SdkTracer, SdkTracerProvider},
};
use std::sync::OnceLock;
use tracing::info;
use tracing_subscriber::{EnvFilter, layer::SubscriberExt, util::SubscriberInitExt};

use super::structured_log::Adr98JsonFormat;

static TRACING_INIT_RESULT: OnceLock<Result<(), String>> = OnceLock::new();

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
    let res = TRACING_INIT_RESULT.get_or_init(|| do_init().map_err(|e| format!("{e:#}")));
    match res {
        Ok(()) => Ok(()),
        Err(e) => Err(anyhow::anyhow!("Tracing initialization failed: {e}")),
    }
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
        // When OTel is enabled, use OTel layer for trace context propagation.
        // Startup/config errors MUST fail fast (not fallback success)!
        let tracer = init_tracer(&endpoint)?;
        let otel_layer = tracing_opentelemetry::layer().with_tracer(tracer);
        tracing_subscriber::registry()
            .with(env_filter)
            .with(fmt_layer)
            .with(otel_layer)
            .try_init()
            .map_err(|e: tracing_subscriber::util::TryInitError| Error::msg(e.to_string()))?;
        info!(
            otel_enabled = true,
            endpoint = %endpoint,
            "alt.ai.pipeline" = "recap-processing",
            "Tracing initialized with OpenTelemetry"
        );
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

/// Normalize endpoint to explicit /v1/traces path.
/// SDK with_endpoint(base) ASIS posts '/' otherwise.
pub fn normalize_trace_endpoint(endpoint: &str) -> String {
    let trimmed = endpoint.trim();
    if trimmed.ends_with("/v1/traces") {
        trimmed.to_string()
    } else {
        format!("{}/v1/traces", trimmed.trim_end_matches('/'))
    }
}

/// Load and validate ingest token from RASK_INGEST_TOKEN_FILE
/// or documented alias OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE.
/// Rejects missing, empty, or non-RFC6750 tokens without leaking token value in errors.
pub fn load_ingest_token() -> Result<String> {
    let ingest_token_file = std::env::var("RASK_INGEST_TOKEN_FILE")
        .or_else(|_| std::env::var("OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE"))
        .unwrap_or_else(|_| "/run/secrets/rask_ingest_token".to_string());

    let raw = std::fs::read_to_string(&ingest_token_file).context(format!(
        "Failed to read ingest token file: {}",
        ingest_token_file
    ))?;
    let token = raw.trim().to_string();

    if token.is_empty() {
        anyhow::bail!("Ingest token in {} is empty", ingest_token_file);
    }
    if !token.is_ascii() {
        anyhow::bail!(
            "Ingest token in {} contains non-ASCII characters",
            ingest_token_file
        );
    }
    if token.contains('\r') || token.contains('\n') {
        anyhow::bail!(
            "Ingest token in {} contains embedded CR/LF characters",
            ingest_token_file
        );
    }
    if token.bytes().any(|b| b < 0x20 || b == 0x7F) {
        anyhow::bail!(
            "Ingest token in {} contains control characters",
            ingest_token_file
        );
    }
    if !is_rfc6750_token68(&token) {
        anyhow::bail!(
            "Ingest token in {} does not conform to RFC 6750 token68 syntax",
            ingest_token_file
        );
    }

    Ok(token)
}

/// Validates that a token adheres to RFC 6750 Section 2.1 token68 syntax:
/// `1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="`
pub fn is_rfc6750_token68(token: &str) -> bool {
    if token.is_empty() {
        return false;
    }
    let mut non_padding_count = 0usize;
    let mut seen_equal = false;
    for b in token.bytes() {
        if b == b'=' {
            seen_equal = true;
        } else if seen_equal {
            return false;
        } else if b.is_ascii_alphanumeric()
            || b == b'-'
            || b == b'.'
            || b == b'_'
            || b == b'~'
            || b == b'+'
            || b == b'/'
        {
            non_padding_count += 1;
        } else {
            return false;
        }
    }
    non_padding_count >= 1
}

/// Build an OTLP HTTP SpanExporter with custom reqwest client enforcing:
/// - no redirects
/// - finite timeout (10s)
/// - explicit /v1/traces path
/// - Authorization: Bearer <token>
pub fn build_span_exporter(
    endpoint: &str,
    token: &str,
) -> Result<opentelemetry_otlp::SpanExporter> {
    let trace_endpoint = normalize_trace_endpoint(endpoint);

    let mut headers = std::collections::HashMap::new();
    headers.insert("Authorization".to_string(), format!("Bearer {}", token));

    let http_client = reqwest::Client::builder()
        .redirect(reqwest::redirect::Policy::none())
        .timeout(std::time::Duration::from_secs(10))
        .build()
        .context("Failed to build HTTP client for OTLP exporter")?;

    let exporter = opentelemetry_otlp::SpanExporter::builder()
        .with_http()
        .with_http_client(http_client)
        .with_endpoint(&trace_endpoint)
        .with_headers(headers)
        .with_timeout(std::time::Duration::from_secs(10))
        .build()
        .context("failed to build OTLP span exporter")?;

    Ok(exporter)
}

/// Wrapper around SpanExporter that enters the Tokio runtime context
/// before awaiting exporter futures. This prevents "no reactor running"
/// panics when BatchSpanProcessor's background worker thread executes async export.
#[derive(Debug, Clone)]
pub struct TokioSpanExporter {
    inner: std::sync::Arc<opentelemetry_otlp::SpanExporter>,
    handle: Option<tokio::runtime::Handle>,
}

impl TokioSpanExporter {
    pub fn new(inner: opentelemetry_otlp::SpanExporter) -> Self {
        Self {
            inner: std::sync::Arc::new(inner),
            handle: tokio::runtime::Handle::try_current().ok(),
        }
    }
}

impl opentelemetry_sdk::trace::SpanExporter for TokioSpanExporter {
    async fn export(
        &self,
        batch: Vec<opentelemetry_sdk::trace::SpanData>,
    ) -> opentelemetry_sdk::error::OTelSdkResult {
        if let Some(handle) = &self.handle {
            let inner = self.inner.clone();
            handle
                .spawn(async move { inner.export(batch).await })
                .await
                .map_err(|e| {
                    opentelemetry_sdk::error::OTelSdkError::InternalFailure(format!(
                        "Task spawn failed: {e}"
                    ))
                })?
        } else {
            self.inner.export(batch).await
        }
    }
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

    let token = load_ingest_token()?;
    let exporter = build_span_exporter(endpoint, &token)?;
    let tokio_exporter = TokioSpanExporter::new(exporter);

    let resource = Resource::builder()
        .with_attributes([
            KeyValue::new("service.name", "recap-worker"),
            KeyValue::new("service.version", env!("CARGO_PKG_VERSION")),
        ])
        .build();

    let tracer_provider = SdkTracerProvider::builder()
        .with_batch_exporter(tokio_exporter)
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

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;
    use wiremock::{
        Mock, MockServer, ResponseTemplate,
        matchers::{header, method, path},
    };

    #[test]
    fn test_normalize_trace_endpoint_appends_v1_traces() {
        assert_eq!(
            normalize_trace_endpoint("http://localhost:4318"),
            "http://localhost:4318/v1/traces"
        );
        assert_eq!(
            normalize_trace_endpoint("http://localhost:4318/"),
            "http://localhost:4318/v1/traces"
        );
        assert_eq!(
            normalize_trace_endpoint("http://localhost:4318/v1/traces"),
            "http://localhost:4318/v1/traces"
        );
    }

    #[test]
    fn test_load_ingest_token_missing_file_errors() {
        temp_env::with_vars(
            [
                ("RASK_INGEST_TOKEN_FILE", Some("/nonexistent/token/path")),
                ("OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE", None),
            ],
            || {
                let err = load_ingest_token().unwrap_err();
                assert!(err.to_string().contains("Failed to read"));
            },
        );
    }

    #[test]
    fn test_load_ingest_token_empty_file_errors() {
        let mut file = NamedTempFile::new().unwrap();
        writeln!(file, "   ").unwrap();
        temp_env::with_vars(
            [
                (
                    "RASK_INGEST_TOKEN_FILE",
                    Some(file.path().to_str().unwrap()),
                ),
                ("OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE", None),
            ],
            || {
                let err = load_ingest_token().unwrap_err();
                assert!(err.to_string().contains("empty"));
            },
        );
    }

    #[test]
    fn test_load_ingest_token_invalid_chars_errors() {
        let mut file = NamedTempFile::new().unwrap();
        writeln!(file, "invalid\x01token").unwrap();
        temp_env::with_vars(
            [
                (
                    "RASK_INGEST_TOKEN_FILE",
                    Some(file.path().to_str().unwrap()),
                ),
                ("OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE", None),
            ],
            || {
                let err = load_ingest_token().unwrap_err();
                assert!(err.to_string().contains("control characters"));
            },
        );
    }

    #[test]
    fn test_load_ingest_token_invalid_token68_errors() {
        let mut file = NamedTempFile::new().unwrap();
        writeln!(file, "invalid$token").unwrap();
        temp_env::with_vars(
            [
                (
                    "RASK_INGEST_TOKEN_FILE",
                    Some(file.path().to_str().unwrap()),
                ),
                ("OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE", None),
            ],
            || {
                let err = load_ingest_token().unwrap_err();
                assert!(err.to_string().contains("token68"));
                // Must not leak token value in error message
                assert!(!err.to_string().contains("invalid$token"));
            },
        );
    }

    #[test]
    fn test_load_ingest_token_unifies_rask_and_otel_alias() {
        let mut file = NamedTempFile::new().unwrap();
        writeln!(file, "valid-secret-token-xyz").unwrap();

        // 1. RASK_INGEST_TOKEN_FILE
        temp_env::with_vars(
            [
                (
                    "RASK_INGEST_TOKEN_FILE",
                    Some(file.path().to_str().unwrap()),
                ),
                ("OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE", None),
            ],
            || {
                let token = load_ingest_token().unwrap();
                assert_eq!(token, "valid-secret-token-xyz");
            },
        );

        // 2. OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE (alias)
        temp_env::with_vars(
            [
                ("RASK_INGEST_TOKEN_FILE", None),
                (
                    "OTEL_EXPORTER_OTLP_INGEST_TOKEN_FILE",
                    Some(file.path().to_str().unwrap()),
                ),
            ],
            || {
                let token = load_ingest_token().unwrap();
                assert_eq!(token, "valid-secret-token-xyz");
            },
        );
    }

    #[test]
    fn test_token68_padding_only_rejected() {
        assert!(!is_rfc6750_token68("="));
        assert!(!is_rfc6750_token68("=="));
        assert!(!is_rfc6750_token68("==="));
        assert!(!is_rfc6750_token68("===="));
    }

    #[test]
    fn test_token68_malformed_rejected() {
        assert!(!is_rfc6750_token68(""));
        assert!(!is_rfc6750_token68("=abc"));
        assert!(!is_rfc6750_token68("abc=def"));
        assert!(!is_rfc6750_token68("abc==d"));
        assert!(!is_rfc6750_token68("abc def"));
        assert!(!is_rfc6750_token68("abc\ndef"));
        assert!(!is_rfc6750_token68("abc\rdef"));
        assert!(!is_rfc6750_token68("abc$token"));
    }

    #[test]
    fn test_token68_non_ascii_rejected() {
        assert!(!is_rfc6750_token68("token\u{1f600}"));
        assert!(!is_rfc6750_token68("tokén"));
        assert!(!is_rfc6750_token68("トークン"));
    }

    #[test]
    fn test_token68_valid_accepted() {
        assert!(is_rfc6750_token68("a"));
        assert!(is_rfc6750_token68("valid_token"));
        assert!(is_rfc6750_token68("valid-token.123~_"));
        assert!(is_rfc6750_token68("valid+token/abc="));
        assert!(is_rfc6750_token68("valid+token/abc=="));
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn test_actual_exporter_wiremock_export_path_auth_body_and_no_redirects() {
        use opentelemetry::trace::Tracer;
        use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
        use prost::Message;

        let mock_server = MockServer::start().await;
        let token = "test-recap-telemetry-token-123";

        // Setup mock that STRICTLY checks POST to /v1/traces with Bearer token
        Mock::given(method("POST"))
            .and(path("/v1/traces"))
            .and(header("authorization", format!("Bearer {token}").as_str()))
            .respond_with(ResponseTemplate::new(200))
            .mount(&mock_server)
            .await;

        // Passing base URL mock_server.uri() (without /v1/traces) must be normalized to /v1/traces
        let exporter = build_span_exporter(&mock_server.uri(), token).unwrap();
        let tokio_exporter = TokioSpanExporter::new(exporter);
        let provider = SdkTracerProvider::builder()
            .with_batch_exporter(tokio_exporter)
            .build();

        let tracer = provider.tracer("test-tracer");
        tracer.in_span("test-recap-span", |_cx| {
            // span generated
        });

        let flush_res = provider.force_flush();
        assert!(
            flush_res.is_ok(),
            "force_flush failed: {:?}",
            flush_res.err()
        );

        // Bounded clean shutdown
        let shutdown_res = provider.shutdown();
        assert!(
            shutdown_res.is_ok(),
            "shutdown failed: {:?}",
            shutdown_res.err()
        );

        // Verify request was received at /v1/traces with correct header and non-empty body
        let requests = mock_server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 1, "Must have exactly 1 original request");
        assert_eq!(requests[0].method, "POST");
        assert_eq!(requests[0].url.path(), "/v1/traces");
        assert_eq!(
            requests[0]
                .headers
                .get("authorization")
                .unwrap()
                .to_str()
                .unwrap(),
            format!("Bearer {token}")
        );
        assert!(
            !requests[0].body.is_empty(),
            "Protobuf body must be non-empty"
        );

        // Positive wire decode of the OTLP trace protobuf payload
        let decoded = ExportTraceServiceRequest::decode(&requests[0].body[..])
            .expect("must parse valid protobuf span payload");
        assert!(
            !decoded.resource_spans.is_empty(),
            "Resource spans must not be empty"
        );
        let scope_spans = &decoded.resource_spans[0].scope_spans;
        assert!(!scope_spans.is_empty(), "Scope spans must not be empty");
        let spans = &scope_spans[0].spans;
        assert!(
            !spans.is_empty(),
            "Inner scope_spans.spans must not be empty"
        );
        assert_eq!(
            spans[0].name, "test-recap-span",
            "Span name must match expected"
        );
    }

    #[tokio::test(flavor = "multi_thread")]
    async fn test_actual_exporter_does_not_follow_redirects() {
        use opentelemetry::trace::Tracer;
        use opentelemetry_proto::tonic::collector::trace::v1::ExportTraceServiceRequest;
        use prost::Message;

        let mock_server = MockServer::start().await;
        let token = "test-recap-telemetry-token-123";

        // Setup mock that attempts 307 redirect
        Mock::given(method("POST"))
            .and(path("/v1/traces"))
            .respond_with(ResponseTemplate::new(307).insert_header("Location", "/redirected"))
            .mount(&mock_server)
            .await;

        let exporter = build_span_exporter(&mock_server.uri(), token).unwrap();
        let tokio_exporter = TokioSpanExporter::new(exporter);
        let provider = SdkTracerProvider::builder()
            .with_batch_exporter(tokio_exporter)
            .build();

        let tracer = provider.tracer("test-tracer");
        tracer.in_span("test-recap-span", |_cx| {});

        // Because redirect policy is none, 307 is treated as error and not followed
        let _ = provider.force_flush();

        // Ensure shutdown is clean and bounded
        let _ = provider.shutdown();

        // Exactly 1 original request was sent to /v1/traces with Bearer token
        let requests = mock_server.received_requests().await.unwrap();
        assert_eq!(requests.len(), 1, "Must have exactly 1 original request");
        let req = &requests[0];
        assert_eq!(req.method, "POST");
        assert_eq!(req.url.path(), "/v1/traces");
        assert_eq!(
            req.headers.get("authorization").unwrap().to_str().unwrap(),
            format!("Bearer {token}")
        );
        assert!(!req.body.is_empty(), "Protobuf body must be non-empty");

        // Parse valid NONEMPTY protobuf span payload
        let decoded = ExportTraceServiceRequest::decode(&req.body[..])
            .expect("must parse valid protobuf span payload");
        assert!(
            !decoded.resource_spans.is_empty(),
            "Decoded spans must not be empty"
        );
        let scope_spans = &decoded.resource_spans[0].scope_spans;
        assert!(!scope_spans.is_empty(), "Scope spans must not be empty");
        let spans = &scope_spans[0].spans;
        assert!(
            !spans.is_empty(),
            "Inner scope_spans.spans must not be empty"
        );
        assert_eq!(
            spans[0].name, "test-recap-span",
            "Span name must match expected"
        );

        // Ensure zero destination hit (no request reached /redirected)
        for r in &requests {
            assert_ne!(
                r.url.path(),
                "/redirected",
                "Exporter must not follow redirects!"
            );
        }
    }
}
