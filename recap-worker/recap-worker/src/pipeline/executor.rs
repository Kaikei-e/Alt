use anyhow::{Context, Result};
use uuid::Uuid;

use super::{
    dedup::DeduplicatedCorpus,
    dispatch::{DispatchResult, DispatchResultLightweight},
    fetch::{FetchedCorpus, FetchedCorpusLight},
    genre::GenreBundle,
    persist::PersistResult,
    preprocess::PreprocessedCorpus,
    select::SelectedSummary,
};
use crate::pipeline::evidence::EvidenceBundle;
use crate::pipeline::fetch::FetchedArticle;
use crate::scheduler::JobContext;

/// Explicit skip marker for resume paths that bypass a stage.
///
/// Prefer this over sentinel empty corpora (`Uuid::nil()` + empty `Vec`) so
/// a wiring mistake surfaces as `require()` rather than silently processing
/// nothing.
#[derive(Debug, Clone)]
pub(crate) enum StageInput<T> {
    Ready(T),
    Skipped,
}

impl<T> StageInput<T> {
    fn require(self, stage: &str) -> Result<T> {
        match self {
            Self::Ready(value) => Ok(value),
            Self::Skipped => Err(anyhow::anyhow!(
                "stage `{stage}` received StageInput::Skipped; resume wiring bug"
            )),
        }
    }
}

/// パイプラインステージ実行のヘルパー
pub(crate) struct StageExecutor<'a> {
    orchestrator: &'a super::PipelineOrchestrator,
}

impl<'a> StageExecutor<'a> {
    pub(crate) fn new(orchestrator: &'a super::PipelineOrchestrator) -> Self {
        Self { orchestrator }
    }

    /// Fetchステージを実行（リジューム対応）
    pub(crate) async fn execute_fetch_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
    ) -> Result<StageInput<FetchedCorpus>> {
        if resume_stage_idx > 0 {
            if resume_stage_idx == 1 {
                // 軽量版チェックポイントから再構築
                let lightweight: FetchedCorpusLight = self.load_state(job.job_id, "fetch").await?;
                Ok(StageInput::Ready(
                    self.reconstruct_fetched_corpus(job.job_id, &lightweight)
                        .await?,
                ))
            } else {
                Ok(StageInput::Skipped)
            }
        } else {
            let res = self.orchestrator.stages().fetch.fetch(job).await?;
            // 軽量版を保存（記事IDのみ）
            let lightweight = res.to_lightweight();
            self.save_state(job.job_id, "fetch", &lightweight).await?;
            Ok(StageInput::Ready(res))
        }
    }

    /// 軽量版チェックポイントからFetchedCorpusを再構築
    async fn reconstruct_fetched_corpus(
        &self,
        job_id: Uuid,
        lightweight: &FetchedCorpusLight,
    ) -> Result<FetchedCorpus> {
        // recap_job_articlesから記事データを取得
        let article_data: Vec<crate::store::dao::article::FetchedArticleData> = self
            .orchestrator
            .recap_dao()
            .get_articles_by_ids(job_id, &lightweight.article_ids)
            .await
            .context("failed to reconstruct articles from recap_job_articles")?;

        // FetchedArticleに変換（tagsは空）
        let articles: Vec<FetchedArticle> = article_data
            .into_iter()
            .map(|data| FetchedArticle {
                id: data.id,
                title: data.title,
                body: data.body,
                language: data.language,
                published_at: data.published_at,
                source_url: data.source_url,
                tags: vec![], // リジューム時はtagsは不要
            })
            .collect();

        Ok(FetchedCorpus { job_id, articles })
    }

    /// Preprocessステージを実行（リジューム対応）
    pub(crate) async fn execute_preprocess_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        fetched: StageInput<FetchedCorpus>,
    ) -> Result<StageInput<PreprocessedCorpus>> {
        if resume_stage_idx > 1 {
            if resume_stage_idx == 2 {
                Ok(StageInput::Ready(
                    self.load_state(job.job_id, "preprocess").await?,
                ))
            } else {
                Ok(StageInput::Skipped)
            }
        } else {
            let fetched = fetched.require("preprocess")?;
            let res = self
                .orchestrator
                .stages()
                .preprocess
                .preprocess(job, fetched)
                .await?;
            self.save_state(job.job_id, "preprocess", &res).await?;
            Ok(StageInput::Ready(res))
        }
    }

    /// Dedupステージを実行（リジューム対応）
    pub(crate) async fn execute_dedup_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        preprocessed: StageInput<PreprocessedCorpus>,
    ) -> Result<StageInput<DeduplicatedCorpus>> {
        if resume_stage_idx > 2 {
            if resume_stage_idx == 3 {
                Ok(StageInput::Ready(
                    self.load_state(job.job_id, "dedup").await?,
                ))
            } else {
                Ok(StageInput::Skipped)
            }
        } else {
            let preprocessed = preprocessed.require("dedup")?;
            let res = self
                .orchestrator
                .stages()
                .dedup
                .deduplicate(job, preprocessed)
                .await?;
            self.save_state(job.job_id, "dedup", &res).await?;
            Ok(StageInput::Ready(res))
        }
    }

    /// Genreステージを実行（リジューム対応）
    pub(crate) async fn execute_genre_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        deduplicated: StageInput<DeduplicatedCorpus>,
    ) -> Result<StageInput<GenreBundle>> {
        if resume_stage_idx > 3 {
            if resume_stage_idx == 4 {
                Ok(StageInput::Ready(
                    self.load_state(job.job_id, "genre").await?,
                ))
            } else {
                Ok(StageInput::Skipped)
            }
        } else {
            let deduplicated = deduplicated.require("genre")?;
            let res = self
                .orchestrator
                .stages()
                .genre
                .assign(job, deduplicated)
                .await?;
            self.save_state(job.job_id, "genre", &res).await?;
            Ok(StageInput::Ready(res))
        }
    }

    /// Selectステージを実行（リジューム対応）
    pub(crate) async fn execute_select_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        genre_bundle: StageInput<GenreBundle>,
    ) -> Result<StageInput<SelectedSummary>> {
        if resume_stage_idx > 4 {
            if resume_stage_idx == 5 {
                Ok(StageInput::Ready(
                    self.load_state(job.job_id, "select").await?,
                ))
            } else {
                Ok(StageInput::Skipped)
            }
        } else {
            let genre_bundle = genre_bundle.require("select")?;
            let res = self
                .orchestrator
                .stages()
                .select
                .select(job, genre_bundle)
                .await?;
            self.save_state(job.job_id, "select", &res).await?;
            Ok(StageInput::Ready(res))
        }
    }
}

