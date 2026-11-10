//! Provider verification for recap-worker.
//!
//! Replays consumer-driven pact files against a minimal stub axum Router
//! that mirrors the real recap-worker endpoints. Each interaction's request
//! is issued against the stub, and the response status + body shape is
//! compared against the pact's expected values.
//!
//! Lightweight by design — pact_verifier crate would be stronger but adds
//! ~50 transitive deps and multiplies compile time. The trade-off for a
//! subset of matchers is acceptable for a first provider-verify gate.
//!
//! Run with: `cargo test --test provider_verification -- --ignored`

use std::net::SocketAddr;
use std::path::{Path as FsPath, PathBuf};
use std::sync::{Arc, RwLock};

use axum::{
    Router,
    extract::Path,
    response::Json,
    routing::{get, post},
};
use chrono::{DateTime, NaiveDate, Utc};
use recap_worker::api::cards::{
    CardsDao, LatestCardsRun, PreviousCardsJobMeta, RecapCard, RecapCardJobStats,
    get_topic_cards_with_clock,
};
use recap_worker::api::fetch::{
    ClusterEvidence, ClusterWithEvidence, GenreWithSummary, MorningLetter, MorningLetterReadDao,
    RecapJob, RecapReadDao, get_3days_recap_impl, get_latest_morning_letter_impl,
    get_morning_letter_by_date_impl,
};
use recap_worker::api::pulse::{
    PulseGenerationRow, PulseReadDao, PulseWindow, get_latest_pulse_impl,
};
use recap_worker::api::surface_status::SurfaceRun;
use recap_worker::pipeline::pulse::{
    ClusterQualityMetrics, PulseDiagnostics, PulseResult, PulseTopic, PulseVersion, QualityTier,
    ScoreBreakdown, TopicRole, types::RepresentativeArticle,
};
use serde_json::{Value, json};
use uuid::Uuid;

/// Schema identifier for the verification-evidence document. The Ansible
/// bridge (`playbooks/_publish-bridge.yml`) refuses to publish a success
/// record unless it can read a document carrying exactly this schema.
const EVIDENCE_SCHEMA: &str = "alt.pact.evidence.v1";

/// Label the broker records as the verifier that produced the result. The
/// bridge registry names the same string and the two must agree, so the
/// `implementation` field on a verification record is an assertion about
/// what ran rather than a free-form note.
const EVIDENCE_IMPLEMENTATION: &str = "rust-stub-replay";

/// What one replayed pact proves. Filled in only on the success path — the
/// per-interaction assertions panic first, so an evidence file existing at
/// all means every interaction in that pact was replayed and matched.
struct Evidence<'a> {
    provider: &'a str,
    consumer: &'a str,
    implementation: &'a str,
    interactions_verified: usize,
    interactions_skipped: usize,
}

fn evidence_document(evidence: &Evidence<'_>, run_id: &str, provider_version: &str) -> Value {
    json!({
        "schema": EVIDENCE_SCHEMA,
        "run_id": run_id,
        "provider": evidence.provider,
        "consumer": evidence.consumer,
        "implementation": evidence.implementation,
        "provider_version": provider_version,
        "verifier_version": env!("CARGO_PKG_VERSION"),
        "verified_at": chrono::Utc::now().to_rfc3339_opts(chrono::SecondsFormat::Secs, true),
        "interactions_verified": evidence.interactions_verified,
        "interactions_skipped": evidence.interactions_skipped,
        "result": "passed",
    })
}

fn write_evidence_to(dir: &FsPath, doc: &Value) -> PathBuf {
    let path = dir.join(format!(
        "{}__{}.json",
        doc["provider"].as_str().expect("provider"),
        doc["consumer"].as_str().expect("consumer"),
    ));
    std::fs::create_dir_all(dir)
        .unwrap_or_else(|e| panic!("failed to create evidence dir {}: {e}", dir.display()));
    std::fs::write(
        &path,
        serde_json::to_vec_pretty(doc).expect("evidence is serializable"),
    )
    .unwrap_or_else(|e| panic!("failed to write evidence {}: {e}", path.display()));
    path
}

/// Emit the evidence document for one replayed pact. A write failure aborts
/// the test: the bridge treats a missing file as "never verified", so a
/// silently-dropped write would downgrade a passing verification into a
/// deploy-blocking failure with no explanation.
fn record_evidence(evidence: &Evidence<'_>) {
    let Ok(dir) = std::env::var("PACT_EVIDENCE_DIR") else {
        eprintln!(
            "PACT_EVIDENCE_DIR unset — no verification evidence emitted for {}/{}. \
             The bridge publish step will fail until this runs under scripts/pact-check.sh.",
            evidence.provider, evidence.consumer
        );
        return;
    };
    let run_id = std::env::var("PACT_EVIDENCE_RUN_ID").unwrap_or_default();
    let provider_version = std::env::var("PACT_PROVIDER_VERSION").unwrap_or_default();
    let doc = evidence_document(evidence, &run_id, &provider_version);
    let path = write_evidence_to(FsPath::new(&dir), &doc);
    eprintln!("verification evidence written: {}", path.display());
}

#[derive(Clone, Default)]
struct StubState {
    provider_state: Arc<RwLock<Option<String>>>,
}

struct VerificationCardsDao {
    provider_state: Arc<RwLock<Option<String>>>,
}

#[async_trait::async_trait]
impl CardsDao for VerificationCardsDao {
    async fn get_latest_completed_cards_job(&self) -> Result<Option<PreviousCardsJobMeta>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(None),
            Some(
                "a completed cards job with cards exists"
                | "a completed cards job whose card has no genre and no why"
                | "a failed topic cards run is newer than the latest completed run",
            ) => Ok(Some(PreviousCardsJobMeta {
                job_id: Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap(),
                kicked_at: DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                from_ts: DateTime::parse_from_rfc3339("2026-09-19T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                to_ts: DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                params_version: "cards-v0.2".to_string(),
            })),
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }

