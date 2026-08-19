use std::path::Path;
use std::time::Duration;

use super::error::PkiError;
use super::filesafe::{MAX_ENV_FILE_BYTES, read_regular_no_follow};

pub const MODE_ENABLED: &str = "enabled";
pub const MODE_DISABLED: &str = "disabled";

const DEFAULT_RENEW_AT: f64 = 0.66;
const DEFAULT_TICK: Duration = Duration::from_secs(5 * 60);
const DEFAULT_BACKOFF: Duration = Duration::from_secs(1);
const DEFAULT_RETRIES: u32 = 5;

const DEFAULT_CERT_PATH: &str = "/certs/svc-cert.pem";
const DEFAULT_KEY_PATH: &str = "/certs/svc-key.pem";
const DEFAULT_CA_URL: &str = "https://step-ca:9000";
const DEFAULT_ROOT_FILE: &str = "/trust/ca-bundle.pem";

/// Typed enrollment input for one binary/subject.
#[derive(Debug, Clone)]
pub struct Config {
    pub mode: String,
    pub subject: String,
    pub sans: Vec<String>,
    pub cert_path: String,
    pub key_path: String,
    pub ca_url: String,
    pub root_file: String,
    pub provisioner: String,
    pub password_file: String,
    pub renew_at_fraction: f64,
    pub tick_interval: Duration,
    pub retry_backoff: Duration,
    pub retry_attempts: u32,
    pub issue_timeout: Duration,
}

impl Config {
    #[must_use]
    pub fn is_enabled(&self) -> bool {
        self.mode == MODE_ENABLED
    }
}

#[must_use]
pub fn provisioner_name(subject: &str) -> String {
    format!("pki-agent-{subject}")
}

#[must_use]
pub fn provisioner_password_file(subject: &str) -> String {
    format!("/run/secrets/pki-agent-{subject}-jwk")
}

#[must_use]
pub fn provisioner_password_basename(subject: &str) -> String {
    format!("pki-agent-{subject}-jwk")
}

/// Load enrollment config for `service_name`.
///
/// `PKI_ENROLLMENT` must be an explicit `enabled` / `disabled` (default
/// disabled). `CERT_PATH`/`KEY_PATH` fall back to the existing
/// `MTLS_CERT_FILE`/`MTLS_KEY_FILE` so in-process enrollment can write the
/// same pair the rustls reloader already watches — without a compose cutover.
pub fn load_config(service_name: &str) -> Result<Config, PkiError> {
    let mode = load_enrollment_mode()?;
    let subject = get_env("CERT_SUBJECT", service_name)?;
    let mut cfg = Config {
        mode,
        sans: Vec::new(),
        cert_path: first_env(&["CERT_PATH", "MTLS_CERT_FILE"], DEFAULT_CERT_PATH)?,
        key_path: first_env(&["KEY_PATH", "MTLS_KEY_FILE"], DEFAULT_KEY_PATH)?,
        ca_url: get_env("STEP_CA_URL", DEFAULT_CA_URL)?,
        root_file: first_env(&["STEP_CA_ROOT_FILE", "MTLS_CA_FILE"], DEFAULT_ROOT_FILE)?,
        provisioner: get_env("STEP_CA_PROVISIONER", &provisioner_name(&subject))?,
        password_file: get_env(
            "STEP_CA_PROVISIONER_PASSWORD_FILE",
            &provisioner_password_file(&subject),
        )?,
        subject,
        renew_at_fraction: DEFAULT_RENEW_AT,
        tick_interval: DEFAULT_TICK,
        retry_backoff: DEFAULT_BACKOFF,
        retry_attempts: DEFAULT_RETRIES,
        issue_timeout: Duration::from_secs(15),
    };

    if let Ok(s) = std::env::var("CERT_SANS") {
        cfg.sans = s
            .split(',')
            .map(str::trim)
            .filter(|p| !p.is_empty())
            .map(ToOwned::to_owned)
            .collect();
    }
    if cfg.sans.is_empty() && !cfg.subject.is_empty() {
        cfg.sans = vec![cfg.subject.clone()];
    }
    if let Ok(v) = std::env::var("RENEW_AT_FRACTION")
        && !v.is_empty()
    {
        cfg.renew_at_fraction = v
            .parse::<f64>()
            .map_err(|e| PkiError::other(format!("RENEW_AT_FRACTION: {e}")))?;
    }
    if let Ok(v) = std::env::var("PKI_ENROLLMENT_TICK_INTERVAL")
        && !v.is_empty()
    {
        cfg.tick_interval = parse_duration(&v)
            .map_err(|e| PkiError::other(format!("PKI_ENROLLMENT_TICK_INTERVAL: {e}")))?;
    }
    cfg.validate()?;
    Ok(cfg)
}

