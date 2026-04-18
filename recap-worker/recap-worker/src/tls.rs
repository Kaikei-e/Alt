//! Server-side mTLS helpers for recap-worker's axum listener.
//!
//! Outbound mTLS client construction lives in `crate::clients::mtls`; this
//! module provides only the server side.
//!
//! Primitives:
//!   - [`load_server_tls_config`]: loads the pair of PEM files into a
//!     `rustls::ServerConfig` suitable for `axum_server::bind_rustls`. When
//!     `MTLS_ENFORCE` is not `true`, returns `None` so the caller can fall
//!     back to the plaintext `tokio::net::TcpListener`.
//!   - [`allowed_peers_from_env`]: parses `MTLS_ALLOWED_PEERS` for the
//!     peer-identity middleware.
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
use rustls::pki_types::CertificateDer;
use rustls::{RootCertStore, ServerConfig};

/// Returns true when `MTLS_ENFORCE=true`. Any other value (including unset)
/// keeps the service in plaintext mode — the shared-secret layer continues
/// to gate outbound calls until the hard-cutover lands.
pub fn enforced() -> bool {
    std::env::var("MTLS_ENFORCE")
        .ok()
        .is_some_and(|v| v.eq_ignore_ascii_case("true"))
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
pub fn load_server_tls_config() -> Result<Option<Arc<ServerConfig>>> {
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

    let client_verifier =
        rustls::server::WebPkiClientVerifier::builder(Arc::new(trust_store)).build()?;

    let resolver = crate::clients::mtls::server_cert_resolver(&cert_file, &key_file)?;

    let server_config = ServerConfig::builder()
        .with_client_cert_verifier(client_verifier)
        .with_cert_resolver(resolver);

    Ok(Some(Arc::new(server_config)))
}

/// Parse `MTLS_ALLOWED_PEERS=csv` into a Vec of CNs. Trim + drop empties.
pub fn allowed_peers_from_env() -> Vec<String> {
    std::env::var("MTLS_ALLOWED_PEERS")
        .unwrap_or_default()
        .split(',')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect()
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
            assert!(load_server_tls_config().unwrap().is_none());
        });
    }

    #[test]
    fn allowed_peers_parses_csv() {
        temp_env::with_var(
            "MTLS_ALLOWED_PEERS",
            Some(" alt-backend , search-indexer ,  , acolyte-orchestrator "),
            || {
                assert_eq!(
                    allowed_peers_from_env(),
                    vec!["alt-backend", "search-indexer", "acolyte-orchestrator"]
                );
            },
        );
    }

    #[test]
    fn allowed_peers_empty_when_unset() {
        temp_env::with_var("MTLS_ALLOWED_PEERS", None::<&str>, || {
            assert!(allowed_peers_from_env().is_empty());
        });
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
                assert!(load_server_tls_config().is_err());
            },
        );
    }
}
