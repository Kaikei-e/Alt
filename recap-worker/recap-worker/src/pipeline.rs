//! Pipeline module - orchestrates the recap generation pipeline.
//!
//! The pipeline consists of multiple stages that process articles
//! from fetching through summarization and persistence.

pub mod dedup;
pub(crate) mod dispatch;
pub(crate) mod embedding;
pub(crate) mod evidence;
pub(crate) mod executor;
pub(crate) mod fetch;
pub(crate) mod genre;
pub(crate) mod genre_canonical;
pub(crate) mod genre_keywords;
pub(crate) mod genre_refine;
pub(crate) mod genre_remote;
pub(crate) mod graph_override;
pub mod minhash;
pub mod morning;
mod orchestrator;
pub(crate) mod persist;
pub mod preprocess;
pub mod pulse;
mod pulse_integration;
pub(crate) mod select;
pub(crate) mod tag_signal;

// Re-export core orchestrator types
pub(crate) use orchestrator::PipelineOrchestrator;

#[cfg(test)]
mod tests {
    use std::sync::{Arc, Mutex};

    use async_trait::async_trait;
    use sqlx::postgres::PgPoolOptions;
    use uuid::Uuid;

    use super::*;
    use crate::clients::SubworkerClient;
    use crate::config::{Config, ENV_MUTEX};
    use crate::pipeline::evidence::EvidenceBundle;
    use crate::pipeline::{
        dedup::{DedupStage, DeduplicatedCorpus},
        dispatch::{DispatchResult, DispatchStage},
        fetch::{FetchStage, FetchedArticle, FetchedCorpus},
        genre::{FeatureProfile, GenreAssignment, GenreBundle, GenreCandidate, GenreStage},
        persist::{PersistResult, PersistStage},
        preprocess::{PreprocessStage, PreprocessedArticle, PreprocessedCorpus},
        select::{SelectStage, SelectedSummary},
    };
    use crate::scheduler::JobContext;

    fn setup_config() -> Arc<Config> {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        Arc::new(temp_env::with_vars(
            [
                ("RECAP_DB_DSN", Some("postgres://recap:recap@localhost:5999/recap_db")),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:8002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
            ],
            || Config::from_env().expect("config should load for tests"),
        ))
    }

    #[tokio::test]
    async fn orchestrator_runs_stages_in_order() {
        let order: Arc<Mutex<Vec<&'static str>>> = Arc::new(Mutex::new(Vec::new()));
        let config = setup_config();

        // テスト用のモックRecapDaoを作成（DB接続なし）
        // このテストはステージの実行順序のみを検証するため、DB操作は不要
        let recap_dao: Arc<dyn crate::store::dao::RecapDao> =
            Arc::new(crate::store::dao::mock::MockRecapDao::new());

        // テスト用のダミーSubworkerClientを作成
        let subworker_client = Arc::new(
            SubworkerClient::new("http://localhost:8002/", 10)
                .expect("failed to create test subworker client"),
        );

        // テスト用のキューを作成（モックDAOを使用するため、実際のDB接続は不要）
        // ただし、ClassificationJobQueueはQueueStoreを要求するため、ダミーのプールが必要
        // このテストではキューは使用されないため、ダミーで問題ない
        let pool = PgPoolOptions::new()
            .max_connections(1)
            .min_connections(0)
            .connect_lazy("postgres://recap:recap@localhost:5999/recap_db")
            .expect("failed to create test pool");
        let queue_store = crate::queue::QueueStore::new(pool);
        let classification_queue = Arc::new(crate::queue::ClassificationJobQueue::new(
            queue_store,
            (*subworker_client).clone(),
            // このテストはステージの実行順序のみを検証するため、
            // バックグラウンド worker を起動しない（DB コネクション枯渇を避ける）。
            0,
            200,
            3,
            5000,
        ));

        let pipeline = PipelineOrchestrator::builder(Arc::clone(&config))
            .with_fetch_stage(Arc::new(RecordingFetch::new(Arc::clone(&order))))
            .with_preprocess_stage(Arc::new(RecordingPreprocess::new(Arc::clone(&order))))
            .with_dedup_stage(Arc::new(RecordingDedup::new(Arc::clone(&order))))
            .with_genre_stage(Arc::new(RecordingGenre::new(Arc::clone(&order))))
            .with_select_stage(Arc::new(RecordingSelect::new(Arc::clone(&order))))
            .with_dispatch_stage(Arc::new(RecordingDispatch::new(Arc::clone(&order))))
            .with_persist_stage(Arc::new(RecordingPersist::new(Arc::clone(&order))))
            .build(recap_dao, subworker_client, classification_queue, None);

        let job = JobContext::new(Uuid::new_v4(), vec!["ai".to_string()]);

        let result = pipeline
            .execute(&job)
            .await
            .expect("pipeline should succeed");

        assert!(result.genres_stored > 0);

        let stages = order.lock().expect("order lock").clone();
        assert_eq!(
            stages,
            vec![
                "fetch",
                "preprocess",
                "dedup",
                "genre",
                "select",
                "dispatch",
                "persist",
            ]
        );
    }

