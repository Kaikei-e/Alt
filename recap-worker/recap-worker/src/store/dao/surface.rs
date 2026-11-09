use crate::observability::surface_health::{SurfaceHealthSeed, SurfaceHealthTarget};
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

    pub async fn get_three_day_recap_health_seed(pool: &PgPool) -> Result<SurfaceHealthSeed> {
        let row = sqlx::query(
            r"
            WITH latest_success AS (
                SELECT MAX(ro.created_at) AS completed_at
                FROM recap_jobs rj
                JOIN recap_outputs ro ON ro.job_id = rj.job_id
                WHERE rj.window_days = 3
                  AND rj.status = 'completed'
                  AND rj.trigger_source NOT IN ('cards', 'cards_replay', 'morning')
                GROUP BY rj.job_id, rj.kicked_at, rj.updated_at
                ORDER BY MAX(ro.created_at) DESC, rj.kicked_at DESC, rj.updated_at DESC
                LIMIT 1
            )
            SELECT
                (SELECT completed_at FROM latest_success) AS last_success_at,
                (
                    SELECT COUNT(*)
                    FROM recap_jobs
                    WHERE window_days = 3
                      AND status = 'failed'
                      AND trigger_source NOT IN ('cards', 'cards_replay', 'morning')
                      AND (
                          (SELECT completed_at FROM latest_success) IS NULL
                          OR updated_at > (SELECT completed_at FROM latest_success)
                      )
                ) AS consecutive_failures
            ",
        )
        .fetch_one(pool)
        .await
        .context("failed to query three_day_recap health seed")?;

        let last_success_at: Option<DateTime<Utc>> = row.try_get("last_success_at")?;
        let consecutive_failures: i64 = row.try_get("consecutive_failures")?;
        Ok(SurfaceHealthSeed {
            last_success_at,
            consecutive_failures: u64::try_from(consecutive_failures).unwrap_or(0),
        })
    }

    pub async fn get_topic_cards_health_seed(pool: &PgPool) -> Result<SurfaceHealthSeed> {
        let row = sqlx::query(
            r"
            WITH latest_success AS (
                SELECT updated_at
                FROM recap_jobs
                WHERE trigger_source = 'cards'
                  AND status = 'completed'
                ORDER BY kicked_at DESC, updated_at DESC
                LIMIT 1
            )
            SELECT
                (SELECT updated_at FROM latest_success) AS last_success_at,
                (
                    SELECT COUNT(*)
                    FROM recap_jobs
                    WHERE trigger_source = 'cards'
                      AND status = 'failed'
                      AND (
                          (SELECT updated_at FROM latest_success) IS NULL
                          OR updated_at > (SELECT updated_at FROM latest_success)
                      )
                ) AS consecutive_failures
            ",
        )
        .fetch_one(pool)
        .await
        .context("failed to query topic_cards health seed")?;

        let last_success_at: Option<DateTime<Utc>> = row.try_get("last_success_at")?;
        let consecutive_failures: i64 = row.try_get("consecutive_failures")?;
        Ok(SurfaceHealthSeed {
            last_success_at,
            consecutive_failures: u64::try_from(consecutive_failures).unwrap_or(0),
        })
    }

    pub async fn get_evening_pulse_health_seed(pool: &PgPool) -> Result<SurfaceHealthSeed> {
        let row = sqlx::query(
            r"
            WITH latest_success AS (
                SELECT COALESCE(finished_at, started_at) AS completed_at
                FROM pulse_generations
                WHERE status = 'succeeded'
                ORDER BY COALESCE(finished_at, started_at) DESC, id DESC
                LIMIT 1
            )
            SELECT
                (SELECT completed_at FROM latest_success) AS last_success_at,
                (
                    SELECT COUNT(*)
                    FROM pulse_generations
                    WHERE status = 'failed'
                      AND (
                          (SELECT completed_at FROM latest_success) IS NULL
                          OR COALESCE(finished_at, started_at) > (SELECT completed_at FROM latest_success)
                      )
                ) AS consecutive_failures
            ",
        )
        .fetch_one(pool)
        .await
        .context("failed to query evening_pulse health seed")?;

        let last_success_at: Option<DateTime<Utc>> = row.try_get("last_success_at")?;
        let consecutive_failures: i64 = row.try_get("consecutive_failures")?;
        Ok(SurfaceHealthSeed {
            last_success_at,
            consecutive_failures: u64::try_from(consecutive_failures).unwrap_or(0),
        })
    }

    pub async fn get_morning_letter_health_seed(pool: &PgPool) -> Result<SurfaceHealthSeed> {
        let row = sqlx::query(
            r"
            WITH latest_success AS (
                SELECT created_at
                FROM morning_letters
                ORDER BY created_at DESC, generation_revision DESC
                LIMIT 1
            )
            SELECT
                (SELECT created_at FROM latest_success) AS last_success_at,
                (
                    SELECT COUNT(*)
                    FROM recap_jobs
                    WHERE trigger_source = 'morning'
                      AND status = 'failed'
                      AND (
                          (SELECT created_at FROM latest_success) IS NULL
                          OR updated_at > (SELECT created_at FROM latest_success)
                      )
                ) AS consecutive_failures
            ",
        )
        .fetch_one(pool)
        .await
        .context("failed to query morning_letter health seed")?;

        let last_success_at: Option<DateTime<Utc>> = row.try_get("last_success_at")?;
        let consecutive_failures: i64 = row.try_get("consecutive_failures")?;
        Ok(SurfaceHealthSeed {
            last_success_at,
            consecutive_failures: u64::try_from(consecutive_failures).unwrap_or(0),
        })
    }

    pub async fn get_all_surface_health_seeds(
        pool: &PgPool,
    ) -> Result<[(SurfaceHealthTarget, SurfaceHealthSeed); 4]> {
        let three_day = Self::get_three_day_recap_health_seed(pool).await?;
        let topic_cards = Self::get_topic_cards_health_seed(pool).await?;
        let evening_pulse = Self::get_evening_pulse_health_seed(pool).await?;
        let morning_letter = Self::get_morning_letter_health_seed(pool).await?;
        Ok([
            (SurfaceHealthTarget::ThreeDayRecap, three_day),
            (SurfaceHealthTarget::TopicCards, topic_cards),
            (SurfaceHealthTarget::EveningPulse, evening_pulse),
            (SurfaceHealthTarget::MorningLetter, morning_letter),
        ])
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

        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;

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

    async fn create_surface_health_temp_tables(pool: &PgPool) -> Result<()> {
        for ddl in [
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
            r"
            CREATE TEMP TABLE recap_outputs (
                job_id UUID NOT NULL,
                genre TEXT NOT NULL,
                created_at TIMESTAMPTZ NOT NULL
            )
            ",
            r"
            CREATE TEMP TABLE pulse_generations (
                id BIGSERIAL PRIMARY KEY,
                job_id UUID NOT NULL,
                target_date DATE NOT NULL,
                status TEXT NOT NULL,
                started_at TIMESTAMPTZ NOT NULL,
                finished_at TIMESTAMPTZ
            )
            ",
            r"
            CREATE TEMP TABLE morning_letters (
                id UUID PRIMARY KEY,
                target_date DATE NOT NULL,
                generation_revision INTEGER NOT NULL DEFAULT 1,
                created_at TIMESTAMPTZ NOT NULL
            )
            ",
        ] {
            sqlx::query(ddl).execute(pool).await?;
        }
        Ok(())
    }

    async fn seed_surface_health_rows(
        pool: &PgPool,
        t0: DateTime<Utc>,
        t1: DateTime<Utc>,
        t2: DateTime<Utc>,
        t3: DateTime<Utc>,
    ) -> Result<()> {
        let completed_job_id = Uuid::new_v4();
        for (job_id, ts, status) in [
            (Uuid::new_v4(), t0, "failed"),
            (completed_job_id, t1, "completed"),
            (Uuid::new_v4(), t1 + Duration::minutes(15), "failed"),
            (Uuid::new_v4(), t2, "failed"),
            (Uuid::new_v4(), t3, "failed"),
        ] {
            sqlx::query(
                r"
                INSERT INTO recap_jobs (job_id, kicked_at, status, window_days, trigger_source, updated_at)
                VALUES ($1, $2, $3, 3, 'system', $2)
                ",
            )
            .bind(job_id)
            .bind(ts)
            .bind(status)
            .execute(pool)
            .await?;
        }

        sqlx::query(
            r"
            INSERT INTO recap_outputs (job_id, genre, created_at)
            VALUES ($1, 'ai', $2),
                   ($1, 'space', $3)
            ",
        )
        .bind(completed_job_id)
        .bind(t1 + Duration::minutes(10))
        .bind(t1 + Duration::minutes(25))
        .execute(pool)
        .await?;

        sqlx::query(
            r"
            INSERT INTO recap_jobs (job_id, kicked_at, status, window_days, trigger_source, updated_at)
            VALUES ($1, $2, 'failed', 3, 'cards', $2),
                   ($3, $4, 'failed', 1, 'morning', $4)
            ",
        )
        .bind(Uuid::new_v4())
        .bind(t2)
        .bind(Uuid::new_v4())
        .bind(t3)
        .execute(pool)
        .await?;

        sqlx::query(
            r"
            INSERT INTO pulse_generations (job_id, target_date, status, started_at, finished_at)
            VALUES ($1, '2026-11-09', 'failed', $2, $2),
                   ($3, '2026-11-09', 'succeeded', $4, $4)
            ",
        )
        .bind(Uuid::new_v4())
        .bind(t0)
        .bind(Uuid::new_v4())
        .bind(t2)
        .execute(pool)
        .await?;

        sqlx::query(
            r"
            INSERT INTO morning_letters (id, target_date, generation_revision, created_at)
            VALUES ($1, '2026-11-09', 1, $2)
            ",
        )
        .bind(Uuid::new_v4())
        .bind(t1)
        .execute(pool)
        .await?;

        Ok(())
    }

    #[tokio::test]
    async fn surface_health_seeds_compute_last_success_and_newer_failures() -> Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };

        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await?;

        create_surface_health_temp_tables(&pool).await?;

        let empty_seeds = SurfaceDaoOps::get_all_surface_health_seeds(&pool).await?;
        for (_surface, seed) in empty_seeds {
            assert_eq!(seed.last_success_at, None);
            assert_eq!(seed.consecutive_failures, 0);
        }

        let t0 = DateTime::parse_from_rfc3339("2026-11-09T01:00:00Z")?.with_timezone(&Utc);
        let t1 = DateTime::parse_from_rfc3339("2026-11-09T02:00:00Z")?.with_timezone(&Utc);
        let t2 = DateTime::parse_from_rfc3339("2026-11-09T03:00:00Z")?.with_timezone(&Utc);
        let t3 = DateTime::parse_from_rfc3339("2026-11-09T04:00:00Z")?.with_timezone(&Utc);

        seed_surface_health_rows(&pool, t0, t1, t2, t3).await?;

        let seeds = SurfaceDaoOps::get_all_surface_health_seeds(&pool).await?;
        assert_eq!(
            seeds[0],
            (
                SurfaceHealthTarget::ThreeDayRecap,
                SurfaceHealthSeed {
                    last_success_at: Some(t1 + Duration::minutes(25)),
                    consecutive_failures: 2,
                }
            )
        );
        assert_eq!(
            seeds[1],
            (
                SurfaceHealthTarget::TopicCards,
                SurfaceHealthSeed {
                    last_success_at: None,
                    consecutive_failures: 1,
                }
            )
        );
        assert_eq!(
            seeds[2],
            (
                SurfaceHealthTarget::EveningPulse,
                SurfaceHealthSeed {
                    last_success_at: Some(t2),
                    consecutive_failures: 0,
                }
            )
        );
        assert_eq!(
            seeds[3],
            (
                SurfaceHealthTarget::MorningLetter,
                SurfaceHealthSeed {
                    last_success_at: Some(t1),
                    consecutive_failures: 1,
                }
            )
        );

        Ok(())
    }
}
