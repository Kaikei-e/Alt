use std::sync::Arc;

use anyhow::{Context, Result};

use crate::pipeline::evidence::EvidenceBundle;
use crate::{
    clients::alt_backend::{AltBackendClient, AltBackendConfig},
    clients::{NewsCreatorClient, SubworkerClient, TagGeneratorClient},
    config::Config,
    observability::metrics::Metrics,
    scheduler::JobContext,
    store::dao::RecapDao,
    util::retry::RetryConfig,
};

pub mod dedup;
pub(crate) mod dispatch;
pub(crate) mod embedding;
pub(crate) mod evidence;
pub(crate) mod fetch;
pub(crate) mod genre;
pub(crate) mod genre_canonical;
pub(crate) mod genre_keywords;
pub(crate) mod genre_refine;
pub(crate) mod genre_remote;
pub(crate) mod graph_override;
pub mod morning;
pub(crate) mod persist;
pub mod preprocess;
pub(crate) mod select;
pub(crate) mod tag_signal;

use dedup::{DedupStage, HashDedupStage};
use dispatch::{DispatchStage, MlLlmDispatchStage};
use fetch::{AltBackendFetchStage, FetchStage};
use genre::{CoarseGenreStage, GenreStage, RefineRollout, TwoStageGenreStage};
use genre_refine::{DbTagLabelGraphSource, DefaultRefineEngine, RefineConfig, TagLabelGraphSource};
use graph_override::GraphOverrideSettings;
use persist::PersistStage;
use preprocess::{PreprocessStage, TextPreprocessStage};
use select::{SelectStage, SummarySelectStage};

