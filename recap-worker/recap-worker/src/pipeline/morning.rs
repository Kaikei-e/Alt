use crate::clients::NewsCreatorClient;
use crate::clients::alt_backend::{AltBackendClient, AltBackendConfig};
use crate::clients::mtls::{self, MtlsPaths};
use crate::clients::news_creator::models::{
    MorningLetterGenerateRequest, MorningLetterGroupInput, MorningLetterRecapInput,
    RepresentativeSentence,
};
use crate::config::Config;
use crate::pipeline::dedup::{DedupStage, HashDedupStage};
use crate::pipeline::fetch::{AltBackendFetchStage, FetchStage};
use crate::pipeline::preprocess::{PreprocessStage, TextPreprocessStage};
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;
use crate::store::models::{MorningLetter, MorningLetterSource};
use crate::util::retry::RetryConfig;
use anyhow::Result;
use std::collections::HashMap;
use std::sync::Arc;
use uuid::Uuid;

struct RecapContext {
    summaries: Option<Vec<MorningLetterRecapInput>>,
    source_job_id: Option<Uuid>,
    is_degraded: bool,
    window_days: Option<u32>,
}

impl RecapContext {
    fn degraded() -> Self {
        Self { summaries: None, source_job_id: None, is_degraded: true, window_days: None }
    }
}

pub struct MorningPipeline {
    #[allow(dead_code)]
    config: Arc<Config>,
    fetch: Arc<dyn FetchStage>,
    preprocess: Arc<dyn PreprocessStage>,
    dedup: Arc<dyn DedupStage>,
    recap_dao: Arc<dyn RecapDao>,
    news_creator_client: Arc<NewsCreatorClient>,
}

impl MorningPipeline {
    pub(crate) fn new(
        config: Arc<Config>,
        recap_dao: Arc<dyn RecapDao>,
        news_creator_client: Arc<NewsCreatorClient>,
    ) -> Self {
        let mtls_paths = MtlsPaths::from_env().expect("mTLS env configuration (fail-closed)");
        let alt_backend_url = if mtls_paths.is_some() {
            std::env::var("ALT_BACKEND_MTLS_URL")
                .unwrap_or_else(|_| config.alt_backend_base_url().to_string())
        } else {
            config.alt_backend_base_url().to_string()
        };
        let alt_backend_config = AltBackendConfig {
            base_url: alt_backend_url,
            connect_timeout: config.alt_backend_connect_timeout(),
            total_timeout: config.alt_backend_total_timeout(),
            service_token: config.alt_backend_service_token().map(ToString::to_string),
        };
        let alt_backend_client = Arc::new(
            if let Some(paths) = mtls_paths.as_ref() {
                let client = mtls::build_mtls_client(
                    paths,
                    alt_backend_config.connect_timeout,
                    alt_backend_config.total_timeout,
                )
                .expect("failed to build alt-backend mTLS client (fail-closed)");
                AltBackendClient::new_with_client(alt_backend_config, client)
            } else {
                AltBackendClient::new(alt_backend_config)
            }
            .expect("failed to create alt-backend client"),
        );

        let retry_config = RetryConfig {
            max_attempts: config.http_max_retries(),
            base_delay_ms: config.http_backoff_base_ms(),
            max_delay_ms: config.http_backoff_cap_ms(),
        };

        let cpu_count = num_cpus::get();
        let max_concurrent = (cpu_count * 3) / 2;

        // Morning Update uses a 1-day window (window_days is now taken from JobContext)
        let fetch = Arc::new(AltBackendFetchStage::new(
            alt_backend_client,
            None, // No tag generator needed for just grouping
            Arc::clone(&recap_dao),
            retry_config,
        ));

        let subworker_client = Arc::new(
            crate::clients::SubworkerClient::new(
                config.subworker_base_url(),
                config.min_documents_per_genre(),
            )
            .expect("failed to create subworker client"),
        );
        let preprocess = Arc::new(TextPreprocessStage::new(
            max_concurrent.max(2),
            Arc::clone(&recap_dao),
            Arc::clone(&subworker_client),
        ));

        let dedup = Arc::new(HashDedupStage::with_defaults());

        Self {
            config,
            fetch,
            preprocess,
            dedup,
            recap_dao,
            news_creator_client,
        }
    }

