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
              AND trigger_source NOT IN ('cards', 'cards_replay', 'morning')
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch latest 3-day recap run")?;

        row.map(|r| -> Result<SurfaceRunRecord> {
            Ok(SurfaceRunRecord {
                job_id: r.try_get("job_id")?,
                status: r.try_get("status")?,
                kicked_at: r.try_get("kicked_at")?,
                updated_at: r.try_get("updated_at")?,
            })
        })
        .transpose()
        .context("failed to decode latest 3-day recap run")
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

        row.map(|r| -> Result<SurfaceRunRecord> {
            Ok(SurfaceRunRecord {
                job_id: r.try_get("job_id")?,
                status: r.try_get("status")?,
                kicked_at: r.try_get("kicked_at")?,
                updated_at: r.try_get("updated_at")?,
            })
        })
        .transpose()
        .context("failed to decode latest morning recap run")
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

        row.map(|r| -> Result<SurfaceRunRecord> {
            Ok(SurfaceRunRecord {
                job_id: r.try_get("job_id")?,
                status: r.try_get("status")?,
                kicked_at: r.try_get("started_at")?,
                updated_at: r.try_get("updated_at")?,
            })
        })
        .transpose()
        .context("failed to decode latest pulse generation run")
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

        row.map(|r| -> Result<PulseSurfaceProvenance> {
            let finished_at: DateTime<Utc> = r.try_get("finished_at")?;
            let window_days: Option<i32> = r.try_get("window_days")?;
            let window_end: Option<DateTime<Utc>> = r.try_get("window_end")?;
            let window = match (window_days, window_end) {
                (Some(days), Some(end)) => {
                    let start = end - Duration::days(i64::from(days));
                    Some((start, end))
                }
                _ => None,
            };
            Ok(PulseSurfaceProvenance {
                finished_at,
                window,
            })
        })
        .transpose()
        .context("failed to decode pulse provenance")
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

        row.map(|r| -> Result<(DateTime<Utc>, DateTime<Utc>)> {
            let window_days: i32 = r.try_get("window_days")?;
            let window_end: DateTime<Utc> = r.try_get("window_end")?;
            let window_start = window_end - Duration::days(i64::from(window_days));
            Ok((window_start, window_end))
        })
        .transpose()
        .context("failed to decode recap job window")
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

        row.map(|r| r.try_get("updated_at"))
            .transpose()
            .context("failed to decode recap job updated_at")
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sqlx::postgres::PgPoolOptions;

    #[tokio::test]
    async fn get_latest_three_day_recap_run_excludes_cards_and_morning_triggers() -> Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };

        let Ok(pool) = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await
        else {
            return Ok(());
        };

        sqlx::query(
            r"
            CREATE TEMP TABLE recap_jobs (
                job_id UUID PRIMARY KEY,
                kicked_at TIMESTAMPTZ NOT NULL,
                status TEXT NOT NULL,
                window_days INTEGER NOT NULL,
                trigger_source TEXT NOT NULL,
                updated_at TIMESTAMPTZ NOT NULL
            )
            ",
        )
        .execute(&pool)
        .await?;

        let three_day_job_id = Uuid::new_v4();
        let cards_job_id = Uuid::new_v4();
        let cards_replay_job_id = Uuid::new_v4();
        let morning_job_id = Uuid::new_v4();

        let base_time = DateTime::parse_from_rfc3339("2026-01-26T00:00:00Z")?.with_timezone(&Utc);

        for (job_id, trigger_source, offset_hours) in [
            (three_day_job_id, "system", 0),
            (cards_job_id, "cards", 1),
            (cards_replay_job_id, "cards_replay", 2),
            (morning_job_id, "morning", 3),
        ] {
            let kicked_at = base_time + Duration::hours(offset_hours);
            let updated_at = kicked_at + Duration::minutes(5);
            sqlx::query(
                r"
                INSERT INTO recap_jobs (job_id, kicked_at, status, window_days, trigger_source, updated_at)
                VALUES ($1, $2, 'completed', 3, $3, $4)
                ",
            )
            .bind(job_id)
            .bind(kicked_at)
            .bind(trigger_source)
            .bind(updated_at)
            .execute(&pool)
            .await?;
        }

        let latest = SurfaceDaoOps::get_latest_three_day_recap_run(&pool)
            .await?
            .expect("3-day recap run should be returned");
        assert_eq!(latest.job_id, three_day_job_id);

        Ok(())
    }
}
