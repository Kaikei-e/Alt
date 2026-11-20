use super::log_entry::EnrichedLogEntry;
use regex::{Captures, Regex};
use std::borrow::Cow;

const REDACTED: &str = "[REDACTED]";

/// Compiled regex patterns used to strip credentials from log entries before
/// they leave the host. Constructed once at startup via [`compile_patterns`]
/// so any invalid pattern fails startup instead of silently disabling
/// redaction.
#[derive(Debug, Clone)]
pub struct RedactionPatterns {
    query_param: Regex,
    bare_key_value: Regex,
    cookie_header: Regex,
    token_header: Regex,
    json_container_key: Regex,
    json_string_pair: Regex,
}

/// Compile every secret-redaction regex once at startup. Any pattern that
/// fails to compile is propagated as an error rather than skipped.
pub fn compile_patterns() -> Result<RedactionPatterns, regex::Error> {
    Ok(RedactionPatterns {
        query_param: Regex::new(r#"([?&;])([A-Za-z0-9_.\-]+)=([^&\s"'#]*)"#)?,
        bare_key_value: Regex::new(r#"(^|[\s,])([A-Za-z0-9_.\-]+)=([^\s,&;"'#]+)"#)?,
        // Cookie headers hold `name=value; name=value` pairs, so the value runs to the
        // end of the line or the next unescaped quote.
        cookie_header: Regex::new(r#"(?i)\b(set-cookie|cookie)(\s*[:=]\s*)(?:[^\r\n"\\]|\\.)*"#)?,
        // Single-credential headers: an optional auth scheme plus one token.
        token_header: Regex::new(
            r#"(?i)\b((?:proxy-)?authorization\s*[:=]\s*(?:(?:bearer|basic|digest|token|negotiate)\s+)?|x-[a-z0-9-]*-(?:token|secret|key)\s*[:=]\s*)(?:[^\s,;"\\]|\\.)+"#,
        )?,
        json_container_key: Regex::new(r#""([A-Za-z0-9_.\-]+)"\s*:\s*[\[{]"#)?,
        json_string_pair: Regex::new(r#""([A-Za-z0-9_.\-]+)"(\s*:\s*)"(?:[^"\\]|\\.)*""#)?,
    })
}

fn is_sensitive_name(name: &str) -> bool {
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
    let n = name.to_ascii_lowercase().replace('-', "_");
    EXACT.contains(&n.as_str())
        || SUFFIXES.iter().any(|s| n.ends_with(s))
        || (n.starts_with("x_")
            && (n.contains("token") || n.contains("secret") || n.ends_with("_key")))
}

fn is_sensitive_query_name(name: &str) -> bool {
    let n = name.to_ascii_lowercase();
    is_sensitive_name(&n)
        || matches!(
            n.as_str(),
            "auth" | "key" | "sig" | "signature" | "code" | "sid"
        )
}

/// Index just past the bracket pair opened at `open`; the end of the text when unbalanced.
fn container_end(text: &str, open: usize) -> usize {
    let bytes = text.as_bytes();
    let mut depth = 0usize;
    let mut in_string = false;
    let mut i = open;
    while i < bytes.len() {
        match bytes[i] {
            b'\\' if in_string => i += 1,
            b'"' => in_string = !in_string,
            b'[' | b'{' if !in_string => depth += 1,
            b']' | b'}' if !in_string => {
                depth -= 1;
                if depth == 0 {
                    return i + 1;
                }
            }
            _ => {}
        }
        i += 1;
    }
    bytes.len()
}

fn redact_json_containers<'a>(input: &'a str, json_container_key: &Regex) -> Cow<'a, str> {
    let mut out = String::new();
    let mut copied = 0;
    let mut pos = 0;
    while let Some(c) = json_container_key.captures_at(input, pos) {
        let Some(whole) = c.get(0) else {
            break;
        };
        let Some(key) = c.get(1).map(|m| m.as_str()) else {
            pos = whole.end();
            continue;
        };
        if is_sensitive_name(key) {
            let end = container_end(input, whole.end().saturating_sub(1));
            if let Some(prefix) = input.get(copied..whole.start()) {
                out.push_str(prefix);
            }
            out.push('"');
            out.push_str(key);
            out.push_str("\":\"");
            out.push_str(REDACTED);
            out.push('"');
            copied = end;
            pos = end;
        } else {
            pos = whole.end();
        }
    }
    if copied == 0 {
        return Cow::Borrowed(input);
    }
    if let Some(tail) = input.get(copied..) {
        out.push_str(tail);
    }
    Cow::Owned(out)
}

fn redact_text<'a>(input: &'a str, patterns: &RedactionPatterns) -> Cow<'a, str> {
    let mut text = Cow::Borrowed(input);
    if text.contains('=') {
        let replaced = patterns.query_param.replace_all(&text, |c: &Captures| {
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
    if let Cow::Owned(s) = patterns
        .cookie_header
        .replace_all(&text, format!("${{1}}${{2}}{REDACTED}"))
    {
        text = Cow::Owned(s);
    }
    if let Cow::Owned(s) = patterns
        .token_header
        .replace_all(&text, format!("${{1}}{REDACTED}"))
    {
        text = Cow::Owned(s);
    }
    if text.contains('=') {
        let replaced = patterns.bare_key_value.replace_all(&text, |c: &Captures| {
            if is_sensitive_name(&c[2]) {
                format!("{}{}={REDACTED}", &c[1], &c[2])
            } else {
                c[0].to_string()
            }
        });
        if let Cow::Owned(s) = replaced {
            text = Cow::Owned(s);
        }
    }
    if text.contains('"') {
        if let Cow::Owned(s) = redact_json_containers(&text, &patterns.json_container_key) {
            text = Cow::Owned(s);
        }
        let replaced = patterns
            .json_string_pair
            .replace_all(&text, |c: &Captures| {
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
    pub fn redact_sensitive(&mut self, patterns: &RedactionPatterns) {
        self.message = redact_text(&self.message, patterns).into_owned();
        if let Some(path) = &self.path {
            self.path = Some(redact_text(path, patterns).into_owned());
        }
        for (key, value) in &mut self.fields {
            if is_sensitive_name(key) {
                *value = REDACTED.to_string();
            } else {
                *value = redact_text(value, patterns).into_owned();
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

    fn patterns() -> RedactionPatterns {
        compile_patterns().expect("built-in redaction regex literals must compile")
    }

    #[test]
    fn compile_patterns_succeeds() {
        compile_patterns().expect("built-in redaction regex literals must compile");
    }

    #[test]
    fn sensitive_query_params_are_redacted_in_path_and_message() {
        let p = patterns();
        let mut e = entry(
            r#"1.2.3.4 - - "GET /api/feeds?access_token=abc123&page=2 HTTP/1.1" 200 5"#,
            Some("/api/feeds?access_token=abc123&page=2&api_key=k1"),
        );
        e.redact_sensitive(&p);
        assert_eq!(
            e.path.as_deref(),
            Some("/api/feeds?access_token=[REDACTED]&page=2&api_key=[REDACTED]")
        );
        assert!(!e.message.contains("abc123"));
        assert!(e.message.contains("page=2"));
    }

    #[test]
    fn auth_style_headers_in_message_text_are_redacted() {
        let p = patterns();
        for (input, secret) in [
            ("Authorization: Bearer abc.def.ghi", "abc.def.ghi"),
            ("cookie: sid=s3cr3t; theme=dark", "s3cr3t"),
            ("Set-Cookie: sid=s3cr3t; Path=/; HttpOnly", "s3cr3t"),
            ("X-Alt-Backend-Token: tok-999", "tok-999"),
        ] {
            let mut e = entry(input, None);
            e.redact_sensitive(&p);
            assert!(!e.message.contains(secret), "leaked in {:?}", e.message);
            assert!(e.message.contains("[REDACTED]"));
        }
    }

    #[test]
    fn json_string_values_for_sensitive_keys_are_redacted() {
        let p = patterns();
        let mut e = entry(
            r#"{"msg":"req","authorization":"Bearer abc","client_secret":"zzz","status":"200"}"#,
            None,
        );
        e.redact_sensitive(&p);
        assert!(!e.message.contains("abc"));
        assert!(!e.message.contains("zzz"));
        assert!(e.message.contains(r#""status":"200""#));
    }

    #[test]
    fn sensitive_field_names_and_embedded_urls_are_redacted() {
        let p = patterns();
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
        e.redact_sensitive(&p);
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
        let p = patterns();
        let mut e = entry(
            r"GET /health?author=bob&page=3 HTTP/1.1 200",
            Some("/health?author=bob&page=3"),
        );
        let before = e.clone();
        e.redact_sensitive(&p);
        assert_eq!(e.message, before.message);
        assert_eq!(e.path, before.path);
    }

    #[test]
    fn json_array_and_object_values_for_sensitive_keys_are_redacted() {
        let p = patterns();
        let mut e = entry(
            r#"{"authorization":["Bearer abc","Basic def"],"cookie":{"sid":"ghi"},"ok":["keep"]}"#,
            None,
        );
        e.redact_sensitive(&p);
        for secret in ["abc", "def", "ghi"] {
            assert!(
                !e.message.contains(secret),
                "leaked {secret}: {}",
                e.message
            );
        }
        assert!(e.message.contains(r#""ok":["keep"]"#));
    }

    #[test]
    fn escaped_quotes_do_not_end_a_redacted_value_early() {
        let p = patterns();
        let mut e = entry(
            r#"{"client_secret":"abc\"TAIL-SECRET","status":"200"}"#,
            None,
        );
        e.redact_sensitive(&p);
        assert!(!e.message.contains("TAIL-SECRET"), "{}", e.message);
        assert!(e.message.contains(r#""status":"200""#));

        let mut h = entry(
            r#"{"msg":"Authorization: Bearer abc\"TAIL-SECRET","n":"1"}"#,
            None,
        );
        h.redact_sensitive(&p);
        assert!(!h.message.contains("TAIL-SECRET"), "{}", h.message);
    }

    #[test]
    fn plain_key_value_text_is_redacted_without_a_query_delimiter() {
        let p = patterns();
        for (input, secret) in [
            ("password=hunter2 user=bob", "hunter2"),
            ("login failed, passwd=hunter2, retrying", "hunter2"),
            ("secret=s3 token=t0k", "s3"),
            ("api_key=k-1 other=1", "k-1"),
            ("got access_token=at-1 refresh_token=rt-1", "at-1"),
            ("got access_token=at-1 refresh_token=rt-1", "rt-1"),
            ("authorization=Bearer abc.def", "abc.def"),
        ] {
            let mut e = entry(input, None);
            e.redact_sensitive(&p);
            assert!(!e.message.contains(secret), "leaked in {:?}", e.message);
        }
        let mut e = entry("password=hunter2 user=bob", None);
        e.redact_sensitive(&p);
        assert!(e.message.contains("user=bob"));
    }

    #[test]
    fn x_api_key_header_is_redacted() {
        let p = patterns();
        let mut e = entry("X-Api-Key: key-123 accept: */*", None);
        e.redact_sensitive(&p);
        assert!(!e.message.contains("key-123"));
        assert!(e.message.contains("accept: */*"));
        let mut j = entry(r#"{"x-api-key":"key-123"}"#, None);
        j.redact_sensitive(&p);
        assert!(!j.message.contains("key-123"));
    }

    #[test]
    fn single_token_header_values_stop_at_the_value_boundary() {
        let p = patterns();
        let mut e = entry("X-Alt-Backend-Token: tok-9 status=200", None);
        e.redact_sensitive(&p);
        assert!(!e.message.contains("tok-9"));
        assert!(e.message.contains("status=200"), "{}", e.message);
        let mut a = entry("Authorization: Bearer abc.def next=1", None);
        a.redact_sensitive(&p);
        assert!(!a.message.contains("abc.def"));
        assert!(a.message.contains("next=1"), "{}", a.message);
    }

    #[test]
    fn ordinary_numeric_and_named_fields_stay_readable() {
        let p = patterns();
        let mut e = entry(
            r#"{"max_tokens":512,"prompt_tokens":10,"msg":"max_tokens=512 prompt_tokens=10 author=bob"}"#,
            None,
        );
        e.fields.insert("max_tokens".to_string(), "512".to_string());
        e.fields
            .insert("prompt_tokens".to_string(), "10".to_string());
        e.fields
            .insert("completion_tokens".to_string(), "7".to_string());
        e.fields.insert("author".to_string(), "bob".to_string());
        let before = e.clone();
        e.redact_sensitive(&p);
        assert_eq!(e.message, before.message);
        assert_eq!(e.fields, before.fields);
    }

    #[test]
    fn next_page_token_is_knowingly_redacted() {
        let p = patterns();
        let mut e = entry("ok", None);
        e.fields
            .insert("next_page_token".to_string(), "p2".to_string());
        e.redact_sensitive(&p);
        assert_eq!(e.fields["next_page_token"], "[REDACTED]");
    }
}
