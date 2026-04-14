//! Shared mTLS `reqwest::Client` builder for east-west calls.
//!
//! When `MTLS_ENFORCE=true`, each per-service caller (alt-backend,
//! tag-generator) can be constructed with a client that presents the
//! recap-worker leaf cert on every handshake and trusts the internal alt-ca.

use std::time::Duration;

use anyhow::{Context, Result};
use reqwest::{Certificate, Client, Identity};

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

/// Builds a `reqwest::Client` that presents the supplied identity and trusts
/// the supplied CA bundle. `connect_timeout` and `total_timeout` mirror the
/// per-service client settings so the resulting client keeps the same
/// resource-bounding behaviour as the non-mTLS path.
pub(crate) fn build_mtls_client(
    paths: &MtlsPaths,
    connect_timeout: Duration,
    total_timeout: Duration,
) -> Result<Client> {
    let cert_pem = std::fs::read(&paths.cert)
        .with_context(|| format!("failed to read mTLS cert {}", paths.cert))?;
    let key_pem = std::fs::read(&paths.key)
        .with_context(|| format!("failed to read mTLS key {}", paths.key))?;
    let ca_pem = std::fs::read(&paths.ca)
        .with_context(|| format!("failed to read CA bundle {}", paths.ca))?;

    // reqwest's Identity::from_pem expects cert + private key concatenated.
    let mut identity_pem = Vec::with_capacity(cert_pem.len() + key_pem.len() + 1);
    identity_pem.extend_from_slice(&cert_pem);
    if !cert_pem.ends_with(b"\n") {
        identity_pem.push(b'\n');
    }
    identity_pem.extend_from_slice(&key_pem);

    let identity =
        Identity::from_pem(&identity_pem).context("failed to parse mTLS identity (cert+key)")?;
    let ca = Certificate::from_pem(&ca_pem).context("failed to parse CA bundle")?;

    Client::builder()
        .identity(identity)
        .add_root_certificate(ca)
        .https_only(true)
        .connect_timeout(connect_timeout)
        .timeout(total_timeout)
        .build()
        .context("failed to build mTLS reqwest Client")
}
