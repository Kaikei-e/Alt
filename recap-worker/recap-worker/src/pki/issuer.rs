//! Production step-ca client for in-process enrollment.
//!
//! Official APIs used:
//!   - `josekit` — JWE unwrap of the subject-scoped JWK + ES256 OTT JWT
//!   - `rcgen` — local ECDSA P-256 key + CSR
//!   - `reqwest` + rustls with a pinned RootCertStore (never skip verify)
//!
//! Residual secret lifetime: decrypted JWK JSON and the provisioner password
//! are held in `zeroize::Zeroizing` and wiped on drop. The live `Jwk` stays
//! in `ProvisionerCred` for OTT signing and is not cloned out of that mutex.
//! josekit's `Pbes2HmacAeskwJweDecrypter.private_key` is a plain `Vec<u8>`
//! with no `Zeroize` on drop, so PBKDF2 password bytes (and the derived KEK
//! inside OpenSSL during `decrypt`) can remain in heap until allocator reuse.
//! That residue is unavoidable without forking josekit.
//!
//! The HTTP contract (GET /provisioners, POST /sign + OTT, POST /rekey + mTLS)
//! matches `alt-backend/app/internal/pki.NativeStepCAIssuer`.

use std::collections::HashSet;
use std::net::IpAddr;
use std::path::Path;
use std::sync::Arc;
use std::time::Duration;

use async_trait::async_trait;
use base64::Engine;
use base64::engine::general_purpose::URL_SAFE_NO_PAD;
use josekit::jwe::{self, JweDecrypter};
use josekit::jwk::Jwk;
use josekit::jws::{ES256, JwsHeader};
use josekit::jwt::{self, JwtPayload};
use rand::Rng;
use rcgen::{CertificateParams, DistinguishedName, DnType, KeyPair, SanType};
use rustls::ClientConfig;
use rustls::RootCertStore;
use rustls::pki_types::{CertificateDer, PrivateKeyDer};
use sha2::{Digest, Sha256};
use tokio::sync::Mutex;
use tokio_util::sync::CancellationToken;
use zeroize::{Zeroize, Zeroizing};

use super::certfile::{LoadedCert, parse_leaf_der, pem_all_certs, pem_first_cert};
use super::config::{Config, require_https};
use super::error::PkiError;
use super::filesafe::{MAX_PASSWORD_BYTES, MAX_ROOT_PEM_BYTES, read_regular_no_follow};
use super::manager::Issuer;
use x509_parser::prelude::{FromDer, X509Certificate};

const DEFAULT_ISSUE_TIMEOUT: Duration = Duration::from_secs(15);
const OTT_LIFETIME: Duration = Duration::from_secs(5 * 60);

pub(crate) const MAX_RESPONSE_BYTES: u64 = 2 << 20;
pub(crate) const MAX_PROVISIONER_PAGES: usize = 20;
pub(crate) const STEP_CA_PBES2_P2C: u64 = 600_000;
const MAX_COMPACT_JWE_BYTES: usize = 16 * 1024;
const MAX_DECRYPTED_JWK_BYTES: usize = 8 * 1024;
const MAX_JWE_HEADER_BYTES: usize = 1024;

const JWE_HEADER_ALLOWLIST: &[&str] = &["alg", "enc", "p2c", "p2s", "kid", "cty"];
const PBES2_ALGS: &[&str] = &[
    "PBES2-HS256+A128KW",
    "PBES2-HS384+A192KW",
    "PBES2-HS512+A256KW",
];
const GCM_ENCS: &[&str] = &["A128GCM", "A256GCM"];

struct ProvisionerCred {
    name: String,
    jwk: Jwk,
    fingerprint: String,
    audience: String,
}

pub struct NativeStepCAIssuer {
    pub(crate) ca_url: String,
    pub(crate) root_file: String,
    pub(crate) provisioner: String,
    pub(crate) password_file: String,
    pub(crate) timeout: Duration,
    cred: Mutex<Option<ProvisionerCred>>,
}

impl NativeStepCAIssuer {
    #[must_use]
    pub fn from_config(cfg: &Config) -> Self {
        Self::new(
            cfg.ca_url.clone(),
            cfg.root_file.clone(),
            cfg.provisioner.clone(),
            cfg.password_file.clone(),
            cfg.issue_timeout,
        )
    }

    #[must_use]
    pub fn new(
        ca_url: String,
        root_file: String,
        provisioner: String,
        password_file: String,
        timeout: Duration,
    ) -> Self {
        Self {
            ca_url,
            root_file,
            provisioner,
            password_file,
            timeout: if timeout.is_zero() {
                DEFAULT_ISSUE_TIMEOUT
            } else {
                timeout
            },
            cred: Mutex::new(None),
        }
    }

