use async_trait::async_trait;
use uuid::Uuid;

use crate::{
    clients::subworker::{SubworkerClient, SubworkerCorpus},
    scheduler::JobContext,
};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FetchedArticle {
    pub(crate) id: Uuid,
    pub(crate) title: String,
    pub(crate) body: String,
    pub(crate) language: Option<String>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct FetchedCorpus {
    pub(crate) job_id: Uuid,
    pub(crate) articles: Vec<FetchedArticle>,
}

#[async_trait]
pub(crate) trait FetchStage: Send + Sync {
    async fn fetch(&self, job: &JobContext) -> anyhow::Result<FetchedCorpus>;
}

pub(crate) struct HttpFetchStage {
    subworker: SubworkerClient,
}

impl HttpFetchStage {
    pub(crate) fn new(subworker: SubworkerClient) -> Self {
        Self { subworker }
    }
}

#[async_trait]
impl FetchStage for HttpFetchStage {
    async fn fetch(&self, job: &JobContext) -> anyhow::Result<FetchedCorpus> {
        let corpus = self.subworker.fetch_corpus(job.job_id).await?;
        Ok(map_corpus(corpus))
    }
}

fn map_corpus(corpus: SubworkerCorpus) -> FetchedCorpus {
    FetchedCorpus {
        job_id: corpus.job_id,
        articles: corpus
            .articles
            .into_iter()
            .map(|article| FetchedArticle {
                id: article.id,
                title: article.title,
                body: article.body,
                language: article.language,
            })
            .collect(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::clients::subworker::SubworkerArticle;

    #[test]
    fn map_corpus_transforms_articles() {
        let job_id = Uuid::new_v4();
        let article_id = Uuid::new_v4();
        let corpus = SubworkerCorpus {
            job_id,
            articles: vec![SubworkerArticle {
                id: article_id,
                title: "Title".to_string(),
                body: "Body".to_string(),
                language: Some("en".to_string()),
            }],
        };

        let mapped = map_corpus(corpus);

        assert_eq!(mapped.job_id, job_id);
        assert_eq!(mapped.articles.len(), 1);
        let article = &mapped.articles[0];
        assert_eq!(article.id, article_id);
        assert_eq!(article.title, "Title");
        assert_eq!(article.body, "Body");
        assert_eq!(article.language.as_deref(), Some("en"));
    }
}
