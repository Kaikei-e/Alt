//! Clustering operations for dispatch stage.

use std::collections::HashMap;
use std::sync::Arc;

use tokio::{sync::Semaphore, time::timeout};
use tracing::{debug, error, info, warn};
use uuid::Uuid;

use crate::clients::subworker::{ClusteringResponse, SubworkerClient};
use crate::config::Config;
use crate::error::{RecapError, Result};
use crate::pipeline::evidence::{EvidenceBundle, EvidenceCorpus};
use crate::scheduler::JobContext;
use crate::store::dao::RecapDao;

/// Clustering operations helper.
pub(crate) struct ClusteringOps<'a> {
    pub(crate) subworker_client: &'a Arc<SubworkerClient>,
    pub(crate) dao: &'a Arc<dyn RecapDao>,
    pub(crate) config: &'a Arc<Config>,
    pub(crate) concurrency_semaphore: &'a Arc<Semaphore>,
}

impl ClusteringOps<'_> {
    /// 単一ジャンルのクラスタリングのみを実行する。
    pub(crate) async fn cluster_genre(
        &self,
        job_id: Uuid,
        genre: &str,
        evidence: &EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        debug!(
            job_id = %job_id,
            genre = %genre,
            article_count = evidence.articles.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.genre = %genre,
            "clustering genre"
        );

        let mut clustering_response = self
            .run_subworker_clustering(job_id, genre, evidence)
            .await?;

        // Fallback: If clustering succeeded but returned NO clusters (e.g. all noise),
        // we force a fallback response using the evidence corpus.
        if clustering_response.clusters.is_empty() && !evidence.articles.is_empty() {
            clustering_response = Self::force_empty_clusters_fallback(job_id, genre, evidence);
        }

        // Handle fallback response (run_id == 0)
        // If the subworker client returns a fallback response (due to insufficient documents),
        // it will have run_id = 0, which doesn't exist in the database.
        // We need to insert a record for it so that we can persist clusters (foreign key constraint).
        if clustering_response.run_id == 0 {
            self.record_fallback_subworker_run(
                job_id,
                genre,
                evidence.articles.len(),
                &mut clustering_response,
            )
            .await?;
        }

        info!(
            job_id = %job_id,
            genre = %genre,
            cluster_count = clustering_response.clusters.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.genre = %genre,
            alt.processing.status = "completed",
            "clustering completed successfully"
        );

        self.save_clustering_metrics(job_id, genre, &clustering_response.diagnostics)
            .await;

        Ok(clustering_response)
    }

    async fn run_subworker_clustering(
        &self,
        job_id: Uuid,
        genre: &str,
        evidence: &EvidenceCorpus,
    ) -> Result<ClusteringResponse> {
        let genre_timeout = self.config.clustering_genre_timeout();
        let stuck_threshold = Some(self.config.clustering_stuck_threshold());
        let article_count = evidence.articles.len();
        timeout(
            genre_timeout,
            self.subworker_client.cluster_corpus_with_timeout(
                job_id,
                evidence,
                genre_timeout,
                stuck_threshold,
            ),
        )
        .await
        .map_err(|_| {
            RecapError::Clustering(format!(
                "clustering timeout: genre={genre} articles={article_count} timeout_s={}",
                genre_timeout.as_secs()
            ))
        })?
        .map_err(|e| {
            e.context(format!(
                "clustering failed: genre={genre} articles={article_count}"
            ))
        })
    }

    fn force_empty_clusters_fallback(
        job_id: Uuid,
        genre: &str,
        evidence: &EvidenceCorpus,
    ) -> ClusteringResponse {
        warn!(
            job_id = %job_id,
            genre = %genre,
            article_count = evidence.articles.len(),
            "clustering returned no clusters (noise), forcing fallback response"
        );
        SubworkerClient::create_fallback_response(job_id, evidence)
    }

    async fn record_fallback_subworker_run(
        &self,
        job_id: Uuid,
        genre: &str,
        article_count: usize,
        clustering_response: &mut ClusteringResponse,
    ) -> Result<()> {
        info!(
            job_id = %job_id,
            genre = %genre,
            "handling fallback clustering response (run_id=0), creating db record"
        );

        let run = crate::store::models::NewSubworkerRun::new(
            job_id,
            genre,
            serde_json::json!({
                "fallback": true,
                "reason": "insufficient_documents",
                "article_count": article_count
            }),
        )
        .with_status(crate::store::models::SubworkerRunStatus::Succeeded);

        let new_run_id = self
            .dao
            .insert_subworker_run(&run)
            .await
            .map_err(|e| e.context("failed to insert fallback subworker run"))?;

        // Update the response with the real DB ID
        clustering_response.run_id = new_run_id;

        // Also mark it as success with the cluster count
        self.dao
            .mark_subworker_run_success(
                new_run_id,
                clustering_response.clusters.len() as i32,
                &serde_json::json!({"fallback": true}),
            )
            .await
            .map_err(|e| e.context("failed to mark fallback run as success"))?;

        Ok(())
    }

    async fn save_clustering_metrics(
        &self,
        job_id: Uuid,
        genre: &str,
        diagnostics: &serde_json::Value,
    ) {
        if let Err(e) = self
            .dao
            .save_system_metrics(job_id, "clustering", diagnostics)
            .await
        {
            warn!(
                job_id = %job_id,
                genre = %genre,
                error = ?e,
                "failed to save clustering metrics"
            );
        }
    }

    /// Phase 1: 全ジャンルを並列でクラスタリング
    pub(crate) async fn cluster_all_genres(
        &self,
        job: &JobContext,
        genres: &[String],
        evidence: Arc<EvidenceBundle>,
    ) -> HashMap<String, Result<ClusteringResponse>> {
        let total_genres = genres.len();
        info!(
            job_id = %job.job_id,
            genre_count = total_genres,
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.progress.total = total_genres,
            "starting parallel clustering for all genres"
        );

        let tasks = self.spawn_all_clustering_tasks(job.job_id, genres, &evidence);
        let clustering_results = Self::collect_clustering_results(job.job_id, tasks).await;

        info!(
            job_id = %job.job_id,
            completed_count = clustering_results.len(),
            alt.processing.stage = "dispatch",
            alt.processing.phase = "clustering",
            alt.processing.progress.current = clustering_results.len(),
            alt.processing.progress.total = total_genres,
            alt.processing.status = "completed",
            "completed parallel clustering phase"
        );

        clustering_results
    }

    fn spawn_all_clustering_tasks(
        &self,
        job_id: Uuid,
        genres: &[String],
        evidence: &Arc<EvidenceBundle>,
    ) -> Vec<(String, tokio::task::JoinHandle<Result<ClusteringResponse>>)> {
        let mut tasks = Vec::new();
        for genre in genres {
            // Check existence without cloning
            if evidence.get_corpus(genre).is_some() {
                let task = self.spawn_genre_clustering_task(job_id, genre, evidence);
                tasks.push((genre.clone(), task));
            } else {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    "evidence corpus missing for genre"
                );
            }
        }
        tasks
    }

    fn spawn_genre_clustering_task(
        &self,
        job_id: Uuid,
        genre: &str,
        evidence: &Arc<EvidenceBundle>,
    ) -> tokio::task::JoinHandle<Result<ClusteringResponse>> {
        // Capture Arc instead of cloning corpus data
        let evidence_clone = evidence.clone();
        let subworker_client = Arc::clone(self.subworker_client);
        let dao = Arc::clone(self.dao);
        let config = Arc::clone(self.config);
        let genre_clone = genre.to_string();
        let semaphore = Arc::clone(self.concurrency_semaphore);

        tokio::spawn(async move {
            // Acquire permission to run (throttling)
            #[expect(clippy::expect_used, reason = "dispatch semaphore is never closed")]
            let _permit = semaphore
                .clone()
                .acquire_owned()
                .await
                .expect("dispatch semaphore is never closed");

            // Lazy access: Get corpus reference only AFTER acquiring semaphore
            // This ensures we don't load memory until allowed to run
            #[expect(
                clippy::expect_used,
                reason = "evidence is an immutable Arc<EvidenceBundle> verified via is_some before spawn"
            )]
            let corpus = evidence_clone.get_corpus(&genre_clone).expect(
                "evidence is an immutable Arc<EvidenceBundle> verified via is_some before spawn",
            );

            // Create ops for this task
            let ops = ClusteringOps {
                subworker_client: &subworker_client,
                dao: &dao,
                config: &config,
                concurrency_semaphore: &semaphore,
            };

            // `cluster_genre` already deadline-manages itself (it wraps the
            // subworker call in its own `timeout(genre_timeout, ..)`), so
            // wrapping it in a second identical timeout here added no
            // additional protection — just a redundant layer with the
            // deadline duplicated in two places.
            ops.cluster_genre(job_id, &genre_clone, corpus).await
        })
    }

    async fn collect_clustering_results(
        job_id: Uuid,
        tasks: Vec<(String, tokio::task::JoinHandle<Result<ClusteringResponse>>)>,
    ) -> HashMap<String, Result<ClusteringResponse>> {
        let mut clustering_results: HashMap<String, Result<ClusteringResponse>> = HashMap::new();

        for (genre, task) in tasks {
            match task.await {
                Ok(clustering_result) => {
                    clustering_results.insert(genre, clustering_result);
                }
                Err(join_error) => {
                    let err = Self::handle_clustering_join_error(job_id, &genre, join_error);
                    clustering_results.insert(genre, Err(err));
                }
            }
        }

        clustering_results
    }

    fn handle_clustering_join_error(
        job_id: Uuid,
        genre: &str,
        join_error: tokio::task::JoinError,
    ) -> RecapError {
        // A panicked/cancelled task must still surface as a failure
        // for this genre — leaving it out of `clustering_results`
        // makes dispatch.rs misclassify it as "no evidence (no
        // articles assigned)" instead of a real clustering failure.
        let message = match join_error.try_into_panic() {
            Ok(panic_payload) => {
                let panic_message = Self::extract_panic_message(panic_payload.as_ref());
                error!(
                    job_id = %job_id,
                    genre = %genre,
                    panic_message,
                    "clustering task panicked"
                );
                format!("clustering task panicked: {panic_message}")
            }
            Err(join_error) => {
                warn!(
                    job_id = %job_id,
                    genre = %genre,
                    error = ?join_error,
                    "clustering task failed"
                );
                format!("clustering task failed: {join_error}")
            }
        };
        RecapError::Clustering(message)
    }

    fn extract_panic_message(panic_payload: &(dyn std::any::Any + Send)) -> String {
        panic_payload
            .downcast_ref::<&str>()
            .map(|s| (*s).to_string())
            .or_else(|| {
                panic_payload
                    .downcast_ref::<String>()
                    .map(std::string::ToString::to_string)
            })
            .unwrap_or_else(|| "unknown panic payload".to_string())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::time::Duration;

    use serde_json::json;
    use wiremock::matchers::{method, path};
    use wiremock::{Mock, MockServer, ResponseTemplate};

    use crate::clients::subworker::ClusterJobStatus;
    use crate::config::ENV_MUTEX;
    use crate::pipeline::evidence::{CorpusMetadata, EvidenceArticle};
    use crate::store::dao::mock::MockRecapDao;

    fn test_config(subworker_url: &str, genre_timeout_secs: &str) -> Arc<Config> {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        temp_env::with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://recap:recap@localhost:5555/recap_db"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
                ("SUBWORKER_BASE_URL", Some(subworker_url)),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                ("RECAP_ADMIN_AUTH", Some("disabled")),
                ("MTLS_ALLOWED_PEERS", Some("disabled")),
                ("RECAP_EVAL_LISTENER", Some("disabled")),
                ("RECAP_CARDS_JOB", Some("disabled")),
                ("RECAP_THEMES_ENABLED", Some("false")),
                ("RECAP_PRE_REFRESH_GRAPH_ENABLED", Some("false")),
                ("RECAP_FRESHNESS_THREE_DAY_RECAP_HOURS", Some("30")),
                ("RECAP_FRESHNESS_TOPIC_CARDS_HOURS", Some("30")),
                ("RECAP_FRESHNESS_MORNING_LETTER_HOURS", Some("3")),
                (
                    "RECAP_CLUSTERING_GENRE_TIMEOUT_SECS",
                    Some(genre_timeout_secs),
                ),
                ("MTLS_ENFORCE", None),
            ],
            || Arc::new(Config::from_env().expect("config should load")),
        )
    }

    fn sample_corpus(genre: &str, article_count: usize) -> EvidenceCorpus {
        let articles = (0..article_count)
            .map(|i| EvidenceArticle {
                article_id: format!("{genre}-art-{i}"),
                title: Some(format!("Title {i}")),
                sentences: vec![format!(
                    "This is a sufficiently long test sentence for clustering article {i}."
                )],
                language: "en".to_string(),
                published_at: None,
                source_url: None,
                score: 0.9,
                genre_scores: None,
                confidence: None,
                signals: None,
            })
            .collect();

        EvidenceCorpus {
            genre: genre.to_string(),
            articles,
            total_sentences: article_count,
            metadata: CorpusMetadata {
                article_count,
                sentence_count: article_count,
                primary_language: "en".to_string(),
                language_distribution: HashMap::from([("en".to_string(), article_count)]),
                character_count: article_count * 64,
                classifier: None,
            },
        }
    }

    fn subworker_response_json(
        run_id: i64,
        job_id: Uuid,
        genre: &str,
        clusters: serde_json::Value,
    ) -> serde_json::Value {
        let cluster_count = clusters.as_array().map_or(0, Vec::len);
        json!({
            "run_id": run_id,
            "job_id": job_id,
            "genre": genre,
            "status": "succeeded",
            "cluster_count": cluster_count,
            "clusters": clusters,
            "diagnostics": { "source": "wiremock" }
        })
    }

    fn single_cluster_json(article_id: &str) -> serde_json::Value {
        json!([{
            "cluster_id": 0,
            "size": 1,
            "label": "ai",
            "top_terms": ["ai", "ml"],
            "stats": {},
            "representatives": [{
                "article_id": article_id,
                "paragraph_idx": 0,
                "sentence_text": "This representative sentence is longer than twenty characters.",
                "lang": "en",
                "score": 0.95
            }]
        }])
    }

    #[tokio::test]
    async fn cluster_genre_succeeds_without_fallback_when_run_id_is_positive() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let body = subworker_response_json(42, job_id, "ai", single_cluster_json("ai-art-0"));

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .expect(1)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 2, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };
        let corpus = sample_corpus("ai", 2);

        let response = ops
            .cluster_genre(job_id, "ai", &corpus)
            .await
            .expect("cluster_genre should succeed");

        assert_eq!(response.run_id, 42);
        assert_eq!(response.genre, "ai");
        assert_eq!(response.status, ClusterJobStatus::Succeeded);
        assert_eq!(response.clusters.len(), 1);
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_genre_forces_fallback_when_subworker_returns_empty_clusters() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let body = subworker_response_json(7, job_id, "ai", json!([]));

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .expect(1)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 2, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };
        let corpus = sample_corpus("ai", 2);

        let response = ops
            .cluster_genre(job_id, "ai", &corpus)
            .await
            .expect("empty cluster response should fall back cleanly");

        // MockRecapDao::insert_subworker_run returns Ok(0)
        assert_eq!(response.run_id, 0);
        assert_eq!(response.clusters.len(), 1);
        assert_eq!(response.clusters[0].cluster_id, 0);
        assert_eq!(response.clusters[0].size, 2);
        assert_eq!(response.clusters[0].label.as_deref(), Some("ai"));
        assert_eq!(response.diagnostics["fallback"], json!(true));
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_genre_uses_client_fallback_below_min_documents_without_http() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 5, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };
        let job_id = Uuid::new_v4();
        let corpus = sample_corpus("ai", 2);

        let response = ops
            .cluster_genre(job_id, "ai", &corpus)
            .await
            .expect("client fallback should succeed without HTTP");

        assert_eq!(response.run_id, 0);
        assert_eq!(response.clusters.len(), 1);
        assert_eq!(response.clusters[0].size, 2);
        assert_eq!(response.diagnostics["fallback"], json!(true));
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_genre_preserves_insufficient_documents_when_corpus_is_empty() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(500))
            .expect(0)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 5, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };
        let corpus = sample_corpus("ai", 0);

        let err = ops
            .cluster_genre(Uuid::new_v4(), "ai", &corpus)
            .await
            .expect_err("empty corpus must fail with InsufficientDocuments");

        assert_eq!(err, RecapError::InsufficientDocuments { min: 1, found: 0 });
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_genre_wraps_subworker_error_with_genre_and_article_count() {
        let server = MockServer::start().await;
        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(500).set_body_string("subworker exploded"))
            .expect(1)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 1, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };
        let corpus = sample_corpus("ai", 2);

        let err = ops
            .cluster_genre(Uuid::new_v4(), "ai", &corpus)
            .await
            .expect_err("500 response must fail");

        match err {
            RecapError::Clustering(msg) => {
                assert!(
                    msg.contains("clustering failed: genre=ai articles=2"),
                    "missing context prefix in error: {msg}"
                );
            }
            other => panic!("expected RecapError::Clustering, got {other:?}"),
        }
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_genre_returns_timeout_error_when_genre_deadline_elapses() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let body = subworker_response_json(10, job_id, "ai", single_cluster_json("ai-art-0"));

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(
                ResponseTemplate::new(200)
                    .set_body_json(body)
                    .set_delay(Duration::from_secs(2)),
            )
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 1, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "1");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };
        let corpus = sample_corpus("ai", 2);

        let err = ops
            .cluster_genre(job_id, "ai", &corpus)
            .await
            .expect_err("slow subworker must trigger genre timeout");

        match err {
            RecapError::Clustering(msg) => {
                assert!(
                    msg.contains("clustering timeout: genre=ai articles=2 timeout_s=1"),
                    "unexpected timeout error message: {msg}"
                );
            }
            other => panic!("expected RecapError::Clustering timeout, got {other:?}"),
        }
    }

    #[tokio::test]
    async fn cluster_all_genres_omits_missing_corpora_and_collects_success_and_error_outcomes() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let body = subworker_response_json(11, job_id, "ai", single_cluster_json("ai-art-0"));

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(ResponseTemplate::new(200).set_body_json(body))
            .expect(1)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 1, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(2));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };

        let evidence = Arc::new(EvidenceBundle {
            job_id,
            corpora: HashMap::from([
                ("ai".to_string(), sample_corpus("ai", 2)),
                ("tech".to_string(), sample_corpus("tech", 0)),
            ]),
        });
        let job = JobContext::new(job_id, vec![]);
        let genres = vec!["ai".to_string(), "tech".to_string(), "missing".to_string()];

        let results = ops.cluster_all_genres(&job, &genres, evidence).await;

        assert_eq!(results.len(), 2);
        assert!(!results.contains_key("missing"));
        let ai_res = results
            .get("ai")
            .expect("ai result present")
            .as_ref()
            .expect("ai clustering succeeded");
        assert_eq!(ai_res.run_id, 11);
        assert!(matches!(
            results.get("tech"),
            Some(Err(RecapError::InsufficientDocuments { min: 1, found: 0 }))
        ));
        server.verify().await;
    }

    #[tokio::test]
    async fn cluster_all_genres_respects_concurrency_semaphore() {
        let server = MockServer::start().await;
        let job_id = Uuid::new_v4();
        let in_flight = Arc::new(AtomicUsize::new(0));
        let max_in_flight = Arc::new(AtomicUsize::new(0));
        let in_flight_clone = Arc::clone(&in_flight);
        let max_clone = Arc::clone(&max_in_flight);

        Mock::given(method("POST"))
            .and(path("/v1/runs"))
            .respond_with(move |req: &wiremock::Request| {
                let current = in_flight_clone.fetch_add(1, Ordering::SeqCst) + 1;
                max_clone.fetch_max(current, Ordering::SeqCst);
                std::thread::sleep(Duration::from_millis(30));
                in_flight_clone.fetch_sub(1, Ordering::SeqCst);

                let genre = req
                    .headers
                    .get("x-alt-genre")
                    .and_then(|v| v.to_str().ok())
                    .unwrap_or("ai");
                ResponseTemplate::new(200).set_body_json(subworker_response_json(
                    21,
                    job_id,
                    genre,
                    single_cluster_json("art-0"),
                ))
            })
            .expect(2)
            .mount(&server)
            .await;

        let subworker_client =
            Arc::new(SubworkerClient::new(server.uri(), 1, None).expect("subworker client builds"));
        let dao: Arc<dyn RecapDao> = Arc::new(MockRecapDao::new());
        let config = test_config(&server.uri(), "30");
        let semaphore = Arc::new(Semaphore::new(1));
        let ops = ClusteringOps {
            subworker_client: &subworker_client,
            dao: &dao,
            config: &config,
            concurrency_semaphore: &semaphore,
        };

        let evidence = Arc::new(EvidenceBundle {
            job_id,
            corpora: HashMap::from([
                ("ai".to_string(), sample_corpus("ai", 2)),
                ("tech".to_string(), sample_corpus("tech", 2)),
            ]),
        });
        let job = JobContext::new(job_id, vec![]);
        let genres = vec!["ai".to_string(), "tech".to_string()];

        let results = ops.cluster_all_genres(&job, &genres, evidence).await;

        assert_eq!(results.len(), 2);
        assert!(results.get("ai").is_some_and(Result::is_ok));
        assert!(results.get("tech").is_some_and(Result::is_ok));
        assert_eq!(max_in_flight.load(Ordering::SeqCst), 1);
        server.verify().await;
    }
}
