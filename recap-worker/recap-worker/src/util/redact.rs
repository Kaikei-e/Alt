#![allow(dead_code)]

pub(crate) fn redact(input: &str) -> String {
    if input.len() <= 4 {
        "****".to_string()
    } else if let Some(prefix) = input.get(..4) {
        format!("{prefix}***")
    } else {
        let prefix = input
            .char_indices()
            .map(|(idx, _)| idx)
            .take_while(|&idx| idx < 4)
            .filter_map(|idx| input.get(..idx))
            .last()
            .unwrap_or("");
        format!("{prefix}***")
    }
}

#[cfg(test)]
mod tests {
    use super::redact;

    #[test]
    fn redact_preserves_ascii_behavior_and_handles_multibyte_safely() {
        assert_eq!(redact("abc"), "****");
        assert_eq!(redact("abcd"), "****");
        assert_eq!(redact("abcdef"), "abcd***");
        assert_eq!(redact("あいう"), "あ***");
        assert_eq!(redact("abあいう"), "ab***");
        assert_eq!(redact("aあい"), "aあ***");
    }
}
