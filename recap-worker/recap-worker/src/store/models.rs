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
