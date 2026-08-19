use std::fs::{self};
use std::path::PathBuf;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use x509_parser::prelude::*;

use super::error::PkiError;

#[derive(Debug, Clone)]
pub struct LoadedCert {
    pub not_before: SystemTime,
    pub not_after: SystemTime,
    pub common_name: String,
    pub dns_sans: Vec<String>,
    pub ip_sans: Vec<std::net::IpAddr>,
    pub email_sans: Vec<String>,
    pub uri_sans: Vec<String>,
    pub der: Vec<u8>,
}

/// Fault injected into `CertFile::write` from tests. Production builds have
/// no corresponding branch.
#[cfg(test)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) enum InstallFault {
    None = 0,
    FailCertRename = 1,
    CrashAfterKeyRename = 2,
}

/// Reads and atomically replaces a PEM leaf + key pair.
#[derive(Debug)]
pub struct CertFile {
    pub cert_path: PathBuf,
    pub key_path: PathBuf,
    #[cfg(test)]
    fault: std::sync::atomic::AtomicU8,
}

impl Clone for CertFile {
    fn clone(&self) -> Self {
        Self {
            cert_path: self.cert_path.clone(),
            key_path: self.key_path.clone(),
            #[cfg(test)]
            fault: std::sync::atomic::AtomicU8::new(
                self.fault.load(std::sync::atomic::Ordering::SeqCst),
            ),
        }
    }
}

impl CertFile {
    #[must_use]
    pub fn new(cert_path: impl Into<PathBuf>, key_path: impl Into<PathBuf>) -> Self {
        Self {
            cert_path: cert_path.into(),
            key_path: key_path.into(),
            #[cfg(test)]
            fault: std::sync::atomic::AtomicU8::new(InstallFault::None as u8),
        }
    }

    #[cfg(test)]
    pub(crate) fn inject_fault(&self, fault: InstallFault) {
        self.fault
            .store(fault as u8, std::sync::atomic::Ordering::SeqCst);
    }

    pub fn load(&self) -> Result<LoadedCert, PkiError> {
        self.recover_incomplete_install()?;
        let cert_raw = match crate::pki::filesafe::read_regular_no_follow(
            &self.cert_path,
            crate::pki::filesafe::MAX_ROOT_PEM_BYTES,
        ) {
            Ok(raw) => raw,
            Err(err) => {
                if is_not_found(&self.cert_path) {
                    return Err(PkiError::CertNotFound);
                }
                return Err(err);
            }
        };
        let key_raw = match crate::pki::filesafe::read_regular_no_follow(
            &self.key_path,
            crate::pki::filesafe::MAX_ROOT_PEM_BYTES,
        ) {
            Ok(raw) => raw,
            Err(err) => {
                if is_not_found(&self.key_path) {
                    return Err(PkiError::CertKeyMismatch);
                }
                return Err(err);
            }
        };
        assert_pair_matches(&cert_raw, &key_raw)?;
        parse_leaf_pem(&cert_raw)
    }

