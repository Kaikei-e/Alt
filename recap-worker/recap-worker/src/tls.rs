//! Server-side mTLS helpers for recap-worker's axum listener.
//!
//! Outbound mTLS client construction lives in `crate::clients::mtls`; this
//! module provides only the server side.
//!
//! Primitives:
//!   - [`load_server_tls_config`]: loads the pair of PEM files into a
//!     `rustls::ServerConfig` suitable for `PeerIdentityAcceptor`. When
//!     `MTLS_ENFORCE` is not `true`, returns `None` so the caller can fall
//!     back to the plaintext `tokio::net::TcpListener`.
//!
//! Configuration is env-driven:
//!   - `MTLS_ENFORCE=true` enables mTLS; any other value keeps plaintext.
//!   - `MTLS_CERT_FILE` / `MTLS_KEY_FILE`: leaf cert + private key (PEM).
//!   - `MTLS_CA_FILE`: trust anchor for client-cert verification.
//!   - `MTLS_ALLOWED_PEERS`: CSV of CNs permitted to reach the server.

use std::fs;
use std::io;
use std::sync::Arc;

use anyhow::{Context, Result};
use rustls::client::danger::HandshakeSignatureValid;
use rustls::pki_types::{CertificateDer, UnixTime};
use rustls::server::danger::{ClientCertVerified, ClientCertVerifier};
use rustls::{
    DigitallySignedStruct, DistinguishedName, RootCertStore, ServerConfig, SignatureScheme,
};
use x509_parser::prelude::{FromDer, GeneralName, X509Certificate};

/// Client-certificate verifier that delegates cryptographic chain validation to
/// an inner [`ClientCertVerifier`] (typically `WebPkiClientVerifier`) and then
/// enforces that the leaf certificate's Common Name or DNS SAN matches one of
/// the configured `allowed_peers`.
#[derive(Debug)]
pub struct AllowedNamesVerifier {
    inner: Arc<dyn ClientCertVerifier>,
    allowed_peers: Vec<String>,
}

pub(crate) fn verify_cert_peer_identity(
    end_entity: &CertificateDer<'_>,
    allowed_peers: &[String],
) -> std::result::Result<(), rustls::Error> {
    if allowed_peers.is_empty() {
        return Err(rustls::Error::General(
            "MTLS_ALLOWED_PEERS allowlist is empty".into(),
        ));
    }

    let (_, cert) = X509Certificate::from_der(end_entity.as_ref())
        .map_err(|_| rustls::Error::InvalidCertificate(rustls::CertificateError::BadEncoding))?;

    let cn_matches = cert
        .subject()
        .iter_common_name()
        .filter_map(|cn| cn.as_str().ok())
        .any(|cn| allowed_peers.iter().any(|allowed| allowed == cn));

    let san_matches = cert
        .subject_alternative_name()
        .ok()
        .flatten()
        .is_some_and(|ext| {
            ext.value.general_names.iter().any(|name| match name {
                GeneralName::DNSName(dns) => allowed_peers.iter().any(|allowed| allowed == dns),
                _ => false,
            })
        });

    if cn_matches || san_matches {
        Ok(())
    } else {
        Err(rustls::Error::General(
            "client certificate peer identity not in MTLS_ALLOWED_PEERS".into(),
        ))
    }
}

impl AllowedNamesVerifier {
    pub fn new(inner: Arc<dyn ClientCertVerifier>, allowed_peers: Vec<String>) -> Self {
        Self {
            inner,
            allowed_peers,
        }
    }

    pub fn verify_peer_identity(
        &self,
        end_entity: &CertificateDer<'_>,
    ) -> std::result::Result<(), rustls::Error> {
        verify_cert_peer_identity(end_entity, &self.allowed_peers)
    }
}

impl ClientCertVerifier for AllowedNamesVerifier {
    fn offer_client_auth(&self) -> bool {
        self.inner.offer_client_auth()
    }

    fn client_auth_mandatory(&self) -> bool {
        self.inner.client_auth_mandatory()
    }