    fn guard_provisioner(&self) -> Result<(), PkiError> {
        if self.provisioner == "pki-agent" || self.provisioner.is_empty() {
            return Err(PkiError::SharedProvisioner {
                got: self.provisioner.clone(),
            });
        }
        if self.password_file.contains("step_ca_root_password") {
            return Err(PkiError::SharedRootSecret {
                got: self.password_file.clone(),
            });
        }
        Ok(())
    }

    async fn ensure_cred(&self, cancel: &CancellationToken) -> Result<(), PkiError> {
        if cancel.is_cancelled() {
            return Err(PkiError::Canceled);
        }
        {
            let guard = self.cred.lock().await;
            if guard.is_some() {
                return Ok(());
            }
        }
        let mut password = read_provisioner_password(&self.password_file)?;
        let client = self.http_client(None)?;
        let loaded: Result<ProvisionerCred, PkiError> = async {
            let fp = root_fingerprint(&self.root_file)?;
            health_check(&client, &self.ca_url, cancel).await?;
            let jwk =
                load_provisioner_jwk(&client, &self.ca_url, &self.provisioner, &password, cancel)
                    .await?;
            Ok(ProvisionerCred {
                name: self.provisioner.clone(),
                jwk,
                fingerprint: fp,
                audience: join_url(&self.ca_url, "/1.0/sign")?,
            })
        }
        .await;
        password.zeroize();
        let cred = loaded?;
        *self.cred.lock().await = Some(cred);
        Ok(())
    }

    pub(crate) fn http_client(
        &self,
        identity: Option<(Vec<CertificateDer<'static>>, PrivateKeyDer<'static>)>,
    ) -> Result<reqwest::Client, PkiError> {
        require_https(&self.ca_url)?;
        let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
        let pem = read_regular_no_follow(Path::new(&self.root_file), MAX_ROOT_PEM_BYTES)?;
        let mut roots = RootCertStore::empty();
        let mut added = false;
        for cert in rustls_pemfile::certs(&mut pem.as_slice()) {
            let cert = cert
                .map_err(|e| PkiError::other(format!("parse CA root {:?}: {e}", self.root_file)))?;
            roots
                .add(cert)
                .map_err(|e| PkiError::other(format!("add CA root: {e}")))?;
            added = true;
        }
        if !added {
            return Err(PkiError::other(format!(
                "parse CA root {:?}: no certificates",
                self.root_file
            )));
        }
        let builder = ClientConfig::builder_with_provider(Arc::new(
            rustls::crypto::aws_lc_rs::default_provider(),
        ))
        .with_protocol_versions(&[&rustls::version::TLS13])
        .map_err(|e| PkiError::other(format!("tls 1.3 config: {e}")))?
        .with_root_certificates(roots);
        let mut tls = match identity {
            Some((certs, key)) => builder
                .with_client_auth_cert(certs, key)
                .map_err(|e| PkiError::other(format!("client identity: {e}")))?,
            None => builder.with_no_client_auth(),
        };
        tls.alpn_protocols = vec![b"h2".to_vec(), b"http/1.1".to_vec()];
        reqwest::Client::builder()
            .use_preconfigured_tls(tls)
            .https_only(true)
            .redirect(reqwest::redirect::Policy::none())
            .no_proxy()
            .gzip(false)
            .timeout(self.timeout)
            .build()
            .map_err(|e| PkiError::other(format!("http client: {e}")))
    }

    pub(crate) fn validate_and_encode(
        &self,
        subject: &str,
        sans: &[String],
        resp: &SignResponse,
        key_pem: Vec<u8>,
        expected_spki: &[u8],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
        if resp.crt.trim().is_empty() {
            return Err(PkiError::other("empty sign response"));
        }
        let leaf = parse_leaf_der(&pem_first_cert(resp.crt.as_bytes())?)?;
        if leaf.common_name != subject {
            return Err(PkiError::other(format!(
                "issued CN {:?} does not match subject {subject:?}",
                leaf.common_name
            )));
        }
        cert_matches_sans(&leaf, sans)?;
        require_dual_eku(&leaf.der)?;
        let leaf_spki = spki_from_cert_der(&leaf.der)?;
        if leaf_spki.as_slice() != expected_spki {
            return Err(PkiError::other(
                "issued leaf public key does not match CSR key",
            ));
        }
        let root_pem = read_regular_no_follow(Path::new(&self.root_file), MAX_ROOT_PEM_BYTES)?;
        let mut chain_ders: Vec<Vec<u8>> = Vec::new();
        for pem_str in &resp.cert_chain {
            chain_ders.extend(pem_all_certs(pem_str.as_bytes())?);
        }
        if !resp.ca.trim().is_empty() {
            chain_ders.extend(pem_all_certs(resp.ca.as_bytes())?);
        }
        let intermediates: Vec<Vec<u8>> = chain_ders
            .into_iter()
            .filter(|d| d.as_slice() != leaf.der.as_slice())
            .collect();
        verify_issued_chain(&leaf.der, &intermediates, &root_pem)?;
        let mut cert_out = encode_pem("CERTIFICATE", &leaf.der);
        let mut seen = vec![leaf.der.clone()];
        for der in &intermediates {
            if seen.iter().any(|s| s == der) {
                continue;
            }
            seen.push(der.clone());
            cert_out.extend(encode_pem("CERTIFICATE", der));
        }
        assert_pair_usable(&cert_out, &key_pem)?;
        Ok((cert_out, key_pem))
    }
}

