//! D-02 Ingest authentication: file-backed Bearer token validation.
//!
//! All `/v1/aggregate`, `/v1/logs`, `/v1/traces` endpoints require a valid
//! `Authorization: Bearer <token>` header. Health and metrics endpoints are
//! intentionally exempt. Token comparison is constant-time to prevent
//! timing side-channels (CWE-208).

use std::fs;
use std::sync::Arc;

use axum::{
    extract::Request,
    http::{HeaderMap, StatusCode},
    middleware::Next,
    response::Response,
};
use subtle::ConstantTimeEq;
use tracing::warn;

use crate::error::AggregatorError;

/// Validates that a token adheres to RFC 6750 Section 2.1 token68 syntax:
/// `1*( ALPHA / DIGIT / "-" / "." / "_" / "~" / "+" / "/" ) *"="`
pub fn is_rfc6750_token68(token: &str) -> bool {
    if token.is_empty() {
        return false;
    }
    let mut non_padding_count = 0usize;
    let mut seen_equal = false;
    for b in token.bytes() {
        if b == b'=' {
            seen_equal = true;
        } else if seen_equal {
            return false;
        } else if b.is_ascii_alphanumeric()
            || b == b'-'
            || b == b'.'
            || b == b'_'
            || b == b'~'
            || b == b'+'
            || b == b'/'
        {
            non_padding_count += 1;
        } else {
            return false;
        }
    }
    non_padding_count >= 1
}

/// Load and validate the ingest token from the given file path.
///
/// Rejects tokens containing CR, LF, non-ASCII bytes, or that are empty
/// after trimming. Error messages never expose the token value.
pub fn load_ingest_token(path: &str) -> Result<String, AggregatorError> {
    let raw = fs::read_to_string(path).map_err(|e| {
        AggregatorError::Config(format!(
            "RASK_INGEST_TOKEN_FILE ({path}) could not be read: {e}"
        ))
    })?;

    let token = raw.trim().to_string();

    if token.is_empty() {
        return Err(AggregatorError::Config(format!(
            "RASK_INGEST_TOKEN_FILE ({path}) is empty"
        )));
    }

    // RFC 6750 token68 alphabet safety: printable ASCII, no control chars.
    if !token.is_ascii() {
        return Err(AggregatorError::Config(format!(
            "RASK_INGEST_TOKEN_FILE ({path}) contains non-ASCII characters"
        )));
    }
    if token.contains('\r') || token.contains('\n') {
        return Err(AggregatorError::Config(format!(
            "RASK_INGEST_TOKEN_FILE ({path}) contains embedded CR/LF characters"
        )));
    }
    // Reject ASCII control characters (0x00–0x1F, 0x7F).
    if token.bytes().any(|b| b < 0x20 || b == 0x7F) {
        return Err(AggregatorError::Config(format!(
            "RASK_INGEST_TOKEN_FILE ({path}) contains ASCII control characters"
        )));
    }
    if !is_rfc6750_token68(&token) {
        return Err(AggregatorError::Config(format!(
            "RASK_INGEST_TOKEN_FILE ({path}) does not conform to RFC 6750 token68 syntax"
        )));
    }

    Ok(token)
}

/// Extract the Bearer token from the Authorization header value.
///
/// Returns `None` if the header is missing the "Bearer " prefix.
fn extract_bearer(headers: &HeaderMap) -> Option<&str> {
    headers
        .get("authorization")?
        .to_str()
        .ok()?
        .strip_prefix("Bearer ")
}

/// Constant-time comparison of two token strings.
///
/// Prevents timing side-channels (CWE-208) by always comparing all bytes.
#[inline]
fn constant_time_token_eq(a: &str, b: &str) -> bool {
    // `ct_eq` on slices of different lengths returns false in constant time
    // relative to the shorter operand. For equal-length tokens (the common
    // case) it is fully constant-time.
    a.as_bytes().ct_eq(b.as_bytes()).into()
}

/// Axum middleware that enforces `Authorization: Bearer <token>` on every
/// request that reaches it. Wire this onto ingest routes only; health and
/// metrics routes must be mounted outside this layer.
pub async fn require_ingest_token(request: Request, next: Next) -> Result<Response, StatusCode> {
    let expected = request
        .extensions()
        .get::<Arc<IngestToken>>()
        .expect("IngestToken extension must be inserted by the router");

    match extract_bearer(request.headers()) {
        Some(provided) if constant_time_token_eq(provided, &expected.0) => {
            Ok(next.run(request).await)
        }
        Some(_) => {
            // Wrong token — log without exposing either value.
            warn!("ingest auth: invalid bearer token rejected");
            Err(StatusCode::UNAUTHORIZED)
        }
        None => {
            warn!("ingest auth: missing Authorization header");
            Err(StatusCode::UNAUTHORIZED)
        }
    }
}

