use std::fs;
use std::sync::Arc;
use std::time::{Duration, SystemTime};

use async_trait::async_trait;
use tokio_util::sync::CancellationToken;
use tracing::{error, info};

use super::certfile::CertFile;
use super::config::Config;
use super::error::PkiError;
use super::metrics::{NopObserver, Observer};
use super::state::{State, classify_remaining};

/// Mints a freshly-signed leaf. Implementations MUST generate a new private
/// key on every call.
#[async_trait]
pub trait Issuer: Send + Sync {
    async fn issue(
        &self,
        cancel: &CancellationToken,
        subject: &str,
        sans: &[String],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError>;

    fn supports_rekey(&self) -> bool {
        false
    }

    async fn rekey(
        &self,
        _cancel: &CancellationToken,
        _cert_pem: &[u8],
        _key_pem: &[u8],
        _subject: &str,
        _sans: &[String],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
        Err(PkiError::other("rekey not supported"))
    }
}

/// Cert lifecycle state machine. Tests inject a fake Issuer, a CertFile on a
/// temp dir, and a fake clock.
pub struct Manager {
    pub cfg: Config,
    pub issuer: Arc<dyn Issuer>,
    pub files: CertFile,
    pub observer: Arc<dyn Observer>,
    pub now: Box<dyn Fn() -> SystemTime + Send + Sync>,
}

impl Manager {
    pub fn new(cfg: Config, issuer: Arc<dyn Issuer>, files: CertFile) -> Self {
        Self {
            cfg,
            issuer,
            files,
            observer: Arc::new(NopObserver),
            now: Box::new(SystemTime::now),
        }
    }

    fn now(&self) -> SystemTime {
        (self.now)()
    }

    fn remaining_secs(&self, not_after: SystemTime) -> f64 {
        match not_after.duration_since(self.now()) {
            Ok(d) => d.as_secs_f64(),
            Err(e) => -(e.duration().as_secs_f64()),
        }
    }

    /// Runs Tick until the leaf is fresh or the retry budget / context is
    /// exhausted. Failures are never treated as success.
    pub async fn enroll(&self, cancel: &CancellationToken) -> Result<(), PkiError> {
        let mut attempts = self.cfg.retry_attempts;
        if attempts < 1 {
            attempts = 1;
        }
        let mut backoff = self.cfg.retry_backoff;
        if backoff.is_zero() {
            backoff = Duration::from_secs(1);
        }
        let mut last: Option<PkiError> = None;
        for i in 1..=attempts {
            match self.tick(cancel).await {
                Ok(State::Fresh | State::NearExpiry) => return Ok(()),
                Ok(state) => {
                    last = Some(PkiError::other(format!("enroll left state {state}")));
                }
                Err(err) if err.is_canceled() => return Err(err),
                Err(err) => last = Some(err),
            }
            let err = last.as_ref().map(ToString::to_string).unwrap_or_default();
            self.observer.on_retry(i, &err);
            error!(
                subject = %self.cfg.subject,
                attempt = i,
                attempts,
                error = %err,
                "pki_enrollment_retry"
            );
            if i == attempts {
                break;
            }
            tokio::select! {
                () = cancel.cancelled() => return Err(PkiError::Canceled),
                () = tokio::time::sleep(backoff) => {}
            }
        }
        Err(PkiError::other(format!(
            "enroll failed after {attempts} attempts: {}",
            last.map(|e| e.to_string()).unwrap_or_default()
        )))
    }

    pub async fn tick(&self, cancel: &CancellationToken) -> Result<State, PkiError> {
        if cancel.is_cancelled() {
            return Err(PkiError::Canceled);
        }
        let cert = match self.files.load() {
            Ok(cert) => cert,
            Err(PkiError::CertNotFound) => return self.issue(cancel, "missing").await,
            Err(PkiError::CertParseFailed(_) | PkiError::CertKeyMismatch) => {
                self.observer.on_classified(State::Corrupt, 0.0);
                return self.issue(cancel, "corrupt").await;
            }
            Err(err) => return Err(PkiError::other(format!("load cert: {err}"))),
        };
        let now = self.now();
        let state = classify_remaining(
            cert.not_before,
            cert.not_after,
            now,
            self.cfg.renew_at_fraction,
        );
        self.observer
            .on_classified(state, self.remaining_secs(cert.not_after));
        match state {
            State::Fresh => Ok(state),
            State::NearExpiry if self.issuer.supports_rekey() => self.rekey(cancel).await,
            State::NearExpiry => self.issue(cancel, "near_expiry").await,
            State::Expired => self.issue(cancel, "expired").await,
            other => Ok(other),
        }
    }

