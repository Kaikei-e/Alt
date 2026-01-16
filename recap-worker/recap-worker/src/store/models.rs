use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use uuid::Uuid;

// Job model for completed recap jobs
#[derive(Debug, Clone)]
pub(crate) struct RecapJob {
    pub(crate) job_id: Uuid,
    pub(crate) started_at: DateTime<Utc>,
    pub(crate) window_start: DateTime<Utc>,
    pub(crate) window_end: DateTime<Utc>,
    pub(crate) total_articles: Option<i32>,
}

// Genre with summary
#[derive(Debug, Clone)]
pub(crate) struct GenreWithSummary {
    pub(crate) genre_name: String,
    pub(crate) summary_ja: Option<String>,
}

// Cluster with evidence
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterWithEvidence {
    pub(crate) cluster_id: i32,
    pub(crate) top_terms: Option<Vec<String>>,
    pub(crate) evidence: Vec<ClusterEvidence>,
}

// Evidence link
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterEvidence {
    pub(crate) article_id: String,
    pub(crate) title: String,
    pub(crate) source_url: String,
    pub(crate) published_at: DateTime<Utc>,
    pub(crate) lang: Option<String>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PersistedGenre {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) response_id: Option<String>,
}

#[allow(dead_code)]
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

/// Raw記事のバックアップデータ。
///
/// alt-backendから取得した生データをそのまま保存するための構造体。
#[derive(Debug, Clone, PartialEq)]
#[allow(dead_code)]
pub(crate) struct RawArticle {
    pub(crate) article_id: String,
    pub(crate) title: Option<String>,
    pub(crate) fulltext_html: String,
    pub(crate) published_at: Option<DateTime<Utc>>,
    pub(crate) source_url: Option<String>,
    pub(crate) lang_hint: Option<String>,
    pub(crate) normalized_hash: String,
}

impl RawArticle {
    #[must_use]
    #[allow(dead_code)]
    pub(crate) fn new(
        article_id: impl Into<String>,
        title: Option<String>,
        fulltext_html: impl Into<String>,
        published_at: Option<DateTime<Utc>>,
        source_url: Option<String>,
        lang_hint: Option<String>,
        normalized_hash: impl Into<String>,
    ) -> Self {
        Self {
            article_id: article_id.into(),
            title,
            fulltext_html: fulltext_html.into(),
            published_at,
            source_url,
            lang_hint,
            normalized_hash: normalized_hash.into(),
        }
    }
}

/// 前処理統計データ。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct PreprocessMetrics {
    pub(crate) job_id: Uuid,
    pub(crate) total_articles_fetched: i32,
    pub(crate) articles_processed: i32,
    pub(crate) articles_dropped_empty: i32,
    pub(crate) articles_html_cleaned: i32,
    pub(crate) total_characters: i64,
    pub(crate) avg_chars_per_article: Option<f64>,
    pub(crate) languages_detected: Value, // JSON object { "ja": 100, "en": 50, ... }
}

impl PreprocessMetrics {
    #[must_use]
    pub(crate) fn new(
        job_id: Uuid,
        total_articles_fetched: usize,
        articles_processed: usize,
        articles_dropped_empty: usize,
        articles_html_cleaned: usize,
        total_characters: usize,
        languages_detected: Value,
    ) -> Self {
        let avg_chars_per_article = if articles_processed > 0 {
            Some(total_characters as f64 / articles_processed as f64)
        } else {
            None
        };

        Self {
            job_id,
            total_articles_fetched: total_articles_fetched as i32,
            articles_processed: articles_processed as i32,
            articles_dropped_empty: articles_dropped_empty as i32,
            articles_html_cleaned: articles_html_cleaned as i32,
            total_characters: total_characters as i64,
            avg_chars_per_article,
            languages_detected,
        }
    }
}

/// 最終セクション（日本語要約）。
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub(crate) struct RecapFinalSection {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) title_ja: String,
    pub(crate) bullets_ja: Vec<String>,
    pub(crate) model_name: String,
}