#[async_trait]
impl Issuer for NativeStepCAIssuer {
    async fn issue(
        &self,
        cancel: &CancellationToken,
        subject: &str,
        sans: &[String],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
        if cancel.is_cancelled() {
            return Err(PkiError::Canceled);
        }
        self.guard_provisioner()?;
        let sans = if sans.is_empty() {
            vec![subject.to_string()]
        } else {
            sans.to_vec()
        };
        self.ensure_cred(cancel).await?;
        let ott = {
            let guard = self.cred.lock().await;
            let cred = guard
                .as_ref()
                .ok_or_else(|| PkiError::other("provisioner credentials missing after load"))?;
            mint_ott(cred, subject, &sans)?
        };
        let (csr_pem, key_pem, spki) = create_csr(subject, &sans)?;
        let client = self.http_client(None)?;
        let sign_url = join_url(&self.ca_url, "/sign")?;
        let resp = tokio::select! {
            () = cancel.cancelled() => return Err(PkiError::Canceled),
            r = post_json(
                &client,
                &sign_url,
                serde_json::json!({ "csr": csr_pem, "ott": ott }),
            ) => r,
        }?;
        self.validate_and_encode(subject, &sans, &resp, key_pem, &spki)
    }

    fn supports_rekey(&self) -> bool {
        true
    }

    async fn rekey(
        &self,
        cancel: &CancellationToken,
        cert_pem: &[u8],
        key_pem: &[u8],
        subject: &str,
        sans: &[String],
    ) -> Result<(Vec<u8>, Vec<u8>), PkiError> {
        if cancel.is_cancelled() {
            return Err(PkiError::Canceled);
        }
        self.guard_provisioner()?;
        let sans = if sans.is_empty() {
            vec![subject.to_string()]
        } else {
            sans.to_vec()
        };
        let identity = parse_tls_identity(cert_pem, key_pem)?;
        let (csr_pem, new_key, spki) = create_csr(subject, &sans)?;
        let client = self.http_client(Some(identity))?;
        let rekey_url = join_url(&self.ca_url, "/rekey")?;
        let resp = tokio::select! {
            () = cancel.cancelled() => return Err(PkiError::Canceled),
            r = post_json(
                &client,
                &rekey_url,
                serde_json::json!({ "csr": csr_pem }),
            ) => r,
        }?;
        self.validate_and_encode(subject, &sans, &resp, new_key, &spki)
    }
}

#[derive(Debug, Default, serde::Deserialize)]
pub(crate) struct SignResponse {
    #[serde(default)]
    pub(crate) crt: String,
    #[serde(default)]
    pub(crate) ca: String,
    #[serde(default, rename = "certChain")]
    pub(crate) cert_chain: Vec<String>,
}

fn mint_ott(cred: &ProvisionerCred, subject: &str, sans: &[String]) -> Result<String, PkiError> {
    let mut jti_bytes = [0u8; 64];
    rand::rng().fill_bytes(&mut jti_bytes);
    let jti = hex_encode(&jti_bytes);
    let now = std::time::SystemTime::now();
    let exp = now + OTT_LIFETIME;
    let mut payload = JwtPayload::new();
    payload.set_issuer(&cred.name);
    payload.set_subject(subject);
    payload.set_audience(vec![cred.audience.clone()]);
    payload.set_jwt_id(&jti);
    payload.set_not_before(&now);
    payload.set_issued_at(&now);
    payload.set_expires_at(&exp);
    payload
        .set_claim("sans", Some(serde_json::json!(sans)))
        .map_err(|e| PkiError::other(format!("ott sans: {e}")))?;
    if !cred.fingerprint.is_empty() {
        payload
            .set_claim(
                "sha",
                Some(serde_json::Value::String(cred.fingerprint.clone())),
            )
            .map_err(|e| PkiError::other(format!("ott sha: {e}")))?;
    }
    let mut header = JwsHeader::new();
    let alg = cred
        .jwk
        .algorithm()
        .filter(|a| !a.is_empty())
        .unwrap_or("ES256");
    header.set_algorithm(alg);
    if let Some(kid) = cred.jwk.key_id() {
        header.set_key_id(kid);
    }
    let signer = ES256
        .signer_from_jwk(&cred.jwk)
        .map_err(|e| PkiError::other(format!("ott signer: {e}")))?;
    jwt::encode_with_signer(&payload, &header, &signer)
        .map_err(|e| PkiError::other(format!("sign ott: {e}")))
}