impl Config {
    pub(crate) fn validate(&self) -> Result<(), PkiError> {
        if self.subject.is_empty() {
            return Err(PkiError::other("CERT_SUBJECT is required"));
        }
        if self.renew_at_fraction <= 0.0 || self.renew_at_fraction >= 1.0 {
            return Err(PkiError::other(format!(
                "RENEW_AT_FRACTION must be in (0,1), got {}",
                self.renew_at_fraction
            )));
        }
        if !self.is_enabled() {
            return Ok(());
        }
        if self.provisioner == "pki-agent" || self.provisioner.is_empty() {
            return Err(PkiError::SharedProvisioner {
                got: self.provisioner.clone(),
            });
        }
        let want_prov = provisioner_name(&self.subject);
        if self.provisioner != want_prov {
            return Err(PkiError::other(format!(
                "provisioner {:?} must be exactly {want_prov:?}",
                self.provisioner
            )));
        }
        if self.password_file.contains("step_ca_root_password") {
            return Err(PkiError::SharedRootSecret {
                got: self.password_file.clone(),
            });
        }
        let want_base = provisioner_password_basename(&self.subject);
        let got_base = Path::new(&self.password_file)
            .file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("");
        if got_base != want_base {
            return Err(PkiError::other(format!(
                "provisioner password file basename {got_base:?} must be exactly {want_base:?}"
            )));
        }
        let cleaned = Path::new(&self.password_file);
        if cleaned.parent() == Some(Path::new("/run/secrets"))
            && self.password_file != provisioner_password_file(&self.subject)
        {
            return Err(PkiError::other(format!(
                "provisioner password file {:?} must be {:?}",
                self.password_file,
                provisioner_password_file(&self.subject)
            )));
        }
        require_https(&self.ca_url)?;
        for (name, val) in [
            ("CERT_PATH", self.cert_path.as_str()),
            ("KEY_PATH", self.key_path.as_str()),
            ("STEP_CA_URL", self.ca_url.as_str()),
            ("STEP_CA_ROOT_FILE", self.root_file.as_str()),
            (
                "STEP_CA_PROVISIONER_PASSWORD_FILE",
                self.password_file.as_str(),
            ),
        ] {
            if val.trim().is_empty() {
                return Err(PkiError::other(format!(
                    "{name} is required when PKI_ENROLLMENT=enabled"
                )));
            }
        }
        Ok(())
    }
}

fn load_enrollment_mode() -> Result<String, PkiError> {
    if std::env::var_os("PKI_ENROLLMENT_FILE").is_some() {
        let raw = get_env("PKI_ENROLLMENT", "")?;
        let mode = raw.trim().to_ascii_lowercase();
        if mode != MODE_ENABLED && mode != MODE_DISABLED {
            return Err(PkiError::other(format!(
                "PKI_ENROLLMENT={mode:?} must be {MODE_ENABLED:?} or {MODE_DISABLED:?}"
            )));
        }
        return Ok(mode);
    }
    match std::env::var("PKI_ENROLLMENT") {
        Err(std::env::VarError::NotPresent) => Ok(MODE_DISABLED.to_string()),
        Err(err) => Err(PkiError::other(format!("PKI_ENROLLMENT: {err}"))),
        Ok(v) => {
            let mode = v.trim().to_ascii_lowercase();
            if mode != MODE_ENABLED && mode != MODE_DISABLED {
                return Err(PkiError::other(format!(
                    "PKI_ENROLLMENT={v:?} must be {MODE_ENABLED:?} or {MODE_DISABLED:?}"
                )));
            }
            Ok(mode)
        }
    }
}

pub(crate) fn require_https(raw: &str) -> Result<(), PkiError> {
    let parsed = reqwest::Url::parse(raw).ok();
    match parsed {
        Some(u) if u.scheme() == "https" && u.host_str().is_some_and(|h| !h.is_empty()) => Ok(()),
        _ => Err(PkiError::InsecureCaUrl { got: raw.into() }),
    }
}

fn get_env(key: &str, fallback: &str) -> Result<String, PkiError> {
    let file_key = format!("{key}_FILE");
    if let Some(file_ref) = std::env::var_os(&file_key) {
        let file_ref = file_ref.to_string_lossy();
        if file_ref.trim().is_empty() {
            return Err(PkiError::other(format!("{file_key} is empty")));
        }
        let b = read_regular_no_follow(Path::new(file_ref.as_ref()), MAX_ENV_FILE_BYTES)
            .map_err(|e| PkiError::other(format!("read {file_key}: {e}")))?;
        let s = String::from_utf8_lossy(&b).trim().to_string();
        if s.is_empty() {
            return Err(PkiError::other(format!("{file_key} is empty")));
        }
        return Ok(s);
    }
    match std::env::var(key) {
        Ok(v) if !v.is_empty() => Ok(v),
        _ => Ok(fallback.to_string()),
    }
}

fn first_env(keys: &[&str], fallback: &str) -> Result<String, PkiError> {
    for key in keys {
        let file_key = format!("{key}_FILE");
        if std::env::var_os(&file_key).is_some() || std::env::var_os(key).is_some() {
            return get_env(key, "");
        }
    }
    Ok(fallback.to_string())
}