/// New-type wrapper so the token can be inserted as an Axum extension.
#[derive(Clone)]
pub struct IngestToken(pub String);

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn load_ingest_token_rejects_empty_file() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("empty_token");
        fs::write(&path, "   \n").unwrap();

        let err = load_ingest_token(path.to_str().unwrap()).unwrap_err();
        assert!(
            err.to_string().contains("empty"),
            "expected 'empty' in error, got: {err}"
        );
        // Must not expose token content.
        assert!(!err.to_string().contains("   \n"));
    }

    #[test]
    fn load_ingest_token_rejects_embedded_cr() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("cr_token");
        fs::write(&path, "tok\ren").unwrap();

        let err = load_ingest_token(path.to_str().unwrap()).unwrap_err();
        assert!(err.to_string().contains("CR/LF"));
    }

    #[test]
    fn load_ingest_token_rejects_embedded_lf() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("lf_token");
        fs::write(&path, "tok\nen").unwrap();

        let err = load_ingest_token(path.to_str().unwrap()).unwrap_err();
        assert!(err.to_string().contains("CR/LF"));
    }

    #[test]
    fn load_ingest_token_rejects_non_ascii() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("utf8_token");
        fs::write(&path, "tök€n").unwrap();

        let err = load_ingest_token(path.to_str().unwrap()).unwrap_err();
        assert!(err.to_string().contains("non-ASCII"));
    }

    #[test]
    fn load_ingest_token_rejects_control_chars() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("ctrl_token");
        fs::write(&path, "tok\x01en").unwrap();

        let err = load_ingest_token(path.to_str().unwrap()).unwrap_err();
        assert!(err.to_string().contains("control characters"));
    }

    #[test]
    fn load_ingest_token_rejects_missing_file() {
        let err = load_ingest_token("/nonexistent/path/rask_test_token").unwrap_err();
        assert!(err.to_string().contains("could not be read"));
    }

    #[test]
    fn load_ingest_token_trims_and_accepts_valid_ascii() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("valid_token");
        fs::write(&path, "  my-s3cure-tok3n_v1  \n").unwrap();

        let token = load_ingest_token(path.to_str().unwrap()).unwrap();
        assert_eq!(token, "my-s3cure-tok3n_v1");
    }

    #[test]
    fn load_ingest_token_rejects_invalid_token68_chars() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("bad_token68");
        fs::write(&path, "bad$token!").unwrap();

        let err = load_ingest_token(path.to_str().unwrap()).unwrap_err();
        assert!(err.to_string().contains("token68"));
        // Must never leak token content in error message
        assert!(!err.to_string().contains("bad$token!"));
    }

    #[test]
    fn constant_time_eq_matches_identical_tokens() {
        assert!(constant_time_token_eq("abc123", "abc123"));
    }

    #[test]
    fn constant_time_eq_rejects_different_tokens() {
        assert!(!constant_time_token_eq("abc123", "abc124"));
    }

    #[test]
    fn constant_time_eq_rejects_different_lengths() {
        assert!(!constant_time_token_eq("abc", "abcd"));
    }

    #[test]
    fn extract_bearer_parses_valid_header() {
        let mut headers = HeaderMap::new();
        headers.insert("authorization", "Bearer my-token".parse().unwrap());
        assert_eq!(extract_bearer(&headers), Some("my-token"));
    }

    #[test]
    fn extract_bearer_returns_none_for_missing_header() {
        let headers = HeaderMap::new();
        assert_eq!(extract_bearer(&headers), None);
    }

    #[test]
    fn extract_bearer_returns_none_for_wrong_scheme() {
        let mut headers = HeaderMap::new();
        headers.insert("authorization", "Basic dXNlcjpwYXNz".parse().unwrap());
        assert_eq!(extract_bearer(&headers), None);
    }

    #[test]
    fn extract_bearer_is_case_sensitive_on_scheme() {
        let mut headers = HeaderMap::new();
        // "bearer " (lowercase) should NOT match "Bearer " (spec-compliant)
        headers.insert("authorization", "bearer my-token".parse().unwrap());
        assert_eq!(extract_bearer(&headers), None);
    }

    #[test]
    fn test_token68_padding_only_rejected() {
        assert!(!is_rfc6750_token68("="));
        assert!(!is_rfc6750_token68("=="));
        assert!(!is_rfc6750_token68("==="));
        assert!(!is_rfc6750_token68("===="));
    }

    #[test]
    fn test_token68_malformed_rejected() {
        assert!(!is_rfc6750_token68(""));
        assert!(!is_rfc6750_token68("=abc"));
        assert!(!is_rfc6750_token68("abc=def"));
        assert!(!is_rfc6750_token68("abc==d"));
        assert!(!is_rfc6750_token68("abc def"));
        assert!(!is_rfc6750_token68("abc\ndef"));
        assert!(!is_rfc6750_token68("abc\rdef"));
        assert!(!is_rfc6750_token68("abc$token"));
    }

    #[test]
    fn test_token68_non_ascii_rejected() {
        assert!(!is_rfc6750_token68("token\u{1f600}"));
        assert!(!is_rfc6750_token68("tokén"));
        assert!(!is_rfc6750_token68("トークン"));
    }

    #[test]
    fn test_token68_valid_accepted() {
        assert!(is_rfc6750_token68("a"));
        assert!(is_rfc6750_token68("valid_token"));
        assert!(is_rfc6750_token68("valid-token.123~_"));
        assert!(is_rfc6750_token68("valid+token/abc="));
        assert!(is_rfc6750_token68("valid+token/abc=="));
    }
}