#[allow(dead_code)]
impl RecapFinalSection {
    #[must_use]
    pub(crate) fn new(
        job_id: Uuid,
        genre: impl Into<String>,
        title_ja: impl Into<String>,
        bullets_ja: Vec<String>,
        model_name: impl Into<String>,
    ) -> Self {
        Self {
            job_id,
            genre: genre.into(),
            title_ja: title_ja.into(),
            bullets_ja,
            model_name: model_name.into(),
        }
    }
}

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub(crate) struct RecapOutput {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) response_id: String,
    pub(crate) title_ja: String,
    pub(crate) summary_ja: String,
    pub(crate) bullets_ja: Value,
    pub(crate) body_json: Value,
}

#[allow(dead_code)]
impl RecapOutput {
    #[must_use]
    pub(crate) fn new(
        job_id: Uuid,
        genre: impl Into<String>,
        response_id: impl Into<String>,
        title_ja: impl Into<String>,
        summary_ja: impl Into<String>,
        bullets_ja: Value,
        body_json: Value,
    ) -> Self {
        Self {
            job_id,
            genre: genre.into(),
            response_id: response_id.into(),
            title_ja: title_ja.into(),
            summary_ja: summary_ja.into(),
            bullets_ja,
            body_json,
        }
    }
}

/// Coarse候補の学習向けスナップショット。
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub(crate) struct CoarseCandidateRecord {
    pub(crate) genre: String,
    pub(crate) score: f32,
    pub(crate) keyword_support: usize,
    pub(crate) classifier_confidence: f32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) tag_overlap_count: Option<usize>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) graph_boost: Option<f32>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) llm_confidence: Option<f32>,
}

/// Refine判定情報。
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub(crate) struct RefineDecisionRecord {
    pub(crate) final_genre: String,
    pub(crate) confidence: f32,
    pub(crate) strategy: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) llm_trace_id: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) notes: Option<String>,
}

/// Tag Generator要約。
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub(crate) struct TagProfileRecord {
    pub(crate) top_tags: Vec<TagSignalRecord>,
    pub(crate) entropy: f32,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub(crate) struct TagSignalRecord {
    pub(crate) label: String,
    pub(crate) confidence: f32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source_ts: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(from = "(String, String, f32)", into = "(String, String, f32)")]
pub(crate) struct GraphEdgeRecord {
    pub(crate) genre: String,
    pub(crate) tag: String,
    pub(crate) weight: f32,
}

impl From<(String, String, f32)> for GraphEdgeRecord {
    fn from(value: (String, String, f32)) -> Self {
        Self {
            genre: value.0,
            tag: value.1,
            weight: value.2,
        }
    }
}

impl From<GraphEdgeRecord> for (String, String, f32) {
    fn from(value: GraphEdgeRecord) -> Self {
        (value.genre, value.tag, value.weight)
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub(crate) struct FeedbackRecord {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) label: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) notes: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) updated_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Default)]
pub(crate) struct TelemetryRecord {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) refine_duration_ms: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) llm_latency_ms: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) coarse_latency_ms: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) cache_hits: Option<u64>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub(crate) struct LearningTimestamps {
    #[serde(rename = "coarse_started_at")]
    pub(crate) coarse_started: DateTime<Utc>,
    #[serde(
        skip_serializing_if = "Option::is_none",
        rename = "coarse_completed_at"
    )]
    pub(crate) coarse_completed: Option<DateTime<Utc>>,
    #[serde(skip_serializing_if = "Option::is_none", rename = "refine_started_at")]
    pub(crate) refine_started: Option<DateTime<Utc>>,
    #[serde(rename = "refine_completed_at")]
    pub(crate) refine_completed: DateTime<Utc>,
}

