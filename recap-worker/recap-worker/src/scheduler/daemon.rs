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

const fn fixed_offset_from_hours(hours: i32) -> FixedOffset {
    match FixedOffset::east_opt(hours * 3600) {
        Some(tz) => tz,
        None => panic!("fixed offset hours must be within [-23, +23]"),
    }
}

const JST_TZ: FixedOffset = fixed_offset_from_hours(JST_OFFSET_HOURS);
const UTC_TZ: FixedOffset = fixed_offset_from_hours(0);

/// Bind the resolved knowledge-loop owner onto a freshly-built `JobContext`.
///
/// When an owner is present the context gains `user_id` + `tenant_id`, which
/// is the precondition the persist stage checks before emitting
/// `recap.topic_snapshotted.v1`. With no owner the context stays scopeless and
/// emission is left off — the legitimate "intentionally disabled" path.
pub(crate) fn apply_knowledge_owner(
    ctx: JobContext,
    owner: Option<KnowledgeOwnerIds>,
) -> JobContext {
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
    let tz = JST_TZ;
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

    async fn run_boot_hygiene(
        &self,
    ) -> Option<(Uuid, crate::store::dao::JobStatus, Option<String>, u32)> {
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
        let resumable_target = self.scheduler.resolve_boot_time_resumable_target().await;
        match self.scheduler.cleanup_old_jobs().await {
            Ok(0) => {}
            Ok(n) => info!(deleted = n, "boot-time hygiene: old jobs deleted"),
            Err(err) => error!(error = %err, "boot-time hygiene: cleanup_old_jobs failed"),
        }
        resumable_target
    }

    async fn resume_interrupted_job(
        &self,
        target: (Uuid, crate::store::dao::JobStatus, Option<String>, u32),
    ) {
        let (job_id, status, last_stage, resumed_window_days) = target;
        info!(
            %job_id,
            ?status,
            ?last_stage,
            resumed_window_days,
            "found resumable job, resuming..."
        );

        let mut job = apply_knowledge_owner(
            JobContext::new_with_window(job_id, self.genres.clone(), resumed_window_days),
            self.owner,
        );
        if let Some(stage) = last_stage {
            job = job.with_stage(stage);
        }

        match self.scheduler.run_job(job).await {
            Ok(()) => info!(%job_id, "resumed job completed"),
            Err(err) => error!(%job_id, error = %err, "resumed job failed"),
        }
    }

    async fn wait_for_next_scheduled_slot(&self) -> std::ops::ControlFlow<()> {
        let now = Utc::now();
        let next = self.cadence.next_run_from(now);
        let wait = duration_until(next, now);
        let next_local = next.with_timezone(&self.tz);
        info!(
            next_run_utc = %next.to_rfc3339(),
            next_run_jst = %next_local.to_rfc3339(),
            wait_seconds = wait.as_secs(),
            window_days = self.window_days,
            "scheduled automatic {}-day recap batch", self.window_days
        );
        if sleep_or_shutdown(wait, &self.shutdown).await {
            std::ops::ControlFlow::Continue(())
        } else {
            info!("shutdown requested during wait, stopping jst batch daemon");
            std::ops::ControlFlow::Break(())
        }
    }

    async fn run_scheduled_batch(&self) {
        let job_id = Uuid::new_v4();
        let job = apply_knowledge_owner(
            JobContext::new_with_window(job_id, self.genres.clone(), self.window_days),
            self.owner,
        );
        match self.scheduler.run_job(job).await {
            Ok(()) => info!(
                %job_id,
                genres = self.genres.len(),
                "automatic recap batch completed"
            ),
            Err(err) => error!(%job_id, error = %err, "automatic recap batch failed"),
        }
    }

    async fn cleanup_old_jobs_after_batch(&self) {
        if let Err(err) = self.scheduler.cleanup_old_jobs().await {
            error!(error = %err, "failed to cleanup old jobs");
        }
    }

    async fn run(self) {
        if let Some(target) = self.run_boot_hygiene().await {
            self.resume_interrupted_job(target).await;
        }

        loop {
            if self.shutdown.is_cancelled() {
                info!("shutdown requested, stopping jst batch daemon");
                break;
            }

            if self.wait_for_next_scheduled_slot().await.is_break() {
                break;
            }

            self.run_scheduled_batch().await;
            self.cleanup_old_jobs_after_batch().await;
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
            log_morning_update_outcome(job_id, self.scheduler.run_morning_update(job).await);
        }
    }
}

pub(crate) fn log_morning_update_outcome(
    job_id: Uuid,
    result: anyhow::Result<crate::scheduler::MorningUpdateOutcome>,
) {
    match result {
        Ok(crate::scheduler::MorningUpdateOutcome::Completed { letter_id }) => {
            info!(%job_id, %letter_id, "morning update job completed");
        }
        Ok(crate::scheduler::MorningUpdateOutcome::SkippedInFlight) => {
            info!(%job_id, "morning_update_skipped_in_flight");
        }
        Err(err) => {
            error!(%job_id, error = %err, "morning update job failed");
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
pub use crate::pipeline::themes::ThemesJobRunner;

fn latest_scheduled_slot_for_time(
    now: chrono::DateTime<Utc>,
    slot_time: chrono::NaiveTime,
) -> chrono::DateTime<Utc> {
    let today_slot = now.date_naive().and_time(slot_time).and_utc();

    if today_slot <= now {
        today_slot
    } else {
        let yesterday = now.date_naive() - chrono::Duration::days(1);
        yesterday.and_time(slot_time).and_utc()
    }
}

/// Compute the latest scheduled slot <= now for a daily UTC cadence.
/// If today's slot has already passed, it is today's slot.
/// Otherwise, it is yesterday's slot.
#[expect(
    clippy::expect_used,
    reason = "callers pass validated UTC hour (0..24) and minute (0..60)"
)]
pub fn latest_scheduled_slot(
    now: chrono::DateTime<Utc>,
    utc_hour: u32,
    utc_minute: u32,
) -> chrono::DateTime<Utc> {
    let slot_time = chrono::NaiveTime::from_hms_opt(utc_hour, utc_minute, 0)
        .expect("callers pass validated UTC hour (0..24) and minute (0..60)");
    latest_scheduled_slot_for_time(now, slot_time)
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

#[async_trait::async_trait]
pub trait ThemesLedger: Send + Sync {
    async fn has_completed_theme_run_since(
        &self,
        user_id: Uuid,
        since: chrono::DateTime<Utc>,
    ) -> anyhow::Result<bool>;
}

#[async_trait::async_trait]
impl ThemesLedger for sqlx::PgPool {
    async fn has_completed_theme_run_since(
        &self,
        user_id: Uuid,
        since: chrono::DateTime<Utc>,
    ) -> anyhow::Result<bool> {
        crate::store::dao::themes::ThemesDaoOps::has_completed_run_since(self, user_id, since)
            .await
            .map_err(Into::into)
    }
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

fn try_acquire_in_flight(
    in_flight: &std::sync::Mutex<Option<Uuid>>,
    run_id: Uuid,
) -> std::result::Result<InFlightGuard<'_>, Uuid> {
    let mut lock = in_flight
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner);
    if let Some(running_id) = *lock {
        return Err(running_id);
    }
    *lock = Some(run_id);
    Ok(InFlightGuard(in_flight))
}

#[async_trait::async_trait]
trait ScheduledBatchTarget: Send + Sync {
    fn kind(&self) -> &'static str;
    async fn has_completed_since(&self, since: chrono::DateTime<Utc>) -> anyhow::Result<bool>;
    async fn run_once(&self) -> anyhow::Result<Option<Uuid>>;
    fn is_fatal_error(&self, err: &anyhow::Error) -> bool;
}

struct CardsBatchTarget {
    runner: std::sync::Arc<dyn CardsJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    ledger: std::sync::Arc<dyn CardsLedger>,
    window_days: u32,
}

#[async_trait::async_trait]
impl ScheduledBatchTarget for CardsBatchTarget {
    fn kind(&self) -> &'static str {
        "cards"
    }

    async fn has_completed_since(&self, since: chrono::DateTime<Utc>) -> anyhow::Result<bool> {
        self.ledger.has_completed_cards_job_since(since).await
    }

    async fn run_once(&self) -> anyhow::Result<Option<Uuid>> {
        CardsBatchDaemon::run_once(self.runner.as_ref(), &self.in_flight, self.window_days).await
    }

    fn is_fatal_error(&self, err: &anyhow::Error) -> bool {
        crate::clients::subworker::is_subworker_auth_error(err)
    }
}