impl StageExecutor<'_> {
    /// Dispatchステージを実行（リジューム対応）
    pub(crate) async fn execute_dispatch_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        evidence_bundle: EvidenceBundle,
    ) -> Result<DispatchResult> {
        if resume_stage_idx > 5 {
            if resume_stage_idx == 6 {
                // 軽量版を読み込んで、完全なDispatchResultに再構築
                let lightweight: DispatchResultLightweight =
                    self.load_state(job.job_id, "dispatch").await?;
                Ok(StageExecutor::reconstruct_dispatch_result(
                    job.job_id,
                    lightweight,
                ))
            } else {
                Err(anyhow::anyhow!("Pipeline already completed"))
            }
        } else {
            let res = self
                .orchestrator
                .stages()
                .dispatch
                .dispatch(job, evidence_bundle)
                .await?;
            // 軽量版に変換して保存（大きなデータを除外）
            let lightweight = res.to_lightweight();
            self.save_state(job.job_id, "dispatch", &lightweight)
                .await?;
            Ok(res)
        }
    }

    /// Persistステージを実行（リジューム対応）
    pub(crate) async fn execute_persist_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        dispatched: DispatchResult,
    ) -> Result<PersistResult> {
        if resume_stage_idx > 6 {
            Err(anyhow::anyhow!("Pipeline already completed"))
        } else {
            let res = self
                .orchestrator
                .stages()
                .persist
                .persist(job, dispatched)
                .await?;
            self.save_state(job.job_id, "persist", &res).await?;
            Ok(res)
        }
    }

    async fn load_state<T: serde::de::DeserializeOwned>(
        &self,
        job_id: Uuid,
        stage: &str,
    ) -> Result<T> {
        let state_json = self
            .orchestrator
            .recap_dao()
            .load_stage_state(job_id, stage)
            .await
            .context("failed to load stage state from dao")?
            .ok_or_else(|| anyhow::anyhow!("State not found for stage: {}", stage))?;

        serde_json::from_value(state_json).context("failed to deserialize stage state")
    }

    /// Save this stage's checkpoint state. On failure, records the failure
    /// against `stage` via `insert_failed_task` before propagating the
    /// error — previously only `execute_fetch_stage` did this explicitly,
    /// so a `save_state` failure in every other stage (preprocess/dedup/
    /// genre/select/dispatch) surfaced with the wrong stage label at the
    /// `Scheduler::run_job` catch-all (`context.current_stage` is never
    /// updated mid-pipeline, so it falls back to the generic
    /// `"pipeline_execution"` label there).
    async fn save_state<T: serde::Serialize>(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &T,
    ) -> Result<()> {
        if let Err(e) = self.save_state_inner(job_id, stage, state_data).await {
            let error_msg = format!("{e:#}");
            let _ = self
                .orchestrator
                .recap_dao()
                .insert_failed_task(job_id, stage, None, Some(&error_msg))
                .await;
            return Err(e).with_context(|| format!("failed to save {stage} stage state"));
        }
        Ok(())
    }

    async fn save_state_inner<T: serde::Serialize>(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &T,
    ) -> Result<()> {
        let state_json =
            serde_json::to_value(state_data).context("failed to serialize stage state")?;

        tracing::debug!(
            %job_id,
            %stage,
            "saving stage state to recap_stage_state"
        );

        self.orchestrator
            .recap_dao()
            .save_stage_state(job_id, stage, &state_json)
            .await?;

        tracing::debug!(
            %job_id,
            %stage,
            "updating job status in recap_jobs"
        );

        self.orchestrator
            .recap_dao()
            .update_job_status_with_history(
                job_id,
                crate::store::dao::JobStatus::Running,
                Some(stage),
                None,
            )
            .await?;

        tracing::debug!(
            %job_id,
            %stage,
            "stage state saved and job status updated successfully"
        );

        // Log stage completion
        if let Err(e) = self
            .orchestrator
            .recap_dao()
            .insert_stage_log(job_id, stage, "completed", None)
            .await
        {
            tracing::warn!(
                error = ?e,
                job_id = %job_id,
                stage = %stage,
                "failed to insert stage log"
            );
        }

        Ok(())
    }
}

