use std::time::Duration;

use chrono::{FixedOffset, Utc};
use tokio::{task::JoinHandle, time::sleep};
use tokio_util::sync::CancellationToken;
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
/// Config validation refuses every half-wired combination at startup, so
/// `None` here can only mean the explicit RECAP_KNOWLEDGE_EMIT=false — a
/// legitimate per-deployment choice, logged rather than fatal.
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
            "recap_topic_snapshot_emit_disabled: RECAP_KNOWLEDGE_EMIT=false — recap.topic_snapshotted.v1 will not be emitted"
        );
    }
}

pub fn spawn_jst_batch_daemon(
    scheduler: Scheduler,
    genres: Vec<String>,
    window_days: u32,
    owner: Option<KnowledgeOwnerIds>,
    shutdown: CancellationToken,
) -> JoinHandle<()> {
    log_topic_snapshot_emit_wiring(owner, "jst_batch");
    let tz = FixedOffset::east_opt(JST_OFFSET_HOURS * 3600).expect("valid JST offset");
    let cadence = DailyCadence::new(tz, BATCH_HOUR, BATCH_MINUTE)
        .unwrap_or_else(|e| panic!("batch cadence: {e}"));
    BatchDaemon::new(scheduler, cadence, genres, tz, window_days, owner, shutdown).spawn()
}

struct BatchDaemon {
    scheduler: Scheduler,
    cadence: DailyCadence,
    genres: Vec<String>,
    tz: FixedOffset,
    window_days: u32,
    owner: Option<KnowledgeOwnerIds>,
    shutdown: CancellationToken,
}