    pub(crate) async fn execute_update(&self, job: &JobContext) -> Result<()> {
        tracing::info!(job_id = %job.job_id, "starting morning update pipeline");

        let fetched = self.fetch.fetch(job).await?;
        let preprocessed = self.preprocess.preprocess(job, fetched).await?;
        let deduplicated = self.dedup.deduplicate(job, preprocessed).await?;

        // Build groups and collect article metadata for Morning Letter generation
        let mut groups = Vec::new();
        let mut group_articles: HashMap<Uuid, Vec<RepresentativeSentence>> = HashMap::new();

        for article in &deduplicated.articles {
            let group_id = Uuid::new_v4();

            if let Ok(article_id) = Uuid::parse_str(&article.id) {
                groups.push((group_id, article_id, true));
                let repr = RepresentativeSentence {
                    text: article.title.clone().unwrap_or_default(),
                    published_at: article.published_at.map(|dt| dt.to_rfc3339()),
                    source_url: article.source_url.clone(),
                    article_id: Some(article.id.clone()),
                    is_centroid: true,
                };
                group_articles.entry(group_id).or_default().push(repr);
                for dup_id_str in &article.duplicates {
                    if let Ok(dup_id) = Uuid::parse_str(dup_id_str) {
                        groups.push((group_id, dup_id, false));
                    }
                }
            } else {
                tracing::warn!(article_id = %article.id, "skipping non-uuid article id");
            }
        }

        if !groups.is_empty() {
            self.recap_dao.save_morning_article_groups(&groups).await?;
            tracing::info!(job_id = %job.job_id, groups_count = groups.len(), "persisted morning article groups");
        }

        let recap_ctx = self.load_recap_context(job).await;

        let overnight_groups: Vec<MorningLetterGroupInput> = group_articles
            .into_iter()
            .map(|(group_id, articles)| MorningLetterGroupInput { group_id, articles })
            .collect();

        let target_date = chrono::Utc::now().format("%Y-%m-%d").to_string();
        let edition_timezone = "Asia/Tokyo".to_string();

        let request = MorningLetterGenerateRequest {
            target_date: target_date.clone(),
            edition_timezone: edition_timezone.clone(),
            recap_summaries: recap_ctx.summaries,
            overnight_groups,
        };

        let ml_response = self.news_creator_client.generate_morning_letter(&request).await?;
        let final_is_degraded = recap_ctx.is_degraded || ml_response.metadata.is_degraded;

        let letter_id = self
            .persist_morning_letter(
                &target_date,
                &edition_timezone,
                recap_ctx.source_job_id,
                recap_ctx.window_days,
                final_is_degraded,
                &ml_response,
                &groups,
            )
            .await?;

        tracing::info!(
            job_id = %job.job_id, letter_id = %letter_id,
            is_degraded = final_is_degraded,
            "morning letter generated and saved"
        );
        Ok(())
    }

