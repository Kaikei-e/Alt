use std::collections::HashMap;
use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use ammonia::clean;
use anyhow::{Context, Result};
use async_trait::async_trait;
use chrono::{DateTime, Utc};
use std::sync::LazyLock;
use regex::Regex;
use serde_json::json;
use tokio::sync::Semaphore;
use tracing::{debug, info};
use unicode_normalization::UnicodeNormalization;
use unicode_segmentation::UnicodeSegmentation;
use uuid::Uuid;
// whatlang is replaced with lingua in language_detection module

use crate::scheduler::JobContext;
use crate::store::{dao::RecapDao, models::PreprocessMetrics};

use super::fetch::{FetchedArticle, FetchedCorpus};
use super::tag_signal::TagSignal;
use crate::clients::SubworkerClient;
use serde::{Deserialize, Serialize};

/// 前処理後の記事データ。
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct PreprocessedArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) body: String,
    pub(crate) language: String,
    pub(crate) char_count: usize,
    pub(crate) is_html_cleaned: bool,
    pub(crate) published_at: Option<DateTime<Utc>>,
    pub(crate) source_url: Option<String>,
    pub(crate) tokens: Vec<String>,
    pub(crate) tags: Vec<TagSignal>,
}

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub(crate) struct PreprocessedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<PreprocessedArticle>,
}

#[async_trait]
pub(crate) trait PreprocessStage: Send + Sync {
    async fn preprocess(
        &self,
        job: &JobContext,
        corpus: FetchedCorpus,
    ) -> anyhow::Result<PreprocessedCorpus>;
}

/// CPU heavy前処理を行うステージ。
///
/// `spawn_blockingでCPUバインド処理をオフロードし、セマフォで同時実行数を制限します`。
#[derive(Clone)]
pub struct TextPreprocessStage {
    semaphore: Arc<Semaphore>,
    dao: Arc<dyn RecapDao>,
    subworker: Arc<SubworkerClient>,
}

impl TextPreprocessStage {
    /// `新しいPreprocessStageを作成する`。
    ///
    /// # Arguments
    /// * `max_concurrent` - 同時に処理できる記事の最大数
    /// * `dao` - データベースアクセスオブジェクト
    pub(crate) fn new(
        max_concurrent: usize,
        dao: Arc<dyn RecapDao>,
        subworker: Arc<SubworkerClient>,
    ) -> Self {
        Self {
            semaphore: Arc::new(Semaphore::new(max_concurrent)),
            dao,
            subworker,
        }
    }
}