    async fn get_cards_for_job(&self, job_id: Uuid) -> Result<Vec<RecapCard>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(Vec::new()),
            Some(
                "a completed cards job with cards exists"
                | "a failed topic cards run is newer than the latest completed run",
            ) => Ok(vec![RecapCard {
                id: Uuid::parse_str("22222222-3333-4444-5555-666666666666").unwrap(),
                job_id,
                rank: 1,
                story_id: Uuid::parse_str("33333333-4444-5555-6666-777777777777").unwrap(),
                continues_card_id: None,
                merged_from: None,
                headline_ja: "日本のAIスタートアップが新モデルを発表".to_string(),
                what_ja: "最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"
                    .to_string(),
                why_ja: Some("日本語処理の効率化が期待される。[1]".to_string()),
                genre: Some("Technology".to_string()),
                member_feed_ids: vec![
                    Uuid::parse_str("44444444-5555-6666-7777-888888888888").unwrap(),
                ],
                sources: json!([
                    {
                        "feed_id": "44444444-5555-6666-7777-888888888888",
                        "host": "example.com",
                        "n": 1,
                        "pub_date": "2026-09-22T10:00:00Z",
                        "title": "新モデル発表のニュース",
                        "url": "https://example.com/ai-news"
                    }
                ]),
                centroid: Some(vec![0.1, 0.2]),
                scores: json!({}),
                gates: json!({}),
                generation: json!({}),
                created_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
            }]),
            Some("a completed cards job whose card has no genre and no why") => {
                Ok(vec![RecapCard {
                    id: Uuid::parse_str("22222222-3333-4444-5555-666666666666").unwrap(),
                    job_id,
                    rank: 1,
                    story_id: Uuid::parse_str("33333333-4444-5555-6666-777777777777").unwrap(),
                    continues_card_id: None,
                    merged_from: None,
                    headline_ja: "日本のAIスタートアップが新モデルを発表".to_string(),
                    what_ja: "最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]"
                        .to_string(),
                    why_ja: None,
                    genre: None,
                    member_feed_ids: vec![
                        Uuid::parse_str("44444444-5555-6666-7777-888888888888").unwrap(),
                    ],
                    sources: json!([
                        {
                            "feed_id": "44444444-5555-6666-7777-888888888888",
                            "host": "example.com",
                            "n": 1,
                            "pub_date": "2026-09-22T10:00:00Z",
                            "title": "新モデル発表のニュース",
                            "url": "https://example.com/ai-news"
                        }
                    ]),
                    centroid: Some(vec![0.1, 0.2]),
                    scores: json!({}),
                    gates: json!({}),
                    generation: json!({}),
                    created_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                        .unwrap()
                        .with_timezone(&Utc),
                }])
            }
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }

    async fn get_job_stats(&self, job_id: Uuid) -> Result<Option<RecapCardJobStats>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(None),
            Some(
                "a completed cards job with cards exists"
                | "a completed cards job whose card has no genre and no why"
                | "a failed topic cards run is newer than the latest completed run",
            ) => Ok(Some(RecapCardJobStats {
                job_id,
                items_fetched: 100,
                items_after_noise: 80,
                items_after_dedup: 60,
                clusters: 20,
                candidates: 10,
                cards_selected: 8,
                cards_dropped: json!({}),
                embed_ms: 10,
                cluster_ms: 10,
                llm_ms: 10,
                total_ms: 30,
                params_version: "cards-v0.2".to_string(),
                embed_cache_hits: 0,
                embed_cache_misses: 0,
                created_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
            })),
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }

    async fn get_latest_cards_run(&self) -> Result<Option<LatestCardsRun>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(None),
            Some(
                "a completed cards job with cards exists"
                | "a completed cards job whose card has no genre and no why",
            ) => Ok(Some(LatestCardsRun {
                job_id: Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap(),
                status: "completed".to_string(),
                kicked_at: DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
                updated_at: DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
            })),
            Some("a failed topic cards run is newer than the latest completed run") => {
                Ok(Some(LatestCardsRun {
                    job_id: Uuid::parse_str("99999999-8888-7777-6666-555555555555").unwrap(),
                    status: "failed".to_string(),
                    kicked_at: DateTime::parse_from_rfc3339("2026-09-23T17:00:00Z")
                        .unwrap()
                        .with_timezone(&Utc),
                    updated_at: DateTime::parse_from_rfc3339("2026-09-23T17:05:00Z")
                        .unwrap()
                        .with_timezone(&Utc),
                }))
            }
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }

    async fn get_job_updated_at(&self, _job_id: Uuid) -> Result<Option<DateTime<Utc>>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("no cards job exists") => Ok(None),
            Some(
                "a completed cards job with cards exists"
                | "a completed cards job whose card has no genre and no why"
                | "a failed topic cards run is newer than the latest completed run",
            ) => Ok(Some(
                DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc),
            )),
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationCardsDao: {unknown}")
            }
            None => panic!("missing provider state in VerificationCardsDao"),
        }
    }
}

struct VerificationPulseDao {
    provider_state: Arc<RwLock<Option<String>>>,
}

