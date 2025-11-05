use std::collections::VecDeque;

use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::dedup::DeduplicatedCorpus;
use super::preprocess::PreprocessedArticle;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct GenreAssignment {
    pub(crate) genre: String,
    pub(crate) article: PreprocessedArticle,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct GenreBundle {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
}

#[async_trait]
pub(crate) trait GenreStage: Send + Sync {
    async fn assign(
        &self,
        job: &JobContext,
        corpus: DeduplicatedCorpus,
    ) -> anyhow::Result<GenreBundle>;
}

#[derive(Debug, Clone, Default)]
pub(crate) struct BalancedGenreStage;

impl BalancedGenreStage {
    pub(crate) fn new() -> Self {
        Self
    }

    fn resolve_genres(job: &JobContext) -> Vec<String> {
        if job.genres().is_empty() {
            return vec!["general".to_string()];
        }

        job.genres()
            .iter()
            .map(|genre| genre.trim().to_lowercase())
            .filter(|genre| !genre.is_empty())
            .collect()
    }
}

#[async_trait]
impl GenreStage for BalancedGenreStage {
    async fn assign(
        &self,
        job: &JobContext,
        corpus: DeduplicatedCorpus,
    ) -> anyhow::Result<GenreBundle> {
        let genres = Self::resolve_genres(job);
        let mut wheel: VecDeque<String> = genres.iter().cloned().collect();

        let mut assignments = Vec::with_capacity(corpus.articles.len());
        for article in corpus.articles {
            if wheel.is_empty() {
                break;
            }
            let genre = wheel.pop_front().expect("genre wheel empty");
            assignments.push(GenreAssignment {
                genre: genre.clone(),
                article,
            });
            wheel.push_back(genre);
        }

        Ok(GenreBundle {
            job_id: job.job_id,
            assignments,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn article(title: &str) -> PreprocessedArticle {
        PreprocessedArticle {
            id: Uuid::new_v4(),
            title: title.into(),
            body: "body".into(),
            language: "en".into(),
        }
    }

    #[tokio::test]
    async fn distributes_articles_round_robin() {
        let stage = BalancedGenreStage::new();
        let job = JobContext::new(Uuid::new_v4(), vec!["ai".into(), "security".into()]);
        let corpus = DeduplicatedCorpus {
            job_id: job.job_id,
            articles: vec![article("A"), article("B"), article("C")],
        };

        let bundle = stage
            .assign(&job, corpus)
            .await
            .expect("genre cluster succeeds");

        assert_eq!(bundle.assignments.len(), 3);
        assert_eq!(bundle.assignments[0].genre, "ai");
        assert_eq!(bundle.assignments[1].genre, "security");
        assert_eq!(bundle.assignments[2].genre, "ai");
    }
}