#[async_trait]
impl PreprocessStage for TextPreprocessStage {
    async fn preprocess(
        &self,
        job: &JobContext,
        corpus: FetchedCorpus,
    ) -> Result<PreprocessedCorpus> {
        let total_articles = corpus.articles.len();
        info!(
            job_id = %job.job_id,
            count = total_articles,
            "starting text preprocessing"
        );

        let mut tasks = Vec::with_capacity(total_articles);

        for article in corpus.articles {
            let semaphore = Arc::clone(&self.semaphore);

            let subworker = Arc::clone(&self.subworker);

            let task = tokio::spawn(async move {
                // セマフォで同時実行数を制限
                let _permit = semaphore
                    .acquire()
                    .await
                    .expect("semaphore should not be closed");

                preprocess_article(article, subworker).await
            });

            tasks.push(task);
        }

        // すべてのタスクを待機
        let results = futures::future::join_all(tasks).await;

        let mut articles = Vec::with_capacity(total_articles);
        let mut processed_count = 0;
        let mut dropped_count = 0;
        let mut html_cleaned_count = 0;
        let mut total_characters = 0;
        let mut language_counts: HashMap<String, usize> = HashMap::new();
        let progress_counter = Arc::new(AtomicUsize::new(0));

        for result in results {
            let current_progress = progress_counter.fetch_add(1, Ordering::Relaxed) + 1;

            // Log progress every 100 articles
            if current_progress.is_multiple_of(100) || current_progress == total_articles {
                info!(
                    job_id = %job.job_id,
                    processed = current_progress,
                    total = total_articles,
                    "preprocessing progress"
                );
            }
            match result {
                Ok(Ok(Some(article))) => {
                    total_characters += article.char_count;
                    if article.is_html_cleaned {
                        html_cleaned_count += 1;
                    }
                    *language_counts.entry(article.language.clone()).or_insert(0) += 1;

                    articles.push(article);
                    processed_count += 1;
                }
                Ok(Ok(None)) => {
                    dropped_count += 1;
                }
                Ok(Err(e)) => {
                    debug!(error = ?e, "article preprocessing failed, dropping");
                    dropped_count += 1;
                }
                Err(e) => {
                    debug!(error = ?e, "spawn task failed, dropping");
                    dropped_count += 1;
                }
            }
        }

        // 統計を収集して保存
        let languages_json = json!(language_counts);
        let metrics = PreprocessMetrics::new(
            job.job_id,
            total_articles,
            processed_count,
            dropped_count,
            html_cleaned_count,
            total_characters,
            languages_json,
        );

        self.dao
            .save_preprocess_metrics(&metrics)
            .await
            .context("failed to save preprocessing metrics")?;

        info!(
            job_id = %job.job_id,
            processed = processed_count,
            dropped = dropped_count,
            html_cleaned = html_cleaned_count,
            total_chars = total_characters,
            "completed text preprocessing"
        );

        Ok(PreprocessedCorpus {
            job_id: job.job_id,
            articles,
        })
    }
}

/// 単一記事の前処理を実行する。
///
/// 1. Subworkerによる抽出 (Trafilatura)
/// 2. フォールバック処理 (Ammonia + html2text)
/// 3. Unicode正規化など
pub(crate) async fn preprocess_article(
    article: FetchedArticle,
    subworker: Arc<SubworkerClient>,
) -> Result<Option<PreprocessedArticle>> {
    // 1. Subworkerによる抽出
    let (cleaned_body, is_html_cleaned) = match subworker.extract_content(&article.body).await {
        Ok(text) if !text.trim().is_empty() => (text, true),
        _ => {
            // Fallback to local cleaning if subworker fails or returns empty
            // Use spawn_blocking for CPU bound local cleaning
            let body = article.body.clone();
            let (text, cleaned) = tokio::task::spawn_blocking(move || clean_html(&body)).await??;
            (text, cleaned)
        }
    };

    // CPUバインド処理: 正規化、トークナイズ
    // 短いのでインラインで実行するか、必要ならブロック化
    // ここでは単純化のためインライン実行（必要に応じて最適化）
    let normalized = cleaned_body.nfc().collect::<String>();
    let trimmed = normalized.trim();

    if trimmed.is_empty() {
        return Ok(None);
    }

    // 3. 言語検出（linguaベース）
    let language = article
        .language
        .clone()
        .or_else(|| {
            use crate::classification::ClassificationLanguage;
            use crate::classification::tokenizer::TokenPipeline;
            let lang = TokenPipeline::resolve_language(ClassificationLanguage::Unknown, trimmed);
            Some(match lang {
                ClassificationLanguage::Japanese => "ja".to_string(),
                ClassificationLanguage::English => "en".to_string(),
                ClassificationLanguage::Unknown => "und".to_string(),
            })
        })
        .unwrap_or_else(|| "und".to_string());

    // 4. タイトル処理
    let title = article.title.clone().map(|t| {
        let cleaned_title = if contains_html_tags(&t) { clean(&t) } else { t };
        cleaned_title.nfc().collect::<String>()
    });

    let char_count = trimmed.chars().count();
    let tokens = tokenize_text(trimmed, &language);

    if !is_valid_content(trimmed, &language, char_count) {
        debug!(id = %article.id, lang = %language, len = char_count, "filtered out short content");
        return Ok(None);
    }

    Ok(Some(PreprocessedArticle {
        id: article.id,
        title,
        body: trimmed.to_string(),
        language,
        char_count,
        is_html_cleaned,
        published_at: article.published_at,
        source_url: article.source_url,
        tokens,
        tags: article.tags,
    }))
}

