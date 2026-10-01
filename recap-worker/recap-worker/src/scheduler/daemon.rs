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

/// Compute the latest scheduled slot <= now for a daily UTC cadence.
/// If today's slot has already passed, it is today's slot.
/// Otherwise, it is yesterday's slot.
pub fn latest_scheduled_slot(
    now: chrono::DateTime<Utc>,
    utc_hour: u32,
    utc_minute: u32,
) -> chrono::DateTime<Utc> {
    let today_slot = now
        .date_naive()
        .and_hms_opt(utc_hour, utc_minute, 0)
        .expect("valid UTC hour and minute")
        .and_utc();

    if today_slot <= now {
        today_slot
    } else {
        let yesterday = now.date_naive() - chrono::Duration::days(1);
        yesterday
            .and_hms_opt(utc_hour, utc_minute, 0)
            .expect("valid UTC hour and minute")
            .and_utc()
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CatchupDecision {
    Run { slot: chrono::DateTime<Utc> },
    Skip { reason: &'static str },
}

pub fn should_catchup(
    catchup_enabled: bool,
    slot: chrono::DateTime<Utc>,
    has_completed_since_slot: bool,
) -> CatchupDecision {
    if !catchup_enabled {
        CatchupDecision::Skip {
            reason: "catchup_disabled",
        }
    } else if has_completed_since_slot {
        CatchupDecision::Skip {
            reason: "completed_run_exists",
        }
    } else {
        CatchupDecision::Run { slot }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RetryDecision {
    Retry { next_attempt: u32, delay: Duration },
    Exhausted { attempts: u32 },
}

pub fn decide_next_attempt(
    current_attempt: u32,
    max_attempts: u32,
    delay_minutes: u64,
) -> RetryDecision {
    if current_attempt < max_attempts {
        RetryDecision::Retry {
            next_attempt: current_attempt + 1,
            delay: Duration::from_mins(delay_minutes),
        }
    } else {
        RetryDecision::Exhausted {
            attempts: current_attempt,
        }
    }
}

#[async_trait::async_trait]
pub trait CardsLedger: Send + Sync {
    async fn has_completed_cards_job_since(
        &self,
        since: chrono::DateTime<Utc>,
    ) -> anyhow::Result<bool>;
}

#[async_trait::async_trait]
impl CardsLedger for sqlx::PgPool {
    async fn has_completed_cards_job_since(
        &self,
        since: chrono::DateTime<Utc>,
    ) -> anyhow::Result<bool> {
        crate::store::dao::cards::CardsDaoOps::has_completed_cards_job_since(self, since)
            .await
            .map_err(Into::into)
    }
}

#[allow(clippy::too_many_arguments)]
pub fn spawn_cards_batch_daemon(
    runner: std::sync::Arc<dyn CardsJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    ledger: std::sync::Arc<dyn CardsLedger>,
    window_days: u32,
    retry_delay_minutes: u64,
    max_attempts: u32,
    catchup_enabled: bool,
    hour: u32,
    minute: u32,
    shutdown: CancellationToken,
) -> JoinHandle<()> {
    let tz = FixedOffset::east_opt(0).expect("valid UTC offset");
    let cadence =
        DailyCadence::new(tz, hour, minute).unwrap_or_else(|e| panic!("cards batch cadence: {e}"));
    CardsBatchDaemon::new(
        runner,
        in_flight,
        ledger,
        cadence,
        window_days,
        retry_delay_minutes,
        max_attempts,
        catchup_enabled,
        hour,
        minute,
        shutdown,
    )
    .spawn()
}

struct CardsBatchDaemon {
    runner: std::sync::Arc<dyn CardsJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    ledger: std::sync::Arc<dyn CardsLedger>,
    cadence: DailyCadence,
    window_days: u32,
    retry_delay_minutes: u64,
    max_attempts: u32,
    catchup_enabled: bool,
    utc_hour: u32,
    utc_minute: u32,
    shutdown: CancellationToken,
}

impl CardsBatchDaemon {
    #[allow(clippy::too_many_arguments)]
    fn new(
        runner: std::sync::Arc<dyn CardsJobRunner>,
        in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
        ledger: std::sync::Arc<dyn CardsLedger>,
        cadence: DailyCadence,
        window_days: u32,
        retry_delay_minutes: u64,
        max_attempts: u32,
        catchup_enabled: bool,
        utc_hour: u32,
        utc_minute: u32,
        shutdown: CancellationToken,
    ) -> Self {
        Self {
            runner,
            in_flight,
            ledger,
            cadence,
            window_days,
            retry_delay_minutes,
            max_attempts,
            catchup_enabled,
            utc_hour,
            utc_minute,
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
        window_days: u32,
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
        let from = to - chrono::Duration::days(i64::from(window_days));

        info!(
            %job_id,
            from = %from.to_rfc3339(),
            to = %to.to_rfc3339(),
            "starting automatic cards batch"
        );

        runner.run_cards(job_id, from, to).await?;

        Ok(Some(job_id))
    }

    pub(crate) async fn run_with_retry(
        runner: &dyn CardsJobRunner,
        in_flight: &std::sync::Mutex<Option<Uuid>>,
        window_days: u32,
        retry_delay_minutes: u64,
        max_attempts: u32,
        shutdown: &CancellationToken,
    ) {
        let mut attempt = 1;
        loop {
            if shutdown.is_cancelled() {
                break;
            }

            if attempt > 1 {
                let lock = in_flight
                    .lock()
                    .unwrap_or_else(std::sync::PoisonError::into_inner);
                if let Some(running_id) = *lock {
                    warn!(
                        attempt,
                        running_job_id = %running_id,
                        "cards retry skipped: another cards run is in flight"
                    );
                    break;
                }
            }

            match Self::run_once(runner, in_flight, window_days).await {
                Ok(Some(job_id)) => {
                    info!(%job_id, attempt, "automatic cards batch completed");
                    break;
                }
                Ok(None) => {
                    warn!(
                        attempt,
                        "automatic cards batch skipped: another cards run is in flight"
                    );
                    break;
                }
                Err(err) => {
                    error!(attempt, error = %err, "automatic cards batch failed");
                    match decide_next_attempt(attempt, max_attempts, retry_delay_minutes) {
                        RetryDecision::Retry {
                            next_attempt,
                            delay,
                        } => {
                            info!(
                                attempt = next_attempt,
                                max_attempts,
                                delay_minutes = retry_delay_minutes,
                                "cards_retry_scheduled"
                            );
                            tokio::select! {
                                () = sleep(delay) => {
                                    attempt = next_attempt;
                                }
                                () = shutdown.cancelled() => {
                                    info!("shutdown requested during retry wait, stopping cards batch retry");
                                    break;
                                }
                            }
                        }
                        RetryDecision::Exhausted { attempts } => {
                            warn!(attempts, "cards_retry_exhausted");
                            break;
                        }
                    }
                }
            }
        }
    }

    async fn run(self) {
        let state = self;

        let now = Utc::now();
        if state.catchup_enabled {
            let slot = latest_scheduled_slot(now, state.utc_hour, state.utc_minute);
            let catchup_res = state.ledger.has_completed_cards_job_since(slot).await;
            match catchup_res {
                Err(err) => {
                    error!(error = %err, reason = "ledger_query_failed", "cards_catchup_skipped");
                }
                Ok(has_completed) => {
                    match should_catchup(state.catchup_enabled, slot, has_completed) {
                        CatchupDecision::Run { slot } => {
                            info!(
                                slot = %slot.to_rfc3339(),
                                delay_minutes = state.retry_delay_minutes,
                                "cards_catchup_scheduled"
                            );
                            let grace = Duration::from_mins(state.retry_delay_minutes);
                            tokio::select! {
                                () = sleep(grace) => {
                                    match state.ledger.has_completed_cards_job_since(slot).await {
                                        Err(err) => {
                                            error!(error = %err, reason = "ledger_query_failed", "cards_catchup_skipped");
                                        }
                                        Ok(has_completed_after_grace) => {
                                            let decision_after_grace = should_catchup(state.catchup_enabled, slot, has_completed_after_grace);
                                            match decision_after_grace {
                                                CatchupDecision::Run { .. } => {
                                                    Self::run_with_retry(
                                                        state.runner.as_ref(),
                                                        &state.in_flight,
                                                        state.window_days,
                                                        state.retry_delay_minutes,
                                                        state.max_attempts,
                                                        &state.shutdown,
                                                    )
                                                    .await;
                                                }
                                                CatchupDecision::Skip { .. } => {
                                                    info!(reason = "completed_during_grace", "cards_catchup_skipped");
                                                }
                                            }
                                        }
                                    }
                                }
                                () = state.shutdown.cancelled() => {
                                    info!("shutdown requested during catch-up grace delay, stopping cards batch daemon");
                                    return;
                                }
                            }
                        }
                        CatchupDecision::Skip { reason } => {
                            info!(reason, "cards_catchup_skipped");
                        }
                    }
                }
            }
        } else {
            info!(reason = "catchup_disabled", "cards_catchup_skipped");
        }

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

            if state.shutdown.is_cancelled() {
                break;
            }

            Self::run_with_retry(
                state.runner.as_ref(),
                &state.in_flight,
                state.window_days,
                state.retry_delay_minutes,
                state.max_attempts,
                &state.shutdown,
            )
            .await;
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

    struct FakeCardsLedger {
        results: std::sync::Mutex<std::collections::VecDeque<anyhow::Result<bool>>>,
        queried_slots: std::sync::Mutex<Vec<chrono::DateTime<Utc>>>,
    }

    impl FakeCardsLedger {
        fn new(results: Vec<anyhow::Result<bool>>) -> Self {
            Self {
                results: std::sync::Mutex::new(results.into()),
                queried_slots: std::sync::Mutex::new(Vec::new()),
            }
        }
    }

    #[async_trait::async_trait]
    impl CardsLedger for FakeCardsLedger {
        async fn has_completed_cards_job_since(
            &self,
            since: chrono::DateTime<Utc>,
        ) -> anyhow::Result<bool> {
            self.queried_slots.lock().unwrap().push(since);
            self.results.lock().unwrap().pop_front().unwrap_or(Ok(true))
        }
    }

    #[tokio::test]
    async fn test_spawn_cards_batch_daemon_shuts_down() {
        let runner = std::sync::Arc::new(RecordingCardsRunner::default());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        let ledger = std::sync::Arc::new(FakeCardsLedger::new(vec![Ok(true)]));
        let shutdown = CancellationToken::new();
        shutdown.cancel();
        let handle =
            spawn_cards_batch_daemon(runner, in_flight, ledger, 3, 15, 3, false, 17, 30, shutdown);
        handle.await.expect("task completes on cancellation");
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_window_matches_configured() {
        let runner = RecordingCardsRunner::default();
        let in_flight = std::sync::Mutex::new(None);
        let before_kick = Utc::now();
        let job_id = CardsBatchDaemon::run_once(&runner, &in_flight, 5)
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
            chrono::Duration::days(5),
            "from must be exactly to - configured window_days"
        );
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_skips_when_in_flight() {
        let runner = RecordingCardsRunner::default();
        let existing_job_id = Uuid::new_v4();
        let in_flight = std::sync::Mutex::new(Some(existing_job_id));

        let res = CardsBatchDaemon::run_once(&runner, &in_flight, 3)
            .await
            .expect("run_once succeeds");
        assert_eq!(res, None, "must return None and skip when in-flight");

        let runs = runner.runs.lock().unwrap();
        assert!(runs.is_empty(), "runner must not be invoked when in-flight");
    }

    struct MockFailingCardsRunner {
        fail_count: std::sync::atomic::AtomicU32,
        runs: std::sync::Mutex<Vec<RecordedRun>>,
    }

    impl MockFailingCardsRunner {
        fn new(fail_times: u32) -> Self {
            Self {
                fail_count: std::sync::atomic::AtomicU32::new(fail_times),
                runs: std::sync::Mutex::new(Vec::new()),
            }
        }
    }

    #[async_trait::async_trait]
    impl CardsJobRunner for MockFailingCardsRunner {
        async fn run_cards(
            &self,
            job_id: Uuid,
            from: chrono::DateTime<Utc>,
            to: chrono::DateTime<Utc>,
        ) -> anyhow::Result<()> {
            self.runs.lock().unwrap().push((job_id, from, to));
            let prev = self
                .fail_count
                .fetch_update(
                    std::sync::atomic::Ordering::SeqCst,
                    std::sync::atomic::Ordering::SeqCst,
                    |x| if x > 0 { Some(x - 1) } else { Some(0) },
                )
                .unwrap();
            if prev > 0 {
                anyhow::bail!("mock runner failure");
            }
            Ok(())
        }
    }

    #[tokio::test]
    async fn test_run_with_retry_new_job_per_attempt() {
        let runner = MockFailingCardsRunner::new(1);
        let in_flight = std::sync::Mutex::new(None);
        let shutdown = CancellationToken::new();

        CardsBatchDaemon::run_with_retry(&runner, &in_flight, 3, 0, 3, &shutdown).await;

        let runs = runner.runs.lock().unwrap().clone();
        assert_eq!(
            runs.len(),
            2,
            "runner must be called twice (initial + 1 retry)"
        );
        assert_ne!(
            runs[0].0, runs[1].0,
            "every retry attempt must generate a new job_id"
        );
    }

    #[tokio::test]
    async fn test_run_with_retry_exhaustion() {
        let runner = MockFailingCardsRunner::new(10);
        let in_flight = std::sync::Mutex::new(None);
        let shutdown = CancellationToken::new();

        CardsBatchDaemon::run_with_retry(&runner, &in_flight, 3, 0, 2, &shutdown).await;

        let runs = runner.runs.lock().unwrap().clone();
        assert_eq!(runs.len(), 2, "must exhaust after max_attempts");
    }

    #[tokio::test]
    async fn test_run_with_retry_skip_when_in_flight() {
        let runner = MockFailingCardsRunner::new(2);
        let in_flight = std::sync::Mutex::new(Some(Uuid::new_v4()));
        let shutdown = CancellationToken::new();

        CardsBatchDaemon::run_with_retry(&runner, &in_flight, 3, 0, 3, &shutdown).await;

        let runs = runner.runs.lock().unwrap().clone();
        assert_eq!(
            runs.len(),
            0,
            "run must be skipped when in_flight is already occupied"
        );
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

    #[test]
    fn test_latest_scheduled_slot_after_today_slot() {
        let now = chrono::DateTime::parse_from_rfc3339("2026-10-01T15:30:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let slot = latest_scheduled_slot(now, 14, 0);
        let expected = chrono::DateTime::parse_from_rfc3339("2026-10-01T14:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        assert_eq!(slot, expected);
    }

    #[test]
    fn test_latest_scheduled_slot_before_today_slot() {
        let now = chrono::DateTime::parse_from_rfc3339("2026-10-01T12:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let slot = latest_scheduled_slot(now, 14, 0);
        let expected = chrono::DateTime::parse_from_rfc3339("2026-09-30T14:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        assert_eq!(slot, expected);
    }

    #[test]
    fn test_latest_scheduled_slot_across_midnight_utc() {
        let now = chrono::DateTime::parse_from_rfc3339("2026-10-01T00:15:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let slot = latest_scheduled_slot(now, 23, 30);
        let expected = chrono::DateTime::parse_from_rfc3339("2026-09-30T23:30:00Z")
            .unwrap()
            .with_timezone(&Utc);
        assert_eq!(slot, expected);
    }

    #[test]
    fn test_should_catchup_completed_job_present_skips() {
        let slot = chrono::DateTime::parse_from_rfc3339("2026-10-01T14:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let decision = should_catchup(true, slot, true);
        assert_eq!(
            decision,
            CatchupDecision::Skip {
                reason: "completed_run_exists"
            }
        );
    }

    #[test]
    fn test_should_catchup_only_failed_job_present_runs() {
        let slot = chrono::DateTime::parse_from_rfc3339("2026-10-01T14:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let decision = should_catchup(true, slot, false);
        assert_eq!(decision, CatchupDecision::Run { slot });
    }

    #[test]
    fn test_should_catchup_disabled_skips() {
        let slot = chrono::DateTime::parse_from_rfc3339("2026-10-01T14:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let decision = should_catchup(false, slot, false);
        assert_eq!(
            decision,
            CatchupDecision::Skip {
                reason: "catchup_disabled"
            }
        );
    }

    #[test]
    fn test_decide_next_attempt_retry_counting() {
        let decision = decide_next_attempt(1, 3, 15);
        assert_eq!(
            decision,
            RetryDecision::Retry {
                next_attempt: 2,
                delay: Duration::from_mins(15)
            }
        );

        let decision2 = decide_next_attempt(2, 3, 15);
        assert_eq!(
            decision2,
            RetryDecision::Retry {
                next_attempt: 3,
                delay: Duration::from_mins(15)
            }
        );
    }

    #[test]
    fn test_decide_next_attempt_exhaustion() {
        let decision = decide_next_attempt(3, 3, 15);
        assert_eq!(decision, RetryDecision::Exhausted { attempts: 3 });

        let decision_over = decide_next_attempt(4, 3, 15);
        assert_eq!(decision_over, RetryDecision::Exhausted { attempts: 4 });
    }

    #[test]
    fn test_latest_scheduled_slot_when_now_equals_slot_exactly() {
        let now = chrono::DateTime::parse_from_rfc3339("2026-10-01T14:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let slot = latest_scheduled_slot(now, 14, 0);
        assert_eq!(slot, now);
    }

    #[tokio::test]
    async fn test_run_with_retry_skips_due_retry_when_another_run_in_flight() {
        struct FailingThenInFlightRunner {
            in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
            runs: std::sync::Mutex<Vec<RecordedRun>>,
        }

        #[async_trait::async_trait]
        impl CardsJobRunner for FailingThenInFlightRunner {
            async fn run_cards(
                &self,
                job_id: Uuid,
                from: chrono::DateTime<Utc>,
                to: chrono::DateTime<Utc>,
            ) -> anyhow::Result<()> {
                self.runs.lock().unwrap().push((job_id, from, to));
                let in_flight = std::sync::Arc::clone(&self.in_flight);
                tokio::spawn(async move {
                    tokio::task::yield_now().await;
                    *in_flight.lock().unwrap() = Some(Uuid::new_v4());
                });
                anyhow::bail!("attempt 1 failure");
            }
        }

        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        let runner = FailingThenInFlightRunner {
            in_flight: std::sync::Arc::clone(&in_flight),
            runs: std::sync::Mutex::new(Vec::new()),
        };
        let shutdown = CancellationToken::new();

        CardsBatchDaemon::run_with_retry(&runner, &in_flight, 3, 0, 3, &shutdown).await;

        let runs = runner.runs.lock().unwrap().clone();
        assert_eq!(
            runs.len(),
            1,
            "only attempt 1 should run; attempt 2 must be skipped due to in_flight check"
        );
        assert!(
            in_flight.lock().unwrap().is_some(),
            "in_flight must remain occupied by the other run"
        );
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_catchup_ledger_query_failed_skips() {
        let runner = std::sync::Arc::new(RecordingCardsRunner::default());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        let ledger =
            std::sync::Arc::new(FakeCardsLedger::new(vec![Err(anyhow::anyhow!("db error"))]));
        let shutdown = CancellationToken::new();

        let handle = spawn_cards_batch_daemon(
            runner.clone(),
            in_flight,
            ledger,
            3,
            0,
            1,
            true,
            17,
            30,
            shutdown.clone(),
        );

        tokio::time::sleep(std::time::Duration::from_millis(25)).await;
        shutdown.cancel();
        handle.await.expect("daemon task exits");

        assert_eq!(
            runner.runs.lock().unwrap().len(),
            0,
            "catchup must be skipped when ledger query fails"
        );
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_catchup_completed_during_grace_skips() {
        let runner = std::sync::Arc::new(RecordingCardsRunner::default());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        // First check: false (missing). Second check after grace: true (completed during grace).
        let ledger = std::sync::Arc::new(FakeCardsLedger::new(vec![Ok(false), Ok(true)]));
        let shutdown = CancellationToken::new();

        let handle = spawn_cards_batch_daemon(
            runner.clone(),
            in_flight,
            ledger.clone(),
            3,
            0,
            1,
            true,
            17,
            30,
            shutdown.clone(),
        );

        tokio::time::sleep(std::time::Duration::from_millis(25)).await;
        shutdown.cancel();
        handle.await.expect("daemon task exits");

        assert_eq!(
            runner.runs.lock().unwrap().len(),
            0,
            "catchup must be skipped when completed during grace"
        );
        assert_eq!(
            ledger.queried_slots.lock().unwrap().len(),
            2,
            "ledger must be queried twice (before and after grace)"
        );
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_catchup_runs_when_slot_missing_and_not_completed_during_grace()
    {
        let runner = std::sync::Arc::new(RecordingCardsRunner::default());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        // First check: false. Second check after grace: false.
        let ledger = std::sync::Arc::new(FakeCardsLedger::new(vec![Ok(false), Ok(false)]));
        let shutdown = CancellationToken::new();

        let handle = spawn_cards_batch_daemon(
            runner.clone(),
            in_flight,
            ledger.clone(),
            3,
            0,
            1,
            true,
            17,
            30,
            shutdown.clone(),
        );

        tokio::time::sleep(std::time::Duration::from_millis(25)).await;
        shutdown.cancel();
        handle.await.expect("daemon task exits");

        assert_eq!(
            runner.runs.lock().unwrap().len(),
            1,
            "catchup must execute cards run"
        );
        assert_eq!(
            ledger.queried_slots.lock().unwrap().len(),
            2,
            "ledger must be queried twice (before and after grace)"
        );
    }

    #[tokio::test]
    async fn test_cards_batch_daemon_catchup_disabled_does_not_query_ledger() {
        let runner = std::sync::Arc::new(RecordingCardsRunner::default());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        let ledger = std::sync::Arc::new(FakeCardsLedger::new(vec![]));
        let shutdown = CancellationToken::new();

        let handle = spawn_cards_batch_daemon(
            runner.clone(),
            in_flight,
            ledger.clone(),
            3,
            0,
            1,
            false,
            17,
            30,
            shutdown.clone(),
        );

        tokio::time::sleep(std::time::Duration::from_millis(25)).await;
        shutdown.cancel();
        handle.await.expect("daemon task exits");

        assert_eq!(
            ledger.queried_slots.lock().unwrap().len(),
            0,
            "ledger must not be queried when catchup is disabled"
        );
        assert_eq!(
            runner.runs.lock().unwrap().len(),
            0,
            "runner must not be invoked when catchup is disabled"
        );
    }
}
