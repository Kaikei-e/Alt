use bytes::Bytes;
use simd_json::OwnedValue;
use simd_json::prelude::{ValueAsObject, ValueAsScalar};
use thiserror::Error;

#[derive(Error, Debug)]
pub enum ParseError {
    #[error("JSON parse error: {0}")]
    JsonError(#[from] simd_json::Error),
    #[error("Missing required field: {0}")]
    MissingField(String),
    #[error("Invalid field type: {0}")]
    InvalidFieldType(String),
    #[error("Invalid log format")]
    InvalidFormat,
}

#[derive(Debug, Clone)]
pub struct DockerLogEntry {
    pub log: String,
    pub stream: String,
    pub time: String,
}

pub struct DockerJsonParser {
    // Can add caching/optimization fields here
}

impl Default for DockerJsonParser {
    fn default() -> Self {
        Self::new()
    }
}

impl DockerJsonParser {
    pub fn new() -> Self {
        Self {}
    }

    pub fn parse(&self, bytes: Bytes) -> Result<DockerLogEntry, ParseError> {
        // Use SIMD-JSON for fast parsing
        let mut data = bytes.to_vec();
        let json: OwnedValue = simd_json::from_slice(&mut data)?;

        let obj = json.as_object().ok_or(ParseError::InvalidFormat)?;

        // Extract required fields
        let log = obj
            .get("log")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ParseError::MissingField("log".to_string()))?
            .to_string();

        let stream = obj
            .get("stream")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ParseError::MissingField("stream".to_string()))?
            .to_string();

        let time = obj
            .get("time")
            .and_then(|v| v.as_str())
            .ok_or_else(|| ParseError::MissingField("time".to_string()))?
            .to_string();

        Ok(DockerLogEntry { log, stream, time })
    }

    pub fn parse_batch(&self, logs: Vec<Bytes>) -> Vec<Result<DockerLogEntry, ParseError>> {
        logs.into_iter().map(|bytes| self.parse(bytes)).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_docker_json_parsing() {
        let parser = DockerJsonParser::new();

        let json_log =
            r#"{"log":"Hello nginx\n","stream":"stdout","time":"2024-01-01T12:00:00.123456789Z"}"#;
        let bytes = Bytes::from(json_log);

        let entry = parser.parse(bytes).unwrap();

        assert_eq!(entry.log, "Hello nginx\n");
        assert_eq!(entry.stream, "stdout");
        assert_eq!(entry.time, "2024-01-01T12:00:00.123456789Z");
    }

    #[test]
    fn test_docker_json_stderr_parsing() {
        let parser = DockerJsonParser::new();

        let json_log = r#"{"log":"ERROR: Something went wrong\n","stream":"stderr","time":"2024-01-01T12:00:01.000000000Z"}"#;
        let bytes = Bytes::from(json_log);

        let entry = parser.parse(bytes).unwrap();

        assert_eq!(entry.stream, "stderr");
        assert!(entry.log.contains("ERROR"));
    }

    #[test]
    fn test_docker_json_multiline_log() {
        let parser = DockerJsonParser::new();

        let json_log = r#"{"log":"Line 1\nLine 2\nLine 3\n","stream":"stdout","time":"2024-01-01T12:00:00.000000000Z"}"#;
        let bytes = Bytes::from(json_log);

        let entry = parser.parse(bytes).unwrap();

        assert!(entry.log.contains("Line 1"));
        assert!(entry.log.contains("Line 2"));
        assert!(entry.log.contains("Line 3"));
    }

    #[test]
    fn test_docker_json_invalid_format() {
        let parser = DockerJsonParser::new();

        let invalid_json = r#"{"log":"Missing fields"}"#;
        let bytes = Bytes::from(invalid_json);

        let result = parser.parse(bytes);
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), ParseError::MissingField(_)));
    }

    #[test]
    fn test_docker_json_malformed() {
        let parser = DockerJsonParser::new();

        let malformed = r#"{"log":"test""#; // Missing closing brace
        let bytes = Bytes::from(malformed);

        let result = parser.parse(bytes);
        assert!(result.is_err());
        assert!(matches!(result.unwrap_err(), ParseError::JsonError(_)));
    }
}
