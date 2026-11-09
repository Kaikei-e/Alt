//! Pulse generation integration for the pipeline.

use std::collections::HashMap;

use chrono::{DateTime, NaiveDate, Utc};
use uuid::Uuid;

use crate::clients::knowledge_sovereign::KnowledgeSovereignClient;
use crate::clients::knowledge_sovereign::publish::publish_evening_pulse_generated;
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;

use super::PipelineOrchestrator;
use super::dispatch::DispatchResult;
use super::pulse::{self, PulseRollout, PulseStage};

impl PipelineOrchestrator {
    /// Generate Evening Pulse if enabled via rollout.
    ///
    /// Extracts cluster data from dispatch results and generates pulse topics.
    /// Results are saved to the pulse_generations table.
    pub(crate) async fn generate_pulse_if_enabled(
        &self,
        job: &JobContext,
        dispatched: &DispatchResult,
    ) {
        // Check if Pulse is enabled for this job
        if !self.pulse_rollout().allows(job.job_id) {
            tracing::debug!(
                job_id = %job.job_id,
                rollout_pct = self.pulse_rollout().percentage(),
                "pulse generation skipped: not in rollout"
            );
            return;
        }

        tracing::info!(
            job_id = %job.job_id,
            version = %self.pulse_rollout().version(),
            "starting pulse generation"
        );

        // Collect all article_ids from dispatch results
        let article_ids: Vec<String> = dispatched
            .genre_results
            .values()
            .filter_map(|gr| gr.clustering_response.as_ref())
            .flat_map(|cr| &cr.clusters)
            .flat_map(|c| &c.representatives)
            .map(|rep| rep.article_id.clone())
            .collect();

        // Fetch metadata (published_at, source_url) from DB
        let metadata = self
            .recap_dao()
            .get_article_metadata(job.job_id, &article_ids)
            .await
            .unwrap_or_else(|e| {
                tracing::warn!(
                    job_id = %job.job_id,
                    error = ?e,
                    "failed to fetch article metadata for pulse, using empty metadata"
                );
                HashMap::new()
            });

        tracing::debug!(
            job_id = %job.job_id,
            article_count = article_ids.len(),
            metadata_count = metadata.len(),
            "fetched article metadata for pulse generation"
        );

        // Convert dispatch results to PulseInput with metadata
        let pulse_input = build_pulse_input(job.job_id, dispatched, &metadata);

        // Generate pulse
        match self.pulse_stage().generate(pulse_input).await {
            Ok(result) => {
                let target_date = chrono::Utc::now().date_naive();
                let owner_scope = self
                    .config
                    .knowledge_owner()
                    .map(|o| (o.user_id, o.tenant_id));

                record_pulse_generation(
                    self.recap_dao().as_ref(),
                    self.sovereign_client.as_deref(),
                    owner_scope,
                    job.job_id,
                    &result,
                    target_date,
                )
                .await;
            }
            Err(e) => {
                tracing::warn!(
                    job_id = %job.job_id,
                    error = ?e,
                    "pulse generation failed"
                );
            }
        }
    }

    /// Access the pulse rollout configuration.
    pub(super) fn pulse_rollout(&self) -> &PulseRollout {
        &self.pulse_rollout
    }

    /// Access the pulse stage.
    pub(super) fn pulse_stage(&self) -> &dyn PulseStage {
        self.pulse_stage.as_ref()
    }
}

/// Metadata for an article (published_at, source_url).
type ArticleMetadata = HashMap<String, (Option<DateTime<Utc>>, Option<String>)>;

/// Build PulseInput from dispatch results with article metadata.
pub(super) fn build_pulse_input(
    job_id: uuid::Uuid,
    dispatched: &DispatchResult,
    metadata: &ArticleMetadata,
) -> pulse::PulseInput {
    let mut clusters = Vec::new();

    for (genre, genre_result) in &dispatched.genre_results {
        if let Some(clustering_response) = &genre_result.clustering_response {
            for cluster in &clustering_response.clusters {
                // Convert cluster representatives to articles with metadata from DB
                let articles: Vec<pulse::ArticleInput> = cluster
                    .representatives
                    .iter()
                    .map(|rep| {
                        let (published_at, source_url) = metadata
                            .get(&rep.article_id)
                            .cloned()
                            .unwrap_or((None, None));

                        pulse::ArticleInput {
                            id: rep.article_id.clone(),
                            title: rep.text.clone(),
                            source_url: source_url.unwrap_or_default(),
                            canonical_url: None,
                            og_url: None,
                            entities: Vec::new(), // Not stored in DB
                            published_at: published_at.map(|dt| dt.to_rfc3339()),
                        }
                    })
                    .collect();

                if !articles.is_empty() {
                    clusters.push(pulse::ClusterInput {
                        cluster_id: i64::from(cluster.cluster_id),
                        label: cluster
                            .label
                            .clone()
                            .or_else(|| Some(format!("{} Cluster {}", genre, cluster.cluster_id))),
                        articles,
                        embeddings: Vec::new(), // Not available in clustering response
                        impact_score: None,
                        burst_score: None,
                        novelty_score: None,
                        recency_score: None,
                        genre: Some(genre.clone()),
                    });
                }
            }
        }
    }

    pulse::PulseInput { job_id, clusters }
}