struct ThemesBatchTarget {
    runner: std::sync::Arc<dyn ThemesJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    ledger: std::sync::Arc<dyn ThemesLedger>,
    owner_user_id: Uuid,
}

#[async_trait::async_trait]
impl ScheduledBatchTarget for ThemesBatchTarget {
    fn kind(&self) -> &'static str {
        "themes"
    }

    async fn has_completed_since(&self, since: chrono::DateTime<Utc>) -> anyhow::Result<bool> {
        self.ledger
            .has_completed_theme_run_since(self.owner_user_id, since)
            .await
    }

    async fn run_once(&self) -> anyhow::Result<Option<Uuid>> {
        ThemesBatchDaemon::run_once(self.runner.as_ref(), &self.in_flight).await
    }

    fn is_fatal_error(&self, err: &anyhow::Error) -> bool {
        crate::clients::subworker::is_subworker_auth_error(err)
    }
}

enum RetryStep {
    ContinueWithAttempt(u32),
    Break,
}

fn current_in_flight_id(in_flight: &std::sync::Mutex<Option<Uuid>>) -> Option<Uuid> {
    *in_flight
        .lock()
        .unwrap_or_else(std::sync::PoisonError::into_inner)
}

/// Returns `true` if `duration` elapsed, or `false` if `shutdown` was cancelled.
async fn sleep_or_shutdown(duration: Duration, shutdown: &CancellationToken) -> bool {
    tokio::select! {
        () = sleep(duration) => true,
        () = shutdown.cancelled() => false,
    }
}

async fn wait_for_retry_slot(
    kind: &'static str,
    attempt: u32,
    max_attempts: u32,
    retry_delay_minutes: u64,
    shutdown: &CancellationToken,
) -> RetryStep {
    match decide_next_attempt(attempt, max_attempts, retry_delay_minutes) {
        RetryDecision::Retry {
            next_attempt,
            delay,
        } => {
            info!(
                kind,
                attempt = next_attempt,
                max_attempts,
                delay_minutes = retry_delay_minutes,
                "batch_retry_scheduled"
            );
            if sleep_or_shutdown(delay, shutdown).await {
                RetryStep::ContinueWithAttempt(next_attempt)
            } else {
                info!(kind, "shutdown requested during retry wait");
                RetryStep::Break
            }
        }
        RetryDecision::Exhausted { attempts } => {
            warn!(kind, attempts, "batch_retry_exhausted");
            RetryStep::Break
        }
    }
}

async fn handle_target_failure(
    target: &dyn ScheduledBatchTarget,
    err: &anyhow::Error,
    attempt: u32,
    max_attempts: u32,
    retry_delay_minutes: u64,
    shutdown: &CancellationToken,
) -> RetryStep {
    let kind = target.kind();
    if target.is_fatal_error(err) {
        error!(
            kind,
            attempt,
            max_attempts,
            error = %format!("{err:#}"),
            "subworker authentication error is fatal; aborting batch retry"
        );
        return RetryStep::Break;
    }
    error!(kind, attempt, error = %err, "automatic batch failed");
    wait_for_retry_slot(kind, attempt, max_attempts, retry_delay_minutes, shutdown).await
}

fn should_skip_retry_for_in_flight(
    kind: &'static str,
    attempt: u32,
    in_flight: &std::sync::Mutex<Option<Uuid>>,
) -> bool {
    if attempt <= 1 {
        return false;
    }
    if let Some(running_id) = current_in_flight_id(in_flight) {
        warn!(
            kind,
            attempt,
            running_id = %running_id,
            "retry skipped: another run is in flight"
        );
        return true;
    }
    false
}

async fn run_single_retry_attempt(
    target: &dyn ScheduledBatchTarget,
    in_flight: &std::sync::Mutex<Option<Uuid>>,
    attempt: u32,
    max_attempts: u32,
    retry_delay_minutes: u64,
    shutdown: &CancellationToken,
) -> RetryStep {
    let kind = target.kind();
    if should_skip_retry_for_in_flight(kind, attempt, in_flight) {
        return RetryStep::Break;
    }

    match target.run_once().await {
        Ok(Some(run_id)) => {
            info!(kind, %run_id, attempt, "automatic batch completed");
            RetryStep::Break
        }
        Ok(None) => {
            warn!(kind, attempt, "automatic batch skipped: run is in flight");
            RetryStep::Break
        }
        Err(err) => {
            handle_target_failure(
                target,
                &err,
                attempt,
                max_attempts,
                retry_delay_minutes,
                shutdown,
            )
            .await
        }
    }
}

