//! Fake step-ca HTTPS server + NativeStepCAIssuer unit tests.
//! Never talks to Alt's compose CA.

use super::*;
use crate::pki::certfile::pem_first_cert;
use crate::pki::manager::Issuer as PkiIssuer;
use http_body_util::{BodyExt, Full};
use hyper::body::{Bytes, Incoming};
use hyper::server::conn::http1;
use hyper::service::service_fn;
use hyper::{Method, Request, Response, StatusCode};
use hyper_util::rt::TokioIo;
use josekit::jwe::{self, JweHeader};
use josekit::jwk::Jwk;
use josekit::jws::ES256;
use josekit::jwt;
use rcgen::{
    BasicConstraints, CertificateParams, DistinguishedName, DnType, IsCa, Issuer, KeyPair,
    KeyUsagePurpose, SanType,
};
use rustls::ServerConfig;
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use rustls::server::WebPkiClientVerifier;
use std::collections::HashSet;
use std::path::PathBuf;
use std::sync::Arc;
use std::sync::atomic::{AtomicBool, AtomicU16, Ordering};
use std::time::Duration;
use tokio::net::TcpListener;
use tokio::sync::{Mutex, oneshot};
use tokio_rustls::TlsAcceptor;
use tokio_util::sync::CancellationToken;

struct FakeCa {
    provisioner: String,
    password: Vec<u8>,
    jwk: Jwk,
    enc_key: String,
    ca_cert_pem: Vec<u8>,
    server_cert_pem: Vec<u8>,
    server_key: KeyPair,
    issuer: Issuer<'static, KeyPair>,
    used_jti: Mutex<HashSet<String>>,
    seen: Mutex<Vec<(String, String, Vec<u8>)>>,
    sign_delay: Duration,
    block_sign: Mutex<Option<oneshot::Receiver<()>>>,
    malformed_sign: AtomicBool,
    mutate_cn: Mutex<Option<String>>,
    mutate_sans: Mutex<Option<Vec<String>>>,
    reject_expired: bool,
    reject_reuse: bool,
    redirect_sign: AtomicU16,
    sign_status: AtomicU16,
    sign_body: Mutex<Option<Vec<u8>>>,
    endless_provisioners: AtomicBool,
    extra_dns: Mutex<Vec<String>>,
}

impl FakeCa {
    fn new(provisioner: &str, password: &[u8]) -> Arc<Self> {
        let mut jwk = Jwk::generate_ec_key(josekit::jwk::P_256).expect("jwk");
        jwk.set_key_id("test-kid");
        jwk.set_algorithm("ES256");
        jwk.set_key_use("sig");
        let enc_key = encrypt_jwk(&jwk, password);
        let (ca_cert_pem, server_cert_pem, server_key, issuer) = generate_test_ca();
        Arc::new(Self {
            provisioner: provisioner.into(),
            password: password.to_vec(),
            jwk,
            enc_key,
            ca_cert_pem,
            server_cert_pem,
            server_key,
            issuer,
            used_jti: Mutex::new(HashSet::new()),
            seen: Mutex::new(Vec::new()),
            sign_delay: Duration::ZERO,
            block_sign: Mutex::new(None),
            malformed_sign: AtomicBool::new(false),
            mutate_cn: Mutex::new(None),
            mutate_sans: Mutex::new(None),
            reject_expired: true,
            reject_reuse: true,
            redirect_sign: AtomicU16::new(0),
            sign_status: AtomicU16::new(0),
            sign_body: Mutex::new(None),
            endless_provisioners: AtomicBool::new(false),
            extra_dns: Mutex::new(Vec::new()),
        })
    }

    async fn start(self: &Arc<Self>) -> (String, PathBuf) {
        self.start_with_versions(&[&rustls::version::TLS13, &rustls::version::TLS12])
            .await
    }

    async fn start_tls12_only(self: &Arc<Self>) -> (String, PathBuf) {
        self.start_with_versions(&[&rustls::version::TLS12]).await
    }

