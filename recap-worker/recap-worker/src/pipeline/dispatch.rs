use std::sync::Arc;

use async_trait::async_trait;
use serde::Serialize;
use tracing::instrument;
use uuid::Uuid;

use crate::{clients::NewsCreatorClient, scheduler::JobContext};

use super::select::SelectedSummary;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DispatchResult {
    pub(crate) job_id: Uuid,
    pub(crate) response_id: Option<String>,
}

#[async_trait]
pub(crate) trait DispatchStage: Send + Sync {
    async fn dispatch(
        &self,
        job: &JobContext,
        summary: SelectedSummary,
    ) -> anyhow::Result<DispatchResult>;
}

#[derive(Debug, Clone)]
pub(crate) struct NewsCreatorDispatchStage {
    client: Arc<NewsCreatorClient>,
}

impl NewsCreatorDispatchStage {
    pub(crate) fn new(client: Arc<NewsCreatorClient>) -> Self {
        Self { client }
    }

    fn build_payload(&self, summary: &SelectedSummary) -> LlmPayload {
        let mut grouped: std::collections::HashMap<&str, Vec<LlmEvidenceArticle>> =
            std::collections::HashMap::new();

        for assignment in &summary.assignments {
            grouped
                .entry(assignment.genre.as_str())
                .or_default()
                .push(LlmEvidenceArticle {
                    article_id: assignment.article.id,
                    title: assignment.article.title.clone(),
                    body: assignment.article.body.clone(),
                    language: assignment.article.language.clone(),
                });
        }

        let mut genres = Vec::new();
        for (genre, articles) in grouped {
            genres.push(LlmGenrePayload {
                genre: genre.to_string(),
                articles,
            });
        }

        LlmPayload {
            job_id: summary.job_id,
            genres,
        }
    }
}

#[async_trait]
impl DispatchStage for NewsCreatorDispatchStage {
    #[instrument(skip_all, fields(job_id = %job.job_id))]
    async fn dispatch(
        &self,
        job: &JobContext,
        summary: SelectedSummary,
    ) -> anyhow::Result<DispatchResult> {
        if summary.assignments.is_empty() {
            return Ok(DispatchResult {
                job_id: job.job_id,
                response_id: None,
            });
        }

        let client = NewsCreatorClient::get();
        let payload = self.build_payload(&summary);
        let response = client.summarize(payload).await?;

        Ok(DispatchResult {
            job_id: job.job_id,
            response_id: Some(response.response_id),
        })
    }
}

#[derive(Debug, Serialize)]
struct LlmPayload {
    job_id: Uuid,
    genres: Vec<LlmGenrePayload>,
}

#[derive(Debug, Serialize)]
struct LlmGenrePayload {
    genre: String,
    articles: Vec<LlmEvidenceArticle>,
}

#[derive(Debug, Serialize)]
struct LlmEvidenceArticle {
    article_id: Uuid,
    title: String,
    body: String,
    language: String,
}
