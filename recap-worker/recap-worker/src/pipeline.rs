use std::sync::Arc;

use anyhow::Result;

use crate::{
    clients::SubworkerClient, config::Config, scheduler::JobContext, store::dao::RecapDao,
};

pub(crate) mod dedup;
pub(crate) mod dispatch;
pub(crate) mod fetch;
pub(crate) mod genre;
pub(crate) mod persist;
pub(crate) mod preprocess;
pub(crate) mod select;

use dedup::{DedupStage, HashDedupStage};
use dispatch::{DispatchStage, NewsCreatorDispatchStage};
use fetch::{FetchStage, HttpFetchStage};
use genre::{BalancedGenreStage, GenreStage};
use persist::PersistStage;
use preprocess::{PreprocessStage, TextPreprocessStage};
use select::{SelectStage, SummarySelectStage};

pub(crate) struct PipelineOrchestrator {
    config: Arc<Config>,
    stages: PipelineStages,
}

struct PipelineStages {
    fetch: Arc<dyn FetchStage>,
    preprocess: Arc<dyn PreprocessStage>,
    dedup: Arc<dyn DedupStage>,
    genre: Arc<dyn GenreStage>,
    select: Arc<dyn SelectStage>,
    dispatch: Arc<dyn DispatchStage>,
    persist: Arc<dyn PersistStage>,
}

pub(crate) struct PipelineBuilder {
    config: Arc<Config>,
    fetch: Option<Arc<dyn FetchStage>>,
    preprocess: Option<Arc<dyn PreprocessStage>>,
    dedup: Option<Arc<dyn DedupStage>>,
    genre: Option<Arc<dyn GenreStage>>,
    select: Option<Arc<dyn SelectStage>>,
    dispatch: Option<Arc<dyn DispatchStage>>,
    persist: Option<Arc<dyn PersistStage>>,
}

impl PipelineOrchestrator {
    pub(crate) fn new(
        config: Arc<Config>,
        subworker: SubworkerClient,
        recap_dao: Arc<RecapDao>,
    ) -> Self {
        PipelineBuilder::new(config)
            .with_fetch_stage(Arc::new(HttpFetchStage::new(subworker)))
            .with_preprocess_stage(Arc::new(TextPreprocessStage::new()))
            .with_dedup_stage(Arc::new(HashDedupStage::new()))
            .with_genre_stage(Arc::new(BalancedGenreStage::new()))
            .with_select_stage(Arc::new(SummarySelectStage::new()))
            .with_dispatch_stage(Arc::new(NewsCreatorDispatchStage::new()))
            .with_persist_stage(Arc::new(persist::LoggingPersistStage::new(recap_dao)))
            .build()
    }

    #[cfg(test)]
    pub(crate) fn builder(config: Arc<Config>) -> PipelineBuilder {
        PipelineBuilder::new(config)
    }

    pub(crate) async fn execute(&self, job: &JobContext) -> Result<persist::PersistResult> {
        tracing::debug!(job_id = %job.job_id, prompt_version = %self.config.llm_prompt_version(), "recap pipeline started");
        let fetched = self.stages.fetch.fetch(job).await?;
        let preprocessed = self.stages.preprocess.preprocess(job, fetched).await?;
        let deduplicated = self.stages.dedup.deduplicate(job, preprocessed).await?;
        let genre_bundle = self.stages.genre.assign(job, deduplicated).await?;
        let selected = self.stages.select.select(job, genre_bundle).await?;
        let dispatched = self.stages.dispatch.dispatch(job, selected).await?;
        let persisted = self.stages.persist.persist(job, dispatched).await?;
        tracing::debug!(job_id = %job.job_id, stored = persisted.stored, "recap pipeline completed");
        Ok(persisted)
    }
}

impl PipelineBuilder {
    pub(crate) fn new(config: Arc<Config>) -> Self {
        Self {
            config,
            fetch: None,
            preprocess: None,
            dedup: None,
            genre: None,
            select: None,
            dispatch: None,
            persist: None,
        }
    }

    pub(crate) fn with_fetch_stage(mut self, stage: Arc<dyn FetchStage>) -> Self {
        self.fetch = Some(stage);
        self
    }