    async fn load_recap_context(&self, job: &JobContext) -> RecapContext {
        // Morning Letter now grounds on the **3-day** recap window
        // (previously 7). The 3-day window is produced by the 3days
        // recap job and reflects fresher editorial signals.
        let recap_result = self.recap_dao.get_latest_completed_job(3).await;
        match recap_result {
            Ok(Some(recap_job)) => {
                let window_days = {
                    let duration = recap_job.window_end - recap_job.window_start;
                    #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
                    {
                        duration.num_days().max(1) as u32
                    }
                };
                match self.recap_dao.get_genres_by_job(recap_job.job_id).await {
                    Ok(genres) => {
                        // news-creator requires each recap to have at least
                        // one bullet (Pydantic `min_length=1`). Drop empties
                        // so 422 validation errors never reach the LLM path.
                        let inputs: Vec<_> = genres
                            .iter()
                            .filter_map(|g| {
                                let (title, bullets) =
                                    extract_title_and_bullets(g.summary_ja.as_deref());
                                if bullets.is_empty() {
                                    tracing::debug!(
                                        genre = %g.genre_name,
                                        "dropping recap summary with no bullets"
                                    );
                                    return None;
                                }
                                Some(MorningLetterRecapInput {
                                    genre: g.genre_name.clone(),
                                    title,
                                    bullets,
                                    window_days,
                                })
                            })
                            .collect();
                        tracing::info!(
                            job_id = %job.job_id, recap_job_id = %recap_job.job_id,
                            genre_count = genres.len(), "loaded recap summaries for morning letter"
                        );
                        RecapContext {
                            summaries: if inputs.is_empty() { None } else { Some(inputs) },
                            source_job_id: Some(recap_job.job_id),
                            is_degraded: false,
                            window_days: Some(window_days),
                        }
                    }
                    Err(e) => {
                        tracing::warn!(job_id = %job.job_id, error = %e, "failed to load recap genres, degraded mode");
                        RecapContext { summaries: None, source_job_id: Some(recap_job.job_id), is_degraded: true, window_days: Some(window_days) }
                    }
                }
            }
            Ok(None) => {
                tracing::info!(job_id = %job.job_id, "no completed recap found, morning letter will be degraded");
                RecapContext::degraded()
            }
            Err(e) => {
                tracing::warn!(job_id = %job.job_id, error = %e, "failed to query recap jobs, degraded mode");
                RecapContext::degraded()
            }
        }
    }

    #[allow(clippy::too_many_arguments)]
    async fn persist_morning_letter(
        &self,
        target_date: &str,
        edition_timezone: &str,
        source_recap_job_id: Option<Uuid>,
        recap_window_days: Option<u32>,
        is_degraded: bool,
        ml_response: &crate::clients::news_creator::models::MorningLetterGenerateResponse,
        groups: &[(Uuid, Uuid, bool)],
    ) -> Result<Uuid> {
        let letter_id = Uuid::new_v4();
        let content = &ml_response.content;

        // Event-sourced editorial enrichment: through-line, previous-letter
        // link, and per-bullet why_reasons — produced deterministically from
        // the signals we already have (recap summaries + overnight groups).
        let unique_groups: std::collections::HashSet<Uuid> =
            groups.iter().map(|(gid, _, _)| *gid).collect();
        let overnight_count = unique_groups.len();

        // Deterministic editorial through-line: prefer concrete section
        // labels the LLM chose, fall back to dominant genres, then to a
        // count sentence. The degraded flag is surfaced visually by the
        // UI badge, so we no longer prefix the prose with "Partial".
        let labels = editorial_labels_for(&content.sections, 2);
        let genres = dominant_genres_for(&content.sections, 2);
        let through_line = build_through_line(&labels, &genres, overnight_count);
        let _ = is_degraded; // kept in persisted metadata, not in the through-line
        let _ = recap_window_days;

        let previous = self
            .load_previous_letter_ref(edition_timezone, target_date)
            .await;

        let sections_json = content
            .sections
            .iter()
            .map(|s| {
                let whys: Vec<serde_json::Value> = s
                    .bullets
                    .iter()
                    .map(|_| {
                        let code = if s.genre.as_deref().is_some_and(|g| !g.is_empty()) {
                            "in_weekly_recap"
                        } else if s.key == "top3" || s.key == "what_changed" {
                            "pulse_need_to_know"
                        } else {
                            "new_unread"
                        };
                        serde_json::json!({ "code": code })
                    })
                    .collect();
                serde_json::json!({
                    "key": s.key,
                    "title": s.title,
                    "bullets": s.bullets,
                    "genre": s.genre,
                    "narrative": s.narrative,
                    "why_reasons": whys,
                })
            })
            .collect::<Vec<_>>();

        let result_jsonb = serde_json::json!({
            "lead": content.lead,
            "sections": sections_json,
            "generated_at": content.generated_at,
            "source_recap_window_days": recap_window_days,
            "through_line": through_line,
            "previous_letter_ref": previous,
        });
        let generation_metadata_jsonb = serde_json::json!({
            "model": ml_response.metadata.model,
            "is_degraded": is_degraded,
            "degradation_reason": ml_response.metadata.degradation_reason,
            "processing_time_ms": ml_response.metadata.processing_time_ms,
        });
        let target_date_parsed = chrono::NaiveDate::parse_from_str(target_date, "%Y-%m-%d")
            .unwrap_or_else(|_| chrono::Utc::now().date_naive());

        let letter = MorningLetter {
            id: letter_id,
            target_date: target_date_parsed,
            edition_timezone: edition_timezone.to_string(),
            source_recap_job_id,
            is_degraded,
            schema_version: content.schema_version,
            generation_revision: 1,
            result_jsonb,
            model: Some(ml_response.metadata.model.clone()),
            generation_metadata_jsonb,
            created_at: chrono::Utc::now(),
        };
        // UPSERT returns the *effective* letter id: on CONFLICT the
        // existing row's id is preserved, so the in-memory `letter_id`
        // we generated is discarded. Use the returned id for sources
        // (they FK-reference morning_letters.id).
        let effective_letter_id = self.recap_dao.save_morning_letter(&letter).await?;

        // morning_letter_sources typically has a uniqueness constraint on
        // (letter_id, section_key, article_id). The same article_id can
        // appear in `groups` multiple times when it is listed as a
        // duplicate of several primaries, so de-dup before insert.
        let mut seen_article_ids = std::collections::HashSet::new();
        let mut sources = Vec::new();
        for (_, article_id, _) in groups.iter() {
            if !seen_article_ids.insert(*article_id) {
                continue;
            }
            #[allow(clippy::cast_possible_truncation, clippy::cast_possible_wrap)]
            sources.push(MorningLetterSource {
                letter_id: effective_letter_id,
                section_key: "overnight".to_string(),
                article_id: *article_id,
                source_type: "overnight_group".to_string(),
                position: sources.len() as i32,
            });
        }
        if !sources.is_empty() {
            self.recap_dao.save_morning_letter_sources(&sources).await?;
        }
        Ok(effective_letter_id)
    }
}