    struct RecordingFetch {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingFetch {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl FetchStage for RecordingFetch {
        async fn fetch(&self, job: &JobContext) -> anyhow::Result<FetchedCorpus> {
            self.order.lock().expect("order lock").push("fetch");
            Ok(FetchedCorpus {
                job_id: job.job_id,
                articles: vec![FetchedArticle {
                    id: Uuid::new_v4().to_string(),
                    title: Some("Title".to_string()),
                    body: "Body".to_string(),
                    language: Some("en".to_string()),
                    published_at: None,
                    source_url: None,
                    tags: Vec::new(),
                }],
            })
        }
    }

    struct RecordingPreprocess {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingPreprocess {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl PreprocessStage for RecordingPreprocess {
        async fn preprocess(
            &self,
            job: &JobContext,
            corpus: FetchedCorpus,
        ) -> anyhow::Result<PreprocessedCorpus> {
            assert_eq!(corpus.articles.len(), 1);
            self.order.lock().expect("order lock").push("preprocess");
            Ok(PreprocessedCorpus {
                job_id: job.job_id,
                articles: vec![PreprocessedArticle {
                    id: Uuid::new_v4().to_string(),
                    title: Some("Title".to_string()),
                    body: "Processed".to_string(),
                    language: "en".to_string(),
                    char_count: 9,
                    is_html_cleaned: true,
                    published_at: Some(chrono::Utc::now()),
                    source_url: None,
                    tokens: vec!["hello".to_string(), "world".to_string()],
                    tags: Vec::new(),
                }],
            })
        }
    }

    struct RecordingDedup {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingDedup {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl DedupStage for RecordingDedup {
        async fn deduplicate(
            &self,
            _job: &JobContext,
            corpus: PreprocessedCorpus,
        ) -> anyhow::Result<DeduplicatedCorpus> {
            assert_eq!(corpus.articles.len(), 1);
            self.order.lock().expect("order lock").push("dedup");
            Ok(DeduplicatedCorpus {
                job_id: corpus.job_id,
                articles: corpus
                    .articles
                    .into_iter()
                    .map(|article| super::dedup::DeduplicatedArticle {
                        id: article.id,
                        title: article.title,
                        sentences: vec![article.body],
                        sentence_hashes: vec![],
                        language: "en".to_string(),
                        published_at: article.published_at,
                        source_url: None,
                        tags: Vec::new(),
                        duplicates: Vec::new(),
                    })
                    .collect(),
                stats: super::dedup::DedupStats::default(),
            })
        }
    }

    struct RecordingGenre {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingGenre {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl GenreStage for RecordingGenre {
        async fn assign(
            &self,
            _job: &JobContext,
            corpus: DeduplicatedCorpus,
        ) -> anyhow::Result<GenreBundle> {
            assert_eq!(corpus.articles.len(), 1);
            self.order.lock().expect("order lock").push("genre");
            let article = corpus.articles.into_iter().next().expect("article");
            Ok(GenreBundle {
                job_id: corpus.job_id,
                assignments: vec![GenreAssignment {
                    genres: vec!["science".to_string()],
                    candidates: vec![GenreCandidate {
                        name: "science".to_string(),
                        score: 0.82,
                        keyword_support: 5,
                        classifier_confidence: 0.8,
                    }],
                    genre_scores: std::collections::HashMap::from([("science".to_string(), 10)]),
                    genre_confidence: std::collections::HashMap::new(),
                    feature_profile: FeatureProfile::default(),
                    article: article.clone(),
                    embedding: None,
                }],
                genre_distribution: std::collections::HashMap::from([("science".to_string(), 1)]),
            })
        }
    }

    struct RecordingSelect {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingSelect {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl SelectStage for RecordingSelect {
        async fn select(
            &self,
            _job: &JobContext,
            bundle: GenreBundle,
        ) -> anyhow::Result<SelectedSummary> {
            assert_eq!(bundle.assignments.len(), 1);
            self.order.lock().expect("order lock").push("select");
            Ok(SelectedSummary {
                job_id: bundle.job_id,
                assignments: bundle.assignments,
            })
        }
    }

    struct RecordingDispatch {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingDispatch {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl DispatchStage for RecordingDispatch {
        async fn dispatch(
            &self,
            job: &JobContext,
            evidence: EvidenceBundle,
        ) -> anyhow::Result<DispatchResult> {
            assert_eq!(evidence.genres().len(), 1);
            self.order.lock().expect("order lock").push("dispatch");
            Ok(DispatchResult {
                job_id: job.job_id,
                genre_results: std::collections::HashMap::new(),
                success_count: 1,
                failure_count: 0,
                all_genres: vec![],
            })
        }
    }

    struct RecordingPersist {
        order: Arc<Mutex<Vec<&'static str>>>,
    }

    impl RecordingPersist {
        fn new(order: Arc<Mutex<Vec<&'static str>>>) -> Self {
            Self { order }
        }
    }

    #[async_trait]
    impl PersistStage for RecordingPersist {
        async fn persist(
            &self,
            job: &JobContext,
            result: DispatchResult,
        ) -> anyhow::Result<PersistResult> {
            assert_eq!(result.job_id, job.job_id);
            self.order.lock().expect("order lock").push("persist");
            Ok(PersistResult {
                job_id: job.job_id,
                genres_stored: 1,
                genres_failed: 0,
                genres_skipped: 0,
                genres_no_evidence: 0,
                total_genres: result.all_genres.len(),
            })
        }
    }
}