    /// Stages both PEMs as sibling temps, journals the previous key, then
    /// renames key then cert. A failed cert rename restores the previous key.
    /// A crash between the two renames is repaired by `load` via the journal.
    pub fn write(&self, cert_pem: &[u8], key_pem: &[u8]) -> Result<(), PkiError> {
        self.recover_incomplete_install()?;
        if let Some(dir) = self.cert_path.parent() {
            fs::create_dir_all(dir).map_err(|e| PkiError::other(format!("mkdir cert dir: {e}")))?;
        }
        if let Some(dir) = self.key_path.parent() {
            fs::create_dir_all(dir).map_err(|e| PkiError::other(format!("mkdir key dir: {e}")))?;
        }
        crate::pki::filesafe::assert_trusted_dest(&self.cert_path)
            .map_err(|e| PkiError::other(format!("cert dest: {e}")))?;
        crate::pki::filesafe::assert_trusted_dest(&self.key_path)
            .map_err(|e| PkiError::other(format!("key dest: {e}")))?;
        let cert_tmp = crate::pki::filesafe::write_temp_nofollow(&self.cert_path, cert_pem, 0o444)?;
        let key_tmp =
            match crate::pki::filesafe::write_temp_nofollow(&self.key_path, key_pem, 0o400) {
                Ok(p) => p,
                Err(err) => {
                    let _ = crate::pki::filesafe::unlink_nofollow(&cert_tmp);
                    return Err(err);
                }
            };

        let prev = self.prev_path();
        let journal = self.journal_path();
        let had_old_key = fs::symlink_metadata(&self.key_path).is_ok_and(|m| m.is_file());
        if had_old_key {
            let old_key = crate::pki::filesafe::read_regular_no_follow(
                &self.key_path,
                crate::pki::filesafe::MAX_ROOT_PEM_BYTES,
            )?;
            if let Err(err) = crate::pki::filesafe::write_regular_nofollow(&prev, &old_key, 0o400) {
                let _ = crate::pki::filesafe::unlink_nofollow(&cert_tmp);
                let _ = crate::pki::filesafe::unlink_nofollow(&key_tmp);
                return Err(err);
            }
            if let Err(err) =
                crate::pki::filesafe::write_regular_nofollow(&journal, b"in_progress\n", 0o600)
            {
                let _ = crate::pki::filesafe::unlink_nofollow(&cert_tmp);
                let _ = crate::pki::filesafe::unlink_nofollow(&key_tmp);
                let _ = crate::pki::filesafe::unlink_nofollow(&prev);
                return Err(err);
            }
        }

        if let Err(err) = crate::pki::filesafe::rename_nofollow(&key_tmp, &self.key_path) {
            let _ = crate::pki::filesafe::unlink_nofollow(&cert_tmp);
            let _ = crate::pki::filesafe::unlink_nofollow(&key_tmp);
            if had_old_key {
                let _ = crate::pki::filesafe::unlink_nofollow(&journal);
                let _ = crate::pki::filesafe::unlink_nofollow(&prev);
            }
            return Err(err);
        }

        let fault = self.take_fault();
        if fault == 2 {
            return Err(PkiError::other("injected crash after key rename"));
        }

        let cert_rename = if fault == 1 {
            Err(PkiError::other("rename cert: injected failure"))
        } else {
            crate::pki::filesafe::rename_nofollow(&cert_tmp, &self.cert_path)
        };
        if let Err(err) = cert_rename {
            self.rollback_key(had_old_key, &prev, &journal);
            let _ = crate::pki::filesafe::unlink_nofollow(&cert_tmp);
            let _ = crate::pki::filesafe::unlink_nofollow(&key_tmp);
            return Err(err);
        }

        let _ = crate::pki::filesafe::unlink_nofollow(&journal);
        let _ = crate::pki::filesafe::unlink_nofollow(&prev);
        self.cleanup_enroll_temps();
        Ok(())
    }

    fn take_fault(&self) -> u8 {
        #[cfg(test)]
        {
            self.fault.swap(0, std::sync::atomic::Ordering::SeqCst)
        }
        #[cfg(not(test))]
        {
            let _ = &self.cert_path;
            0
        }
    }

    fn prev_path(&self) -> PathBuf {
        sibling_with_suffix(&self.key_path, ".pki-prev")
    }

    fn journal_path(&self) -> PathBuf {
        sibling_with_suffix(&self.key_path, ".pki-journal")
    }

    fn recover_incomplete_install(&self) -> Result<(), PkiError> {
        let journal = self.journal_path();
        let prev = self.prev_path();
        let journal_exists = match fs::symlink_metadata(&journal) {
            Ok(info) if info.file_type().is_symlink() => {
                return Err(PkiError::Symlink {
                    path: journal.display().to_string(),
                });
            }
            Ok(_) => true,
            Err(err) if err.kind() == std::io::ErrorKind::NotFound => false,
            Err(err) => {
                return Err(PkiError::other(format!(
                    "lstat {}: {err}",
                    journal.display()
                )));
            }
        };
        if journal_exists {
            if !self.pair_matches_on_disk() && !is_not_found(&prev) {
                crate::pki::filesafe::rename_nofollow(&prev, &self.key_path)?;
            }
            let _ = crate::pki::filesafe::unlink_nofollow(&journal);
            let _ = crate::pki::filesafe::unlink_nofollow(&prev);
            self.cleanup_enroll_temps();
            return Ok(());
        }
        if !is_not_found(&prev) {
            let _ = crate::pki::filesafe::unlink_nofollow(&prev);
        }
        Ok(())
    }

