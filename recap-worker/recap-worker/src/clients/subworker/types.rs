use std::collections::HashMap;
use std::fmt;

use serde::{Deserialize, Serialize};
use serde_json::Value;
use uuid::Uuid;

// Constants
pub(crate) const DEFAULT_MAX_SENTENCES_TOTAL: usize = 2_000;
pub(crate) const DEFAULT_UMAP_N_COMPONENTS: usize = 25;
pub(crate) const DEFAULT_HDBSCAN_MIN_CLUSTER_SIZE: usize = 5;
pub(crate) const DEFAULT_MMR_LAMBDA: f32 = 0.35;
pub(crate) const MIN_PARAGRAPH_LEN: usize = 30;
pub(crate) const MAX_POLL_ATTEMPTS: usize = 200;
pub(crate) const INITIAL_POLL_INTERVAL_MS: u64 = 2_000;
pub(crate) const MAX_POLL_INTERVAL_MS: u64 = 30_000;
pub(crate) const SUBWORKER_TIMEOUT_SECS: u64 = 3600;
pub(crate) const MAX_ERROR_MESSAGE_LENGTH: usize = 500;
pub(crate) const EXTRACTION_TIMEOUT_SECS: u64 = 30;
pub(crate) const MIN_FALLBACK_DOCUMENTS: usize = 2;
pub(crate) const ADMIN_JOB_INITIAL_BACKOFF_MS: u64 = 5_000;
pub(crate) const ADMIN_JOB_MAX_BACKOFF_MS: u64 = 20_000;
pub(crate) const ADMIN_JOB_TIMEOUT_SECS: u64 = 600;
pub(crate) const CLASSIFY_POST_RETRIES: usize = 3;
pub(crate) const CLASSIFY_POST_BACKOFF_MS: u64 = 5_000;
pub(crate) const CLASSIFY_CHUNK_SIZE: usize = 200;
pub(crate) const POLL_REQUEST_RETRIES: usize = 3;
pub(crate) const POLL_REQUEST_RETRY_DELAY_MS: u64 = 1_000;

// Clustering types
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClusteringResponse {
    pub(crate) run_id: i64,
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) status: ClusterJobStatus,
    #[serde(default)]
    pub(crate) cluster_count: usize,
    #[serde(default)]
    pub(crate) clusters: Vec<ClusterInfo>,
    #[serde(default)]
    pub(crate) genre_highlights: Option<Vec<ClusterRepresentative>>,
    #[serde(default)]
    pub(crate) diagnostics: Value,
}

impl ClusteringResponse {
    pub(crate) fn is_success(&self) -> bool {
        self.status.is_success()
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
#[serde(rename_all = "lowercase")]
pub(crate) enum ClusterJobStatus {
    Running,
    Succeeded,
    Partial,
    Failed,
}

impl ClusterJobStatus {
    pub(crate) fn is_running(&self) -> bool {
        matches!(self, ClusterJobStatus::Running)
    }

    pub(crate) fn is_success(&self) -> bool {
        matches!(
            self,
            ClusterJobStatus::Succeeded | ClusterJobStatus::Partial
        )
    }

    pub(crate) fn is_terminal(&self) -> bool {
        !self.is_running()
    }
}

impl fmt::Display for ClusterJobStatus {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ClusterJobStatus::Running => write!(f, "running"),
            ClusterJobStatus::Succeeded => write!(f, "succeeded"),
            ClusterJobStatus::Partial => write!(f, "partial"),
            ClusterJobStatus::Failed => write!(f, "failed"),
        }
    }
}

#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClusterInfo {
    pub(crate) cluster_id: i32,
    pub(crate) size: usize,
    #[serde(default)]
    pub(crate) label: Option<String>,
    #[serde(default)]
    pub(crate) top_terms: Vec<String>,
    #[serde(default)]
    pub(crate) stats: Value,
    #[serde(default)]
    pub(crate) representatives: Vec<ClusterRepresentative>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClusterRepresentative {
    #[serde(default)]
    pub(crate) article_id: String,
    #[serde(default)]
    pub(crate) paragraph_idx: Option<i32>,
    #[serde(rename = "sentence_text")]
    pub(crate) text: String,
    #[serde(default)]
    pub(crate) lang: Option<String>,
    #[serde(default)]
    pub(crate) score: Option<f32>,
    #[serde(default)]
    pub(crate) reasons: Vec<String>,
}

// Classification types
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClassificationResult {
    pub(crate) top_genre: String,
    pub(crate) confidence: f32,
    pub(crate) scores: HashMap<String, f32>,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClassificationRequest {
    pub(crate) texts: Vec<String>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClassificationResponse {
    pub(crate) results: Vec<ClassificationResult>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ClassificationJobResponse {
    pub(crate) run_id: i64,
    pub(crate) job_id: String,
    pub(crate) status: String,
    pub(crate) result_count: usize,
    pub(crate) results: Option<Vec<ClassificationResult>>,
    pub(crate) error_message: Option<String>,
}

// Clustering request types
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterJobRequest<'a> {
    pub(crate) params: ClusterJobParams,
    pub(crate) documents: Vec<ClusterDocument<'a>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) metadata: Option<&'a crate::pipeline::evidence::CorpusMetadata>,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterJobParams {
    pub(crate) max_sentences_total: usize,
    pub(crate) max_sentences_per_cluster: usize,
    pub(crate) umap_n_components: usize,
    pub(crate) hdbscan_min_cluster_size: usize,
    pub(crate) mmr_lambda: f32,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterDocument<'a> {
    pub(crate) article_id: &'a str,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) title: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) lang_hint: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) published_at: Option<&'a String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source_url: Option<&'a String>,
    pub(crate) paragraphs: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) genre_scores: Option<&'a HashMap<String, usize>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) confidence: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) signals: Option<&'a crate::pipeline::evidence::ArticleFeatureSignal>,
}

// Admin job types
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct AdminJobKickResponse {
    pub(crate) job_id: Uuid,
}

#[derive(Debug, Clone, Deserialize)]
#[allow(dead_code)]
pub(crate) struct AdminJobStatusResponse {
    pub(crate) job_id: Uuid,
    pub(crate) kind: String,
    pub(crate) status: String,
    #[serde(default)]
    pub(crate) result: Option<Value>,
    #[serde(default)]
    pub(crate) error: Option<String>,
}

// Utility request/response types
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ExtractRequest<'a> {
    pub(crate) html: &'a str,
    pub(crate) include_comments: bool,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct ExtractResponse {
    pub(crate) text: String,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct CoarseClassifyRequest<'a> {
    pub(crate) text: &'a str,
}

#[derive(Debug, Clone, Deserialize)]
pub(crate) struct CoarseClassifyResponse {
    pub(crate) scores: HashMap<String, f32>,
}

#[derive(Debug, serde::Serialize)]
#[allow(dead_code)]
pub(crate) struct SubClusterOtherRequest {
    pub(crate) sentences: Vec<String>,
}

#[derive(Debug, serde::Deserialize)]
#[allow(dead_code)]
pub(crate) struct SubClusterOtherResponse {
    pub(crate) cluster_ids: Vec<i32>,
    pub(crate) labels: Option<Vec<i32>>,
    pub(crate) centers: Option<Vec<Vec<f32>>>,
}
