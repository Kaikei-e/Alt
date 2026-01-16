use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// JobStatus - リキャップジョブの状態を表す列挙型
#[derive(Debug, Clone, PartialEq, sqlx::Type, Serialize, Deserialize)]
#[sqlx(type_name = "text", rename_all = "lowercase")]
#[serde(rename_all = "lowercase")]
pub enum JobStatus {
    Pending,
    Running,
    Completed,
    Failed,
}

impl AsRef<str> for JobStatus {
    fn as_ref(&self) -> &str {
        match self {
            JobStatus::Pending => "pending",
            JobStatus::Running => "running",
            JobStatus::Completed => "completed",
            JobStatus::Failed => "failed",
        }
    }
}

/// TriggerSource - ジョブのトリガー元
#[derive(Debug, Clone, PartialEq, Default, sqlx::Type, Serialize, Deserialize)]
#[sqlx(type_name = "text", rename_all = "lowercase")]
#[serde(rename_all = "lowercase")]
pub enum TriggerSource {
    #[default]
    System,
    User,
}

impl AsRef<str> for TriggerSource {
    fn as_ref(&self) -> &str {
        match self {
            TriggerSource::System => "system",
            TriggerSource::User => "user",
        }
    }
}

/// GenreStatus - ジャンルの処理状態
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum GenreStatus {
    Pending,
    Running,
    Succeeded,
    Failed,
}

impl AsRef<str> for GenreStatus {
    fn as_ref(&self) -> &str {
        match self {
            GenreStatus::Pending => "pending",
            GenreStatus::Running => "running",
            GenreStatus::Succeeded => "succeeded",
            GenreStatus::Failed => "failed",
        }
    }
}

/// PipelineStage - パイプラインのステージ
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum PipelineStage {
    Fetch,
    Preprocess,
    Dedup,
    Genre,
    Select,
    Evidence,
    Dispatch,
    Persist,
}

impl PipelineStage {
    #[allow(dead_code)]
    pub const ALL: [PipelineStage; 8] = [
        PipelineStage::Fetch,
        PipelineStage::Preprocess,
        PipelineStage::Dedup,
        PipelineStage::Genre,
        PipelineStage::Select,
        PipelineStage::Evidence,
        PipelineStage::Dispatch,
        PipelineStage::Persist,
    ];

    pub fn index(self) -> usize {
        match self {
            PipelineStage::Fetch => 0,
            PipelineStage::Preprocess => 1,
            PipelineStage::Dedup => 2,
            PipelineStage::Genre => 3,
            PipelineStage::Select => 4,
            PipelineStage::Evidence => 5,
            PipelineStage::Dispatch => 6,
            PipelineStage::Persist => 7,
        }
    }

    pub fn from_str(s: &str) -> Option<Self> {
        match s.to_lowercase().as_str() {
            "fetch" => Some(PipelineStage::Fetch),
            "preprocess" => Some(PipelineStage::Preprocess),
            "dedup" => Some(PipelineStage::Dedup),
            "genre" => Some(PipelineStage::Genre),
            "select" => Some(PipelineStage::Select),
            "evidence" => Some(PipelineStage::Evidence),
            "dispatch" => Some(PipelineStage::Dispatch),
            "persist" => Some(PipelineStage::Persist),
            _ => None,
        }
    }
}

impl AsRef<str> for PipelineStage {
    fn as_ref(&self) -> &str {
        match self {
            PipelineStage::Fetch => "fetch",
            PipelineStage::Preprocess => "preprocess",
            PipelineStage::Dedup => "dedup",
            PipelineStage::Genre => "genre",
            PipelineStage::Select => "select",
            PipelineStage::Evidence => "evidence",
            PipelineStage::Dispatch => "dispatch",
            PipelineStage::Persist => "persist",
        }
    }
}

/// StatusTransitionActor - ステータス遷移を引き起こしたアクター
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum StatusTransitionActor {
    System,
    Scheduler,
    ManualRepair,
    MigrationBackfill,
}

impl AsRef<str> for StatusTransitionActor {
    fn as_ref(&self) -> &str {
        match self {
            StatusTransitionActor::System => "system",
            StatusTransitionActor::Scheduler => "scheduler",
            StatusTransitionActor::ManualRepair => "manual_repair",
            StatusTransitionActor::MigrationBackfill => "migration_backfill",
        }
    }
}

impl StatusTransitionActor {
    #[allow(dead_code)]
    pub fn from_str(s: &str) -> Self {
        match s {
            "system" => StatusTransitionActor::System,
            "scheduler" => StatusTransitionActor::Scheduler,
            "manual_repair" => StatusTransitionActor::ManualRepair,
            "migration_backfill" => StatusTransitionActor::MigrationBackfill,
            _ => StatusTransitionActor::System, // Default fallback
        }
    }
}

/// JobStatusTransition - ジョブステータス遷移イベント（イミュータブル）
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct JobStatusTransition {
    pub id: i64,
    pub job_id: Uuid,
    pub status: JobStatus,
    pub stage: Option<String>,
    pub transitioned_at: DateTime<Utc>,
    pub reason: Option<String>,
    pub actor: StatusTransitionActor,
}