async fn run_target_with_retry(
    target: &dyn ScheduledBatchTarget,
    in_flight: &std::sync::Mutex<Option<Uuid>>,
    retry_delay_minutes: u64,
    max_attempts: u32,
    shutdown: &CancellationToken,
) {
    let mut attempt = 1;
    loop {
        if shutdown.is_cancelled() {
            break;
        }

        match run_single_retry_attempt(
            target,
            in_flight,
            attempt,
            max_attempts,
            retry_delay_minutes,
            shutdown,
        )
        .await
        {
            RetryStep::ContinueWithAttempt(next_attempt) => attempt = next_attempt,
            RetryStep::Break => break,
        }
    }
}

struct DailyBatchSchedule {
    cadence: DailyCadence,
    retry_delay_minutes: u64,
    max_attempts: u32,
    catchup_enabled: bool,
}

async fn run_catchup_after_grace<T: ScheduledBatchTarget>(
    target: &T,
    in_flight: &std::sync::Mutex<Option<Uuid>>,
    schedule: &DailyBatchSchedule,
    slot: chrono::DateTime<Utc>,
    shutdown: &CancellationToken,
) {
    let kind = target.kind();
    match target.has_completed_since(slot).await {
        Err(err) => {
            error!(kind, error = %err, reason = "ledger_query_failed", "batch_catchup_skipped");
        }
        Ok(has_completed_after_grace) => {
            match should_catchup(schedule.catchup_enabled, slot, has_completed_after_grace) {
                CatchupDecision::Run { .. } => {
                    run_target_with_retry(
                        target,
                        in_flight,
                        schedule.retry_delay_minutes,
                        schedule.max_attempts,
                        shutdown,
                    )
                    .await;
                }
                CatchupDecision::Skip { .. } => {
                    info!(
                        kind,
                        reason = "completed_during_grace",
                        "batch_catchup_skipped"
                    );
                }
            }
        }
    }
}

async fn execute_catchup_after_grace<T: ScheduledBatchTarget>(
    target: &T,
    in_flight: &std::sync::Mutex<Option<Uuid>>,
    schedule: &DailyBatchSchedule,
    slot: chrono::DateTime<Utc>,
    shutdown: &CancellationToken,
) -> std::ops::ControlFlow<()> {
    let kind = target.kind();
    info!(
        kind,
        slot = %slot.to_rfc3339(),
        delay_minutes = schedule.retry_delay_minutes,
        "batch_catchup_scheduled"
    );
    let grace = Duration::from_mins(schedule.retry_delay_minutes);
    if !sleep_or_shutdown(grace, shutdown).await {
        info!(kind, "shutdown requested during catch-up grace delay");
        return std::ops::ControlFlow::Break(());
    }
    run_catchup_after_grace(target, in_flight, schedule, slot, shutdown).await;
    std::ops::ControlFlow::Continue(())
}

async fn run_startup_catchup<T: ScheduledBatchTarget>(
    target: &T,
    in_flight: &std::sync::Mutex<Option<Uuid>>,
    schedule: &DailyBatchSchedule,
    now: chrono::DateTime<Utc>,
    shutdown: &CancellationToken,
) -> std::ops::ControlFlow<()> {
    let kind = target.kind();
    let slot = latest_scheduled_slot_for_time(now, schedule.cadence.target());
    match target.has_completed_since(slot).await {
        Err(err) => {
            error!(kind, error = %err, reason = "ledger_query_failed", "batch_catchup_skipped");
            std::ops::ControlFlow::Continue(())
        }
        Ok(has_completed) => match should_catchup(schedule.catchup_enabled, slot, has_completed) {
            CatchupDecision::Run { slot } => {
                execute_catchup_after_grace(target, in_flight, schedule, slot, shutdown).await
            }
            CatchupDecision::Skip { reason } => {
                info!(kind, reason, "batch_catchup_skipped");
                std::ops::ControlFlow::Continue(())
            }
        },
    }
}

async fn wait_for_next_daily_run(
    kind: &'static str,
    cadence: &DailyCadence,
    shutdown: &CancellationToken,
) -> std::ops::ControlFlow<()> {
    let now = Utc::now();
    let next = cadence.next_run_from(now);
    let wait = duration_until(next, now);
    info!(
        kind,
        next_run_utc = %next.to_rfc3339(),
        wait_seconds = wait.as_secs(),
        "scheduled automatic daily batch"
    );
    tokio::select! {
        () = sleep(wait) => std::ops::ControlFlow::Continue(()),
        () = shutdown.cancelled() => {
            info!(kind, "shutdown requested during wait, stopping daily batch daemon");
            std::ops::ControlFlow::Break(())
        }
    }
}

