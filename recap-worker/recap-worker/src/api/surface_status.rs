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
        job_id: Uuid,
        status: &str,
        kicked_at: DateTime<Utc>,
        updated_at: DateTime<Utc>,
    ) -> Self {
        Self {
            job_id: job_id.to_string(),
            status: normalize_run_status(status).to_string(),
            kicked_at: format_rfc3339(kicked_at),
            updated_at: format_rfc3339(updated_at),
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

#[must_use]
pub fn format_rfc3339(dt: DateTime<Utc>) -> String {
    dt.to_rfc3339_opts(SecondsFormat::Secs, true)
}

#[must_use]
pub fn normalize_run_status(raw: &str) -> &'static str {
    match raw.trim().to_ascii_lowercase().as_str() {
        "pending" => "pending",
        "running" => "running",
        "completed" | "succeeded" | "morning_completed" => "completed",
        _ => "failed",
    }
}

#[must_use]
pub fn build_surface_status(
    generated_at: DateTime<Utc>,
    window: Option<(DateTime<Utc>, DateTime<Utc>)>,
    latest_run: Option<SurfaceRun>,
    degradation_reasons: Vec<DegradationReason>,
    freshness_budget: Duration,
    now: DateTime<Utc>,
) -> SurfaceStatus {
    let stale_after_dt = generated_at + freshness_budget;
    let (window_start, window_end) = match window {
        Some((start, end)) => (format_rfc3339(start), format_rfc3339(end)),
        None => (String::new(), String::new()),
    };

    SurfaceStatus {
        generated_at: format_rfc3339(generated_at),
        window_start,
        window_end,
        is_stale: now > stale_after_dt,
        stale_after: format_rfc3339(stale_after_dt),
        latest_run,
        degradation_reasons,
    }
}

#[must_use]
pub fn is_body_json_degraded(body_json: &Value) -> bool {
    body_json
        .get("metadata")
        .and_then(|m| m.get("is_degraded"))
        .and_then(Value::as_bool)
        .unwrap_or(false)
}

#[must_use]
pub fn three_day_recap_reasons(
    degraded_genres: usize,
    total_genres: usize,
) -> Vec<DegradationReason> {
    if degraded_genres == 0 {
        return Vec::new();
    }
    vec![DegradationReason::new(
        "extractive_genres",
        format!(
            "{degraded_genres} of {total_genres} genres are extractive excerpts, not AI summaries"
        ),
    )]
}

#[must_use]
pub fn topic_cards_reasons(cards_selected: i32, degraded: bool) -> Vec<DegradationReason> {
    if !degraded {
        return Vec::new();
    }
    vec![DegradationReason::new(
        "few_cards",
        format!("{cards_selected} cards selected"),
    )]
}

#[must_use]
pub fn evening_pulse_reasons(fallback_level: i32) -> Vec<DegradationReason> {
    if fallback_level <= 0 {
        return Vec::new();
    }
    vec![DegradationReason::new(
        "pulse_fallback",
        format!("fallback level {fallback_level}"),
    )]
}

#[must_use]
pub fn morning_letter_reasons(
    is_degraded: bool,
    generation_metadata_jsonb: &Value,
) -> Vec<DegradationReason> {
    if !is_degraded {
        return Vec::new();
    }
    let stored = generation_metadata_jsonb
        .get("degradation_reason")
        .and_then(Value::as_str);
    let code = stored
        .filter(|reason| {
            matches!(
                *reason,
                "placeholder_output"
                    | "duplicate_bullets"
                    | "low_japanese_ratio"
                    | "stale_recap_input"
                    | "recap_unavailable"
            )
        })
        .unwrap_or("generation_failed");
    vec![DegradationReason::new(code, "")]
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