impl BatchDaemon {
    fn new(
        scheduler: Scheduler,
        cadence: DailyCadence,
        genres: Vec<String>,
        tz: FixedOffset,
        window_days: u32,
        owner: Option<KnowledgeOwnerIds>,
        shutdown: CancellationToken,
    ) -> Self {
        Self {
            scheduler,
            cadence,
            genres,
            tz,
            window_days,
            owner,
            shutdown,
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
        //      DB エラーは「再開対象なし」と混同しない — その場合は
        //      mark_abandoned_jobs sweep 自体をスキップする
        //      (`Scheduler::resolve_boot_time_resumable_target` 参照)。
        //   2. **その 1 件以外** の pending/running を全部 `failed` に sweep。
        //      プロセスが起動し直したということは前プロセスは死んでいる
        //      ので、選ばれなかった in-flight 行は定義上全て orphan。
        //   3. 保持期間 (`RECAP_JOB_RETENTION_DAYS`) を超えた古い行を削除。
        //      daemon が当日走らない日は cleanup が呼ばれず累積する問題への
        //      対策で、起動時に必ず一度叩く。
        let resumable_target = state.scheduler.resolve_boot_time_resumable_target().await;
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
            if state.shutdown.is_cancelled() {
                info!("shutdown requested, stopping jst batch daemon");
                break;
            }

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
            tokio::select! {
                () = sleep(wait) => {}
                () = state.shutdown.cancelled() => {
                    info!("shutdown requested during wait, stopping jst batch daemon");
                    break;
                }
            }

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
    shutdown: CancellationToken,
) -> JoinHandle<()> {
    log_topic_snapshot_emit_wiring(owner, "morning_update");
    MorningUpdateDaemon::new(scheduler, owner, shutdown).spawn()
}

struct MorningUpdateDaemon {
    scheduler: Scheduler,
    owner: Option<KnowledgeOwnerIds>,
    shutdown: CancellationToken,
}

impl MorningUpdateDaemon {
    fn new(
        scheduler: Scheduler,
        owner: Option<KnowledgeOwnerIds>,
        shutdown: CancellationToken,
    ) -> Self {
        Self {
            scheduler,
            owner,
            shutdown,
        }
    }

    fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    async fn run(self) {
        let interval = Duration::from_mins(30);
        loop {
            tokio::select! {
                () = sleep(interval) => {}
                () = self.shutdown.cancelled() => {
                    info!("shutdown requested, stopping morning update daemon");
                    break;
                }
            }
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

pub use crate::pipeline::cards::CardsJobRunner;

pub fn spawn_cards_batch_daemon(
    runner: std::sync::Arc<dyn CardsJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    hour: u32,
    minute: u32,
    shutdown: CancellationToken,
) -> JoinHandle<()> {
    let tz = FixedOffset::east_opt(0).expect("valid UTC offset");
    let cadence =
        DailyCadence::new(tz, hour, minute).unwrap_or_else(|e| panic!("cards batch cadence: {e}"));
    CardsBatchDaemon::new(runner, in_flight, cadence, shutdown).spawn()
}

struct CardsBatchDaemon {
    runner: std::sync::Arc<dyn CardsJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    cadence: DailyCadence,
    shutdown: CancellationToken,
}

impl CardsBatchDaemon {
    fn new(
        runner: std::sync::Arc<dyn CardsJobRunner>,
        in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
        cadence: DailyCadence,
        shutdown: CancellationToken,
    ) -> Self {
        Self {
            runner,
            in_flight,
            cadence,
            shutdown,
        }
    }

    fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    pub(crate) async fn run_once(
        runner: &dyn CardsJobRunner,
        in_flight: &std::sync::Mutex<Option<Uuid>>,
    ) -> anyhow::Result<Option<Uuid>> {
        let job_id = Uuid::new_v4();

        {
            let mut lock = in_flight
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner);
            if let Some(running_id) = *lock {
                warn!(
                    running_job_id = %running_id,
                    "scheduled cards batch skipped: another cards run is in flight"
                );
                return Ok(None);
            }
            *lock = Some(job_id);
        }

        struct InFlightGuard<'a>(&'a std::sync::Mutex<Option<Uuid>>);
        impl Drop for InFlightGuard<'_> {
            fn drop(&mut self) {
                *self
                    .0
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner) = None;
            }
        }
        let _guard = InFlightGuard(in_flight);

        let kick_time = Utc::now();
        let to = kick_time;
        let from = to - chrono::Duration::days(3);

        info!(
            %job_id,
            from = %from.to_rfc3339(),
            to = %to.to_rfc3339(),
            "starting automatic cards batch"
        );

        runner.run_cards(job_id, from, to).await?;
        Ok(Some(job_id))
    }

    async fn run(self) {
        let state = self;
        loop {
            if state.shutdown.is_cancelled() {
                info!("shutdown requested, stopping cards batch daemon");
                break;
            }

            let now = Utc::now();
            let next = state.cadence.next_run_from(now);
            let wait = duration_until(next, now);
            info!(
                next_run_utc = %next.to_rfc3339(),
                wait_seconds = wait.as_secs(),
                "scheduled automatic cards recap batch"
            );
            tokio::select! {
                () = sleep(wait) => {}
                () = state.shutdown.cancelled() => {
                    info!("shutdown requested during wait, stopping cards batch daemon");
                    break;
                }
            }

            match Self::run_once(state.runner.as_ref(), &state.in_flight).await {
                Ok(Some(job_id)) => info!(%job_id, "automatic cards batch completed"),
                Ok(None) => {}
                Err(err) => error!(error = %err, "automatic cards batch failed"),
            }
        }
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

    type RecordedRun = (Uuid, chrono::DateTime<Utc>, chrono::DateTime<Utc>);

    #[derive(Default)]
    struct RecordingCardsRunner {
        runs: std::sync::Mutex<Vec<RecordedRun>>,
    }

    #[async_trait::async_trait]
    impl CardsJobRunner for RecordingCardsRunner {
        async fn run_cards(
            &self,
            job_id: Uuid,
            from: chrono::DateTime<Utc>,
            to: chrono::DateTime<Utc>,
        ) -> anyhow::Result<()> {
            self.runs.lock().unwrap().push((job_id, from, to));
            Ok(())
        }
    }

    #[tokio::test]
    async fn test_spawn_cards_batch_daemon_shuts_down() {
        let runner = std::sync::Arc::new(RecordingCardsRunner::default());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        let shutdown = CancellationToken::new();
        shutdown.cancel();
        let handle = spawn_cards_batch_daemon(runner, in_flight, 17, 30, shutdown);
        handle.await.expect("task completes on cancellation");
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_window_is_three_days() {
        let runner = RecordingCardsRunner::default();
        let in_flight = std::sync::Mutex::new(None);
        let before_kick = Utc::now();
        let job_id = CardsBatchDaemon::run_once(&runner, &in_flight)
            .await
            .expect("run_once succeeds")
            .expect("run executed");
        let after_kick = Utc::now();

        let runs = runner.runs.lock().unwrap();
        assert_eq!(runs.len(), 1);
        let (run_job_id, from, to) = runs[0];
        assert_eq!(run_job_id, job_id);

        assert!(
            to >= before_kick && to <= after_kick,
            "to must be the kick time"
        );
        let diff = to - from;
        assert_eq!(
            diff,
            chrono::Duration::days(3),
            "from must be exactly to - 3 days"
        );
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_skips_when_in_flight() {
        let runner = RecordingCardsRunner::default();
        let existing_job_id = Uuid::new_v4();
        let in_flight = std::sync::Mutex::new(Some(existing_job_id));

        let res = CardsBatchDaemon::run_once(&runner, &in_flight)
            .await
            .expect("run_once succeeds");
        assert_eq!(res, None, "must return None and skip when in-flight");

        let runs = runner.runs.lock().unwrap();
        assert!(runs.is_empty(), "runner must not be invoked when in-flight");
    }

    #[test]
    fn test_cards_batch_cadence_utc_time() {
        let tz = FixedOffset::east_opt(0).unwrap();
        let cadence = DailyCadence::new(tz, 17, 30).expect("valid cadence");
        let now = chrono::DateTime::parse_from_rfc3339("2026-09-22T10:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let next = cadence.next_run_from(now);
        let expected = chrono::DateTime::parse_from_rfc3339("2026-09-22T17:30:00Z")
            .unwrap()
            .with_timezone(&Utc);
        assert_eq!(next, expected);
    }
}
