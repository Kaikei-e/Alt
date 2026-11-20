use anyhow::{Context, Result};
use std::sync::Arc;
use tokenizers::Tokenizer;
use tracing::{info, warn};

/// トークン数を計算するためのカウンタ。
/// Gemma 3 (google/gemma-3-4b-it) のトークナイザーを使用する。
#[derive(Debug, Clone)]
pub(crate) struct TokenCounter {
    tokenizer: Option<Arc<Tokenizer>>,
}

impl TokenCounter {
    /// 新しいTokenCounterを作成する。
    /// HuggingFaceからトークナイザーをダウンロードする。
    pub(crate) fn new() -> Result<Self> {
        info!("Initializing TokenCounter with google/gemma-3-4b-it tokenizer...");

        let token = resolve_hf_token()?;
        let tokenizer = load_gemma_tokenizer(token)?;

        Ok(Self {
            tokenizer: Some(Arc::new(tokenizer)),
        })
    }

    /// テスト用のダミーTokenCounterを作成する（トークナイザーなし、文字数カウントのみ）。
    /// 本番環境でも初期化に失敗した場合のフォールバックとして使用可能。
    pub(crate) fn dummy() -> Self {
        Self { tokenizer: None }
    }

    /// テキストのトークン数を計算する。
    pub(crate) fn count_tokens(&self, text: &str) -> usize {
        if let Some(tokenizer) = &self.tokenizer {
            match tokenizer.encode(text, false) {
                Ok(encoding) => encoding.len(),
                Err(e) => {
                    warn!("Failed to encode text for token counting: {}", e);
                    text.chars().count()
                }
            }
        } else {
            // トークナイザーがない場合は文字数を使用
            text.chars().count()
        }
    }
}

fn resolve_hf_token() -> Result<String> {
    // Hugging Face トークンを読み込む
    if let Ok(token) = std::env::var("HF_TOKEN") {
        info!("Using Hugging Face token from HF_TOKEN environment variable");
        return Ok(token);
    }

    let token_path = std::env::var("HUGGING_FACE_TOKEN_PATH").context(
        "HUGGING_FACE_TOKEN_PATH environment variable is not set (HF_TOKEN also not set)",
    )?;

    let token = std::fs::read_to_string(&token_path)
        .with_context(|| format!("Failed to read Hugging Face token from {}", token_path))?
        .trim()
        .to_string();

    info!("Loaded Hugging Face token from {}", token_path);
    Ok(token)
}

fn load_gemma_tokenizer(token: String) -> Result<Tokenizer> {
    // Gemma 3のトークナイザーをロード
    // 注意: 初回実行時にダウンロードが発生するため、インターネット接続が必要
    // FromPretrainedParameters を使用してトークンを明示的に設定
    // 注意: Gemma 3モデルは利用条件への同意が必要です
    let token_preview = preview_token_for_log(&token);
    info!("Using token: {}", token_preview);

    // `FromPretrainedParameters.token` is threaded straight into hf-hub's
    // `ApiBuilder::with_token`, so there's no need to also mutate the
    // process-wide `HF_TOKEN` env var at runtime (unsound alongside
    // concurrent env reads, and forbidden outside tests per DECREE §1).
    let params = tokenizers::FromPretrainedParameters {
        token: Some(token),
        ..Default::default()
    };

    Tokenizer::from_pretrained("google/gemma-3-4b-it", Some(params))
        .map_err(|e| anyhow::anyhow!("Failed to load tokenizer: {}. Note: Gemma 3 models require accepting the terms of use on Hugging Face.", e))
}

/// Build a log-safe token preview without panicking on non-ASCII byte boundaries.
fn preview_token_for_log(token: &str) -> String {
    let char_count = token.chars().count();
    if char_count <= 10 {
        return "***".to_string();
    }
    let prefix: String = token.chars().take(4).collect();
    let suffix: String = token
        .chars()
        .rev()
        .take(4)
        .collect::<String>()
        .chars()
        .rev()
        .collect();
    format!("{prefix}...{suffix}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn preview_token_for_log_handles_multibyte_chars() {
        // Non-ASCII token must not panic on byte slicing.
        let preview = preview_token_for_log("αβγδεζηθικλμνξοπ");
        assert!(preview.contains("..."), "{preview}");
        assert!(!preview.contains('\u{fffd}'));
    }

    #[test]
    fn preview_token_for_log_masks_short_tokens() {
        assert_eq!(preview_token_for_log("short"), "***");
    }

    #[test]
    fn new_fails_when_neither_hf_token_nor_token_path_is_set() {
        use std::sync::{Arc, Mutex};

        #[derive(Clone, Default)]
        struct BufWriter(Arc<Mutex<Vec<u8>>>);
        impl std::io::Write for BufWriter {
            fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
                self.0.lock().unwrap().extend_from_slice(buf);
                Ok(buf.len())
            }
            fn flush(&mut self) -> std::io::Result<()> {
                Ok(())
            }
        }

        let _lock = crate::config::ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);

        let buf = BufWriter::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_max_level(tracing::Level::INFO)
            .without_time()
            .with_writer(move || writer.clone())
            .finish();

        let err = tracing::subscriber::with_default(subscriber, || {
            temp_env::with_vars(
                [
                    ("HF_TOKEN", None::<&str>),
                    ("HUGGING_FACE_TOKEN_PATH", None::<&str>),
                ],
                || TokenCounter::new().expect_err("missing env vars must fail"),
            )
        });

        assert!(
            err.to_string()
                .contains("HUGGING_FACE_TOKEN_PATH environment variable is not set")
        );
        let logs = String::from_utf8(buf.0.lock().unwrap().clone()).expect("utf8");
        assert!(logs.contains("Initializing TokenCounter with google/gemma-3-4b-it tokenizer..."));
    }

    #[test]
    fn new_fails_when_token_path_file_does_not_exist() {
        let _lock = crate::config::ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);

        let err = temp_env::with_vars(
            [
                ("HF_TOKEN", None::<&str>),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/nonexistent/recap-worker-hf-token"),
                ),
            ],
            || TokenCounter::new().expect_err("missing token file must fail"),
        );

        assert!(
            err.to_string().contains(
                "Failed to read Hugging Face token from /nonexistent/recap-worker-hf-token"
            )
        );
    }

    #[test]
    fn dummy_fallback_counts_unicode_characters() {
        let counter = TokenCounter::dummy();
        assert_eq!(counter.count_tokens("hello世界"), 7);
    }

    #[test]
    #[ignore = "requires network access to download tokenizer model"]
    fn test_token_counter_initialization() {
        let counter = TokenCounter::new().expect("Should initialize tokenizer");
        let text = "こんにちは、世界！";
        let count = counter.count_tokens(text);
        assert!(count > 0);
        println!("Token count for '{}': {}", text, count);
    }
}
