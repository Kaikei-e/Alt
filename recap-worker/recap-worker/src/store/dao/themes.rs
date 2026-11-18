//! Themes persistence DAO for read-theme runs, append-only versions, and publication ledger.

use crate::error::{RecapError, Result};
use chrono::{DateTime, Utc};
use serde::{Deserialize, Serialize};
use sqlx::types::Json;
use sqlx::{PgPool, Row};
use std::collections::HashMap;
use uuid::Uuid;

/// Claim sentence with source article citations used in key points, changes, and open questions.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeClaim {
    pub text: String,
    pub source_article_ids: Vec<Uuid>,
}

/// Dated timeline entry with source article citations.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeTimelineEntry {
    pub date: String,
    pub text: String,
    pub source_article_ids: Vec<Uuid>,
}

/// Source article metadata referenced by a theme version body.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeSource {
    pub article_id: Uuid,
    pub title: String,
    pub url: String,
}

/// Structured JSON body persisted in `read_theme_versions.body_json`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeBody {
    pub key_points: Vec<ThemeClaim>,
    pub timeline: Vec<ThemeTimelineEntry>,
    pub what_changed: Vec<ThemeClaim>,
    pub open_questions: Vec<ThemeClaim>,
    pub sources: Vec<ThemeSource>,
}

/// Execution status stored in `read_theme_runs.status`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ThemeRunStatus {
    Running,
    Completed,
    Failed,
}

impl ThemeRunStatus {
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::Running => "running",
            Self::Completed => "completed",
            Self::Failed => "failed",
        }
    }
}

impl TryFrom<&str> for ThemeRunStatus {
    type Error = anyhow::Error;

    fn try_from(value: &str) -> std::result::Result<Self, Self::Error> {
        match value {
            "running" => Ok(Self::Running),
            "completed" => Ok(Self::Completed),
            "failed" => Ok(Self::Failed),
            other => Err(anyhow::anyhow!("invalid read_theme_runs status: '{other}'")),
        }
    }
}

/// Terminal outcome and counters recorded when finishing a theme run.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeRunFinish {
    pub status: ThemeRunStatus,
    pub finished_at: DateTime<Utc>,
    pub source_count: i32,
    pub versions_created: i32,
    pub versions_deferred: i32,
    pub generation_rejected: i32,
    pub verification_dropped: i32,
    pub publish_failures: i32,
    pub error_message: Option<String>,
}

/// Outcome recorded for a skipped theme generation attempt.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "snake_case")]
pub enum ThemeAttemptOutcome {
    GenerationRejected,
    VerificationDropped,
}

impl ThemeAttemptOutcome {
    #[must_use]
    pub const fn as_str(self) -> &'static str {
        match self {
            Self::GenerationRejected => "generation_rejected",
            Self::VerificationDropped => "verification_dropped",
        }
    }
}

impl TryFrom<&str> for ThemeAttemptOutcome {
    type Error = anyhow::Error;

    fn try_from(value: &str) -> std::result::Result<Self, Self::Error> {
        match value {
            "generation_rejected" => Ok(Self::GenerationRejected),
            "verification_dropped" => Ok(Self::VerificationDropped),
            other => Err(anyhow::anyhow!(
                "invalid read_theme_generation_attempts outcome: '{other}'"
            )),
        }
    }
}

/// Insert-only record of a rejected or verification-dropped theme generation attempt.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct NewThemeGenerationAttempt {
    pub attempt_id: Uuid,
    pub run_id: Uuid,
    pub user_id: Uuid,
    pub theme_id: Uuid,
    pub membership_hash: String,
    pub outcome: ThemeAttemptOutcome,
    pub created_at: DateTime<Utc>,
}

/// Latest version summary per theme needed for continuation matching and prompt history.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct LatestThemeVersion {
    pub theme_id: Uuid,
    pub version_id: Uuid,
    pub version: i32,
    pub membership_hash: String,
    pub centroid: Vec<f32>,
    pub title: String,
    pub key_point_texts: Vec<String>,
    pub open_question_texts: Vec<String>,
    pub created_at: DateTime<Utc>,
}

/// Input payload for appending a new theme version.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct NewThemeVersion {
    pub version_id: Uuid,
    pub user_id: Uuid,
    pub theme_id: Uuid,
    pub version: i32,
    pub run_id: Uuid,
    pub title: String,
    pub member_article_ids: Vec<Uuid>,
    pub membership_hash: String,
    pub centroid: Vec<f32>,
    pub body: ThemeBody,
    pub created_at: DateTime<Utc>,
}

/// Detailed theme version returned for page rendering.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeVersionDetail {
    pub theme_id: Uuid,
    pub version_id: Uuid,
    pub version: i32,
    pub latest_version: i32,
    pub title: String,
    pub body: ThemeBody,
    pub member_article_ids: Vec<Uuid>,
    pub created_at: DateTime<Utc>,
}

/// Lightweight version history entry ordered newest first.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct ThemeVersionSummary {
    pub version_id: Uuid,
    pub version: i32,
    pub title: String,
    pub created_at: DateTime<Utc>,
    pub article_count: i32,
}

/// Theme version pending event publication.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct UnpublishedThemeVersion {
    pub version_id: Uuid,
    pub user_id: Uuid,
    pub theme_id: Uuid,
    pub version: i32,
    pub title: String,
    pub member_article_ids: Vec<Uuid>,
    pub created_at: DateTime<Utc>,
}

