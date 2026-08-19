use std::net::SocketAddr;
use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use prometheus::Registry;
use tokio::net::TcpListener;
use tokio::sync::Mutex;
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;
use tracing::info;

use super::PkiError;
use super::certfile::CertFile;
use super::config::{Config, load_config};
use super::filesafe::{MAX_ENV_FILE_BYTES, read_regular_no_follow};
use super::issuer::NativeStepCAIssuer;
use super::manager::{Issuer, Manager};
use super::metrics::{NopObserver, Observer, PromObserver, render_registry};

const STOP_JOIN_TIMEOUT: Duration = Duration::from_secs(15);
const METRICS_STOP_TIMEOUT: Duration = Duration::from_secs(2);

/// Running enrollment loop, stopped on process shutdown.
#[derive(Clone)]
pub struct Handle {
    cancel: CancellationToken,
    metrics_cancel: CancellationToken,
    task: Arc<Mutex<Option<JoinHandle<()>>>>,
    metrics_task: Arc<Mutex<Option<JoinHandle<()>>>>,
}

impl std::fmt::Debug for Handle {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Handle").finish_non_exhaustive()
    }
}

impl Handle {
    /// Cancels the renewal loop, joins in-flight work, and only aborts if the
    /// join exceeds the bounded timeout. Idempotent.
    pub async fn stop(&self) {
        self.cancel.cancel();
        self.metrics_cancel.cancel();
        join_or_abort(&self.task, STOP_JOIN_TIMEOUT).await;
        join_or_abort(&self.metrics_task, METRICS_STOP_TIMEOUT).await;
    }

    async fn serve_metrics(&self, registry: Registry, bind: SocketAddr) -> Result<(), PkiError> {
        let listener = TcpListener::bind(bind)
            .await
            .map_err(|e| PkiError::other(format!("pki metrics bind {bind}: {e}")))?;
        let registry = Arc::new(registry);
        let cancel = self.metrics_cancel.clone();
        let task = tokio::spawn(async move {
            let app = axum::Router::new().route(
                "/metrics",
                axum::routing::get({
                    let registry = Arc::clone(&registry);
                    move || {
                        let registry = Arc::clone(&registry);
                        async move { render_registry(&registry) }
                    }
                }),
            );
            let _ = axum::serve(listener, app)
                .with_graceful_shutdown(async move { cancel.cancelled().await })
                .await;
        });
        *self.metrics_task.lock().await = Some(task);
        Ok(())
    }
}

async fn join_or_abort(slot: &Mutex<Option<JoinHandle<()>>>, timeout: Duration) {
    let Some(task) = slot.lock().await.take() else {
        return;
    };
    let abort = task.abort_handle();
    if tokio::time::timeout(timeout, task).await.is_err() {
        abort.abort();
    }
}

fn metrics_bind() -> Result<Option<SocketAddr>, PkiError> {
    if std::env::var_os("PKI_METRICS_BIND_FILE").is_some() {
        let file_ref = std::env::var("PKI_METRICS_BIND_FILE").unwrap_or_default();
        if file_ref.trim().is_empty() {
            return Err(PkiError::other("PKI_METRICS_BIND_FILE is empty"));
        }
        let raw = read_regular_no_follow(Path::new(&file_ref), MAX_ENV_FILE_BYTES)
            .map_err(|e| PkiError::other(format!("read PKI_METRICS_BIND_FILE: {e}")))?;
        let s = String::from_utf8_lossy(&raw).trim().to_string();
        if s.is_empty() {
            return Err(PkiError::other("PKI_METRICS_BIND_FILE is empty"));
        }
        return parse_metrics_bind(&s);
    }
    match std::env::var("PKI_METRICS_BIND") {
        Err(std::env::VarError::NotPresent) => parse_metrics_bind("127.0.0.1:9110"),
        Err(err) => Err(PkiError::other(format!("PKI_METRICS_BIND: {err}"))),
        Ok(v) => parse_metrics_bind(&v),
    }
}

fn parse_metrics_bind(raw: &str) -> Result<Option<SocketAddr>, PkiError> {
    let v = raw.trim();
    if v.is_empty() {
        return Err(PkiError::other("PKI_METRICS_BIND is empty"));
    }
    if v.eq_ignore_ascii_case("disabled") {
        return Ok(None);
    }
    let normalized = if let Some(port) = v.strip_prefix(':') {
        format!("0.0.0.0:{port}")
    } else {
        v.to_string()
    };
    normalized
        .parse()
        .map(Some)
        .map_err(|e| PkiError::other(format!("PKI_METRICS_BIND: {e}")))
}