    fn root_hint_subjects(&self) -> &[DistinguishedName] {
        self.inner.root_hint_subjects()
    }

    fn verify_client_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        intermediates: &[CertificateDer<'_>],
        now: UnixTime,
    ) -> std::result::Result<ClientCertVerified, rustls::Error> {
        let verified = self
            .inner
            .verify_client_cert(end_entity, intermediates, now)?;
        self.verify_peer_identity(end_entity)?;
        Ok(verified)
    }

    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> std::result::Result<HandshakeSignatureValid, rustls::Error> {
        self.inner.verify_tls12_signature(message, cert, dss)
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> std::result::Result<HandshakeSignatureValid, rustls::Error> {
        self.inner.verify_tls13_signature(message, cert, dss)
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        self.inner.supported_verify_schemes()
    }
}

/// Returns true when `MTLS_ENFORCE=true`. Any other value (including unset)
/// keeps the service in plaintext mode — the shared-secret layer continues
/// to gate outbound calls until the hard-cutover lands.
pub fn enforced() -> bool {
    std::env::var("MTLS_ENFORCE").is_ok_and(|v| v.eq_ignore_ascii_case("true"))
}

fn load_pem_certs(path: &str) -> Result<Vec<CertificateDer<'static>>> {
    let raw = fs::read(path).with_context(|| format!("read cert file {path}"))?;
    let mut reader = io::Cursor::new(raw);
    let certs: Vec<_> = rustls_pemfile::certs(&mut reader).collect::<Result<Vec<_>, _>>()?;
    if certs.is_empty() {
        anyhow::bail!("no certificates found in {path}");
    }
    Ok(certs)
}

/// Load the leaf cert + key + CA bundle from env-specified files and produce
/// a rustls `ServerConfig` that **requires** a valid client cert signed by
/// the alt-CA. Returns None if `MTLS_ENFORCE=false` — the caller should then
/// fall back to the plaintext listener.
///
/// The leaf cert / key are served via a `ReloadingCertResolver` so the
/// pki-agent sidecar can rotate them on disk without restarting the process.
pub fn load_server_tls_config(
    peer_identity: &crate::config::PeerIdentityConfig,
) -> Result<Option<Arc<ServerConfig>>> {
    if !enforced() {
        return Ok(None);
    }

    let cert_file = std::env::var("MTLS_CERT_FILE").context("MTLS_CERT_FILE unset")?;
    let key_file = std::env::var("MTLS_KEY_FILE").context("MTLS_KEY_FILE unset")?;
    let ca_file = std::env::var("MTLS_CA_FILE").context("MTLS_CA_FILE unset")?;

    let mut trust_store = RootCertStore::empty();
    for cert in load_pem_certs(&ca_file)? {
        trust_store.add(cert).context("add CA to trust store")?;
    }

    let webpki_verifier =
        rustls::server::WebPkiClientVerifier::builder(Arc::new(trust_store)).build()?;
    let client_verifier: Arc<dyn ClientCertVerifier> = match peer_identity {
        crate::config::PeerIdentityConfig::Disabled => webpki_verifier,
        crate::config::PeerIdentityConfig::Enabled { allowed_peers } => {
            if allowed_peers.is_empty() {
                anyhow::bail!("MTLS_ALLOWED_PEERS allowlist is empty");
            }
            if allowed_peers
                .iter()
                .any(|peer| peer.eq_ignore_ascii_case("localhost"))
            {
                anyhow::bail!("'localhost' is not permitted in MTLS_ALLOWED_PEERS");
            }
            Arc::new(AllowedNamesVerifier::new(
                webpki_verifier,
                allowed_peers.clone(),
            ))
        }
    };

    let resolver = crate::clients::mtls::server_cert_resolver(&cert_file, &key_file)?;

    let server_config = ServerConfig::builder()
        .with_client_cert_verifier(client_verifier)
        .with_cert_resolver(resolver);

    Ok(Some(Arc::new(server_config)))
}