/// Extract previous key point and open question texts for LLM prompt continuity.
#[must_use]
#[allow(dead_code)]
pub fn extract_previous_prompt_texts(body: &ThemeBody) -> (Vec<String>, Vec<String>) {
    let key_points = body.key_points.iter().map(|kp| kp.text.clone()).collect();
    let open_questions = body
        .open_questions
        .iter()
        .map(|oq| oq.text.clone())
        .collect();
    (key_points, open_questions)
}

fn validate_non_nil_user(user_id: Uuid) -> Result<()> {
    if user_id.is_nil() {
        return Err(RecapError::Db("user_id must not be nil".to_string()));
    }
    Ok(())
}

fn validate_finish_input(finish: &ThemeRunFinish) -> Result<()> {
    if finish.status == ThemeRunStatus::Running {
        return Err(RecapError::Db(
            "finish_run requires terminal status (completed or failed)".to_string(),
        ));
    }
    if finish.source_count < 0
        || finish.versions_created < 0
        || finish.versions_deferred < 0
        || finish.generation_rejected < 0
        || finish.verification_dropped < 0
        || finish.publish_failures < 0
    {
        return Err(RecapError::Db(
            "finish_run counters must be non-negative".to_string(),
        ));
    }
    Ok(())
}

fn validate_generation_attempt(input: &NewThemeGenerationAttempt) -> Result<()> {
    validate_non_nil_user(input.user_id)?;
    if input.theme_id.is_nil() {
        return Err(RecapError::Db("theme_id must not be nil".to_string()));
    }
    if input.membership_hash.trim().is_empty() {
        return Err(RecapError::Db(
            "attempt membership_hash must not be empty".to_string(),
        ));
    }
    Ok(())
}

fn validate_new_theme_version(input: &NewThemeVersion) -> Result<()> {
    validate_non_nil_user(input.user_id)?;
    if input.theme_id.is_nil() {
        return Err(RecapError::Db("theme_id must not be nil".to_string()));
    }
    if input.version < 1 {
        return Err(RecapError::Db(format!(
            "theme version must be >= 1, got {}",
            input.version
        )));
    }
    if input.title.trim().is_empty() {
        return Err(RecapError::Db("theme title must not be empty".to_string()));
    }
    if input.membership_hash.trim().is_empty() {
        return Err(RecapError::Db(
            "theme membership_hash must not be empty".to_string(),
        ));
    }
    if input.centroid.is_empty() {
        return Err(RecapError::Db(
            "theme centroid must not be empty".to_string(),
        ));
    }
    Ok(())
}

pub struct ThemesDaoOps;