async fn run_daily_batch_daemon<T: ScheduledBatchTarget>(
    target: T,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    schedule: DailyBatchSchedule,
    shutdown: CancellationToken,
) {
    let kind = target.kind();
    let now = Utc::now();
    if schedule.catchup_enabled {
        if run_startup_catchup(&target, &in_flight, &schedule, now, &shutdown)
            .await
            .is_break()
        {
            return;
        }
    } else {
        info!(kind, reason = "catchup_disabled", "batch_catchup_skipped");
    }

    loop {
        if shutdown.is_cancelled() {
            info!(kind, "shutdown requested, stopping daily batch daemon");
            break;
        }

        if wait_for_next_daily_run(kind, &schedule.cadence, &shutdown)
            .await
            .is_break()
        {
            break;
        }

        if shutdown.is_cancelled() {
            break;
        }

        run_target_with_retry(
            &target,
            &in_flight,
            schedule.retry_delay_minutes,
            schedule.max_attempts,
            &shutdown,
        )
        .await;
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
    let cadence = DailyCadence::new(UTC_TZ, hour, minute)
        .unwrap_or_else(|e| panic!("cards batch cadence: {e}"));
    let schedule = DailyBatchSchedule {
        cadence,
        retry_delay_minutes,
        max_attempts,
        catchup_enabled,
    };
    CardsBatchDaemon::new(runner, in_flight, ledger, window_days, schedule, shutdown).spawn()
}

struct CardsBatchDaemon {
    runner: std::sync::Arc<dyn CardsJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    ledger: std::sync::Arc<dyn CardsLedger>,
    window_days: u32,
    schedule: DailyBatchSchedule,
    shutdown: CancellationToken,
}

impl CardsBatchDaemon {
    fn new(
        runner: std::sync::Arc<dyn CardsJobRunner>,
        in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
        ledger: std::sync::Arc<dyn CardsLedger>,
        window_days: u32,
        schedule: DailyBatchSchedule,
        shutdown: CancellationToken,
    ) -> Self {
        Self {
            runner,
            in_flight,
            ledger,
            window_days,
            schedule,
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
        let _guard = match try_acquire_in_flight(in_flight, job_id) {
            Ok(guard) => guard,
            Err(running_id) => {
                warn!(
                    running_job_id = %running_id,
                    "scheduled cards batch skipped: another cards run is in flight"
                );
                return Ok(None);
            }
        };

        let kick_time = Utc::now();
        let to = kick_time;
        let from = to - chrono::Duration::days(i64::from(window_days));

        info!(
            %job_id,
            from = %from.to_rfc3339(),
            to = %to.to_rfc3339(),
            "starting automatic cards batch"
        );

        if let Err(err) = runner.run_cards(job_id, from, to).await {
            if crate::clients::subworker::is_subworker_auth_error(&err) {
                error!(
                    %job_id,
                    from = %from.to_rfc3339(),
                    to = %to.to_rfc3339(),
                    window_days,
                    error = %format!("{err:#}"),
                    "automatic cards batch failed due to subworker authentication error"
                );
            }
            return Err(err);
        }

        Ok(Some(job_id))
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) async fn run_with_retry(
        runner: &dyn CardsJobRunner,
        in_flight: &std::sync::Mutex<Option<Uuid>>,
        window_days: u32,
        retry_delay_minutes: u64,
        max_attempts: u32,
        shutdown: &CancellationToken,
    ) {
        struct RefCardsTarget<'a> {
            runner: &'a dyn CardsJobRunner,
            in_flight: &'a std::sync::Mutex<Option<Uuid>>,
            window_days: u32,
        }
        #[async_trait::async_trait]
        impl ScheduledBatchTarget for RefCardsTarget<'_> {
            fn kind(&self) -> &'static str {
                "cards"
            }
            async fn has_completed_since(
                &self,
                _since: chrono::DateTime<Utc>,
            ) -> anyhow::Result<bool> {
                Ok(false)
            }
            async fn run_once(&self) -> anyhow::Result<Option<Uuid>> {
                CardsBatchDaemon::run_once(self.runner, self.in_flight, self.window_days).await
            }
            fn is_fatal_error(&self, err: &anyhow::Error) -> bool {
                crate::clients::subworker::is_subworker_auth_error(err)
            }
        }
        let target = RefCardsTarget {
            runner,
            in_flight,
            window_days,
        };
        run_target_with_retry(
            &target,
            in_flight,
            retry_delay_minutes,
            max_attempts,
            shutdown,
        )
        .await;
    }

    async fn run(self) {
        let in_flight = std::sync::Arc::clone(&self.in_flight);
        let target = CardsBatchTarget {
            runner: self.runner,
            in_flight: std::sync::Arc::clone(&in_flight),
            ledger: self.ledger,
            window_days: self.window_days,
        };
        run_daily_batch_daemon(target, in_flight, self.schedule, self.shutdown).await;
    }
}

#[allow(clippy::too_many_arguments)]
pub fn spawn_themes_batch_daemon(
    runner: std::sync::Arc<dyn ThemesJobRunner>,
    in_flight: std::sync::Arc<std::sync::Mutex<Option<Uuid>>>,
    ledger: std::sync::Arc<dyn ThemesLedger>,
    owner_user_id: Uuid,
    retry_delay_minutes: u64,
    max_attempts: u32,
    catchup_enabled: bool,
    hour: u32,
    minute: u32,
    shutdown: CancellationToken,
) -> JoinHandle<()> {
    let cadence = DailyCadence::new(UTC_TZ, hour, minute)
        .unwrap_or_else(|e| panic!("themes batch cadence: {e}"));
    let target = ThemesBatchTarget {
        runner,
        in_flight: std::sync::Arc::clone(&in_flight),
        ledger,
        owner_user_id,
    };
    let schedule = DailyBatchSchedule {
        cadence,
        retry_delay_minutes,
        max_attempts,
        catchup_enabled,
    };
    tokio::spawn(run_daily_batch_daemon(
        target, in_flight, schedule, shutdown,
    ))
}

struct ThemesBatchDaemon;

