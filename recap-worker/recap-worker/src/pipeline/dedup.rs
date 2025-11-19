use std::collections::HashSet;
use std::sync::Arc;

use anyhow::Result;
use async_trait::async_trait;
use rayon::prelude::*;
use rustc_hash::{FxHashMap, FxHashSet};
use smallvec::SmallVec;
use tokio::sync::Semaphore;
use tracing::{debug, info};
use uuid::Uuid;

use crate::scheduler::JobContext;
use crate::util::text::{hash_text, rolling_hash_windows, split_sentences};

use super::preprocess::{PreprocessedArticle, PreprocessedCorpus};
use super::tag_signal::TagSignal;

/// 重複排除後の記事データ。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct DeduplicatedArticle {
    pub(crate) id: String,
    pub(crate) title: Option<String>,
    pub(crate) sentences: Vec<String>,
    pub(crate) sentence_hashes: Vec<u64>,
    pub(crate) language: String,
    pub(crate) tags: Vec<TagSignal>,
}

#[derive(Debug, Clone, PartialEq)]
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
pub struct HashDedupStage {
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
    pub fn with_defaults() -> Self {
        let cpu_count = num_cpus::get();
        Self::new(cpu_count.max(2), 0.8, 100)
    }
}

impl Default for HashDedupStage {
    fn default() -> Self {
        Self::with_defaults()
    }
}

struct DedupState<'a> {
    keep_flags: &'a mut [bool],
    unique_signatures: &'a mut Vec<ArticleSignature>,
    exact_hashes: &'a mut FxHashMap<u64, usize>,
    window_index: &'a mut FxHashMap<u64, SmallVec<[usize; 8]>>,
    stats: &'a mut DedupStats,
}

impl HashDedupStage {
    /// 署名を処理して重複を検出し、keep_flagsとインデックスを更新する。
    fn process_signatures(
        &self,
        signatures: Vec<ArticleSignature>,
        articles: &[PreprocessedArticle],
        state: &mut DedupState<'_>,
        job: &JobContext,
        total_articles: usize,
    ) {
        let mut processed_articles = 0usize;

        for signature in signatures {
            processed_articles += 1;

            if let Some(&unique_idx) = state.exact_hashes.get(&signature.primary_hash) {
                let existing_idx = state.unique_signatures[unique_idx].index;
                if articles[existing_idx].body == articles[signature.index].body {
                    state.keep_flags[signature.index] = false;
                    state.stats.duplicate_articles += 1;
                    debug!(
                        article_id = %articles[signature.index].id,
                        "dropped duplicate article (exact match)"
                    );
                    continue;
                }
            }

            let mut candidates: FxHashSet<usize> = FxHashSet::default();
            for hash in signature.window_keys.iter() {
                if let Some(indices) = state.window_index.get(hash) {
                    candidates.extend(indices.iter().copied());
                }
            }

            let mut is_duplicate = false;
            for unique_idx in candidates {
                let other = &state.unique_signatures[unique_idx];
                if window_similarity(other, &signature) >= self.near_duplicate_threshold {
                    state.keep_flags[signature.index] = false;
                    state.stats.duplicate_articles += 1;
                    debug!(
                        article_id = %articles[signature.index].id,
                        "dropped duplicate article (near match)"
                    );
                    is_duplicate = true;
                    break;
                }
            }

            if is_duplicate {
                continue;
            }

            let unique_idx = state.unique_signatures.len();
            for hash in signature.window_keys.iter() {
                state
                    .window_index
                    .entry(*hash)
                    .or_default()
                    .push(unique_idx);
            }
            state
                .exact_hashes
                .insert(signature.primary_hash, unique_idx);
            state.unique_signatures.push(signature);

            if processed_articles.is_multiple_of(DEDUP_PROGRESS_INTERVAL) {
                debug!(
                    job_id = %job.job_id,
                    processed = processed_articles,
                    total = total_articles,
                    unique_so_far = state.unique_signatures.len(),
                    "deduplication progress"
                );
            }
        }
    }

