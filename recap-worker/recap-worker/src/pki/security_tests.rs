//! Ports of `alt-backend/app/internal/pki/security_test.go` that do not
//! need the fake step-ca HTTPS server (those live in `issuer_http_tests.rs`).

use super::certfile::CertFile;
use super::config::{MODE_ENABLED, load_config};
use super::error::PkiError;
use super::manager::Issuer;
use super::metrics::{PromObserver, render_registry};
use super::start::{start, start_with};
use super::test_util::self_signed_pem;
use async_trait::async_trait;
use prometheus::Registry;
use std::path::Path;
use std::sync::Arc;
use std::sync::atomic::{AtomicU32, Ordering};
use std::time::{Duration, SystemTime};
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;

#[test]
fn load_config_unset_enrollment_defaults_disabled() {
    temp_env::with_vars(
        [
            ("PKI_ENROLLMENT", None::<&str>),
            ("PKI_ENROLLMENT_FILE", None::<&str>),
        ],
        || {
            let c = load_config("recap-worker").expect("load");
            assert_eq!(c.mode, super::config::MODE_DISABLED);
        },
    );
}

#[test]
fn load_config_empty_enrollment_fails() {
    temp_env::with_var("PKI_ENROLLMENT", Some(""), || {
        assert!(load_config("recap-worker").is_err());
    });
}

#[test]
fn load_config_file_missing_fails() {
    let dir = tempfile::TempDir::new().unwrap();
    let missing = dir.path().join("missing");
    temp_env::with_var(
        "PKI_ENROLLMENT_FILE",
        Some(missing.to_str().unwrap()),
        || {
            assert!(load_config("recap-worker").is_err());
        },
    );
}

#[test]
fn load_config_file_empty_fails() {
    let dir = tempfile::TempDir::new().unwrap();
    let path = dir.path().join("empty");
    std::fs::write(&path, " \n").unwrap();
    temp_env::with_var("PKI_ENROLLMENT_FILE", Some(path.to_str().unwrap()), || {
        assert!(load_config("recap-worker").is_err());
    });
}

#[test]
fn load_config_file_empty_path_fails() {
    temp_env::with_var("PKI_ENROLLMENT_FILE", Some(""), || {
        assert!(load_config("recap-worker").is_err());
    });
}

#[test]
fn load_config_rejects_http_and_schemeless_ca_url() {
    for raw in ["http://step-ca:9000", "step-ca:9000", "https://"] {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                ("STEP_CA_URL", Some(raw)),
            ],
            || {
                let err = load_config("recap-worker").expect_err(raw);
                assert!(
                    matches!(err, PkiError::InsecureCaUrl { .. }),
                    "{raw}: {err:?}"
                );
            },
        );
    }
}

#[test]
fn load_config_exact_provisioner_and_password_basename() {
    temp_env::with_vars(
        [
            ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
            ("CERT_SUBJECT", Some("rag")),
            ("STEP_CA_PROVISIONER", Some("pki-agent-rag-orchestrator")),
            (
                "STEP_CA_PROVISIONER_PASSWORD_FILE",
                Some("/run/secrets/pki-agent-rag-orchestrator-jwk"),
            ),
        ],
        || {
            let err = load_config("rag").expect_err("substring");
            assert!(
                !matches!(err, PkiError::SharedProvisioner { .. }),
                "wrong sentinel: {err:?}"
            );
        },
    );
}

#[test]
fn load_config_rejects_wrong_password_basename() {
    let dir = tempfile::TempDir::new().unwrap();
    let wrong = dir.path().join("wrong-name");
    temp_env::with_vars(
        [
            ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
            ("CERT_SUBJECT", Some("recap-worker")),
            (
                "STEP_CA_PROVISIONER_PASSWORD_FILE",
                Some(wrong.to_str().unwrap()),
            ),
        ],
        || {
            assert!(load_config("recap-worker").is_err());
        },
    );
}

