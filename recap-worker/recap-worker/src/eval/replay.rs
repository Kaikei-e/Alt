//! Topic cards evaluation replay runner.

use anyhow::{Context, Result};
use chrono::{DateTime, Utc};
use sqlx::PgPool;
use std::sync::Arc;
use tracing::info;

use crate::clients::alt_backend::{AltBackendClient, AltBackendConfig};
use crate::clients::mtls::{MtlsPaths, build_mtls_client};
use crate::clients::news_creator::NewsCreatorClient;
use crate::clients::subworker::SubworkerClient;
use crate::clients::subworker::cards::SubworkerCardsClient;
use crate::config::Config;
use crate::pipeline::cards::adapters::{
    AltBackendFeedSource, NewsCreatorCardGenerator, SubworkerCardVerifier, SubworkerEmbedCluster,
    SubworkerGenreTagger,
};
use crate::pipeline::cards::{CardsParams, CardsPipeline, ReplayResult};
use crate::store::dao::impls::UnifiedDao;

pub type EvalReplayResult = ReplayResult;

fn build_alt_backend_feed_source(
    config: &Config,
    mtls_paths: Option<&MtlsPaths>,
) -> Result<Arc<AltBackendFeedSource>> {
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
    };

    let alt_backend_client = Arc::new(
        if let Some(paths) = mtls_paths {
            let client = build_mtls_client(
                paths,
                alt_backend_config.connect_timeout,
                alt_backend_config.total_timeout,
            )
            .context("failed to build alt-backend mTLS client")?;
            AltBackendClient::new_with_client(alt_backend_config, client)
        } else {
            AltBackendClient::new(alt_backend_config)
        }
        .context("failed to create alt-backend client")?,
    );

    Ok(Arc::new(AltBackendFeedSource::new(alt_backend_client)))
}

fn build_subworker_cards_client(
    config: &Config,
    mtls_paths: Option<&MtlsPaths>,
) -> Result<Arc<SubworkerCardsClient>> {
    let client = Arc::new(
        if let Some(paths) = mtls_paths {
            let mtls_client = build_mtls_client(
                paths,
                std::time::Duration::from_secs(5),
                std::time::Duration::from_mins(2),
            )
            .context("failed to build subworker mTLS client")?;
            SubworkerCardsClient::new_with_client(config.subworker_base_url(), mtls_client)
        } else {
            SubworkerCardsClient::new(config.subworker_base_url())
        }
        .context("failed to create subworker cards client")?
        .with_admin_token(config.admin_auth_token().map(str::to_string)),
    );
    Ok(client)
}

async fn build_news_creator_card_generator(
    config: &Config,
    mtls_paths: Option<&MtlsPaths>,
) -> Result<Arc<NewsCreatorCardGenerator>> {
    let news_creator_client = Arc::new(
        if let Some(paths) = mtls_paths {
            let client = build_mtls_client(
                paths,
                std::time::Duration::from_secs(5),
                config.llm_summary_timeout(),
            )
            .context("failed to build news-creator mTLS client")?;
            NewsCreatorClient::new_with_client(
                config.news_creator_base_url(),
                config.llm_summary_timeout(),
                client,
            )
            .await
        } else {
            NewsCreatorClient::new(config.news_creator_base_url(), config.llm_summary_timeout())
                .await
        }
        .context("failed to create news-creator client")?,
    );
    Ok(Arc::new(NewsCreatorCardGenerator::new(news_creator_client)))
}

fn build_subworker_genre_tagger(
    config: &Config,
    mtls_paths: Option<&MtlsPaths>,
) -> Result<Arc<SubworkerGenreTagger>> {
    let subworker_client = Arc::new(
        if let Some(paths) = mtls_paths {
            let client = build_mtls_client(
                paths,
                std::time::Duration::from_secs(5),
                std::time::Duration::from_mins(2),
            )
            .context("failed to build subworker general mTLS client")?;
            SubworkerClient::new_with_client(
                config.subworker_base_url(),
                config.min_documents_per_genre(),
                client,
            )
        } else {
            SubworkerClient::new(
                config.subworker_base_url(),
                config.min_documents_per_genre(),
            )
        }
        .context("failed to create subworker general client")?
        .with_admin_token(config.admin_auth_token().map(str::to_string)),
    );
    Ok(Arc::new(SubworkerGenreTagger::new(subworker_client)))
}

/// Construct `SubworkerEmbedCluster` configured with expected model and dimension from params.
pub fn build_cards_embed_cluster(
    config: &Config,
    params: &CardsParams,
) -> Result<Arc<SubworkerEmbedCluster>> {
    let mtls_paths = MtlsPaths::from_env().context("resolving mTLS env for outbound clients")?;
    let subworker_cards_client = build_subworker_cards_client(config, mtls_paths.as_ref())?;
    Ok(Arc::new(SubworkerEmbedCluster::with_expected(
        subworker_cards_client,
        &params.expected_embed_model,
        params.expected_embed_dim,
    )))
}