pub(crate) fn create_csr(
    subject: &str,
    sans: &[String],
) -> Result<(String, Vec<u8>, Vec<u8>), PkiError> {
    let mut params = CertificateParams::new(Vec::new())
        .map_err(|e| PkiError::other(format!("csr params: {e}")))?;
    params.distinguished_name = DistinguishedName::new();
    params.distinguished_name.push(DnType::CommonName, subject);
    params.subject_alt_names = sans
        .iter()
        .map(|san| split_san(san))
        .collect::<Result<Vec<_>, _>>()?;
    let key = KeyPair::generate().map_err(|e| PkiError::other(format!("generate key: {e}")))?;
    let spki = spki_from_public_pem(&key.public_key_pem())?;
    let csr = params
        .serialize_request(&key)
        .map_err(|e| PkiError::other(format!("create csr: {e}")))?;
    let csr_pem = csr
        .pem()
        .map_err(|e| PkiError::other(format!("csr pem: {e}")))?;
    Ok((csr_pem, key.serialize_pem().into_bytes(), spki))
}

fn split_san(san: &str) -> Result<SanType, PkiError> {
    if let Ok(ip) = san.parse::<IpAddr>() {
        return Ok(SanType::IpAddress(ip));
    }
    if san.contains("://") {
        return Ok(SanType::URI(
            san.try_into()
                .map_err(|e| PkiError::other(format!("san {san:?}: {e}")))?,
        ));
    }
    if san.contains('@') {
        return Ok(SanType::Rfc822Name(
            san.try_into()
                .map_err(|e| PkiError::other(format!("san {san:?}: {e}")))?,
        ));
    }
    Ok(SanType::DnsName(san.try_into().map_err(|e| {
        PkiError::other(format!("san {san:?}: {e}"))
    })?))
}

async fn load_provisioner_jwk(
    client: &reqwest::Client,
    ca_url: &str,
    name: &str,
    password: &[u8],
    cancel: &CancellationToken,
) -> Result<Jwk, PkiError> {
    let mut cursor = String::new();
    for _page in 0..MAX_PROVISIONER_PAGES {
        if cancel.is_cancelled() {
            return Err(PkiError::Canceled);
        }
        let mut url = reqwest::Url::parse(&join_url(ca_url, "/provisioners")?).map_err(|_| {
            PkiError::InsecureCaUrl {
                got: ca_url.to_string(),
            }
        })?;
        url.query_pairs_mut().append_pair("limit", "100");
        if !cursor.is_empty() {
            url.query_pairs_mut().append_pair("cursor", &cursor);
        }
        let resp = client.get(url).send().await.map_err(map_reqwest)?;
        let status = resp.status();
        let body = read_capped(resp).await?;
        if status.as_u16() >= 400 {
            return Err(PkiError::classify_ca_status(status.as_u16()));
        }
        let list: ProvisionersJson = serde_json::from_slice(&body)
            .map_err(|e| PkiError::other(format!("decode provisioners: {e}")))?;
        for p in list.provisioners {
            if !p.r#type.eq_ignore_ascii_case("JWK") || p.name != name || p.encrypted_key.is_empty()
            {
                continue;
            }
            return decrypt_provisioner_jwk(&p.encrypted_key, password);
        }
        if list.next_cursor.is_empty() {
            return Err(PkiError::other(format!(
                "jwk provisioner {name:?} not found (or password is wrong)"
            )));
        }
        cursor = list.next_cursor;
    }
    Err(PkiError::ProvisionerPageLimit)
}

pub(crate) fn decrypt_provisioner_jwk(
    encrypted_key: &str,
    password: &[u8],
) -> Result<Jwk, PkiError> {
    let header_alg = inspect_jwe_header(encrypted_key)?;
    let decrypter: Box<dyn JweDecrypter> = match header_alg.as_str() {
        "PBES2-HS256+A128KW" => Box::new(
            jwe::PBES2_HS256_A128KW
                .decrypter_from_bytes(password)
                .map_err(|_| PkiError::other("jwe decrypter"))?,
        ),
        "PBES2-HS384+A192KW" => Box::new(
            jwe::PBES2_HS384_A192KW
                .decrypter_from_bytes(password)
                .map_err(|_| PkiError::other("jwe decrypter"))?,
        ),
        "PBES2-HS512+A256KW" => Box::new(
            jwe::PBES2_HS512_A256KW
                .decrypter_from_bytes(password)
                .map_err(|_| PkiError::other("jwe decrypter"))?,
        ),
        _ => return Err(PkiError::other("unsupported provisioner jwe alg")),
    };
    let (data, _hdr) = jwe::deserialize_compact(encrypted_key, decrypter.as_ref())
        .map_err(|_| PkiError::other("decrypt provisioner jwk"))?;
    let data = Zeroizing::new(data);
    if data.len() > MAX_DECRYPTED_JWK_BYTES {
        return Err(PkiError::other(
            "decrypted provisioner jwk exceeds size cap",
        ));
    }
    Jwk::from_bytes(data.as_slice()).map_err(|_| PkiError::other("unmarshal provisioner jwk"))
}