impl VerificationPulseDao {
    fn build_topic(role: TopicRole, cluster_id: i64) -> PulseTopic {
        PulseTopic {
            cluster_id,
            role,
            score: 0.85,
            rationale: "High impact news with multiple sources".to_string(),
            articles: vec![
                "44444444-5555-6666-7777-888888888888".to_string(),
                "55555555-6666-7777-8888-999999999999".to_string(),
                "66666666-7777-8888-9999-aaaaaaaaaaaa".to_string(),
            ],
            quality_metrics: ClusterQualityMetrics {
                cohesion: 0.8,
                ambiguity: 0.2,
                entity_consistency: 0.9,
                tier: QualityTier::Ok,
            },
            score_breakdown: ScoreBreakdown {
                impact_score: 0.5,
                burst_score: 0.15,
                novelty_score: 0.1,
                recency_score: 0.1,
            },
            representative_articles: vec![RepresentativeArticle {
                article_id: "44444444-5555-6666-7777-888888888888".to_string(),
                title: "Breaking news article".to_string(),
                source_url: "https://example.com/article/1".to_string(),
                source_name: "Example".to_string(),
                published_at: "2026-09-22T12:00:00Z".to_string(),
            }],
            top_entities: vec!["Entity1".to_string(), "Entity2".to_string()],
            source_names: vec!["Example".to_string()],
            genre: Some("Tech".to_string()),
        }
    }
}

#[async_trait::async_trait]
impl PulseReadDao for VerificationPulseDao {
    async fn get_latest_pulse(&self) -> Result<Option<PulseGenerationRow>, String> {
        let current = self.provider_state.read().unwrap().clone();
        match current.as_deref() {
            Some("a succeeded evening pulse generation with fallback exists") | None => {
                let job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
                let generated_at = DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                    .unwrap()
                    .with_timezone(&Utc);
                let result = PulseResult {
                    job_id,
                    version: PulseVersion::V4,
                    topics: vec![
                        Self::build_topic(TopicRole::NeedToKnow, 42),
                        Self::build_topic(TopicRole::Trend, 43),
                        Self::build_topic(TopicRole::Serendipity, 44),
                    ],
                    generated_at,
                    diagnostics: PulseDiagnostics {
                        syndication_removed: 2,
                        clusters_evaluated: 12,
                        fallback_level: 1,
                        duration_ms: 250,
                        ..Default::default()
                    },
                };
                Ok(Some(PulseGenerationRow {
                    job_id,
                    target_date: NaiveDate::from_ymd_opt(2026, 9, 22).unwrap(),
                    version: "v4".to_string(),
                    result_payload: Some(serde_json::to_value(&result).unwrap()),
                    finished_at: generated_at,
                }))
            }
            Some(unknown) => {
                panic!("unrecognised provider state in VerificationPulseDao: {unknown}")
            }
        }
    }

    async fn get_pulse_by_date(
        &self,
        _date: NaiveDate,
    ) -> Result<Option<PulseGenerationRow>, String> {
        self.get_latest_pulse().await
    }

    async fn get_latest_pulse_run(&self) -> Result<Option<SurfaceRun>, String> {
        Ok(Some(SurfaceRun {
            job_id: "11111111-2222-3333-4444-555555555555".to_string(),
            status: "completed".to_string(),
            kicked_at: "2026-09-22T17:00:00Z".to_string(),
            updated_at: "2026-09-22T17:05:00Z".to_string(),
        }))
    }

    async fn get_recap_job_window(&self, _job_id: Uuid) -> Result<Option<PulseWindow>, String> {
        let win_start = DateTime::parse_from_rfc3339("2026-09-19T17:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        let win_end = DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
            .unwrap()
            .with_timezone(&Utc);
        Ok(Some((win_start, win_end)))
    }
}

struct ThreeDayRecapGenreFixture {
    genre_name: String,
    summary_ja: String,
    top_terms: Vec<String>,
    article_id: String,
    title: String,
    source_url: String,
    published_at: DateTime<Utc>,
    lang: String,
    is_degraded: bool,
}

struct ThreeDayRecapFixtureDao {
    job_id: Uuid,
    started_at: DateTime<Utc>,
    window_start: DateTime<Utc>,
    window_end: DateTime<Utc>,
    total_articles: i32,
    genres: Vec<ThreeDayRecapGenreFixture>,
    latest_run: Option<SurfaceRun>,
}

#[async_trait::async_trait]
impl RecapReadDao for ThreeDayRecapFixtureDao {
    async fn get_latest_completed_job(
        &self,
        _window_days: i32,
    ) -> Result<Option<RecapJob>, String> {
        Ok(Some(RecapJob {
            job_id: self.job_id,
            started_at: self.started_at,
            window_start: self.window_start,
            window_end: self.window_end,
            total_articles: Some(self.total_articles),
        }))
    }

    async fn get_genres_by_job(&self, _job_id: Uuid) -> Result<Vec<GenreWithSummary>, String> {
        Ok(self
            .genres
            .iter()
            .map(|g| GenreWithSummary {
                genre_name: g.genre_name.clone(),
                summary_ja: Some(g.summary_ja.clone()),
                title_ja: None,
                bullets_ja: None,
            })
            .collect())
    }

    async fn get_clusters_by_job(
        &self,
        _job_id: Uuid,
    ) -> Result<std::collections::HashMap<String, Vec<ClusterWithEvidence>>, String> {
        let mut map = std::collections::HashMap::new();
        for (idx, g) in self.genres.iter().enumerate() {
            map.insert(
                g.genre_name.clone(),
                vec![ClusterWithEvidence {
                    cluster_id: i32::try_from(idx + 1).unwrap_or(1),
                    top_terms: Some(g.top_terms.clone()),
                    evidence: vec![ClusterEvidence {
                        article_id: g.article_id.clone(),
                        title: g.title.clone(),
                        source_url: g.source_url.clone(),
                        published_at: g.published_at,
                        lang: Some(g.lang.clone()),
                    }],
                }],
            );
        }
        Ok(map)
    }