    fn pair_matches_on_disk(&self) -> bool {
        let Ok(cert) = crate::pki::filesafe::read_regular_no_follow(
            &self.cert_path,
            crate::pki::filesafe::MAX_ROOT_PEM_BYTES,
        ) else {
            return false;
        };
        let Ok(key) = crate::pki::filesafe::read_regular_no_follow(
            &self.key_path,
            crate::pki::filesafe::MAX_ROOT_PEM_BYTES,
        ) else {
            return false;
        };
        assert_pair_matches(&cert, &key).is_ok()
    }

    fn rollback_key(&self, had_old_key: bool, prev: &std::path::Path, journal: &std::path::Path) {
        if had_old_key {
            let _ = crate::pki::filesafe::rename_nofollow(prev, &self.key_path);
        } else {
            let _ = crate::pki::filesafe::unlink_nofollow(&self.key_path);
        }
        let _ = crate::pki::filesafe::unlink_nofollow(journal);
        let _ = crate::pki::filesafe::unlink_nofollow(prev);
    }

    fn cleanup_enroll_temps(&self) {
        for dir in [self.cert_path.parent(), self.key_path.parent()]
            .into_iter()
            .flatten()
        {
            let Ok(entries) = fs::read_dir(dir) else {
                continue;
            };
            for entry in entries.flatten() {
                let name = entry.file_name();
                let Some(name) = name.to_str() else {
                    continue;
                };
                if name.starts_with(".pki-enroll-") {
                    let _ = crate::pki::filesafe::unlink_nofollow(&entry.path());
                }
            }
        }
    }
}

fn is_not_found(path: &std::path::Path) -> bool {
    fs::symlink_metadata(path)
        .err()
        .is_some_and(|e| e.kind() == std::io::ErrorKind::NotFound)
}

fn sibling_with_suffix(path: &std::path::Path, suffix: &str) -> PathBuf {
    let mut name = path.file_name().unwrap_or_default().to_os_string();
    name.push(suffix);
    match path.parent().filter(|p| !p.as_os_str().is_empty()) {
        Some(dir) => dir.join(name),
        None => PathBuf::from(name),
    }
}

fn assert_pair_matches(cert_pem: &[u8], key_pem: &[u8]) -> Result<(), PkiError> {
    let _ = rustls::crypto::aws_lc_rs::default_provider().install_default();
    let certs = rustls_pemfile::certs(&mut &cert_pem[..])
        .collect::<Result<Vec<_>, _>>()
        .map_err(|_| PkiError::CertKeyMismatch)?;
    if certs.is_empty() {
        return Err(PkiError::CertKeyMismatch);
    }
    let key = rustls_pemfile::private_key(&mut &key_pem[..])
        .map_err(|_| PkiError::CertKeyMismatch)?
        .ok_or(PkiError::CertKeyMismatch)?;
    let signing = rustls::crypto::aws_lc_rs::sign::any_supported_type(&key)
        .map_err(|_| PkiError::CertKeyMismatch)?;
    let pair = rustls::sign::CertifiedKey::new(certs, signing);
    pair.keys_match().map_err(|_| PkiError::CertKeyMismatch)
}

pub(crate) fn parse_leaf_pem(raw: &[u8]) -> Result<LoadedCert, PkiError> {
    let pem = pem_first_cert(raw)?;
    parse_leaf_der(&pem)
}

pub(crate) fn parse_leaf_der(der: &[u8]) -> Result<LoadedCert, PkiError> {
    let (_, cert) = X509Certificate::from_der(der)
        .map_err(|e| PkiError::CertParseFailed(format!("no PEM/DER: {e}")))?;
    let cn = cert
        .subject()
        .iter_common_name()
        .next()
        .and_then(|n| n.as_str().ok())
        .unwrap_or("")
        .to_string();
    let sans = collect_sans(&cert);
    let not_before = asn1_to_system(cert.validity().not_before.timestamp())?;
    let not_after = asn1_to_system(cert.validity().not_after.timestamp())?;
    Ok(LoadedCert {
        not_before,
        not_after,
        common_name: cn,
        dns_sans: sans.dns,
        ip_sans: sans.ip,
        email_sans: sans.email,
        uri_sans: sans.uri,
        der: der.to_vec(),
    })
}

struct CollectedSans {
    dns: Vec<String>,
    ip: Vec<std::net::IpAddr>,
    email: Vec<String>,
    uri: Vec<String>,
}

