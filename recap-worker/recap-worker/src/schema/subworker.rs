/// Subworker APIのJSON Schema定義。
///
/// クラスタリング結果のスキーマを定義します。
use once_cell::sync::Lazy;
use serde_json::{json, Value};

/// Subworker clustering responseのJSON Schema。
pub(crate) static CLUSTERING_RESPONSE_SCHEMA: Lazy<Value> = Lazy::new(|| {
    json!({
        "$schema": "https://json-schema.org/draft/2020-12/schema",
        "$id": "https://alt.dev/schemas/subworker/clustering-response.json",
        "title": "Subworker Clustering Response",
        "description": "Response schema for subworker clustering API",
        "type": "object",
        "properties": {
            "job_id": {
                "type": "string",
                "format": "uuid",
                "description": "Job ID for tracking"
            },
            "genre": {
                "type": "string",
                "description": "Genre of the processed corpus"
            },
            "clusters": {
                "type": "array",
                "description": "Array of identified clusters",
                "items": {
                    "$ref": "#/$defs/cluster"
                },
                "minItems": 0
            },
            "metadata": {
                "type": "object",
                "description": "Processing metadata",
                "properties": {
                    "total_sentences": {
                        "type": "integer",
                        "minimum": 0
                    },
                    "cluster_count": {
                        "type": "integer",
                        "minimum": 0
                    },
                    "processing_time_ms": {
                        "type": "integer",
                        "minimum": 0
                    }
                },
                "required": ["total_sentences", "cluster_count"]
            }
        },
        "required": ["job_id", "genre", "clusters", "metadata"],
        "$defs": {
            "cluster": {
                "type": "object",
                "description": "A single cluster of related sentences",
                "properties": {
                    "cluster_id": {
                        "type": "integer",
                        "minimum": 0,
                        "description": "Unique cluster identifier within this job"
                    },
                    "sentences": {
                        "type": "array",
                        "description": "Sentences in this cluster",
                        "items": {
                            "$ref": "#/$defs/sentence"
                        },
                        "minItems": 1
                    },
                    "centroid": {
                        "type": "array",
                        "description": "Cluster centroid vector (optional)",
                        "items": {
                            "type": "number"
                        }
                    },
                    "top_terms": {
                        "type": "array",
                        "description": "Most representative terms for this cluster",
                        "items": {
                            "type": "string"
                        }
                    },
                    "coherence_score": {
                        "type": "number",
                        "minimum": 0,
                        "maximum": 1,
                        "description": "Cluster coherence score (0-1)"
                    }
                },
                "required": ["cluster_id", "sentences", "top_terms"]
            },
            "sentence": {
                "type": "object",
                "description": "A sentence with its source article",
                "properties": {
                    "sentence_id": {
                        "type": "integer",
                        "minimum": 0,
                        "description": "Sentence ID within source article"
                    },
                    "text": {
                        "type": "string",
                        "minLength": 1,
                        "description": "Sentence text"
                    },
                    "source_article_id": {
                        "type": "string",
                        "description": "ID of the source article"
                    },
                    "embedding": {
                        "type": "array",
                        "description": "Sentence embedding vector (optional)",
                        "items": {
                            "type": "number"
                        }
                    }
                },
                "required": ["sentence_id", "text", "source_article_id"]
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
    fn schema_accepts_valid_clustering_response() {
        let response = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "clusters": [
                {
                    "cluster_id": 0,
                    "sentences": [
                        {
                            "sentence_id": 0,
                            "text": "Machine learning is advancing rapidly.",
                            "source_article_id": "art-1"
                        }
                    ],
                    "top_terms": ["machine", "learning", "ai"]
                }
            ],
            "metadata": {
                "total_sentences": 100,
                "cluster_count": 5,
                "processing_time_ms": 1500
            }
        });

        let result = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response);
        assert!(result.valid, "Errors: {:?}", result.errors);
    }

    #[test]
    fn schema_rejects_missing_required_fields() {
        let response = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai"
            // missing clusters and metadata
        });

        let result = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response);
        assert!(!result.valid);
    }

    #[test]
    fn schema_validates_cluster_structure() {
        let response = json!({
            "job_id": "550e8400-e29b-41d4-a716-446655440000",
            "genre": "ai",
            "clusters": [
                {
                    "cluster_id": 0,
                    "sentences": [], // empty sentences array should be invalid (minItems: 1)
                    "top_terms": ["term"]
                }
            ],
            "metadata": {
                "total_sentences": 0,
                "cluster_count": 0
            }
        });

        let result = validate_json(&CLUSTERING_RESPONSE_SCHEMA, &response);
        assert!(!result.valid);
    }
}