    async fn get_recap_output_body_json(
        &self,
        _job_id: Uuid,
        genre: &str,
    ) -> Result<Option<Value>, String> {
        let is_degraded = self
            .genres
            .iter()
            .find(|g| g.genre_name == genre)
            .is_some_and(|g| g.is_degraded);
        Ok(Some(json!({
            "metadata": {
                "is_degraded": is_degraded
            }
        })))
    }

    async fn get_latest_three_day_recap_run(&self) -> Result<Option<SurfaceRun>, String> {
        Ok(self.latest_run.clone())
    }

    fn observe_cluster_query_duration(&self, _duration_secs: f64) {}

    fn inc_evidence_duplicates(&self, _removed: usize) {}

    fn observe_latest_fetch_duration(&self, _duration_secs: f64) {}
}

struct MorningLetterFixtureDao {
    id: Uuid,
    target_date: NaiveDate,
    edition_timezone: String,
    source_recap_job_id: Option<Uuid>,
    is_degraded: bool,
    schema_version: i32,
    generation_revision: i32,
    result_jsonb: Value,
    model: Option<String>,
    generation_metadata_jsonb: Value,
    created_at: DateTime<Utc>,
    latest_run: Option<SurfaceRun>,
    window: Option<(DateTime<Utc>, DateTime<Utc>)>,
}

impl MorningLetterFixtureDao {
    fn to_model(&self) -> MorningLetter {
        MorningLetter {
            id: self.id,
            target_date: self.target_date,
            edition_timezone: self.edition_timezone.clone(),
            source_recap_job_id: self.source_recap_job_id,
            is_degraded: self.is_degraded,
            schema_version: self.schema_version,
            generation_revision: self.generation_revision,
            result_jsonb: self.result_jsonb.clone(),
            model: self.model.clone(),
            generation_metadata_jsonb: self.generation_metadata_jsonb.clone(),
            created_at: self.created_at,
        }
    }
}

#[async_trait::async_trait]
impl MorningLetterReadDao for MorningLetterFixtureDao {
    async fn get_latest_morning_letter(&self) -> Result<Option<MorningLetter>, String> {
        Ok(Some(self.to_model()))
    }

    async fn get_morning_letter_by_id(&self, id: Uuid) -> Result<Option<MorningLetter>, String> {
        Ok((self.id == id).then(|| self.to_model()))
    }

    async fn get_morning_letter_by_date(
        &self,
        _date: NaiveDate,
    ) -> Result<Option<MorningLetter>, String> {
        Ok(Some(self.to_model()))
    }

    async fn get_latest_morning_run(&self) -> Result<Option<SurfaceRun>, String> {
        Ok(self.latest_run.clone())
    }

