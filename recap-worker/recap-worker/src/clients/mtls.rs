//! Shared mTLS `reqwest::Client` builder for east-west calls.
//!
//! When `MTLS_ENFORCE=true`, each per-service caller (alt-backend,
//! tag-generator) can be constructed with a client that presents the
//! recap-worker leaf cert on every handshake and trusts the internal alt-ca.
//!
//! The client cert / key are re-read from disk whenever their mtime advances,
//! so the pki-agent sidecar can rotate the leaf without restarting the
//! process. This mirrors the `certReloader` pattern in
//! `alt-backend/app/tlsutil/tlsutil.go`.

use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::{Duration, SystemTime};

use anyhow::{Context, Result, anyhow};
use reqwest::Client;
use rustls::RootCertStore;
use rustls::client::ResolvesClientCert;
use rustls::crypto::aws_lc_rs::sign::any_supported_type;
use rustls::pki_types::PrivateKeyDer;
use rustls::server::{ClientHello, ResolvesServerCert};
use rustls::sign::CertifiedKey;

/// Paths the builder reads from disk. All three files are required.
#[derive(Debug, Clone)]
pub(crate) struct MtlsPaths {
    pub(crate) cert: String,
    pub(crate) key: String,
    pub(crate) ca: String,
}

impl MtlsPaths {
    /// Returns `Some(paths)` when all three `MTLS_*_FILE` env vars are set.
    /// Returns an error when `MTLS_ENFORCE=true` but one of the paths is
    /// missing — fail-closed to match the Go-side behaviour in alt-backend.
    pub(crate) fn from_env() -> Result<Option<Self>> {
        if std::env::var("MTLS_ENFORCE").unwrap_or_default() != "true" {
            return Ok(None);
        }
        let cert = std::env::var("MTLS_CERT_FILE")
            .context("MTLS_ENFORCE=true but MTLS_CERT_FILE is unset (fail-closed)")?;
        let key = std::env::var("MTLS_KEY_FILE")
            .context("MTLS_ENFORCE=true but MTLS_KEY_FILE is unset (fail-closed)")?;
        let ca = std::env::var("MTLS_CA_FILE")
            .context("MTLS_ENFORCE=true but MTLS_CA_FILE is unset (fail-closed)")?;
        Ok(Some(Self { cert, key, ca }))
    }
}

/// Resolver that re-reads a cert/key pair when the underlying files' mtimes
/// advance. Used as both `ResolvesClientCert` and `ResolvesServerCert`, so a
/// single implementation covers inbound and outbound mTLS on the same host.
///
/// A transient read error (truncated file during atomic rotation, stat
/// failure) falls back to the last successfully loaded `CertifiedKey` so the
/// listener never drops to `None` mid-flight. The initial load still
/// surfaces errors via `new`.
#[derive(Debug)]
pub(crate) struct ReloadingCertResolver {
    cert_path: PathBuf,
    key_path: PathBuf,
    state: Mutex<Option<ResolverState>>,
}

#[derive(Debug)]
struct ResolverState {
    certified: Arc<CertifiedKey>,
    cert_mtime: SystemTime,
    key_mtime: SystemTime,
}

impl ReloadingCertResolver {
    /// Load the cert/key pair for the first time; return an error if the
    /// initial load fails so startup is fail-closed.
    pub(crate) fn new(
        cert_path: impl Into<PathBuf>,
        key_path: impl Into<PathBuf>,
    ) -> Result<Arc<Self>> {
        let resolver = Arc::new(Self {
            cert_path: cert_path.into(),
            key_path: key_path.into(),
            state: Mutex::new(None),
        });
        if resolver.current().is_none() {
            return Err(anyhow!(
                "failed to load initial mTLS identity from {} / {}",
                resolver.cert_path.display(),
                resolver.key_path.display()
            ));
        }
        Ok(resolver)
    }

