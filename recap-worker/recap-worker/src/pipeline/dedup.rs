use std::collections::{HashMap, HashSet};
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use tokio::sync::Semaphore;
use tracing::{debug, info};
use uuid::Uuid;

use crate::scheduler::JobContext;
use crate::util::text::{hash_text, is_near_duplicate, split_sentences};

use super::preprocess::{PreprocessedArticle, PreprocessedCorpus};

/// 重複排除後の記事データ。
#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DeduplicatedArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) sentences: Vec<String>,
    pub(crate) sentence_hashes: Vec<u64>,
    pub(crate) language: String,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DeduplicatedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<DeduplicatedArticle>,
    pub(crate) stats: DedupStats,
}

/// 重複排除統計。
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub(crate) struct DedupStats {
    pub(crate) total_articles: usize,
    pub(crate) unique_articles: usize,
    pub(crate) duplicate_articles: usize,
    pub(crate) total_sentences: usize,
    pub(crate) unique_sentences: usize,
    pub(crate) duplicate_sentences: usize,
}

#[async_trait]
pub(crate) trait DedupStage: Send + Sync {
    async fn deduplicate(
        &self,
        job: &JobContext,
        corpus: PreprocessedCorpus,
    ) -> anyhow::Result<DeduplicatedCorpus>;
}

/// XXH3ハッシュと文分割による重複排除ステージ。
#[derive(Debug, Clone)]
pub(crate) struct HashDedupStage {
    semaphore: Arc<Semaphore>,
    near_duplicate_threshold: f64,
    window_size: usize,
}

impl HashDedupStage {
    /// 新しいDedupStageを作成する。
    ///
    /// # Arguments
    /// * `max_concurrent` - 同時処理数
    /// * `near_duplicate_threshold` - 近似重複判定の閾値（0.0〜1.0）
    /// * `window_size` - ローリングウィンドウサイズ
    pub(crate) fn new(
        max_concurrent: usize,
        near_duplicate_threshold: f64,
        window_size: usize,
    ) -> Self {
        Self {
            semaphore: Arc::new(Semaphore::new(max_concurrent)),
            near_duplicate_threshold,
            window_size,
        }
    }

    /// デフォルトパラメータで作成する。
    pub(crate) fn with_defaults() -> Self {
        let cpu_count = num_cpus::get();
        Self::new(cpu_count.max(2), 0.8, 100)
    }
}

impl Default for HashDedupStage {
    fn default() -> Self {
        Self::with_defaults()
    }
}

#[async_trait]
impl DedupStage for HashDedupStage {
    async fn deduplicate(
        &self,
        job: &JobContext,
        corpus: PreprocessedCorpus,
    ) -> Result<DeduplicatedCorpus> {
        let total_articles = corpus.articles.len();
        info!(
            job_id = %job.job_id,
            count = total_articles,
            "starting deduplication with XXH3 hashing"
        );

        let mut stats = DedupStats {
            total_articles,
            ..Default::default()
        };

        // 記事ハッシュで重複チェック
        let mut article_hashes: HashMap<u64, String> = HashMap::new();
        let mut unique_articles = Vec::with_capacity(total_articles);

        for article in corpus.articles {
            // 記事本文全体のハッシュ
            let article_hash = hash_text(&article.body);

            // 既存の記事と近似重複チェック
            let is_duplicate = article_hashes.values().any(|existing_body| {
                is_near_duplicate(
                    &article.body,
                    existing_body,
                    self.window_size,
                    self.near_duplicate_threshold,
                )
            });

            if is_duplicate {
                stats.duplicate_articles += 1;
                debug!(
                    article_id = %article.id,
                    "dropped duplicate article"
                );
            } else {
                article_hashes.insert(article_hash, article.body.clone());
                unique_articles.push(article);
            }
        }

        stats.unique_articles = unique_articles.len();

        // 文分割と文レベルの重複排除（並列処理）
        let mut tasks = Vec::with_capacity(unique_articles.len());

        for article in unique_articles {
            let semaphore = Arc::clone(&self.semaphore);

            let task = tokio::spawn(async move {
                let _permit = semaphore.acquire().await.expect("semaphore should not be closed");

                tokio::task::spawn_blocking(move || deduplicate_sentences(article))
                    .await
                    .expect("sentence dedup should not panic")
            });

            tasks.push(task);
        }

        let results = futures::future::join_all(tasks).await;

        let mut articles = Vec::with_capacity(results.len());

        for result in results {
            match result {
                Ok((article, sentence_stats)) => {
                    stats.total_sentences += sentence_stats.0;
                    stats.unique_sentences += sentence_stats.1;
                    stats.duplicate_sentences += sentence_stats.2;
                    articles.push(article);
                }
                Err(e) => {
                    debug!(error = ?e, "sentence dedup task failed");
                }
            }
        }

        info!(
            job_id = %job.job_id,
            unique_articles = stats.unique_articles,
            duplicate_articles = stats.duplicate_articles,
            unique_sentences = stats.unique_sentences,
            duplicate_sentences = stats.duplicate_sentences,
            "completed deduplication"
        );

        Ok(DeduplicatedCorpus {
            job_id: job.job_id,
            articles,
            stats,
        })
    }
}

