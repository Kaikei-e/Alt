use thiserror::Error;

/// Typed enrollment failures. Password, JWT, JWE, and CA bodies never appear
/// in `Display`.
#[derive(Debug, Error)]
pub enum PkiError {
    #[error(
        "pki: shared provisioner pki-agent is forbidden for in-process enrollment (got {got:?})"
    )]
    SharedProvisioner { got: String },
    #[error(
        "pki: provisioner password file must not be the shared step_ca_root_password secret (got {got:?})"
    )]
    SharedRootSecret { got: String },
    #[error("pki: STEP_CA_URL must use https (got {got:?})")]
    InsecureCaUrl { got: String },
    #[error("pki: refusing HTTP redirect to step-ca")]
    Redirect,
    #[error("pki: CA response exceeded size cap")]
    ResponseTooLarge,
    #[error("pki: provisioner listing exceeded page cap")]
    ProvisionerPageLimit,
    #[error("pki: password file exceeded size cap")]
    PasswordTooLarge,
    #[error("pki: CA rejected enrollment (status {status})")]
    CaRejected { status: u16 },
    #[error("pki: CA unavailable (status {status})")]
    CaUnavailable { status: u16 },
    #[error("pki: {path:?} is a symlink")]
    Symlink { path: String },
    #[error("pki: cert not found")]
    CertNotFound,
    #[error("pki: cert parse failed: {0}")]
    CertParseFailed(String),
    #[error("pki: cert and private key do not match")]
    CertKeyMismatch,
    #[error("pki: enroll canceled")]
    Canceled,
    #[error("pki: {0}")]
    Other(String),
}

impl PkiError {
    pub(crate) fn other(msg: impl Into<String>) -> Self {
        Self::Other(msg.into())
    }

    pub(crate) fn is_canceled(&self) -> bool {
        matches!(self, Self::Canceled)
    }

    pub(crate) fn classify_ca_status(status: u16) -> Self {
        if status >= 500 {
            Self::CaUnavailable { status }
        } else {
            Self::CaRejected { status }
        }
    }
}

impl From<std::io::Error> for PkiError {
    fn from(err: std::io::Error) -> Self {
        if err.kind() == std::io::ErrorKind::NotFound {
            Self::CertNotFound
        } else {
            Self::other(format!("io: {err}"))
        }
    }
}
