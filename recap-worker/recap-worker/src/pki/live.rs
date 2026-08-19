//! Isolated live gate against a disposable `smallstep/step-ca:0.30.2`.
//! Default `cargo test` skips this so the suite never talks to Alt's compose CA.
//!
//! ```text
//! PKI_NATIVE_LIVE_CA=1 cargo test -p recap-worker --lib pki::live -- --nocapture
//! ```

use super::certfile::CertFile;
use super::config::Config;
use super::issuer::NativeStepCAIssuer;
use super::manager::{Issuer, Manager};
use super::state::State;
use std::path::{Path, PathBuf};
use std::process::Command;
use std::time::{Duration, Instant};
use tokio_util::sync::CancellationToken;

const LIVE_IMAGE: &str = "smallstep/step-ca:0.30.2";
const LIVE_CONTAINER: &str = "alt-pki-recap-worker-itest";
const LIVE_NETWORK: &str = "alt-pki-recap-worker-itest-net";
const LIVE_HOST_PORT: &str = "19001";
const LIVE_CA_PASSWORD: &str = "itest-only";
const LIVE_JWK_PASSWORD: &str = "itest-jwk-recap-worker";
const LIVE_PROVISIONER: &str = "pki-agent-recap-worker";
const LIVE_SUBJECT: &str = "recap-worker";

