use std::sync::Arc;

use anyhow::{Context, Result, anyhow};
use axum::Router;
use sqlx::postgres::PgPoolOptions;
use tokio::task::JoinHandle;
use tokio_util::sync::CancellationToken;

use crate::{
    api,
    clients::{NewsCreatorClient, SubworkerClient, datahub::DataHubClient},
    config::Config,
    notification::{NotificationRelay, RelayConfig},
    observability::Telemetry,
    pipeline::PipelineOrchestrator,
    queue::{ClassificationJobQueue, QueueStore},
    scheduler::Scheduler,
    store::dao::{RecapDao, RecapDaoImpl},
};

#[derive(Clone)]
pub(crate) struct AppState {
    registry: Arc<ComponentRegistry>,
}

pub struct ComponentRegistry {
    config: Arc<Config>,
    telemetry: Telemetry,
    scheduler: Scheduler,
    news_creator_client: Arc<NewsCreatorClient>,
    subworker_client: Arc<SubworkerClient>,
    recap_dao: Arc<dyn RecapDao>,
    recap_pool: sqlx::PgPool,
    /// `None` only when `RECAP_NOTIFICATION_RELAY_ENABLED` explicitly turned
    /// it off. A construction failure fails startup instead of landing here,
    /// so "nobody wired the relay" can never masquerade as "it is off on
    /// purpose" (CLAUDE.md rule 8).
    notification_relay: Option<Arc<NotificationRelay>>,
}

impl AppState {
    pub(crate) fn new(registry: ComponentRegistry) -> Self {
        Self {
            registry: Arc::new(registry),
        }
    }

    pub(crate) fn telemetry(&self) -> &Telemetry {
        &self.registry.telemetry
    }

    pub(crate) fn scheduler(&self) -> &Scheduler {
        &self.registry.scheduler
    }

    pub(crate) fn config(&self) -> &Config {
        &self.registry.config
    }

    pub(crate) fn news_creator_client(&self) -> Arc<NewsCreatorClient> {
        Arc::clone(&self.registry.news_creator_client)
    }

    pub(crate) fn subworker_client(&self) -> Arc<SubworkerClient> {
        Arc::clone(&self.registry.subworker_client)
    }

    pub(crate) fn dao(&self) -> Arc<dyn RecapDao> {
        Arc::clone(&self.registry.recap_dao)
    }

    pub(crate) fn pool(&self) -> &sqlx::PgPool {
        &self.registry.recap_pool
    }
}