pub(crate) struct PipelineOrchestrator {
    config: Arc<Config>,
    stages: PipelineStages,
    recap_dao: Arc<RecapDao>,
    subworker_client: Arc<SubworkerClient>,
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
    #[allow(clippy::too_many_lines)]
    pub(crate) async fn new(
        config: Arc<Config>,
        subworker: SubworkerClient,
        news_creator: Arc<NewsCreatorClient>,
        recap_dao: Arc<RecapDao>,
        metrics: Arc<Metrics>,
    ) -> Result<Self> {
        let alt_backend_config = AltBackendConfig {
            base_url: config.alt_backend_base_url().to_string(),
            connect_timeout: config.alt_backend_connect_timeout(),
            total_timeout: config.alt_backend_total_timeout(),
            service_token: config.alt_backend_service_token().map(ToString::to_string),
        };
        let alt_backend_client = Arc::new(
            AltBackendClient::new(alt_backend_config).expect("failed to create alt-backend client"),
        );
        let tag_generator_config = crate::clients::tag_generator::TagGeneratorConfig {
            base_url: config.tag_generator_base_url().to_string(),
            connect_timeout: config.tag_generator_connect_timeout(),
            total_timeout: config.tag_generator_total_timeout(),
            service_token: config
                .tag_generator_service_token()
                .map(ToString::to_string),
        };
        let tag_generator_client = TagGeneratorClient::new(tag_generator_config)
            .ok()
            .map(Arc::new);
        let retry_config = RetryConfig {
            max_attempts: config.http_max_retries(),
            base_delay_ms: config.http_backoff_base_ms(),
            max_delay_ms: config.http_backoff_cap_ms(),
        };
        let subworker_client = Arc::new(subworker);
        let cpu_count = num_cpus::get();
        let max_concurrent = (cpu_count * 3) / 2;
        let window_days = config.recap_window_days();

        let embedding_service = crate::pipeline::embedding::EmbeddingService::new().ok();
        if embedding_service.is_none() {
            tracing::warn!(
                "Embedding service failed to initialize. Falling back to keyword-only filtering."
            );
        }

        use crate::pipeline::genre_remote::RemoteGenreStage;
        let coarse_stage = Arc::new(RemoteGenreStage::new(Arc::clone(&subworker_client)));
        let rollout = RefineRollout::new(config.genre_refine_rollout_pct());
        let genre_stage: Arc<dyn GenreStage> = if config.genre_refine_enabled() {
            // デフォルト設定でRefineConfigを初期化（実行時に動的に更新される）
            let refine_config = RefineConfig::new(config.genre_refine_require_tags());
            let graph_loader = Arc::new(DbTagLabelGraphSource::new(
                Arc::clone(&recap_dao),
                config.tag_label_graph_window().to_string(),
                config.tag_label_graph_ttl(),
            ));
            graph_loader
                .preload()
                .await
                .context("failed to preload tag label graph cache")?;
            let graph_source: Arc<dyn TagLabelGraphSource> = graph_loader;
            let refine_engine = Arc::new(DefaultRefineEngine::new(refine_config, graph_source));
            Arc::new(TwoStageGenreStage::new(
                Arc::clone(&coarse_stage) as Arc<dyn GenreStage>,
                refine_engine,
                Arc::clone(&recap_dao),
                config.genre_refine_require_tags(),
                rollout.clone(),
                Arc::clone(&metrics),
            ))
        } else {
            coarse_stage as Arc<dyn GenreStage>
        };

        let min_documents_per_genre = config.min_documents_per_genre();
        let coherence_similarity_threshold = config.coherence_similarity_threshold();

        let config_for_dispatch = Arc::clone(&config);
        Ok(PipelineBuilder::new(config)
            .with_fetch_stage(Arc::new(AltBackendFetchStage::new(
                alt_backend_client,
                tag_generator_client,
                Arc::clone(&recap_dao),
                retry_config,
                window_days,
            )))
            .with_preprocess_stage(Arc::new(TextPreprocessStage::new(
                max_concurrent.max(2),
                Arc::clone(&recap_dao),
                Arc::clone(&subworker_client),
            )))
            .with_dedup_stage(Arc::new(HashDedupStage::new(cpu_count.max(2), 0.8, 100)))
            .with_genre_stage(genre_stage)
            .with_select_stage(Arc::new(SummarySelectStage::new(
                embedding_service,
                min_documents_per_genre,
                coherence_similarity_threshold,
                Some(Arc::clone(&recap_dao)),
                Some(Arc::clone(&subworker_client)),
            )))
            .with_dispatch_stage(Arc::new(MlLlmDispatchStage::new(
                Arc::clone(&subworker_client),
                news_creator,
                Arc::clone(&recap_dao),
                max_concurrent,
                config_for_dispatch,
            )))
            .with_persist_stage(Arc::new(persist::FinalSectionPersistStage::new(
                Arc::clone(&recap_dao),
            )))
            .build(Arc::clone(&recap_dao), subworker_client))
    }

    #[cfg(test)]
    pub(crate) fn builder(config: Arc<Config>) -> PipelineBuilder {
        PipelineBuilder::new(config)
    }