    async fn get_recap_job_window(
        &self,
        _job_id: Uuid,
    ) -> Result<Option<(DateTime<Utc>, DateTime<Utc>)>, String> {
        Ok(self.window)
    }
}

fn three_day_recap_fixture_for_state(state_name: Option<&str>) -> ThreeDayRecapFixtureDao {
    match state_name {
        Some("a completed 3-day recap job with one extractive genre exists") | None => {
            let job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
            let started_at = DateTime::parse_from_rfc3339("2026-09-22T17:00:00Z")
                .unwrap()
                .with_timezone(&Utc);
            let window_start = DateTime::parse_from_rfc3339("2026-09-19T17:00:00Z")
                .unwrap()
                .with_timezone(&Utc);
            let window_end = DateTime::parse_from_rfc3339("2026-09-22T17:05:00Z")
                .unwrap()
                .with_timezone(&Utc);
            let published_at = DateTime::parse_from_rfc3339("2026-09-22T10:00:00Z")
                .unwrap()
                .with_timezone(&Utc);
            ThreeDayRecapFixtureDao {
                job_id,
                started_at,
                window_start,
                window_end,
                total_articles: 12,
                genres: vec![
                    ThreeDayRecapGenreFixture {
                        genre_name: "ai".to_string(),
                        summary_ja: r#"{"title":"AI開発の動向","bullets":["新しい大規模言語モデルが公開された"]}"#
                            .to_string(),
                        top_terms: vec!["AI".to_string(), "LLM".to_string()],
                        article_id: "44444444-5555-6666-7777-888888888888".to_string(),
                        title: "AI article".to_string(),
                        source_url: "https://example.com/ai".to_string(),
                        published_at,
                        lang: "ja".to_string(),
                        is_degraded: false,
                    },
                    ThreeDayRecapGenreFixture {
                        genre_name: "space".to_string(),
                        summary_ja: r#"{"title":"宇宙開発","bullets":["新型ロケットの打ち上げ試験が実施された"]}"#
                            .to_string(),
                        top_terms: vec!["rocket".to_string(), "space".to_string()],
                        article_id: "55555555-6666-7777-8888-999999999999".to_string(),
                        title: "Space article".to_string(),
                        source_url: "https://example.com/space".to_string(),
                        published_at,
                        lang: "en".to_string(),
                        is_degraded: true,
                    },
                ],
                latest_run: Some(SurfaceRun {
                    job_id: job_id.to_string(),
                    status: "completed".to_string(),
                    kicked_at: "2026-09-22T17:00:00Z".to_string(),
                    updated_at: "2026-09-22T17:05:00Z".to_string(),
                }),
            }
        }
        Some(unknown) => {
            panic!("unrecognised provider state in three_day_recap_fixture_for_state: {unknown}")
        }
    }
}

fn morning_letter_fixture_for_state(
    state_name: Option<&str>,
    target_date_override: Option<NaiveDate>,
) -> MorningLetterFixtureDao {
    let is_degraded = matches!(state_name, Some("a degraded morning letter version exists"));
    let generation_metadata_jsonb = if is_degraded {
        json!({ "degradation_reason": "placeholder_output" })
    } else {
        json!({})
    };
    let recap_job_id = Uuid::parse_str("11111111-2222-3333-4444-555555555555").unwrap();
    let created_at = DateTime::parse_from_rfc3339("2026-09-22T06:00:00Z")
        .unwrap()
        .with_timezone(&Utc);
    let win_start = DateTime::parse_from_rfc3339("2026-09-19T06:00:00Z")
        .unwrap()
        .with_timezone(&Utc);
    let win_end = DateTime::parse_from_rfc3339("2026-09-22T06:00:00Z")
        .unwrap()
        .with_timezone(&Utc);
    let target_date =
        target_date_override.unwrap_or_else(|| NaiveDate::from_ymd_opt(2026, 4, 15).unwrap());

    MorningLetterFixtureDao {
        id: Uuid::parse_str("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee").unwrap(),
        target_date,
        edition_timezone: "Asia/Tokyo".to_string(),
        source_recap_job_id: Some(recap_job_id),
        is_degraded,
        schema_version: 1,
        generation_revision: 1,
        result_jsonb: json!({
            "lead": "Today's key developments...",
            "sections": [
                {
                    "key": "top3",
                    "title": "Top Stories",
                    "bullets": ["Story A"],
                    "genre": "ai",
                    "narrative": "Summary of top developments."
                }
            ],
            "generated_at": "2026-09-22T06:00:00Z",
            "source_recap_window_days": 3,
            "through_line": "Daily briefing"
        }),
        model: Some("gemma4-e4b-12k".to_string()),
        generation_metadata_jsonb,
        created_at,
        latest_run: Some(SurfaceRun {
            job_id: recap_job_id.to_string(),
            status: "completed".to_string(),
            kicked_at: "2026-09-22T05:55:00Z".to_string(),
            updated_at: "2026-09-22T06:00:00Z".to_string(),
        }),
        window: Some((win_start, win_end)),
    }
}

/// Build a stub Router that matches the endpoints asserted by
/// recap-worker's consumer pacts. Each handler returns a canned response
/// that satisfies the pact's expected body shape.
fn stub_router() -> Router {
    stub_router_with_state(StubState::default())
}

#[allow(clippy::too_many_lines)]
fn stub_router_with_state(state: StubState) -> Router {
    Router::new()
        .route(
            "/_pact/provider-state",
            post({
                let state = state.clone();
                move |axum::Json(payload): axum::Json<Value>| {
                    let state = state.clone();
                    async move {
                        let name = payload
                            .get("state")
                            .and_then(|v| v.as_str())
                            .map(ToString::to_string);
                        *state.provider_state.write().unwrap() = name;
                        axum::http::StatusCode::OK
                    }
                }
            }),
        )
        // search-indexer-recap-worker.json
        .route(
            "/v1/recaps/genres/indexable",
            get(|| async {
                Json(json!({
                    "genres": [
                        {"genre": "technology", "last_indexed_at": "2026-04-10T00:00:00Z"}
                    ]
                }))
            }),
        )
        // recap-evaluator-recap-worker.json — literal-segment routes first
        // so /latest is matched as a static path rather than parsed as the
        // `{run_id}` integer path parameter (an i64 parse of "latest" returns
        // 400 and trips the pact assertion).
        .route(
            "/v1/evaluation/genres",
            post(|| async {
                Json(json!({
                    "run_id": 1,
                    "status": "running",
                }))
            }),
        )
        .route(
            "/v1/evaluation/genres/latest",
            get(|| async {
                Json(json!({
                    "run_id": 42,
                    "status": "succeeded",
                    "accuracy": 0.85,
                    "macro_f1": 0.82,
                }))
            }),
        )
        .route(
            "/v1/evaluation/genres/{run_id}",
            get(|Path(run_id): Path<i64>| async move {
                Json(json!({
                    "run_id": run_id,
                    "status": "succeeded",
                    "accuracy": 0.85,
                    "macro_f1": 0.82,
                }))
            }),
        )
        // rag-orchestrator-recap-worker.json & alt-backend-recap-worker.json
        .route(
            "/v1/morning/letters/latest",
            get({
                let state = state.clone();
                move || {
                    let state = state.clone();
                    async move {
                        let current = state.provider_state.read().unwrap().clone();
                        let fixture = morning_letter_fixture_for_state(current.as_deref(), None);
                        let now = DateTime::parse_from_rfc3339("2026-09-22T07:00:00Z")
                            .unwrap()
                            .with_timezone(&Utc);
                        get_latest_morning_letter_impl(&fixture, chrono::Duration::hours(3), now)
                            .await
                    }
                }
            }),
        )
        .route(
            "/v1/morning/letters/{date}",
            get({
                let state = state.clone();
                move |Path(date): Path<String>| {
                    let state = state.clone();
                    async move {
                        let current = state.provider_state.read().unwrap().clone();
                        let parsed_date = NaiveDate::parse_from_str(&date, "%Y-%m-%d").ok();
                        let fixture =
                            morning_letter_fixture_for_state(current.as_deref(), parsed_date);
                        let now = DateTime::parse_from_rfc3339("2026-09-22T07:00:00Z")
                            .unwrap()
                            .with_timezone(&Utc);
                        get_morning_letter_by_date_impl(
                            &fixture,
                            &date,
                            chrono::Duration::hours(3),
                            now,
                        )
                        .await
                    }
                }
            }),
        )
        // alt-backend-recap-worker.json
        .route(
            "/v1/recaps/3days",
            get({
                let state = state.clone();
                move || {
                    let state = state.clone();
                    async move {
                        let current = state.provider_state.read().unwrap().clone();
                        let fixture = three_day_recap_fixture_for_state(current.as_deref());
                        let now = DateTime::parse_from_rfc3339("2026-09-22T18:00:00Z")
                            .unwrap()
                            .with_timezone(&Utc);
                        get_3days_recap_impl(&fixture, chrono::Duration::hours(30), now).await
                    }
                }
            }),
        )
        .route(
            "/v1/pulse/latest",
            get({
                let state = state.clone();
                move || {
                    let state = state.clone();
                    async move {
                        let dao = VerificationPulseDao {
                            provider_state: state.provider_state.clone(),
                        };
                        let now = DateTime::parse_from_rfc3339("2026-09-22T18:00:00Z")
                            .unwrap()
                            .with_timezone(&Utc);
                        get_latest_pulse_impl(&dao, None, chrono::Duration::hours(30), now).await
                    }
                }
            }),
        )
        .route(
            "/v1/topic-cards",
            get({
                let state = state.clone();
                move || {
                    let state = state.clone();
                    async move {
                        let dao = VerificationCardsDao {
                            provider_state: state.provider_state.clone(),
                        };
                        let now = DateTime::parse_from_rfc3339("2026-09-22T18:00:00Z")
                            .unwrap()
                            .with_timezone(&Utc);
                        get_topic_cards_with_clock(&dao, chrono::Duration::hours(30), now).await
                    }
                }
            }),
        )
}

async fn start_stub_server() -> SocketAddr {
    let router = stub_router();
    let listener = tokio::net::TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    tokio::spawn(async move {
        axum::serve(listener, router).await.unwrap();
    });
    // Give the listener a moment to accept connections.
    tokio::time::sleep(std::time::Duration::from_millis(50)).await;
    addr
}

fn load_pact(path: &str) -> Value {
    let raw =
        std::fs::read_to_string(path).unwrap_or_else(|e| panic!("failed to read pact {path}: {e}"));
    serde_json::from_str(&raw).unwrap_or_else(|e| panic!("pact {path} is not JSON: {e}"))
}

async fn set_provider_state(addr: SocketAddr, state_name: &str) {
    let url = format!("http://{addr}/_pact/provider-state");
    let client = reqwest::Client::new();
    client
        .post(&url)
        .json(&json!({ "state": state_name }))
        .send()
        .await
        .unwrap_or_else(|e| panic!("failed to set provider state {state_name}: {e}"));
}

fn assert_shape_matches(actual: &Value, expected: &Value, path: &str) {
    if expected.is_null() {
        assert!(actual.is_null(), "{path}: expected null, got {actual}");
        return;
    }

    assert!(
        !actual.is_null(),
        "{path}: expected non-null ({expected}), got null"
    );

    if let Some(expected_obj) = expected.as_object() {
        let actual_obj = actual
            .as_object()
            .unwrap_or_else(|| panic!("{path}: expected object, got {actual}"));
        for (k, expected_val) in expected_obj {
            let actual_val = actual_obj
                .get(k)
                .unwrap_or_else(|| panic!("{path}: missing required key '{k}'"));
            assert_shape_matches(actual_val, expected_val, &format!("{path}.{k}"));
        }
    } else if let Some(expected_arr) = expected.as_array() {
        let actual_arr = actual
            .as_array()
            .unwrap_or_else(|| panic!("{path}: expected array, got {actual}"));
        if expected_arr.is_empty() {
            assert!(
                actual_arr.is_empty(),
                "{path}: expected empty array, got len {}",
                actual_arr.len()
            );
        } else {
            assert!(
                !actual_arr.is_empty(),
                "{path}: expected non-empty array, got empty"
            );
            assert_shape_matches(&actual_arr[0], &expected_arr[0], &format!("{path}[0]"));
        }
    } else if expected.is_boolean() {
        assert!(
            actual.is_boolean(),
            "{path}: expected boolean, got {actual}"
        );
    } else if expected.is_number() {
        assert!(actual.is_number(), "{path}: expected number, got {actual}");
    } else if expected.is_string() {
        assert!(actual.is_string(), "{path}: expected string, got {actual}");
    } else {
        panic!("{path}: unexpected JSON value type in expected: {expected}");
    }
}

async fn verify_interaction(
    addr: SocketAddr,
    method: &str,
    path: &str,
    expected_status: u16,
    expected_body: Option<&Value>,
) -> Value {
    let url = format!("http://{addr}{path}");
    let client = reqwest::Client::new();
    let req = match method {
        "GET" => client.get(&url),
        "POST" => client.post(&url),
        _ => panic!("unsupported method {method}"),
    };
    let resp = req
        .send()
        .await
        .unwrap_or_else(|e| panic!("request to {url} failed: {e}"));
    assert_eq!(
        resp.status().as_u16(),
        expected_status,
        "unexpected status for {method} {path}"
    );
    // Body is JSON — ensure it parses successfully (structural validity).
    let body: Value = resp
        .json()
        .await
        .unwrap_or_else(|e| panic!("response body for {path} not JSON: {e}"));
    assert!(
        body.is_object(),
        "response for {path} must be a JSON object"
    );
    if let Some(expected) = expected_body {
        assert_shape_matches(&body, expected, "$");
    }
    body
}

#[cfg(test)]
mod evidence_tests {
    use super::*;