impl LearningTimestamps {
    #[must_use]
    pub(crate) fn new(coarse_started: DateTime<Utc>, refine_completed: DateTime<Utc>) -> Self {
        Self {
            coarse_started,
            coarse_completed: Some(coarse_started),
            refine_started: Some(refine_completed),
            refine_completed,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub(crate) struct GenreLearningRecord {
    pub(crate) job_id: Uuid,
    pub(crate) article_id: String,
    pub(crate) coarse_candidates: Vec<CoarseCandidateRecord>,
    pub(crate) refine_decision: RefineDecisionRecord,
    pub(crate) tag_profile: TagProfileRecord,
    #[serde(default, skip_serializing_if = "Vec::is_empty")]
    pub(crate) graph_context: Vec<GraphEdgeRecord>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) feedback: Option<FeedbackRecord>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) telemetry: Option<TelemetryRecord>,
    pub(crate) timestamps: LearningTimestamps,
}

impl GenreLearningRecord {
    #[must_use]
    pub(crate) fn new(
        job_id: Uuid,
        article_id: impl Into<String>,
        coarse_candidates: Vec<CoarseCandidateRecord>,
        refine_decision: RefineDecisionRecord,
        tag_profile: TagProfileRecord,
        timestamps: LearningTimestamps,
    ) -> Self {
        Self {
            job_id,
            article_id: article_id.into(),
            coarse_candidates,
            refine_decision,
            tag_profile,
            graph_context: Vec::new(),
            feedback: None,
            telemetry: None,
            timestamps,
        }
    }

    #[must_use]
    pub(crate) fn with_telemetry(mut self, telemetry: Option<TelemetryRecord>) -> Self {
        self.telemetry = telemetry;
        self
    }
}

/// ジャンル評価実行のメタデータ。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct GenreEvaluationRun {
    pub(crate) run_id: Uuid,
    pub(crate) dataset_path: String,
    pub(crate) total_items: i32,
    pub(crate) macro_precision: f64,
    pub(crate) macro_recall: f64,
    pub(crate) macro_f1: f64,
    pub(crate) summary_tp: i32,
    pub(crate) summary_fp: i32,
    pub(crate) summary_fn: i32,
    pub(crate) micro_precision: Option<f64>,
    pub(crate) micro_recall: Option<f64>,
    pub(crate) micro_f1: Option<f64>,
    pub(crate) weighted_f1: Option<f64>,
    pub(crate) macro_f1_valid: Option<f64>,
    pub(crate) valid_genre_count: Option<i32>,
    pub(crate) undefined_genre_count: Option<i32>,
}

impl GenreEvaluationRun {
    #[must_use]
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn new(
        dataset_path: impl Into<String>,
        total_items: usize,
        macro_precision: f64,
        macro_recall: f64,
        macro_f1: f64,
        summary_tp: usize,
        summary_false_positive: usize,
        summary_false_negative: usize,
    ) -> Self {
        Self {
            run_id: Uuid::new_v4(),
            dataset_path: dataset_path.into(),
            total_items: total_items as i32,
            macro_precision,
            macro_recall,
            macro_f1,
            summary_tp: summary_tp as i32,
            summary_fp: summary_false_positive as i32,
            summary_fn: summary_false_negative as i32,
            micro_precision: None,
            micro_recall: None,
            micro_f1: None,
            weighted_f1: None,
            macro_f1_valid: None,
            valid_genre_count: None,
            undefined_genre_count: None,
        }
    }

    #[must_use]
    #[allow(clippy::too_many_arguments)]
    pub(crate) fn with_extended_metrics(
        mut self,
        micro_precision: f64,
        micro_recall: f64,
        micro_f1: f64,
        weighted_f1: f64,
        macro_f1_valid: f64,
        valid_genre_count: usize,
        undefined_genre_count: usize,
    ) -> Self {
        self.micro_precision = Some(micro_precision);
        self.micro_recall = Some(micro_recall);
        self.micro_f1 = Some(micro_f1);
        self.weighted_f1 = Some(weighted_f1);
        self.macro_f1_valid = Some(macro_f1_valid);
        self.valid_genre_count = Some(valid_genre_count as i32);
        self.undefined_genre_count = Some(undefined_genre_count as i32);
        self
    }
}

/// ジャンルごとの評価メトリクス。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct GenreEvaluationMetric {
    pub(crate) genre: String,
    pub(crate) tp: i32,
    pub(crate) fp: i32,
    pub(crate) fn_count: i32,
    pub(crate) precision: f64,
    pub(crate) recall: f64,
    pub(crate) f1_score: f64,
}

