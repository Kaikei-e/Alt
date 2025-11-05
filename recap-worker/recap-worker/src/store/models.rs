use serde_json::Value;
use uuid::Uuid;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PersistedGenre {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) response_id: Option<String>,
}

impl PersistedGenre {
    pub(crate) fn new(job_id: Uuid, genre: impl Into<String>) -> Self {
        Self {
            job_id,
            genre: genre.into(),
            response_id: None,
        }
    }

    pub(crate) fn with_response_id(mut self, response_id: Option<String>) -> Self {
        self.response_id = response_id;
        self
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[allow(dead_code)]
pub(crate) enum SubworkerRunStatus {
    Running,
    Succeeded,
    Partial,
    Failed,
}

impl SubworkerRunStatus {
    #[must_use]
    pub(crate) fn as_str(self) -> &'static str {
        match self {
            SubworkerRunStatus::Running => "running",
            SubworkerRunStatus::Succeeded => "succeeded",
            SubworkerRunStatus::Partial => "partial",
            SubworkerRunStatus::Failed => "failed",
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
#[allow(dead_code)]
pub(crate) struct NewSubworkerRun {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) status: SubworkerRunStatus,
    pub(crate) request_payload: Value,
}

impl NewSubworkerRun {
    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn new(job_id: Uuid, genre: impl Into<String>, request_payload: Value) -> Self {
        Self {
            job_id,
            genre: genre.into(),
            status: SubworkerRunStatus::Running,
            request_payload,
        }
    }

    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn with_status(mut self, status: SubworkerRunStatus) -> Self {
        self.status = status;
        self
    }
}

#[derive(Debug, Clone, PartialEq)]
#[allow(dead_code)]
pub(crate) struct PersistedCluster {
    pub(crate) cluster_id: i32,
    pub(crate) size: i32,
    pub(crate) label: Option<String>,
    pub(crate) top_terms: Value,
    pub(crate) stats: Value,
    pub(crate) sentences: Vec<PersistedSentence>,
}

impl PersistedCluster {
    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn new(
        cluster_id: i32,
        size: i32,
        label: Option<String>,
        top_terms: Value,
        stats: Value,
        sentences: Vec<PersistedSentence>,
    ) -> Self {
        Self {
            cluster_id,
            size,
            label,
            top_terms,
            stats,
            sentences,
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
#[allow(dead_code)]
pub(crate) struct PersistedSentence {
    pub(crate) article_id: String,
    pub(crate) sentence_id: i32,
    pub(crate) text: String,
    pub(crate) lang: String,
    pub(crate) paragraph_idx: Option<i32>,
    pub(crate) score: f32,
}

impl PersistedSentence {
    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn new(
        article_id: impl Into<String>,
        sentence_id: i32,
        text: impl Into<String>,
        lang: impl Into<String>,
        paragraph_idx: Option<i32>,
        score: f32,
    ) -> Self {
        Self {
            article_id: article_id.into(),
            sentence_id,
            text: text.into(),
            lang: lang.into(),
            paragraph_idx,
            score,
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
#[allow(dead_code)]
pub(crate) struct DiagnosticEntry {
    pub(crate) key: String,
    pub(crate) value: Value,
}

impl DiagnosticEntry {
    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn new(key: impl Into<String>, value: Value) -> Self {
        Self {
            key: key.into(),
            value,
        }
    }
}