#[async_trait::async_trait]
pub(crate) trait PulseGenerationSink: Send + Sync {
    async fn save_pulse_generation(
        &self,
        result: &pulse::PulseResult,
        target_date: NaiveDate,
    ) -> crate::error::Result<(i64, DateTime<Utc>)>;
}

#[async_trait::async_trait]
impl<T: RecapDao + ?Sized> PulseGenerationSink for T {
    async fn save_pulse_generation(
        &self,
        result: &pulse::PulseResult,
        target_date: NaiveDate,
    ) -> crate::error::Result<(i64, DateTime<Utc>)> {
        RecapDao::save_pulse_generation(self, result, target_date).await
    }
}

pub(crate) async fn record_pulse_generation<D: PulseGenerationSink + ?Sized>(
    dao: &D,
    client: Option<&KnowledgeSovereignClient>,
    owner_scope: Option<(Uuid, Uuid)>,
    job_id: Uuid,
    result: &pulse::PulseResult,
    target_date: NaiveDate,
) -> Option<(i64, DateTime<Utc>)> {
    let topic_count = result.topic_count();
    match dao.save_pulse_generation(result, target_date).await {
        Ok((generation_id, finished_at)) => {
            tracing::info!(
                job_id = %job_id,
                generation_id = generation_id,
                topic_count = topic_count,
                version = %result.version,
                "pulse generation saved successfully"
            );
            if let Err(e) = publish_evening_pulse_generated(
                client,
                owner_scope,
                result,
                target_date,
                finished_at,
            )
            .await
            {
                tracing::error!(
                    job_id = %job_id,
                    error = ?e,
                    "failed to emit recap.evening_pulse_generated.v1"
                );
            }
            Some((generation_id, finished_at))
        }
        Err(e) => {
            tracing::warn!(
                job_id = %job_id,
                error = ?e,
                "failed to save pulse generation"
            );
            None
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use base64::Engine;
    use chrono::TimeZone;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use crate::pipeline::pulse::{
        ClusterQualityMetrics, PulseDiagnostics, PulseResult, PulseTopic, PulseVersion,
        QualityTier, ScoreBreakdown, TopicRole,
    };

    struct FixedFinishedAtDao {
        finished_at: DateTime<Utc>,
    }

    #[async_trait::async_trait]
    impl PulseGenerationSink for FixedFinishedAtDao {
        async fn save_pulse_generation(
            &self,
            _result: &PulseResult,
            _target_date: NaiveDate,
        ) -> crate::error::Result<(i64, DateTime<Utc>)> {
            Ok((42, self.finished_at))
        }
    }

    fn sample_topic() -> PulseTopic {
        PulseTopic {
            cluster_id: 1,
            role: TopicRole::NeedToKnow,
            score: 0.9,
            rationale: "important topic".to_string(),
            articles: vec!["art-1".to_string()],
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.8,
                ambiguity: 0.1,
                entity_consistency: 0.9,
                tier: QualityTier::Ok,
            },
            score_breakdown: ScoreBreakdown::default(),
            representative_articles: vec![],
            top_entities: vec![],
            source_names: vec![],
            genre: Some("AI".to_string()),
        }
    }

    #[tokio::test]
    async fn emits_evening_pulse_event_only_for_succeeded_with_occurred_at_equal_to_finished_at() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .respond_with(
                ResponseTemplate::new(200)
                    .insert_header("Content-Type", "application/json")
                    .set_body_string(r#"{"eventSeq":"77"}"#),
            )
            .expect(1)
            .mount(&server)
            .await;

        let client = KnowledgeSovereignClient::new_for_test(server.uri());
        let user_id = Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap();
        let tenant_id = Uuid::parse_str("11111111-1111-1111-1111-111111111111").unwrap();
        let job_id = Uuid::parse_str("22222222-2222-4222-8222-222222222222").unwrap();
        let target_date = NaiveDate::from_ymd_opt(2026, 11, 9).unwrap();
        let started_at = Utc.with_ymd_and_hms(2026, 11, 9, 10, 0, 0).unwrap();
        let finished_at = Utc.with_ymd_and_hms(2026, 11, 9, 18, 34, 56).unwrap();

        let dao = FixedFinishedAtDao { finished_at };

        let diagnostics = PulseDiagnostics {
            fallback_level: 1,
            ..Default::default()
        };
        let succeeded = PulseResult {
            job_id,
            version: PulseVersion::V4,
            topics: vec![sample_topic()],
            generated_at: started_at,
            diagnostics,
        };
        assert!(succeeded.is_success());

        let saved = record_pulse_generation(
            &dao,
            Some(&client),
            Some((user_id, tenant_id)),
            job_id,
            &succeeded,
            target_date,
        )
        .await;
        assert_eq!(saved, Some((42, finished_at)));

        let failed = PulseResult {
            job_id,
            version: PulseVersion::V4,
            topics: vec![],
            generated_at: started_at,
            diagnostics: PulseDiagnostics::default(),
        };
        assert!(!failed.is_success());
        let saved_failed = record_pulse_generation(
            &dao,
            Some(&client),
            Some((user_id, tenant_id)),
            job_id,
            &failed,
            target_date,
        )
        .await;
        assert_eq!(saved_failed, Some((42, finished_at)));

        let requests = server.received_requests().await.unwrap();
        assert_eq!(
            requests.len(),
            1,
            "only succeeded generation must emit event"
        );

        let body: serde_json::Value = serde_json::from_slice(&requests[0].body).unwrap();
        let event = &body["event"];
        assert_eq!(event["eventType"], "recap.evening_pulse_generated.v1");
        assert_eq!(event["occurredAt"], finished_at.to_rfc3339());
        assert_eq!(event["userId"], user_id.to_string());
        assert_eq!(event["tenantId"], tenant_id.to_string());
        assert_eq!(event["actorType"], "service:recap-worker");
        assert_eq!(event["actorId"], "pulse-stage");
        assert_eq!(event["aggregateType"], "evening_pulse");
        assert_eq!(event["aggregateId"], "evening-pulse:2026-11-09");
        assert_eq!(
            event["dedupeKey"],
            format!("recap.evening_pulse_generated.v1:{user_id}:{job_id}")
        );

        let payload_bytes = base64::engine::general_purpose::STANDARD
            .decode(event["payload"].as_str().unwrap())
            .unwrap();
        let payload: serde_json::Value = serde_json::from_slice(&payload_bytes).unwrap();
        assert_eq!(payload["job_id"], job_id.to_string());
        assert_eq!(payload["target_date"], "2026-11-09");
        assert_eq!(payload["version"], "v4");
        assert_eq!(payload["topic_count"], 1);
        assert_eq!(payload["fallback_level"], 1);
    }

    #[tokio::test]
    async fn unwired_client_fails_when_owner_scope_is_configured() {
        let user_id = Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap();
        let tenant_id = Uuid::parse_str("11111111-1111-1111-1111-111111111111").unwrap();
        let job_id = Uuid::parse_str("22222222-2222-4222-8222-222222222222").unwrap();
        let target_date = NaiveDate::from_ymd_opt(2026, 11, 9).unwrap();
        let finished_at = Utc.with_ymd_and_hms(2026, 11, 9, 18, 34, 56).unwrap();

        let succeeded = PulseResult {
            job_id,
            version: PulseVersion::V4,
            topics: vec![sample_topic()],
            generated_at: finished_at,
            diagnostics: PulseDiagnostics::default(),
        };

        let res = publish_evening_pulse_generated(
            None,
            Some((user_id, tenant_id)),
            &succeeded,
            target_date,
            finished_at,
        )
        .await;
        assert!(res.is_err(), "unwired client must not silently return Ok");
    }

    #[tokio::test]
    async fn saves_generation_even_when_emission_fails() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path(
                "/services.sovereign.v1.KnowledgeSovereignService/AppendKnowledgeEvent",
            ))
            .respond_with(ResponseTemplate::new(500).set_body_string("sovereign down"))
            .expect(1)
            .mount(&server)
            .await;

        let client = KnowledgeSovereignClient::new_for_test(server.uri());
        let user_id = Uuid::parse_str("44444444-4444-4444-4444-444444444444").unwrap();
        let tenant_id = Uuid::parse_str("11111111-1111-1111-1111-111111111111").unwrap();
        let job_id = Uuid::parse_str("22222222-2222-4222-8222-222222222222").unwrap();
        let target_date = NaiveDate::from_ymd_opt(2026, 11, 9).unwrap();
        let finished_at = Utc.with_ymd_and_hms(2026, 11, 9, 18, 34, 56).unwrap();

        let dao = FixedFinishedAtDao { finished_at };
        let succeeded = PulseResult {
            job_id,
            version: PulseVersion::V4,
            topics: vec![sample_topic()],
            generated_at: finished_at,
            diagnostics: PulseDiagnostics::default(),
        };

        let saved = record_pulse_generation(
            &dao,
            Some(&client),
            Some((user_id, tenant_id)),
            job_id,
            &succeeded,
            target_date,
        )
        .await;
        assert_eq!(
            saved,
            Some((42, finished_at)),
            "pulse generation must be saved even when event emission fails"
        );

        let saved_unwired = record_pulse_generation(
            &dao,
            None,
            Some((user_id, tenant_id)),
            job_id,
            &succeeded,
            target_date,
        )
        .await;
        assert_eq!(
            saved_unwired,
            Some((42, finished_at)),
            "pulse generation must be saved even when client is unwired"
        );
    }
}
