use std::time::Duration;

use chrono::{FixedOffset, Utc};
use tokio::{task::JoinHandle, time::sleep};
use tracing::{error, info};
use uuid::Uuid;

use crate::scheduler::{JobContext, Scheduler, cadence::DailyCadence};

const JST_OFFSET_HOURS: i32 = 9;
const BATCH_HOUR: u32 = 4;
const BATCH_MINUTE: u32 = 0;

pub fn spawn_jst_batch_daemon(
    scheduler: Scheduler,
    genres: Vec<String>,
    window_days: u32,
) -> JoinHandle<()> {
    let tz = FixedOffset::east_opt(JST_OFFSET_HOURS * 3600).expect("valid JST offset");
    let cadence = DailyCadence::new(tz, BATCH_HOUR, BATCH_MINUTE);
    BatchDaemon::new(scheduler, cadence, genres, tz, window_days).spawn()
}

struct BatchDaemon {
    scheduler: Scheduler,
    cadence: DailyCadence,
    genres: Vec<String>,
    tz: FixedOffset,
    window_days: u32,
}

impl BatchDaemon {
    fn new(
        scheduler: Scheduler,
        cadence: DailyCadence,
        genres: Vec<String>,
        tz: FixedOffset,
        window_days: u32,
    ) -> Self {
        Self {
            scheduler,
            cadence,
            genres,
            tz,
            window_days,
        }
    }

    fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    async fn run(self) {
        let state = self;

        // 起動時に中断されたジョブがないか確認
        if let Ok(Some((job_id, status, last_stage, resumed_window_days))) =
            state.scheduler.find_resumable_job().await
        {
            info!(
                %job_id,
                ?status,
                ?last_stage,
                resumed_window_days,
                "found resumable job, resuming..."
            );

            let mut job =
                JobContext::new_with_window(job_id, state.genres.clone(), resumed_window_days);
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
            let job = JobContext::new_with_window(job_id, state.genres.clone(), state.window_days);
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

pub fn spawn_morning_update_daemon(scheduler: Scheduler) -> JoinHandle<()> {
    MorningUpdateDaemon::new(scheduler).spawn()
}

struct MorningUpdateDaemon {
    scheduler: Scheduler,
}

impl MorningUpdateDaemon {
    fn new(scheduler: Scheduler) -> Self {
        Self { scheduler }
    }

    fn spawn(self) -> JoinHandle<()> {
        tokio::spawn(async move {
            self.run().await;
        })
    }

    async fn run(self) {
        let interval = Duration::from_secs(30 * 60); // 30 minutes
        loop {
            sleep(interval).await;
            let job_id = Uuid::new_v4();
            let job = JobContext::new(job_id, Vec::new()); // No genres needed for update
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
