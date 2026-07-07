//! Crate-level domain error classification.
//!
//! Most of recap-worker's call chains still return `anyhow::Result` directly
//! (see the 2026-07-06 large-repo review, "error design" finding). `RecapError`
//! is not an attempt to migrate every call site — it exists so the one call
//! chain that needs to *distinguish* error categories (subworker clustering
//! -> dispatch -> `pipeline::persist`) can do so on a typed enum instead of
//! pattern-matching on formatted message text.
//!
//! `GenreResult::error_kind` (see `pipeline::dispatch::types`) carries this
//! enum across the resumable job-state boundary, so it derives
//! `Serialize`/`Deserialize` alongside the usual `thiserror::Error` derive.

use thiserror::Error;

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
