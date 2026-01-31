//! Types for the select stage.

use serde::{Deserialize, Serialize};
use uuid::Uuid;

use crate::pipeline::genre::GenreAssignment;

/// Selected summary result.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct SelectedSummary {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
}

/// Configuration for subgenre clustering.
#[derive(Clone)]
pub(crate) struct SubgenreConfig {
    pub(crate) max_docs_per_genre: usize,
    pub(crate) target_docs_per_subgenre: usize,
    pub(crate) max_k: usize,
}

impl SubgenreConfig {
    pub(crate) fn new(
        max_docs_per_genre: usize,
        target_docs_per_subgenre: usize,
        max_k: usize,
    ) -> Self {
        Self {
            max_docs_per_genre,
            target_docs_per_subgenre,
            max_k,
        }
    }
}