    /// Returns the current `CertifiedKey`, reloading from disk when either
    /// file's mtime has advanced. Falls back to the cached value when the
    /// fresh read fails so a transient truncation window cannot take the
    /// TLS stack down.
    fn current(&self) -> Option<Arc<CertifiedKey>> {
        let mut guard = self
            .state
            .lock()
            .expect("ReloadingCertResolver state mutex poisoned");

        let cert_mtime = mtime_of(&self.cert_path);
        let key_mtime = mtime_of(&self.key_path);

        let needs_reload = match (&*guard, cert_mtime, key_mtime) {
            (None, _, _) => true,
            (Some(state), Some(cm), Some(km)) => cm > state.cert_mtime || km > state.key_mtime,
            // If we can't stat either file, keep the cached value.
            (Some(_), _, _) => false,
        };

        if needs_reload {
            match load_certified_key(&self.cert_path, &self.key_path) {
                Ok(certified) => {
                    let cm = cert_mtime.unwrap_or(SystemTime::UNIX_EPOCH);
                    let km = key_mtime.unwrap_or(SystemTime::UNIX_EPOCH);
                    *guard = Some(ResolverState {
                        certified: Arc::clone(&certified),
                        cert_mtime: cm,
                        key_mtime: km,
                    });
                    return Some(certified);
                }
                Err(_) => {
                    // Fall back to the last good value if we have one.
                    return guard.as_ref().map(|s| Arc::clone(&s.certified));
                }
            }
        }

        guard.as_ref().map(|s| Arc::clone(&s.certified))
    }
}

fn mtime_of(path: &Path) -> Option<SystemTime> {
    std::fs::metadata(path).ok().and_then(|m| m.modified().ok())
}

fn load_certified_key(cert: &Path, key: &Path) -> Result<Arc<CertifiedKey>> {
    let cert_bytes =
        std::fs::read(cert).with_context(|| format!("read cert file {}", cert.display()))?;
    let key_bytes =
        std::fs::read(key).with_context(|| format!("read key file {}", key.display()))?;

    let certs = rustls_pemfile::certs(&mut &cert_bytes[..])
        .collect::<std::result::Result<Vec<_>, _>>()
        .context("parse cert PEM")?;
    if certs.is_empty() {
        return Err(anyhow!(
            "no certificates found in {}",
            cert.display()
        ));
    }

    let key_der: PrivateKeyDer<'static> = rustls_pemfile::private_key(&mut &key_bytes[..])
        .context("parse key PEM")?
        .ok_or_else(|| anyhow!("no private key found in {}", key.display()))?;
    let signer = any_supported_type(&key_der).context("load private key into signer")?;

    Ok(Arc::new(CertifiedKey::new(certs, signer)))
}

impl ResolvesClientCert for ReloadingCertResolver {
    fn resolve(
        &self,
        _acceptable_issuers: &[&[u8]],
        _sigschemes: &[rustls::SignatureScheme],
    ) -> Option<Arc<CertifiedKey>> {
        self.current()
    }

    fn has_certs(&self) -> bool {
        true
    }
}

impl ResolvesServerCert for ReloadingCertResolver {
    fn resolve(&self, _client_hello: ClientHello<'_>) -> Option<Arc<CertifiedKey>> {
        self.current()
    }
}

/// Builds a `reqwest::Client` that presents the supplied identity and trusts
/// the supplied CA bundle. `connect_timeout` and `total_timeout` mirror the
/// per-service client settings so the resulting client keeps the same
/// resource-bounding behaviour as the non-mTLS path.
///
/// The identity is resolved via a `ReloadingCertResolver`, so cert rotations
/// on disk (pki-agent atomic replace) are picked up on the next handshake
/// without restarting the process.
pub(crate) fn build_mtls_client(
    paths: &MtlsPaths,
    connect_timeout: Duration,
    total_timeout: Duration,
) -> Result<Client> {
    // Installing the default crypto provider is idempotent; callers that
    // already installed one (e.g. `main`) will see this as a no-op.
    let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

    let ca_pem = std::fs::read(&paths.ca)
        .with_context(|| format!("failed to read CA bundle {}", paths.ca))?;
    let mut roots = RootCertStore::empty();
    for cert in rustls_pemfile::certs(&mut &ca_pem[..]) {
        let cert = cert.context("parse CA bundle PEM")?;
        roots.add(cert).context("add CA to trust store")?;
    }

    let resolver = ReloadingCertResolver::new(&paths.cert, &paths.key)?;

    let tls_config = rustls::ClientConfig::builder()
        .with_root_certificates(roots)
        .with_client_cert_resolver(resolver);

    Client::builder()
        .use_preconfigured_tls(tls_config)
        .https_only(true)
        .connect_timeout(connect_timeout)
        .timeout(total_timeout)
        .build()
        .context("failed to build mTLS reqwest Client")
}

