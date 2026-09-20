//! Bearer-token guard for recap-worker's own admin/mutating routes.
//!
//! Protects `/admin/jobs/retry` and `/admin/genre-learning` behind
//! `RECAP_ADMIN_TOKEN_FILE` / `RECAP_ADMIN_AUTH=disabled` (see the
//! `AdminAuth` config in `config.rs`). The guard token is carried as an
//! `Extension`, not `AppState`, so this middleware — and its tests — never
//! need the full `ComponentRegistry` (DB pool, mTLS, pki, LLM clients).
//!
//! `/v1/generate/recaps/{7,3}days` and `/v1/morning/letters/regenerate`
//! stay unauthenticated in this slice: their callers (alt-frontend-sv,
//! alt-backend) are outside this change's scope, and protecting the route
//! without updating the caller would 401 real production traffic instead
//! of closing a hole.

use std::sync::Arc;

use axum::{
    body::Body,
    extract::Extension,
    http::{
        HeaderValue, Request, StatusCode,
        header::{AUTHORIZATION, WWW_AUTHENTICATE},
    },
    middleware::Next,
    response::{IntoResponse, Response},
};

/// Expected bearer token for this recap-worker instance's protected
/// routes. `None` exactly when `RECAP_ADMIN_AUTH=disabled` was set
/// explicitly.
#[derive(Clone)]
pub(crate) struct AdminAuthGuard(Option<Arc<str>>);

impl AdminAuthGuard {
    pub(crate) fn new(token: Option<&str>) -> Self {
        Self(token.map(Arc::from))
    }
}

/// Rejects requests to the protected sub-router unless they carry
/// `Authorization: Bearer <token>` matching the configured admin token.
/// Missing/malformed header and a present-but-wrong token both -> 401 with
/// `WWW-Authenticate: Bearer` and the same body, so a caller can't use the
/// response to tell "no token sent" apart from "wrong token sent".
/// `RECAP_ADMIN_AUTH=disabled` -> pass-through unauthenticated.
pub(crate) async fn require_admin_token(
    Extension(guard): Extension<AdminAuthGuard>,
    request: Request<Body>,
    next: Next,
) -> Response {
    let Some(expected) = guard.0.as_deref() else {
        return next.run(request).await;
    };

    let presented = request
        .headers()
        .get(AUTHORIZATION)
        .and_then(|value| value.to_str().ok())
        .and_then(|value| value.strip_prefix("Bearer "));

    let authorized =
        presented.is_some_and(|token| constant_time_eq(token.as_bytes(), expected.as_bytes()));
    if authorized {
        return next.run(request).await;
    }
    unauthorized_response()
}

fn unauthorized_response() -> Response {
    let mut response = (StatusCode::UNAUTHORIZED, "unauthorized").into_response();
    response
        .headers_mut()
        .insert(WWW_AUTHENTICATE, HeaderValue::from_static("Bearer"));
    response
}

/// Constant-time byte comparison so a wrong-token response doesn't leak the
/// expected token's content via response-timing. Neither `subtle` nor
/// `constant_time_eq` is already a dependency of this crate, so this is a
/// small manual loop rather than pulling in a new crate for one comparison.
fn constant_time_eq(a: &[u8], b: &[u8]) -> bool {
    if a.len() != b.len() {
        return false;
    }
    a.iter()
        .zip(b.iter())
        .fold(0u8, |acc, (x, y)| acc | (x ^ y))
        == 0
}

#[cfg(test)]
mod tests {
    use super::*;
    use axum::{Router, http::Method, middleware, routing::get};
    use tower::ServiceExt;

    fn protected_app(token: Option<&str>) -> Router {
        Router::new()
            .route("/protected", get(|| async { StatusCode::OK }))
            .route_layer(middleware::from_fn(require_admin_token))
            .layer(Extension(AdminAuthGuard::new(token)))
    }

    fn request(method: Method, auth_header: Option<&str>) -> Request<Body> {
        let mut builder = Request::builder().method(method).uri("/protected");
        if let Some(value) = auth_header {
            builder = builder.header(AUTHORIZATION, value);
        }
        builder.body(Body::empty()).expect("request builds")
    }

    #[tokio::test]
    async fn missing_authorization_header_is_unauthorized() {
        let app = protected_app(Some("expected-token-0123456789"));
        let response = app
            .oneshot(request(Method::GET, None))
            .await
            .expect("request succeeds");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
        assert_eq!(
            response
                .headers()
                .get(WWW_AUTHENTICATE)
                .and_then(|v| v.to_str().ok()),
            Some("Bearer")
        );
    }

    #[tokio::test]
    async fn wrong_token_is_unauthorized() {
        let app = protected_app(Some("expected-token-0123456789"));
        let response = app
            .oneshot(request(Method::GET, Some("Bearer wrong-token-0123456789")))
            .await
            .expect("request succeeds");
        assert_eq!(response.status(), StatusCode::UNAUTHORIZED);
        assert_eq!(
            response
                .headers()
                .get(WWW_AUTHENTICATE)
                .and_then(|v| v.to_str().ok()),
            Some("Bearer")
        );
    }

    #[tokio::test]
    async fn missing_and_wrong_token_bodies_are_indistinguishable() {
        // Body/detail must be stable across failure modes so a caller can't
        // use the response to tell "no token sent" apart from "wrong token
        // sent".
        let missing = protected_app(Some("expected-token-0123456789"))
            .oneshot(request(Method::GET, None))
            .await
            .expect("request succeeds");
        let wrong = protected_app(Some("expected-token-0123456789"))
            .oneshot(request(Method::GET, Some("Bearer wrong-token-0123456789")))
            .await
            .expect("request succeeds");

        let missing_body = axum::body::to_bytes(missing.into_body(), usize::MAX)
            .await
            .expect("missing body bytes");
        let wrong_body = axum::body::to_bytes(wrong.into_body(), usize::MAX)
            .await
            .expect("wrong body bytes");
        assert_eq!(missing_body, wrong_body);
    }

    #[tokio::test]
    async fn correct_token_passes_through() {
        let app = protected_app(Some("expected-token-0123456789"));
        let response = app
            .oneshot(request(
                Method::GET,
                Some("Bearer expected-token-0123456789"),
            ))
            .await
            .expect("request succeeds");
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[tokio::test]
    async fn disabled_auth_passes_through_without_header() {
        let app = protected_app(None);
        let response = app
            .oneshot(request(Method::GET, None))
            .await
            .expect("request succeeds");
        assert_eq!(response.status(), StatusCode::OK);
    }

    #[test]
    fn constant_time_eq_matches_equal_bytes() {
        assert!(constant_time_eq(b"abc123", b"abc123"));
    }

    #[test]
    fn constant_time_eq_rejects_different_bytes() {
        assert!(!constant_time_eq(b"abc123", b"abc124"));
    }

    #[test]
    fn constant_time_eq_rejects_different_lengths() {
        assert!(!constant_time_eq(b"short", b"much-longer-token"));
    }
}
