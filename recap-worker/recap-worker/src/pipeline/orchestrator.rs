//! Pipeline orchestrator and builder for the recap pipeline.

use std::sync::Arc;

use anyhow::{Context, Result};

use crate::{
    clients::alt_backend::{AltBackendClient, AltBackendConfig},
    clients::{NewsCreatorClient, SubworkerClient, TagGeneratorClient},
    config::Config,
    observability::metrics::Metrics,
    queue::ClassificationJobQueue,
    scheduler::JobContext,
    store::dao::RecapDao,
    util::retry::RetryConfig,
};

use super::dedup::{DedupStage, HashDedupStage};
use super::dispatch::{DispatchStage, MlLlmDispatchStage};
use super::executor;
use super::fetch::{AltBackendFetchStage, FetchStage};
use super::genre::{CoarseGenreStage, GenreStage, RefineRollout, TwoStageGenreStage};
use super::genre_refine::{DbTagLabelGraphSource, DefaultRefineEngine, RefineConfig, TagLabelGraphSource};
use super::graph_override::GraphOverrideSettings;
use super::persist::{self, PersistStage};
use super::preprocess::{PreprocessStage, TextPreprocessStage};
use super::pulse::{DefaultPulseStage, PulseConfig, PulseRollout, PulseStage};
use super::select::{SelectStage, SubgenreConfig, SummarySelectStage};

/// Core pipeline orchestrator that coordinates all stages.
pub(crate) struct PipelineOrchestrator {
    pub(super) config: Arc<Config>,
    pub(super) stages: PipelineStages,
    pub(super) recap_dao: Arc<dyn RecapDao>,
    pub(super) subworker_client: Arc<SubworkerClient>,
    #[allow(dead_code)]
    pub(super) classification_queue: Arc<ClassificationJobQueue>,
    pub(super) pulse_stage: Arc<dyn PulseStage>,
    pub(super) pulse_rollout: PulseRollout,
}

impl PipelineOrchestrator {
    pub(crate) fn stages(&self) -> &PipelineStages {
        &self.stages
    }

    pub(crate) fn recap_dao(&self) -> &Arc<dyn RecapDao> {
        &self.recap_dao
    }
}

/// Container for all pipeline stages.
pub(crate) struct PipelineStages {
    pub(super) fetch: Arc<dyn FetchStage>,
    pub(super) preprocess: Arc<dyn PreprocessStage>,
    pub(super) dedup: Arc<dyn DedupStage>,
    pub(super) genre: Arc<dyn GenreStage>,
    pub(super) select: Arc<dyn SelectStage>,
    pub(super) dispatch: Arc<dyn DispatchStage>,
    pub(super) persist: Arc<dyn PersistStage>,
}

/// Builder pattern for constructing `PipelineOrchestrator`.
pub(crate) struct PipelineBuilder {
    config: Arc<Config>,
    fetch: Option<Arc<dyn FetchStage>>,
    preprocess: Option<Arc<dyn PreprocessStage>>,
    dedup: Option<Arc<dyn DedupStage>>,
    genre: Option<Arc<dyn GenreStage>>,
    select: Option<Arc<dyn SelectStage>>,
    dispatch: Option<Arc<dyn DispatchStage>>,
    persist: Option<Arc<dyn PersistStage>>,
    pulse_stage: Option<Arc<dyn PulseStage>>,
    pulse_rollout: Option<PulseRollout>,
}

