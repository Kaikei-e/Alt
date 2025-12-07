use serde::{Deserialize, Serialize};
use uuid::Uuid;

/// エラーメッセージの最大長
pub(crate) const MAX_ERROR_MESSAGE_LENGTH: usize = 500;

/// エラーメッセージを要約して切り詰める。
pub(crate) fn truncate_error_message(msg: &str) -> String {
    let char_count = msg.chars().count();
    if char_count <= MAX_ERROR_MESSAGE_LENGTH {
        return msg.to_string();
    }
    let truncated: String = msg.chars().take(MAX_ERROR_MESSAGE_LENGTH).collect();
    format!("{truncated}... (truncated, {char_count} chars)")
}

/// LLMタイブレークに渡す候補（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize)]
pub(crate) struct GenreTieBreakCandidate {
    pub(crate) name: String,
    pub(crate) score: f32,
    pub(crate) keyword_support: usize,
    pub(crate) classifier_confidence: f32,
}

/// LLMタイブレークリクエスト（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize)]
pub(crate) struct GenreTieBreakRequest {
    pub(crate) job_id: Uuid,
    pub(crate) article_id: String,
    pub(crate) language: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) body_preview: Option<String>,
    pub(crate) candidates: Vec<GenreTieBreakCandidate>,
    pub(crate) tags: Vec<TagSignalPayload>,
}

/// LLMに渡すタグ要約（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize)]
pub(crate) struct TagSignalPayload {
    pub(crate) label: String,
    pub(crate) confidence: f32,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source: Option<String>,
}

/// LLMタイブレーク応答（後方互換性のため保持）。
#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct GenreTieBreakResponse {
    pub(crate) genre: String,
    pub(crate) confidence: f32,
    #[serde(default)]
    pub(crate) trace_id: Option<String>,
}

/// 旧バージョンの要約レスポンス。
#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub(crate) struct NewsCreatorSummary {
    pub(crate) response_id: String,
}

/// 日本語要約リクエスト。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct SummaryRequest {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) clusters: Vec<ClusterInput>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) genre_highlights: Option<Vec<RepresentativeSentence>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) options: Option<SummaryOptions>,
}

/// 代表文のメタデータ。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct RepresentativeSentence {
    pub(crate) text: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) published_at: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) source_url: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) article_id: Option<String>,
    #[serde(default)]
    pub(crate) is_centroid: bool,
}

/// クラスター入力データ。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct ClusterInput {
    pub(crate) cluster_id: i32,
    pub(crate) representative_sentences: Vec<RepresentativeSentence>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) top_terms: Option<Vec<String>>,
}

/// 要約生成オプション。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct SummaryOptions {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) max_bullets: Option<usize>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) temperature: Option<f64>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct SummaryResponse {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) summary: Summary,
    pub(crate) metadata: SummaryMetadata,
}

/// 要約内容。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct Summary {
    pub(crate) title: String,
    pub(crate) bullets: Vec<String>,
    pub(crate) language: String,
}

/// 要約メタデータ。
#[allow(dead_code)]
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct SummaryMetadata {
    pub(crate) model: String,
    #[serde(default)]
    temperature: Option<f64>,
    #[serde(default)]
    prompt_tokens: Option<usize>,
    #[serde(default)]
    completion_tokens: Option<usize>,
    #[serde(default)]
    processing_time_ms: Option<usize>,
}