#[derive(Clone)]
pub struct PeerIdentityAcceptor {
    inner: axum_server::tls_rustls::RustlsAcceptor,
}

impl PeerIdentityAcceptor {
    #[must_use]
    pub fn new(config: axum_server::tls_rustls::RustlsConfig) -> Self {
        Self {
            inner: axum_server::tls_rustls::RustlsAcceptor::new(config),
        }
    }
}

#[derive(Clone)]
pub struct PeerCertExtensionService<S> {
    inner: S,
    leaf_cert: Option<CertificateDer<'static>>,
}

impl<S, B> tower::Service<axum::http::Request<B>> for PeerCertExtensionService<S>
where
    S: tower::Service<axum::http::Request<B>>,
{
    type Response = S::Response;
    type Error = S::Error;
    type Future = S::Future;

    fn poll_ready(
        &mut self,
        cx: &mut std::task::Context<'_>,
    ) -> std::task::Poll<std::result::Result<(), Self::Error>> {
        self.inner.poll_ready(cx)
    }

    fn call(&mut self, mut req: axum::http::Request<B>) -> Self::Future {
        req.headers_mut().remove("x-alt-peer-identity");
        if let Some(cert) = self.leaf_cert.as_ref() {
            req.extensions_mut().insert(cert.clone());
        }
        self.inner.call(req)
    }
}