pub(crate) fn inspect_jwe_header(compact: &str) -> Result<String, PkiError> {
    if compact.len() > MAX_COMPACT_JWE_BYTES {
        return Err(PkiError::other("provisioner jwe exceeds compact size cap"));
    }
    let mut parts = compact.split('.');
    let header_b64 = parts
        .next()
        .ok_or_else(|| PkiError::other("malformed provisioner jwe"))?;
    if parts.clone().count() != 4 {
        return Err(PkiError::other("malformed provisioner jwe"));
    }
    let json = URL_SAFE_NO_PAD
        .decode(header_b64)
        .or_else(|_| base64::engine::general_purpose::URL_SAFE.decode(header_b64))
        .map_err(|_| PkiError::other("malformed provisioner jwe"))?;
    if json.len() > MAX_JWE_HEADER_BYTES {
        return Err(PkiError::other("provisioner jwe header exceeds size cap"));
    }
    let v: serde_json::Value =
        serde_json::from_slice(&json).map_err(|_| PkiError::other("malformed provisioner jwe"))?;
    let obj = v
        .as_object()
        .ok_or_else(|| PkiError::other("malformed provisioner jwe"))?;
    for key in obj.keys() {
        if !JWE_HEADER_ALLOWLIST.contains(&key.as_str()) {
            return Err(PkiError::other(format!(
                "provisioner jwe has unknown header {key}"
            )));
        }
    }
    if obj.contains_key("zip") {
        return Err(PkiError::other("provisioner jwe zip is forbidden"));
    }
    let alg = obj
        .get("alg")
        .and_then(|a| a.as_str())
        .ok_or_else(|| PkiError::other("provisioner jwe missing alg"))?;
    if !PBES2_ALGS.contains(&alg) {
        return Err(PkiError::other("unsupported provisioner jwe alg"));
    }
    let enc = obj
        .get("enc")
        .and_then(|a| a.as_str())
        .ok_or_else(|| PkiError::other("provisioner jwe missing enc"))?;
    if !GCM_ENCS.contains(&enc) {
        return Err(PkiError::other("provisioner jwe enc must be GCM"));
    }
    let p2c = obj.get("p2c").and_then(serde_json::Value::as_u64);
    if p2c != Some(STEP_CA_PBES2_P2C) {
        return Err(PkiError::other("provisioner jwe p2c must be 600000"));
    }
    if obj.get("p2s").and_then(|s| s.as_str()).is_none() {
        return Err(PkiError::other("provisioner jwe missing p2s"));
    }
    if let Some(cty) = obj.get("cty") {
        let cty = cty.as_str().unwrap_or("");
        if !cty.eq_ignore_ascii_case("jwk+json") {
            return Err(PkiError::other("provisioner jwe cty must be jwk+json"));
        }
    }
    Ok(alg.to_string())
}

#[derive(Debug, serde::Deserialize)]
struct ProvisionersJson {
    #[serde(default)]
    provisioners: Vec<ProvisionerJson>,
    #[serde(default, rename = "nextCursor")]
    next_cursor: String,
}

#[derive(Debug, serde::Deserialize)]
struct ProvisionerJson {
    #[serde(default)]
    r#type: String,
    #[serde(default)]
    name: String,
    #[serde(default, rename = "encryptedKey")]
    encrypted_key: String,
}

async fn post_json(
    client: &reqwest::Client,
    url: &str,
    payload: serde_json::Value,
) -> Result<SignResponse, PkiError> {
    let resp = client
        .post(url)
        .json(&payload)
        .send()
        .await
        .map_err(map_reqwest)?;
    let status = resp.status();
    let body = read_capped(resp).await?;
    if status.as_u16() >= 400 {
        return Err(PkiError::classify_ca_status(status.as_u16()));
    }
    serde_json::from_slice(&body).map_err(|_| PkiError::other("decode sign response"))
}

async fn read_capped(resp: reqwest::Response) -> Result<Vec<u8>, PkiError> {
    let status = resp.status().as_u16();
    if (300..400).contains(&status) {
        return Err(PkiError::Redirect);
    }
    if resp
        .content_length()
        .is_some_and(|n| n > MAX_RESPONSE_BYTES)
    {
        return Err(PkiError::ResponseTooLarge);
    }
    let mut resp = resp;
    let mut buf = Vec::new();
    loop {
        let chunk = resp
            .chunk()
            .await
            .map_err(|_| PkiError::other("read CA response"))?;
        let Some(chunk) = chunk else {
            break;
        };
        let next = u64::try_from(buf.len().saturating_add(chunk.len())).unwrap_or(u64::MAX);
        if next > MAX_RESPONSE_BYTES {
            return Err(PkiError::ResponseTooLarge);
        }
        buf.extend_from_slice(&chunk);
    }
    Ok(buf)
}