    async fn issue(&self, cancel: &CancellationToken, reason: &str) -> Result<State, PkiError> {
        self.observer.on_reissued(reason);
        info!(
            subject = %self.cfg.subject,
            reason,
            provisioner = %self.cfg.provisioner,
            "pki_enrollment_reissue"
        );
        let (cert_pem, key_pem) = match self
            .issuer
            .issue(cancel, &self.cfg.subject, &self.cfg.sans)
            .await
        {
            Ok(pair) => pair,
            Err(err) => {
                self.observer.on_renewed(false);
                error!(subject = %self.cfg.subject, reason, error = %err, "pki_enrollment_failed");
                return Err(PkiError::other(format!("issue cert: {err}")));
            }
        };
        if let Err(err) = self.files.write(&cert_pem, &key_pem) {
            self.observer.on_renewed(false);
            error!(subject = %self.cfg.subject, reason, error = %err, "pki_enrollment_failed");
            return Err(PkiError::other(format!("write cert: {err}")));
        }
        self.observer.on_renewed(true);
        self.load_after_write()
    }

    async fn rekey(&self, cancel: &CancellationToken) -> Result<State, PkiError> {
        self.observer.on_reissued("near_expiry");
        info!(
            subject = %self.cfg.subject,
            reason = "near_expiry",
            provisioner = %self.cfg.provisioner,
            "pki_enrollment_rekey"
        );
        let cert_pem = fs::read(&self.files.cert_path).map_err(|e| {
            self.observer.on_renewed(false);
            PkiError::other(format!("read cert for rekey: {e}"))
        })?;
        let key_pem = fs::read(&self.files.key_path).map_err(|e| {
            self.observer.on_renewed(false);
            PkiError::other(format!("read key for rekey: {e}"))
        })?;
        let (new_cert, new_key) = match self
            .issuer
            .rekey(
                cancel,
                &cert_pem,
                &key_pem,
                &self.cfg.subject,
                &self.cfg.sans,
            )
            .await
        {
            Ok(pair) => pair,
            Err(err) => {
                self.observer.on_renewed(false);
                error!(
                    subject = %self.cfg.subject,
                    reason = "near_expiry",
                    error = %err,
                    "pki_enrollment_failed"
                );
                return Err(PkiError::other(format!("rekey cert: {err}")));
            }
        };
        if let Err(err) = self.files.write(&new_cert, &new_key) {
            self.observer.on_renewed(false);
            error!(
                subject = %self.cfg.subject,
                reason = "near_expiry",
                error = %err,
                "pki_enrollment_failed"
            );
            return Err(PkiError::other(format!("write rekeyed cert: {err}")));
        }
        self.observer.on_renewed(true);
        self.load_after_write()
    }

    fn load_after_write(&self) -> Result<State, PkiError> {
        match self.files.load() {
            Ok(cert) => {
                let state = classify_remaining(
                    cert.not_before,
                    cert.not_after,
                    self.now(),
                    self.cfg.renew_at_fraction,
                );
                self.observer
                    .on_classified(state, self.remaining_secs(cert.not_after));
                Ok(state)
            }
            Err(err) => {
                self.observer.on_classified(State::Corrupt, 0.0);
                Err(PkiError::other(format!("post-write load: {err}")))
            }
        }
    }

