//! DAO methods for job status dashboard queries.

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use sqlx::{PgPool, Row};
use uuid::Uuid;

use super::types::{JobStatus, TriggerSource};
use crate::store::models::{ExtendedRecapJob, JobStats};

/// Job status DAO implementation
pub struct JobStatusDao;

impl JobStatusDao {
    /// Get extended job information including user_id and trigger_source.
    pub async fn get_extended_jobs(
        pool: &PgPool,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>> {
        let rows = sqlx::query(
            r"
            SELECT job_id, status, last_stage, kicked_at, updated_at,
                   user_id, COALESCE(trigger_source, 'system') as trigger_source, note
            FROM recap_jobs
            WHERE kicked_at > NOW() - make_interval(secs => $1)
            ORDER BY kicked_at DESC
            LIMIT $2
            ",
        )
        .bind(window_seconds as f64)
        .bind(limit)
        .fetch_all(pool)
        .await
        .context("failed to fetch extended recap jobs")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let job = parse_extended_job_row(&row)?;
            results.push(job);
        }

        Ok(results)
    }

    /// Get jobs for a specific user (either owned or containing user's articles).
    pub async fn get_user_jobs(
        pool: &PgPool,
        user_id: Uuid,
        window_seconds: i64,
        limit: i64,
    ) -> Result<Vec<ExtendedRecapJob>> {
        let rows = sqlx::query(
            r"
            SELECT DISTINCT rj.job_id, rj.status, rj.last_stage, rj.kicked_at, rj.updated_at,
                   rj.user_id, COALESCE(rj.trigger_source, 'system') as trigger_source, rj.note
            FROM recap_jobs rj
            LEFT JOIN recap_job_articles rja ON rja.job_id = rj.job_id
            WHERE rj.kicked_at > NOW() - make_interval(secs => $1)
              AND (rj.user_id = $2 OR rja.original_user_id = $2)
            ORDER BY rj.kicked_at DESC
            LIMIT $3
            ",
        )
        .bind(window_seconds as f64)
        .bind(user_id)
        .bind(limit)
        .fetch_all(pool)
        .await
        .context("failed to fetch user recap jobs")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let job = parse_extended_job_row(&row)?;
            results.push(job);
        }

        Ok(results)
    }

    /// Get currently running job.
    pub async fn get_running_job(pool: &PgPool) -> Result<Option<ExtendedRecapJob>> {
        let row = sqlx::query(
            r"
            SELECT job_id, status, last_stage, kicked_at, updated_at,
                   user_id, COALESCE(trigger_source, 'system') as trigger_source, note
            FROM recap_jobs
            WHERE status = 'running'
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .context("failed to fetch running job")?;

        match row {
            Some(row) => Ok(Some(parse_extended_job_row(&row)?)),
            None => Ok(None),
        }
    }

    /// Get job statistics for the last 24 hours.
    pub async fn get_job_stats(pool: &PgPool) -> Result<JobStats> {
        let row = sqlx::query(
            r"
            SELECT
                COUNT(*) FILTER (WHERE status = 'completed')::float / NULLIF(COUNT(*), 0)::float as success_rate,
                AVG(EXTRACT(EPOCH FROM (updated_at - kicked_at)))::bigint as avg_duration,
                COUNT(*) as total_jobs,
                COUNT(*) FILTER (WHERE status = 'running') as running_jobs,
                COUNT(*) FILTER (WHERE status = 'failed') as failed_jobs
            FROM recap_jobs
            WHERE kicked_at > NOW() - INTERVAL '24 hours'
            ",
        )
        .fetch_one(pool)
        .await
        .context("failed to fetch job stats")?;

        let success_rate: Option<f64> = row.try_get("success_rate")?;
        let avg_duration: Option<i64> = row.try_get("avg_duration")?;
        let total_jobs: i64 = row.try_get("total_jobs")?;
        let running_jobs: i64 = row.try_get("running_jobs")?;
        let failed_jobs: i64 = row.try_get("failed_jobs")?;

        Ok(JobStats {
            success_rate_24h: success_rate.unwrap_or(0.0),
            avg_duration_secs: avg_duration,
            total_jobs_24h: total_jobs as i32,
            running_jobs: running_jobs as i32,
            failed_jobs_24h: failed_jobs as i32,
        })
    }

    /// Get user article count for a specific job.
    pub async fn get_user_article_count_for_job(
        pool: &PgPool,
        job_id: Uuid,
        user_id: Uuid,
    ) -> Result<i32> {
        let row = sqlx::query(
            r"
            SELECT COUNT(*)::int as count
            FROM recap_job_articles
            WHERE job_id = $1 AND original_user_id = $2
            ",
        )
        .bind(job_id)
        .bind(user_id)
        .fetch_one(pool)
        .await
        .context("failed to count user articles for job")?;

        let count: i32 = row.try_get("count")?;
        Ok(count)
    }

    /// Get total article count for a job.
    pub async fn get_total_article_count_for_job(pool: &PgPool, job_id: Uuid) -> Result<i32> {
        let row = sqlx::query(
            r"
            SELECT COUNT(*)::int as count
            FROM recap_job_articles
            WHERE job_id = $1
            ",
        )
        .bind(job_id)
        .fetch_one(pool)
        .await
        .context("failed to count total articles for job")?;

        let count: i32 = row.try_get("count")?;
        Ok(count)
    }

    /// Get genre progress for a job from subworker runs.
    pub async fn get_genre_progress(
        pool: &PgPool,
        job_id: Uuid,
    ) -> Result<Vec<(String, String, Option<i32>)>> {
        let rows = sqlx::query(
            r"
            SELECT genre, status, cluster_count
            FROM recap_subworker_runs
            WHERE job_id = $1
            ORDER BY started_at
            ",
        )
        .bind(job_id)
        .fetch_all(pool)
        .await
        .context("failed to fetch genre progress")?;

        let mut results = Vec::with_capacity(rows.len());
        for row in rows {
            let genre: String = row.try_get("genre")?;
            let status: String = row.try_get("status")?;
            let cluster_count: Option<i32> = row.try_get("cluster_count").ok();
            results.push((genre, status, cluster_count));
        }

        Ok(results)
    }

    /// Get completed stages for a job.
    pub async fn get_completed_stages(pool: &PgPool, job_id: Uuid) -> Result<Vec<String>> {
        let rows = sqlx::query(
            r"
            SELECT DISTINCT stage
            FROM recap_job_stage_logs
            WHERE job_id = $1 AND status = 'completed'
            ORDER BY stage
            ",
        )
        .bind(job_id)
        .fetch_all(pool)
        .await
        .context("failed to fetch completed stages")?;

        let mut stages = Vec::with_capacity(rows.len());
        for row in rows {
            let stage: String = row.try_get("stage")?;
            stages.push(stage);
        }

        Ok(stages)
    }

    /// Create a new job with user_id and trigger_source.
    #[allow(dead_code)]
    pub async fn create_user_triggered_job(
        pool: &PgPool,
        job_id: Uuid,
        user_id: Uuid,
        note: Option<&str>,
    ) -> Result<()> {
        sqlx::query(
            r"
            INSERT INTO recap_jobs (job_id, kicked_at, note, user_id, trigger_source)
            VALUES ($1, NOW(), $2, $3, 'user')
            ON CONFLICT (job_id) DO NOTHING
            ",
        )
        .bind(job_id)
        .bind(note)
        .bind(user_id)
        .execute(pool)
        .await
        .context("failed to create user-triggered job")?;

        Ok(())
    }

    /// Get user jobs count.
    pub async fn get_user_jobs_count(
        pool: &PgPool,
        user_id: Uuid,
        window_seconds: i64,
    ) -> Result<i32> {
        let row = sqlx::query(
            r"
            SELECT COUNT(DISTINCT rj.job_id)::int as count
            FROM recap_jobs rj
            LEFT JOIN recap_job_articles rja ON rja.job_id = rj.job_id
            WHERE rj.kicked_at > NOW() - make_interval(secs => $1)
              AND (rj.user_id = $2 OR rja.original_user_id = $2)
            ",
        )
        .bind(window_seconds as f64)
        .bind(user_id)
        .fetch_one(pool)
        .await
        .context("failed to count user jobs")?;

        let count: i32 = row.try_get("count")?;
        Ok(count)
    }
}