fn join_url(base: &str, path: &str) -> Result<String, PkiError> {
    let parsed = reqwest::Url::parse(base).map_err(|_| PkiError::InsecureCaUrl {
        got: base.to_string(),
    })?;
    parsed
        .join(path)
        .map(|u| u.to_string())
        .map_err(|_| PkiError::InsecureCaUrl {
            got: base.to_string(),
        })
}

pub(crate) fn cert_matches_sans(cert: &LoadedCert, sans: &[String]) -> Result<(), PkiError> {
    let (want_dns, want_ip, want_email, want_uri) = split_sans(sans);
    if !same_folded_set(&cert.dns_sans, &want_dns) {
        return Err(PkiError::other("issued DNS SAN set mismatch"));
    }
    if !same_ip_set(&cert.ip_sans, &want_ip) {
        return Err(PkiError::other("issued IP SAN set mismatch"));
    }
    if !same_folded_set(&cert.email_sans, &want_email) {
        return Err(PkiError::other("issued email SAN set mismatch"));
    }
    if !same_str_set(&cert.uri_sans, &want_uri) {
        return Err(PkiError::other("issued URI SAN set mismatch"));
    }
    Ok(())
}

fn split_sans(sans: &[String]) -> (Vec<String>, Vec<IpAddr>, Vec<String>, Vec<String>) {
    let mut dns = Vec::new();
    let mut ip = Vec::new();
    let mut email = Vec::new();
    let mut uri = Vec::new();
    for san in sans {
        if let Ok(addr) = san.parse::<IpAddr>() {
            ip.push(addr);
        } else if san.contains("://") {
            uri.push(san.clone());
        } else if san.contains('@') {
            email.push(san.clone());
        } else {
            dns.push(san.clone());
        }
    }
    (dns, ip, email, uri)
}

fn same_folded_set(got: &[String], want: &[String]) -> bool {
    if got.len() != want.len() {
        return false;
    }
    let mut counts = std::collections::HashMap::<String, i32>::new();
    for w in want {
        *counts.entry(w.to_ascii_lowercase()).or_insert(0) += 1;
    }
    for g in got {
        let k = g.to_ascii_lowercase();
        match counts.get_mut(&k) {
            Some(c) if *c > 0 => *c -= 1,
            _ => return false,
        }
    }
    true
}

fn same_str_set(got: &[String], want: &[String]) -> bool {
    if got.len() != want.len() {
        return false;
    }
    let mut counts = std::collections::HashMap::<String, i32>::new();
    for w in want {
        *counts.entry(w.clone()).or_insert(0) += 1;
    }
    for g in got {
        match counts.get_mut(g) {
            Some(c) if *c > 0 => *c -= 1,
            _ => return false,
        }
    }
    true
}

fn same_ip_set(got: &[IpAddr], want: &[IpAddr]) -> bool {
    if got.len() != want.len() {
        return false;
    }
    let mut used = vec![false; want.len()];
    for g in got {
        let mut matched = false;
        for (i, w) in want.iter().enumerate() {
            if used[i] {
                continue;
            }
            if g == w {
                used[i] = true;
                matched = true;
                break;
            }
        }
        if !matched {
            return false;
        }
    }
    true
}

fn require_dual_eku(leaf_der: &[u8]) -> Result<(), PkiError> {
    let (_, cert) = X509Certificate::from_der(leaf_der)
        .map_err(|e| PkiError::CertParseFailed(e.to_string()))?;
    let eku = cert
        .extended_key_usage()
        .ok()
        .flatten()
        .ok_or_else(|| PkiError::other("issued cert missing EKU"))?;
    if !eku.value.server_auth || !eku.value.client_auth {
        return Err(PkiError::other(
            "issued cert must have both clientAuth and serverAuth EKUs",
        ));
    }
    Ok(())
}

fn spki_from_cert_der(der: &[u8]) -> Result<Vec<u8>, PkiError> {
    let (_, cert) =
        X509Certificate::from_der(der).map_err(|e| PkiError::CertParseFailed(e.to_string()))?;
    Ok(cert.public_key().raw.to_vec())
}

fn spki_from_public_pem(pem: &str) -> Result<Vec<u8>, PkiError> {
    let mut reader = pem.as_bytes();
    for item in rustls_pemfile::read_all(&mut reader) {
        if let rustls_pemfile::Item::SubjectPublicKeyInfo(der) =
            item.map_err(|e| PkiError::other(format!("parse CSR public key: {e}")))?
        {
            return Ok(der.to_vec());
        }
    }
    Err(PkiError::other("CSR public key missing SPKI"))
}