#[allow(dead_code)]
impl ThemesDaoOps {
    /// Insert a new run row in `running` state for a user.
    pub async fn create_run(
        pool: &PgPool,
        run_id: Uuid,
        user_id: Uuid,
        kicked_at: DateTime<Utc>,
        params_version: &str,
    ) -> Result<()> {
        validate_non_nil_user(user_id)?;
        if params_version.trim().is_empty() {
            return Err(RecapError::Db(
                "params_version must not be empty".to_string(),
            ));
        }

        sqlx::query(
            r"
            INSERT INTO read_theme_runs (
                run_id, user_id, kicked_at, status, params_version
            ) VALUES ($1, $2, $3, $4, $5)
            ",
        )
        .bind(run_id)
        .bind(user_id)
        .bind(kicked_at)
        .bind(ThemeRunStatus::Running.as_str())
        .bind(params_version)
        .execute(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to create read_theme_run: {e}")))?;

        Ok(())
    }

    /// Record terminal status and counters on an existing run owned by `user_id`.
    pub async fn finish_run(
        pool: &PgPool,
        run_id: Uuid,
        user_id: Uuid,
        finish: &ThemeRunFinish,
    ) -> Result<()> {
        validate_non_nil_user(user_id)?;
        validate_finish_input(finish)?;

        let result = sqlx::query(
            r"
            UPDATE read_theme_runs
            SET finished_at = $3,
                status = $4,
                source_count = $5,
                versions_created = $6,
                versions_deferred = $7,
                generation_rejected = $8,
                verification_dropped = $9,
                publish_failures = $10,
                error_message = $11
            WHERE run_id = $1 AND user_id = $2
            ",
        )
        .bind(run_id)
        .bind(user_id)
        .bind(finish.finished_at)
        .bind(finish.status.as_str())
        .bind(finish.source_count)
        .bind(finish.versions_created)
        .bind(finish.versions_deferred)
        .bind(finish.generation_rejected)
        .bind(finish.verification_dropped)
        .bind(finish.publish_failures)
        .bind(finish.error_message.as_deref())
        .execute(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to finish read_theme_run: {e}")))?;

        if result.rows_affected() != 1 {
            return Err(RecapError::Db(format!(
                "read_theme_run {run_id} for user {user_id} not found"
            )));
        }

        Ok(())
    }

    /// Fetch the latest version of every theme belonging to `user_id`.
    pub async fn latest_versions_for_user(
        pool: &PgPool,
        user_id: Uuid,
    ) -> Result<Vec<LatestThemeVersion>> {
        validate_non_nil_user(user_id)?;

        let rows = sqlx::query(
            r"
            SELECT DISTINCT ON (theme_id)
                version_id, theme_id, version, membership_hash, centroid,
                title, body_json, created_at
            FROM read_theme_versions
            WHERE user_id = $1
            ORDER BY theme_id, version DESC
            ",
        )
        .bind(user_id)
        .fetch_all(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to query latest theme versions: {e}")))?;

        let mut out = Vec::with_capacity(rows.len());
        for row in rows {
            let body: ThemeBody = row.try_get::<Json<ThemeBody>, _>("body_json")?.0;
            let (key_point_texts, open_question_texts) = extract_previous_prompt_texts(&body);
            out.push(LatestThemeVersion {
                theme_id: row.try_get("theme_id")?,
                version_id: row.try_get("version_id")?,
                version: row.try_get("version")?,
                membership_hash: row.try_get("membership_hash")?,
                centroid: row.try_get("centroid")?,
                title: row.try_get("title")?,
                key_point_texts,
                open_question_texts,
                created_at: row.try_get("created_at")?,
            });
        }
        Ok(out)
    }

    /// Insert the theme identity row if absent and append a new version in one transaction.
    pub async fn insert_theme_version(pool: &PgPool, input: &NewThemeVersion) -> Result<()> {
        validate_new_theme_version(input)?;

        let mut tx = pool
            .begin()
            .await
            .map_err(|e| RecapError::Db(format!("failed to begin insert_theme_version tx: {e}")))?;

        sqlx::query(
            r"
            INSERT INTO read_themes (user_id, theme_id, created_at)
            VALUES ($1, $2, $3)
            ON CONFLICT (user_id, theme_id) DO NOTHING
            ",
        )
        .bind(input.user_id)
        .bind(input.theme_id)
        .bind(input.created_at)
        .execute(&mut *tx)
        .await
        .map_err(|e| RecapError::Db(format!("failed to ensure read_themes identity row: {e}")))?;

        let current_row = sqlx::query(
            r"
            SELECT COALESCE(MAX(version), 0) AS current_version
            FROM read_theme_versions
            WHERE user_id = $1 AND theme_id = $2
            ",
        )
        .bind(input.user_id)
        .bind(input.theme_id)
        .fetch_one(&mut *tx)
        .await
        .map_err(|e| RecapError::Db(format!("failed to check current theme version: {e}")))?;
        let current_version: i32 = current_row.try_get("current_version")?;
        if input.version > current_version + 1 {
            return Err(RecapError::Db(format!(
                "theme version must be previous + 1 (current {current_version}, got {})",
                input.version
            )));
        }

        // No ON CONFLICT clause: concurrent or duplicate versions must fail on the unique constraint.
        sqlx::query(
            r"
            INSERT INTO read_theme_versions (
                version_id, user_id, theme_id, version, run_id,
                title, member_article_ids, membership_hash, centroid,
                body_json, created_at
            ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
            ",
        )
        .bind(input.version_id)
        .bind(input.user_id)
        .bind(input.theme_id)
        .bind(input.version)
        .bind(input.run_id)
        .bind(&input.title)
        .bind(&input.member_article_ids)
        .bind(&input.membership_hash)
        .bind(&input.centroid)
        .bind(Json(&input.body))
        .bind(input.created_at)
        .execute(&mut *tx)
        .await
        .map_err(|e| RecapError::Db(format!("failed to insert read_theme_version: {e}")))?;

        tx.commit().await.map_err(|e| {
            RecapError::Db(format!("failed to commit insert_theme_version tx: {e}"))
        })?;

        Ok(())
    }

    /// Retrieve a specific theme version or the latest version when `version` is `None`.
    pub async fn get_theme_version(
        pool: &PgPool,
        user_id: Uuid,
        theme_id: Uuid,
        version: Option<i32>,
    ) -> Result<Option<ThemeVersionDetail>> {
        validate_non_nil_user(user_id)?;
        if let Some(v) = version {
            if v < 1 {
                return Err(RecapError::Db(format!(
                    "theme version must be >= 1, got {v}"
                )));
            }
        }

        let row = sqlx::query(
            r"
            WITH max_v AS (
                SELECT MAX(version) AS latest_version
                FROM read_theme_versions
                WHERE user_id = $1 AND theme_id = $2
            )
            SELECT v.theme_id, v.version_id, v.version, m.latest_version,
                   v.title, v.body_json, v.member_article_ids, v.created_at
            FROM read_theme_versions v
            CROSS JOIN max_v m
            WHERE v.user_id = $1
              AND v.theme_id = $2
              AND ($3::int IS NULL OR v.version = $3)
            ORDER BY v.version DESC
            LIMIT 1
            ",
        )
        .bind(user_id)
        .bind(theme_id)
        .bind(version)
        .fetch_optional(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to get theme version: {e}")))?;

        match row {
            Some(r) => {
                let body: ThemeBody = r.try_get::<Json<ThemeBody>, _>("body_json")?.0;
                Ok(Some(ThemeVersionDetail {
                    theme_id: r.try_get("theme_id")?,
                    version_id: r.try_get("version_id")?,
                    version: r.try_get("version")?,
                    latest_version: r.try_get("latest_version")?,
                    title: r.try_get("title")?,
                    body,
                    member_article_ids: r.try_get("member_article_ids")?,
                    created_at: r.try_get("created_at")?,
                }))
            }
            None => Ok(None),
        }
    }

    /// List versions of a user's theme ordered newest first.
    pub async fn list_theme_versions(
        pool: &PgPool,
        user_id: Uuid,
        theme_id: Uuid,
    ) -> Result<Vec<ThemeVersionSummary>> {
        validate_non_nil_user(user_id)?;

        let rows = sqlx::query(
            r"
            SELECT version_id, version, title, created_at,
                   COALESCE(cardinality(member_article_ids), 0) AS article_count
            FROM read_theme_versions
            WHERE user_id = $1 AND theme_id = $2
            ORDER BY version DESC
            ",
        )
        .bind(user_id)
        .bind(theme_id)
        .fetch_all(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to list theme versions: {e}")))?;

        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            out.push(ThemeVersionSummary {
                version_id: r.try_get("version_id")?,
                version: r.try_get("version")?,
                title: r.try_get("title")?,
                created_at: r.try_get("created_at")?,
                article_count: r.try_get("article_count")?,
            });
        }
        Ok(out)
    }

    /// Return versions for `user_id` that have not yet been recorded in the publication ledger.
    pub async fn unpublished_versions(
        pool: &PgPool,
        user_id: Uuid,
    ) -> Result<Vec<UnpublishedThemeVersion>> {
        validate_non_nil_user(user_id)?;

        let rows = sqlx::query(
            r"
            SELECT v.version_id, v.user_id, v.theme_id, v.version,
                   v.title, v.member_article_ids, v.created_at
            FROM read_theme_versions v
            LEFT JOIN read_theme_version_publications p ON p.version_id = v.version_id
            WHERE v.user_id = $1
              AND p.version_id IS NULL
            ORDER BY v.created_at ASC, v.version ASC
            ",
        )
        .bind(user_id)
        .fetch_all(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to list unpublished theme versions: {e}")))?;

        let mut out = Vec::with_capacity(rows.len());
        for r in rows {
            out.push(UnpublishedThemeVersion {
                version_id: r.try_get("version_id")?,
                user_id: r.try_get("user_id")?,
                theme_id: r.try_get("theme_id")?,
                version: r.try_get("version")?,
                title: r.try_get("title")?,
                member_article_ids: r.try_get("member_article_ids")?,
                created_at: r.try_get("created_at")?,
            });
        }
        Ok(out)
    }

    /// Idempotently record that a user's theme version event reached knowledge-sovereign.
    pub async fn mark_published(
        pool: &PgPool,
        user_id: Uuid,
        version_id: Uuid,
        published_at: DateTime<Utc>,
    ) -> Result<()> {
        validate_non_nil_user(user_id)?;

        let exists_row = sqlx::query(
            r"
            SELECT EXISTS (
                SELECT 1
                FROM read_theme_versions
                WHERE version_id = $1 AND user_id = $2
            ) AS owned
            ",
        )
        .bind(version_id)
        .bind(user_id)
        .fetch_one(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to verify theme version ownership: {e}")))?;

        let owned: bool = exists_row.try_get("owned")?;
        if !owned {
            return Err(RecapError::Db(format!(
                "read_theme_version {version_id} for user {user_id} not found"
            )));
        }

        sqlx::query(
            r"
            INSERT INTO read_theme_version_publications (version_id, published_at)
            VALUES ($1, $2)
            ON CONFLICT (version_id) DO NOTHING
            ",
        )
        .bind(version_id)
        .bind(published_at)
        .execute(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to mark theme version published: {e}")))?;

        Ok(())
    }

    /// Return whether a completed theme run exists for `user_id` with `kicked_at >= since`.
    pub async fn has_completed_run_since(
        pool: &PgPool,
        user_id: Uuid,
        since: DateTime<Utc>,
    ) -> Result<bool> {
        validate_non_nil_user(user_id)?;
        let row = sqlx::query(
            r"
            SELECT EXISTS (
                SELECT 1 FROM read_theme_runs
                WHERE user_id = $1
                  AND status = 'completed'
                  AND kicked_at >= $2
            ) AS exists
            ",
        )
        .bind(user_id)
        .bind(since)
        .fetch_one(pool)
        .await
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to check completed read_theme_runs since slot: {e}"
            ))
        })?;

        let exists: bool = row.try_get("exists")?;
        Ok(exists)
    }

    /// Return the newest running theme run ID, if any.
    pub async fn find_running_run(pool: &PgPool) -> Result<Option<Uuid>> {
        let row = sqlx::query(
            r"
            SELECT run_id
            FROM read_theme_runs
            WHERE status = 'running'
            ORDER BY kicked_at DESC
            LIMIT 1
            ",
        )
        .fetch_optional(pool)
        .await
        .map_err(|e| RecapError::Db(format!("failed to query running read_theme_runs: {e}")))?;

        Ok(row.map(|r| r.get("run_id")))
    }

    /// Append one rejected or verification-dropped generation attempt record.
    pub async fn insert_generation_attempt(
        pool: &PgPool,
        input: &NewThemeGenerationAttempt,
    ) -> Result<()> {
        validate_generation_attempt(input)?;

        sqlx::query(
            r"
            INSERT INTO read_theme_generation_attempts (
                attempt_id, run_id, user_id, theme_id, membership_hash, outcome, created_at
            ) VALUES ($1, $2, $3, $4, $5, $6, $7)
            ",
        )
        .bind(input.attempt_id)
        .bind(input.run_id)
        .bind(input.user_id)
        .bind(input.theme_id)
        .bind(&input.membership_hash)
        .bind(input.outcome.as_str())
        .bind(input.created_at)
        .execute(pool)
        .await
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to insert read_theme_generation_attempts row: {e}"
            ))
        })?;

        Ok(())
    }

