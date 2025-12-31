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

        // Hugging Face トークンを読み込む
        let token = if let Ok(token) = std::env::var("HF_TOKEN") {
            info!("Using Hugging Face token from HF_TOKEN environment variable");
            token
        } else {
            let token_path = std::env::var("HUGGING_FACE_TOKEN_PATH").context(
                "HUGGING_FACE_TOKEN_PATH environment variable is not set (HF_TOKEN also not set)",
            )?;

            let token = std::fs::read_to_string(&token_path)
                .with_context(|| format!("Failed to read Hugging Face token from {}", token_path))?
                .trim()
                .to_string();

            // 環境変数にも設定しておく（他のライブラリが使用する可能性があるため）
            // SAFETY: This function is called during TokenCounter initialization, which typically
            // occurs once at application startup. While theoretically multiple threads could call
            // this concurrently, in practice this is initialized by the ComponentRegistry during
            // single-threaded startup. The environment variable is set to a read-only value and
            // not modified afterwards. If concurrent initialization becomes a concern, consider
            // using std::sync::Once or an initialization lock.
            unsafe {
                std::env::set_var("HF_TOKEN", &token);
            }
            info!("Loaded Hugging Face token from {}", token_path);
            token
        };

        // Gemma 3のトークナイザーをロード
        // 注意: 初回実行時にダウンロードが発生するため、インターネット接続が必要
        // FromPretrainedParameters を使用してトークンを明示的に設定
        // 注意: Gemma 3モデルは利用条件への同意が必要です
        let token_preview = if token.len() > 10 {
            format!("{}...{}", &token[..4], &token[token.len() - 4..])
        } else {
            "***".to_string()
        };
        info!("Using token: {}", token_preview);

        let params = tokenizers::FromPretrainedParameters {
            token: Some(token.clone()),
            ..Default::default()
        };

        // 環境変数も設定（hf-hubが環境変数を読み取る可能性があるため）
        // SAFETY: Setting HF_TOKEN environment variable for use by hf-hub and related libraries.
        // This is safe because: (1) it's called during initialization before concurrent access,
        // (2) the token value is immutable after being set, and (3) this duplicates the setting
        // from above (line 35) to ensure the variable is available for FromPretrainedParameters.
        unsafe {
            std::env::set_var("HF_TOKEN", &token);
        }

        let tokenizer = Tokenizer::from_pretrained("google/gemma-3-4b-it", Some(params))
            .map_err(|e| anyhow::anyhow!("Failed to load tokenizer: {}. Note: Gemma 3 models require accepting the terms of use on Hugging Face.", e))?;

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

#[cfg(test)]
mod tests {
    use super::*;

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
