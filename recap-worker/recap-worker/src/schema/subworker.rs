/// Subworker APIのJSON Schema定義。
///
/// クラスタリング結果のスキーマを定義します。
use std::sync::LazyLock;

use serde_json::{Value, json};

/// Subworker clustering responseのJSON Schema。
pub(crate) static CLUSTERING_RESPONSE_SCHEMA: LazyLock<Value> = LazyLock::new(|| {
    json!({
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://alt.dev/schemas/subworker/cluster-job-response.json",
        "title": "Subworker Run Response",
        "description": "Response schema for recap-subworker run endpoints",
        "type": "object",
        "properties": {
            "run_id": {
                "type": "integer",
                "minimum": 0,
                "description": "Unique identifier for the subworker run"
            },
            "job_id": {
                "type": "string",
                "format": "uuid",
                "description": "Job ID for tracking"
            },
            "genre": {
                "type": "string",
                "description": "Genre of the processed corpus"
            },
            "status": {
                "type": "string",
                "enum": ["running", "succeeded", "partial", "failed"],
                "description": "Run status"
            },
            "cluster_count": {
                "type": "integer",
                "minimum": 0,
                "description": "Total cluster count"
            },
            "clusters": {
                "type": "array",
                "description": "Array of identified clusters",
                "items": { "$ref": "#/$defs/cluster" },
                "minItems": 0
            },
            "diagnostics": {
                "type": "object",
                "description": "Diagnostic metadata captured during processing"
            }
        },
        "required": ["run_id", "job_id", "genre", "status", "cluster_count", "clusters", "diagnostics"],
        "$defs": {
            "cluster": {
                "type": "object",
                "description": "A single cluster of related sentences",
                "properties": {
                    "cluster_id": {
                        "type": "integer",
                        "minimum": -1,
                        "description": "Unique cluster identifier within this job (-1 may be used for noise clusters)"
                    },
                    "size": {
                        "type": "integer",
                        "minimum": 0,
                        "description": "Number of supporting sentences"
                    },
                    "label": {
                        "type": ["null", "string"],
                        "description": "Optional short label for the cluster"
                    },
                    "top_terms": {
                        "type": "array",
                        "description": "Most representative terms for this cluster",
                        "items": { "type": "string" }
                    },
                    "stats": {
                        "type": "object",
                        "description": "Additional cluster statistics"
                    },
                    "representatives": {
                        "type": "array",
                        "description": "Representative sentences for this cluster",
                        "items": { "$ref": "#/$defs/representative" },
                        "minItems": 0
                    }
                },
                "required": ["cluster_id", "size", "label", "top_terms", "stats", "representatives"]
            },
            "representative": {
                "type": "object",
                "description": "Representative sentence payload",
                "properties": {
                    "article_id": {
                        "type": "string",
                        "description": "ID of the source article"
                    },
                    "paragraph_idx": {
                        "type": ["null", "integer"],
                        "minimum": 0,
                        "description": "Paragraph index within the article"
                    },
                    "sentence_text": {
                        "type": "string",
                        "minLength": 20,
                        "description": "Representative sentence text"
                    },
                    "lang": {
                        "type": ["null", "string"],
                        "description": "Language hint for the sentence"
                    },
                    "score": {
                        "type": ["null", "number"],
                        "description": "Heuristic confidence score"
                    }
                },
                "required": ["article_id", "sentence_text"]
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
    fn schema_accepts_valid_response() {
        let response = json!({
            "run_id": 42,
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "status": "succeeded",
            "cluster_count": 1,
            "clusters": [
                {
                    "cluster_id": -1,
                    "size": 3,
                    "label": "ai",
                    "top_terms": ["machine", "learning", "ai"],
                    "stats": {
                        "avg_sim": 0.87
                    },
                    "representatives": [
                        {
                            "article_id": "art-1",
                            "paragraph_idx": 0,
                            "sentence_text": "Machine learning is advancing rapidly and impacting industry.",
                            "lang": "en",
                            "score": 0.95
                        }
                    ],
                    "top_terms": ["machine", "learning", "ai"]
                }
            ],
            "diagnostics": {}
        });

        let result = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response);
        assert!(result.valid, "Errors: {:?}", result.errors);
    }

    #[test]
    fn schema_rejects_missing_required_fields() {
        let response = json!({
            "run_id": 1,
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai"
            // missing status, cluster_count, clusters, diagnostics
        });

        let result = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response);
        assert!(!result.valid);
    }

    #[test]
    fn schema_validates_representatives() {
        let response = json!({
            "run_id": 1,
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "status": "partial",
            "cluster_count": 1,
            "clusters": [
                {
                    "cluster_id": 0,
                    "size": 1,
                    "label": null,
                    "top_terms": [],
                    "stats": {},
                    "representatives": [
                        {
                            "article_id": "art-1",
                            "sentence_text": "Too short."
                        }
                    ]
                }
            ],
            "diagnostics": {}
        });

        let result = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response);
        assert!(!result.valid);
    }
}