#[tokio::test]
#[allow(clippy::too_many_lines)]
async fn disposable_live_ca() {
    if std::env::var("PKI_NATIVE_LIVE_CA").ok().as_deref() != Some("1") {
        eprintln!(
            "skipping: set PKI_NATIVE_LIVE_CA=1 to run isolated disposable step-ca; never talks to Alt compose CA"
        );
        return;
    }
    assert!(
        Command::new("docker").arg("version").output().is_ok(),
        "docker is required for the live gate"
    );

    let deadline = Instant::now() + Duration::from_secs(50);
    let tmp = tempfile::TempDir::new().unwrap();
    let _cleanup = LiveCleanup;

    let _ = docker(&["rm", "-f", LIVE_CONTAINER]);
    let _ = docker(&["network", "rm", LIVE_NETWORK]);
    docker(&["network", "create", LIVE_NETWORK]).expect("network create");
    docker(&[
        "run",
        "--rm",
        "-d",
        "--name",
        LIVE_CONTAINER,
        "--network",
        LIVE_NETWORK,
        "-p",
        &format!("127.0.0.1:{LIVE_HOST_PORT}:9000"),
        "-e",
        "DOCKER_STEPCA_INIT_NAME=alt-itest",
        "-e",
        "DOCKER_STEPCA_INIT_DNS_NAMES=localhost,127.0.0.1",
        "-e",
        &format!("DOCKER_STEPCA_INIT_PASSWORD={LIVE_CA_PASSWORD}"),
        "-e",
        &format!("DOCKER_STEPCA_INIT_WITH_CA_URL=https://127.0.0.1:{LIVE_HOST_PORT}"),
        LIVE_IMAGE,
    ])
    .expect("docker run");

    let root_file = tmp.path().join("root_ca.crt");
    wait_live_ca_root(&root_file, deadline);
    add_live_jwk_provisioner(deadline);
    let password_file = write_password_file(tmp.path(), LIVE_JWK_PASSWORD);

    let ca_url = format!("https://127.0.0.1:{LIVE_HOST_PORT}");
    let iss = NativeStepCAIssuer::new(
        ca_url,
        root_file.to_string_lossy().into(),
        LIVE_PROVISIONER.into(),
        password_file.to_string_lossy().into(),
        Duration::from_secs(10),
    );
    let cancel = CancellationToken::new();
    let (cert_pem, key_pem) = iss
        .issue(&cancel, LIVE_SUBJECT, &[LIVE_SUBJECT.to_string()])
        .await
        .unwrap_or_else(|e| panic!("issue: {e}"));
    let leaf1 = must_live_leaf(&cert_pem, &key_pem, &root_file);

    let (rekey_cert, rekey_key) = iss
        .rekey(
            &cancel,
            &cert_pem,
            &key_pem,
            LIVE_SUBJECT,
            &[LIVE_SUBJECT.to_string()],
        )
        .await
        .unwrap_or_else(|e| panic!("rekey: {e}"));
    let leaf2 = must_live_leaf(&rekey_cert, &rekey_key, &root_file);
    assert_ne!(leaf1, leaf2, "rekey returned the same certificate");

    let files = CertFile::new(
        tmp.path().join("svc-cert.pem"),
        tmp.path().join("svc-key.pem"),
    );
    files.write(&rekey_cert, &rekey_key).unwrap();
    let mut mgr = Manager::new(
        Config {
            mode: "enabled".into(),
            subject: LIVE_SUBJECT.into(),
            sans: vec![LIVE_SUBJECT.into()],
            cert_path: files.cert_path.to_string_lossy().into(),
            key_path: files.key_path.to_string_lossy().into(),
            ca_url: format!("https://127.0.0.1:{LIVE_HOST_PORT}"),
            root_file: root_file.to_string_lossy().into(),
            provisioner: LIVE_PROVISIONER.into(),
            password_file: password_file.to_string_lossy().into(),
            renew_at_fraction: 0.66,
            tick_interval: Duration::from_secs(3600),
            retry_backoff: Duration::from_millis(1),
            retry_attempts: 1,
            issue_timeout: Duration::from_secs(10),
        },
        std::sync::Arc::new(iss),
        files.clone(),
    );

    let parsed = super::certfile::parse_leaf_pem(&rekey_cert).unwrap();
    let total = parsed.not_after.duration_since(parsed.not_before).unwrap();
    let near = parsed.not_before + (total * 3 / 4);
    mgr.now = Box::new(move || near);
    let state = mgr
        .tick(&cancel)
        .await
        .unwrap_or_else(|e| panic!("near-expiry tick: {e}"));
    assert!(
        matches!(state, State::Fresh | State::NearExpiry),
        "near-expiry state={state}"
    );
    let after_rekey = std::fs::read(&files.cert_path).unwrap();
    let leaf3 = must_live_leaf(
        &after_rekey,
        &std::fs::read(&files.key_path).unwrap(),
        &root_file,
    );
    assert_ne!(leaf2, leaf3, "manager rekey did not replace the leaf");

    let expired = super::certfile::parse_leaf_pem(&after_rekey)
        .unwrap()
        .not_after
        + Duration::from_secs(60);
    mgr.now = Box::new(move || expired);
    mgr.tick(&cancel)
        .await
        .unwrap_or_else(|e| panic!("expired tick: {e}"));
    let after_expire = std::fs::read(&files.cert_path).unwrap();
    let _ = must_live_leaf(
        &after_expire,
        &std::fs::read(&files.key_path).unwrap(),
        &root_file,
    );
    assert_ne!(
        after_rekey, after_expire,
        "expired re-enroll did not replace the leaf"
    );

    std::fs::remove_file(&files.cert_path).unwrap();
    std::fs::remove_file(&files.key_path).unwrap();
    mgr.now = Box::new(std::time::SystemTime::now);
    mgr.tick(&cancel)
        .await
        .unwrap_or_else(|e| panic!("missing tick: {e}"));
    let after_missing = std::fs::read(&files.cert_path).unwrap();
    let _ = must_live_leaf(
        &after_missing,
        &std::fs::read(&files.key_path).unwrap(),
        &root_file,
    );
}

struct LiveCleanup;

impl Drop for LiveCleanup {
    fn drop(&mut self) {
        let _ = docker(&["rm", "-f", LIVE_CONTAINER]);
        let _ = docker(&["network", "rm", LIVE_NETWORK]);
    }
}

fn docker(args: &[&str]) -> Result<String, String> {
    let out = Command::new("docker")
        .args(args)
        .output()
        .map_err(|e| e.to_string())?;
    let stdout = String::from_utf8_lossy(&out.stdout).into_owned();
    let stderr = String::from_utf8_lossy(&out.stderr).into_owned();
    if out.status.success() {
        Ok(stdout + &stderr)
    } else {
        Err(format!("{} {stdout}{stderr}", out.status))
    }
}