impl ThemesBatchDaemon {
    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) async fn run_once(
        runner: &dyn ThemesJobRunner,
        in_flight: &std::sync::Mutex<Option<Uuid>>,
    ) -> anyhow::Result<Option<Uuid>> {
        let run_id = Uuid::new_v4();
        let _guard = match try_acquire_in_flight(in_flight, run_id) {
            Ok(guard) => guard,
            Err(running_id) => {
                warn!(
                    running_run_id = %running_id,
                    "scheduled themes batch skipped: another themes run is in flight"
                );
                return Ok(None);
            }
        };

        let run_time = Utc::now();
        info!(
            %run_id,
            run_time = %run_time.to_rfc3339(),
            "starting automatic themes batch"
        );

        runner.run_themes(run_id, run_time).await?;
        Ok(Some(run_id))
    }

    #[cfg_attr(not(test), allow(dead_code))]
    pub(crate) async fn run_with_retry(
        runner: &dyn ThemesJobRunner,
        in_flight: &std::sync::Mutex<Option<Uuid>>,
        retry_delay_minutes: u64,
        max_attempts: u32,
        shutdown: &CancellationToken,
    ) {
        struct RefThemesTarget<'a> {
            runner: &'a dyn ThemesJobRunner,
            in_flight: &'a std::sync::Mutex<Option<Uuid>>,
        }
        #[async_trait::async_trait]
        impl ScheduledBatchTarget for RefThemesTarget<'_> {
            fn kind(&self) -> &'static str {
                "themes"
            }
            async fn has_completed_since(
                &self,
                _since: chrono::DateTime<Utc>,
            ) -> anyhow::Result<bool> {
                Ok(false)
            }
            async fn run_once(&self) -> anyhow::Result<Option<Uuid>> {
                ThemesBatchDaemon::run_once(self.runner, self.in_flight).await
            }
            fn is_fatal_error(&self, err: &anyhow::Error) -> bool {
                crate::clients::subworker::is_subworker_auth_error(err)
            }
        }
        let target = RefThemesTarget { runner, in_flight };
        run_target_with_retry(
            &target,
            in_flight,
            retry_delay_minutes,
            max_attempts,
            shutdown,
        )
        .await;
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
                .try_update(
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
    async fn test_run_with_retry_does_not_retry_subworker_auth_error() {
        struct AuthFailingCardsRunner {
            runs: std::sync::Mutex<Vec<RecordedRun>>,
        }

        #[async_trait::async_trait]
        impl CardsJobRunner for AuthFailingCardsRunner {
            async fn run_cards(
                &self,
                job_id: Uuid,
                from: chrono::DateTime<Utc>,
                to: chrono::DateTime<Utc>,
            ) -> anyhow::Result<()> {
                self.runs.lock().unwrap().push((job_id, from, to));
                Err(crate::clients::subworker::SubworkerAuthError::new(
                    "classify_coarse",
                    reqwest::StatusCode::UNAUTHORIZED,
                    "unauthorized",
                )
                .into())
            }
        }

        let once_runner = AuthFailingCardsRunner {
            runs: std::sync::Mutex::new(Vec::new()),
        };
        let in_flight = std::sync::Mutex::new(None);
        let err = CardsBatchDaemon::run_once(&once_runner, &in_flight, 3)
            .await
            .expect_err("run_once must return subworker auth error");
        assert!(
            crate::clients::subworker::is_subworker_auth_error(&err),
            "returned error must be recognized as a subworker auth error: {err:#}"
        );
        assert_eq!(once_runner.runs.lock().unwrap().len(), 1);

        let retry_runner = AuthFailingCardsRunner {
            runs: std::sync::Mutex::new(Vec::new()),
        };
        let shutdown = CancellationToken::new();
        CardsBatchDaemon::run_with_retry(&retry_runner, &in_flight, 3, 0, 3, &shutdown).await;

        let runs = retry_runner.runs.lock().unwrap().clone();
        assert_eq!(
            runs.len(),
            1,
            "subworker auth error is fatal and must not be retried"
        );
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

    struct FakeThemesLedger {
        results: std::sync::Mutex<std::collections::VecDeque<anyhow::Result<bool>>>,
        queried_slots: std::sync::Mutex<Vec<(Uuid, chrono::DateTime<Utc>)>>,
    }

    impl FakeThemesLedger {
        fn new(results: Vec<anyhow::Result<bool>>) -> Self {
            Self {
                results: std::sync::Mutex::new(results.into()),
                queried_slots: std::sync::Mutex::new(Vec::new()),
            }
        }
    }

    #[async_trait::async_trait]
    impl ThemesLedger for FakeThemesLedger {
        async fn has_completed_theme_run_since(
            &self,
            user_id: Uuid,
            since: chrono::DateTime<Utc>,
        ) -> anyhow::Result<bool> {
            self.queried_slots.lock().unwrap().push((user_id, since));
            self.results.lock().unwrap().pop_front().unwrap_or(Ok(true))
        }
    }

    #[tokio::test]
    async fn test_themes_batch_daemon_run_once_uses_kick_time() {
        let runner = crate::pipeline::themes::fakes::FakeThemesJobRunner::new();
        let in_flight = std::sync::Mutex::new(None);
        let before = Utc::now();
        let run_id = ThemesBatchDaemon::run_once(&runner, &in_flight)
            .await
            .expect("run_once succeeds")
            .expect("run executed");
        let after = Utc::now();

        let runs = runner.invocations();
        assert_eq!(runs.len(), 1);
        assert_eq!(runs[0].0, run_id);
        assert!(runs[0].1 >= before && runs[0].1 <= after);
    }

    #[tokio::test]
    async fn test_themes_and_cards_in_flight_guards_are_independent() {
        let cards_runner = RecordingCardsRunner::default();
        let themes_runner = crate::pipeline::themes::fakes::FakeThemesJobRunner::new();
        let cards_in_flight = std::sync::Mutex::new(Some(Uuid::new_v4()));
        let themes_in_flight = std::sync::Mutex::new(None);

        let cards_res = CardsBatchDaemon::run_once(&cards_runner, &cards_in_flight, 3)
            .await
            .expect("cards run_once returns Ok(None)");
        assert!(cards_res.is_none(), "occupied cards guard blocks cards");

        let themes_res = ThemesBatchDaemon::run_once(&themes_runner, &themes_in_flight)
            .await
            .expect("themes run_once succeeds");
        assert!(
            themes_res.is_some(),
            "occupied cards guard must NOT block themes run"
        );
        assert_eq!(themes_runner.invocations().len(), 1);
    }

    #[tokio::test]
    async fn test_themes_batch_daemon_skips_when_themes_in_flight() {
        let runner = crate::pipeline::themes::fakes::FakeThemesJobRunner::new();
        let in_flight = std::sync::Mutex::new(Some(Uuid::new_v4()));

        let res = ThemesBatchDaemon::run_once(&runner, &in_flight)
            .await
            .expect("run_once returns Ok(None)");
        assert_eq!(res, None);
        assert!(runner.invocations().is_empty());
    }

    #[tokio::test]
    async fn test_themes_batch_daemon_retry_recovers_within_max_attempts() {
        struct FlakyThemesRunner {
            fail_first: std::sync::atomic::AtomicU32,
            runs: std::sync::Mutex<Vec<Uuid>>,
        }
        #[async_trait::async_trait]
        impl ThemesJobRunner for FlakyThemesRunner {
            async fn run_themes(
                &self,
                run_id: Uuid,
                _run_time: chrono::DateTime<Utc>,
            ) -> anyhow::Result<()> {
                self.runs.lock().unwrap().push(run_id);
                let prev = self
                    .fail_first
                    .fetch_sub(1, std::sync::atomic::Ordering::SeqCst);
                if prev > 0 {
                    anyhow::bail!("transient failure");
                }
                Ok(())
            }
        }

        let runner = FlakyThemesRunner {
            fail_first: std::sync::atomic::AtomicU32::new(1),
            runs: std::sync::Mutex::new(Vec::new()),
        };
        let in_flight = std::sync::Mutex::new(None);
        let shutdown = CancellationToken::new();

        ThemesBatchDaemon::run_with_retry(&runner, &in_flight, 0, 3, &shutdown).await;
        assert_eq!(runner.runs.lock().unwrap().len(), 2);
    }

    #[tokio::test]
    async fn test_themes_batch_daemon_catchup_runs_and_skips_when_already_completed() {
        let owner_id = Uuid::new_v4();
        let runner =
            std::sync::Arc::new(crate::pipeline::themes::fakes::FakeThemesJobRunner::new());
        let in_flight = std::sync::Arc::new(std::sync::Mutex::new(None));
        let ledger = std::sync::Arc::new(FakeThemesLedger::new(vec![Ok(false), Ok(false)]));
        let shutdown = CancellationToken::new();

        let handle = spawn_themes_batch_daemon(
            runner.clone(),
            in_flight,
            ledger.clone(),
            owner_id,
            0,
            1,
            true,
            19,
            0,
            shutdown.clone(),
        );
        tokio::time::sleep(std::time::Duration::from_millis(25)).await;
        shutdown.cancel();
        handle.await.expect("daemon task exits");

        assert_eq!(runner.invocations().len(), 1);
        let queries = ledger.queried_slots.lock().unwrap();
        assert_eq!(queries.len(), 2);
        assert_eq!(queries[0].0, owner_id);
    }

    #[derive(Clone, Default)]
    struct DaemonLogBuf(std::sync::Arc<std::sync::Mutex<Vec<u8>>>);

    impl std::io::Write for DaemonLogBuf {
        fn write(&mut self, buf: &[u8]) -> std::io::Result<usize> {
            self.0
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .extend_from_slice(buf);
            Ok(buf.len())
        }

        fn flush(&mut self) -> std::io::Result<()> {
            Ok(())
        }
    }

    struct StubFetchStage {
        notify: std::sync::Arc<tokio::sync::Notify>,
        calls: std::sync::Arc<std::sync::atomic::AtomicUsize>,
        fail: bool,
    }

    #[async_trait::async_trait]
    impl crate::pipeline::fetch::FetchStage for StubFetchStage {
        async fn fetch(
            &self,
            job: &JobContext,
        ) -> crate::error::Result<crate::pipeline::fetch::FetchedCorpus> {
            self.calls.fetch_add(1, std::sync::atomic::Ordering::SeqCst);
            self.notify.notify_one();
            if self.fail {
                return Err(crate::error::RecapError::Fetch(
                    "simulated daemon fetch failure".to_string(),
                ));
            }
            Ok(crate::pipeline::fetch::FetchedCorpus {
                job_id: job.job_id,
                articles: Vec::new(),
            })
        }
    }

    struct StubPreprocessStage;
    #[async_trait::async_trait]
    impl crate::pipeline::preprocess::PreprocessStage for StubPreprocessStage {
        async fn preprocess(
            &self,
            _job: &JobContext,
            corpus: crate::pipeline::fetch::FetchedCorpus,
        ) -> anyhow::Result<crate::pipeline::preprocess::PreprocessedCorpus> {
            Ok(crate::pipeline::preprocess::PreprocessedCorpus {
                job_id: corpus.job_id,
                articles: Vec::new(),
            })
        }
    }

    struct StubDedupStage;
    #[async_trait::async_trait]
    impl crate::pipeline::dedup::DedupStage for StubDedupStage {
        async fn deduplicate(
            &self,
            _job: &JobContext,
            corpus: crate::pipeline::preprocess::PreprocessedCorpus,
        ) -> anyhow::Result<crate::pipeline::dedup::DeduplicatedCorpus> {
            Ok(crate::pipeline::dedup::DeduplicatedCorpus {
                job_id: corpus.job_id,
                articles: Vec::new(),
                stats: crate::pipeline::dedup::DedupStats::default(),
            })
        }
    }

    struct StubGenreStage;
    #[async_trait::async_trait]
    impl crate::pipeline::genre::GenreStage for StubGenreStage {
        async fn assign(
            &self,
            _job: &JobContext,
            corpus: crate::pipeline::dedup::DeduplicatedCorpus,
        ) -> anyhow::Result<crate::pipeline::genre::GenreBundle> {
            Ok(crate::pipeline::genre::GenreBundle {
                job_id: corpus.job_id,
                assignments: Vec::new(),
                genre_distribution: std::collections::HashMap::new(),
            })
        }

        async fn update_config(
            &self,
            _overrides: &crate::pipeline::graph_override::GraphOverrideSettings,
        ) {
        }
    }

    struct StubSelectStage;
    #[async_trait::async_trait]
    impl crate::pipeline::select::SelectStage for StubSelectStage {
        async fn select(
            &self,
            _job: &JobContext,
            bundle: crate::pipeline::genre::GenreBundle,
        ) -> anyhow::Result<crate::pipeline::select::SelectedSummary> {
            Ok(crate::pipeline::select::SelectedSummary {
                job_id: bundle.job_id,
                assignments: Vec::new(),
            })
        }
    }

    struct StubDispatchStage;
    #[async_trait::async_trait]
    impl crate::pipeline::dispatch::DispatchStage for StubDispatchStage {
        async fn dispatch(
            &self,
            job: &JobContext,
            _evidence: crate::pipeline::evidence::EvidenceBundle,
        ) -> crate::error::Result<crate::pipeline::dispatch::DispatchResult> {
            Ok(crate::pipeline::dispatch::DispatchResult {
                job_id: job.job_id,
                genre_results: std::collections::HashMap::new(),
                success_count: 1,
                failure_count: 0,
                all_genres: job.genres.clone(),
            })
        }
    }

    struct StubPersistStage {
        notify: std::sync::Arc<tokio::sync::Notify>,
    }

    #[async_trait::async_trait]
    impl crate::pipeline::persist::PersistStage for StubPersistStage {
        async fn persist(
            &self,
            job: &JobContext,
            _result: crate::pipeline::dispatch::DispatchResult,
        ) -> crate::error::Result<crate::pipeline::persist::PersistResult> {
            self.notify.notify_one();
            Ok(crate::pipeline::persist::PersistResult {
                job_id: job.job_id,
                genres_stored: 1,
                genres_failed: 0,
                genres_skipped: 0,
                genres_no_evidence: 0,
                total_genres: 1,
            })
        }
    }

    fn build_batch_daemon_test_scheduler(
        dao: std::sync::Arc<crate::store::dao::mock::MockRecapDao>,
        metrics: std::sync::Arc<crate::observability::metrics::Metrics>,
        fetch_stage: std::sync::Arc<dyn crate::pipeline::fetch::FetchStage>,
        persist_stage: std::sync::Arc<dyn crate::pipeline::persist::PersistStage>,
    ) -> Scheduler {
        use crate::{
            clients::{NewsCreatorClient, SubworkerClient},
            config::{Config, ENV_MUTEX},
            pipeline::{PipelineOrchestrator, morning::MorningPipeline},
            queue::{ClassificationJobQueue, QueueStore},
            store::dao::RecapDao,
        };
        use sqlx::postgres::PgPoolOptions;

        let config = {
            let _lock = ENV_MUTEX
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner);
            temp_env::with_vars(
                [
                    (
                        "RECAP_DB_DSN",
                        Some("postgres://recap:recap@localhost:5555/recap_db"),
                    ),
                    ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
                    ("SUBWORKER_BASE_URL", Some("http://localhost:8002/")),
                    ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
                    ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                    ("RECAP_ADMIN_AUTH", Some("disabled")),
                    ("MTLS_ALLOWED_PEERS", Some("disabled")),
                    ("RECAP_EVAL_LISTENER", Some("disabled")),
                    ("RECAP_CARDS_JOB", Some("disabled")),
                    ("RECAP_THEMES_ENABLED", Some("false")),
                    ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
                    ("CLASSIFICATION_EVAL_ENABLED", Some("false")),
                    ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                    ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                    ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
                    ("MTLS_ENFORCE", None),
                ],
                || std::sync::Arc::new(Config::from_env().expect("config loads")),
            )
        };

        let recap_dao: std::sync::Arc<dyn RecapDao> = dao;
        let subworker = std::sync::Arc::new(
            SubworkerClient::new(
                config.subworker_base_url(),
                config.min_documents_per_genre(),
                None,
            )
            .expect("subworker client"),
        );
        let news_creator = std::sync::Arc::new(NewsCreatorClient::new_for_test(
            config.news_creator_base_url(),
        ));
        let pool = PgPoolOptions::new()
            .connect_lazy("postgres://recap:recap@localhost:5555/recap_db")
            .expect("lazy pool");
        let classification_queue = std::sync::Arc::new(ClassificationJobQueue::new(
            QueueStore::new(pool),
            (*subworker).clone(),
            0,
            10,
            3,
            1000,
        ));

        let pipeline = std::sync::Arc::new(
            PipelineOrchestrator::builder(std::sync::Arc::clone(&config))
                .with_fetch_stage(fetch_stage)
                .with_preprocess_stage(std::sync::Arc::new(StubPreprocessStage))
                .with_dedup_stage(std::sync::Arc::new(StubDedupStage))
                .with_genre_stage(std::sync::Arc::new(StubGenreStage))
                .with_select_stage(std::sync::Arc::new(StubSelectStage))
                .with_dispatch_stage(std::sync::Arc::new(StubDispatchStage))
                .with_persist_stage(persist_stage)
                .with_metrics(metrics)
                .build(
                    std::sync::Arc::clone(&recap_dao),
                    std::sync::Arc::clone(&subworker),
                    classification_queue,
                    None,
                )
                .expect("pipeline builds"),
        );
        let morning_pipeline = {
            let _lock = ENV_MUTEX
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner);
            temp_env::with_vars([("MTLS_ENFORCE", None::<&str>)], || {
                std::sync::Arc::new(
                    MorningPipeline::new(
                        std::sync::Arc::clone(&config),
                        std::sync::Arc::clone(&recap_dao),
                        news_creator,
                        std::sync::Arc::clone(&subworker),
                    )
                    .expect("morning pipeline builds"),
                )
            })
        };

        Scheduler::new(pipeline, morning_pipeline, config, recap_dao, subworker)
    }

    #[tokio::test]
    async fn test_spawn_jst_batch_daemon_shuts_down_when_cancelled_up_front() {
        let dao = std::sync::Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let metrics = std::sync::Arc::new(
            crate::observability::metrics::Metrics::new(prometheus::Registry::new().into())
                .expect("metrics"),
        );
        let notify = std::sync::Arc::new(tokio::sync::Notify::new());
        let fetch_calls = std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0));
        let scheduler = build_batch_daemon_test_scheduler(
            std::sync::Arc::clone(&dao),
            metrics,
            std::sync::Arc::new(StubFetchStage {
                notify: std::sync::Arc::clone(&notify),
                calls: std::sync::Arc::clone(&fetch_calls),
                fail: false,
            }),
            std::sync::Arc::new(StubPersistStage { notify }),
        );

        let shutdown = CancellationToken::new();
        shutdown.cancel();
        let handle = spawn_jst_batch_daemon(scheduler, vec!["ai".into()], 3, None, shutdown);
        handle.await.expect("jst batch daemon completes");

        assert_eq!(dao.mark_abandoned_jobs_calls(), vec![None]);
        assert!(dao.status_transitions().is_empty());
        assert_eq!(fetch_calls.load(std::sync::atomic::Ordering::SeqCst), 0);
    }

    #[tokio::test]
    async fn test_batch_daemon_shuts_down_during_scheduled_wait() {
        use chrono::Timelike;

        let dao = std::sync::Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let metrics = std::sync::Arc::new(
            crate::observability::metrics::Metrics::new(prometheus::Registry::new().into())
                .expect("metrics"),
        );
        let notify = std::sync::Arc::new(tokio::sync::Notify::new());
        let fetch_calls = std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0));
        let scheduler = build_batch_daemon_test_scheduler(
            std::sync::Arc::clone(&dao),
            metrics,
            std::sync::Arc::new(StubFetchStage {
                notify: std::sync::Arc::clone(&notify),
                calls: std::sync::Arc::clone(&fetch_calls),
                fail: false,
            }),
            std::sync::Arc::new(StubPersistStage { notify }),
        );

        let shutdown = CancellationToken::new();
        // Schedule ~1 hour ahead in JST so the daemon parks in sleep_or_shutdown.
        let target_jst = (Utc::now() + chrono::Duration::hours(1)).with_timezone(&JST_TZ);
        let cadence = DailyCadence::new(JST_TZ, target_jst.hour(), target_jst.minute())
            .expect("valid cadence");
        let daemon = BatchDaemon::new(
            scheduler,
            cadence,
            vec!["ai".into()],
            JST_TZ,
            3,
            None,
            shutdown.clone(),
        );

        let handle = tokio::spawn(daemon.run());
        // `tokio/test-util` is not enabled in Cargo.toml, so paused-clock advance is unavailable;
        // yield briefly (<= 100 ms) so boot hygiene finishes and the task enters the wait select.
        tokio::time::sleep(Duration::from_millis(25)).await;
        assert!(
            !handle.is_finished(),
            "batch daemon should remain parked in scheduled wait before shutdown"
        );

        shutdown.cancel();
        tokio::time::timeout(Duration::from_secs(10), handle)
            .await
            .expect("batch daemon shuts down within 10s timeout")
            .expect("batch daemon exits cleanly during wait");

        assert!(dao.status_transitions().is_empty());
        assert_eq!(fetch_calls.load(std::sync::atomic::Ordering::SeqCst), 0);
    }

    #[tokio::test]
    async fn test_batch_daemon_resumes_interrupted_job_and_records_completion() {
        use crate::store::dao::{JobStatus, RecapDao};
        use tracing::instrument::WithSubscriber;

        let dao = std::sync::Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let metrics = std::sync::Arc::new(
            crate::observability::metrics::Metrics::new(prometheus::Registry::new().into())
                .expect("metrics"),
        );
        let resumed_id = Uuid::new_v4();
        let dispatch_state = crate::pipeline::dispatch::DispatchResult {
            job_id: resumed_id,
            genre_results: std::collections::HashMap::new(),
            success_count: 1,
            failure_count: 0,
            all_genres: vec!["ai".to_string()],
        }
        .to_lightweight();
        dao.save_stage_state(
            resumed_id,
            "dispatch",
            &serde_json::to_value(&dispatch_state).expect("serialize dispatch state"),
        )
        .await
        .expect("save dispatch state");
        dao.set_find_resumable_job_result(Ok(Some((
            resumed_id,
            JobStatus::Running,
            Some("dispatch".to_string()),
            3,
        ))));

        let notify = std::sync::Arc::new(tokio::sync::Notify::new());
        let scheduler = build_batch_daemon_test_scheduler(
            std::sync::Arc::clone(&dao),
            std::sync::Arc::clone(&metrics),
            std::sync::Arc::new(StubFetchStage {
                notify: std::sync::Arc::clone(&notify),
                calls: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
                fail: false,
            }),
            std::sync::Arc::new(StubPersistStage { notify }),
        );

        let shutdown = CancellationToken::new();
        shutdown.cancel();
        let cadence = DailyCadence::new(JST_TZ, BATCH_HOUR, BATCH_MINUTE).expect("valid cadence");
        let owner = Some(KnowledgeOwnerIds {
            user_id: Uuid::new_v4(),
            tenant_id: Uuid::new_v4(),
        });
        let daemon = BatchDaemon::new(
            scheduler,
            cadence,
            vec!["ai".into()],
            JST_TZ,
            3,
            owner,
            shutdown,
        );

        let buf = DaemonLogBuf::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_max_level(tracing::Level::INFO)
            .without_time()
            .with_writer(move || writer.clone())
            .finish();

        daemon.run().with_subscriber(subscriber).await;

        assert_eq!(dao.mark_abandoned_jobs_calls(), vec![Some(resumed_id)]);
        let transitions = dao.status_transitions();
        assert_eq!(transitions.len(), 2);
        assert_eq!(transitions[0].status, JobStatus::Running);
        assert_eq!(transitions[1].status, JobStatus::Completed);
        assert!((metrics.jobs_completed.get() - 1.0).abs() < f64::EPSILON);

        let logs = String::from_utf8(
            buf.0
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .clone(),
        )
        .expect("utf8");
        assert!(logs.contains("found resumable job, resuming..."));
        assert!(logs.contains("resumed job completed"));
    }

    #[tokio::test]
    async fn test_batch_daemon_logs_error_when_resumed_job_fails_and_still_enters_loop() {
        use crate::store::dao::JobStatus;
        use tracing::instrument::WithSubscriber;

        let dao = std::sync::Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let metrics = std::sync::Arc::new(
            crate::observability::metrics::Metrics::new(prometheus::Registry::new().into())
                .expect("metrics"),
        );
        let resumed_id = Uuid::new_v4();
        dao.set_find_resumable_job_result(Ok(Some((
            resumed_id,
            JobStatus::Running,
            Some("dispatch".to_string()),
            3,
        ))));

        let notify = std::sync::Arc::new(tokio::sync::Notify::new());
        let scheduler = build_batch_daemon_test_scheduler(
            std::sync::Arc::clone(&dao),
            std::sync::Arc::clone(&metrics),
            std::sync::Arc::new(StubFetchStage {
                notify: std::sync::Arc::clone(&notify),
                calls: std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0)),
                fail: false,
            }),
            std::sync::Arc::new(StubPersistStage { notify }),
        );

        let shutdown = CancellationToken::new();
        shutdown.cancel();
        let cadence = DailyCadence::new(JST_TZ, BATCH_HOUR, BATCH_MINUTE).expect("valid cadence");
        let daemon = BatchDaemon::new(
            scheduler,
            cadence,
            vec!["ai".into()],
            JST_TZ,
            3,
            None,
            shutdown,
        );

        let buf = DaemonLogBuf::default();
        let writer = buf.clone();
        let subscriber = tracing_subscriber::fmt()
            .with_max_level(tracing::Level::INFO)
            .without_time()
            .with_writer(move || writer.clone())
            .finish();

        daemon.run().with_subscriber(subscriber).await;

        let transitions = dao.status_transitions();
        assert_eq!(transitions.len(), 1);
        assert_eq!(transitions[0].status, JobStatus::Failed);
        assert!((metrics.jobs_failed.get() - 1.0).abs() < f64::EPSILON);

        let logs = String::from_utf8(
            buf.0
                .lock()
                .unwrap_or_else(std::sync::PoisonError::into_inner)
                .clone(),
        )
        .expect("utf8");
        assert!(logs.contains("resumed job failed"));
        assert!(logs.contains("shutdown requested, stopping jst batch daemon"));
    }

    #[tokio::test]
    async fn test_batch_daemon_resumes_job_with_no_last_stage_from_fetch() {
        use crate::store::dao::JobStatus;

        let dao = std::sync::Arc::new(crate::store::dao::mock::MockRecapDao::new());
        let metrics = std::sync::Arc::new(
            crate::observability::metrics::Metrics::new(prometheus::Registry::new().into())
                .expect("metrics"),
        );
        let resumed_id = Uuid::new_v4();
        dao.set_find_resumable_job_result(Ok(Some((resumed_id, JobStatus::Running, None, 3))));

        let notify = std::sync::Arc::new(tokio::sync::Notify::new());
        let fetch_calls = std::sync::Arc::new(std::sync::atomic::AtomicUsize::new(0));
        let scheduler = build_batch_daemon_test_scheduler(
            std::sync::Arc::clone(&dao),
            std::sync::Arc::clone(&metrics),
            std::sync::Arc::new(StubFetchStage {
                notify: std::sync::Arc::new(tokio::sync::Notify::new()),
                calls: std::sync::Arc::clone(&fetch_calls),
                fail: false,
            }),
            std::sync::Arc::new(StubPersistStage { notify }),
        );

        let shutdown = CancellationToken::new();
        shutdown.cancel();
        let cadence = DailyCadence::new(JST_TZ, BATCH_HOUR, BATCH_MINUTE).expect("valid cadence");
        let daemon = BatchDaemon::new(
            scheduler,
            cadence,
            vec!["ai".into()],
            JST_TZ,
            3,
            None,
            shutdown,
        );

        daemon.run().await;

        assert_eq!(fetch_calls.load(std::sync::atomic::Ordering::SeqCst), 1);
        assert_eq!(
            dao.status_transitions().last().map(|t| t.status.clone()),
            Some(JobStatus::Completed)
        );
        assert!((metrics.jobs_completed.get() - 1.0).abs() < f64::EPSILON);
    }
}
