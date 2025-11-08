use async_trait::async_trait;
use uuid::Uuid;

use crate::scheduler::JobContext;

use super::genre::{GenreAssignment, GenreBundle};

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct SelectedSummary {
    pub(crate) job_id: Uuid,
    pub(crate) assignments: Vec<GenreAssignment>,
}

#[async_trait]
pub(crate) trait SelectStage: Send + Sync {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary>;
}

#[derive(Debug, Clone)]
pub(crate) struct SummarySelectStage {
    max_articles_per_genre: usize,
}

impl SummarySelectStage {
    pub(crate) fn new() -> Self {
        Self {
            max_articles_per_genre: 20,
        }
    }

    fn trim_assignments(&self, bundle: GenreBundle) -> Vec<GenreAssignment> {
        let mut per_genre_count = std::collections::HashMap::new();
        let mut selected = Vec::new();

        for assignment in bundle.assignments {
            // 最初のジャンルを使用（複数ジャンルがある場合は最初のもの）
            let primary_genre = assignment
                .genres
                .first()
                .cloned()
                .unwrap_or_else(|| "other".to_string());
            let count = per_genre_count
                .entry(primary_genre.clone())
                .or_insert(0usize);
            if *count >= self.max_articles_per_genre {
                continue;
            }
            *count += 1;
            selected.push(assignment);
        }

        selected
    }
}

impl Default for SummarySelectStage {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl SelectStage for SummarySelectStage {
    async fn select(
        &self,
        job: &JobContext,
        bundle: GenreBundle,
    ) -> anyhow::Result<SelectedSummary> {
        let assignments = self.trim_assignments(bundle);

        Ok(SelectedSummary {
            job_id: job.job_id,
            assignments,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn assignment(genre: &str) -> GenreAssignment {
        use super::super::dedup::DeduplicatedArticle;
        GenreAssignment {
            genres: vec![genre.to_string()],
            genre_scores: std::collections::HashMap::from([(genre.to_string(), 10)]),
            article: DeduplicatedArticle {
                id: Uuid::new_v4().to_string(),
                title: Some("title".to_string()),
                sentences: vec!["body".to_string()],
                sentence_hashes: vec![],
                language: "en".to_string(),
            },
        }
    }

    #[tokio::test]
    async fn trims_to_max_per_genre() {
        let stage = SummarySelectStage {
            max_articles_per_genre: 1,
        };
        let job = JobContext::new(Uuid::new_v4(), vec![]);
        let bundle = GenreBundle {
            job_id: job.job_id,
            assignments: vec![assignment("ai"), assignment("ai"), assignment("security")],
            genre_distribution: std::collections::HashMap::new(),
        };

        let selected = stage
            .select(&job, bundle)
            .await
            .expect("selection succeeds");

        assert_eq!(selected.assignments.len(), 2);
        assert!(
            selected
                .assignments
                .iter()
                .any(|a| a.genres.contains(&"ai".to_string()))
        );
    }
}
