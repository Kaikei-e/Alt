/// News-Creator APIのJSON Schema定義。
///
/// 日本語要約生成のレスポンススキーマを定義します。
use once_cell::sync::Lazy;
use serde_json::{Value, json};

/// News-Creator summary responseのJSON Schema。
pub(crate) static SUMMARY_RESPONSE_SCHEMA: Lazy<Value> = Lazy::new(|| {
    json!({
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://alt.dev/schemas/news-creator/summary-response.json",
        "title": "News-Creator Summary Response",
        "description": "Response schema for news-creator summary generation API",
        "type": "object",
        "properties": {
            "job_id": {
                "type": "string",
                "format": "uuid",
                "description": "Job ID for tracking"
            },
            "genre": {
                "type": "string",
                "description": "Genre of the summary"
            },
            "summary": {
                "type": "object",
                "description": "Generated summary content",
                "properties": {
                    "title": {
                        "type": "string",
                        "minLength": 1,
                        "maxLength": 200,
                        "description": "Summary title in Japanese"
                    },
                    "bullets": {
                        "type": "array",
                        "description": "Bullet-point summary in Japanese",
                        "items": {
                            "type": "string",
                            "minLength": 1,
                            "maxLength": 500
                        },
                        "minItems": 1,
                        "maxItems": 10
                    },
                    "language": {
                        "type": "string",
                        "pattern": "^ja$",
                        "description": "Language code (must be 'ja' for Japanese)"
                    }
                },
                "required": ["title", "bullets", "language"]
            },
            "metadata": {
                "type": "object",
                "description": "Generation metadata",
                "properties": {
                    "model": {
                        "type": "string",
                        "description": "LLM model used for generation"
                    },
                    "temperature": {
                        "type": "number",
                        "minimum": 0,
                        "maximum": 2,
                        "description": "Temperature parameter"
                    },
                    "prompt_tokens": {
                        "type": "integer",
                        "minimum": 0,
                        "description": "Number of tokens in the prompt"
                    },
                    "completion_tokens": {
                        "type": "integer",
                        "minimum": 0,
                        "description": "Number of tokens in the completion"
                    },
                    "processing_time_ms": {
                        "type": "integer",
                        "minimum": 0,
                        "description": "Processing time in milliseconds"
                    }
                },
                "required": ["model"]
            }
        },
        "required": ["job_id", "genre", "summary", "metadata"]
    })
});

/// News-Creator summary requestのJSON Schema。
#[allow(dead_code)]
pub(crate) static SUMMARY_REQUEST_SCHEMA: Lazy<Value> = Lazy::new(|| {
    json!({
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://alt.dev/schemas/news-creator/summary-request.json",
        "title": "News-Creator Summary Request",
        "description": "Request schema for news-creator summary generation API",
        "type": "object",
        "properties": {
            "job_id": {
                "type": "string",
                "format": "uuid",
                "description": "Job ID for tracking"
            },
            "genre": {
                "type": "string",
                "description": "Genre of the content to summarize"
            },
            "clusters": {
                "type": "array",
                "description": "Clusters to summarize",
                "items": {
                    "$ref": "#/$defs/cluster_input"
                },
                "minItems": 1,
                "maxItems": 20
            },
            "options": {
                "type": "object",
                "description": "Generation options",
                "properties": {
                    "max_bullets": {
                        "type": "integer",
                        "minimum": 1,
                        "maximum": 10,
                        "default": 5
                    },
                    "temperature": {
                        "type": "number",
                        "minimum": 0,
                        "maximum": 2,
                        "default": 0.7
                    }
                }
            }
        },
        "required": ["job_id", "genre", "clusters"],
        "$defs": {
            "cluster_input": {
                "type": "object",
                "description": "A cluster to be summarized",
                "properties": {
                    "cluster_id": {
                        "type": "integer",
                        "minimum": 0
                    },
                    "representative_sentences": {
                        "type": "array",
                        "description": "Most important sentences from the cluster",
                        "items": {
                            "type": "string",
                            "minLength": 1
                        },
                        "minItems": 1,
                        "maxItems": 10
                    },
                    "top_terms": {
                        "type": "array",
                        "description": "Key terms for this cluster",
                        "items": {
                            "type": "string"
                        }
                    }
                },
                "required": ["cluster_id", "representative_sentences"]
            }
        }
    })
});

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::validate_json;
    use serde_json::json;

    #[test]
    fn schema_accepts_valid_summary_response() {
        let response = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "summary": {
                "title": "AIと機械学習の最新動向",
                "bullets": [
                    "機械学習技術が急速に進歩しています",
                    "自然言語処理の新しい手法が開発されました",
                    "画像認識の精度が向上しています"
                ],
                "language": "ja"
            },
            "metadata": {
                "model": "gemma-3:4b",
                "temperature": 0.7,
                "prompt_tokens": 500,
                "completion_tokens": 200,
                "processing_time_ms": 2500
            }
        });

        let result = validate_json(&SUMMARY_RESPONSE_SCHEMA, &response);
        assert!(result.valid, "Errors: {:?}", result.errors);
    }

    #[test]
    fn schema_rejects_non_japanese_language() {
        let response = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "summary": {
                "title": "Title",
                "bullets": ["Bullet 1"],
                "language": "en" // Should be "ja"
            },
            "metadata": {
                "model": "gemma-3:4b"
            }
        });

        let result = validate_json(&SUMMARY_RESPONSE_SCHEMA, &response);
        assert!(!result.valid);
    }

    #[test]
    fn schema_validates_bullet_count() {
        let response = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "summary": {
                "title": "タイトル",
                "bullets": [], // Empty array should be invalid (minItems: 1)
                "language": "ja"
            },
            "metadata": {
                "model": "gemma-3:4b"
            }
        });

        let result = validate_json(&SUMMARY_RESPONSE_SCHEMA, &response);
        assert!(!result.valid);
    }

    #[test]
    fn schema_accepts_valid_summary_request() {
        let request = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "clusters": [
                {
                    "cluster_id": 0,
                    "representative_sentences": [
                        "Machine learning is advancing.",
                        "AI technologies are improving."
                    ],
                    "top_terms": ["ai", "machine", "learning"]
                }
            ],
            "options": {
                "max_bullets": 5,
                "temperature": 0.7
            }
        });

        let result = validate_json(&SUMMARY_REQUEST_SCHEMA, &request);
        assert!(result.valid, "Errors: {:?}", result.errors);
    }

    #[test]
    fn schema_validates_cluster_limits() {
        let mut clusters = Vec::new();
        for i in 0..25 {
            clusters.push(json!({
                "cluster_id": i,
                "representative_sentences": ["Sentence."]
            }));
        }

        let request = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "clusters": clusters // 25 clusters, but maxItems is 20
        });

        let result = validate_json(&SUMMARY_REQUEST_SCHEMA, &request);
        assert!(!result.valid);
    }
}
