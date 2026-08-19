use std::time::{Duration, SystemTime, UNIX_EPOCH};

use rcgen::{CertificateParams, DistinguishedName, DnType, KeyPair, SanType};
use time::OffsetDateTime;

/// Shared test helpers for the in-process PKI module.
#[cfg(test)]
pub(crate) fn self_signed_pem(
    cn: &str,
    not_before: SystemTime,
    not_after: SystemTime,
) -> (Vec<u8>, Vec<u8>) {
    let mut params = CertificateParams::new(vec![cn.to_string()]).expect("params");
    params.distinguished_name = DistinguishedName::new();
    params.distinguished_name.push(DnType::CommonName, cn);
    params.not_before = offset(not_before);
    params.not_after = offset(not_after);
    params.subject_alt_names = vec![SanType::DnsName(cn.try_into().expect("dns"))];
    let key = KeyPair::generate().expect("key");
    let cert = params.self_signed(&key).expect("self-sign");
    (cert.pem().into_bytes(), key.serialize_pem().into_bytes())
}

#[cfg(test)]
fn offset(ts: SystemTime) -> OffsetDateTime {
    let secs = ts
        .duration_since(UNIX_EPOCH)
        .unwrap_or(Duration::ZERO)
        .as_secs() as i64;
    OffsetDateTime::from_unix_timestamp(secs).unwrap_or(OffsetDateTime::UNIX_EPOCH)
}
