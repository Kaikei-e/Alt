use std::collections::HashSet;

use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::preprocess::{PreprocessedArticle, PreprocessedCorpus};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DeduplicatedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<PreprocessedArticle>,
}

#[async_trait]
pub(crate) trait DedupStage: Send + Sync {
    async fn deduplicate(
        &self,
        job: &JobContext,
        corpus: PreprocessedCorpus,
    ) -> anyhow::Result<DeduplicatedCorpus>;
}

#[derive(Debug, Default, Clone)]
pub(crate) struct HashDedupStage;

impl HashDedupStage {
    pub(crate) fn new() -> Self {
        Self
    }
}

#[async_trait]
impl DedupStage for HashDedupStage {
    async fn deduplicate(
        &self,
        job: &JobContext,
        corpus: PreprocessedCorpus,
    ) -> anyhow::Result<DeduplicatedCorpus> {
        let mut seen = HashSet::new();
        let mut articles = Vec::with_capacity(corpus.articles.len());

        for article in corpus.articles {
            let fingerprint = normalize_text(&article.body);
            if seen.insert(fingerprint) {
                articles.push(article);
            }
        }

        Ok(DeduplicatedCorpus {
            job_id: job.job_id,
            articles,
        })
    }
}

fn normalize_text(text: &str) -> String {
    text.split_whitespace()
        .map(|fragment| fragment.to_lowercase())
        .collect::<Vec<_>>()
        .join(" ")
}

#[cfg(test)]
mod tests {
    use super::*;

    fn article(body: &str) -> PreprocessedArticle {
        PreprocessedArticle {
            id: Uuid::new_v4(),
            title: "title".into(),
            body: body.into(),
            language: "en".into(),
        }
    }

    #[tokio::test]
    async fn deduplicate_filters_duplicates() {
        let stage = HashDedupStage::new();
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let corpus = PreprocessedCorpus {
            job_id: job.job_id,
            articles: vec![article("Text"), article("text  "), article("Another")],
        };

        let result = stage
            .deduplicate(&job, corpus)
            .await
            .expect("dedup succeeds");

        assert_eq!(result.articles.len(), 2);
    }
}