    async fn start_with_versions(
        self: &Arc<Self>,
        versions: &[&'static rustls::SupportedProtocolVersion],
    ) -> (String, PathBuf) {
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
        let ca_der = pem_first_cert(&self.ca_cert_pem).expect("ca der");
        let server_der = pem_first_cert(&self.server_cert_pem).expect("server der");
        let key_der = self.server_key.serialize_der();
        let mut roots = rustls::RootCertStore::empty();
        roots.add(CertificateDer::from(ca_der)).unwrap();
        let verifier = WebPkiClientVerifier::builder(Arc::new(roots))
            .allow_unauthenticated()
            .build()
            .unwrap();
        let mut server = ServerConfig::builder_with_provider(Arc::new(
            rustls::crypto::aws_lc_rs::default_provider(),
        ))
        .with_protocol_versions(versions)
        .expect("tls versions")
        .with_client_cert_verifier(verifier)
        .with_single_cert(
            vec![CertificateDer::from(server_der)],
            PrivateKeyDer::Pkcs8(key_der.into()),
        )
        .unwrap();
        server.alpn_protocols = vec![b"http/1.1".to_vec()];
        let acceptor = TlsAcceptor::from(Arc::new(server));
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let state = Arc::clone(self);
        tokio::spawn(async move {
            loop {
                let Ok((tcp, _)) = listener.accept().await else {
                    break;
                };
                let acceptor = acceptor.clone();
                let state = Arc::clone(&state);
                tokio::spawn(async move {
                    let Ok(tls) = acceptor.accept(tcp).await else {
                        return;
                    };
                    let peer = tls
                        .get_ref()
                        .1
                        .peer_certificates()
                        .map(<[CertificateDer<'_>]>::to_vec);
                    let io = TokioIo::new(tls);
                    let svc = service_fn(move |req| {
                        let state = Arc::clone(&state);
                        let peer = peer.clone();
                        async move { state.handle(req, peer).await }
                    });
                    let _ = http1::Builder::new().serve_connection(io, svc).await;
                });
            }
        });
        let dir = tempfile::TempDir::new().unwrap();
        // Leak the tempdir so the root file survives the test helper return.
        // Tests keep `root_file` on NativeStepCAIssuer for the duration.
        let root_file = dir.path().join("root.pem");
        std::fs::write(&root_file, &self.ca_cert_pem).unwrap();
        std::mem::forget(dir);
        (format!("https://{addr}"), root_file)
    }

    async fn handle(
        &self,
        req: Request<Incoming>,
        peer: Option<Vec<CertificateDer<'static>>>,
    ) -> Result<Response<Full<Bytes>>, std::convert::Infallible> {
        let method = req.method().clone();
        let path = req.uri().path().to_string();
        let body = req
            .collect()
            .await
            .map(http_body_util::Collected::to_bytes)
            .unwrap_or_default();
        self.seen
            .lock()
            .await
            .push((method.to_string(), path.clone(), body.to_vec()));
        if (path == "/sign" || path == "/1.0/sign")
            && self.redirect_sign.load(Ordering::SeqCst) != 0
        {
            let code = self.redirect_sign.load(Ordering::SeqCst);
            return Ok(Response::builder()
                .status(StatusCode::from_u16(code).unwrap_or(StatusCode::TEMPORARY_REDIRECT))
                .header("location", "/elsewhere")
                .body(Full::new(Bytes::new()))
                .unwrap());
        }
        let resp = match (method, path.as_str()) {
            (Method::GET, "/health") => json(StatusCode::OK, serde_json::json!({"status":"ok"})),
            (Method::GET, "/provisioners") => self.write_provisioners(),
            (Method::POST, "/sign" | "/1.0/sign") => self.handle_sign(&body).await,
            (Method::POST, "/rekey") => self.handle_rekey(&body, peer.as_deref()),
            _ => json(
                StatusCode::NOT_FOUND,
                serde_json::json!({"message":"not found"}),
            ),
        };
        Ok(resp)
    }

    fn write_provisioners(&self) -> Response<Full<Bytes>> {
        if self.endless_provisioners.load(Ordering::SeqCst) {
            return json(
                StatusCode::OK,
                serde_json::json!({"provisioners":[],"nextCursor":"next"}),
            );
        }
        let pub_jwk = self
            .jwk
            .to_public_key()
            .unwrap_or_else(|_| self.jwk.clone());
        let key_json = serde_json::to_value(&pub_jwk).unwrap_or(serde_json::Value::Null);
        json(
            StatusCode::OK,
            serde_json::json!({
                "provisioners": [{
                    "type": "JWK",
                    "name": self.provisioner,
                    "key": key_json,
                    "encryptedKey": self.enc_key,
                }]
            }),
        )
    }

    async fn handle_sign(&self, body: &Bytes) -> Response<Full<Bytes>> {
        if let Some(raw) = self.sign_body.lock().await.as_ref() {
            let status = self.sign_status.load(Ordering::SeqCst);
            return Response::builder()
                .status(StatusCode::from_u16(status).unwrap_or(StatusCode::CREATED))
                .header("content-type", "application/json")
                .body(Full::new(Bytes::from(raw.clone())))
                .unwrap();
        }
        if !self.sign_delay.is_zero() {
            tokio::time::sleep(self.sign_delay).await;
        }
        if let Some(rx) = self.block_sign.lock().await.as_mut() {
            let _ = rx.await;
        }
        if self.malformed_sign.load(Ordering::SeqCst) {
            return Response::builder()
                .status(StatusCode::CREATED)
                .body(Full::new(Bytes::from("{not-json")))
                .unwrap();
        }
        let req: serde_json::Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(e) => {
                return json(
                    StatusCode::BAD_REQUEST,
                    serde_json::json!({"message": format!("bad json: {e}")}),
                );
            }
        };
        let ott = req.get("ott").and_then(|v| v.as_str()).unwrap_or("");
        let csr = req.get("csr").and_then(|v| v.as_str()).unwrap_or("");
        match self.verify_ott(ott).await {
            Ok(_claims) => {}
            Err(e) => {
                return json(
                    StatusCode::UNAUTHORIZED,
                    serde_json::json!({"message": format!("ott: {e}")}),
                );
            }
        }
        self.sign_csr(csr)
    }

    fn handle_rekey(
        &self,
        body: &Bytes,
        peer: Option<&[CertificateDer<'static>]>,
    ) -> Response<Full<Bytes>> {
        if peer.is_none_or(<[CertificateDer<'_>]>::is_empty) {
            return json(
                StatusCode::BAD_REQUEST,
                serde_json::json!({"message": "missing client certificate"}),
            );
        }
        let req: serde_json::Value = match serde_json::from_slice(body) {
            Ok(v) => v,
            Err(_) => {
                return json(
                    StatusCode::BAD_REQUEST,
                    serde_json::json!({"message": "bad json"}),
                );
            }
        };
        let csr = req.get("csr").and_then(|v| v.as_str()).unwrap_or("");
        self.sign_csr(csr)
    }

    fn decode_ott(&self, ott: &str) -> Result<jwt::JwtPayload, String> {
        let verifier = ES256
            .verifier_from_jwk(&self.jwk)
            .map_err(|e| e.to_string())?;
        let (payload, _hdr) =
            jwt::decode_with_verifier(ott, &verifier).map_err(|e| e.to_string())?;
        if payload.issuer() != Some(self.provisioner.as_str()) {
            return Err("wrong issuer".into());
        }
        Ok(payload)
    }

    async fn verify_ott(&self, ott: &str) -> Result<jwt::JwtPayload, String> {
        let payload = self.decode_ott(ott)?;
        let aud_ok = payload
            .audience()
            .is_some_and(|a| a.iter().any(|x| x.contains("/1.0/sign")));
        if !aud_ok {
            return Err("wrong audience".into());
        }
        if self.reject_expired
            && let Some(exp) = payload.expires_at()
            && exp < std::time::SystemTime::now()
        {
            return Err("expired ott".into());
        }
        let jti = payload.jwt_id().unwrap_or("").to_string();
        if jti.is_empty() {
            return Err("missing jti".into());
        }
        let mut used = self.used_jti.lock().await;
        if self.reject_reuse && !used.insert(jti) {
            return Err("reused ott".into());
        }
        Ok(payload)
    }

    fn sign_csr(&self, csr_pem: &str) -> Response<Full<Bytes>> {
        let csr = match rcgen::CertificateSigningRequestParams::from_pem(csr_pem) {
            Ok(c) => c,
            Err(e) => {
                return json(
                    StatusCode::BAD_REQUEST,
                    serde_json::json!({"message": format!("csr: {e}")}),
                );
            }
        };
        let mut params = csr.params;
        if let Some(cn) = self.mutate_cn.try_lock().ok().and_then(|g| g.clone()) {
            params.distinguished_name = DistinguishedName::new();
            params
                .distinguished_name
                .push(DnType::CommonName, cn.clone());
            params.subject_alt_names = vec![SanType::DnsName(cn.try_into().unwrap())];
        }
        if let Some(sans) = self.mutate_sans.try_lock().ok().and_then(|g| g.clone()) {
            params.subject_alt_names = sans
                .into_iter()
                .map(|s| SanType::DnsName(s.try_into().unwrap()))
                .collect();
        }
        if let Ok(extra) = self.extra_dns.try_lock() {
            for d in extra.iter() {
                params
                    .subject_alt_names
                    .push(SanType::DnsName(d.as_str().try_into().unwrap()));
            }
        }
        params.extended_key_usages = vec![
            rcgen::ExtendedKeyUsagePurpose::ServerAuth,
            rcgen::ExtendedKeyUsagePurpose::ClientAuth,
        ];
        let csr = rcgen::CertificateSigningRequestParams {
            params,
            public_key: csr.public_key,
        };
        let cert = match csr.signed_by(&self.issuer) {
            Ok(c) => c,
            Err(e) => {
                return json(
                    StatusCode::INTERNAL_SERVER_ERROR,
                    serde_json::json!({"message": format!("sign: {e}")}),
                );
            }
        };
        let leaf_pem = cert.pem();
        let ca_pem = String::from_utf8_lossy(&self.ca_cert_pem).into_owned();
        json(
            StatusCode::CREATED,
            serde_json::json!({
                "crt": leaf_pem,
                "ca": ca_pem,
                "certChain": [leaf_pem, ca_pem],
            }),
        )
    }

    async fn last_sign(&self) -> Option<(String, Vec<u8>)> {
        let seen = self.seen.lock().await;
        seen.iter()
            .rev()
            .find(|(_, p, _)| p == "/sign" || p == "/1.0/sign")
            .map(|(m, _, b)| (m.clone(), b.clone()))
    }

    async fn last_rekey(&self) -> Option<(String, String)> {
        let seen = self.seen.lock().await;
        seen.iter()
            .rev()
            .find(|(_, p, _)| p == "/rekey")
            .map(|(m, p, _)| (m.clone(), p.clone()))
    }
}

fn json(status: StatusCode, v: serde_json::Value) -> Response<Full<Bytes>> {
    Response::builder()
        .status(status)
        .header("content-type", "application/json")
        .body(Full::new(Bytes::from(serde_json::to_vec(&v).unwrap())))
        .unwrap()
}

fn encrypt_jwk(jwk: &Jwk, password: &[u8]) -> String {
    let raw = serde_json::to_vec(jwk.as_ref()).expect("jwk bytes");
    let mut header = JweHeader::new();
    header.set_content_encryption("A128GCM");
    let mut encrypter = jwe::PBES2_HS256_A128KW
        .encrypter_from_bytes(password)
        .expect("encrypter");
    encrypter.set_iter_count(usize::try_from(STEP_CA_PBES2_P2C).expect("p2c"));
    jwe::serialize_compact(&raw, &header, &encrypter).expect("jwe")
}

fn generate_test_ca() -> (Vec<u8>, Vec<u8>, KeyPair, Issuer<'static, KeyPair>) {
    let mut ca_params = CertificateParams::new(Vec::<String>::new()).unwrap();
    ca_params.distinguished_name = DistinguishedName::new();
    ca_params
        .distinguished_name
        .push(DnType::CommonName, "alt-test-step-ca");
    ca_params.is_ca = IsCa::Ca(BasicConstraints::Unconstrained);
    ca_params.key_usages = vec![
        KeyUsagePurpose::DigitalSignature,
        KeyUsagePurpose::KeyCertSign,
        KeyUsagePurpose::CrlSign,
    ];
    let ca_key = KeyPair::generate().unwrap();
    let ca_cert = ca_params.self_signed(&ca_key).unwrap();
    let ca_pem = ca_cert.pem();
    let issuer =
        Issuer::from_ca_cert_pem(&ca_pem, KeyPair::from_pem(&ca_key.serialize_pem()).unwrap())
            .unwrap();

    let mut srv = CertificateParams::new(vec!["localhost".into()]).unwrap();
    srv.distinguished_name = DistinguishedName::new();
    srv.distinguished_name.push(DnType::CommonName, "localhost");
    srv.subject_alt_names = vec![
        SanType::DnsName("localhost".try_into().unwrap()),
        SanType::IpAddress(std::net::IpAddr::from([127, 0, 0, 1])),
    ];
    srv.extended_key_usages = vec![rcgen::ExtendedKeyUsagePurpose::ServerAuth];
    let server_key = KeyPair::generate().unwrap();
    let server_cert = srv.signed_by(&server_key, &issuer).unwrap();
    (
        ca_pem.into_bytes(),
        server_cert.pem().into_bytes(),
        server_key,
        issuer,
    )
}

fn write_password_file(password: &str, mode: u32) -> PathBuf {
    let dir = tempfile::TempDir::new().unwrap();
    let path = dir.path().join("pki-agent-recap-worker-jwk");
    std::fs::write(&path, format!("{password}\n")).unwrap();
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(mode)).unwrap();
    std::mem::forget(dir);
    path
}

async fn new_native_issuer(ca: &Arc<FakeCa>) -> NativeStepCAIssuer {
    let (url, root) = ca.start().await;
    let pw = write_password_file(std::str::from_utf8(&ca.password).unwrap(), 0o400);
    NativeStepCAIssuer::new(
        url,
        root.to_string_lossy().into(),
        ca.provisioner.clone(),
        pw.to_string_lossy().into(),
        Duration::from_secs(10),
    )
}

#[tokio::test]
async fn issue_success() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"subject-scoped-jwk-password");
    let iss = new_native_issuer(&ca).await;
    let (cert_pem, key_pem) = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .unwrap_or_else(|e| panic!("issue: {e}"));
    assert!(!cert_pem.is_empty() && !key_pem.is_empty());
    let leaf = crate::pki::certfile::parse_leaf_pem(&cert_pem).unwrap();
    assert_eq!(leaf.common_name, "recap-worker");
    let (method, body) = ca.last_sign().await.expect("sign");
    assert_eq!(method, "POST");
    let v: serde_json::Value = serde_json::from_slice(&body).unwrap();
    assert!(!v["ott"].as_str().unwrap_or("").is_empty());
    assert!(!v["csr"].as_str().unwrap_or("").is_empty());
    let claims = ca.decode_ott(v["ott"].as_str().unwrap()).unwrap();
    assert_eq!(claims.issuer(), Some("pki-agent-recap-worker"));
    assert_eq!(claims.subject(), Some("recap-worker"));
}

#[tokio::test]
async fn mints_distinct_otts() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-distinct");
    let iss = new_native_issuer(&ca).await;
    let cancel = CancellationToken::new();
    iss.issue(&cancel, "recap-worker", &["recap-worker".into()])
        .await
        .unwrap();
    iss.issue(&cancel, "recap-worker", &["recap-worker".into()])
        .await
        .unwrap();
    let seen = ca.seen.lock().await;
    let mut jtis = Vec::new();
    for (_, path, body) in seen.iter() {
        if path != "/sign" {
            continue;
        }
        let v: serde_json::Value = serde_json::from_slice(body).unwrap();
        let claims = ca.decode_ott(v["ott"].as_str().unwrap()).unwrap();
        jtis.push(claims.jwt_id().unwrap_or("").to_string());
    }
    assert_eq!(jtis.len(), 2);
    assert_ne!(jtis[0], jtis[1]);
    assert!(!jtis[0].is_empty());
}