fn collect_sans(cert: &X509Certificate<'_>) -> CollectedSans {
    let mut out = CollectedSans {
        dns: Vec::new(),
        ip: Vec::new(),
        email: Vec::new(),
        uri: Vec::new(),
    };
    let Some(ext) = cert.subject_alternative_name().ok().flatten() else {
        return out;
    };
    for n in &ext.value.general_names {
        match n {
            x509_parser::extensions::GeneralName::DNSName(d) => out.dns.push((*d).to_string()),
            x509_parser::extensions::GeneralName::IPAddress(b) => {
                if let Some(ip) = ip_from_octets(b) {
                    out.ip.push(ip);
                }
            }
            x509_parser::extensions::GeneralName::RFC822Name(e) => {
                out.email.push((*e).to_string());
            }
            x509_parser::extensions::GeneralName::URI(u) => out.uri.push((*u).to_string()),
            _ => {}
        }
    }
    out
}

fn ip_from_octets(b: &[u8]) -> Option<std::net::IpAddr> {
    match b.len() {
        4 => Some(std::net::IpAddr::V4(std::net::Ipv4Addr::new(
            b[0], b[1], b[2], b[3],
        ))),
        16 => {
            let mut a = [0u8; 16];
            a.copy_from_slice(b);
            Some(std::net::IpAddr::V6(std::net::Ipv6Addr::from(a)))
        }
        _ => None,
    }
}

pub(crate) fn pem_first_cert(raw: &[u8]) -> Result<Vec<u8>, PkiError> {
    let mut reader = raw;
    let certs = rustls_pemfile::certs(&mut reader)
        .collect::<Result<Vec<_>, _>>()
        .map_err(|e| PkiError::CertParseFailed(e.to_string()))?;
    certs
        .first()
        .map(|c| c.as_ref().to_vec())
        .ok_or_else(|| PkiError::CertParseFailed("no PEM block".into()))
}

pub(crate) fn pem_all_certs(raw: &[u8]) -> Result<Vec<Vec<u8>>, PkiError> {
    let mut reader = raw;
    rustls_pemfile::certs(&mut reader)
        .map(|c| {
            c.map(|d| d.as_ref().to_vec())
                .map_err(|e| PkiError::CertParseFailed(e.to_string()))
        })
        .collect()
}