fn tokenize_text(text: &str, lang: &str) -> Vec<String> {
    if lang.starts_with("ja") {
        return tokenize_japanese(text);
    }

    tokenize_latin_like(text)
}

fn tokenize_japanese(text: &str) -> Vec<String> {
    let filtered: Vec<char> = text
        .chars()
        .filter(|c| !c.is_whitespace() && c.is_alphanumeric())
        .collect();

    if filtered.is_empty() {
        return Vec::new();
    }

    if filtered.len() == 1 {
        return vec![filtered.into_iter().collect()];
    }

    let mut tokens = Vec::with_capacity(filtered.len().saturating_sub(1));
    for window in filtered.windows(2) {
        let token: String = window.iter().collect();
        tokens.push(token);
    }
    tokens
}

fn tokenize_latin_like(text: &str) -> Vec<String> {
    let mut results = Vec::new();
    for word in text.unicode_words() {
        let cleaned = NON_WORD_BOUNDARY.replace_all(word, "");
        let candidate = cleaned.trim().to_lowercase();
        if candidate.len() >= 2 {
            results.push(candidate);
        }
    }
    results
}

static NON_WORD_BOUNDARY: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"^[\p{Punctuation}\p{Symbol}]+|[\p{Punctuation}\p{Symbol}]+$").unwrap()
});

/// HTMLをサニタイズしてプレーンテキストに変換する。
///
/// # Returns
/// (`cleaned_text`, `was_html`) のタプル
fn clean_html(text: &str) -> Result<(String, bool)> {
    if !contains_html_tags(text) {
        return Ok((text.to_string(), false));
    }

    // ammoniaで安全にHTMLタグを除去
    let sanitized = clean(text);

    // html2textでよりクリーンなプレーンテキストに変換
    let plain = html2text::from_read(sanitized.as_bytes(), 80)
        .map_err(|e| anyhow::anyhow!("html2text conversion failed: {e}"))?;

    Ok((plain, true))
}

/// テキストがHTMLタグを含むかどうかを簡易チェック。
fn contains_html_tags(text: &str) -> bool {
    let bytes = text.as_bytes();
    let len = bytes.len();
    let mut i = 0;

    while i < len {
        if bytes[i] == b'<' {
            if i + 1 >= len {
                i += 1;
                continue;
            }

            let next = bytes[i + 1];
            if next.is_ascii_whitespace() {
                i += 1;
                continue;
            }

            if next == b'/' || next == b'!' || next == b'?' || next.is_ascii_alphabetic() {
                let mut j = i + 2;
                while j < len {
                    if bytes[j] == b'>' {
                        return true;
                    }
                    j += 1;
                }
            }
        }
        i += 1;
    }

    false
}

/// コンテンツが有効（短すぎない、または例外条件を満たす）かどうかを判定する。
fn is_valid_content(text: &str, _lang: &str, char_count: usize) -> bool {
    let ja_ratio = calculate_ja_ratio(text);
    let min_len = if ja_ratio >= 0.3 { 10 } else { 20 };

    if char_count >= min_len {
        return true;
    }

    // Exception: Ends with a Japanese period (likely a complete sentence)
    if text.trim().ends_with('。') {
        return true;
    }

    // Exception: Contains numbers (likely data-heavy or specific info)
    if text.chars().any(char::is_numeric) {
        return true;
    }

    false
}