impl ComponentRegistry {
    /// 構成情報と依存をまとめて初期化し、アプリケーションの共有レジストリを構築する。
    ///
    /// # Errors
    /// Telemetry の初期化や HTTP クライアント構築が失敗した場合はエラーを返す。
    pub async fn build(config: Config) -> Result<Self> {
        let config = Arc::new(config);
        let telemetry = Telemetry::new()?;

        // When MTLS_ENFORCE=true, present the recap-worker leaf cert on every
        // outbound request. Fail-closed: missing cert/key/CA fails startup.
        let mtls_paths = crate::clients::mtls::MtlsPaths::from_env()
            .context("resolving mTLS env for outbound clients")?;

        let news_creator_client =
            Arc::new(build_news_creator_client(&config, mtls_paths.as_ref()).await?);

        let subworker_client = Arc::new(
            if let Some(paths) = mtls_paths.as_ref() {
                let client = crate::clients::mtls::build_mtls_client(
                    paths,
                    std::time::Duration::from_secs(5),
                    std::time::Duration::from_hours(1),
                )?;
                SubworkerClient::new_with_client(
                    config.subworker_base_url(),
                    config.min_documents_per_genre(),
                    client,
                )?
            } else {
                SubworkerClient::new(
                    config.subworker_base_url(),
                    config.min_documents_per_genre(),
                )?
            }
            .with_coarse_classify_timeout(config.subworker_coarse_classify_timeout()),
        );
        let recap_pool = PgPoolOptions::new()
            .max_connections(config.recap_db_max_connections())
            .min_connections(config.recap_db_min_connections())
            .acquire_timeout(config.recap_db_acquire_timeout())
            .idle_timeout(Some(config.recap_db_idle_timeout()))
            .max_lifetime(Some(config.recap_db_max_lifetime()))
            .test_before_acquire(true)
            .connect_lazy(config.recap_db_dsn())
            .context("failed to configure recap_db connection pool")?;
        let recap_dao: Arc<dyn RecapDao> = Arc::new(RecapDaoImpl::new(recap_pool.clone()));

        // Initialize classification job queue (use same pool)
        let queue_store = QueueStore::new(recap_pool.clone());
        let classification_queue = Arc::new(ClassificationJobQueue::new(
            queue_store,
            (*subworker_client).clone(),
            config.classification_queue_concurrency(),
            config.classification_queue_chunk_size(),
            config.classification_queue_max_retries(),
            config.classification_queue_retry_delay_ms(),
        ));

        let metrics = telemetry.metrics_arc();
        let pipeline = Arc::new(
            PipelineOrchestrator::new(
                Arc::clone(&config),
                (*subworker_client).clone(),
                Arc::clone(&news_creator_client),
                Arc::clone(&recap_dao),
                Arc::clone(&classification_queue),
                metrics,
            )
            .await?,
        );
        let morning_pipeline = Arc::new(crate::pipeline::morning::MorningPipeline::new(
            Arc::clone(&config),
            Arc::clone(&recap_dao),
            Arc::clone(&news_creator_client),
            Arc::clone(&subworker_client),
        )?);
        let scheduler = Scheduler::new(
            Arc::clone(&pipeline),
            morning_pipeline,
            Arc::clone(&config),
            Arc::clone(&recap_dao),
            Arc::clone(&subworker_client),
        );

        let notification_relay = build_notification_relay(
            &config,
            mtls_paths.as_ref(),
            recap_pool.clone(),
            telemetry.metrics_arc(),
        )?;

        Ok(Self {
            config,
            telemetry,
            scheduler,
            news_creator_client,
            subworker_client,
            recap_dao,
            recap_pool,
            notification_relay,
        })
    }

    /// Start the notification outbox relay, if it is enabled.
    ///
    /// Returns the task handle so the caller can await it during shutdown
    /// rather than letting an in-flight forward be aborted mid-write.
    /// `None` means the relay is off by explicit configuration — which the
    /// startup log already said out loud.
    pub fn spawn_notification_relay(&self, cancel: CancellationToken) -> Option<JoinHandle<()>> {
        let relay = Arc::clone(self.notification_relay.as_ref()?);
        Some(tokio::spawn(async move { relay.run(cancel).await }))
    }

    #[must_use]
    pub fn scheduler(&self) -> &Scheduler {
        &self.scheduler
    }

    #[must_use]
    pub fn config(&self) -> Arc<Config> {
        Arc::clone(&self.config)
    }

    #[must_use]
    pub fn telemetry(&self) -> &Telemetry {
        &self.telemetry
    }
}

async fn build_news_creator_client(
    config: &Config,
    mtls_paths: Option<&crate::clients::mtls::MtlsPaths>,
) -> Result<NewsCreatorClient> {
    let Some(paths) = mtls_paths else {
        return Ok(NewsCreatorClient::new(
            config.news_creator_base_url(),
            config.llm_summary_timeout(),
        )
        .await?);
    };
    let client = crate::clients::mtls::build_mtls_client(
        paths,
        std::time::Duration::from_secs(5),
        config.llm_summary_timeout(),
    )?;
    Ok(NewsCreatorClient::new_with_client(
        config.news_creator_base_url(),
        config.llm_summary_timeout(),
        client,
    )
    .await?)
}

/// Env flag deciding whether the outbox relay runs. Read strictly: an
/// unparseable value fails startup rather than silently resolving to a
/// default, because "the relay is off" and "the operator fat-fingered the
/// flag" must not look the same from the outside.
const RELAY_ENABLED_ENV: &str = "RECAP_NOTIFICATION_RELAY_ENABLED";

fn relay_enabled() -> Result<bool> {
    let Ok(raw) = std::env::var(RELAY_ENABLED_ENV) else {
        return Ok(true);
    };
    match raw.trim().to_ascii_lowercase().as_str() {
        "true" | "1" | "yes" | "on" => Ok(true),
        "false" | "0" | "no" | "off" => Ok(false),
        other => Err(anyhow!(
            "{RELAY_ENABLED_ENV} must be a boolean, got {other:?}"
        )),
    }
}