fn asn1_to_system(secs: i64) -> Result<SystemTime, PkiError> {
    if secs < 0 {
        return Err(PkiError::CertParseFailed("negative cert timestamp".into()));
    }
    Ok(UNIX_EPOCH + Duration::from_secs(secs.cast_unsigned()))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::pki::test_util::self_signed_pem;
    use std::os::unix::fs::PermissionsExt;

    #[test]
    fn write_and_load() {
        let dir = tempfile::TempDir::new().unwrap();
        let cf = CertFile::new(
            dir.path().join("svc-cert.pem"),
            dir.path().join("svc-key.pem"),
        );
        let now = SystemTime::now();
        let (cert, key) = self_signed_pem(
            "recap-worker",
            now - Duration::from_secs(60),
            now + Duration::from_secs(3600),
        );
        cf.write(&cert, &key).unwrap();
        let cert_mode = fs::metadata(&cf.cert_path).unwrap().permissions().mode() & 0o777;
        let key_mode = fs::metadata(&cf.key_path).unwrap().permissions().mode() & 0o777;
        assert_eq!(cert_mode, 0o444);
        assert_eq!(key_mode, 0o400);
        let got = cf.load().unwrap();
        assert_eq!(got.common_name, "recap-worker");
    }

    #[test]
    fn load_missing() {
        let dir = tempfile::TempDir::new().unwrap();
        let cf = CertFile::new(dir.path().join("absent.pem"), dir.path().join("absent.key"));
        assert!(matches!(cf.load(), Err(PkiError::CertNotFound)));
    }

    #[test]
    fn write_leaves_no_temp_on_success() {
        let dir = tempfile::TempDir::new().unwrap();
        let cf = CertFile::new(
            dir.path().join("svc-cert.pem"),
            dir.path().join("svc-key.pem"),
        );
        let now = SystemTime::now();
        let (cert, key) = self_signed_pem("recap-worker", now, now + Duration::from_secs(3600));
        cf.write(&cert, &key).unwrap();
        for e in fs::read_dir(dir.path()).unwrap() {
            let name = e.unwrap().file_name().into_string().unwrap();
            assert!(
                !name.starts_with(".pki-enroll-"),
                "temp left behind: {name}"
            );
            assert!(
                !name.ends_with(".pki-prev") && !name.ends_with(".pki-journal"),
                "install journal left behind: {name}"
            );
        }
    }

    #[test]
    fn load_classifies_cert_key_mismatch_corrupt() {
        let dir = tempfile::TempDir::new().unwrap();
        let cert_path = dir.path().join("svc-cert.pem");
        let key_path = dir.path().join("svc-key.pem");
        let cf = CertFile::new(&cert_path, &key_path);
        let now = SystemTime::now();
        let (cert_a, key_a) = self_signed_pem("recap-worker", now, now + Duration::from_secs(3600));
        let (_cert_b, key_b) =
            self_signed_pem("recap-worker", now, now + Duration::from_secs(3600));
        cf.write(&cert_a, &key_a).unwrap();
        let mut perms = fs::metadata(&key_path).unwrap().permissions();
        perms.set_mode(0o600);
        fs::set_permissions(&key_path, perms).unwrap();
        fs::write(&key_path, &key_b).unwrap();
        let err = cf.load().expect_err("mismatched pair must not load");
        assert!(
            matches!(err, PkiError::CertKeyMismatch),
            "mismatch must be CertKeyMismatch, got {err}"
        );
    }

    #[test]
    fn write_rolls_back_key_when_cert_rename_fails() {
        let dir = tempfile::TempDir::new().unwrap();
        let cert_path = dir.path().join("svc-cert.pem");
        let key_path = dir.path().join("svc-key.pem");
        let cf = CertFile::new(&cert_path, &key_path);
        let now = SystemTime::now();
        let (cert_a, key_a) = self_signed_pem("old-cn", now, now + Duration::from_secs(3600));
        let (cert_b, key_b) = self_signed_pem("new-cn", now, now + Duration::from_secs(3600));
        cf.write(&cert_a, &key_a).unwrap();
        cf.inject_fault(InstallFault::FailCertRename);
        cf.write(&cert_b, &key_b)
            .expect_err("second rename must fail");
        assert_eq!(
            fs::read(&key_path).unwrap(),
            key_a,
            "old key must be restored when cert rename fails"
        );
        assert_eq!(fs::read(&cert_path).unwrap(), cert_a);
        let got = cf.load().expect("rolled-back pair must still load");
        assert_eq!(got.common_name, "old-cn");
        assert!(!key_path.with_file_name("svc-key.pem.pki-prev").exists());
        assert!(!key_path.with_file_name("svc-key.pem.pki-journal").exists());
    }

    #[test]
    fn write_crash_after_key_rename_recovers_on_restart_load() {
        let dir = tempfile::TempDir::new().unwrap();
        let cert_path = dir.path().join("svc-cert.pem");
        let key_path = dir.path().join("svc-key.pem");
        let cf = CertFile::new(&cert_path, &key_path);
        let now = SystemTime::now();
        let (cert_a, key_a) = self_signed_pem("old-cn", now, now + Duration::from_secs(3600));
        let (cert_b, key_b) = self_signed_pem("new-cn", now, now + Duration::from_secs(3600));
        cf.write(&cert_a, &key_a).unwrap();
        cf.inject_fault(InstallFault::CrashAfterKeyRename);
        cf.write(&cert_b, &key_b)
            .expect_err("simulated crash after key rename");
        assert_eq!(
            fs::read(&key_path).unwrap(),
            key_b,
            "crash leaves the new key in place"
        );
        assert_eq!(fs::read(&cert_path).unwrap(), cert_a);

        let restarted = CertFile::new(&cert_path, &key_path);
        let got = restarted
            .load()
            .expect("restart load must recover the old key");
        assert_eq!(got.common_name, "old-cn");
        assert_eq!(fs::read(&key_path).unwrap(), key_a);
        assert_eq!(fs::read(&cert_path).unwrap(), cert_a);
        assert!(!key_path.with_file_name("svc-key.pem.pki-prev").exists());
        assert!(!key_path.with_file_name("svc-key.pem.pki-journal").exists());
    }
}