fn assert_pair_usable(cert_pem: &[u8], key_pem: &[u8]) -> Result<(), PkiError> {
    let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
    let certs = rustls_pemfile::certs(&mut &cert_pem[..])
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| PkiError::other(format!("issued cert/key pair is not usable: {e}")))?;
    if certs.is_empty() {
        return Err(PkiError::other("issued cert/key pair is not usable"));
    }
    let key = rustls_pemfile::private_key(&mut &key_pem[..])
        .map_err(|e| PkiError::other(format!("issued cert/key pair is not usable: {e}")))?
        .ok_or_else(|| PkiError::other("issued cert/key pair is not usable"))?;
    let signing = rustls::crypto::aws_lc_rs::sign::any_supported_type(&key)
        .map_err(|e| PkiError::other(format!("issued cert/key pair is not usable: {e}")))?;
    let _pair = rustls::sign::CertifiedKey::new(certs, signing);
    Ok(())
}

pub(crate) fn verify_issued_chain(
    leaf_der: &[u8],
    intermediates: &[Vec<u8>],
    root_pem: &[u8],
) -> Result<(), PkiError> {
    let roots = pem_all_certs(root_pem)?;
    if roots.is_empty() {
        return Err(PkiError::other("parse CA root: no certificates"));
    }
    let root_set: HashSet<&[u8]> = roots.iter().map(Vec::as_slice).collect();
    let mut pool: Vec<Vec<u8>> = intermediates.to_vec();
    pool.extend(roots.iter().cloned());
    let mut current = leaf_der.to_vec();
    for _ in 0..8 {
        if root_set.contains(current.as_slice()) {
            if current.as_slice() == leaf_der {
                return Err(PkiError::other(
                    "issued chain does not verify against pinned CA root",
                ));
            }
            return Ok(());
        }
        let current_owned = current.clone();
        let (_, cert) = X509Certificate::from_der(&current_owned)
            .map_err(|e| PkiError::CertParseFailed(e.to_string()))?;
        let mut next = None;
        for cand in &pool {
            if cand.as_slice() == current.as_slice() {
                continue;
            }
            let Ok((_, issuer)) = X509Certificate::from_der(cand) else {
                continue;
            };
            if cert.verify_signature(Some(issuer.public_key())).is_ok() {
                next = Some(cand.clone());
                break;
            }
        }
        let Some(n) = next else {
            return Err(PkiError::other(
                "issued chain does not verify against pinned CA root",
            ));
        };
        current = n;
    }
    Err(PkiError::other(
        "issued chain does not verify against pinned CA root",
    ))
}

fn root_fingerprint(root_file: &str) -> Result<String, PkiError> {
    let pem = read_regular_no_follow(Path::new(root_file), MAX_ROOT_PEM_BYTES)?;
    let certs = pem_all_certs(&pem)?;
    let last = certs
        .last()
        .ok_or_else(|| PkiError::other(format!("parse CA root {root_file:?}: no certificates")))?;
    Ok(sha256_hex(last))
}

fn parse_tls_identity(
    cert_pem: &[u8],
    key_pem: &[u8],
) -> Result<(Vec<CertificateDer<'static>>, PrivateKeyDer<'static>), PkiError> {
    let certs = rustls_pemfile::certs(&mut &cert_pem[..])
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| PkiError::other(format!("parse current cert for rekey: {e}")))?;
    if certs.is_empty() {
        return Err(PkiError::other(
            "parse current cert for rekey: no certificates",
        ));
    }
    let key = rustls_pemfile::private_key(&mut &key_pem[..])
        .map_err(|e| PkiError::other(format!("parse current cert for rekey: {e}")))?
        .ok_or_else(|| PkiError::other("parse current cert for rekey: no private key"))?;
    Ok((certs, key))
}

async fn health_check(
    client: &reqwest::Client,
    ca_url: &str,
    cancel: &CancellationToken,
) -> Result<(), PkiError> {
    let health_url = join_url(ca_url, "/health")?;
    tokio::select! {
        () = cancel.cancelled() => Err(PkiError::Canceled),
        r = client.get(&health_url).send() => {
            let resp = r.map_err(map_reqwest)?;
            let status = resp.status().as_u16();
            let _body = read_capped(resp).await?;
            if status >= 400 {
                return Err(PkiError::classify_ca_status(status));
            }
            Ok(())
        }
    }
}

fn read_provisioner_password(path: &str) -> Result<Zeroizing<Vec<u8>>, PkiError> {
    let mut raw = Zeroizing::new(read_regular_no_follow(Path::new(path), MAX_PASSWORD_BYTES)?);
    let trimmed = std::str::from_utf8(raw.as_slice())
        .unwrap_or("")
        .trim()
        .as_bytes()
        .to_vec();
    raw.zeroize();
    if trimmed.is_empty() {
        return Err(PkiError::other(format!(
            "provisioner password file {path:?} is empty"
        )));
    }
    Ok(Zeroizing::new(trimmed))
}