#[tokio::test]
async fn reused_ott_rejected_by_ca() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-reuse");
    let iss = new_native_issuer(&ca).await;
    iss.issue(
        &CancellationToken::new(),
        "recap-worker",
        &["recap-worker".into()],
    )
    .await
    .unwrap();
    let (_, body) = ca.last_sign().await.unwrap();
    let client = iss.http_client(None).unwrap();
    let url = format!("{}/sign", iss_ca_url(&iss));
    let resp = client
        .post(url)
        .body(body)
        .header("content-type", "application/json")
        .send()
        .await
        .unwrap();
    assert!(
        resp.status().as_u16() >= 400,
        "reused ott status={}",
        resp.status()
    );
}

#[tokio::test]
async fn expired_ott_rejected_by_ca() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-exp");
    let iss = new_native_issuer(&ca).await;
    let now = std::time::SystemTime::now() - Duration::from_secs(3600);
    let mut payload = jwt::JwtPayload::new();
    payload.set_issuer(&ca.provisioner);
    payload.set_subject("recap-worker");
    payload.set_audience(vec![format!("{}/1.0/sign", iss_ca_url(&iss))]);
    payload.set_jwt_id("expired-jti");
    payload.set_not_before(&now);
    payload.set_issued_at(&now);
    payload.set_expires_at(&(now + Duration::from_secs(60)));
    payload
        .set_claim("sans", Some(serde_json::json!(["recap-worker"])))
        .unwrap();
    let mut header = josekit::jws::JwsHeader::new();
    header.set_algorithm("ES256");
    header.set_key_id("test-kid");
    let signer = ES256.signer_from_jwk(&ca.jwk).unwrap();
    let ott = jwt::encode_with_signer(&payload, &header, &signer).unwrap();
    let (csr, _, _) = create_csr("recap-worker", &["recap-worker".into()]).unwrap();
    let client = iss.http_client(None).unwrap();
    let resp = client
        .post(format!("{}/sign", iss_ca_url(&iss)))
        .json(&serde_json::json!({"csr": csr, "ott": ott}))
        .send()
        .await
        .unwrap();
    assert!(
        resp.status().as_u16() >= 400,
        "expired ott status={}",
        resp.status()
    );
}

