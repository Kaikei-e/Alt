//! Crate-level domain error classification.
//!
//! `RecapError` is the typed error for the domain call chains that feed
//! `pipeline::persist`'s genre-outcome classification (2026-07-06 large-repo
//! review, "error design" finding): fetch (`pipeline::fetch`), clustering
//! (`clients::subworker::clustering` -> `pipeline::dispatch`), summary
//! (`clients::news_creator` -> `pipeline::dispatch::summarization`), and the
//! DB access layer (`store::dao`, `queue::store`, `util::idempotency`).
//! Those chains return `Result<_, RecapError>` end-to-end, so persist can
//! classify on the enum instead of pattern-matching formatted message text.
//!
//! `GenreResult::error_kind` (see `pipeline::dispatch::types`) carries this
//! enum across the resumable job-state boundary, so it derives
//! `Serialize`/`Deserialize` alongside the usual `thiserror::Error` derive.
//!
//! `anyhow` intentionally remains in:
//! - binary/bootstrap surfaces (`main`, `app`, `observability`, `tls`,
//!   `clients::mtls`) and the axum handlers under `api/` — binary
//!   boundaries per the Rust DECREE (`RecapError` converts into `anyhow`
//!   there via `?`);
//! - job orchestration that only folds failures into job-status text
//!   (`pipeline::executor`/`orchestrator`, `queue`, `scheduler`, `replay`)
//!   and the pipeline stages whose errors fail the whole job rather than
//!   classify a single genre (`preprocess`, `dedup`, `genre*`, `select`,
//!   `embedding`, `morning`, `pulse`), plus the classification/evaluation
//!   chains and their clients — none of these feed `error_kind`, and
//!   forcing them into the variants below would blur the classification;
//! - `clients::alt_backend`, deliberately: `pipeline::fetch`'s retry loop
//!   downcasts to `reqwest::Error` for retryability, which a
//!   `String`-payload variant would erase. The fetch stage converts to
//!   `RecapError::Fetch` at its boundary instead;
//! - test code.

use thiserror::Error;

/// Crate-default `Result` for the migrated domain call chains.
pub(crate) type Result<T, E = RecapError> = std::result::Result<T, E>;

#[derive(Debug, Error, Clone, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub(crate) enum RecapError {
    /// Fetching source documents/evidence failed.
    #[error("fetch failed: {0}")]
    Fetch(String),

    /// The subworker clustering call failed for a reason other than
    /// insufficient documents (timeout, HTTP error, validation error, ...).
    #[error("clustering failed: {0}")]
    Clustering(String),

    /// Summary generation (news-creator) failed.
    #[error("summary generation failed: {0}")]
    Summary(String),

    /// A database read/write failed.
    #[error("database operation failed: {0}")]
    Db(String),

    /// The genre had too few documents to cluster, below even the fallback
    /// single-cluster threshold. This is an expected skip, not a failure.
    #[error("insufficient documents for clustering: expected >= {min}, found {found}")]
    InsufficientDocuments { min: usize, found: usize },

    /// The genre had no articles assigned at all. This is an expected
    /// completion state, not a failure.
    #[error("no evidence: no articles assigned to genre")]
    NoEvidence,
}

impl RecapError {
    /// Prefix a higher-level message onto the error, preserving the variant.
    ///
    /// The typed replacement for `anyhow::Context`: adding call-site framing
    /// must never collapse the classification (in particular, the benign
    /// `InsufficientDocuments`/`NoEvidence` states pass through untouched so
    /// `pipeline::persist` still recognizes them).
    pub(crate) fn context(self, msg: impl std::fmt::Display) -> Self {
        match self {
            Self::Fetch(m) => Self::Fetch(format!("{msg}: {m}")),
            Self::Clustering(m) => Self::Clustering(format!("{msg}: {m}")),
            Self::Summary(m) => Self::Summary(format!("{msg}: {m}")),
            Self::Db(m) => Self::Db(format!("{msg}: {m}")),
            e @ (Self::InsufficientDocuments { .. } | Self::NoEvidence) => e,
        }
    }
}

impl From<sqlx::Error> for RecapError {
    fn from(e: sqlx::Error) -> Self {
        Self::Db(e.to_string())
    }
}