impl StageExecutor<'_> {
    /// ステージの状態遷移をStatus Historyに記録する（状態データの保存なし）
    ///
    /// evidenceステージのように実際の状態保存は不要だが、
    /// フロントエンドでの表示一貫性のためにStatus Historyへの記録が必要な場合に使用
    pub(crate) async fn record_stage_transition(&self, job_id: Uuid, stage: &str) -> Result<()> {
        tracing::debug!(
            %job_id,
            %stage,
            "recording stage transition to status history"
        );

        self.orchestrator
            .recap_dao()
            .update_job_status_with_history(
                job_id,
                crate::store::dao::JobStatus::Running,
                Some(stage),
                None,
            )
            .await?;

        if let Err(e) = self
            .orchestrator
            .recap_dao()
            .insert_stage_log(job_id, stage, "completed", None)
            .await
        {
            tracing::warn!(
                error = ?e,
                job_id = %job_id,
                stage = %stage,
                "failed to insert stage log for transition"
            );
        }

        Ok(())
    }

    /// SelectedSummaryからEvidenceBundleを構築
    pub(crate) fn build_evidence_bundle(
        job: &JobContext,
        resume_stage_idx: usize,
        selected: StageInput<SelectedSummary>,
    ) -> Result<EvidenceBundle> {
        if resume_stage_idx <= 5 {
            let selected = selected.require("evidence")?;
            Ok(EvidenceBundle::from_genre_bundle(
                job.job_id,
                GenreBundle {
                    job_id: selected.job_id,
                    assignments: selected.assignments,
                    genre_distribution: std::collections::HashMap::new(),
                },
            ))
        } else {
            Ok(EvidenceBundle {
                job_id: job.job_id,
                corpora: std::collections::HashMap::new(),
            })
        }
    }

    /// 軽量版から完全なDispatchResultを再構築
    /// 注: clustering_responseとsummary_responseはNoneに設定される
    /// （persistステージでデータベースから再取得される）
    fn reconstruct_dispatch_result(
        job_id: Uuid,
        lightweight: DispatchResultLightweight,
    ) -> DispatchResult {
        use super::dispatch::GenreResult;
        use std::collections::HashMap;

        let genre_results: HashMap<String, GenreResult> = lightweight
            .genre_results
            .into_iter()
            .map(|(genre, light_result)| {
                (
                    genre.clone(),
                    GenreResult {
                        genre: light_result.genre,
                        clustering_response: None, // データベースから再取得される
                        summary_response_id: light_result.summary_response_id,
                        summary_response: None, // データベースから再取得される
                        error: light_result.error,
                        error_kind: light_result.error_kind,
                    },
                )
            })
            .collect();

        DispatchResult {
            job_id,
            genre_results,
            success_count: lightweight.success_count,
            failure_count: lightweight.failure_count,
            all_genres: lightweight.all_genres,
        }
    }
}