#[tokio::test]
async fn wrong_ca_root() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-root");
    let mut iss = new_native_issuer(&ca).await;
    let (other_pem, _, _, _) = generate_test_ca();
    let dir = tempfile::TempDir::new().unwrap();
    let wrong = dir.path().join("wrong.pem");
    std::fs::write(&wrong, other_pem).unwrap();
    iss.root_file = wrong.to_string_lossy().into();
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("wrong root");
    assert!(
        !err.to_string().to_ascii_lowercase().contains("insecure"),
        "must not skip TLS verify: {err}"
    );
}

#[tokio::test]
async fn wrong_subject_rejected() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-sub");
    *ca.mutate_cn.lock().await = Some("evil".into());
    let iss = new_native_issuer(&ca).await;
    assert!(
        iss.issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()]
        )
        .await
        .is_err()
    );
}

#[tokio::test]
async fn wrong_san_rejected() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-san");
    *ca.mutate_sans.lock().await = Some(vec!["not-the-requested-san".into()]);
    let iss = new_native_issuer(&ca).await;
    assert!(
        iss.issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()]
        )
        .await
        .is_err()
    );
}

#[tokio::test]
async fn malformed_response() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-malformed");
    ca.malformed_sign.store(true, Ordering::SeqCst);
    let iss = new_native_issuer(&ca).await;
    assert!(
        iss.issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()]
        )
        .await
        .is_err()
    );
}

