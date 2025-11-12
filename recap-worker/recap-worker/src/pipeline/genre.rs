use std::collections::HashMap;

use anyhow::Result;
use async_trait::async_trait;
use tracing::{debug, info};
use uuid::Uuid;

use crate::classification::{ClassificationLanguage, GenreClassifier};
use crate::scheduler::JobContext;

use super::dedup::{DeduplicatedArticle, DeduplicatedCorpus};
use super::genre_keywords::GenreKeywords;

/// ジャンル付き記事。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct GenreAssignment {
    pub(crate) genres: Vec<String>,                  // 1〜3個のジャンル
    pub(crate) genre_scores: HashMap<String, usize>, // 全スコア
    pub(crate) genre_confidence: HashMap<String, f32>,
    pub(crate) feature_profile: FeatureProfile,
    pub(crate) article: DeduplicatedArticle,
}

/// ジャンル別の記事グループ。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct GenreBundle {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
    pub(crate) genre_distribution: HashMap<String, usize>, // ジャンル別記事数
}

#[derive(Debug, Clone, Default, PartialEq)]
pub(crate) struct FeatureProfile {
    pub(crate) tfidf_sum: f32,
    pub(crate) bm25_peak: f32,
    pub(crate) token_count: usize,
}

#[async_trait]
pub(crate) trait GenreStage: Send + Sync {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle>;
}

/// キーワードベースのジャンル付与ステージ。
///
/// タイトル+本文からキーワードマッチングで1〜3個のジャンルを付与します。
#[derive(Debug)]
pub(crate) struct HybridGenreStage {
    classifier: GenreClassifier,
    fallback_keywords: GenreKeywords,
    min_genres: usize,
    max_genres: usize,
}

impl HybridGenreStage {
    /// 新しいKeywordGenreStageを作成する。
    ///
    /// # Arguments
    /// * `min_genres` - 最小ジャンル数（デフォルト: 1）
    /// * `max_genres` - 最大ジャンル数（デフォルト: 3）
    pub(crate) fn new(min_genres: usize, max_genres: usize) -> Self {
        Self {
            classifier: GenreClassifier::new_default(),
            fallback_keywords: GenreKeywords::default_keywords(),
            min_genres,
            max_genres,
        }
    }

    /// デフォルトパラメータで作成する（1〜3ジャンル）。
    pub(crate) fn with_defaults() -> Self {
        Self::new(1, 3)
    }

    /// 記事にジャンルを付与する。
    fn assign_genres(
        &self,
        article: &DeduplicatedArticle,
    ) -> anyhow::Result<(
        Vec<String>,
        HashMap<String, usize>,
        HashMap<String, f32>,
        FeatureProfile,
    )> {
        let title = article.title.as_deref().unwrap_or("");
        let body = article.sentences.join(" ");
        let language = ClassificationLanguage::from_code(&article.language);

        let classification = self.classifier.predict(title, &body, language)?;
        let mut genres = classification.top_genres.clone();

        // 最低ジャンル数を満たすまでランキングから補完
        if genres.len() < self.min_genres {
            for (candidate, _) in &classification.ranking {
                if genres.contains(candidate) {
                    continue;
                }
                genres.push(candidate.clone());
                if genres.len() == self.min_genres {
                    break;
                }
            }
        }

        if genres.is_empty() {
            genres.push("other".to_string());
        }

        if genres.len() > self.max_genres {
            genres.truncate(self.max_genres);
        }

        let mut genre_scores = classification.keyword_hits.clone();
        for genre in &genres {
            genre_scores.entry(genre.clone()).or_insert_with(|| {
                classification
                    .scores
                    .get(genre)
                    .map(|score| (score.max(0.0) * 100.0).round() as usize)
                    .unwrap_or(0)
            });
        }

        if genre_scores.is_empty() {
            // フォールバックとしてキーワードスコアを計算
            let combined = format!("{title} {body}");
            genre_scores = self.fallback_keywords.score_text(&combined);
        }

        let low_support = genres
            .iter()
            .all(|genre| genre_scores.get(genre).copied().unwrap_or(0) == 0);
        if low_support {
            genres.clear();
            genres.push("other".to_string());
            genre_scores.entry("other".to_string()).or_insert(100);
        }

        let mut genre_confidence: HashMap<String, f32> = classification
            .scores
            .iter()
            .map(|(genre, score)| (genre.clone(), score.clamp(0.0, 1.0)))
            .collect();
        for genre in &genres {
            genre_confidence.entry(genre.clone()).or_insert(0.0);
        }

        let tfidf_sum: f32 = classification.feature_snapshot.tfidf.iter().sum();
        let feature_profile = FeatureProfile {
            tfidf_sum,
            bm25_peak: classification.feature_snapshot.max_bm25().unwrap_or(0.0),
            token_count: classification.token_count,
        };

        Ok((genres, genre_scores, genre_confidence, feature_profile))
    }
}