#[test]
fn load_config_temp_dir_allowed_when_basename_matches() {
    let dir = tempfile::TempDir::new().unwrap();
    let path = dir.path().join("pki-agent-recap-worker-jwk");
    temp_env::with_vars(
        [
            ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
            ("CERT_SUBJECT", Some("recap-worker")),
            (
                "STEP_CA_PROVISIONER_PASSWORD_FILE",
                Some(path.to_str().unwrap()),
            ),
        ],
        || {
            let c = load_config("recap-worker").expect("load");
            assert_eq!(c.password_file, path.to_string_lossy());
        },
    );
}

#[test]
fn certfile_rejects_symlink_dest_and_parent() {
    let now = SystemTime::now();
    let (cert, key) = self_signed_pem("recap-worker", now, now + Duration::from_secs(3600));

    let dir = tempfile::TempDir::new().unwrap();
    let target = dir.path().join("target.pem");
    std::fs::write(&target, b"nope").unwrap();
    let dest = dir.path().join("svc-cert.pem");
    std::os::unix::fs::symlink(&target, &dest).unwrap();
    let cf = CertFile::new(dest, dir.path().join("svc-key.pem"));
    assert!(cf.write(&cert, &key).is_err());

    let root = tempfile::TempDir::new().unwrap();
    let real_dir = root.path().join("real");
    std::fs::create_dir(&real_dir).unwrap();
    let link_dir = root.path().join("link");
    std::os::unix::fs::symlink(&real_dir, &link_dir).unwrap();
    let cf = CertFile::new(link_dir.join("svc-cert.pem"), link_dir.join("svc-key.pem"));
    assert!(cf.write(&cert, &key).is_err());
}

struct StickyIssuer {
    inner_nb: SystemTime,
    gate: Mutex<Option<tokio::sync::oneshot::Receiver<()>>>,
    entered: AtomicU32,
    writes: AtomicU32,
}

#[async_trait]
impl Issuer for StickyIssuer {
    async fn issue(
        &self,
        _cancel: &CancellationToken,
        subject: &str,
        _sans: &[String],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
        self.entered.fetch_add(1, Ordering::SeqCst);
        if let Some(rx) = self.gate.lock().await.take() {
            let _ = rx.await;
        }
        self.writes.fetch_add(1, Ordering::SeqCst);
        let nb = self.inner_nb;
        Ok(self_signed_pem(
            subject,
            nb,
            nb + Duration::from_secs(24 * 3600),
        ))
    }
}

fn enabled_cfg(dir: &Path, subject: &str) -> super::config::Config {
    super::config::Config {
        mode: MODE_ENABLED.into(),
        subject: subject.into(),
        sans: vec![subject.into()],
        cert_path: dir.join("svc-cert.pem").to_string_lossy().into(),
        key_path: dir.join("svc-key.pem").to_string_lossy().into(),
        ca_url: "https://step-ca:9000".into(),
        root_file: "/trust/ca-bundle.pem".into(),
        provisioner: format!("pki-agent-{subject}"),
        password_file: format!("/run/secrets/pki-agent-{subject}-jwk"),
        renew_at_fraction: 0.66,
        tick_interval: Duration::from_millis(15),
        retry_backoff: Duration::from_millis(1),
        retry_attempts: 1,
        issue_timeout: Duration::from_secs(10),
    }
}