#[tokio::test]
async fn timeout() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-timeout");
    let (tx, rx) = oneshot::channel();
    *ca.block_sign.lock().await = Some(rx);
    let mut iss = new_native_issuer(&ca).await;
    iss.timeout = Duration::from_millis(50);
    let start = std::time::Instant::now();
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("timeout");
    let _ = tx.send(());
    assert!(
        start.elapsed() < Duration::from_millis(1500),
        "timeout did not bound: {:?}",
        start.elapsed()
    );
    let _ = err;
}

#[tokio::test]
async fn canceled() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-cancel");
    let (tx, rx) = oneshot::channel();
    *ca.block_sign.lock().await = Some(rx);
    let iss = new_native_issuer(&ca).await;
    let cancel = CancellationToken::new();
    let cancel2 = cancel.clone();
    let task = tokio::spawn(async move {
        iss.issue(&cancel2, "recap-worker", &["recap-worker".into()])
            .await
    });
    tokio::time::sleep(Duration::from_millis(200)).await;
    cancel.cancel();
    let err = tokio::time::timeout(Duration::from_secs(5), task)
        .await
        .expect("join")
        .expect("task")
        .expect_err("canceled");
    assert!(
        err.is_canceled() || err.to_string().contains("cancel"),
        "{err}"
    );
    let _ = tx.send(());
}