/// Build the relay, or state plainly that it is disabled.
///
/// The data-hub URL resolution mirrors `pipeline::orchestrator`: under
/// `MTLS_ENFORCE` the east-west URL (`ALT_BACKEND_MTLS_URL`, pointed at
/// alt-data-hub since ADR-000954 Wave 2-A) wins, otherwise the plaintext base
/// URL. Building the client is fail-closed — an enabled relay that cannot
/// construct its client is a startup failure, never a quiet `None`.
fn build_notification_relay(
    config: &Config,
    mtls_paths: Option<&crate::clients::mtls::MtlsPaths>,
    pool: sqlx::PgPool,
    metrics: Arc<crate::observability::metrics::Metrics>,
) -> Result<Option<Arc<NotificationRelay>>> {
    if !relay_enabled()? {
        tracing::info!(
            "notification_outbox_relay_disabled: {RELAY_ENABLED_ENV} is off; completed \
             recaps will enqueue outbox rows that nothing forwards"
        );
        return Ok(None);
    }

    let base_url = if mtls_paths.is_some() {
        std::env::var("ALT_BACKEND_MTLS_URL")
            .unwrap_or_else(|_| config.alt_backend_base_url().to_string())
    } else {
        config.alt_backend_base_url().to_string()
    };

    let client = if let Some(paths) = mtls_paths {
        let http = crate::clients::mtls::build_mtls_client(
            paths,
            config.alt_backend_connect_timeout(),
            config.alt_backend_total_timeout(),
        )
        .context("failed to build alt-data-hub mTLS client for the notification relay")?;
        DataHubClient::new_with_client(base_url, http)
    } else {
        DataHubClient::new(
            base_url,
            config.alt_backend_connect_timeout(),
            config.alt_backend_total_timeout(),
        )
    }
    .context("failed to build alt-data-hub client for the notification relay")?;

    // Identifies the lease holder in `locked_by`; the pid distinguishes two
    // relays that shared a hostname across a restart.
    let locked_by = format!(
        "{}:{}",
        std::env::var("HOSTNAME").unwrap_or_else(|_| "recap-worker".to_string()),
        std::process::id()
    );

    Ok(Some(Arc::new(NotificationRelay::new(
        pool,
        Arc::new(client),
        metrics,
        locked_by,
        RelayConfig::default(),
    ))))
}

pub fn build_router(registry: ComponentRegistry) -> Router {
    let state = AppState::new(registry);
    api::router(state)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::config::ENV_MUTEX;

    #[tokio::test]
    #[ignore = "loads ML model, slow"]
    // See the identical justification in `api::generate::tests` — a
    // single-threaded test, not a shared multi-task resource.
    #[allow(clippy::await_holding_lock)]
    async fn component_registry_builds() {
        // Lock stays held across `ComponentRegistry::build` too — it
        // re-reads `MTLS_ENFORCE` directly from the process env via
        // `MtlsPaths::from_env()`, so releasing the lock right after
        // `Config::from_env` would leave a window for a concurrent test's
        // env mutation to leak in.
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        let registry = temp_env::async_with_vars(
            [
                (
                    "RECAP_DB_DSN",
                    Some("postgres://user:pass@localhost:5555/recap_db"),
                ),
                ("NEWS_CREATOR_BASE_URL", Some("http://localhost:8001/")),
                ("SUBWORKER_BASE_URL", Some("http://localhost:8002/")),
                ("ALT_BACKEND_BASE_URL", Some("http://localhost:9000/")),
                ("RECAP_KNOWLEDGE_EMIT", Some("false")),
                (
                    "HUGGING_FACE_TOKEN_PATH",
                    Some("/tmp/test-token-which-does-not-exist"),
                ),
            ],
            async {
                let config = Config::from_env().expect("config loads");
                ComponentRegistry::build(config)
                    .await
                    .expect("registry builds")
            },
        )
        .await;
        let state = AppState::new(registry);

        state.telemetry().record_ready_probe();
        let _ = state.news_creator_client();
        let _ = state.subworker_client();

        let job = crate::scheduler::JobContext::new(uuid::Uuid::new_v4(), vec![]);
        let result = state.scheduler().run_job(job).await;
        assert!(result.is_err(), "default pipeline should be unimplemented");
    }
}
