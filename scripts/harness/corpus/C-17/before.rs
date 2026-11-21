use regex::Regex;
use std::borrow::Cow;

const REDACTED: &str = "[REDACTED]";

pub struct MaskSet {
    bearer_header: Regex,
}

impl MaskSet {
    pub fn compile() -> Result<Self, regex::Error> {
        Ok(Self {
            bearer_header: Regex::new(r"Bearer\s+[A-Za-z0-9._-]+")?,
        })
    }

    pub fn redact<'a>(&self, line: &'a str) -> Cow<'a, str> {
        self.bearer_header.replace_all(line, REDACTED)
    }
}