#[tokio::test]
async fn password_file_errors() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-files");
    let (url, root) = ca.start().await;
    let cases: Vec<(&str, PathBuf)> = vec![
        ("missing", {
            let dir = tempfile::TempDir::new().unwrap();
            let p = dir.path().join("nope");
            std::mem::forget(dir);
            p
        }),
        ("empty", write_password_file("", 0o400)),
        ("directory", {
            let dir = tempfile::TempDir::new().unwrap();
            let p = dir.path().to_path_buf();
            std::mem::forget(dir);
            p
        }),
        ("world-writable", write_password_file("secret", 0o666)),
    ];
    for (name, file) in cases {
        let iss = NativeStepCAIssuer::new(
            url.clone(),
            root.to_string_lossy().into(),
            "pki-agent-recap-worker".into(),
            file.to_string_lossy().into(),
            Duration::from_secs(1),
        );
        assert!(
            iss.issue(
                &CancellationToken::new(),
                "recap-worker",
                &["recap-worker".into()]
            )
            .await
            .is_err(),
            "{name}"
        );
    }
}

#[tokio::test]
async fn does_not_log_secrets() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"super-secret-jwk-password");
    let mut iss = new_native_issuer(&ca).await;
    iss.issue(
        &CancellationToken::new(),
        "recap-worker",
        &["recap-worker".into()],
    )
    .await
    .unwrap();
    iss.password_file = tempfile::TempDir::new()
        .unwrap()
        .path()
        .join("missing")
        .to_string_lossy()
        .into();
    *iss.cred.lock().await = None;
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("missing");
    assert!(
        !err.to_string().contains("super-secret-jwk-password"),
        "password leaked in error"
    );
}

