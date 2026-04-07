use crate::clients::NewsCreatorClient;
use crate::clients::alt_backend::{AltBackendClient, AltBackendConfig};
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
        let alt_backend_config = AltBackendConfig {
            base_url: config.alt_backend_base_url().to_string(),
            connect_timeout: config.alt_backend_connect_timeout(),
            total_timeout: config.alt_backend_total_timeout(),
            service_token: config.alt_backend_service_token().map(ToString::to_string),
        };
        let alt_backend_client = Arc::new(
            AltBackendClient::new(alt_backend_config).expect("failed to create alt-backend client"),
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
                // Primary article
                groups.push((group_id, article_id, true));

                // Build representative sentence for the primary article
                let repr = RepresentativeSentence {
                    text: article.title.clone().unwrap_or_default(),
                    published_at: article.published_at.map(|dt| dt.to_rfc3339()),
                    source_url: article.source_url.clone(),
                    article_id: Some(article.id.clone()),
                    is_centroid: true,
                };
                group_articles.entry(group_id).or_default().push(repr);

                // Duplicates
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
            tracing::info!(
                job_id = %job.job_id,
                groups_count = groups.len(),
                "persisted morning article groups"
            );
        }

        // --- Morning Letter generation ---
        let target_date = chrono::Utc::now().format("%Y-%m-%d").to_string();
        let edition_timezone = "Asia/Tokyo".to_string();

        // Try to fetch the latest completed recap job (look back up to 7-day recaps)
        let recap_result = self.recap_dao.get_latest_completed_job(7).await;
        let mut recap_summaries = None;
        let mut source_recap_job_id = None;
        let mut is_degraded = false;
        let mut recap_window_days: Option<u32> = None;

        match recap_result {
            Ok(Some(recap_job)) => {
                source_recap_job_id = Some(recap_job.job_id);
                let window_days = {
                    let duration = recap_job.window_end - recap_job.window_start;
                    #[allow(clippy::cast_possible_truncation, clippy::cast_sign_loss)]
                    {
                        duration.num_days().max(1) as u32
                    }
                };
                recap_window_days = Some(window_days);

                match self.recap_dao.get_genres_by_job(recap_job.job_id).await {
                    Ok(genres) => {
                        let mut inputs = Vec::new();
                        for genre in &genres {
                            let (title, bullets) =
                                extract_title_and_bullets(genre.summary_ja.as_deref());
                            inputs.push(MorningLetterRecapInput {
                                genre: genre.genre_name.clone(),
                                title,
                                bullets,
                                window_days,
                            });
                        }
                        if !inputs.is_empty() {
                            recap_summaries = Some(inputs);
                        }
                        tracing::info!(
                            job_id = %job.job_id,
                            recap_job_id = %recap_job.job_id,
                            genre_count = genres.len(),
                            "loaded recap summaries for morning letter"
                        );
                    }
                    Err(e) => {
                        tracing::warn!(
                            job_id = %job.job_id,
                            error = %e,
                            "failed to load recap genres, proceeding in degraded mode"
                        );
                        is_degraded = true;
                    }
                }
            }
            Ok(None) => {
                tracing::info!(
                    job_id = %job.job_id,
                    "no completed recap found, morning letter will be degraded"
                );
                is_degraded = true;
            }
            Err(e) => {
                tracing::warn!(
                    job_id = %job.job_id,
                    error = %e,
                    "failed to query recap jobs, proceeding in degraded mode"
                );
                is_degraded = true;
            }
        }

        // Build overnight groups for the request
        let overnight_groups: Vec<MorningLetterGroupInput> = group_articles
            .into_iter()
            .map(|(group_id, articles)| MorningLetterGroupInput { group_id, articles })
            .collect();

        let request = MorningLetterGenerateRequest {
            target_date: target_date.clone(),
            edition_timezone: edition_timezone.clone(),
            recap_summaries,
            overnight_groups,
        };

        // Call news-creator to generate the Morning Letter
        let ml_response = self
            .news_creator_client
            .generate_morning_letter(&request)
            .await?;

        // Determine degradation from either our side or news-creator side
        let final_is_degraded = is_degraded || ml_response.metadata.is_degraded;

        // Build the MorningLetter model for DB persistence
        let letter_id = Uuid::new_v4();
        let content = &ml_response.content;
        let result_jsonb = serde_json::json!({
            "lead": content.lead,
            "sections": content.sections.iter().map(|s| {
                serde_json::json!({
                    "key": s.key,
                    "title": s.title,
                    "bullets": s.bullets,
                    "genre": s.genre,
                })
            }).collect::<Vec<_>>(),
            "generated_at": content.generated_at,
            "source_recap_window_days": recap_window_days,
        });

        let generation_metadata_jsonb = serde_json::json!({
            "model": ml_response.metadata.model,
            "is_degraded": final_is_degraded,
            "degradation_reason": ml_response.metadata.degradation_reason,
            "processing_time_ms": ml_response.metadata.processing_time_ms,
        });

        let target_date_parsed = chrono::NaiveDate::parse_from_str(&target_date, "%Y-%m-%d")
            .unwrap_or_else(|_| chrono::Utc::now().date_naive());

        let letter = MorningLetter {
            id: letter_id,
            target_date: target_date_parsed,
            edition_timezone: edition_timezone.clone(),
            source_recap_job_id,
            is_degraded: final_is_degraded,
            schema_version: content.schema_version,
            generation_revision: 1,
            result_jsonb,
            model: Some(ml_response.metadata.model.clone()),
            generation_metadata_jsonb,
            created_at: chrono::Utc::now(),
        };

        self.recap_dao.save_morning_letter(&letter).await?;

        // Build and save sources (provenance) from the response sections
        let mut sources = Vec::new();
        for section in &content.sections {
            // Each section's genre link provides provenance back to recap data
            if let Some(genre) = &section.genre {
                // We don't have direct article IDs from recap summaries,
                // but we can link the section to the recap genre for traceability
                tracing::debug!(
                    section_key = %section.key,
                    genre = %genre,
                    "morning letter section linked to recap genre"
                );
            }
        }

        // Save overnight article sources
        for (position, group) in groups.iter().enumerate() {
            let (_, article_id, _) = group;
            #[allow(clippy::cast_possible_truncation, clippy::cast_possible_wrap)]
            {
                sources.push(MorningLetterSource {
                    letter_id,
                    section_key: "overnight".to_string(),
                    article_id: *article_id,
                    source_type: "overnight_group".to_string(),
                    position: position as i32,
                });
            }
        }

        if !sources.is_empty() {
            self.recap_dao.save_morning_letter_sources(&sources).await?;
        }

        tracing::info!(
            job_id = %job.job_id,
            letter_id = %letter_id,
            is_degraded = final_is_degraded,
            sections = content.sections.len(),
            "morning letter generated and saved"
        );

        Ok(())
    }
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

    let obj = match value.as_object() {
        Some(o) => o,
        None => return (String::new(), Vec::new()),
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
                        .map(|s| s.trim())
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
    fn extract_title_and_bullets_filters_empty_bullets() {
        let json = r#"{"title": "T", "bullets": ["Good", "", "  ", "Also Good"]}"#;
        let (title, bullets) = extract_title_and_bullets(Some(json));
        assert_eq!(title, "T");
        assert_eq!(bullets, vec!["Good", "Also Good"]);
    }
}