impl PipelineOrchestrator {
    /// Create a new pipeline orchestrator with default stage implementations.
    #[allow(clippy::too_many_lines)]
    pub(crate) async fn new(
        config: Arc<Config>,
        subworker: SubworkerClient,
        news_creator: Arc<NewsCreatorClient>,
        recap_dao: Arc<dyn RecapDao>,
        classification_queue: Arc<ClassificationJobQueue>,
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

        let embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>> =
            crate::pipeline::embedding::EmbeddingService::new()
                .ok()
                .map(|s| Arc::new(s) as Arc<dyn crate::pipeline::embedding::Embedder>);

        if embedding_service.is_none() {
            tracing::warn!(
                "Embedding service failed to initialize. Falling back to keyword-only filtering."
            );
        }

        // Initialize Pulse stage and rollout
        let pulse_config = PulseConfig::from_env();
        let pulse_rollout = PulseRollout::from_env();
        let pulse_stage: Arc<dyn PulseStage> =
            Arc::new(DefaultPulseStage::new(pulse_config, pulse_rollout.clone()));

        use crate::pipeline::genre_remote::RemoteGenreStage;
        let coarse_stage = Arc::new(RemoteGenreStage::new(
            Arc::clone(&subworker_client),
            Arc::clone(&classification_queue),
        ));
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
        let subgenre_max_docs_per_genre = config.subgenre_max_docs_per_genre();
        let subgenre_target_docs_per_subgenre = config.subgenre_target_docs_per_subgenre();
        let subgenre_max_k = config.subgenre_max_k();

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
                embedding_service.clone(),
                min_documents_per_genre,
                coherence_similarity_threshold,
                Some(Arc::clone(&recap_dao)),
                Some(Arc::clone(&subworker_client)),
                SubgenreConfig::new(
                    subgenre_max_docs_per_genre,
                    subgenre_target_docs_per_subgenre,
                    subgenre_max_k,
                ),
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
            .with_pulse_stage(pulse_stage)
            .with_pulse_rollout(pulse_rollout)
            .build(
                Arc::clone(&recap_dao),
                subworker_client,
                Arc::clone(&classification_queue),
                embedding_service,
            ))
    }

    #[cfg(test)]
    pub(crate) fn builder(config: Arc<Config>) -> PipelineBuilder {
        PipelineBuilder::new(config)
    }

    /// Execute the full pipeline for a given job.
    pub(crate) async fn execute(&self, job: &JobContext) -> Result<persist::PersistResult> {
        tracing::debug!(job_id = %job.job_id, prompt_version = %self.config.llm_prompt_version(), "recap pipeline started");

        self.prepare_pipeline().await;

        let resume_stage_idx = Self::get_resume_stage_index(job);
        tracing::info!(
            job_id = %job.job_id,
            current_stage = ?job.current_stage,
            resume_idx = resume_stage_idx,
            "pipeline execution context determined"
        );

        let executor = executor::StageExecutor::new(self);
        let fetched = executor.execute_fetch_stage(job, resume_stage_idx).await?;
        let preprocessed = executor
            .execute_preprocess_stage(job, resume_stage_idx, fetched)
            .await?;
        let deduplicated = executor
            .execute_dedup_stage(job, resume_stage_idx, preprocessed)
            .await?;
        let genre_bundle = executor
            .execute_genre_stage(job, resume_stage_idx, deduplicated)
            .await?;
        let selected = executor
            .execute_select_stage(job, resume_stage_idx, genre_bundle)
            .await?;
        let evidence_bundle =
            executor::StageExecutor::build_evidence_bundle(job, resume_stage_idx, selected);
        // evidence ステージの状態遷移を記録（実際の状態保存は不要だが、Status History への記録が必要）
        if resume_stage_idx <= 5 {
            executor
                .record_stage_transition(job.job_id, "evidence")
                .await?;
        }
        let dispatched = executor
            .execute_dispatch_stage(job, resume_stage_idx, evidence_bundle)
            .await?;

        // Generate Evening Pulse after dispatch (before persist, so we have clustering data)
        self.generate_pulse_if_enabled(job, &dispatched).await;

        let persisted = executor
            .execute_persist_stage(job, resume_stage_idx, dispatched)
            .await?;

        tracing::debug!(
            job_id = %job.job_id,
            total_genres = persisted.total_genres,
            genres_stored = persisted.genres_stored,
            "recap pipeline completed"
        );
        Ok(persisted)
    }

    /// パイプライン実行前の初期化処理（グラフ更新と設定読み込み）
    async fn prepare_pipeline(&self) {
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
        let overrides_result = if let Some(pool) = self.recap_dao.pool() {
            GraphOverrideSettings::load_with_fallback(pool).await
        } else {
            // モックの場合、環境変数から読み込む
            GraphOverrideSettings::load_from_env()
        };
        match overrides_result {
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

    fn get_resume_stage_index(job: &JobContext) -> usize {
        let stages = [
            "fetch",
            "preprocess",
            "dedup",
            "genre",
            "select",
            "dispatch",
            "persist",
        ];
        match &job.current_stage {
            Some(stage) => stages.iter().position(|&s| s == stage).map_or(0, |i| i + 1),
            None => 0,
        }
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
            pulse_stage: None,
            pulse_rollout: None,
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

    pub(crate) fn with_pulse_stage(mut self, stage: Arc<dyn PulseStage>) -> Self {
        self.pulse_stage = Some(stage);
        self
    }

    pub(crate) fn with_pulse_rollout(mut self, rollout: PulseRollout) -> Self {
        self.pulse_rollout = Some(rollout);
        self
    }

    pub(crate) fn build(
        self,
        recap_dao: Arc<dyn RecapDao>,
        subworker_client: Arc<SubworkerClient>,
        classification_queue: Arc<ClassificationJobQueue>,
        embedding_service: Option<Arc<dyn crate::pipeline::embedding::Embedder>>,
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
                    embedding_service,
                    self.config.min_documents_per_genre(),
                    self.config.coherence_similarity_threshold(),
                    Some(recap_dao.clone()),
                    Some(Arc::clone(&subworker_client)),
                    SubgenreConfig::new(
                        self.config.subgenre_max_docs_per_genre(),
                        self.config.subgenre_target_docs_per_subgenre(),
                        self.config.subgenre_max_k(),
                    ),
                ))
            }),
            dispatch: self
                .dispatch
                .expect("dispatch stage must be configured before build"),
            persist: self
                .persist
                .unwrap_or_else(|| panic!("persist stage must be configured before build")),
        };

        // Initialize pulse stage and rollout with defaults if not provided
        let pulse_rollout = self.pulse_rollout.unwrap_or_default();
        let pulse_stage = self.pulse_stage.unwrap_or_else(|| {
            Arc::new(DefaultPulseStage::new(
                PulseConfig::default(),
                pulse_rollout.clone(),
            ))
        });

        PipelineOrchestrator {
            config: self.config,
            stages,
            recap_dao,
            subworker_client,
            classification_queue,
            pulse_stage,
            pulse_rollout,
        }
    }
}