    /// 文レベルの重複排除を並列処理で実行する。
    async fn deduplicate_sentences_parallel(
        &self,
        unique_articles: Vec<PreprocessedArticle>,
        stats: &mut DedupStats,
    ) -> Vec<DeduplicatedArticle> {
        let mut tasks = Vec::with_capacity(unique_articles.len());

        for article in unique_articles {
            let semaphore = Arc::clone(&self.semaphore);

            let task = tokio::spawn(async move {
                let _permit = semaphore
                    .acquire()
                    .await
                    .expect("semaphore should not be closed");

                tokio::task::spawn_blocking(move || deduplicate_sentences(article))
                    .await
                    .expect("sentence dedup should not panic")
            });

            tasks.push(task);
        }

        let results = futures::future::join_all(tasks).await;

        let mut deduped_articles = Vec::with_capacity(results.len());

        for result in results {
            match result {
                Ok((article, sentence_stats)) => {
                    stats.total_sentences += sentence_stats.0;
                    stats.unique_sentences += sentence_stats.1;
                    stats.duplicate_sentences += sentence_stats.2;
                    deduped_articles.push(article);
                }
                Err(e) => {
                    debug!(error = ?e, "sentence dedup task failed");
                }
            }
        }

        deduped_articles
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

        let articles = corpus.articles;
        let signatures = build_signatures(&articles, self.window_size);

        let mut keep_flags = vec![true; total_articles];
        let mut unique_signatures: Vec<ArticleSignature> = Vec::new();
        let mut exact_hashes: FxHashMap<u64, usize> = FxHashMap::default();
        let mut window_index: FxHashMap<u64, SmallVec<[usize; 8]>> = FxHashMap::default();

        let mut dedup_state = DedupState {
            keep_flags: &mut keep_flags,
            unique_signatures: &mut unique_signatures,
            exact_hashes: &mut exact_hashes,
            window_index: &mut window_index,
            stats: &mut stats,
        };

        self.process_signatures(signatures, &articles, &mut dedup_state, job, total_articles);

        let mut unique_articles = Vec::with_capacity(total_articles - stats.duplicate_articles);

        for (idx, article) in articles.into_iter().enumerate() {
            if keep_flags[idx] {
                unique_articles.push(article);
            }
        }

        stats.unique_articles = unique_articles.len();

        let deduped_articles = self
            .deduplicate_sentences_parallel(unique_articles, &mut stats)
            .await;

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
            articles: deduped_articles,
            stats,
        })
    }
}

const MAX_WINDOW_SAMPLE: usize = 256;
const DEDUP_PROGRESS_INTERVAL: usize = 200;

#[derive(Debug, Clone)]
struct ArticleSignature {
    index: usize,
    primary_hash: u64,
    window_keys: SmallVec<[u64; MAX_WINDOW_SAMPLE]>,
    window_histogram: FxHashMap<u64, u32>,
    total_windows: u32,
}

fn build_signatures(articles: &[PreprocessedArticle], window_size: usize) -> Vec<ArticleSignature> {
    articles
        .par_iter()
        .enumerate()
        .map(|(index, article)| ArticleSignature::new(index, article, window_size))
        .collect()
}

impl ArticleSignature {
    fn new(index: usize, article: &PreprocessedArticle, window_size: usize) -> Self {
        let primary_hash = hash_text(&article.body);
        let mut window_keys: SmallVec<[u64; MAX_WINDOW_SAMPLE]> = SmallVec::new();
        let mut histogram: FxHashMap<u64, u32> = FxHashMap::default();

        let windows = rolling_hash_windows(&article.body, window_size);
        let step = (windows.len() / MAX_WINDOW_SAMPLE).max(1);

        for (idx, hash) in windows.into_iter().enumerate() {
            if idx % step != 0 {
                continue;
            }
            if window_keys.len() >= MAX_WINDOW_SAMPLE {
                break;
            }
            *histogram.entry(hash).or_insert(0) += 1;
            window_keys.push(hash);
        }

        if window_keys.is_empty() {
            window_keys.push(primary_hash);
            histogram.insert(primary_hash, 1);
        }

        let total_windows = window_keys.len() as u32;

        Self {
            index,
            primary_hash,
            window_keys,
            window_histogram: histogram,
            total_windows,
        }
    }
}

fn window_similarity(a: &ArticleSignature, b: &ArticleSignature) -> f64 {
    let mut intersection = 0u32;

    for (hash, count_a) in &a.window_histogram {
        if let Some(count_b) = b.window_histogram.get(hash) {
            intersection += (*count_a).min(*count_b);
        }
    }

    let total = a.total_windows + b.total_windows;
    if total == 0 {
        0.0
    } else {
        f64::from(2 * intersection) / f64::from(total)
    }
}

/// 単一記事の文分割と文レベル重複排除を実行する（CPU heavy）。
///
/// # Returns
/// (DeduplicatedArticle, (total_sentences, unique_sentences, duplicate_sentences))
fn deduplicate_sentences(
    article: PreprocessedArticle,
) -> (DeduplicatedArticle, (usize, usize, usize)) {
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
        tags: article.tags,
    };

    (
        dedup_article,
        (total_sentences, unique_count, duplicate_count),
    )
}

#[cfg(test)]
mod tests {
    use super::*;

    fn article(id: &str, body: &str, title: Option<&str>) -> PreprocessedArticle {
        PreprocessedArticle {
            id: id.to_string(),
            title: title.map(std::string::ToString::to_string),
            body: body.to_string(),
            language: "en".to_string(),
            char_count: body.chars().count(),
            is_html_cleaned: false,
            tokens: body
                .split_whitespace()
                .map(str::to_lowercase)
                .collect(),
            tags: Vec::new(),
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