    #[test]
    fn evidence_document_binds_the_run_and_counts_the_interactions() {
        let doc = evidence_document(
            &Evidence {
                provider: "recap-worker",
                consumer: "search-indexer",
                implementation: "rust-stub-replay",
                interactions_verified: 2,
                interactions_skipped: 1,
            },
            "run-xyz",
            "deadbee",
        );

        assert_eq!(doc["schema"], EVIDENCE_SCHEMA);
        assert_eq!(doc["run_id"], "run-xyz");
        assert_eq!(doc["provider_version"], "deadbee");
        assert_eq!(doc["provider"], "recap-worker");
        assert_eq!(doc["consumer"], "search-indexer");
        assert_eq!(doc["implementation"], "rust-stub-replay");
        assert_eq!(doc["interactions_verified"], 2);
        assert_eq!(doc["interactions_skipped"], 1);
        assert_eq!(doc["result"], "passed");
        assert!(
            doc["verified_at"]
                .as_str()
                .is_some_and(|s| s.ends_with('Z')),
            "verified_at must be an RFC3339 UTC instant"
        );
    }

    #[test]
    fn write_evidence_to_names_the_file_after_the_bridge_row() {
        let dir = tempfile::tempdir().unwrap();
        let doc = evidence_document(
            &Evidence {
                provider: "recap-worker",
                consumer: "recap-evaluator",
                implementation: "rust-stub-replay",
                interactions_verified: 3,
                interactions_skipped: 0,
            },
            "run-1",
            "cafe123",
        );

        let path = write_evidence_to(dir.path(), &doc);

        assert_eq!(
            path.file_name().unwrap(),
            "recap-worker__recap-evaluator.json"
        );
        let back: Value = serde_json::from_str(&std::fs::read_to_string(&path).unwrap()).unwrap();
        assert_eq!(back, doc);
    }