    pub async fn run(&self, cancel: &CancellationToken) -> Result<(), PkiError> {
        let mut interval = self.cfg.tick_interval;
        if interval.is_zero() {
            interval = Duration::from_secs(5 * 60);
        }
        let mut ticker = tokio::time::interval(interval);
        ticker.tick().await; // skip immediate tick; Enroll already ran
        loop {
            tokio::select! {
                () = cancel.cancelled() => {
                    info!(subject = %self.cfg.subject, "pki_enrollment_stopped");
                    return Err(PkiError::Canceled);
                }
                _ = ticker.tick() => {
                    if let Err(err) = self.tick(cancel).await {
                        error!(subject = %self.cfg.subject, error = %err, "pki_enrollment_tick_failed");
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pki::config::MODE_DISABLED;
    use crate::pki::test_util::self_signed_pem;
    use std::os::unix::fs::PermissionsExt;
    use std::sync::Mutex;
    use std::sync::atomic::{AtomicU32, Ordering};
    use std::time::UNIX_EPOCH;

    struct RecObs {
        classified: Mutex<Vec<State>>,
        reissued: Mutex<Vec<String>>,
        renewed: Mutex<Vec<bool>>,
        retries: AtomicU32,
    }

    impl RecObs {
        fn new() -> Arc<Self> {
            Arc::new(Self {
                classified: Mutex::new(Vec::new()),
                reissued: Mutex::new(Vec::new()),
                renewed: Mutex::new(Vec::new()),
                retries: AtomicU32::new(0),
            })
        }
    }

    impl Observer for RecObs {
        fn on_classified(&self, state: State, _remaining_secs: f64) {
            self.classified.lock().unwrap().push(state);
        }
        fn on_reissued(&self, reason: &str) {
            self.reissued.lock().unwrap().push(reason.to_string());
        }
        fn on_renewed(&self, success: bool) {
            self.renewed.lock().unwrap().push(success);
        }
        fn on_retry(&self, _attempt: u32, _err: &str) {
            self.retries.fetch_add(1, Ordering::SeqCst);
        }
    }

    struct FakeIssuer {
        calls: AtomicU32,
        err: Mutex<Option<String>>,
        not_before: Mutex<SystemTime>,
        lifetime: Duration,
        block: tokio::sync::Mutex<Option<CancellationToken>>,
        rekey_calls: AtomicU32,
        rekey: bool,
    }

    impl FakeIssuer {
        fn new(not_before: SystemTime, lifetime: Duration) -> Arc<Self> {
            Arc::new(Self {
                calls: AtomicU32::new(0),
                err: Mutex::new(None),
                not_before: Mutex::new(not_before),
                lifetime,
                block: tokio::sync::Mutex::new(None),
                rekey_calls: AtomicU32::new(0),
                rekey: false,
            })
        }

        fn failing(msg: &str) -> Arc<Self> {
            let s = Self::new(SystemTime::now(), Duration::from_secs(3600));
            *s.err.lock().unwrap() = Some(msg.into());
            s
        }

        fn call_count(&self) -> u32 {
            self.calls.load(Ordering::SeqCst)
        }
    }

    #[async_trait]
    impl Issuer for FakeIssuer {
        async fn issue(
            &self,
            cancel: &CancellationToken,
            subject: &str,
            _sans: &[String],
        ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
            self.calls.fetch_add(1, Ordering::SeqCst);
            {
                let guard = self.block.lock().await;
                if let Some(block) = guard.as_ref() {
                    tokio::select! {
                        () = cancel.cancelled() => return Err(PkiError::Canceled),
                        () = block.cancelled() => {}
                    }
                }
            }
            if let Some(err) = self.err.lock().unwrap().clone() {
                return Err(PkiError::other(err));
            }
            let nb = *self.not_before.lock().unwrap();
            Ok(self_signed_pem(subject, nb, nb + self.lifetime))
        }

        fn supports_rekey(&self) -> bool {
            self.rekey
        }

        async fn rekey(
            &self,
            cancel: &CancellationToken,
            _cert_pem: &[u8],
            _key_pem: &[u8],
            subject: &str,
            sans: &[String],
        ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
            self.rekey_calls.fetch_add(1, Ordering::SeqCst);
            self.issue(cancel, subject, sans).await
        }
    }

    fn ts(secs: u64) -> SystemTime {
        UNIX_EPOCH + Duration::from_secs(secs)
    }

    fn test_cfg() -> Config {
        Config {
            mode: MODE_DISABLED.into(),
            subject: "recap-worker".into(),
            sans: vec!["recap-worker".into()],
            cert_path: String::new(),
            key_path: String::new(),
            ca_url: "https://step-ca:9000".into(),
            root_file: String::new(),
            provisioner: "pki-agent-recap-worker".into(),
            password_file: "/run/secrets/pki-agent-recap-worker-jwk".into(),
            renew_at_fraction: 0.66,
            tick_interval: Duration::from_secs(300),
            retry_backoff: Duration::from_millis(1),
            retry_attempts: 3,
            issue_timeout: Duration::from_secs(10),
        }
    }

    fn new_manager(
        issuer: Arc<dyn Issuer>,
        now: SystemTime,
    ) -> (Manager, tempfile::TempDir, Arc<RecObs>) {
        let dir = tempfile::TempDir::new().unwrap();
        let files = CertFile::new(
            dir.path().join("svc-cert.pem"),
            dir.path().join("svc-key.pem"),
        );
        let obs = RecObs::new();
        let mut cfg = test_cfg();
        cfg.cert_path = files.cert_path.to_string_lossy().into();
        cfg.key_path = files.key_path.to_string_lossy().into();
        let mgr = Manager {
            cfg,
            issuer,
            files,
            observer: Arc::clone(&obs) as Arc<dyn Observer>,
            now: Box::new(move || now),
        };
        (mgr, dir, obs)
    }

    #[tokio::test]
    async fn tick_missing_triggers_issue() {
        let nb = ts(1_755_475_200);
        let iss = FakeIssuer::new(nb, Duration::from_secs(24 * 3600));
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, nb);
        let state = mgr.tick(&CancellationToken::new()).await.unwrap();
        assert_eq!(state, State::Fresh);
        assert_eq!(iss.call_count(), 1);
        assert_eq!(obs.reissued.lock().unwrap().as_slice(), ["missing"]);
    }

    #[tokio::test]
    async fn tick_mismatch_classifies_corrupt_and_reissues() {
        let nb = ts(1_755_475_200);
        let now = nb + Duration::from_secs(3600);
        let iss = FakeIssuer::new(now, Duration::from_secs(24 * 3600));
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, now);
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        let (_other_cert, other_key) =
            self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        let mut perms = fs::metadata(&mgr.files.key_path).unwrap().permissions();
        perms.set_mode(0o600);
        fs::set_permissions(&mgr.files.key_path, perms).unwrap();
        fs::write(&mgr.files.key_path, other_key).unwrap();
        let state = mgr.tick(&CancellationToken::new()).await.unwrap();
        let classified = obs.classified.lock().unwrap().clone();
        assert_eq!(
            classified.first(),
            Some(&State::Corrupt),
            "mismatch must be classified Corrupt before reissue, got {classified:?}"
        );
        assert_eq!(obs.reissued.lock().unwrap().as_slice(), ["corrupt"]);
        assert_eq!(iss.call_count(), 1);
        mgr.files
            .load()
            .expect("reissue must leave a matching pair");
        assert_eq!(
            state,
            State::Fresh,
            "successful corrupt reissue yields a matching Fresh leaf"
        );
    }

    #[tokio::test]
    async fn tick_fresh_noop() {
        let nb = ts(1_755_475_200);
        let iss = FakeIssuer::new(nb, Duration::from_secs(24 * 3600));
        let (mgr, _dir, _obs) = new_manager(
            Arc::clone(&iss) as Arc<dyn Issuer>,
            nb + Duration::from_secs(3600),
        );
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        mgr.tick(&CancellationToken::new()).await.unwrap();
        assert_eq!(iss.call_count(), 0);
    }

    #[tokio::test]
    async fn tick_near_expiry_reissues() {
        let nb = ts(1_755_475_200);
        let now = nb + Duration::from_secs(16 * 3600);
        let iss = FakeIssuer::new(now, Duration::from_secs(24 * 3600));
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, now);
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        mgr.tick(&CancellationToken::new()).await.unwrap();
        assert_eq!(iss.call_count(), 1);
        assert_eq!(obs.reissued.lock().unwrap().as_slice(), ["near_expiry"]);
    }

    #[tokio::test]
    async fn tick_expired_reenrolls_not_renews() {
        let nb = ts(1_755_475_200);
        let now = nb + Duration::from_secs(25 * 3600);
        let iss = FakeIssuer::new(now, Duration::from_secs(24 * 3600));
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, now);
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        mgr.tick(&CancellationToken::new()).await.unwrap();
        assert_eq!(iss.call_count(), 1);
        assert_eq!(obs.reissued.lock().unwrap().as_slice(), ["expired"]);
    }

    #[tokio::test]
    async fn tick_issuer_fails_propagates() {
        let iss = FakeIssuer::failing("CA down");
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, SystemTime::now());
        assert!(mgr.tick(&CancellationToken::new()).await.is_err());
        assert_eq!(obs.renewed.lock().unwrap().as_slice(), [false]);
    }