impl GenreEvaluationMetric {
    #[must_use]
    pub(crate) fn new(
        genre: impl Into<String>,
        tp: usize,
        fp: usize,
        fn_count: usize,
        precision: f64,
        recall: f64,
        f1_score: f64,
    ) -> Self {
        Self {
            genre: genre.into(),
            tp: tp as i32,
            fp: fp as i32,
            fn_count: fn_count as i32,
            precision,
            recall,
            f1_score,
        }
    }
}

// ============================================================
// Job Status Dashboard models
// ============================================================

use crate::store::dao::{GenreStatus, JobStatus, TriggerSource};
use std::collections::HashMap;

/// ジョブ進捗イベント - ストリーミングで送信されるペイロード
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JobProgressEvent {
    pub active_job: Option<ActiveJobInfo>,
    pub recent_jobs: Vec<RecentJobSummary>,
    pub stats: JobStats,
    pub user_context: Option<UserJobContext>,
}

/// アクティブ（実行中）ジョブの詳細情報
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ActiveJobInfo {
    pub job_id: Uuid,
    pub status: JobStatus,
    pub current_stage: Option<String>,
    pub stage_index: usize,
    pub stages_completed: Vec<String>,
    pub genre_progress: HashMap<String, GenreProgressInfo>,
    pub total_articles: Option<i32>,
    pub user_article_count: Option<i32>,
    pub kicked_at: DateTime<Utc>,
    pub trigger_source: TriggerSource,
}

/// ジャンルごとの進捗情報
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GenreProgressInfo {
    pub status: GenreStatus,
    pub cluster_count: Option<i32>,
    pub article_count: Option<i32>,
}

/// ステータス遷移レスポンス
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StatusTransitionResponse {
    pub id: i64,
    pub status: JobStatus,
    pub stage: Option<String>,
    pub transitioned_at: DateTime<Utc>,
    pub reason: Option<String>,
    pub actor: String,
}

/// 最近のジョブサマリー
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RecentJobSummary {
    pub job_id: Uuid,
    pub status: JobStatus,
    pub last_stage: Option<String>,
    pub kicked_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub duration_secs: Option<i64>,
    pub trigger_source: TriggerSource,
    pub user_id: Option<Uuid>,
    pub status_history: Vec<StatusTransitionResponse>,
}

/// ジョブ統計情報
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct JobStats {
    pub success_rate_24h: f64,
    pub avg_duration_secs: Option<i64>,
    pub total_jobs_24h: i32,
    pub running_jobs: i32,
    pub failed_jobs_24h: i32,
}

/// ユーザー固有のジョブコンテキスト
#[derive(Debug, Clone, Serialize, Deserialize)]
#[allow(clippy::struct_field_names)]
pub struct UserJobContext {
    pub user_article_count: i32,
    pub user_jobs_count: i32,
    pub user_feed_ids: Vec<Uuid>,
}

/// 拡張されたジョブ情報（user_idとtrigger_source含む）
#[derive(Debug, Clone)]
pub struct ExtendedRecapJob {
    pub job_id: Uuid,
    pub status: JobStatus,
    pub last_stage: Option<String>,
    pub kicked_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub user_id: Option<Uuid>,
    pub trigger_source: TriggerSource,
    #[allow(dead_code)]
    pub note: Option<String>,
}

impl ExtendedRecapJob {
    pub fn duration_secs(&self) -> Option<i64> {
        if self.status == JobStatus::Completed || self.status == JobStatus::Failed {
            Some((self.updated_at - self.kicked_at).num_seconds())
        } else {
            None
        }
    }

    pub fn to_summary(&self) -> RecentJobSummary {
        RecentJobSummary {
            job_id: self.job_id,
            status: self.status.clone(),
            last_stage: self.last_stage.clone(),
            kicked_at: self.kicked_at,
            updated_at: self.updated_at,
            duration_secs: self.duration_secs(),
            trigger_source: self.trigger_source.clone(),
            user_id: self.user_id,
            status_history: Vec::new(), // Populated by the API handler
        }
    }
}