/// Load config for `service_name`, log enabled/disabled, and either return
/// `None` (disabled) or fail-fast enroll then run the loop.
///
/// PKI metrics use a private Prometheus registry and a dedicated ops listener
/// (default `127.0.0.1:9110`). They are never registered on the app `/metrics`.
pub async fn start(service_name: &str) -> Result<Option<Handle>, PkiError> {
    let cfg = load_config(service_name)?;
    if !cfg.is_enabled() {
        return start_with(cfg, None, None).await;
    }
    let bind = metrics_bind()?;
    let registry = Registry::new();
    let observer =
        PromObserver::new(&cfg.subject, &registry).map_err(|e| PkiError::other(e.to_string()))?;
    let handle = start_with(cfg, None, Some(Arc::new(observer))).await?;
    let Some(handle) = handle else {
        return Ok(None);
    };
    if let Some(addr) = bind
        && let Err(err) = handle.serve_metrics(registry, addr).await
    {
        handle.stop().await;
        return Err(err);
    }
    Ok(Some(handle))
}

/// Test seam: a `Some` issuer skips the native CA client.
pub async fn start_with(
    cfg: Config,
    issuer: Option<Arc<dyn Issuer>>,
    observer: Option<Arc<dyn Observer>>,
) -> Result<Option<Handle>, PkiError> {
    if !cfg.is_enabled() {
        info!(
            service = %cfg.subject,
            mode = %cfg.mode,
            reason = "sidecar still owns cert files until compose cutover for remaining subjects",
            "pki_enrollment_disabled"
        );
        return Ok(None);
    }
    info!(
        service = %cfg.subject,
        provisioner = %cfg.provisioner,
        password_file = %cfg.password_file,
        cert_path = %cfg.cert_path,
        "pki_enrollment_enabled"
    );
    let issuer: Arc<dyn Issuer> = match issuer {
        Some(i) => i,
        None => Arc::new(NativeStepCAIssuer::from_config(&cfg)),
    };
    let files = CertFile::new(&cfg.cert_path, &cfg.key_path);
    let observer = observer.unwrap_or_else(|| Arc::new(NopObserver));
    let mut mgr = Manager::new(cfg, issuer, files);
    mgr.observer = observer;
    let mgr = Arc::new(mgr);
    let cancel = CancellationToken::new();
    mgr.enroll(&cancel).await?;
    let run_cancel = cancel.clone();
    let run_mgr = Arc::clone(&mgr);
    let task = tokio::spawn(async move {
        let _ = run_mgr.run(&run_cancel).await;
    });
    Ok(Some(Handle {
        cancel,
        metrics_cancel: CancellationToken::new(),
        task: Arc::new(Mutex::new(Some(task))),
        metrics_task: Arc::new(Mutex::new(None)),
    }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pki::config::{MODE_DISABLED, MODE_ENABLED};
    use crate::pki::manager::Issuer;
    use crate::pki::test_util::self_signed_pem;
    use async_trait::async_trait;
    use std::time::Duration;
    use tracing::subscriber::set_default;
    use tracing_subscriber::fmt::MakeWriter;

    struct Buf(std::sync::Arc<std::sync::Mutex<Vec<u8>>>);
    impl std::io::Write for Buf {
        fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
            self.0.lock().unwrap().extend_from_slice(buf);
            Ok(buf.len())
        }
        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }
    impl<'a> MakeWriter<'a> for Buf {
        type Writer = Buf;
        fn make_writer(&'a self) -> Self::Writer {
            Buf(std::sync::Arc::clone(&self.0))
        }
    }

    struct StubIssuer;

    #[async_trait]
    impl Issuer for StubIssuer {
        async fn issue(
            &self,
            _cancel: &CancellationToken,
            subject: &str,
            _sans: &[String],
        ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
            let nb = std::time::SystemTime::now();
            Ok(self_signed_pem(
                subject,
                nb,
                nb + Duration::from_secs(24 * 3600),
            ))
        }
    }

    #[tokio::test]
    async fn start_disabled_logs() {
        let buf = std::sync::Arc::new(std::sync::Mutex::new(Vec::new()));
        let subscriber = tracing_subscriber::fmt()
            .with_writer(Buf(std::sync::Arc::clone(&buf)))
            .with_ansi(false)
            .finish();
        let h = temp_env::async_with_vars([("PKI_ENROLLMENT", Some(MODE_DISABLED))], async {
            let _guard = set_default(subscriber);
            start("recap-worker").await
        })
        .await
        .expect("start");
        assert!(h.is_none());
        let logged = String::from_utf8(buf.lock().unwrap().clone()).unwrap();
        assert!(logged.contains("pki_enrollment_disabled"), "log={logged}");
    }

    #[tokio::test]
    async fn start_enabled_does_not_require_step_binary() {
        let dir = tempfile::TempDir::new().unwrap();
        let err = temp_env::async_with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                (
                    "STEP_BINARY",
                    Some(dir.path().join("no-such-step").to_str().unwrap()),
                ),
                ("STEP_CA_URL", Some("https://127.0.0.1:1")),
                (
                    "STEP_CA_ROOT_FILE",
                    Some(dir.path().join("missing-root.pem").to_str().unwrap()),
                ),
                (
                    "STEP_CA_PROVISIONER_PASSWORD_FILE",
                    Some(dir.path().join("missing-jwk").to_str().unwrap()),
                ),
            ],
            async { start("recap-worker").await },
        )
        .await
        .expect_err("native issuer must fail without CA materials");
        let msg = err.to_string();
        assert!(
            !msg.contains("step CLI"),
            "enabled startup must not depend on step executable: {msg}"
        );
    }

    #[tokio::test]
    async fn start_with_enabled_enrolls_and_stops() {
        let dir = tempfile::TempDir::new().unwrap();
        let cert_path = dir.path().join("svc-cert.pem");
        let key_path = dir.path().join("svc-key.pem");
        let cfg = Config {
            mode: MODE_ENABLED.into(),
            subject: "recap-worker".into(),
            sans: vec!["recap-worker".into()],
            cert_path: cert_path.to_string_lossy().into(),
            key_path: key_path.to_string_lossy().into(),
            ca_url: "https://step-ca:9000".into(),
            root_file: "/trust/ca-bundle.pem".into(),
            provisioner: "pki-agent-recap-worker".into(),
            password_file: "/run/secrets/pki-agent-recap-worker-jwk".into(),
            renew_at_fraction: 0.66,
            tick_interval: Duration::from_secs(3600),
            retry_backoff: Duration::from_millis(1),
            retry_attempts: 1,
            issue_timeout: Duration::from_secs(10),
        };
        let buf = std::sync::Arc::new(std::sync::Mutex::new(Vec::new()));
        let subscriber = tracing_subscriber::fmt()
            .with_writer(Buf(std::sync::Arc::clone(&buf)))
            .with_ansi(false)
            .finish();
        let _guard = set_default(subscriber);
        let h = start_with(cfg, Some(Arc::new(StubIssuer)), None)
            .await
            .expect("start")
            .expect("handle");
        assert!(cert_path.exists());
        let logged = String::from_utf8(buf.lock().unwrap().clone()).unwrap();
        assert!(logged.contains("pki_enrollment_enabled"), "log={logged}");
        h.stop().await;
    }

    #[tokio::test]
    async fn start_enabled_shared_secret_rejected() {
        let err = temp_env::async_with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                (
                    "STEP_CA_PROVISIONER_PASSWORD_FILE",
                    Some("/run/secrets/step_ca_root_password"),
                ),
            ],
            async { start("recap-worker").await },
        )
        .await
        .expect_err("shared root secret");
        assert!(matches!(err, crate::pki::PkiError::SharedRootSecret { .. }));
    }

    #[tokio::test]
    async fn start_with_observer_wires_prom_when_enabled() {
        let dir = tempfile::TempDir::new().unwrap();
        let cfg = Config {
            mode: MODE_ENABLED.into(),
            subject: "recap-worker-metrics".into(),
            sans: vec!["recap-worker-metrics".into()],
            cert_path: dir.path().join("svc-cert.pem").to_string_lossy().into(),
            key_path: dir.path().join("svc-key.pem").to_string_lossy().into(),
            ca_url: "https://step-ca:9000".into(),
            root_file: "/trust/ca-bundle.pem".into(),
            provisioner: "pki-agent-recap-worker-metrics".into(),
            password_file: "/run/secrets/pki-agent-recap-worker-metrics-jwk".into(),
            renew_at_fraction: 0.66,
            tick_interval: Duration::from_secs(3600),
            retry_backoff: Duration::from_millis(1),
            retry_attempts: 1,
            issue_timeout: Duration::from_secs(10),
        };
        let reg = Registry::new();
        let obs = PromObserver::new(&cfg.subject, &reg).unwrap();
        let h = start_with(cfg, Some(Arc::new(StubIssuer)), Some(Arc::new(obs)))
            .await
            .expect("start")
            .expect("handle");
        let body = crate::pki::metrics::render_registry(&reg);
        assert!(
            body.contains("pki_enrollment_healthy{subject=\"recap-worker-metrics\"} 1"),
            "{body}"
        );
        h.stop().await;
    }

    #[test]
    fn parse_metrics_bind_compose_value_is_portable() {
        let yaml = include_str!(concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/../../compose/recap.yaml"
        ));
        let mut bind = None;
        for line in yaml.lines() {
            let line = line.trim().trim_start_matches('-').trim();
            if let Some(rest) = line.strip_prefix("PKI_METRICS_BIND=") {
                bind = Some(rest.trim().to_string());
                break;
            }
        }
        let bind = bind.expect("compose/recap.yaml must set PKI_METRICS_BIND");
        assert_eq!(
            bind, "0.0.0.0:9110",
            "compose PKI_METRICS_BIND must be the portable SocketAddr 0.0.0.0:9110, got {bind}"
        );
        let addr = parse_metrics_bind(&bind).expect("compose bind must parse");
        assert_eq!(addr, Some("0.0.0.0:9110".parse().unwrap()));
    }

    #[test]
    fn parse_metrics_bind_bare_port_maps_to_all_interfaces() {
        let addr = parse_metrics_bind(":9110").expect("bare :9110 must parse");
        assert_eq!(addr, Some("0.0.0.0:9110".parse().unwrap()));
    }
}
