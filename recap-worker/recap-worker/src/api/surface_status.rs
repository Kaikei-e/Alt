use chrono::{DateTime, Duration, SecondsFormat, Utc};
use serde::{Deserialize, Serialize};
use serde_json::Value;
use uuid::Uuid;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SurfaceRun {
    pub job_id: String,
    pub status: String,
    pub kicked_at: String,
    pub updated_at: String,
}

impl SurfaceRun {
    #[must_use]
    pub fn new(
        _job_id: Uuid,
        _status: &str,
        _kicked_at: DateTime<Utc>,
        _updated_at: DateTime<Utc>,
    ) -> Self {
        Self {
            job_id: String::new(),
            status: String::new(),
            kicked_at: String::new(),
            updated_at: String::new(),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct DegradationReason {
    pub code: String,
    pub detail: String,
}

impl DegradationReason {
    #[must_use]
    pub fn new(code: impl Into<String>, detail: impl Into<String>) -> Self {
        Self {
            code: code.into(),
            detail: detail.into(),
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SurfaceStatus {
    pub generated_at: String,
    pub window_start: String,
    pub window_end: String,
    pub is_stale: bool,
    pub stale_after: String,
    pub latest_run: Option<SurfaceRun>,
    pub degradation_reasons: Vec<DegradationReason>,
}

impl SurfaceStatus {
    #[must_use]
    pub fn without_artifact(_latest_run: Option<SurfaceRun>) -> Self {
        Self {
            generated_at: String::new(),
            window_start: String::new(),
            window_end: String::new(),
            is_stale: true,
            stale_after: String::new(),
            latest_run: None,
            degradation_reasons: Vec::new(),
        }
    }
}

#[must_use]
pub fn format_rfc3339(dt: DateTime<Utc>) -> String {
    dt.to_rfc3339_opts(SecondsFormat::Secs, true)
}

#[must_use]
pub fn normalize_run_status(_raw: &str) -> &'static str {
    ""
}

#[must_use]
pub fn build_surface_status(
    _generated_at: DateTime<Utc>,
    _window: Option<(DateTime<Utc>, DateTime<Utc>)>,
    _latest_run: Option<SurfaceRun>,
    _degradation_reasons: Vec<DegradationReason>,
    _freshness_budget: Duration,
    _now: DateTime<Utc>,
) -> SurfaceStatus {
    SurfaceStatus::without_artifact(None)
}

#[must_use]
pub fn is_body_json_degraded(_body_json: &Value) -> bool {
    false
}

#[must_use]
pub fn three_day_recap_reasons(
    _degraded_genres: usize,
    _total_genres: usize,
) -> Vec<DegradationReason> {
    Vec::new()
}

#[must_use]
pub fn topic_cards_reasons(_cards_selected: i32, _degraded: bool) -> Vec<DegradationReason> {
    Vec::new()
}

#[must_use]
pub fn evening_pulse_reasons(_fallback_level: i32) -> Vec<DegradationReason> {
    Vec::new()
}

#[must_use]
pub fn morning_letter_reasons(
    _is_degraded: bool,
    _generation_metadata_jsonb: &Value,
) -> Vec<DegradationReason> {
    Vec::new()
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn ts(s: &str) -> DateTime<Utc> {
        DateTime::parse_from_rfc3339(s)
            .expect("valid rfc3339")
            .with_timezone(&Utc)
    }

    #[test]
    fn builder_marks_fresh_at_exact_stale_after_boundary_and_stale_after_it() {
        let generated_at = ts("2026-11-08T00:00:00Z");
        let window_start = ts("2026-11-05T00:00:00Z");
        let window_end = ts("2026-11-08T00:00:00Z");
        let budget = Duration::hours(30);
        let at_boundary = ts("2026-11-09T06:00:00Z");
        let past_boundary = ts("2026-11-09T06:00:01Z");

        let fresh = build_surface_status(
            generated_at,
            Some((window_start, window_end)),
            None,
            Vec::new(),
            budget,
            at_boundary,
        );
        assert_eq!(fresh.generated_at, "2026-11-08T00:00:00Z");
        assert_eq!(fresh.window_start, "2026-11-05T00:00:00Z");
        assert_eq!(fresh.window_end, "2026-11-08T00:00:00Z");
        assert_eq!(fresh.stale_after, "2026-11-09T06:00:00Z");
        assert!(!fresh.is_stale, "now == stale_after must not be stale");

        let stale = build_surface_status(
            generated_at,
            Some((window_start, window_end)),
            None,
            Vec::new(),
            budget,
            past_boundary,
        );
        assert!(stale.is_stale, "now > stale_after must be stale");
    }

    #[test]
    fn builder_serializes_empty_reasons_as_empty_array_and_null_latest_run() {
        let status = build_surface_status(
            ts("2026-11-09T00:00:00Z"),
            None,
            None,
            Vec::new(),
            Duration::hours(3),
            ts("2026-11-09T01:00:00Z"),
        );

        let val = serde_json::to_value(&status).expect("serializes");
        assert_eq!(val["window_start"], "");
        assert_eq!(val["window_end"], "");
        assert!(val["latest_run"].is_null());
        assert_eq!(val["degradation_reasons"], json!([]));
    }

    #[test]
    fn surface_run_normalizes_status_vocabulary() {
        let job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
        let kicked = ts("2026-11-09T00:00:00Z");
        let updated = ts("2026-11-09T00:05:00Z");

        let pulse_run = SurfaceRun::new(job_id, "succeeded", kicked, updated);
        assert_eq!(pulse_run.status, "completed");
        assert_eq!(pulse_run.job_id, job_id.to_string());
        assert_eq!(pulse_run.kicked_at, "2026-11-09T00:00:00Z");
        assert_eq!(pulse_run.updated_at, "2026-11-09T00:05:00Z");

        let morning_run = SurfaceRun::new(job_id, "morning_completed", kicked, updated);
        assert_eq!(morning_run.status, "completed");

        let pending_run = SurfaceRun::new(job_id, "pending", kicked, updated);
        assert_eq!(pending_run.status, "pending");

        let running_run = SurfaceRun::new(job_id, "running", kicked, updated);
        assert_eq!(running_run.status, "running");

        let failed_run = SurfaceRun::new(job_id, "failed", kicked, updated);
        assert_eq!(failed_run.status, "failed");
    }

    #[test]
    fn degradation_reason_builders_follow_contract_codes_and_details() {
        assert!(three_day_recap_reasons(0, 5).is_empty());
        assert_eq!(
            three_day_recap_reasons(2, 5),
            vec![DegradationReason::new(
                "extractive_genres",
                "2 of 5 genres are extractive excerpts, not AI summaries"
            )]
        );

        assert!(topic_cards_reasons(5, false).is_empty());
        assert_eq!(
            topic_cards_reasons(3, true),
            vec![DegradationReason::new("few_cards", "3 cards selected")]
        );

        assert!(evening_pulse_reasons(0).is_empty());
        assert_eq!(
            evening_pulse_reasons(2),
            vec![DegradationReason::new("pulse_fallback", "fallback level 2")]
        );

        assert!(
            morning_letter_reasons(false, &json!({"degradation_reason": "placeholder_output"}))
                .is_empty()
        );
        for known in [
            "placeholder_output",
            "duplicate_bullets",
            "low_japanese_ratio",
            "stale_recap_input",
            "recap_unavailable",
        ] {
            assert_eq!(
                morning_letter_reasons(true, &json!({"degradation_reason": known})),
                vec![DegradationReason::new(known, "")]
            );
        }
        assert_eq!(
            morning_letter_reasons(true, &json!({"degradation_reason": "something_else"})),
            vec![DegradationReason::new("generation_failed", "")]
        );
        assert_eq!(
            morning_letter_reasons(true, &json!({})),
            vec![DegradationReason::new("generation_failed", "")]
        );
    }
}
