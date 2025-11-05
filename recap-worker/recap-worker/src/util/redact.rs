#![allow(dead_code)]

pub(crate) fn redact(input: &str) -> String {
    if input.len() <= 4 {
        "****".to_string()
    } else {
        format!("{}***", &input[..4])
    }
}
