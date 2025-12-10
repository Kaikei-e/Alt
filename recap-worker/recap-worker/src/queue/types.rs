use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use uuid::Uuid;

/// Status of a queued classification job
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub(crate) enum QueuedJobStatus {
    Pending,
    Running,
    Completed,
    Failed,
    Retrying,
}

impl QueuedJobStatus {
    #[allow(dead_code)]
    pub(crate) fn as_str(self) -> &'static str {
        match self {
            QueuedJobStatus::Pending => "pending",
            QueuedJobStatus::Running => "running",
            QueuedJobStatus::Completed => "completed",
            QueuedJobStatus::Failed => "failed",
            QueuedJobStatus::Retrying => "retrying",
        }
    }

    pub(crate) fn from_str(s: &str) -> Option<Self> {
        match s {
            "pending" => Some(QueuedJobStatus::Pending),
            "running" => Some(QueuedJobStatus::Running),
            "completed" => Some(QueuedJobStatus::Completed),
            "failed" => Some(QueuedJobStatus::Failed),
            "retrying" => Some(QueuedJobStatus::Retrying),
            _ => None,
        }
    }
}

/// Classification result for a single text
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct ClassificationResult {
    pub(crate) top_genre: String,
    pub(crate) confidence: f32,
    pub(crate) scores: HashMap<String, f32>,
}

/// Queued job ID (database primary key)
pub(crate) type QueuedJobId = i32;

/// A classification job queued for processing
#[derive(Debug, Clone)]
pub(crate) struct QueuedJob {
    pub(crate) id: QueuedJobId,
    pub(crate) recap_job_id: Uuid,
    pub(crate) chunk_idx: usize,
    pub(crate) status: QueuedJobStatus,
    pub(crate) texts: Vec<String>,
    #[allow(dead_code)]
    pub(crate) result: Option<Vec<ClassificationResult>>,
    #[allow(dead_code)]
    pub(crate) error_message: Option<String>,
    pub(crate) retry_count: i32,
    pub(crate) max_retries: i32,
    #[allow(dead_code)]
    pub(crate) created_at: chrono::DateTime<chrono::Utc>,
    #[allow(dead_code)]
    pub(crate) started_at: Option<chrono::DateTime<chrono::Utc>>,
    #[allow(dead_code)]
    pub(crate) completed_at: Option<chrono::DateTime<chrono::Utc>>,
}

/// New job to be inserted into the queue
#[derive(Debug, Clone)]
pub(crate) struct NewQueuedJob {
    pub(crate) recap_job_id: Uuid,
    pub(crate) chunk_idx: usize,
    pub(crate) texts: Vec<String>,
    pub(crate) max_retries: i32,
}