impl<I, S> axum_server::accept::Accept<I, S> for PeerIdentityAcceptor
where
    I: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
    S: Send + 'static,
{
    type Stream = tokio_rustls::server::TlsStream<I>;
    type Service = PeerCertExtensionService<S>;
    type Future = futures::future::BoxFuture<'static, io::Result<(Self::Stream, Self::Service)>>;

    fn accept(&self, stream: I, service: S) -> Self::Future {
        let inner = self.inner.clone();
        Box::pin(async move {
            let (tls_stream, service) = inner.accept(stream, service).await?;
            let leaf_cert = tls_stream
                .get_ref()
                .1
                .peer_certificates()
                .and_then(|certs| certs.first())
                .map(|cert| cert.clone().into_owned());
            Ok((
                tls_stream,
                PeerCertExtensionService {
                    inner: service,
                    leaf_cert,
                },
            ))
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn enforced_false_by_default() {
        temp_env::with_var("MTLS_ENFORCE", None::<&str>, || {
            assert!(!enforced());
        });
    }

    #[test]
    fn enforced_true_when_env_true() {
        temp_env::with_var("MTLS_ENFORCE", Some("true"), || {
            assert!(enforced());
        });
    }

    #[test]
    fn enforced_case_insensitive() {
        temp_env::with_var("MTLS_ENFORCE", Some("TRUE"), || {
            assert!(enforced());
        });
    }

    #[test]
    fn load_server_config_returns_none_when_not_enforced() {
        temp_env::with_var("MTLS_ENFORCE", Some("false"), || {
            assert!(
                load_server_tls_config(&crate::config::PeerIdentityConfig::Disabled)
                    .unwrap()
                    .is_none()
            );
        });
    }

    #[test]
    fn allowed_peers_parses_csv() {
        temp_env::with_var(
            "MTLS_ALLOWED_PEERS",
            Some(" alt-backend , search-indexer ,  , acolyte-orchestrator "),
            || {
                let parsed = crate::config::load_peer_identity_from_env().unwrap();
                assert_eq!(
                    parsed.allowed_peers(),
                    Some(
                        [
                            "alt-backend".to_string(),
                            "search-indexer".to_string(),
                            "acolyte-orchestrator".to_string()
                        ]
                        .as_slice()
                    )
                );
            },
        );
    }

    #[test]
    fn load_server_config_uses_passed_peer_identity_without_re_reading_env_and_rejects_bad_peers() {
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
        let temp_dir = tempfile::tempdir().unwrap();
        let ca_params = rcgen::CertificateParams::new(vec!["alt-ca".to_string()]).unwrap();
        let ca_key = rcgen::KeyPair::generate().unwrap();
        let ca_cert = ca_params.self_signed(&ca_key).unwrap();

        let leaf_params = rcgen::CertificateParams::new(vec!["recap-worker".to_string()]).unwrap();
        let leaf_key = rcgen::KeyPair::generate().unwrap();
        let leaf_cert = leaf_params.self_signed(&leaf_key).unwrap();

        let cert_path = temp_dir.path().join("cert.pem");
        let key_path = temp_dir.path().join("key.pem");
        let ca_path = temp_dir.path().join("ca.pem");
        std::fs::write(&cert_path, leaf_cert.pem()).unwrap();
        std::fs::write(&key_path, leaf_key.serialize_pem()).unwrap();
        std::fs::write(&ca_path, ca_cert.pem()).unwrap();

        temp_env::with_vars(
            [
                ("MTLS_ENFORCE", Some("true")),
                ("MTLS_CERT_FILE", Some(cert_path.to_str().unwrap())),
                ("MTLS_KEY_FILE", Some(key_path.to_str().unwrap())),
                ("MTLS_CA_FILE", Some(ca_path.to_str().unwrap())),
                ("MTLS_ALLOWED_PEERS", None::<&str>),
            ],
            || {
                let valid = crate::config::PeerIdentityConfig::Enabled {
                    allowed_peers: vec!["alt-backend".to_string()],
                };
                assert!(
                    load_server_tls_config(&valid).unwrap().is_some(),
                    "load_server_tls_config must use passed PeerIdentityConfig without reading MTLS_ALLOWED_PEERS from env"
                );

                for bad_peers in [Vec::new(), vec!["localhost".to_string()]] {
                    let bad = crate::config::PeerIdentityConfig::Enabled {
                        allowed_peers: bad_peers,
                    };
                    let err = load_server_tls_config(&bad).unwrap_err();
                    assert!(
                        format!("{err:#}").contains("MTLS_ALLOWED_PEERS"),
                        "error must name MTLS_ALLOWED_PEERS, got: {err:#}"
                    );
                }
            },
        );
    }

    #[test]
    fn load_server_config_missing_cert_env_errors_when_enforced() {
        temp_env::with_vars(
            [
                ("MTLS_ENFORCE", Some("true")),
                ("MTLS_CERT_FILE", None),
                ("MTLS_KEY_FILE", None),
                ("MTLS_CA_FILE", None),
            ],
            || {
                assert!(
                    load_server_tls_config(&crate::config::PeerIdentityConfig::Disabled).is_err()
                );
            },
        );
    }

    #[test]
    fn allowed_names_verifier_accepts_permitted_peer_and_rejects_unlisted_peer() {
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();

        let ca_params = rcgen::CertificateParams::new(vec!["alt-ca".to_string()]).unwrap();
        let ca_key = rcgen::KeyPair::generate().unwrap();
        let ca_cert = ca_params.self_signed(&ca_key).unwrap();

        let mut store = RootCertStore::empty();
        store.add(ca_cert.der().clone()).unwrap();
        let inner = rustls::server::WebPkiClientVerifier::builder(Arc::new(store))
            .build()
            .unwrap();

        let verifier = AllowedNamesVerifier::new(inner, vec!["alt-backend".to_string()]);

        let allowed_params =
            rcgen::CertificateParams::new(vec!["alt-backend".to_string()]).unwrap();
        let allowed_key = rcgen::KeyPair::generate().unwrap();
        let allowed_cert = allowed_params.self_signed(&allowed_key).unwrap();
        assert!(verifier.verify_peer_identity(allowed_cert.der()).is_ok());

        let denied_params =
            rcgen::CertificateParams::new(vec!["untrusted-peer".to_string()]).unwrap();
        let denied_key = rcgen::KeyPair::generate().unwrap();
        let denied_cert = denied_params.self_signed(&denied_key).unwrap();
        assert!(verifier.verify_peer_identity(denied_cert.der()).is_err());
    }
}
