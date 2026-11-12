use super::log_entry::EnrichedLogEntry;
use regex::{Captures, Regex};
use std::borrow::Cow;
use std::sync::LazyLock;

const REDACTED: &str = "[REDACTED]";

static QUERY_PARAM: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r#"([?&;])([A-Za-z0-9_.\-]+)=([^&\s"'#]*)"#).unwrap());

static HEADER_TEXT: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(
        r#"(?i)\b((?:proxy-)?authorization|set-cookie|cookie|x-[a-z0-9-]*-(?:token|secret))(\s*[:=]\s*)[^\r\n"]*"#,
    )
    .unwrap()
});

static JSON_STRING_PAIR: LazyLock<Regex> =
    LazyLock::new(|| Regex::new(r#""([A-Za-z0-9_.\-]+)"(\s*:\s*)"[^"]*""#).unwrap());

fn is_sensitive_name(name: &str) -> bool {
    let n = name.to_ascii_lowercase().replace('-', "_");
    const EXACT: &[&str] = &[
        "authorization",
        "proxy_authorization",
        "cookie",
        "set_cookie",
        "token",
        "secret",
        "password",
        "passwd",
        "pwd",
        "apikey",
        "api_key",
        "session",
        "session_id",
        "credential",
        "credentials",
    ];
    const SUFFIXES: &[&str] = &[
        "_token",
        "_secret",
        "_password",
        "_passwd",
        "_api_key",
        "_apikey",
        "_credential",
    ];
    EXACT.contains(&n.as_str())
        || SUFFIXES.iter().any(|s| n.ends_with(s))
        || (n.starts_with("x_") && (n.contains("token") || n.contains("secret")))
}

fn is_sensitive_query_name(name: &str) -> bool {
    let n = name.to_ascii_lowercase();
    is_sensitive_name(&n)
        || matches!(
            n.as_str(),
            "auth" | "key" | "sig" | "signature" | "code" | "sid"
        )
}

fn redact_text(input: &str) -> Cow<'_, str> {
    let mut text = Cow::Borrowed(input);
    if text.contains('=') {
        let replaced = QUERY_PARAM.replace_all(&text, |c: &Captures| {
            if is_sensitive_query_name(&c[2]) {
                format!("{}{}={REDACTED}", &c[1], &c[2])
            } else {
                c[0].to_string()
            }
        });
        if let Cow::Owned(s) = replaced {
            text = Cow::Owned(s);
        }
    }
    if let Cow::Owned(s) = HEADER_TEXT.replace_all(&text, format!("${{1}}${{2}}{REDACTED}")) {
        text = Cow::Owned(s);
    }
    if text.contains('"') {
        let replaced = JSON_STRING_PAIR.replace_all(&text, |c: &Captures| {
            if is_sensitive_name(&c[1]) {
                format!("\"{}\"{}\"{REDACTED}\"", &c[1], &c[2])
            } else {
                c[0].to_string()
            }
        });
        if let Cow::Owned(s) = replaced {
            text = Cow::Owned(s);
        }
    }
    text
}

impl EnrichedLogEntry {
    /// Masks credentials that HTTP access logs and application logs can carry:
    /// sensitive query parameters, auth/cookie header text, and sensitive
    /// JSON/field names. Runs before the entry leaves the forwarder.
    pub fn redact_sensitive(&mut self) {
        self.message = redact_text(&self.message).into_owned();
        if let Some(path) = &self.path {
            self.path = Some(redact_text(path).into_owned());
        }
        for (key, value) in &mut self.fields {
            if is_sensitive_name(key) {
                *value = REDACTED.to_string();
            } else {
                *value = redact_text(value).into_owned();
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::collections::HashMap;

    fn entry(message: &str, path: Option<&str>) -> EnrichedLogEntry {
        EnrichedLogEntry {
            service_type: "nginx".to_string(),
            log_type: "access".to_string(),
            message: message.to_string(),
            level: None,
            timestamp: "2026-01-01T00:00:00Z".to_string(),
            stream: "stdout".to_string(),
            method: None,
            path: path.map(str::to_string),
            status_code: None,
            response_size: None,
            ip_address: None,
            user_agent: None,
            container_id: "c1".to_string(),
            service_name: "nginx".to_string(),
            service_group: None,
            trace_id: None,
            span_id: None,
            fields: HashMap::new(),
        }
    }

    #[test]
    fn sensitive_query_params_are_redacted_in_path_and_message() {
        let mut e = entry(
            r#"1.2.3.4 - - "GET /api/feeds?access_token=abc123&page=2 HTTP/1.1" 200 5"#,
            Some("/api/feeds?access_token=abc123&page=2&api_key=k1"),
        );
        e.redact_sensitive();
        assert_eq!(
            e.path.as_deref(),
            Some("/api/feeds?access_token=[REDACTED]&page=2&api_key=[REDACTED]")
        );
        assert!(!e.message.contains("abc123"));
        assert!(e.message.contains("page=2"));
    }

    #[test]
    fn auth_style_headers_in_message_text_are_redacted() {
        for (input, secret) in [
            ("Authorization: Bearer abc.def.ghi", "abc.def.ghi"),
            ("cookie: sid=s3cr3t; theme=dark", "s3cr3t"),
            ("Set-Cookie: sid=s3cr3t; Path=/; HttpOnly", "s3cr3t"),
            ("X-Alt-Backend-Token: tok-999", "tok-999"),
        ] {
            let mut e = entry(input, None);
            e.redact_sensitive();
            assert!(!e.message.contains(secret), "leaked in {:?}", e.message);
            assert!(e.message.contains("[REDACTED]"));
        }
    }

    #[test]
    fn json_string_values_for_sensitive_keys_are_redacted() {
        let mut e = entry(
            r#"{"msg":"req","authorization":"Bearer abc","client_secret":"zzz","status":"200"}"#,
            None,
        );
        e.redact_sensitive();
        assert!(!e.message.contains("abc"));
        assert!(!e.message.contains("zzz"));
        assert!(e.message.contains(r#""status":"200""#));
    }

    #[test]
    fn sensitive_field_names_and_embedded_urls_are_redacted() {
        let mut e = entry("ok", None);
        e.fields
            .insert("x-alt-backend-token".to_string(), "tok".to_string());
        e.fields.insert("Cookie".to_string(), "sid=1".to_string());
        e.fields.insert(
            "url".to_string(),
            "https://example.test/cb?token=tok2&x=1".to_string(),
        );
        e.fields.insert("max_tokens".to_string(), "512".to_string());
        e.fields.insert("user".to_string(), "alice".to_string());
        e.redact_sensitive();
        assert_eq!(e.fields["x-alt-backend-token"], "[REDACTED]");
        assert_eq!(e.fields["Cookie"], "[REDACTED]");
        assert_eq!(
            e.fields["url"],
            "https://example.test/cb?token=[REDACTED]&x=1"
        );
        assert_eq!(e.fields["max_tokens"], "512");
        assert_eq!(e.fields["user"], "alice");
    }

    #[test]
    fn benign_entries_are_unchanged() {
        let mut e = entry(
            r#"GET /health?author=bob&page=3 HTTP/1.1 200"#,
            Some("/health?author=bob&page=3"),
        );
        let before = e.clone();
        e.redact_sensitive();
        assert_eq!(e.message, before.message);
        assert_eq!(e.path, before.path);
    }
}