impl MorningPipeline {
    /// Locate the previous Morning Letter in the same edition timezone and
    /// extract its through_line (or a placeholder) for the Since-yesterday band.
    async fn load_previous_letter_ref(
        &self,
        edition_timezone: &str,
        target_date: &str,
    ) -> Option<serde_json::Value> {
        let Ok(date) = chrono::NaiveDate::parse_from_str(target_date, "%Y-%m-%d") else {
            return None;
        };
        match self
            .recap_dao
            .get_previous_morning_letter(edition_timezone, date)
            .await
        {
            Ok(Some(prev)) => {
                let through_line = prev
                    .result_jsonb
                    .get("through_line")
                    .and_then(serde_json::Value::as_str)
                    .unwrap_or("Yesterday's edition")
                    .to_string();
                Some(serde_json::json!({
                    "id": prev.id.to_string(),
                    "target_date": prev.target_date.to_string(),
                    "through_line": through_line,
                }))
            }
            _ => None,
        }
    }
}

/// Build a deterministic one-sentence editorial through-line from the
/// deterministic signals we already have. Pure / idempotent so the line
/// is reproducible on reproject. The `is_degraded` flag is surfaced by
/// the UI badge separately — no need to prefix the prose with "Partial".
///
/// Precedence:
/// 1. Two or more labels → "X and Y led overnight — N new threads surfaced."
/// 2. One label → "X dominated overnight — N new threads surfaced."
/// 3. No labels but dominant genres → "Tech and data topics dominated
///    overnight — N new threads."
/// 4. No labels, no genres, overnight_count > 0 →
///    "Overnight brought N new threads across your feeds."
/// 5. Nothing at all → "A quiet day across your feeds …"
fn build_through_line(
    section_labels: &[&str],
    dominant_genres: &[&str],
    overnight_count: usize,
) -> String {
    if section_labels.is_empty() && dominant_genres.is_empty() && overnight_count == 0 {
        return "A quiet day across your feeds — nothing new surfaced overnight.".to_string();
    }

    let thread_tail = match overnight_count {
        0 => String::new(),
        1 => " — 1 new thread surfaced".to_string(),
        n => format!(" — {n} new threads surfaced"),
    };

    if section_labels.len() >= 2 {
        let a = section_labels[0];
        let b = section_labels[1];
        return format!("{a} and {b} led overnight{thread_tail}.");
    }

    if let Some(only) = section_labels.first() {
        return format!("{only} dominated overnight{thread_tail}.");
    }

    if !dominant_genres.is_empty() {
        let topics = if dominant_genres.len() >= 2 {
            format!("{} and {} topics", dominant_genres[0], dominant_genres[1])
        } else {
            format!("{} coverage", dominant_genres[0])
        };
        return format!("{topics} dominated overnight{thread_tail}.");
    }

    match overnight_count {
        0 => "A quiet day across your feeds — nothing new surfaced overnight.".to_string(),
        1 => "Overnight brought 1 new thread across your feeds.".to_string(),
        n => format!("Overnight brought {n} new threads across your feeds."),
    }
}