fn wait_live_ca_root(dest: &Path, deadline: Instant) {
    let mut last = String::new();
    while Instant::now() < deadline {
        match docker(&[
            "exec",
            LIVE_CONTAINER,
            "test",
            "-f",
            "/home/step/certs/root_ca.crt",
        ]) {
            Ok(_) => {
                docker(&[
                    "cp",
                    &format!("{LIVE_CONTAINER}:/home/step/certs/root_ca.crt"),
                    &dest.to_string_lossy(),
                ])
                .expect("docker cp root");
                return;
            }
            Err(e) => last = e,
        }
        std::thread::sleep(Duration::from_millis(200));
    }
    let logs = docker(&["logs", LIVE_CONTAINER]).unwrap_or_default();
    panic!("CA root did not appear: {last}\nlogs:\n{logs}");
}

fn add_live_jwk_provisioner(deadline: Instant) {
    let script = format!(
        r"set -euo pipefail
printf '%s\n' {pw:?} > /tmp/pki-agent-recap-worker-jwk
chmod 400 /tmp/pki-agent-recap-worker-jwk
step ca provisioner add {prov:?} --type JWK --create \
  --password-file /tmp/pki-agent-recap-worker-jwk \
  --ca-config /home/step/config/ca.json
kill -HUP 1
",
        pw = LIVE_JWK_PASSWORD,
        prov = LIVE_PROVISIONER
    );
    docker(&["exec", LIVE_CONTAINER, "sh", "-c", &script]).expect("provisioner add");
    let cfg = docker(&[
        "exec",
        LIVE_CONTAINER,
        "grep",
        "-F",
        LIVE_PROVISIONER,
        "/home/step/config/ca.json",
    ])
    .unwrap_or_default();
    assert!(
        cfg.contains(LIVE_PROVISIONER),
        "provisioner not in ca.json: {cfg}"
    );
    let wait_until = Instant::now() + Duration::from_secs(8);
    let mut last = String::new();
    while Instant::now() < wait_until && Instant::now() < deadline {
        match docker(&[
            "exec",
            LIVE_CONTAINER,
            "step",
            "ca",
            "provisioner",
            "list",
            "--ca-url",
            "https://localhost:9000",
            "--root",
            "/home/step/certs/root_ca.crt",
        ]) {
            Ok(out) if out.contains(LIVE_PROVISIONER) => return,
            Ok(out) => last = out,
            Err(e) => last = e,
        }
        std::thread::sleep(Duration::from_millis(250));
    }
    panic!("provisioner {LIVE_PROVISIONER} not served after SIGHUP: {last}");
}

fn write_password_file(dir: &Path, password: &str) -> PathBuf {
    let path = dir.join("pki-agent-recap-worker-jwk");
    std::fs::write(&path, format!("{password}\n")).unwrap();
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o400)).unwrap();
    path
}

fn must_live_leaf(cert_pem: &[u8], key_pem: &[u8], root_file: &Path) -> Vec<u8> {
    rustls_pemfile::certs(&mut &cert_pem[..])
        .collect::<Result<Vec<_>, _>>()
        .expect("certs")
        .first()
        .expect("leaf")
        .as_ref()
        .to_vec();
    let _ = rustls_pemfile::private_key(&mut &key_pem[..])
        .expect("key parse")
        .expect("key");
    let leaf = super::certfile::parse_leaf_pem(cert_pem).expect("leaf parse");
    assert_eq!(leaf.common_name, LIVE_SUBJECT);
    assert!(
        leaf.dns_sans
            .iter()
            .any(|d| d.eq_ignore_ascii_case(LIVE_SUBJECT)),
        "sans={:?}",
        leaf.dns_sans
    );
    let root_pem = std::fs::read(root_file).unwrap();
    super::issuer::verify_issued_chain(&leaf.der, &[], &root_pem).unwrap_or_else(|e| {
        let all = super::certfile::pem_all_certs(cert_pem).unwrap();
        let inter: Vec<Vec<u8>> = all.into_iter().skip(1).collect();
        super::issuer::verify_issued_chain(&leaf.der, &inter, &root_pem)
            .unwrap_or_else(|e2| panic!("chain verify: {e}; retry: {e2}"));
    });
    leaf.der
}