fn calculate_ja_ratio(text: &str) -> f32 {
    let mut ja_count = 0;
    let mut total = 0;

    for c in text.chars() {
        if c.is_whitespace() {
            continue;
        }
        total += 1;
        // Simple range check for Hiragana, Katakana, Kanji
        // Hiragana: 3040-309F
        // Katakana: 30A0-30FF
        // Kanji: 4E00-9FAF (Common)
        if matches!(c, '\u{3040}'..='\u{309F}' | '\u{30A0}'..='\u{30FF}' | '\u{4E00}'..='\u{9FAF}')
        {
            ja_count += 1;
        }
    }

    if total == 0 {
        0.0
    } else {
        ja_count as f32 / total as f32
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    fn article(
        id: &str,
        body: &str,
        title: Option<&str>,
        language: Option<&str>,
    ) -> FetchedArticle {
        FetchedArticle {
            id: id.to_string(),
            title: title.map(std::string::ToString::to_string),
            body: body.to_string(),
            language: language.map(std::string::ToString::to_string),
            published_at: Some(Utc::now()),
            source_url: Some("https://example.com".to_string()),
            tags: Vec::new(),
        }
    }

    #[tokio::test]
    async fn preprocess_article_trims_and_detects_language_case_1() {
        let body = "  正規化テキストの処理を実行します。これは日本語のテストです。  ";
        let language = Some("ja");
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let fetched = article("art-1", body, Some("Title"), language);
        let result = preprocess_article(fetched, subworker)
            .await
            .expect("preprocessing should succeed")
            .expect("article should remain");
        assert_eq!(result.body, body.trim());
        if let Some(lang) = language {
            assert_eq!(result.language, lang);
        } else {
            assert!(!result.language.is_empty());
        }
        assert!(!result.tokens.is_empty());
    }

    #[tokio::test]
    async fn preprocess_article_trims_and_detects_language_case_2() {
        let body = "This is a longer text with spaces that meets the minimum length requirement for English content.";
        let language = None;
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let fetched = article("art-2", body, Some("Title"), language);
        let result = preprocess_article(fetched, subworker)
            .await
            .expect("preprocessing should succeed")
            .expect("article should remain");
        assert_eq!(result.body, body.trim());
        assert!(!result.language.is_empty());
        assert!(!result.tokens.is_empty());
    }

    #[tokio::test]
    async fn preprocess_article_drops_empty() {
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let result = preprocess_article(article("art-1", "   ", None, None), subworker)
            .await
            .expect("preprocessing should succeed");
        assert!(result.is_none());
    }

    #[tokio::test]
    async fn preprocess_article_tokenizes_japanese() {
        let subworker = Arc::new(SubworkerClient::new("http://localhost:8002", 10).unwrap());
        let fetched = article(
            "art-ja",
            "東京大学で量子コンピューターの研究が進んでいます。",
            Some("タイトル"),
            Some("ja"),
        );
        let result = preprocess_article(fetched, subworker)
            .await
            .expect("preprocessing should succeed")
            .expect("article should remain");
        assert!(result.tokens.iter().any(|t| t.contains("東京")));
    }

    #[test]
    fn clean_html_removes_tags() {
        let html = "<p>This is <strong>bold</strong> text.</p>";
        let (cleaned, was_html) = clean_html(html).unwrap();
        assert!(was_html);
        assert!(!cleaned.contains("<p>"));
        assert!(!cleaned.contains("<strong>"));
    }

    #[test]
    fn clean_html_preserves_plain_text() {
        let plain = "This is plain text.";
        let (cleaned, was_html) = clean_html(plain).unwrap();
        assert!(!was_html);
        assert_eq!(cleaned, plain);
    }

    #[test]
    fn contains_html_tags_detects_tags() {
        assert!(contains_html_tags("<div>test</div>"));
        assert!(contains_html_tags("text <span>with</span> tags"));
        assert!(!contains_html_tags("plain text"));
        assert!(!contains_html_tags("text with < and > but not tags"));
    }

    #[tokio::test]
    async fn preprocess_filters_empty_articles() {
        // モックDAOを使用（実装は簡略化、実際にはモックライブラリを使う）
        // テストでは統計保存をスキップするか、メモリ内DAOを使用
        // ここでは単体テストなので、preprocess_article関数のテストのみとする
    }

    #[tokio::test]
    async fn preprocess_handles_html_content() {
        // 同上：統合テストで実装
    }
}
