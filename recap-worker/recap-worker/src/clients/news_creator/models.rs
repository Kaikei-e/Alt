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
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) window_days: Option<u32>,
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

/// 参照情報。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct Reference {
    pub(crate) id: i32,
    pub(crate) url: String,
    pub(crate) domain: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) article_id: Option<String>,
}

/// 要約内容。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct Summary {
    pub(crate) title: String,
    pub(crate) bullets: Vec<String>,
    pub(crate) language: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) references: Option<Vec<Reference>>,
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
    #[serde(default)]
    pub(crate) is_degraded: bool,
    #[serde(default)]
    pub(crate) degradation_reason: Option<String>,
    #[serde(default)]
    pub(crate) reduce_depth: Option<u32>,
    #[serde(default)]
    pub(crate) reduce_info_retention: Option<f64>,
}

// ============================================================================
// Batch Processing Models (to reduce chatty microservices anti-pattern)
// ============================================================================

/// バッチ要約リクエスト。
/// 複数のジャンル要約を一度に処理することで、HTTP往復を削減する。
#[derive(Debug, Clone, Serialize)]
pub(crate) struct BatchSummaryRequest {
    pub(crate) requests: Vec<SummaryRequest>,
}

/// バッチ内の個別エラー。
#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct BatchSummaryError {
    pub(crate) job_id: Uuid,
    pub(crate) genre: String,
    pub(crate) error: String,
}

/// バッチ要約レスポンス。
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct BatchSummaryResponse {
    pub(crate) responses: Vec<SummaryResponse>,
    pub(crate) errors: Vec<BatchSummaryError>,
}

// ============================================================================
// Morning Letter Models
// ============================================================================

/// Morning Letter generation request (sent to news-creator).
#[derive(Debug, Clone, Serialize)]
pub(crate) struct MorningLetterGenerateRequest {
    pub(crate) target_date: String,
    pub(crate) edition_timezone: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub(crate) recap_summaries: Option<Vec<MorningLetterRecapInput>>,
    pub(crate) overnight_groups: Vec<MorningLetterGroupInput>,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct MorningLetterRecapInput {
    pub(crate) genre: String,
    pub(crate) title: String,
    pub(crate) bullets: Vec<String>,
    pub(crate) window_days: u32,
}

#[derive(Debug, Clone, Serialize)]
pub(crate) struct MorningLetterGroupInput {
    pub(crate) group_id: uuid::Uuid,
    pub(crate) articles: Vec<RepresentativeSentence>,
}

/// Morning Letter generation response (from news-creator).
#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct MorningLetterGenerateResponse {
    pub(crate) target_date: String,
    pub(crate) edition_timezone: String,
    pub(crate) content: MorningLetterResponseContent,
    pub(crate) metadata: MorningLetterResponseMetadata,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct MorningLetterResponseContent {
    pub(crate) schema_version: i32,
    pub(crate) lead: String,
    pub(crate) sections: Vec<MorningLetterResponseSection>,
    pub(crate) generated_at: String,
    pub(crate) source_recap_window_days: Option<u32>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct MorningLetterResponseSection {
    pub(crate) key: String,
    pub(crate) title: String,
    pub(crate) bullets: Vec<String>,
    #[serde(default)]
    pub(crate) genre: Option<String>,
    /// Optional prose paragraph written by the LLM. When empty, bullets
    /// remain renderable without degradation.
    #[serde(default)]
    pub(crate) narrative: Option<String>,
}

#[allow(dead_code)]
#[derive(Debug, Clone, Deserialize)]
pub(crate) struct MorningLetterResponseMetadata {
    pub(crate) model: String,
    #[serde(default)]
    pub(crate) is_degraded: bool,
    #[serde(default)]
    pub(crate) degradation_reason: Option<String>,
    #[serde(default)]
    pub(crate) processing_time_ms: Option<u64>,
}

// ============================================================================
// Card Generation Models
// ============================================================================

fn default_prompt_version() -> String {
    "recap_card.v1".to_string()
}

/// カード生成リクエストの入力アイテム。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardItemInput {
    pub(crate) n: usize,
    pub(crate) feed_id: Uuid,
    pub(crate) title: String,
    pub(crate) host: String,
    pub(crate) url: String,
    #[serde(default)]
    pub(crate) pub_date: Option<String>,
    pub(crate) lede: String,
}

/// カード生成リクエスト (POST /v1/cards/generate)。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardGenerateRequest {
    pub(crate) job_id: Uuid,
    pub(crate) candidate_id: Uuid,
    #[serde(default = "default_prompt_version")]
    pub(crate) prompt_version: String,
    pub(crate) items: Vec<CardItemInput>,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub(crate) revision_note: Option<String>,
}