    #[test]
    fn assert_shape_matches_identical_shape_passes() {
        let expected = json!({
            "cards": [
                {
                    "id": "abc",
                    "rank": 1,
                    "continues_card_id": null
                }
            ],
            "job": {
                "cards_selected": 8,
                "degraded": false
            }
        });
        let actual = json!({
            "cards": [
                {
                    "id": "xyz",
                    "rank": 2,
                    "continues_card_id": null
                }
            ],
            "job": {
                "cards_selected": 10,
                "degraded": true
            }
        });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    #[should_panic(expected = "missing required key 'b'")]
    fn assert_shape_matches_missing_key_panics() {
        let expected = json!({ "a": 1, "b": "str" });
        let actual = json!({ "a": 2 });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    fn assert_shape_matches_extra_key_allowed() {
        let expected = json!({ "a": 1 });
        let actual = json!({ "a": 1, "extra": true });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[tokio::test]
    #[should_panic(expected = "unrecognised provider state in VerificationCardsDao: bogus state")]
    async fn verification_cards_dao_panics_on_unknown_state() {
        let dao = VerificationCardsDao {
            provider_state: Arc::new(RwLock::new(Some("bogus state".to_string()))),
        };
        let _ = dao.get_latest_completed_cards_job().await;
    }

    #[test]
    #[should_panic(expected = "expected null")]
    fn assert_shape_matches_expected_null_panics() {
        let expected = json!({ "job": null });
        let actual = json!({ "job": { "cards_selected": 1 } });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    #[should_panic(expected = "expected non-null")]
    fn assert_shape_matches_unexpected_null_panics() {
        let expected = json!({ "job": { "cards_selected": 1 } });
        let actual = json!({ "job": null });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[test]
    #[should_panic(expected = "expected number")]
    fn assert_shape_matches_type_mismatch_panics() {
        let expected = json!({ "count": 10 });
        let actual = json!({ "count": "10" });
        assert_shape_matches(&actual, &expected, "$");
    }

    #[tokio::test]
    async fn provider_states_serve_surface_status_and_c6_degradation_reasons() {
        let addr = start_stub_server().await;

        set_provider_state(
            addr,
            "a completed 3-day recap job with one extractive genre exists",
        )
        .await;
        let recap_body = verify_interaction(addr, "GET", "/v1/recaps/3days", 200, None).await;
        assert_eq!(
            recap_body["surface_status"]["degradation_reasons"][0]["code"],
            "extractive_genres"
        );

        set_provider_state(
            addr,
            "a succeeded evening pulse generation with fallback exists",
        )
        .await;
        let pulse_body = verify_interaction(addr, "GET", "/v1/pulse/latest", 200, None).await;
        assert_eq!(
            pulse_body["surface_status"]["degradation_reasons"][0]["code"],
            "pulse_fallback"
        );

        set_provider_state(addr, "a degraded morning letter version exists").await;
        let morning_body =
            verify_interaction(addr, "GET", "/v1/morning/letters/latest", 200, None).await;
        assert_eq!(
            morning_body["surface_status"]["degradation_reasons"][0]["code"],
            "placeholder_output"
        );

        set_provider_state(
            addr,
            "a failed topic cards run is newer than the latest completed run",
        )
        .await;
        let cards_body = verify_interaction(addr, "GET", "/v1/topic-cards", 200, None).await;
        assert!(
            !cards_body.as_object().unwrap().contains_key("latest_run"),
            "top-level latest_run must not be present on /v1/topic-cards"
        );
        assert_eq!(
            cards_body["surface_status"]["latest_run"]["status"],
            "failed"
        );
    }
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_search_indexer_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../pacts/search-indexer-recap-worker.json");
    let mut verified = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let req = &interaction["request"];
        let resp = &interaction["response"];
        verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            resp["status"].as_u64().unwrap() as u16,
            None,
        )
        .await;
        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "search-indexer",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: 0,
    });
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_recap_evaluator_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../pacts/recap-evaluator-recap-worker.json");
    let mut verified = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let req = &interaction["request"];
        let resp = &interaction["response"];
        verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            resp["status"].as_u64().unwrap() as u16,
            None,
        )
        .await;
        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "recap-evaluator",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: 0,
    });
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
async fn verify_rag_orchestrator_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../rag-orchestrator/pacts/rag-orchestrator-recap-worker.json");
    let mut verified = 0usize;
    let mut skipped = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let req = &interaction["request"];
        let resp = &interaction["response"];
        let status = resp["status"].as_u64().unwrap() as u16;
        // Skip 404 interactions — stub always returns 200; a real verifier
        // would switch handlers by providerStates, which is out of scope
        // for this lightweight replay. The count is carried into the
        // evidence so the broker record does not overstate what was checked.
        if status == 404 {
            skipped += 1;
            continue;
        }
        verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            status,
            None,
        )
        .await;
        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "rag-orchestrator",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: skipped,
    });
}