fn encode_pem(tag: &str, der: &[u8]) -> Vec<u8> {
    let b64 = base64::engine::general_purpose::STANDARD.encode(der);
    let mut out = format!("-----BEGIN {tag}-----\n").into_bytes();
    for chunk in b64.as_bytes().chunks(64) {
        out.extend_from_slice(chunk);
        out.push(b'\n');
    }
    out.extend(format!("-----END {tag}-----\n").into_bytes());
    out
}

fn sha256_hex(der: &[u8]) -> String {
    hex_encode(&Sha256::digest(der))
}

fn hex_encode(bytes: &[u8]) -> String {
    const HEX: &[u8; 16] = b"0123456789abcdef";
    let mut out = String::with_capacity(bytes.len() * 2);
    for &b in bytes {
        out.push(HEX[(b >> 4) as usize] as char);
        out.push(HEX[(b & 0x0f) as usize] as char);
    }
    out
}

fn map_reqwest(err: reqwest::Error) -> PkiError {
    if err.is_timeout() {
        return PkiError::other("sign: timeout");
    }
    if err.is_connect() || err.is_request() {
        return PkiError::other("sign: transport");
    }
    PkiError::other("sign: request failed")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn source_does_not_skip_tls_verify() {
        let src = include_str!("issuer.rs");
        let prod = src.split("#[cfg(test)]").next().unwrap_or(src);
        assert!(!prod.contains("danger_accept_invalid_certs"));
        assert!(!prod.contains("InsecureSkipVerify"));
        assert!(!prod.contains(".dangerous()"));
        assert!(src.contains("https_only(true)"));
        assert!(src.contains("redirect::Policy::none"));
        assert!(src.contains("no_proxy()"));
        assert!(src.contains("gzip(false)"));
        assert!(src.contains("TLS13"));
        assert!(src.contains("use_preconfigured_tls"));
        assert!(src.contains("with_root_certificates"));
    }

    #[test]
    fn rejects_shared_provisioner() {
        let iss = NativeStepCAIssuer::new(
            "https://ca".into(),
            "/root.pem".into(),
            "pki-agent".into(),
            "/run/secrets/pki-agent-recap-worker-jwk".into(),
            Duration::from_secs(1),
        );
        let err = tokio::runtime::Runtime::new()
            .unwrap()
            .block_on(iss.issue(&CancellationToken::new(), "recap-worker", &[]))
            .expect_err("shared");
        assert!(matches!(err, PkiError::SharedProvisioner { .. }));
    }

    #[test]
    fn cert_matches_sans_rejects_extras() {
        let cert = LoadedCert {
            not_before: std::time::SystemTime::now(),
            not_after: std::time::SystemTime::now(),
            common_name: "recap-worker".into(),
            dns_sans: vec!["recap-worker".into(), "extra".into()],
            ip_sans: vec!["127.0.0.1".parse().unwrap()],
            email_sans: vec!["ops@alt.local".into()],
            uri_sans: vec!["spiffe://alt.local/recap-worker".into()],
            der: Vec::new(),
        };
        assert!(cert_matches_sans(&cert, &["recap-worker".into()]).is_err());
        assert!(
            cert_matches_sans(
                &cert,
                &[
                    "recap-worker".into(),
                    "extra".into(),
                    "127.0.0.1".into(),
                    "spiffe://alt.local/recap-worker".into(),
                    "ops@alt.local".into(),
                ]
            )
            .is_ok()
        );
    }

    #[test]
    fn inspect_jwe_rejects_wrong_p2c_and_zip() {
        let header = serde_json::json!({
            "alg": "PBES2-HS256+A128KW",
            "enc": "A128GCM",
            "p2c": 100_000,
            "p2s": "YWFhYWFhYWE",
        });
        let b64 = URL_SAFE_NO_PAD.encode(serde_json::to_vec(&header).unwrap());
        let compact = format!("{b64}.aaaa.bbbb.cccc.dddd");
        let err = inspect_jwe_header(&compact).expect_err("p2c");
        assert!(err.to_string().contains("p2c"));

        let header = serde_json::json!({
            "alg": "PBES2-HS256+A128KW",
            "enc": "A128GCM",
            "p2c": 600_000,
            "p2s": "YWFhYWFhYWE",
            "zip": "DEF",
        });
        let b64 = URL_SAFE_NO_PAD.encode(serde_json::to_vec(&header).unwrap());
        let compact = format!("{b64}.aaaa.bbbb.cccc.dddd");
        assert!(inspect_jwe_header(&compact).is_err());
    }
}

#[cfg(test)]
#[path = "issuer_http_tests.rs"]
mod http_tests;