/// Construct CardsPipeline with production dependencies in selection_only mode (candidates only).
pub fn build_cards_pipeline(
    pool: &PgPool,
    config: &Config,
    params: &CardsParams,
) -> Result<CardsPipeline> {
    let mtls_paths = MtlsPaths::from_env().context("resolving mTLS env for outbound clients")?;
    let feed_source = build_alt_backend_feed_source(config, mtls_paths.as_ref())?;
    let subworker_cards_client = build_subworker_cards_client(config, mtls_paths.as_ref())?;

    let embed_cluster = Arc::new(SubworkerEmbedCluster::with_expected(
        subworker_cards_client,
        &params.expected_embed_model,
        params.expected_embed_dim,
    ));
    let dao = Arc::new(UnifiedDao::new(pool.clone()));
    let genre_tagger = build_subworker_genre_tagger(config, mtls_paths.as_ref())?;

    let mut pipeline = CardsPipeline::selection_only(feed_source, embed_cluster, dao)
        .with_genre_tagger(genre_tagger);
    if let Some(user_id) = config.cards_user_id() {
        pipeline = pipeline.with_user_id(user_id);
    }

    Ok(pipeline)
}

/// Construct CardsPipeline with production dependencies in full mode (generation + verification).
pub async fn build_production_cards_pipeline(
    pool: &PgPool,
    config: &Config,
    params: &CardsParams,
) -> Result<CardsPipeline> {
    let mtls_paths = MtlsPaths::from_env().context("resolving mTLS env for outbound clients")?;
    let feed_source = build_alt_backend_feed_source(config, mtls_paths.as_ref())?;
    let subworker_cards_client = build_subworker_cards_client(config, mtls_paths.as_ref())?;

    let embed_cluster = Arc::new(SubworkerEmbedCluster::with_expected(
        subworker_cards_client.clone(),
        &params.expected_embed_model,
        params.expected_embed_dim,
    ));
    let dao = Arc::new(UnifiedDao::new(pool.clone()));

    let card_generator = build_news_creator_card_generator(config, mtls_paths.as_ref()).await?;
    let card_verifier = Arc::new(SubworkerCardVerifier::new(subworker_cards_client));
    let genre_tagger = build_subworker_genre_tagger(config, mtls_paths.as_ref())?;

    let mut pipeline = CardsPipeline::full(
        feed_source,
        embed_cluster,
        dao,
        card_generator,
        card_verifier,
    )
    .with_genre_tagger(genre_tagger);

    if let Some(user_id) = config.cards_user_id() {
        pipeline = pipeline.with_user_id(user_id);
    }

    Ok(pipeline)
}