    #[tokio::test]
    async fn enroll_retries_then_fails() {
        let iss = FakeIssuer::failing("CA down");
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, SystemTime::now());
        assert!(mgr.enroll(&CancellationToken::new()).await.is_err());
        assert_eq!(iss.call_count(), 3);
        assert!(obs.retries.load(Ordering::SeqCst) > 0);
    }

    #[tokio::test]
    async fn enroll_canceled() {
        let iss = FakeIssuer::new(SystemTime::now(), Duration::from_secs(3600));
        let block = CancellationToken::new();
        *iss.block.lock().await = Some(block.clone());
        let (mgr, _dir, _obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, SystemTime::now());
        let cancel = CancellationToken::new();
        let cancel2 = cancel.clone();
        let task = tokio::spawn(async move { mgr.enroll(&cancel2).await });
        tokio::time::sleep(Duration::from_millis(20)).await;
        cancel.cancel();
        let err = task.await.unwrap().expect_err("canceled");
        assert!(err.is_canceled(), "{err}");
    }

    #[tokio::test]
    async fn run_stops_on_cancel() {
        let nb = ts(1_755_475_200);
        let iss = FakeIssuer::new(nb, Duration::from_secs(24 * 3600));
        let (mut mgr, _dir, _obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, nb);
        mgr.cfg.tick_interval = Duration::from_millis(30);
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        let cancel = CancellationToken::new();
        let cancel2 = cancel.clone();
        let task = tokio::spawn(async move { mgr.run(&cancel2).await });
        cancel.cancel();
        let err = task.await.unwrap().expect_err("canceled");
        assert!(err.is_canceled(), "{err}");
    }

