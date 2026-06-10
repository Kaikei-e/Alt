use std::time::Duration;

use chrono::{FixedOffset, Utc};
use tokio::{task::JoinHandle, time::sleep};
use tracing::{error, info, warn};
use uuid::Uuid;

use crate::config::KnowledgeOwnerIds;
use crate::scheduler::{JobContext, Scheduler, cadence::DailyCadence};

const JST_OFFSET_HOURS: i32 = 9;
const BATCH_HOUR: u32 = 2;
const BATCH_MINUTE: u32 = 0;

/// Bind the resolved knowledge-loop owner onto a freshly-built `JobContext`.
///
/// When an owner is present the context gains `user_id` + `tenant_id`, which
/// is the precondition the persist stage checks before emitting
/// `recap.topic_snapshotted.v1`. With no owner the context stays scopeless and
/// emission is left off — the legitimate "intentionally disabled" path.
fn apply_knowledge_owner(ctx: JobContext, owner: Option<KnowledgeOwnerIds>) -> JobContext {
    match owner {
        Some(owner) => ctx.with_user_scope(owner.user_id, owner.tenant_id),
        None => ctx,
    }
}

/// Loud startup log of the topic-snapshot emit wiring state (CLAUDE.md #8).
/// Emission being off is a legitimate per-deployment choice, so we log rather
/// than panic — but the state is always surfaced, never silent.
fn log_topic_snapshot_emit_wiring(owner: Option<KnowledgeOwnerIds>, daemon: &'static str) {
    if let Some(owner) = owner {
        info!(
            daemon,
            owner_user_id = %owner.user_id,
            "recap_topic_snapshot_emit_enabled"
        );
    } else {
        warn!(
            daemon,
            "recap_topic_snapshot_emit_disabled: RECAP_KNOWLEDGE_OWNER_USER_ID / RECAP_KNOWLEDGE_OWNER_TENANT_ID not both set — recap.topic_snapshotted.v1 will not be emitted"
        );
    }
}

pub fn spawn_jst_batch_daemon(
    scheduler: Scheduler,
    genres: Vec<String>,
    window_days: u32,
    owner: Option<KnowledgeOwnerIds>,
) -> JoinHandle<()> {
    log_topic_snapshot_emit_wiring(owner, "jst_batch");
    let tz = FixedOffset::east_opt(JST_OFFSET_HOURS * 3600).expect("valid JST offset");
    let cadence = DailyCadence::new(tz, BATCH_HOUR, BATCH_MINUTE);
    BatchDaemon::new(scheduler, cadence, genres, tz, window_days, owner).spawn()
}

struct BatchDaemon {
    scheduler: Scheduler,
    cadence: DailyCadence,
    genres: Vec<String>,
    tz: FixedOffset,
    window_days: u32,
    owner: Option<KnowledgeOwnerIds>,
}

impl BatchDaemon {
    fn new(
        scheduler: Scheduler,
        cadence: DailyCadence,
        genres: Vec<String>,
        tz: FixedOffset,
        window_days: u32,
        owner: Option<KnowledgeOwnerIds>,
    ) -> Self {
        Self {
            scheduler,
            cadence,
            genres,
            tz,
            window_days,
            owner,
        }
    }

    fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    async fn run(self) {
        let state = self;

        // Boot-time job hygiene (順序が大事):
        //   1. `find_resumable_job` で「fresh で再開する価値のある 1 件」を選ぶ。
        //   2. **その 1 件以外** の pending/running を全部 `failed` に sweep。
        //      プロセスが起動し直したということは前プロセスは死んでいる
        //      ので、選ばれなかった in-flight 行は定義上全て orphan。
        //   3. 保持期間 (`RECAP_JOB_RETENTION_DAYS`) を超えた古い行を削除。
        //      daemon が当日走らない日は cleanup が呼ばれず累積する問題への
        //      対策で、起動時に必ず一度叩く。
        let resumable_target = state.scheduler.find_resumable_job().await.ok().flatten();
        let keep_job_id = resumable_target.as_ref().map(|(id, _, _, _)| *id);
        match state.scheduler.mark_abandoned_jobs(keep_job_id).await {
            Ok(0) => {}
            Ok(n) => info!(
                marked_failed = n,
                ?keep_job_id,
                "boot-time hygiene: orphaned jobs sealed"
            ),
            Err(err) => error!(error = %err, "boot-time hygiene: mark_abandoned_jobs failed"),
        }
        match state.scheduler.cleanup_old_jobs().await {
            Ok(0) => {}
            Ok(n) => info!(deleted = n, "boot-time hygiene: old jobs deleted"),
            Err(err) => error!(error = %err, "boot-time hygiene: cleanup_old_jobs failed"),
        }

        // 起動時に中断されたジョブの再開
        if let Some((job_id, status, last_stage, resumed_window_days)) = resumable_target {
            info!(
                %job_id,
                ?status,
                ?last_stage,
                resumed_window_days,
                "found resumable job, resuming..."
            );

            let mut job = apply_knowledge_owner(
                JobContext::new_with_window(job_id, state.genres.clone(), resumed_window_days),
                state.owner,
            );
            if let Some(stage) = last_stage {
                job = job.with_stage(stage);
            }

            match state.scheduler.run_job(job).await {
                Ok(()) => info!(%job_id, "resumed job completed"),
                Err(err) => error!(%job_id, error = %err, "resumed job failed"),
            }
        }

        loop {
            let now = Utc::now();
            let next = state.cadence.next_run_from(now);
            let wait = duration_until(next, now);
            let next_local = next.with_timezone(&state.tz);
            info!(
                next_run_utc = %next.to_rfc3339(),
                next_run_jst = %next_local.to_rfc3339(),
                wait_seconds = wait.as_secs(),
                window_days = state.window_days,
                "scheduled automatic {}-day recap batch", state.window_days
            );
            sleep(wait).await;

            // Start new job
            let job_id = Uuid::new_v4();
            let job = apply_knowledge_owner(
                JobContext::new_with_window(job_id, state.genres.clone(), state.window_days),
                state.owner,
            );
            match state.scheduler.run_job(job).await {
                Ok(()) => info!(
                    %job_id,
                    genres = state.genres.len(),
                    "automatic recap batch completed"
                ),
                Err(err) => error!(%job_id, error = %err, "automatic recap batch failed"),
            }

            // Clean up old jobs after batch execution
            if let Err(err) = state.scheduler.cleanup_old_jobs().await {
                error!(error = %err, "failed to cleanup old jobs");
            }
        }
    }
}

pub fn spawn_morning_update_daemon(
    scheduler: Scheduler,
    owner: Option<KnowledgeOwnerIds>,
) -> JoinHandle<()> {
    log_topic_snapshot_emit_wiring(owner, "morning_update");
    MorningUpdateDaemon::new(scheduler, owner).spawn()
}

struct MorningUpdateDaemon {
    scheduler: Scheduler,
    owner: Option<KnowledgeOwnerIds>,
}

impl MorningUpdateDaemon {
    fn new(scheduler: Scheduler, owner: Option<KnowledgeOwnerIds>) -> Self {
        Self { scheduler, owner }
    }

    fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    async fn run(self) {
        let interval = Duration::from_mins(30);
        loop {
            sleep(interval).await;
            let job_id = Uuid::new_v4();
            // trigger_source="morning" is written into recap_jobs so boot-time
            // find_resumable_job never picks this up as a batch Recap.
            let job = apply_knowledge_owner(JobContext::new_morning_update(job_id), self.owner);
            match self.scheduler.run_morning_update(job).await {
                Ok(()) => info!(%job_id, "morning update job completed"),
                Err(err) => error!(%job_id, error = %err, "morning update job failed"),
            }
        }
    }
}

fn duration_until(next: chrono::DateTime<Utc>, now: chrono::DateTime<Utc>) -> Duration {
    match (next - now).to_std() {
        Ok(duration) => duration,
        Err(_) => Duration::from_secs(0),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// RED→GREEN: when the daemon resolves a knowledge-loop owner, every
    /// JobContext it builds must carry that owner so the persist-stage
    /// `recap.topic_snapshotted.v1` emit guard `(Some, Some, _)` becomes
    /// satisfiable. Before PR-7 the owner was never threaded into the
    /// context, so the producer could never fire.
    #[test]
    fn apply_knowledge_owner_populates_user_and_tenant_scope() {
        let user_id = Uuid::new_v4();
        let tenant_id = Uuid::new_v4();
        let owner = Some(KnowledgeOwnerIds { user_id, tenant_id });

        let ctx = apply_knowledge_owner(JobContext::new(Uuid::new_v4(), vec![]), owner);

        assert_eq!(ctx.user_id(), Some(user_id));
        assert_eq!(ctx.tenant_id(), Some(tenant_id));
        // The exact predicate the persist stage checks before emitting.
        assert!(
            matches!((ctx.user_id(), ctx.tenant_id()), (Some(_), Some(_))),
            "topic-snapshot emit guard must be satisfiable once an owner is wired"
        );
    }

    /// Without a resolved owner the daemon must leave the context scopeless
    /// so the persist guard keeps emission off (the legitimate
    /// "intentionally disabled" deployment path).
    #[test]
    fn apply_knowledge_owner_leaves_scope_none_when_owner_absent() {
        let ctx = apply_knowledge_owner(JobContext::new(Uuid::new_v4(), vec![]), None);

        assert_eq!(ctx.user_id(), None);
        assert_eq!(ctx.tenant_id(), None);
    }
}
