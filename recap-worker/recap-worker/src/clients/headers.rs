/// HTTP Idempotency-Keyヘッダーのヘルパー機能。
///
/// Stripeのベストプラクティスに従い、同一リクエストの再試行を安全に処理します。
use reqwest::header::{HeaderMap, HeaderName, HeaderValue};
use uuid::Uuid;

/// Idempotency-Keyヘッダーの名前。
pub(crate) const IDEMPOTENCY_KEY_HEADER: &str = "Idempotency-Key";

/// Idempotency-Keyヘッダーを含むヘッダーマップを構築する。
///
/// # Arguments
/// * `job_id` - ジョブID
/// * `request_specific_id` - リクエスト固有の識別子（ジャンルなど）
///
/// # Returns
/// Idempotency-Keyヘッダーを含むHeaderMap
pub(crate) fn build_idempotent_headers(job_id: Uuid, request_specific_id: &str) -> HeaderMap {
    let mut headers = HeaderMap::new();
    let key = format!("{}:{}", job_id, request_specific_id);

    if let Ok(value) = HeaderValue::from_str(&key) {
        if let Ok(name) = HeaderName::from_bytes(IDEMPOTENCY_KEY_HEADER.as_bytes()) {
            headers.insert(name, value);
        }
    }

    headers
}

/// Idempotency-KeyをHeaderMapに追加する。
///
/// 既存のHeaderMapに対してIdempotency-Keyを追加する場合に使用します。
pub(crate) fn add_idempotency_key(
    headers: &mut HeaderMap,
    job_id: Uuid,
    request_specific_id: &str,
) {
    let key = format!("{}:{}", job_id, request_specific_id);

    if let Ok(value) = HeaderValue::from_str(&key) {
        if let Ok(name) = HeaderName::from_bytes(IDEMPOTENCY_KEY_HEADER.as_bytes()) {
            headers.insert(name, value);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn build_idempotent_headers_creates_correct_key() {
        let job_id = Uuid::parse_str("550e8400-e29b-41d4-a716-446655440000").unwrap();
        let headers = build_idempotent_headers(job_id, "ai");

        let header_name = HeaderName::from_bytes(IDEMPOTENCY_KEY_HEADER.as_bytes()).unwrap();
        let value = headers.get(&header_name).expect("header should exist");

        assert_eq!(
            value.to_str().unwrap(),
            "550e8400-e29b-41d4-a716-446655440000:ai"
        );
    }

    #[test]
    fn add_idempotency_key_adds_to_existing_headers() {
        let job_id = Uuid::parse_str("550e8400-e29b-41d4-a716-446655440000").unwrap();
        let mut headers = HeaderMap::new();
        headers.insert("X-Custom", HeaderValue::from_static("value"));

        add_idempotency_key(&mut headers, job_id, "tech");

        assert_eq!(headers.len(), 2);
        let header_name = HeaderName::from_bytes(IDEMPOTENCY_KEY_HEADER.as_bytes()).unwrap();
        let value = headers.get(&header_name).expect("header should exist");
        assert_eq!(
            value.to_str().unwrap(),
            "550e8400-e29b-41d4-a716-446655440000:tech"
        );
    }
}