    pub(crate) async fn execute(&self, job: &JobContext) -> Result<persist::PersistResult> {
        tracing::debug!(job_id = %job.job_id, prompt_version = %self.config.llm_prompt_version(), "recap pipeline started");

        // グラフ最新化（失敗してもパイプラインは続行）
        if self.config.recap_pre_refresh_graph_enabled() {
            if let Err(err) = self.refresh_graph_before_pipeline().await {
                tracing::warn!(
                    error = ?err,
                    "graph refresh failed, continuing with existing graph"
                );
            }
        }

        // 設定を再読み込みしてGenreStageを更新
        match GraphOverrideSettings::load_with_fallback(self.recap_dao.pool()).await {
            Ok(overrides) => {
                self.stages.genre.update_config(&overrides).await;
                tracing::debug!("updated graph override settings from database");
            }
            Err(err) => {
                tracing::warn!(
                    error = ?err,
                    "failed to reload graph override config, continuing with current settings"
                );
            }
        }

        let fetched = self.stages.fetch.fetch(job).await?;
        let preprocessed = self.stages.preprocess.preprocess(job, fetched).await?;
        let deduplicated = self.stages.dedup.deduplicate(job, preprocessed).await?;
        let genre_bundle = self.stages.genre.assign(job, deduplicated).await?;
        let selected = self.stages.select.select(job, genre_bundle).await?;
        // SelectedSummaryからEvidenceBundleに変換
        use crate::pipeline::genre::GenreBundle;
        let evidence_bundle = EvidenceBundle::from_genre_bundle(
            job.job_id,
            GenreBundle {
                job_id: selected.job_id,
                assignments: selected.assignments,
                genre_distribution: std::collections::HashMap::new(),
            },
        );
        let dispatched = self.stages.dispatch.dispatch(job, evidence_bundle).await?;
        let persisted = self.stages.persist.persist(job, dispatched).await?;
        tracing::debug!(
            job_id = %job.job_id,
            total_genres = persisted.total_genres,
            genres_stored = persisted.genres_stored,
            genres_failed = persisted.genres_failed,
            genres_skipped = persisted.genres_skipped,
            genres_no_evidence = persisted.genres_no_evidence,
            "recap pipeline completed"
        );
        Ok(persisted)
    }