#[tokio::test]
async fn handle_stop_waits_for_in_flight_run_and_does_not_write_after() {
    let dir = tempfile::TempDir::new().unwrap();
    let cfg = enabled_cfg(dir.path(), "recap-worker");
    let sticky = Arc::new(StickyIssuer {
        inner_nb: SystemTime::now() - Duration::from_secs(60),
        gate: Mutex::new(None),
        entered: AtomicU32::new(0),
        writes: AtomicU32::new(0),
    });
    let h = start_with(
        cfg.clone(),
        Some(Arc::clone(&sticky) as Arc<dyn Issuer>),
        None,
    )
    .await
    .expect("start")
    .expect("handle");
    let (tx, rx) = tokio::sync::oneshot::channel();
    *sticky.gate.lock().await = Some(rx);
    std::fs::remove_file(&cfg.cert_path).unwrap();
    let deadline = tokio::time::Instant::now() + Duration::from_secs(1);
    while sticky.entered.load(Ordering::SeqCst) < 2 && tokio::time::Instant::now() < deadline {
        tokio::time::sleep(Duration::from_millis(5)).await;
    }
    assert!(
        sticky.entered.load(Ordering::SeqCst) >= 2,
        "Run did not enter in-flight Issue"
    );
    let writes_before_release = sticky.writes.load(Ordering::SeqCst);
    let released = tokio::spawn(async move {
        tokio::time::sleep(Duration::from_millis(80)).await;
        let _ = tx.send(());
    });
    let start = tokio::time::Instant::now();
    h.stop().await;
    assert!(
        start.elapsed() >= Duration::from_millis(50),
        "Stop returned before in-flight Issue finished"
    );
    released.await.unwrap();
    let mtime = std::fs::metadata(&cfg.cert_path)
        .ok()
        .and_then(|m| m.modified().ok());
    let writes_at_stop = sticky.writes.load(Ordering::SeqCst);
    assert!(
        writes_at_stop >= writes_before_release,
        "in-flight Issue should complete before Stop returns"
    );
    tokio::time::sleep(Duration::from_millis(80)).await;
    assert_eq!(
        sticky.writes.load(Ordering::SeqCst),
        writes_at_stop,
        "no Issue after Stop"
    );
    let mtime_after = std::fs::metadata(&cfg.cert_path)
        .ok()
        .and_then(|m| m.modified().ok());
    assert_eq!(mtime, mtime_after, "no cert write after Stop");
}

#[tokio::test]
async fn start_does_not_register_on_default_or_app_registry() {
    let dir = tempfile::TempDir::new().unwrap();
    let cfg = enabled_cfg(dir.path(), "recap-worker-private-metrics");
    let app_reg = Registry::new();
    let pki_reg = Registry::new();
    let obs = PromObserver::new(&cfg.subject, &pki_reg).unwrap();
    let h = start_with(cfg.clone(), Some(Arc::new(StubNow)), Some(Arc::new(obs)))
        .await
        .expect("start")
        .expect("handle");
    let default_body = {
        use prometheus::{Encoder, TextEncoder};
        let mut buf = Vec::new();
        let _ = TextEncoder::new().encode(&prometheus::default_registry().gather(), &mut buf);
        String::from_utf8(buf).unwrap_or_default()
    };
    assert!(
        !default_body.contains("recap-worker-private-metrics"),
        "PKI collector leaked onto DefaultRegisterer"
    );
    let app_body = render_registry(&app_reg);
    assert!(
        !app_body.contains("pki_enrollment"),
        "PKI series leaked onto app registry"
    );
    let priv_body = render_registry(&pki_reg);
    assert!(
        priv_body.contains("pki_enrollment_healthy{subject=\"recap-worker-private-metrics\"}"),
        "{priv_body}"
    );
    h.stop().await;
}

struct StubNow;

#[async_trait]
impl Issuer for StubNow {
    async fn issue(
        &self,
        _cancel: &CancellationToken,
        subject: &str,
        _sans: &[String],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
        let nb = SystemTime::now();
        Ok(self_signed_pem(
            subject,
            nb,
            nb + Duration::from_secs(24 * 3600),
        ))
    }
}

#[tokio::test]
async fn start_fail_fast_metrics_bind_file() {
    temp_env::async_with_vars(
        [
            ("PKI_ENROLLMENT", Some(super::config::MODE_DISABLED)),
            ("PKI_METRICS_BIND_FILE", Some("")),
        ],
        async {
            // disabled mode must not consult metrics bind; empty FILE is still
            // fail-fast only when enrollment is enabled. Covered below.
        },
    )
    .await;

    let dir = tempfile::TempDir::new().unwrap();
    let err = temp_env::async_with_vars(
        [
            ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
            ("CERT_SUBJECT", Some("recap-worker")),
            ("PKI_METRICS_BIND_FILE", Some("")),
            (
                "STEP_CA_PROVISIONER_PASSWORD_FILE",
                Some(
                    dir.path()
                        .join("pki-agent-recap-worker-jwk")
                        .to_str()
                        .unwrap(),
                ),
            ),
        ],
        async { start("recap-worker").await },
    )
    .await
    .expect_err("empty FILE");
    assert!(err.to_string().contains("PKI_METRICS_BIND_FILE"));
}
