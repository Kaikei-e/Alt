use anyhow::{Context, Result};
use uuid::Uuid;

use super::{
    dedup::DeduplicatedCorpus, dispatch::DispatchResult, fetch::FetchedCorpus, genre::GenreBundle,
    persist::PersistResult, preprocess::PreprocessedCorpus, select::SelectedSummary,
};
use crate::pipeline::evidence::EvidenceBundle;
use crate::scheduler::JobContext;

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
    ) -> Result<FetchedCorpus> {
        if resume_stage_idx > 0 {
            if resume_stage_idx == 1 {
                self.load_state(job.job_id, "fetch").await
            } else {
                Ok(FetchedCorpus {
                    job_id: Uuid::nil(),
                    articles: vec![],
                })
            }
        } else {
            let res = self.orchestrator.stages().fetch.fetch(job).await?;
            self.save_state(job.job_id, "fetch", &res).await?;
            Ok(res)
        }
    }

    /// Preprocessステージを実行（リジューム対応）
    pub(crate) async fn execute_preprocess_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        fetched: FetchedCorpus,
    ) -> Result<PreprocessedCorpus> {
        if resume_stage_idx > 1 {
            if resume_stage_idx == 2 {
                self.load_state(job.job_id, "preprocess").await
            } else {
                Ok(PreprocessedCorpus {
                    job_id: Uuid::nil(),
                    articles: vec![],
                })
            }
        } else {
            let res = self
                .orchestrator
                .stages()
                .preprocess
                .preprocess(job, fetched)
                .await?;
            self.save_state(job.job_id, "preprocess", &res).await?;
            Ok(res)
        }
    }

    /// Dedupステージを実行（リジューム対応）
    pub(crate) async fn execute_dedup_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        preprocessed: PreprocessedCorpus,
    ) -> Result<DeduplicatedCorpus> {
        if resume_stage_idx > 2 {
            if resume_stage_idx == 3 {
                self.load_state(job.job_id, "dedup").await
            } else {
                Ok(DeduplicatedCorpus {
                    job_id: Uuid::nil(),
                    articles: vec![],
                    stats: super::dedup::DedupStats::default(),
                })
            }
        } else {
            let res = self
                .orchestrator
                .stages()
                .dedup
                .deduplicate(job, preprocessed)
                .await?;
            self.save_state(job.job_id, "dedup", &res).await?;
            Ok(res)
        }
    }

    /// Genreステージを実行（リジューム対応）
    pub(crate) async fn execute_genre_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        deduplicated: DeduplicatedCorpus,
    ) -> Result<GenreBundle> {
        if resume_stage_idx > 3 {
            if resume_stage_idx == 4 {
                self.load_state(job.job_id, "genre").await
            } else {
                Ok(GenreBundle {
                    job_id: job.job_id,
                    assignments: vec![],
                    genre_distribution: std::collections::HashMap::new(),
                })
            }
        } else {
            let res = self
                .orchestrator
                .stages()
                .genre
                .assign(job, deduplicated)
                .await?;
            self.save_state(job.job_id, "genre", &res).await?;
            Ok(res)
        }
    }

    /// Selectステージを実行（リジューム対応）
    pub(crate) async fn execute_select_stage(
        &self,
        job: &JobContext,
        resume_stage_idx: usize,
        genre_bundle: GenreBundle,
    ) -> Result<SelectedSummary> {
        if resume_stage_idx > 4 {
            if resume_stage_idx == 5 {
                self.load_state(job.job_id, "select").await
            } else {
                Ok(SelectedSummary {
                    job_id: Uuid::nil(),
                    assignments: vec![],
                })
            }
        } else {
            let res = self
                .orchestrator
                .stages()
                .select
                .select(job, genre_bundle)
                .await?;
            self.save_state(job.job_id, "select", &res).await?;
            Ok(res)
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
                self.load_state(job.job_id, "dispatch").await
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
            self.save_state(job.job_id, "dispatch", &res).await?;
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

    async fn save_state<T: serde::Serialize>(
        &self,
        job_id: Uuid,
        stage: &str,
        state_data: &T,
    ) -> Result<()> {
        let state_json =
            serde_json::to_value(state_data).context("failed to serialize stage state")?;
        self.orchestrator
            .recap_dao()
            .save_stage_state(job_id, stage, &state_json)
            .await?;
        self.orchestrator
            .recap_dao()
            .update_job_status(job_id, crate::store::dao::JobStatus::Running, Some(stage))
            .await?;

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
    /// SelectedSummaryからEvidenceBundleを構築
    pub(crate) fn build_evidence_bundle(
        job: &JobContext,
        resume_stage_idx: usize,
        selected: SelectedSummary,
    ) -> EvidenceBundle {
        if resume_stage_idx <= 5 {
            EvidenceBundle::from_genre_bundle(
                job.job_id,
                GenreBundle {
                    job_id: selected.job_id,
                    assignments: selected.assignments,
                    genre_distribution: std::collections::HashMap::new(),
                },
            )
        } else {
            EvidenceBundle {
                job_id: job.job_id,
                corpora: std::collections::HashMap::new(),
            }
        }
    }
}
