use anyhow::{Context, Result};
use chrono::{DateTime, Duration, Utc};
use sqlx::{PgPool, Row};
use uuid::Uuid;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SurfaceRunRecord {
    pub job_id: Uuid,
    pub status: String,
    pub kicked_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PulseSurfaceProvenance {
    pub finished_at: DateTime<Utc>,
    pub window: Option<(DateTime<Utc>, DateTime<Utc>)>,
}

pub struct SurfaceDaoOps;

impl SurfaceDaoOps {
    pub async fn get_latest_three_day_recap_run(pool: &PgPool) -> Result<Option<SurfaceRunRecord>> {
        let row = sqlx::query(
            r"
            SELECT job_id, status, kicked_at, updated_at
            FROM recap_jobs
            WHERE window_days = 3
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest 3-day recap run")?;

        Ok(row.map(|r| SurfaceRunRecord {
            job_id: r.get("job_id"),
            status: r.get("status"),
            kicked_at: r.get("kicked_at"),
            updated_at: r.get("updated_at"),
        }))
    }

    pub async fn get_latest_morning_run(pool: &PgPool) -> Result<Option<SurfaceRunRecord>> {
        let row = sqlx::query(
            r"
            SELECT job_id, status, kicked_at, updated_at
            FROM recap_jobs
            WHERE trigger_source = 'morning'
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest morning recap run")?;

        Ok(row.map(|r| SurfaceRunRecord {
            job_id: r.get("job_id"),
            status: r.get("status"),
            kicked_at: r.get("kicked_at"),
            updated_at: r.get("updated_at"),
        }))
    }

    pub async fn get_latest_pulse_run(pool: &PgPool) -> Result<Option<SurfaceRunRecord>> {
        let row = sqlx::query(
            r"
            SELECT
                job_id,
                status,
                started_at,
                COALESCE(finished_at, started_at) AS updated_at
            FROM pulse_generations
            ORDER BY started_at DESC, id DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest pulse generation run")?;

        Ok(row.map(|r| SurfaceRunRecord {
            job_id: r.get("job_id"),
            status: r.get("status"),
            kicked_at: r.get("started_at"),
            updated_at: r.get("updated_at"),
        }))
    }

    pub async fn get_pulse_provenance(
        pool: &PgPool,
        job_id: Uuid,
    ) -> Result<Option<PulseSurfaceProvenance>> {
        let row = sqlx::query(
            r"
            SELECT
                COALESCE(pg.finished_at, pg.started_at) AS finished_at,
                rj.window_days,
                COALESCE(MAX(ro.created_at), rj.kicked_at) AS window_end
            FROM pulse_generations pg
            LEFT JOIN recap_jobs rj ON rj.job_id = pg.job_id
            LEFT JOIN recap_outputs ro ON ro.job_id = rj.job_id
            WHERE pg.job_id = $1 AND pg.status = 'succeeded'
            GROUP BY pg.id, pg.finished_at, pg.started_at, rj.window_days, rj.kicked_at
            ORDER BY pg.finished_at DESC NULLS LAST, pg.id DESC
            LIMIT 1
            ",
        )
        .bind(job_id)
        .fetch_optional(pool)
        .await
        .context("failed to fetch pulse provenance")?;

        Ok(row.map(|r| {
            let finished_at: DateTime<Utc> = r.get("finished_at");
            let window_days: Option<i32> = r.get("window_days");
            let window_end: Option<DateTime<Utc>> = r.get("window_end");
            let window = match (window_days, window_end) {
                (Some(days), Some(end)) => {
                    let start = end - Duration::days(i64::from(days));
                    Some((start, end))
                }
                _ => None,
            };
            PulseSurfaceProvenance {
                finished_at,
                window,
            }
        }))
    }

    pub async fn get_recap_job_window(
        pool: &PgPool,
        job_id: Uuid,
    ) -> Result<Option<(DateTime<Utc>, DateTime<Utc>)>> {
        let row = sqlx::query(
            r"
            SELECT
                rj.window_days,
                COALESCE(MAX(ro.created_at), rj.kicked_at) AS window_end
            FROM recap_jobs rj
            LEFT JOIN recap_outputs ro ON ro.job_id = rj.job_id
            WHERE rj.job_id = $1
            GROUP BY rj.job_id, rj.window_days, rj.kicked_at
            ",
        )
        .bind(job_id)
        .fetch_optional(pool)
        .await
        .context("failed to fetch recap job window")?;

        Ok(row.map(|r| {
            let window_days: i32 = r.get("window_days");
            let window_end: DateTime<Utc> = r.get("window_end");
            let window_start = window_end - Duration::days(i64::from(window_days));
            (window_start, window_end)
        }))
    }

    pub async fn get_job_updated_at(pool: &PgPool, job_id: Uuid) -> Result<Option<DateTime<Utc>>> {
        let row = sqlx::query(
            r"
            SELECT updated_at
            FROM recap_jobs
            WHERE job_id = $1
            ",
        )
        .bind(job_id)
        .fetch_optional(pool)
        .await
        .context("failed to fetch recap job updated_at")?;

        Ok(row.map(|r| r.get("updated_at")))
    }
}