    /// Return the count of prior failed generation attempts per `(theme_id, membership_hash)` for `user_id`.
    pub async fn generation_attempt_counts_for_user(
        pool: &PgPool,
        user_id: Uuid,
    ) -> Result<HashMap<(Uuid, String), usize>> {
        validate_non_nil_user(user_id)?;

        let rows = sqlx::query(
            r"
            SELECT theme_id, membership_hash, COUNT(*) AS attempt_count
            FROM read_theme_generation_attempts
            WHERE user_id = $1
            GROUP BY theme_id, membership_hash
            ",
        )
        .bind(user_id)
        .fetch_all(pool)
        .await
        .map_err(|e| {
            RecapError::Db(format!(
                "failed to query read_theme_generation_attempts counts: {e}"
            ))
        })?;

        let mut out = HashMap::with_capacity(rows.len());
        for row in rows {
            let theme_id: Uuid = row.try_get("theme_id")?;
            let membership_hash: String = row.try_get("membership_hash")?;
            let count: i64 = row.try_get("attempt_count")?;
            out.insert(
                (theme_id, membership_hash),
                usize::try_from(count).unwrap_or(0),
            );
        }
        Ok(out)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sqlx::Executor;
    use sqlx::postgres::PgPoolOptions;

    fn sample_body() -> ThemeBody {
        let a1 = Uuid::new_v4();
        let a2 = Uuid::new_v4();
        ThemeBody {
            key_points: vec![ThemeClaim {
                text: "First key point".to_string(),
                source_article_ids: vec![a1],
            }],
            timeline: vec![ThemeTimelineEntry {
                date: "2026-11-18".to_string(),
                text: "Initial milestone".to_string(),
                source_article_ids: vec![a1, a2],
            }],
            what_changed: vec![ThemeClaim {
                text: "Added second source".to_string(),
                source_article_ids: vec![a2],
            }],
            open_questions: vec![ThemeClaim {
                text: "How does latency behave under load?".to_string(),
                source_article_ids: vec![a2],
            }],
            sources: vec![
                ThemeSource {
                    article_id: a1,
                    title: "Source One".to_string(),
                    url: "https://example.com/1".to_string(),
                },
                ThemeSource {
                    article_id: a2,
                    title: "Source Two".to_string(),
                    url: "https://example.com/2".to_string(),
                },
            ],
        }
    }

    #[test]
    fn theme_body_json_roundtrip_matches_expected_shape() {
        let body = sample_body();
        let value = serde_json::to_value(&body).expect("serialize ThemeBody");
        assert!(value["key_points"].is_array());
        assert!(value["timeline"][0]["date"].is_string());
        assert!(value["what_changed"].is_array());
        assert!(value["open_questions"].is_array());
        assert!(value["sources"][0]["article_id"].is_string());

        let decoded: ThemeBody = serde_json::from_value(value).expect("deserialize ThemeBody");
        assert_eq!(decoded, body);
    }

    #[test]
    fn extract_previous_prompt_texts_maps_key_points_and_open_questions() {
        let body = sample_body();
        let (kps, oqs) = extract_previous_prompt_texts(&body);
        assert_eq!(kps, vec!["First key point"]);
        assert_eq!(oqs, vec!["How does latency behave under load?"]);
    }

    #[test]
    fn theme_run_status_roundtrip_and_rejection() {
        assert_eq!(ThemeRunStatus::Running.as_str(), "running");
        assert_eq!(ThemeRunStatus::Completed.as_str(), "completed");
        assert_eq!(ThemeRunStatus::Failed.as_str(), "failed");
        assert_eq!(
            ThemeRunStatus::try_from("completed").unwrap(),
            ThemeRunStatus::Completed
        );
        assert!(ThemeRunStatus::try_from("unknown").is_err());
    }

    #[test]
    fn theme_attempt_outcome_roundtrip_and_rejection() {
        assert_eq!(
            ThemeAttemptOutcome::GenerationRejected.as_str(),
            "generation_rejected"
        );
        assert_eq!(
            ThemeAttemptOutcome::VerificationDropped.as_str(),
            "verification_dropped"
        );
        assert_eq!(
            ThemeAttemptOutcome::try_from("generation_rejected").unwrap(),
            ThemeAttemptOutcome::GenerationRejected
        );
        assert_eq!(
            ThemeAttemptOutcome::try_from("verification_dropped").unwrap(),
            ThemeAttemptOutcome::VerificationDropped
        );
        assert!(ThemeAttemptOutcome::try_from("unknown").is_err());
    }

    #[tokio::test]
    async fn validation_rejects_invalid_inputs_before_db() {
        let pool = PgPool::connect_lazy("postgres://localhost/dummy").unwrap();
        let now = Utc::now();

        let err_nil_user = ThemesDaoOps::create_run(&pool, Uuid::new_v4(), Uuid::nil(), now, "v1")
            .await
            .expect_err("nil user_id must fail");
        assert!(err_nil_user.to_string().contains("user_id"));

        let err_empty_params =
            ThemesDaoOps::create_run(&pool, Uuid::new_v4(), Uuid::new_v4(), now, "  ")
                .await
                .expect_err("empty params_version must fail");
        assert!(err_empty_params.to_string().contains("params_version"));

        let err_running_finish = ThemesDaoOps::finish_run(
            &pool,
            Uuid::new_v4(),
            Uuid::new_v4(),
            &ThemeRunFinish {
                status: ThemeRunStatus::Running,
                finished_at: now,
                source_count: 0,
                versions_created: 0,
                versions_deferred: 0,
                generation_rejected: 0,
                verification_dropped: 0,
                publish_failures: 0,
                error_message: None,
            },
        )
        .await
        .expect_err("non-terminal finish status must fail");
        assert!(err_running_finish.to_string().contains("terminal status"));

        let err_neg_rejected = ThemesDaoOps::finish_run(
            &pool,
            Uuid::new_v4(),
            Uuid::new_v4(),
            &ThemeRunFinish {
                status: ThemeRunStatus::Completed,
                finished_at: now,
                source_count: 0,
                versions_created: 0,
                versions_deferred: 0,
                generation_rejected: -1,
                verification_dropped: 0,
                publish_failures: 0,
                error_message: None,
            },
        )
        .await
        .expect_err("negative counter must fail");
        assert!(err_neg_rejected.to_string().contains("non-negative"));

        let err_bad_version = ThemesDaoOps::insert_theme_version(
            &pool,
            &NewThemeVersion {
                version_id: Uuid::new_v4(),
                user_id: Uuid::new_v4(),
                theme_id: Uuid::new_v4(),
                version: 0,
                run_id: Uuid::new_v4(),
                title: "Title".to_string(),
                member_article_ids: vec![Uuid::new_v4()],
                membership_hash: "hash".to_string(),
                centroid: vec![0.1, 0.2],
                body: sample_body(),
                created_at: now,
            },
        )
        .await
        .expect_err("version 0 must fail");
        assert!(err_bad_version.to_string().contains("version must be >= 1"));

        let err_get_zero =
            ThemesDaoOps::get_theme_version(&pool, Uuid::new_v4(), Uuid::new_v4(), Some(0))
                .await
                .expect_err("get version 0 must fail");
        assert!(err_get_zero.to_string().contains("version must be >= 1"));
    }

    #[tokio::test]
    async fn attempt_validation_rejects_invalid_inputs_before_db() {
        let pool = PgPool::connect_lazy("postgres://localhost/dummy").unwrap();
        let now = Utc::now();

        let err_nil_attempt_theme = ThemesDaoOps::insert_generation_attempt(
            &pool,
            &NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id: Uuid::new_v4(),
                user_id: Uuid::new_v4(),
                theme_id: Uuid::nil(),
                membership_hash: "h".to_string(),
                outcome: ThemeAttemptOutcome::GenerationRejected,
                created_at: now,
            },
        )
        .await
        .expect_err("nil theme_id must fail");
        assert!(err_nil_attempt_theme.to_string().contains("theme_id"));

        let err_empty_attempt_hash = ThemesDaoOps::insert_generation_attempt(
            &pool,
            &NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id: Uuid::new_v4(),
                user_id: Uuid::new_v4(),
                theme_id: Uuid::new_v4(),
                membership_hash: "   ".to_string(),
                outcome: ThemeAttemptOutcome::VerificationDropped,
                created_at: now,
            },
        )
        .await
        .expect_err("empty membership_hash must fail");
        assert!(
            err_empty_attempt_hash
                .to_string()
                .contains("membership_hash")
        );