#[tokio::test]
#[ignore = "provider verification: run with --ignored"]
#[allow(clippy::too_many_lines)]
async fn verify_alt_backend_pact() {
    let addr = start_stub_server().await;
    let pact = load_pact("../../pacts/alt-backend-recap-worker.json");
    let mut verified = 0usize;
    for interaction in pact["interactions"].as_array().unwrap() {
        let mut state_name = "";
        if let Some(states) = interaction.get("providerStates").and_then(|s| s.as_array()) {
            for state in states {
                if let Some(name) = state.get("name").and_then(|n| n.as_str()) {
                    set_provider_state(addr, name).await;
                    state_name = name;
                }
            }
        }
        let req = &interaction["request"];
        let resp = &interaction["response"];
        let body = verify_interaction(
            addr,
            req["method"].as_str().unwrap(),
            req["path"].as_str().unwrap(),
            resp["status"].as_u64().unwrap() as u16,
            resp.get("body"),
        )
        .await;

        match state_name {
            "a completed cards job with cards exists" => {
                assert!(
                    body["job"].is_object(),
                    "expected job object for state: {state_name}"
                );
                assert!(
                    body["cards"].as_array().is_some_and(|c| !c.is_empty()),
                    "expected non-empty cards for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["genre"].is_string(),
                    "expected genre string for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["why_ja"].is_string(),
                    "expected why_ja string for state: {state_name}"
                );
                assert!(
                    !body.as_object().unwrap().contains_key("latest_run"),
                    "top-level latest_run must not exist for state: {state_name}"
                );
                assert!(
                    body["surface_status"]["latest_run"].is_object(),
                    "expected surface_status.latest_run object for state: {state_name}"
                );
            }
            "a completed cards job whose card has no genre and no why" => {
                assert!(
                    body["job"].is_object(),
                    "expected job object for state: {state_name}"
                );
                assert!(
                    body["cards"].as_array().is_some_and(|c| !c.is_empty()),
                    "expected non-empty cards for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["genre"].is_null(),
                    "expected null genre for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["why_ja"].is_null(),
                    "expected null why_ja for state: {state_name}"
                );
                assert!(
                    body["cards"][0]["continues_card_id"].is_null(),
                    "expected null continues_card_id for state: {state_name}"
                );
                assert!(
                    !body.as_object().unwrap().contains_key("latest_run"),
                    "top-level latest_run must not exist for state: {state_name}"
                );
                assert!(
                    body["surface_status"]["latest_run"].is_object(),
                    "expected surface_status.latest_run object for state: {state_name}"
                );
            }
            "no cards job exists" => {
                assert!(
                    body["job"].is_null(),
                    "expected null job for state: {state_name}"
                );
                assert_eq!(
                    body["cards"],
                    json!([]),
                    "expected empty cards for state: {state_name}"
                );
                assert!(
                    !body.as_object().unwrap().contains_key("latest_run"),
                    "top-level latest_run must not exist for state: {state_name}"
                );
                assert!(
                    body["surface_status"].is_null(),
                    "expected null surface_status for state: {state_name}"
                );
            }
            "a failed topic cards run is newer than the latest completed run" => {
                assert!(
                    body["job"].is_object(),
                    "expected job object for state: {state_name}"
                );
                assert!(
                    body["cards"].as_array().is_some_and(|c| !c.is_empty()),
                    "expected non-empty cards for state: {state_name}"
                );
                assert!(
                    !body.as_object().unwrap().contains_key("latest_run"),
                    "top-level latest_run must not exist for state: {state_name}"
                );
                assert!(
                    body["surface_status"]["latest_run"].is_object(),
                    "expected surface_status.latest_run object for state: {state_name}"
                );
                assert_eq!(
                    body["surface_status"]["latest_run"]["status"], "failed",
                    "expected surface_status.latest_run status to be failed for state: {state_name}"
                );
            }
            "a completed 3-day recap job with one extractive genre exists" => {
                assert!(
                    body["surface_status"].is_object(),
                    "expected surface_status object for state: {state_name}"
                );
                assert_eq!(
                    body["surface_status"]["degradation_reasons"][0]["code"], "extractive_genres",
                    "expected extractive_genres reason for state: {state_name}"
                );
            }
            "a succeeded evening pulse generation with fallback exists" => {
                assert!(
                    body["surface_status"].is_object(),
                    "expected surface_status object for state: {state_name}"
                );
                assert_eq!(
                    body["surface_status"]["degradation_reasons"][0]["code"], "pulse_fallback",
                    "expected pulse_fallback reason for state: {state_name}"
                );
            }
            "a degraded morning letter version exists" => {
                assert!(
                    body["surface_status"].is_object(),
                    "expected surface_status object for state: {state_name}"
                );
                assert_eq!(
                    body["surface_status"]["degradation_reasons"][0]["code"], "placeholder_output",
                    "expected placeholder_output reason for state: {state_name}"
                );
            }
            unknown => panic!("unrecognised provider state in verify_alt_backend_pact: {unknown}"),
        }

        verified += 1;
    }
    assert!(verified > 0, "pact contained no interactions to replay");
    record_evidence(&Evidence {
        provider: "recap-worker",
        consumer: "alt-backend",
        implementation: EVIDENCE_IMPLEMENTATION,
        interactions_verified: verified,
        interactions_skipped: 0,
    });
}