impl Default for HybridGenreStage {
    fn default() -> Self {
        Self::with_defaults()
    }
}

#[async_trait]
impl GenreStage for HybridGenreStage {
    async fn assign(&self, job: &JobContext, corpus: DeduplicatedCorpus) -> Result<GenreBundle> {
        let total_articles = corpus.articles.len();
        info!(
            job_id = %job.job_id,
            count = total_articles,
            "starting genre assignment with keyword heuristics"
        );

        let mut assignments = Vec::with_capacity(total_articles);
        let mut genre_distribution: HashMap<String, usize> = HashMap::new();

        for article in corpus.articles {
            let (genres, genre_scores, genre_confidence, feature_profile) =
                self.assign_genres(&article)?;

            debug!(
                article_id = %article.id,
                genres = ?genres,
                "assigned genres to article"
            );

            // 分布を更新
            for genre in &genres {
                *genre_distribution.entry(genre.clone()).or_insert(0) += 1;
            }

            assignments.push(GenreAssignment {
                genres,
                genre_scores,
                genre_confidence,
                feature_profile,
                article,
            });
        }

        info!(
            job_id = %job.job_id,
            total_assignments = assignments.len(),
            genre_distribution = ?genre_distribution,
            "completed genre assignment"
        );

        Ok(GenreBundle {
            job_id: job.job_id,
            assignments,
            genre_distribution,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::super::dedup::DedupStats;
    use super::*;

    fn article(id: &str, title: Option<&str>, sentences: Vec<&str>) -> DeduplicatedArticle {
        DeduplicatedArticle {
            id: id.to_string(),
            title: title.map(String::from),
            sentences: sentences.into_iter().map(String::from).collect(),
            sentence_hashes: vec![],
            language: "en".to_string(),
        }
    }

    #[tokio::test]
    async fn assigns_genres_based_on_keywords() {
        let stage = HybridGenreStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![
                article(
                    "art-1",
                    Some("Machine Learning and AI"),
                    vec!["This article discusses artificial intelligence and deep learning."],
                ),
                article(
                    "art-2",
                    Some("Football Championship"),
                    vec!["The team won the tournament in an exciting match."],
                ),
            ],
            stats: DedupStats::default(),
        };

        let bundle = stage.assign(&job, corpus).await.unwrap();

        assert_eq!(bundle.assignments.len(), 2);

        // 最初の記事はAI関連のキーワードを含む
        assert!(
            bundle.assignments[0]
                .genres
                .iter()
                .any(|genre| genre == "ai" || genre == "tech")
        );

        // 2番目の記事はスポーツ関連のキーワードを含む
        assert!(
            bundle.assignments[1]
                .genres
                .iter()
                .any(|genre| genre == "sports")
        );
    }

    #[tokio::test]
    async fn assigns_at_least_one_genre() {
        let stage = HybridGenreStage::with_defaults();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![article(
                "art-1",
                Some("Generic Article"),
                vec!["This is a generic article with no specific keywords."],
            )],
            stats: DedupStats::default(),
        };

        let bundle = stage.assign(&job, corpus).await.unwrap();

        assert_eq!(bundle.assignments.len(), 1);
        assert!(!bundle.assignments[0].genres.is_empty());
        // キーワードマッチがない場合は"other"が付与される
        assert!(bundle.assignments[0].genres.contains(&"other".to_string()));
    }

    #[tokio::test]
    async fn respects_max_genres_limit() {
        let stage = HybridGenreStage::new(1, 2);
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![article(
                "art-1",
                Some("Tech Science Business AI Health"),
                vec!["Technology, science, business, AI, and health news."],
            )],
            stats: DedupStats::default(),
        };

        let bundle = stage.assign(&job, corpus).await.unwrap();

        assert_eq!(bundle.assignments.len(), 1);
        // 最大2ジャンル
        assert!(bundle.assignments[0].genres.len() <= 2);
    }
}