/// 単一記事の文分割と文レベル重複排除を実行する（CPU heavy）。
///
/// # Returns
/// (DeduplicatedArticle, (total_sentences, unique_sentences, duplicate_sentences))
fn deduplicate_sentences(article: PreprocessedArticle) -> (DeduplicatedArticle, (usize, usize, usize)) {
    let sentences = split_sentences(&article.body);
    let total_sentences = sentences.len();

    let mut seen_hashes = HashSet::new();
    let mut unique_sentences = Vec::new();
    let mut sentence_hashes = Vec::new();

    for sentence in sentences {
        let hash = hash_text(&sentence);

        if seen_hashes.insert(hash) {
            unique_sentences.push(sentence);
            sentence_hashes.push(hash);
        }
    }

    let unique_count = unique_sentences.len();
    let duplicate_count = total_sentences.saturating_sub(unique_count);

    let dedup_article = DeduplicatedArticle {
        id: article.id,
        title: article.title,
        sentences: unique_sentences,
        sentence_hashes,
        language: article.language,
    };

    (dedup_article, (total_sentences, unique_count, duplicate_count))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn article(id: &str, body: &str, title: Option<&str>) -> PreprocessedArticle {
        PreprocessedArticle {
            id: id.to_string(),
            title: title.map(|t| t.to_string()),
            body: body.to_string(),
            language: "en".to_string(),
            char_count: body.chars().count(),
            is_html_cleaned: false,
        }
    }

    #[tokio::test]
    async fn deduplicate_filters_exact_duplicates() {
        let stage = HashDedupStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = PreprocessedCorpus {
            job_id: job.job_id,
            articles: vec![
                article("art-1", "Text content", Some("Title 1")),
                article("art-2", "Text content", Some("Title 2")), // duplicate
                article("art-3", "Another text", Some("Title 3")),
            ],
        };

        let result = stage
            .deduplicate(&job, corpus)
            .await
            .expect("dedup succeeds");

        assert_eq!(result.articles.len(), 2);
        assert_eq!(result.stats.unique_articles, 2);
        assert_eq!(result.stats.duplicate_articles, 1);
    }

    #[tokio::test]
    async fn deduplicate_splits_sentences() {
        let stage = HashDedupStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let body = "First sentence. Second sentence! Third sentence?";
        let corpus = PreprocessedCorpus {
            job_id: job.job_id,
            articles: vec![article("art-1", body, Some("Title"))],
        };

        let result = stage
            .deduplicate(&job, corpus)
            .await
            .expect("dedup succeeds");

        assert_eq!(result.articles.len(), 1);
        let article = &result.articles[0];
        assert_eq!(article.sentences.len(), 3);
        assert_eq!(article.sentence_hashes.len(), 3);
    }

    #[tokio::test]
    async fn deduplicate_removes_duplicate_sentences() {
        let stage = HashDedupStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let body = "Repeated sentence. Repeated sentence. Unique sentence.";
        let corpus = PreprocessedCorpus {
            job_id: job.job_id,
            articles: vec![article("art-1", body, Some("Title"))],
        };

        let result = stage
            .deduplicate(&job, corpus)
            .await
            .expect("dedup succeeds");

        assert_eq!(result.articles.len(), 1);
        let article = &result.articles[0];
        assert_eq!(article.sentences.len(), 2); // 1つの重複が削除される
        assert!(result.stats.duplicate_sentences > 0);
    }
}