    #[tokio::test]
    async fn tick_atomic_write_visible_to_rustls_reloader() {
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
        let nb = ts(1_755_475_200);
        let iss = FakeIssuer::new(nb, Duration::from_secs(3600));
        let (mgr, _dir, _obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, nb);
        mgr.tick(&CancellationToken::new()).await.unwrap();
        let resolver = crate::clients::mtls::ReloadingCertResolver::new_with_check_interval(
            &mgr.files.cert_path,
            &mgr.files.key_path,
            Duration::ZERO,
        )
        .expect("initial load");
        let first = resolver.current().expect("first");
        let first_der = first.cert[0].to_vec();

        let now = nb + Duration::from_secs(50 * 60);
        // Replace the on-disk cert with an old one, then tick near-expiry
        // so the manager issues a replacement the resolver must observe.
        let (old_cert, old_key) =
            self_signed_pem("recap-worker", nb, nb + Duration::from_secs(3600));
        mgr.files.write(&old_cert, &old_key).unwrap();
        let mgr = Manager {
            now: Box::new(move || now),
            ..mgr
        };
        *iss.not_before.lock().unwrap() = nb + Duration::from_secs(60);
        mgr.tick(&CancellationToken::new()).await.unwrap();

        let second = resolver.current().expect("second");
        let second_der = second.cert[0].to_vec();
        assert_ne!(
            first_der, second_der,
            "reloader did not observe atomic rotation"
        );
    }

    #[tokio::test]
    async fn tick_near_expiry_uses_rekey_when_available() {
        let nb = ts(1_755_475_200);
        let now = nb + Duration::from_secs(16 * 3600);
        let iss = Arc::new(FakeIssuer {
            calls: AtomicU32::new(0),
            err: Mutex::new(None),
            not_before: Mutex::new(now),
            lifetime: Duration::from_secs(24 * 3600),
            block: tokio::sync::Mutex::new(None),
            rekey_calls: AtomicU32::new(0),
            rekey: true,
        });
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, now);
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        mgr.tick(&CancellationToken::new()).await.unwrap();
        assert_eq!(iss.rekey_calls.load(Ordering::SeqCst), 1);
        assert_eq!(iss.call_count(), 1);
        assert_eq!(obs.reissued.lock().unwrap().as_slice(), ["near_expiry"]);
    }

    #[tokio::test]
    async fn tick_expired_does_not_rekey() {
        let nb = ts(1_755_475_200);
        let now = nb + Duration::from_secs(25 * 3600);
        let iss = Arc::new(FakeIssuer {
            calls: AtomicU32::new(0),
            err: Mutex::new(None),
            not_before: Mutex::new(now),
            lifetime: Duration::from_secs(24 * 3600),
            block: tokio::sync::Mutex::new(None),
            rekey_calls: AtomicU32::new(0),
            rekey: true,
        });
        let (mgr, _dir, obs) = new_manager(Arc::clone(&iss) as Arc<dyn Issuer>, now);
        let (cert, key) = self_signed_pem("recap-worker", nb, nb + Duration::from_secs(24 * 3600));
        mgr.files.write(&cert, &key).unwrap();
        mgr.tick(&CancellationToken::new()).await.unwrap();
        assert_eq!(iss.rekey_calls.load(Ordering::SeqCst), 0);
        assert_eq!(iss.call_count(), 1);
        assert_eq!(obs.reissued.lock().unwrap().as_slice(), ["expired"]);
    }
}
