use std::sync::Arc;

use anyhow::{Context, Result};
use axum::Router;
use sqlx::postgres::PgPoolOptions;

use crate::{
    api,
    clients::{NewsCreatorClient, SubworkerClient},
    config::Config,
    observability::Telemetry,
    pipeline::PipelineOrchestrator,
    scheduler::Scheduler,
    store::dao::RecapDao,
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
    recap_dao: Arc<RecapDao>,
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

    pub(crate) fn dao(&self) -> Arc<RecapDao> {
        Arc::clone(&self.registry.recap_dao)
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
        let news_creator_client = Arc::new(NewsCreatorClient::new(
            config.news_creator_base_url(),
            config.llm_summary_timeout(),
        )?);
        let subworker_client = Arc::new(SubworkerClient::new(
            config.subworker_base_url(),
            config.min_documents_per_genre(),
        )?);
        let recap_pool = PgPoolOptions::new()
            .max_connections(config.recap_db_max_connections())
            .min_connections(config.recap_db_min_connections())
            .acquire_timeout(config.recap_db_acquire_timeout())
            .idle_timeout(Some(config.recap_db_idle_timeout()))
            .max_lifetime(Some(config.recap_db_max_lifetime()))
            .test_before_acquire(true)
            .connect_lazy(config.recap_db_dsn())
            .context("failed to configure recap_db connection pool")?;
        let recap_dao = Arc::new(RecapDao::new(recap_pool));
        let metrics = telemetry.metrics_arc();
        let pipeline = Arc::new(
            PipelineOrchestrator::new(
                Arc::clone(&config),
                (*subworker_client).clone(),
                Arc::clone(&news_creator_client),
                Arc::clone(&recap_dao),
                metrics,
            )
            .await?,
        );
        let morning_pipeline = Arc::new(crate::pipeline::morning::MorningPipeline::new(
            Arc::clone(&config),
            Arc::clone(&recap_dao),
        ));
        let scheduler =
            Scheduler::new(Arc::clone(&pipeline), morning_pipeline, Arc::clone(&config));

        Ok(Self {
            config,
            telemetry,
            scheduler,
            news_creator_client,
            subworker_client,
            recap_dao,
        })
    }

    #[must_use]
    pub fn scheduler(&self) -> &Scheduler {
        &self.scheduler
    }

    #[must_use]
    pub fn config(&self) -> Arc<Config> {
        Arc::clone(&self.config)
    }
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
    async fn component_registry_builds() {
        let config = {
            let _lock = ENV_MUTEX.lock().expect("env mutex");
            // SAFETY: test code adjusts deterministic environment state sequentially.
            unsafe {
                std::env::set_var(
                    "RECAP_DB_DSN",
                    "postgres://user:pass@localhost:5555/recap_db",
                );
                std::env::set_var("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
                std::env::set_var("SUBWORKER_BASE_URL", "http://localhost:8002/");
                std::env::set_var("ALT_BACKEND_BASE_URL", "http://localhost:9000/");
                std::env::remove_var("ALT_BACKEND_SERVICE_TOKEN");
            }

            Config::from_env().expect("config loads")
        };
        let registry = ComponentRegistry::build(config)
            .await
            .expect("registry builds");
        let state = AppState::new(registry);

        state.telemetry().record_ready_probe();
        let _ = state.news_creator_client();
        let _ = state.subworker_client();

        let job = crate::scheduler::JobContext::new(uuid::Uuid::new_v4(), vec![]);
        let result = state.scheduler().run_job(job).await;
        assert!(result.is_err(), "default pipeline should be unimplemented");
    }
}
