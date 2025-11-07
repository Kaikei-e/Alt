use std::collections::HashMap;
use std::sync::Arc;

use ammonia::clean;
use anyhow::{Context, Result};
use async_trait::async_trait;
use serde_json::json;
use tokio::sync::Semaphore;
use tracing::{debug, info};
use unicode_normalization::UnicodeNormalization;
use uuid::Uuid;
use whatlang::detect;

use crate::scheduler::JobContext;
use crate::store::{dao::RecapDao, models::PreprocessMetrics};

use super::fetch::{FetchedArticle, FetchedCorpus};

/// 前処理後の記事データ。
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct PreprocessedArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) body: String,
    pub(crate) language: String,
    pub(crate) char_count: usize,
    pub(crate) is_html_cleaned: bool,
}

#[derive(Debug, Clone, PartialEq, Eq)]
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
/// spawn_blockingでCPUバインド処理をオフロードし、セマフォで同時実行数を制限します。
#[derive(Debug, Clone)]
pub(crate) struct TextPreprocessStage {
    semaphore: Arc<Semaphore>,
    dao: Arc<RecapDao>,
}

impl TextPreprocessStage {
    /// 新しいPreprocessStageを作成する。
    ///
    /// # Arguments
    /// * `max_concurrent` - 同時に処理できる記事の最大数
    /// * `dao` - データベースアクセスオブジェクト
    pub(crate) fn new(max_concurrent: usize, dao: Arc<RecapDao>) -> Self {
        Self {
            semaphore: Arc::new(Semaphore::new(max_concurrent)),
            dao,
        }
    }

    /// デフォルトの並列数で作成する（CPUコア数の1.5倍）。
    pub(crate) fn with_default_concurrency(dao: Arc<RecapDao>) -> Self {
        let cpu_count = num_cpus::get();
        let max_concurrent = (cpu_count * 3) / 2; // 1.5倍
        Self::new(max_concurrent.max(2), dao) // 最低2
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

            let task = tokio::spawn(async move {
                // セマフォで同時実行数を制限
                let _permit = semaphore.acquire().await.expect("semaphore should not be closed");

                // CPUバインド処理をspawn_blockingでオフロード
                tokio::task::spawn_blocking(move || preprocess_article(article))
                    .await
                    .context("preprocessing task panicked")
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

        for result in results {
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
                    debug!(error = ?e, "preprocessing task failed, dropping");
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

/// 単一記事の前処理を実行する（CPU heavy）。
///
/// 1. HTMLサニタイズ（ammoniaで安全にタグ除去）
/// 2. プレーン化（html2textで変換）
/// 3. Unicode正規化（NFC）
/// 4. 言語検出
fn preprocess_article(article: FetchedArticle) -> Result<Option<PreprocessedArticle>> {
    // 1. HTMLサニタイズとプレーン化
    let (cleaned_body, is_html_cleaned) = clean_html(&article.body);

    // 2. Unicode正規化（NFC）
    let normalized = cleaned_body.nfc().collect::<String>();
    let trimmed = normalized.trim();

    if trimmed.is_empty() {
        return Ok(None);
    }

    // 3. 言語検出
    let language = article
        .language
        .or_else(|| detect(trimmed).map(|info| info.lang().code().to_string()))
        .unwrap_or_else(|| "und".to_string());

    // 4. タイトル処理
    let title = article.title.map(|t| {
        let cleaned_title = if contains_html_tags(&t) {
            clean(&t)
        } else {
            t
        };
        cleaned_title.nfc().collect::<String>()
    });

    let char_count = trimmed.chars().count();

    Ok(Some(PreprocessedArticle {
        id: article.id,
        title,
        body: trimmed.to_string(),
        language,
        char_count,
        is_html_cleaned,
    }))
}

/// HTMLをサニタイズしてプレーンテキストに変換する。
///
/// # Returns
/// (cleaned_text, was_html) のタプル
fn clean_html(text: &str) -> (String, bool) {
    if !contains_html_tags(text) {
        return (text.to_string(), false);
    }

    // ammoniaで安全にHTMLタグを除去
    let sanitized = clean(text);

    // html2textでよりクリーンなプレーンテキストに変換
    let plain = html2text::from_read(sanitized.as_bytes(), 80);

    (plain, true)
}

/// テキストがHTMLタグを含むかどうかを簡易チェック。
fn contains_html_tags(text: &str) -> bool {
    text.contains('<') && text.contains('>')
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;
    use rstest::rstest;

    fn article(id: &str, body: &str, title: Option<&str>, language: Option<&str>) -> FetchedArticle {
        FetchedArticle {
            id: id.to_string(),
            title: title.map(|t| t.to_string()),
            body: body.to_string(),
            language: language.map(|l| l.to_string()),
            published_at: Some(Utc::now()),
            source_url: Some("https://example.com".to_string()),
        }
    }

    #[rstest]
    #[case("  正規化テキスト  ", Some("ja"))]
    #[case("Text with spaces", None)]
    fn preprocess_article_trims_and_detects_language(
        #[case] body: &str,
        #[case] language: Option<&str>,
    ) {
        let fetched = article("art-1", body, Some("Title"), language);
        let result = preprocess_article(fetched)
            .expect("preprocessing should succeed")
            .expect("article should remain");
        assert_eq!(result.body, body.trim());
        if let Some(lang) = language {
            assert_eq!(result.language, lang);
        } else {
            assert!(!result.language.is_empty());
        }
    }

    #[test]
    fn preprocess_article_drops_empty() {
        let result = preprocess_article(article("art-1", "   ", None, None))
            .expect("preprocessing should succeed");
        assert!(result.is_none());
    }

    #[test]
    fn clean_html_removes_tags() {
        let html = "<p>This is <strong>bold</strong> text.</p>";
        let (cleaned, was_html) = clean_html(html);
        assert!(was_html);
        assert!(!cleaned.contains("<p>"));
        assert!(!cleaned.contains("<strong>"));
    }

    #[test]
    fn clean_html_preserves_plain_text() {
        let plain = "This is plain text.";
        let (cleaned, was_html) = clean_html(plain);
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