    pub(crate) fn with_preprocess_stage(mut self, stage: Arc<dyn PreprocessStage>) -> Self {
        self.preprocess = Some(stage);
        self
    }

    pub(crate) fn with_dedup_stage(mut self, stage: Arc<dyn DedupStage>) -> Self {
        self.dedup = Some(stage);
        self
    }

    pub(crate) fn with_genre_stage(mut self, stage: Arc<dyn GenreStage>) -> Self {
        self.genre = Some(stage);
        self
    }

    pub(crate) fn with_select_stage(mut self, stage: Arc<dyn SelectStage>) -> Self {
        self.select = Some(stage);
        self
    }

    pub(crate) fn with_dispatch_stage(mut self, stage: Arc<dyn DispatchStage>) -> Self {
        self.dispatch = Some(stage);
        self
    }

    pub(crate) fn with_persist_stage(mut self, stage: Arc<dyn PersistStage>) -> Self {
        self.persist = Some(stage);
        self
    }

    pub(crate) fn build(self) -> PipelineOrchestrator {
        let stages = PipelineStages {
            fetch: self
                .fetch
                .unwrap_or_else(|| panic!("fetch stage must be configured before build")),
            preprocess: self
                .preprocess
                .unwrap_or_else(|| Arc::new(TextPreprocessStage::new())),
            dedup: self
                .dedup
                .unwrap_or_else(|| Arc::new(HashDedupStage::new())),
            genre: self
                .genre
                .unwrap_or_else(|| Arc::new(BalancedGenreStage::new())),
            select: self
                .select
                .unwrap_or_else(|| Arc::new(SummarySelectStage::new())),
            dispatch: self
                .dispatch
                .unwrap_or_else(|| Arc::new(NewsCreatorDispatchStage::new())),
            persist: self
                .persist
                .unwrap_or_else(|| Arc::new(persist::UnimplementedPersistStage)),
        };

        PipelineOrchestrator {
            config: self.config,
            stages,
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use uuid::Uuid;

    use super::*;
    use crate::config::ENV_MUTEX;
    use crate::pipeline::{
        dedup::{DedupStage, DeduplicatedCorpus},
        dispatch::{DispatchResult, DispatchStage},
        fetch::{FetchStage, FetchedArticle, FetchedCorpus},
        genre::{GenreAssignment, GenreBundle, GenreStage},
        persist::{PersistResult, PersistStage},
        preprocess::{PreprocessStage, PreprocessedArticle, PreprocessedCorpus},
        select::{SelectStage, SelectedSummary},
    };
    use crate::scheduler::JobContext;

    fn setup_config() -> Arc<Config> {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        // SAFETY: tests adjust environment variables in a controlled manner.
        unsafe {
            std::env::set_var(
                "RECAP_DB_DSN",
                "postgres://recap:recap@localhost:5999/recap_db",
            );
            std::env::set_var("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
            std::env::set_var("SUBWORKER_BASE_URL", "http://localhost:8002/");
        }
        Arc::new(Config::from_env().expect("config should load for tests"))
    }

    #[tokio::test]
    async fn orchestrator_runs_stages_in_order() {
        let order: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
        let config = setup_config();

        let pipeline = PipelineOrchestrator::builder(Arc::clone(&config))
            .with_fetch_stage(Arc::new(RecordingFetch::new(Arc::clone(&order))))
            .with_preprocess_stage(Arc::new(RecordingPreprocess::new(Arc::clone(&order))))
            .with_dedup_stage(Arc::new(RecordingDedup::new(Arc::clone(&order))))
            .with_genre_stage(Arc::new(RecordingGenre::new(Arc::clone(&order))))
            .with_select_stage(Arc::new(RecordingSelect::new(Arc::clone(&order))))
            .with_dispatch_stage(Arc::new(RecordingDispatch::new(Arc::clone(&order))))
            .with_persist_stage(Arc::new(RecordingPersist::new(Arc::clone(&order))))
            .build();

        let job = JobContext::new(Uuid::new_v4(), vec!["ai".to_string()]);

        let result = pipeline
            .execute(&job)
            .await
            .expect("pipeline should succeed");

        assert!(result.stored);

        let stages = order.lock().expect("order lock").clone();
        assert_eq!(
            stages,
            vec![
                "fetch",
                "preprocess",
                "dedup",
                "genre",
                "select",
                "dispatch",
                "persist",
            ]
        );
    }

    struct RecordingFetch {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingFetch {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl FetchStage for RecordingFetch {
        async fn fetch(&self, job: &JobContext) -> anyhow::Result<FetchedCorpus> {
            self.order.lock().expect("order lock").push("fetch");
            Ok(FetchedCorpus {
                job_id: job.job_id,
                articles: vec![FetchedArticle {
                    id: Uuid::new_v4(),
                    title: "Title".into(),
                    body: "Body".into(),
                    language: Some("en".into()),
                }],
            })
        }
    }

    struct RecordingPreprocess {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingPreprocess {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl PreprocessStage for RecordingPreprocess {
        async fn preprocess(
            &self,
            job: &JobContext,
            corpus: FetchedCorpus,
        ) -> anyhow::Result<PreprocessedCorpus> {
            assert_eq!(corpus.articles.len(), 1);
            self.order.lock().expect("order lock").push("preprocess");
            Ok(PreprocessedCorpus {
                job_id: job.job_id,
                articles: vec![PreprocessedArticle {
                    id: Uuid::new_v4(),
                    title: "Title".into(),
                    body: "Processed".into(),
                    language: "en".into(),
                }],
            })
        }
    }

    struct RecordingDedup {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingDedup {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl DedupStage for RecordingDedup {
        async fn deduplicate(
            &self,
            _job: &JobContext,
            corpus: PreprocessedCorpus,
        ) -> anyhow::Result<DeduplicatedCorpus> {
            assert_eq!(corpus.articles.len(), 1);
            self.order.lock().expect("order lock").push("dedup");
            Ok(DeduplicatedCorpus {
                job_id: corpus.job_id,
                articles: corpus.articles,
            })
        }
    }

    struct RecordingGenre {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingGenre {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl GenreStage for RecordingGenre {
        async fn assign(
            &self,
            _job: &JobContext,
            corpus: DeduplicatedCorpus,
        ) -> anyhow::Result<GenreBundle> {
            assert_eq!(corpus.articles.len(), 1);
            self.order.lock().expect("order lock").push("genre");
            Ok(GenreBundle {
                job_id: corpus.job_id,
                assignments: vec![GenreAssignment {
                    genre: "science".into(),
                    article: corpus.articles.into_iter().next().expect("article"),
                }],
            })
        }
    }

    struct RecordingSelect {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingSelect {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl SelectStage for RecordingSelect {
        async fn select(
            &self,
            _job: &JobContext,
            bundle: GenreBundle,
        ) -> anyhow::Result<SelectedSummary> {
            assert_eq!(bundle.assignments.len(), 1);
            self.order.lock().expect("order lock").push("select");
            Ok(SelectedSummary {
                job_id: bundle.job_id,
                assignments: bundle.assignments,
            })
        }
    }

    struct RecordingDispatch {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingDispatch {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl DispatchStage for RecordingDispatch {
        async fn dispatch(
            &self,
            _job: &JobContext,
            summary: SelectedSummary,
        ) -> anyhow::Result<DispatchResult> {
            assert_eq!(summary.assignments.len(), 1);
            self.order.lock().expect("order lock").push("dispatch");
            Ok(DispatchResult {
                job_id: summary.job_id,
                response_id: Some("resp-123".to_string()),
            })
        }
    }

    struct RecordingPersist {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingPersist {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl PersistStage for RecordingPersist {
        async fn persist(
            &self,
            job: &JobContext,
            result: DispatchResult,
        ) -> anyhow::Result<PersistResult> {
            assert_eq!(result.job_id, job.job_id);
            assert_eq!(result.response_id.as_deref(), Some("resp-123"));
            self.order.lock().expect("order lock").push("persist");
            Ok(PersistResult { stored: true })
        }
    }
}