/// Construct the reloading cert resolver from MTLS_CERT_FILE / MTLS_KEY_FILE
/// paths. Callers on the server side consume this via `ResolvesServerCert`.
pub(crate) fn server_cert_resolver(
    cert_path: &str,
    key_path: &str,
) -> Result<Arc<dyn ResolvesServerCert>> {
    let resolver = ReloadingCertResolver::new(cert_path, key_path)?;
    Ok(resolver as Arc<dyn ResolvesServerCert>)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::io::Write;
    use tempfile::TempDir;

    fn install_crypto_provider() {
        // Installing is idempotent per-process; tests may share the process.
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
    }

    fn write_test_identity(dir: &TempDir, cn: &str) -> (PathBuf, PathBuf) {
        // Generate a fresh self-signed cert with a unique common name so we
        // can distinguish reloads by the serialized DER of the leaf cert.
        let params = rcgen::CertificateParams::new(vec![cn.to_string()]).expect("rcgen params");
        let key_pair = rcgen::KeyPair::generate().expect("rcgen keypair");
        let cert = params
            .self_signed(&key_pair)
            .expect("rcgen self-sign");

        let cert_pem = cert.pem();
        let key_pem = key_pair.serialize_pem();

        let cert_path = dir.path().join(format!("{cn}-cert.pem"));
        let key_path = dir.path().join(format!("{cn}-key.pem"));

        fs::write(&cert_path, cert_pem).unwrap();
        fs::write(&key_path, key_pem).unwrap();

        (cert_path, key_path)
    }

    fn bump_mtime(path: &Path) {
        // Push the mtime a few seconds into the future so it strictly
        // advances regardless of filesystem mtime granularity.
        let target = std::time::SystemTime::now() + std::time::Duration::from_secs(2);
        let ft = filetime::FileTime::from_system_time(target);
        filetime::set_file_mtime(path, ft).unwrap();
    }

    fn replace_identity(dir: &TempDir, cert_path: &Path, key_path: &Path, cn: &str) {
        let (new_cert, new_key) = write_test_identity(dir, cn);
        fs::copy(&new_cert, cert_path).unwrap();
        fs::copy(&new_key, key_path).unwrap();
        bump_mtime(cert_path);
        bump_mtime(key_path);
    }

    fn leaf_der(ck: &CertifiedKey) -> Vec<u8> {
        ck.cert[0].to_vec()
    }

    #[test]
    fn reloads_when_cert_mtime_advances() {
        install_crypto_provider();
        let dir = TempDir::new().unwrap();
        let (cert, key) = write_test_identity(&dir, "initial");

        let resolver = ReloadingCertResolver::new(&cert, &key).expect("initial load");
        let first = resolver.current().expect("first resolve");
        let first_der = leaf_der(&first);

        replace_identity(&dir, &cert, &key, "rotated");

        let second = resolver.current().expect("second resolve");
        let second_der = leaf_der(&second);

        assert_ne!(
            first_der, second_der,
            "resolver should pick up new cert after mtime advances"
        );
    }

    #[test]
    fn returns_cached_when_mtime_unchanged() {
        install_crypto_provider();
        let dir = TempDir::new().unwrap();
        let (cert, key) = write_test_identity(&dir, "stable");

        let resolver = ReloadingCertResolver::new(&cert, &key).expect("initial load");
        let first = resolver.current().expect("first resolve");
        let second = resolver.current().expect("second resolve");

        assert!(
            Arc::ptr_eq(&first, &second),
            "expected cached Arc to be returned when mtime has not advanced"
        );
    }

    #[test]
    fn falls_back_to_last_good_on_transient_read_error() {
        install_crypto_provider();
        let dir = TempDir::new().unwrap();
        let (cert, key) = write_test_identity(&dir, "fallback");

        let resolver = ReloadingCertResolver::new(&cert, &key).expect("initial load");
        let before = resolver.current().expect("first resolve");

        // Truncate the cert to simulate the mid-rotation window (file exists
        // but contains no complete certificate). Bump mtime so the reloader
        // will attempt a fresh read and fail.
        let mut f = fs::OpenOptions::new().write(true).open(&cert).unwrap();
        f.set_len(0).unwrap();
        f.flush().unwrap();
        drop(f);
        bump_mtime(&cert);

        let after = resolver.current().expect("fallback resolve");
        assert_eq!(
            leaf_der(&before),
            leaf_der(&after),
            "resolver must return last-good cert when fresh read fails"
        );
    }

    #[test]
    fn initial_load_fails_when_cert_missing() {
        install_crypto_provider();
        let dir = TempDir::new().unwrap();
        let missing_cert = dir.path().join("missing.pem");
        let missing_key = dir.path().join("missing.key");

        let err =
            ReloadingCertResolver::new(&missing_cert, &missing_key).expect_err("missing cert");
        let msg = format!("{err:#}");
        assert!(
            msg.contains("failed to load initial mTLS identity"),
            "expected initial-load error, got: {msg}"
        );
    }
}
