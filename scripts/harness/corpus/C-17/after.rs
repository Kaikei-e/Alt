use regex::Regex;
use std::borrow::Cow;
use std::sync::LazyLock;

const REDACTED: &str = "[REDACTED]";

static MASK_RE: LazyLock<Option<Regex>> =
    LazyLock::new(|| Regex::new(r"Bearer\s+[A-Za-z0-9._-]+").ok());

pub fn redact(line: &str) -> Cow<'_, str> {
    if let Some(re) = MASK_RE.as_ref() {
        re.replace_all(line, REDACTED)
    } else {
        Cow::Borrowed(line)
    }
}