        let err_nil_counts_user =
            ThemesDaoOps::generation_attempt_counts_for_user(&pool, Uuid::nil())
                .await
                .expect_err("nil user_id must fail");
        assert!(err_nil_counts_user.to_string().contains("user_id"));
    }

    async fn setup_themes_schema(pool: &PgPool) -> anyhow::Result<()> {
        pool.execute(
            r"
            CREATE TABLE IF NOT EXISTS read_theme_runs (
                run_id UUID PRIMARY KEY,
                user_id UUID NOT NULL,
                kicked_at TIMESTAMPTZ NOT NULL,
                finished_at TIMESTAMPTZ NULL,
                status TEXT NOT NULL CHECK (status IN ('running', 'completed', 'failed')),
                source_count INT NOT NULL DEFAULT 0,
                versions_created INT NOT NULL DEFAULT 0,
                versions_deferred INT NOT NULL DEFAULT 0,
                generation_rejected INT NOT NULL DEFAULT 0,
                verification_dropped INT NOT NULL DEFAULT 0,
                publish_failures INT NOT NULL DEFAULT 0,
                params_version TEXT NOT NULL,
                error_message TEXT NULL
            );

            ALTER TABLE read_theme_runs
                ADD COLUMN IF NOT EXISTS generation_rejected INT NOT NULL DEFAULT 0,
                ADD COLUMN IF NOT EXISTS verification_dropped INT NOT NULL DEFAULT 0,
                ADD COLUMN IF NOT EXISTS publish_failures INT NOT NULL DEFAULT 0;

            CREATE TABLE IF NOT EXISTS read_themes (
                user_id UUID NOT NULL,
                theme_id UUID NOT NULL,
                created_at TIMESTAMPTZ NOT NULL,
                PRIMARY KEY (user_id, theme_id)
            );

            CREATE TABLE IF NOT EXISTS read_theme_versions (
                version_id UUID PRIMARY KEY,
                user_id UUID NOT NULL,
                theme_id UUID NOT NULL,
                version INT NOT NULL CHECK (version >= 1),
                run_id UUID NOT NULL REFERENCES read_theme_runs (run_id),
                title TEXT NOT NULL,
                member_article_ids UUID[] NOT NULL,
                membership_hash TEXT NOT NULL,
                centroid REAL[] NOT NULL,
                body_json JSONB NOT NULL,
                created_at TIMESTAMPTZ NOT NULL,
                UNIQUE (user_id, theme_id, version),
                FOREIGN KEY (user_id, theme_id) REFERENCES read_themes (user_id, theme_id)
            );

            CREATE OR REPLACE FUNCTION prevent_read_theme_version_mutation()
            RETURNS trigger AS $$
            BEGIN
                RAISE EXCEPTION 'read_theme_versions is append-only: % is forbidden', TG_OP;
            END;
            $$ LANGUAGE plpgsql;

            DROP TRIGGER IF EXISTS trg_read_theme_versions_immutable ON read_theme_versions;
            CREATE TRIGGER trg_read_theme_versions_immutable
                BEFORE UPDATE OR DELETE ON read_theme_versions
                FOR EACH ROW
                EXECUTE FUNCTION prevent_read_theme_version_mutation();

            CREATE TABLE IF NOT EXISTS read_theme_version_publications (
                version_id UUID PRIMARY KEY REFERENCES read_theme_versions (version_id),
                published_at TIMESTAMPTZ NOT NULL
            );

            CREATE TABLE IF NOT EXISTS read_theme_generation_attempts (
                attempt_id UUID PRIMARY KEY,
                run_id UUID NOT NULL REFERENCES read_theme_runs (run_id),
                user_id UUID NOT NULL,
                theme_id UUID NOT NULL,
                membership_hash TEXT NOT NULL,
                outcome TEXT NOT NULL CHECK (outcome IN ('generation_rejected', 'verification_dropped')),
                created_at TIMESTAMPTZ NOT NULL
            );

            CREATE INDEX IF NOT EXISTS idx_read_theme_generation_attempts_lookup
                ON read_theme_generation_attempts (user_id, theme_id, membership_hash);
            ",
        )
        .await?;
        Ok(())
    }

    #[tokio::test]
    #[allow(clippy::too_many_lines)]
    async fn db_theme_lifecycle_versioning_and_user_isolation() -> anyhow::Result<()> {
        let Ok(database_url) = std::env::var("DATABASE_URL") else {
            return Ok(());
        };
        let pool = PgPoolOptions::new()
            .max_connections(2)
            .connect(&database_url)
            .await?;
        setup_themes_schema(&pool).await?;

        let user_a = Uuid::new_v4();
        let user_b = Uuid::new_v4();
        let theme_id = Uuid::new_v4();
        let run1_id = Uuid::new_v4();
        let run2_id = Uuid::new_v4();
        let t1 = Utc::now();
        let t2 = t1 + chrono::Duration::seconds(10);

        ThemesDaoOps::create_run(&pool, run1_id, user_a, t1, "themes-v1").await?;
        ThemesDaoOps::finish_run(
            &pool,
            run1_id,
            user_a,
            &ThemeRunFinish {
                status: ThemeRunStatus::Completed,
                finished_at: t1 + chrono::Duration::seconds(5),
                source_count: 10,
                versions_created: 1,
                versions_deferred: 2,
                generation_rejected: 3,
                verification_dropped: 4,
                publish_failures: 1,
                error_message: None,
            },
        )
        .await?;

        let run_row = sqlx::query(
            r"
            SELECT source_count, versions_created, versions_deferred,
                   generation_rejected, verification_dropped, publish_failures
            FROM read_theme_runs
            WHERE run_id = $1 AND user_id = $2
            ",
        )
        .bind(run1_id)
        .bind(user_a)
        .fetch_one(&pool)
        .await?;
        assert_eq!(run_row.try_get::<i32, _>("source_count")?, 10);
        assert_eq!(run_row.try_get::<i32, _>("versions_created")?, 1);
        assert_eq!(run_row.try_get::<i32, _>("versions_deferred")?, 2);
        assert_eq!(run_row.try_get::<i32, _>("generation_rejected")?, 3);
        assert_eq!(run_row.try_get::<i32, _>("verification_dropped")?, 4);
        assert_eq!(run_row.try_get::<i32, _>("publish_failures")?, 1);

        let v1_id = Uuid::new_v4();
        let v1 = NewThemeVersion {
            version_id: v1_id,
            user_id: user_a,
            theme_id,
            version: 1,
            run_id: run1_id,
            title: "Theme Title v1".to_string(),
            member_article_ids: vec![Uuid::new_v4()],
            membership_hash: "hash-v1".to_string(),
            centroid: vec![1.0, 0.0],
            body: sample_body(),
            created_at: t1,
        };
        ThemesDaoOps::insert_theme_version(&pool, &v1).await?;

        // Concurrent/duplicate version 1 must fail rather than overwrite.
        let dup_err = ThemesDaoOps::insert_theme_version(
            &pool,
            &NewThemeVersion {
                version_id: Uuid::new_v4(),
                ..v1.clone()
            },
        )
        .await;
        assert!(dup_err.is_err(), "duplicate version must fail");

        ThemesDaoOps::create_run(&pool, run2_id, user_a, t2, "themes-v1").await?;
        let v2_id = Uuid::new_v4();
        ThemesDaoOps::insert_theme_version(
            &pool,
            &NewThemeVersion {
                version_id: v2_id,
                user_id: user_a,
                theme_id,
                version: 2,
                run_id: run2_id,
                title: "Theme Title v2".to_string(),
                member_article_ids: vec![Uuid::new_v4(), Uuid::new_v4()],
                membership_hash: "hash-v2".to_string(),
                centroid: vec![0.9, 0.1],
                body: sample_body(),
                created_at: t2,
            },
        )
        .await?;

        // Append-only trigger rejects UPDATE and DELETE on read_theme_versions.
        let update_res =
            sqlx::query("UPDATE read_theme_versions SET title = 'mutated' WHERE version_id = $1")
                .bind(v1_id)
                .execute(&pool)
                .await;
        assert!(
            update_res.is_err(),
            "UPDATE on read_theme_versions must be blocked"
        );

        let delete_res = sqlx::query("DELETE FROM read_theme_versions WHERE version_id = $1")
            .bind(v1_id)
            .execute(&pool)
            .await;
        assert!(
            delete_res.is_err(),
            "DELETE on read_theme_versions must be blocked"
        );

        let latest = ThemesDaoOps::latest_versions_for_user(&pool, user_a).await?;
        assert_eq!(latest.len(), 1);
        assert_eq!(latest[0].version, 2);
        assert_eq!(latest[0].title, "Theme Title v2");
        assert_eq!(latest[0].key_point_texts, vec!["First key point"]);

        let detail_v1 = ThemesDaoOps::get_theme_version(&pool, user_a, theme_id, Some(1))
            .await?
            .expect("v1 exists");
        assert_eq!(detail_v1.version, 1);
        assert_eq!(detail_v1.latest_version, 2);

        let detail_latest = ThemesDaoOps::get_theme_version(&pool, user_a, theme_id, None)
            .await?
            .expect("latest exists");
        assert_eq!(detail_latest.version, 2);
        assert_eq!(detail_latest.latest_version, 2);

        let history = ThemesDaoOps::list_theme_versions(&pool, user_a, theme_id).await?;
        assert_eq!(history.len(), 2);
        assert_eq!(history[0].version, 2);
        assert_eq!(history[0].article_count, 2);
        assert_eq!(history[1].version, 1);
        assert_eq!(history[1].article_count, 1);

        // User B must not see User A's data.
        assert!(
            ThemesDaoOps::latest_versions_for_user(&pool, user_b)
                .await?
                .is_empty()
        );
        assert!(
            ThemesDaoOps::get_theme_version(&pool, user_b, theme_id, None)
                .await?
                .is_none()
        );

        // Publication ledger lifecycle + idempotency.
        let unpublished = ThemesDaoOps::unpublished_versions(&pool, user_a).await?;
        assert_eq!(unpublished.len(), 2);
        ThemesDaoOps::mark_published(&pool, user_a, v1_id, t2).await?;
        ThemesDaoOps::mark_published(&pool, user_a, v1_id, t2).await?;
        let remaining = ThemesDaoOps::unpublished_versions(&pool, user_a).await?;
        assert_eq!(remaining.len(), 1);
        assert_eq!(remaining[0].version_id, v2_id);

        // Generation attempts are insert-only, keyed by (user_id, theme_id, membership_hash),
        // and isolated by user_id.
        ThemesDaoOps::insert_generation_attempt(
            &pool,
            &NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id: run1_id,
                user_id: user_a,
                theme_id,
                membership_hash: "hash-failed-1".to_string(),
                outcome: ThemeAttemptOutcome::GenerationRejected,
                created_at: t1,
            },
        )
        .await?;
        ThemesDaoOps::insert_generation_attempt(
            &pool,
            &NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id: run2_id,
                user_id: user_a,
                theme_id,
                membership_hash: "hash-failed-1".to_string(),
                outcome: ThemeAttemptOutcome::VerificationDropped,
                created_at: t2,
            },
        )
        .await?;
        ThemesDaoOps::insert_generation_attempt(
            &pool,
            &NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id: run2_id,
                user_id: user_a,
                theme_id,
                membership_hash: "hash-failed-2".to_string(),
                outcome: ThemeAttemptOutcome::GenerationRejected,
                created_at: t2,
            },
        )
        .await?;
        ThemesDaoOps::insert_generation_attempt(
            &pool,
            &NewThemeGenerationAttempt {
                attempt_id: Uuid::new_v4(),
                run_id: run2_id,
                user_id: user_b,
                theme_id,
                membership_hash: "hash-failed-1".to_string(),
                outcome: ThemeAttemptOutcome::GenerationRejected,
                created_at: t2,
            },
        )
        .await?;

        let counts_a = ThemesDaoOps::generation_attempt_counts_for_user(&pool, user_a).await?;
        assert_eq!(
            counts_a.get(&(theme_id, "hash-failed-1".to_string())),
            Some(&2)
        );
        assert_eq!(
            counts_a.get(&(theme_id, "hash-failed-2".to_string())),
            Some(&1)
        );
        assert_eq!(counts_a.len(), 2);

        let counts_b = ThemesDaoOps::generation_attempt_counts_for_user(&pool, user_b).await?;
        assert_eq!(
            counts_b.get(&(theme_id, "hash-failed-1".to_string())),
            Some(&1)
        );
        assert_eq!(counts_b.len(), 1);

        Ok(())
    }
}