/// Pick up to `max` display labels from the Morning Letter sections,
/// skipping generic titles that add no editorial value.
fn editorial_labels_for(
    sections: &[crate::clients::news_creator::models::MorningLetterResponseSection],
    max: usize,
) -> Vec<&str> {
    const GENERIC: [&str; 6] = [
        "need to know",
        "today's headlines",
        "top stories",
        "what changed",
        "overview",
        "summary",
    ];
    let mut picked = Vec::with_capacity(max);
    for s in sections {
        let t = s.title.trim();
        if t.is_empty() {
            continue;
        }
        if GENERIC.iter().any(|g| g.eq_ignore_ascii_case(t)) {
            continue;
        }
        picked.push(t);
        if picked.len() >= max {
            break;
        }
    }
    picked
}

/// Collect unique dominant genres from sections, up to `max` in source order.
fn dominant_genres_for(
    sections: &[crate::clients::news_creator::models::MorningLetterResponseSection],
    max: usize,
) -> Vec<&str> {
    let mut seen = std::collections::HashSet::new();
    let mut out = Vec::with_capacity(max);
    for s in sections {
        if let Some(g) = s.genre.as_deref() {
            let g = g.trim();
            if g.is_empty() || !seen.insert(g) {
                continue;
            }
            out.push(g);
            if out.len() >= max {
                break;
            }
        }
    }
    out
}