#[tokio::test]
async fn rekey_uses_client_cert() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pw-rekey");
    let iss = new_native_issuer(&ca).await;
    let cancel = CancellationToken::new();
    let (cert_pem, key_pem) = iss
        .issue(&cancel, "recap-worker", &["recap-worker".into()])
        .await
        .unwrap();
    let (new_cert, new_key) = iss
        .rekey(
            &cancel,
            &cert_pem,
            &key_pem,
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .unwrap();
    assert!(!new_cert.is_empty() && !new_key.is_empty());
    let (method, path) = ca.last_rekey().await.expect("rekey");
    assert_eq!(method, "POST");
    assert_eq!(path, "/rekey");
    assert_ne!(cert_pem, new_cert);
}

fn iss_ca_url(iss: &NativeStepCAIssuer) -> &str {
    &iss.ca_url
}

#[tokio::test]
async fn rejects_307_and_308_redirects() {
    for code in [307_u16, 308_u16] {
        let ca = FakeCa::new("pki-agent-recap-worker", b"redirect-pw");
        ca.redirect_sign.store(code, Ordering::SeqCst);
        let iss = new_native_issuer(&ca).await;
        let err = iss
            .issue(
                &CancellationToken::new(),
                "recap-worker",
                &["recap-worker".into()],
            )
            .await
            .expect_err("redirect");
        assert!(matches!(err, PkiError::Redirect), "code={code} err={err:?}");
    }
}

#[tokio::test]
async fn http_client_requires_https() {
    let iss = NativeStepCAIssuer::new(
        "http://127.0.0.1:1".into(),
        tempfile::TempDir::new()
            .unwrap()
            .path()
            .join("missing.pem")
            .to_string_lossy()
            .into(),
        "pki-agent-recap-worker".into(),
        tempfile::TempDir::new()
            .unwrap()
            .path()
            .join("pki-agent-recap-worker-jwk")
            .to_string_lossy()
            .into(),
        Duration::from_secs(1),
    );
    let err = iss.http_client(None).expect_err("http");
    assert!(matches!(err, PkiError::InsecureCaUrl { .. }));
}

#[tokio::test]
async fn rejects_tls12_only_ca() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"tls12-pw");
    let (url, root) = ca.start_tls12_only().await;
    let pw = write_password_file("tls12-pw", 0o400);
    let iss = NativeStepCAIssuer::new(
        url,
        root.to_string_lossy().into(),
        ca.provisioner.clone(),
        pw.to_string_lossy().into(),
        Duration::from_secs(2),
    );
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("tls12");
    assert!(!err.to_string().is_empty());
}

#[tokio::test]
async fn ca_error_is_sentinel_without_body() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"sentinel-pw");
    let secret = "super-secret-jwk-password-and-ott";
    ca.sign_status.store(401, Ordering::SeqCst);
    *ca.sign_body.lock().await =
        Some(format!(r#"{{"status":401,"message":"{secret}"}}"#).into_bytes());
    let iss = new_native_issuer(&ca).await;
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("rejected");
    assert!(matches!(err, PkiError::CaRejected { status: 401 }));
    assert!(!err.to_string().contains(secret), "CA body leaked: {err}");
}

#[tokio::test]
async fn provisioner_page_cap() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"pages-pw");
    ca.endless_provisioners.store(true, Ordering::SeqCst);
    let iss = new_native_issuer(&ca).await;
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("pages");
    assert!(matches!(err, PkiError::ProvisionerPageLimit));
}

#[tokio::test]
async fn response_size_cap() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"huge-pw");
    ca.sign_status.store(201, Ordering::SeqCst);
    *ca.sign_body.lock().await = Some(vec![b'A'; (MAX_RESPONSE_BYTES as usize) + 8]);
    let iss = new_native_issuer(&ca).await;
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("size");
    assert!(matches!(err, PkiError::ResponseTooLarge));
}