/// カード本文の各文および引用番号。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardSentence {
    pub(crate) text: String,
    pub(crate) refs: Vec<usize>,
}

/// カード本文。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardContent {
    pub(crate) headline_ja: String,
    pub(crate) what_ja: Vec<CardSentence>,
    #[serde(default)]
    pub(crate) why_ja: Option<CardSentence>,
    pub(crate) used_refs: Vec<usize>,
}

/// カード生成メタデータ。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardGenerationMetadata {
    pub(crate) model: String,
    pub(crate) prompt_version: String,
    pub(crate) cache_hit: bool,
    pub(crate) prompt_tokens: usize,
    pub(crate) completion_tokens: usize,
    pub(crate) ms: u64,
    pub(crate) raw_text: String,
}

/// カード生成成功レスポンス (200 OK)。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardGenerateResponse {
    pub(crate) card: CardContent,
    pub(crate) generation: CardGenerationMetadata,
    pub(crate) ja_ratio: f32,
}

/// カード生成失敗レスポンス (422 Unprocessable Entity)。
#[derive(Debug, Clone, Serialize, Deserialize)]
pub(crate) struct CardGenerate422Response {
    pub(crate) reason: String,
    pub(crate) attempts: usize,
    pub(crate) raw_text: String,
    #[serde(default)]
    pub(crate) detail: Option<String>,
}

/// カード生成呼び出し結果。
#[derive(Debug, Clone)]
pub(crate) enum CardGenerateOutcome {
    Success(CardGenerateResponse),
    Rejected(CardGenerate422Response),
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_card_generate_response_deserialization_requires_ja_ratio() {
        let json_with_ja_ratio = serde_json::json!({
            "card": {
                "headline_ja": "見出し",
                "what_ja": [{"text": "内容。[1]", "refs": [1]}],
                "why_ja": null,
                "used_refs": [1]
            },
            "generation": {
                "model": "gemma",
                "prompt_version": "v1",
                "cache_hit": false,
                "prompt_tokens": 10,
                "completion_tokens": 10,
                "ms": 100,
                "raw_text": "raw"
            },
            "ja_ratio": 0.8321
        });

        let resp: CardGenerateResponse =
            serde_json::from_value(json_with_ja_ratio).expect("must deserialize with ja_ratio");
        assert!((resp.ja_ratio - 0.8321).abs() < 1e-4);

        let json_without_ja_ratio = serde_json::json!({
            "card": {
                "headline_ja": "見出し",
                "what_ja": [{"text": "内容。[1]", "refs": [1]}],
                "why_ja": null,
                "used_refs": [1]
            },
            "generation": {
                "model": "gemma",
                "prompt_version": "v1",
                "cache_hit": false,
                "prompt_tokens": 10,
                "completion_tokens": 10,
                "ms": 100,
                "raw_text": "raw"
            }
        });

        let err = serde_json::from_value::<CardGenerateResponse>(json_without_ja_ratio);
        assert!(
            err.is_err(),
            "200 response without ja_ratio must fail deserialization"
        );
    }

    #[test]
    fn test_card_generate_422_response_deserialization_detail() {
        let json_with_detail = serde_json::json!({
            "reason": "parse_failed",
            "attempts": 2,
            "raw_text": "bad output",
            "detail": "sentence_count"
        });
        let resp: CardGenerate422Response =
            serde_json::from_value(json_with_detail).expect("deserialize with detail");
        assert_eq!(resp.detail.as_deref(), Some("sentence_count"));

        let json_null_detail = serde_json::json!({
            "reason": "language",
            "attempts": 1,
            "raw_text": "bad language",
            "detail": null
        });
        let resp_null: CardGenerate422Response =
            serde_json::from_value(json_null_detail).expect("deserialize with null detail");
        assert_eq!(resp_null.detail, None);

        let json_no_detail = serde_json::json!({
            "reason": "parse_failed",
            "attempts": 2,
            "raw_text": "bad output"
        });
        let resp_no: CardGenerate422Response =
            serde_json::from_value(json_no_detail).expect("deserialize without detail");
        assert_eq!(resp_no.detail, None);
    }
}