fn parse_duration(raw: &str) -> Result<Duration, String> {
    // Go time.ParseDuration subset: ns/us/ms/s/m/h
    let raw = raw.trim();
    if let Some(num) = raw.strip_suffix("ms") {
        return num
            .parse::<u64>()
            .map(Duration::from_millis)
            .map_err(|e| e.to_string());
    }
    if let Some(num) = raw.strip_suffix('s') {
        return num
            .parse::<u64>()
            .map(Duration::from_secs)
            .map_err(|e| e.to_string());
    }
    if let Some(num) = raw.strip_suffix('m') {
        return num
            .parse::<u64>()
            .map(|n| Duration::from_secs(n * 60))
            .map_err(|e| e.to_string());
    }
    if let Some(num) = raw.strip_suffix('h') {
        return num
            .parse::<u64>()
            .map(|n| Duration::from_secs(n * 3600))
            .map_err(|e| e.to_string());
    }
    Err(format!("invalid duration {raw:?}"))
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    #[test]
    fn load_config_default_disabled() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", None::<&str>),
                ("CERT_SUBJECT", None::<&str>),
                ("STEP_CA_PROVISIONER", None::<&str>),
                ("STEP_CA_PROVISIONER_PASSWORD_FILE", None::<&str>),
            ],
            || {
                let c = load_config("recap-worker").expect("load");
                assert_eq!(c.mode, MODE_DISABLED);
                assert_eq!(c.subject, "recap-worker");
            },
        );
    }

    #[test]
    fn load_config_garbage_mode_fails() {
        temp_env::with_var("PKI_ENROLLMENT", Some("maybe"), || {
            assert!(load_config("recap-worker").is_err());
        });
    }

    #[test]
    fn load_config_enabled_rejects_shared_provisioner() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                ("STEP_CA_PROVISIONER", Some("pki-agent")),
                (
                    "STEP_CA_PROVISIONER_PASSWORD_FILE",
                    Some("/run/secrets/pki-agent-recap-worker-jwk"),
                ),
            ],
            || {
                let err = load_config("recap-worker").expect_err("shared provisioner");
                assert!(matches!(err, PkiError::SharedProvisioner { .. }));
            },
        );
    }

    #[test]
    fn load_config_enabled_rejects_shared_root_secret() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                (
                    "STEP_CA_PROVISIONER_PASSWORD_FILE",
                    Some("/run/secrets/step_ca_root_password"),
                ),
            ],
            || {
                let err = load_config("recap-worker").expect_err("shared root");
                assert!(matches!(err, PkiError::SharedRootSecret { .. }));
            },
        );
    }

    #[test]
    fn load_config_enabled_subject_scoped_defaults() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                ("STEP_CA_PROVISIONER", None::<&str>),
                ("STEP_CA_PROVISIONER_PASSWORD_FILE", None::<&str>),
            ],
            || {
                let c = load_config("recap-worker").expect("load");
                assert_eq!(c.provisioner, "pki-agent-recap-worker");
                assert_eq!(c.password_file, "/run/secrets/pki-agent-recap-worker-jwk");
            },
        );
    }

    #[test]
    fn load_config_distinct_subjects_do_not_share_identity() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("STEP_CA_PROVISIONER", None::<&str>),
                ("STEP_CA_PROVISIONER_PASSWORD_FILE", None::<&str>),
            ],
            || {
                temp_env::with_var("CERT_SUBJECT", Some("recap-worker"), || {
                    let a = load_config("recap-worker").expect("a");
                    temp_env::with_var("CERT_SUBJECT", Some("alt-data-hub"), || {
                        let b = load_config("alt-data-hub").expect("b");
                        assert_ne!(a.provisioner, b.provisioner);
                        assert_ne!(a.password_file, b.password_file);
                        assert_ne!(
                            Path::new(&a.password_file).file_name(),
                            Path::new(&b.password_file).file_name()
                        );
                    });
                });
            },
        );
    }

    #[test]
    fn load_config_falls_back_to_mtls_cert_paths() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_DISABLED)),
                ("CERT_PATH", None::<&str>),
                ("KEY_PATH", None::<&str>),
                ("MTLS_CERT_FILE", Some("/certs/recap-cert.pem")),
                ("MTLS_KEY_FILE", Some("/certs/recap-key.pem")),
            ],
            || {
                let c = load_config("recap-worker").expect("load");
                assert_eq!(c.cert_path, "/certs/recap-cert.pem");
                assert_eq!(c.key_path, "/certs/recap-key.pem");
            },
        );
    }

    #[test]
    fn load_config_enabled_rejects_http_ca_url() {
        temp_env::with_vars(
            [
                ("PKI_ENROLLMENT", Some(MODE_ENABLED)),
                ("CERT_SUBJECT", Some("recap-worker")),
                ("STEP_CA_URL", Some("http://step-ca:9000")),
            ],
            || {
                assert!(load_config("recap-worker").is_err());
            },
        );
    }
}