/// Extract title and bullets from the summary_ja JSON string.
///
/// summary_ja is stored as a JSON string with format:
/// `{"summary": {"title": "...", "bullets": [...]}, ...}`
/// or simply `{"title": "...", "bullets": [...]}`
fn extract_title_and_bullets(summary_ja: Option<&str>) -> (String, Vec<String>) {
    let raw = match summary_ja {
        Some(s) if !s.trim().is_empty() => s,
        _ => return (String::new(), Vec::new()),
    };

    let value: serde_json::Value = match serde_json::from_str(raw) {
        Ok(v) => v,
        Err(_) => return (raw.to_string(), Vec::new()),
    };

    let Some(obj) = value.as_object() else {
        return (String::new(), Vec::new());
    };

    // Look for nested "summary" key first, then top-level
    let summary_obj = obj
        .get("summary")
        .and_then(serde_json::Value::as_object)
        .unwrap_or(obj);

    let title = summary_obj
        .get("title")
        .and_then(serde_json::Value::as_str)
        .unwrap_or("")
        .to_string();

    let bullets = summary_obj
        .get("bullets")
        .and_then(serde_json::Value::as_array)
        .map(|arr| {
            arr.iter()
                .filter_map(|v| match v {
                    serde_json::Value::String(s) => {
                        let trimmed = s.trim();
                        if trimmed.is_empty() {
                            None
                        } else {
                            Some(trimmed.to_string())
                        }
                    }
                    serde_json::Value::Object(obj) => obj
                        .get("text")
                        .and_then(serde_json::Value::as_str)
                        .map(str::trim)
                        .filter(|s| !s.is_empty())
                        .map(str::to_string),
                    _ => None,
                })
                .collect()
        })
        .unwrap_or_default();

    (title, bullets)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn extract_title_and_bullets_from_nested_summary() {
        let json = r#"{"summary": {"title": "Tech News", "bullets": ["Bullet 1", "Bullet 2"]}, "metadata": {}}"#;
        let (title, bullets) = extract_title_and_bullets(Some(json));
        assert_eq!(title, "Tech News");
        assert_eq!(bullets, vec!["Bullet 1", "Bullet 2"]);
    }

    #[test]
    fn extract_title_and_bullets_from_flat_json() {
        let json = r#"{"title": "Flat Title", "bullets": ["A", "B", "C"]}"#;
        let (title, bullets) = extract_title_and_bullets(Some(json));
        assert_eq!(title, "Flat Title");
        assert_eq!(bullets, vec!["A", "B", "C"]);
    }

    #[test]
    fn extract_title_and_bullets_with_object_bullets() {
        let json = r#"{"title": "ObjBullets", "bullets": [{"text": "X"}, {"text": "Y"}]}"#;
        let (title, bullets) = extract_title_and_bullets(Some(json));
        assert_eq!(title, "ObjBullets");
        assert_eq!(bullets, vec!["X", "Y"]);
    }

    #[test]
    fn extract_title_and_bullets_from_none() {
        let (title, bullets) = extract_title_and_bullets(None);
        assert!(title.is_empty());
        assert!(bullets.is_empty());
    }

    #[test]
    fn extract_title_and_bullets_from_empty_string() {
        let (title, bullets) = extract_title_and_bullets(Some(""));
        assert!(title.is_empty());
        assert!(bullets.is_empty());
    }

    #[test]
    fn extract_title_and_bullets_from_plain_text() {
        let (title, bullets) = extract_title_and_bullets(Some("plain text summary"));
        assert_eq!(title, "plain text summary");
        assert!(bullets.is_empty());
    }

    #[test]
    fn through_line_quiet_day() {
        let line = super::build_through_line(&[], &[], 0);
        assert!(line.contains("quiet day"), "expected quiet-day phrasing: {line}");
    }

    #[test]
    fn through_line_two_labels() {
        let line = super::build_through_line(
            &["AI tooling", "data pipelines"],
            &["AI", "Data"],
            7,
        );
        assert!(line.contains("AI tooling and data pipelines led overnight"), "got: {line}");
        assert!(line.contains("7 new threads surfaced"), "got: {line}");
        assert!(!line.contains("Partial"), "partial prefix should be gone: {line}");
    }

    #[test]
    fn through_line_one_label() {
        let line = super::build_through_line(&["AI regulation"], &[], 3);
        assert!(line.contains("AI regulation dominated overnight"), "got: {line}");
        assert!(line.contains("3 new threads surfaced"), "got: {line}");
    }

    #[test]
    fn through_line_genres_only() {
        let line = super::build_through_line(&[], &["Tech", "Data"], 5);
        assert!(line.contains("Tech and Data topics dominated overnight"), "got: {line}");
        assert!(line.contains("5 new threads surfaced"), "got: {line}");
    }

    #[test]
    fn through_line_overnight_count_only() {
        let line = super::build_through_line(&[], &[], 4);
        assert_eq!(
            line,
            "Overnight brought 4 new threads across your feeds.",
            "got: {line}"
        );
    }

    #[test]
    fn through_line_is_deterministic() {
        let a = super::build_through_line(&["AI", "Data"], &["Tech"], 3);
        let b = super::build_through_line(&["AI", "Data"], &["Tech"], 3);
        assert_eq!(a, b, "through-line must be deterministic for reproject safety");
    }

    #[test]
    fn through_line_never_says_partial() {
        // Regression: "Partial briefing: 257 overnight threads." is the bug
        // we're fixing. No branch of the composer should emit the word
        // "Partial" — degraded state is signalled by the UI badge only.
        for line in [
            super::build_through_line(&[], &[], 257),
            super::build_through_line(&["A"], &[], 1),
            super::build_through_line(&[], &["X", "Y"], 10),
            super::build_through_line(&[], &[], 0),
        ] {
            assert!(!line.contains("Partial"), "got: {line}");
        }
    }

    #[test]
    fn extract_title_and_bullets_filters_empty_bullets() {
        let json = r#"{"title": "T", "bullets": ["Good", "", "  ", "Also Good"]}"#;
        let (title, bullets) = extract_title_and_bullets(Some(json));
        assert_eq!(title, "T");
        assert_eq!(bullets, vec!["Good", "Also Good"]);
    }
}