    /// パイプライン実行前にグラフを最新化する
    async fn refresh_graph_before_pipeline(&self) -> Result<()> {
        tracing::info!("refreshing graph before pipeline execution");

        let timeout = self.config.recap_pre_refresh_timeout();
        tokio::time::timeout(timeout, self.subworker_client.refresh_graph_and_learning())
            .await
            .context("graph refresh timed out")?
            .context("graph refresh failed")?;

        tracing::info!("graph refresh completed successfully");
        Ok(())
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

    pub(crate) fn build(
        self,
        recap_dao: Arc<RecapDao>,
        subworker_client: Arc<SubworkerClient>,
    ) -> PipelineOrchestrator {
        let stages = PipelineStages {
            fetch: self
                .fetch
                .unwrap_or_else(|| panic!("fetch stage must be configured before build")),
            preprocess: self
                .preprocess
                .unwrap_or_else(|| panic!("preprocess stage must be configured before build")),
            dedup: self
                .dedup
                .unwrap_or_else(|| panic!("dedup stage must be configured before build")),
            genre: self.genre.unwrap_or_else(|| {
                Arc::new(CoarseGenreStage::with_defaults(Arc::clone(
                    &subworker_client,
                )))
            }),
            select: self.select.unwrap_or_else(|| {
                Arc::new(SummarySelectStage::new(
                    None,
                    self.config.min_documents_per_genre(),
                    self.config.coherence_similarity_threshold(),
                    Some(recap_dao.clone()),
                    Some(Arc::clone(&subworker_client)),
                ))
            }),
            dispatch: self
                .dispatch
                .expect("dispatch stage must be configured before build"),
            persist: self
                .persist
                .unwrap_or_else(|| panic!("persist stage must be configured before build")),
        };

        PipelineOrchestrator {
            config: self.config,
            stages,
            recap_dao,
            subworker_client,
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use sqlx::postgres::PgPoolOptions;
    use uuid::Uuid;

    use super::*;
    use crate::config::ENV_MUTEX;
    use crate::pipeline::{
        dedup::{DedupStage, DeduplicatedCorpus},
        dispatch::{DispatchResult, DispatchStage},
        fetch::{FetchStage, FetchedArticle, FetchedCorpus},
        genre::{FeatureProfile, GenreAssignment, GenreBundle, GenreCandidate, GenreStage},
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
            std::env::set_var("ALT_BACKEND_BASE_URL", "http://localhost:9000/");
        }
        Arc::new(Config::from_env().expect("config should load for tests"))
    }

    #[tokio::test]
    async fn orchestrator_runs_stages_in_order() {
        let order: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
        let config = setup_config();

        // テスト用のダミーRecapDaoを作成
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect_lazy("postgres://recap:recap@localhost:5999/recap_db")
            .expect("failed to create test pool");
        let recap_dao = Arc::new(crate::store::dao::RecapDao::new(pool));

        // テスト用のダミーSubworkerClientを作成
        let subworker_client = Arc::new(
            SubworkerClient::new("http://localhost:8002/", 10)
                .expect("failed to create test subworker client"),
        );

        let pipeline = PipelineOrchestrator::builder(Arc::clone(&config))
            .with_fetch_stage(Arc::new(RecordingFetch::new(Arc::clone(&order))))
            .with_preprocess_stage(Arc::new(RecordingPreprocess::new(Arc::clone(&order))))
            .with_dedup_stage(Arc::new(RecordingDedup::new(Arc::clone(&order))))
            .with_genre_stage(Arc::new(RecordingGenre::new(Arc::clone(&order))))
            .with_select_stage(Arc::new(RecordingSelect::new(Arc::clone(&order))))
            .with_dispatch_stage(Arc::new(RecordingDispatch::new(Arc::clone(&order))))
            .with_persist_stage(Arc::new(RecordingPersist::new(Arc::clone(&order))))
            .build(recap_dao, subworker_client);

        let job = JobContext::new(Uuid::new_v4(), vec!["ai".to_string()]);

        let result = pipeline
            .execute(&job)
            .await
            .expect("pipeline should succeed");

        assert!(result.genres_stored > 0);

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
                    id: Uuid::new_v4().to_string(),
                    title: Some("Title".to_string()),
                    body: "Body".to_string(),
                    language: Some("en".to_string()),
                    published_at: None,
                    source_url: None,
                    tags: Vec::new(),
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
                    id: Uuid::new_v4().to_string(),
                    title: Some("Title".to_string()),
                    body: "Processed".to_string(),
                    language: "en".to_string(),
                    char_count: 9,
                    is_html_cleaned: false,
                    published_at: None,
                    tokens: vec!["processed".to_string()],
                    tags: Vec::new(),
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
                articles: corpus
                    .articles
                    .into_iter()
                    .map(|article| super::dedup::DeduplicatedArticle {
                        id: article.id,
                        title: article.title,
                        sentences: vec![article.body],
                        sentence_hashes: vec![],
                        language: article.language,
                        tags: Vec::new(),
                        duplicates: Vec::new(),
                    })
                    .collect(),
                stats: super::dedup::DedupStats::default(),
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
            let article = corpus.articles.into_iter().next().expect("article");
            Ok(GenreBundle {
                job_id: corpus.job_id,
                assignments: vec![GenreAssignment {
                    genres: vec!["science".to_string()],
                    candidates: vec![GenreCandidate {
                        name: "science".to_string(),
                        score: 0.82,
                        keyword_support: 5,
                        classifier_confidence: 0.8,
                    }],
                    genre_scores: std::collections::HashMap::from([("science".to_string(), 10)]),
                    genre_confidence: std::collections::HashMap::from([(
                        "science".to_string(),
                        0.8,
                    )]),
                    feature_profile: FeatureProfile::default(),
                    article,
                }],
                genre_distribution: std::collections::HashMap::from([("science".to_string(), 1)]),
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
            job: &JobContext,
            evidence: EvidenceBundle,
        ) -> anyhow::Result<DispatchResult> {
            assert_eq!(evidence.genres().len(), 1);
            self.order.lock().expect("order lock").push("dispatch");
            Ok(DispatchResult {
                job_id: job.job_id,
                genre_results: std::collections::HashMap::new(),
                success_count: 1,
                failure_count: 0,
                all_genres: vec![],
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
            self.order.lock().expect("order lock").push("persist");
            Ok(PersistResult {
                job_id: job.job_id,
                genres_stored: 1,
                genres_failed: 0,
                genres_skipped: 0,
                genres_no_evidence: 0,
                total_genres: result.all_genres.len(),
            })
        }
    }
}