fn parse_extended_job_row(row: &sqlx::postgres::PgRow) -> Result<ExtendedRecapJob> {
    let job_id: Uuid = row.try_get("job_id")?;
    let status_str: String = row.try_get("status")?;
    let last_stage: Option<String> = row.try_get("last_stage")?;
    let kicked_at: DateTime<Utc> = row.try_get("kicked_at")?;
    let updated_at: DateTime<Utc> = row.try_get("updated_at")?;
    let user_id: Option<Uuid> = row.try_get("user_id")?;
    let trigger_source_str: String = row.try_get("trigger_source")?;
    let note: Option<String> = row.try_get("note")?;

    let status = match status_str.as_str() {
        "pending" => JobStatus::Pending,
        "running" => JobStatus::Running,
        "completed" => JobStatus::Completed,
        _ => JobStatus::Failed,
    };

    let trigger_source = match trigger_source_str.as_str() {
        "user" => TriggerSource::User,
        _ => TriggerSource::System,
    };

    Ok(ExtendedRecapJob {
        job_id,
        status,
        last_stage,
        kicked_at,
        updated_at,
        user_id,
        trigger_source,
        note,
    })
}

#[cfg(test)]
mod tests {
    use super::*;
    use sqlx::postgres::PgPoolOptions;

    async fn setup_test_db() -> Option<PgPool> {
        let database_url = std::env::var("DATABASE_URL").ok()?;
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .connect(&database_url)
            .await
            .ok()?;
        Some(pool)
    }

    #[tokio::test]
    async fn test_get_job_stats() -> anyhow::Result<()> {
        let Some(pool) = setup_test_db().await else {
            return Ok(());
        };

        let stats = JobStatusDao::get_job_stats(&pool).await?;

        // Stats should be valid numbers
        assert!(stats.success_rate_24h >= 0.0 && stats.success_rate_24h <= 1.0);
        assert!(stats.total_jobs_24h >= 0);
        assert!(stats.running_jobs >= 0);
        assert!(stats.failed_jobs_24h >= 0);

        Ok(())
    }

    #[tokio::test]
    async fn test_get_extended_jobs() -> anyhow::Result<()> {
        let Some(pool) = setup_test_db().await else {
            return Ok(());
        };

        let jobs = JobStatusDao::get_extended_jobs(&pool, 86400, 10).await?;

        // Should return a vec (empty is fine for test)
        assert!(jobs.len() <= 10);

        Ok(())
    }
}