/// Run an evaluation replay for the given time window and hyperparameters.
pub async fn run_eval_replay(
    pool: &PgPool,
    config: &Config,
    from: DateTime<Utc>,
    to: DateTime<Utc>,
    params: &CardsParams,
) -> Result<ReplayResult> {
    let user_id = config.cards_user_id().context(
        "cards user id not configured (RECAP_CARDS_USER_ID is required for eval replay)",
    )?;

    info!(
        mode = "selection_only",
        params_version = %params.params_version,
        "starting cards pipeline eval replay"
    );

    let pipeline = build_cards_pipeline(pool, config, params)?.with_user_id(user_id);

    pipeline
        .run_replay(from, to, params)
        .await
        .context("cards pipeline replay failed")
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ENV_MUTEX;
    use uuid::Uuid;

    #[test]
    fn test_run_eval_replay_fails_without_user_id() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let _guard = rt.enter();
        let vars = vec![
            (
                "RECAP_DB_DSN",
                Some("postgres://user:pass@localhost:5432/db"),
            ),
            ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
            ("SUBWORKER_BASE_URL", Some("http://localhost:8002/")),
            ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
            ("RECAP_KNOWLEDGE_EMIT", Some("false")),
            ("RECAP_ADMIN_AUTH", Some("disabled")),
            ("RECAP_CARDS_JOB", Some("disabled")),
            ("RECAP_CARDS_USER_ID", None),
            ("RECAP_EVAL_LISTENER", Some("disabled")),
        ];
        temp_env::with_vars(vars, || {
            let config = Config::from_env().expect("config loads");
            assert_eq!(config.cards_user_id(), None);

            let res = rt.block_on(async {
                let pool =
                    sqlx::PgPool::connect_lazy("postgres://user:pass@localhost:5432/db").unwrap();
                let params = CardsParams::default();
                run_eval_replay(&pool, &config, Utc::now(), Utc::now(), &params).await
            });
            assert!(res.is_err());
            let err = res.unwrap_err().to_string();
            assert!(
                err.contains("cards user id not configured"),
                "expected error to mention cards user id not configured, got '{err}'"
            );
        });
    }

    #[test]
    fn test_run_eval_replay_config_with_user_id_passes_validation() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let expected_user_id = "33333333-3333-3333-3333-333333333333";
        let vars = vec![
            (
                "RECAP_DB_DSN",
                Some("postgres://user:pass@localhost:5432/db"),
            ),
            ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
            ("SUBWORKER_BASE_URL", Some("http://localhost:8002/")),
            ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
            ("RECAP_KNOWLEDGE_EMIT", Some("false")),
            ("RECAP_ADMIN_AUTH", Some("disabled")),
            ("RECAP_CARDS_JOB", Some("disabled")),
            ("RECAP_CARDS_USER_ID", Some(expected_user_id)),
            ("RECAP_EVAL_LISTENER", Some("disabled")),
            ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
        ];
        temp_env::with_vars(vars, || {
            let config = Config::from_env().expect("config loads");
            assert_eq!(
                config.cards_user_id(),
                Some(Uuid::parse_str(expected_user_id).unwrap())
            );

            let _guard = rt.enter();
            let pool =
                sqlx::PgPool::connect_lazy("postgres://user:pass@localhost:5432/db").unwrap();
            let params = CardsParams::default();
            let pipeline = build_cards_pipeline(&pool, &config, &params);
            assert!(
                pipeline.is_ok(),
                "pipeline builds successfully when user_id is configured"
            );
            let _pipeline = pipeline
                .unwrap()
                .with_user_id(config.cards_user_id().expect("cards user id configured"));

            let prod_pipeline =
                rt.block_on(build_production_cards_pipeline(&pool, &config, &params));
            assert!(
                prod_pipeline.is_ok(),
                "production pipeline builds successfully when user_id is configured"
            );
        });
    }

    #[test]
    fn test_build_cards_embed_cluster_override_model() {
        let _lock = ENV_MUTEX
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner);
        let rt = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let _guard = rt.enter();

        let (server, server_uri) = rt.block_on(async {
            let server = wiremock::MockServer::start().await;
            let expected_req = serde_json::json!({
                "texts": ["test text"],
                "normalize": true,
            });
            let res_body = serde_json::json!({
                "model": "bge-m3",
                "dim": 1024,
                "embeddings": [vec![0.1f32; 1024]]
            });

            wiremock::Mock::given(wiremock::matchers::method("POST"))
                .and(wiremock::matchers::path("/v1/embed"))
                .and(wiremock::matchers::body_json(&expected_req))
                .respond_with(wiremock::ResponseTemplate::new(200).set_body_json(&res_body))
                .mount(&server)
                .await;

            let uri = server.uri();
            (server, uri)
        });

        let vars = vec![
            (
                "RECAP_DB_DSN",
                Some("postgres://user:pass@localhost:5432/db"),
            ),
            ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
            ("SUBWORKER_BASE_URL", Some(server_uri.as_str())),
            ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
            ("RECAP_KNOWLEDGE_EMIT", Some("false")),
            ("RECAP_ADMIN_AUTH", Some("disabled")),
            ("RECAP_CARDS_JOB", Some("disabled")),
            (
                "RECAP_CARDS_USER_ID",
                Some("33333333-3333-3333-3333-333333333333"),
            ),
            ("RECAP_EVAL_LISTENER", Some("disabled")),
            ("TOKEN_COUNTER_ALLOW_DUMMY_FALLBACK", Some("true")),
        ];

        temp_env::with_vars(vars, || {
            let config = Config::from_env().expect("config loads");

            // 1. Default params expect "bge-m3", dim 1024 -> matches mock response
            let default_params = CardsParams::default();
            let default_adapter = build_cards_embed_cluster(&config, &default_params)
                .expect("embed cluster builder succeeds");
            use crate::pipeline::cards::EmbedCluster;
            let res = rt.block_on(default_adapter.embed(&["test text".to_string()]));
            assert!(
                res.is_ok(),
                "default adapter matches expected bge-m3 response: {res:?}"
            );

            // 2. Overridden params expect "custom-embed-model" -> adapter must fail with identity mismatch
            let overridden_params = default_params
                .with_override(
                    "expected_embed_model",
                    &serde_json::Value::String("custom-embed-model".to_string()),
                )
                .expect("override succeeds");
            assert_eq!(overridden_params.expected_embed_model, "custom-embed-model");

            let overridden_adapter = build_cards_embed_cluster(&config, &overridden_params)
                .expect("embed cluster builder succeeds with override");
            let res_err = rt.block_on(overridden_adapter.embed(&["test text".to_string()]));
            assert!(
                res_err.is_err(),
                "overridden adapter must fail when server returns bge-m3"
            );
            let err_msg = res_err.unwrap_err().to_string();
            assert!(
                err_msg.contains(
                    "subworker embedding identity mismatch: expected model 'custom-embed-model' with dim 1024, got 'bge-m3' with dim 1024"
                ),
                "expected mismatch error message, got: {err_msg}"
            );

            // 3. Pipeline builders also succeed with overridden params
            let pool =
                sqlx::PgPool::connect_lazy("postgres://user:pass@localhost:5432/db").unwrap();
            let pipeline = build_cards_pipeline(&pool, &config, &overridden_params);
            assert!(
                pipeline.is_ok(),
                "build_cards_pipeline succeeds with overridden params"
            );

            let prod_pipeline = rt.block_on(build_production_cards_pipeline(
                &pool,
                &config,
                &overridden_params,
            ));
            assert!(
                prod_pipeline.is_ok(),
                "build_production_cards_pipeline succeeds with overridden params"
            );
        });
        drop(server);
    }
}
