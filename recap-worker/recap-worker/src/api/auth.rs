//! Authentication guards for recap-worker's HTTP surface.
//!
//! Peer identity (`PeerIdentityGuard` / `require_peer_identity`) gates every
//! non-health/metrics route using the verified client certificate attached by
//! the mTLS acceptor (`MTLS_ALLOWED_PEERS`), while `AdminAuthGuard` adds the
//! shared bearer check on `/admin/*`.

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
use rustls::pki_types::CertificateDer;

#[derive(Clone)]
pub(crate) struct PeerIdentityGuard(Option<Arc<[String]>>);

impl PeerIdentityGuard {
    pub(crate) fn new(allowed_peers: Option<&[String]>) -> Self {
        Self(allowed_peers.map(Arc::from))
    }
}

pub(crate) async fn require_peer_identity(
    Extension(guard): Extension<PeerIdentityGuard>,
    mut request: Request<Body>,
    next: Next,
) -> Response {
    request.headers_mut().remove("x-alt-peer-identity");
    let Some(allowed_peers) = guard.0.as_deref() else {
        return next.run(request).await;
    };

    let Some(cert) = request.extensions().get::<CertificateDer<'static>>() else {
        return (StatusCode::UNAUTHORIZED, "unauthenticated peer").into_response();
    };

    if crate::tls::verify_cert_peer_identity(cert, allowed_peers).is_ok() {
        next.run(request).await
    } else {
        (StatusCode::FORBIDDEN, "peer not allowlisted").into_response()
    }
}

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

    fn peer_guarded_app() -> Router {
        let allowed = vec!["alt-backend".to_string()];
        Router::new()
            .route(
                "/v1/recaps/7days",
                get(|| async { StatusCode::OK }),
            )
            .route(
                "/v1/morning/letters/regenerate",
                axum::routing::post(|| async { StatusCode::OK }),
            )
            .route_layer(middleware::from_fn(require_peer_identity))
            .layer(Extension(PeerIdentityGuard::new(Some(&allowed))))
    }

    #[tokio::test]
    async fn peer_guard_logs_warn_with_path_and_reason_on_missing_client_certificate() {
        use std::sync::{Arc, Mutex};
        use tracing::instrument::WithSubscriber;

        #[derive(Clone, Default)]
        struct BufWriter(Arc<Mutex<Vec<u8>>>);
        impl std::io::Write for BufWriter {
            fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
                self.0.lock().unwrap().extend_from_slice(buf);
                Ok(buf.len())
            }
            fn flush(&mut self) -> std::io::Result<()> {
                Ok(())
            }
        }

        let buf = BufWriter::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_max_level(tracing::Level::WARN)
            .without_time()
            .with_writer(move || writer.clone())
            .finish();

        let req = Request::builder()
            .method(Method::GET)
            .uri("/v1/recaps/7days?secret=query-secret-value")
            .header(AUTHORIZATION, "Bearer header-secret-value")
            .header("x-alt-peer-identity", "spoofed-peer-header")
            .body(Body::empty())
            .expect("request builds");

        let resp = peer_guarded_app()
            .oneshot(req)
            .with_subscriber(subscriber)
            .await
            .expect("request succeeds");
        assert_eq!(resp.status(), StatusCode::UNAUTHORIZED);

        let captured = String::from_utf8(buf.0.lock().unwrap().clone()).expect("utf8");
        assert_eq!(
            captured.lines().count(),
            1,
            "expected one WARN line for 401 rejection, got: {captured}"
        );
        assert!(captured.contains("WARN"));
        assert!(captured.contains("/v1/recaps/7days"));
        assert!(captured.contains("no_client_certificate"));
        assert!(!captured.contains("query-secret-value"));
        assert!(!captured.contains("header-secret-value"));
        assert!(!captured.contains("spoofed-peer-header"));
    }

    #[tokio::test]
    async fn peer_guard_logs_warn_with_presented_names_on_unlisted_peer() {
        use std::sync::{Arc, Mutex};
        use tracing::instrument::WithSubscriber;

        #[derive(Clone, Default)]
        struct BufWriter(Arc<Mutex<Vec<u8>>>);
        impl std::io::Write for BufWriter {
            fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
                self.0.lock().unwrap().extend_from_slice(buf);
                Ok(buf.len())
            }
            fn flush(&mut self) -> std::io::Result<()> {
                Ok(())
            }
        }

        let mut params = rcgen::CertificateParams::new(vec![
            "untrusted-san-a".to_string(),
            "untrusted-san-b".to_string(),
        ])
        .expect("cert params");
        let mut dn = rcgen::DistinguishedName::new();
        dn.push(rcgen::DnType::CommonName, "untrusted-cn");
        params.distinguished_name = dn;
        let key = rcgen::KeyPair::generate().expect("keypair");
        let cert = params.self_signed(&key).expect("self-signed");

        let buf = BufWriter::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_max_level(tracing::Level::WARN)
            .without_time()
            .with_writer(move || writer.clone())
            .finish();

        let mut req = Request::builder()
            .method(Method::POST)
            .uri("/v1/morning/letters/regenerate")
            .header(AUTHORIZATION, "Bearer header-secret-value")
            .body(Body::empty())
            .expect("request builds");
        req.extensions_mut().insert(cert.der().clone());

        let resp = peer_guarded_app()
            .oneshot(req)
            .with_subscriber(subscriber)
            .await
            .expect("request succeeds");
        assert_eq!(resp.status(), StatusCode::FORBIDDEN);

        let captured = String::from_utf8(buf.0.lock().unwrap().clone()).expect("utf8");
        assert_eq!(
            captured.lines().count(),
            1,
            "expected one WARN line for 403 rejection, got: {captured}"
        );
        assert!(captured.contains("WARN"));
        assert!(captured.contains("/v1/morning/letters/regenerate"));
        assert!(captured.contains("peer_not_allow_listed"));
        assert!(captured.contains("untrusted-cn"));
        assert!(captured.contains("untrusted-san-a"));
        assert!(captured.contains("untrusted-san-b"));
        assert!(!captured.contains("header-secret-value"));
    }
}