#[tokio::test]
async fn password_symlink_rejected() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"symlink-pw");
    let (url, root) = ca.start().await;
    let dir = tempfile::TempDir::new().unwrap();
    let real = dir.path().join("pki-agent-recap-worker-jwk");
    std::fs::write(&real, "symlink-pw\n").unwrap();
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(&real, std::fs::Permissions::from_mode(0o400)).unwrap();
    let link = dir.path().join("link-jwk");
    std::os::unix::fs::symlink(&real, &link).unwrap();
    let iss = NativeStepCAIssuer::new(
        url,
        root.to_string_lossy().into(),
        "pki-agent-recap-worker".into(),
        link.to_string_lossy().into(),
        Duration::from_secs(1),
    );
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("symlink");
    assert!(matches!(err, PkiError::Symlink { .. }));
}

#[tokio::test]
async fn password_size_cap() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"cap-pw");
    let (url, root) = ca.start().await;
    let dir = tempfile::TempDir::new().unwrap();
    let path = dir.path().join("pki-agent-recap-worker-jwk");
    std::fs::write(
        &path,
        "x".repeat(usize::try_from(crate::pki::filesafe::MAX_PASSWORD_BYTES).unwrap() + 1),
    )
    .unwrap();
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o400)).unwrap();
    std::mem::forget(dir);
    let iss = NativeStepCAIssuer::new(
        url,
        root.to_string_lossy().into(),
        "pki-agent-recap-worker".into(),
        path.to_string_lossy().into(),
        Duration::from_secs(1),
    );
    let err = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .expect_err("size");
    assert!(matches!(err, PkiError::PasswordTooLarge));
}

#[tokio::test]
async fn rejects_extra_dns() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"extra-dns");
    ca.extra_dns.lock().await.push("evil.example".into());
    let iss = new_native_issuer(&ca).await;
    assert!(
        iss.issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()]
        )
        .await
        .is_err()
    );
}

#[tokio::test]
async fn exact_ip_and_uri_sans() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"typed-san");
    let iss = new_native_issuer(&ca).await;
    let sans = vec!["127.0.0.1".into(), "spiffe://alt.local/recap-worker".into()];
    let (cert_pem, key_pem) = iss
        .issue(&CancellationToken::new(), "recap-worker", &sans)
        .await
        .unwrap_or_else(|e| panic!("issue: {e}"));
    assert_pair_roundtrip(&cert_pem, &key_pem);
    let leaf = crate::pki::certfile::parse_leaf_pem(&cert_pem).unwrap();
    assert!(leaf.dns_sans.is_empty(), "dns={:?}", leaf.dns_sans);
    assert_eq!(
        leaf.ip_sans,
        vec!["127.0.0.1".parse::<std::net::IpAddr>().unwrap()]
    );
    assert_eq!(
        leaf.uri_sans,
        vec!["spiffe://alt.local/recap-worker".to_string()]
    );
}

#[tokio::test]
async fn rejects_mismatched_leaf_key() {
    let ca = FakeCa::new("pki-agent-recap-worker", b"key-mismatch");
    let iss = new_native_issuer(&ca).await;
    let (cert_pem, _) = iss
        .issue(
            &CancellationToken::new(),
            "recap-worker",
            &["recap-worker".into()],
        )
        .await
        .unwrap();
    let other = rcgen::KeyPair::generate().unwrap();
    let spki = {
        let pem = other.public_key_pem();
        let mut reader = pem.as_bytes();
        rustls_pemfile::read_all(&mut reader)
            .find_map(|item| match item.ok()? {
                rustls_pemfile::Item::SubjectPublicKeyInfo(der) => Some(der.to_vec()),
                _ => None,
            })
            .unwrap()
    };
    let err = iss
        .validate_and_encode(
            "recap-worker",
            &["recap-worker".into()],
            &SignResponse {
                crt: String::from_utf8(cert_pem).unwrap(),
                ..SignResponse::default()
            },
            other.serialize_pem().into_bytes(),
            &spki,
        )
        .expect_err("mismatch");
    assert!(err.to_string().contains("public key"));
}

fn assert_pair_roundtrip(cert_pem: &[u8], key_pem: &[u8]) {
    let certs = rustls_pemfile::certs(&mut &cert_pem[..])
        .collect::<Result<Vec<_>, _>>()
        .unwrap();
    let key = rustls_pemfile::private_key(&mut &key_pem[..])
        .unwrap()
        .unwrap();
    let signing = rustls::crypto::aws_lc_rs::sign::any_supported_type(&key).unwrap();
    let _ = rustls::sign::CertifiedKey::new(certs, signing);
}
