pub(crate) mod news_creator;
/// JSON Schema 2020-12定義モジュール。
///
/// SubworkerとNews-Creatorとの契約をJSON Schemaで定義し、
/// 実行時に検証を行います。
pub(crate) mod subworker;

use serde_json::Value;

/// スキーマ検証結果。
#[derive(Debug)]
pub(crate) struct ValidationResult {
    pub(crate) valid: bool,
    pub(crate) errors: Vec<String>,
}

impl ValidationResult {
    pub(crate) fn valid() -> Self {
        Self {
            valid: true,
            errors: Vec::new(),
        }
    }

    pub(crate) fn invalid(errors: Vec<String>) -> Self {
        Self {
            valid: false,
            errors,
        }
    }
}

/// JSON Schemaでデータを検証する。
///
/// # Arguments
/// * `schema_json` - JSON Schema定義（JSON形式）
/// * `instance` - 検証対象のデータ（JSON形式）
///
/// # Returns
/// 検証結果
pub(crate) fn validate_json(schema_json: &Value, instance: &Value) -> ValidationResult {
    match jsonschema::validator_for(schema_json) {
        Ok(schema) => {
            if schema.is_valid(instance) {
                ValidationResult::valid()
            } else {
                // 簡易実装: 詳細なエラーメッセージは省略
                ValidationResult::invalid(vec!["Validation failed".to_string()])
            }
        }
        Err(e) => ValidationResult::invalid(vec![format!("Schema compilation error: {}", e)]),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    #[test]
    fn validate_json_accepts_valid_data() {
        let schema = json!({
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "type": "object",
            "properties": {
                "name": { "type": "string" },
                "age": { "type": "integer" }
            },
            "required": ["name"]
        });

        let instance = json!({
            "name": "Alice",
            "age": 30
        });

        let result = validate_json(&schema, &instance);
        assert!(result.valid);
        assert!(result.errors.is_empty());
    }

    #[test]
    fn validate_json_rejects_invalid_data() {
        let schema = json!({
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "type": "object",
            "properties": {
                "name": { "type": "string" }
            },
            "required": ["name"]
        });

        let instance = json!({
            "age": 30
        });

        let result = validate_json(&schema, &instance);
        assert!(!result.valid);
        assert!(!result.errors.is_empty());
    }

    #[test]
    fn validate_json_checks_types() {
        let schema = json!({
            "$schema": "https://json-schema.org/draft/2020-12/schema",
            "type": "object",
            "properties": {
                "count": { "type": "integer" }
            }
        });

        let instance = json!({
            "count": "not a number"
        });

        let result = validate_json(&schema, &instance);
        assert!(!result.valid);
    }
}
