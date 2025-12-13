use std::{env, net::SocketAddr, num::NonZeroUsize, time::Duration};

use thiserror::Error;

use crate::clients::subworker::CLASSIFY_CHUNK_SIZE;

#[cfg(test)]
use once_cell::sync::Lazy;
#[cfg(test)]
pub(crate) static ENV_MUTEX: Lazy<std::sync::Mutex<()>> = Lazy::new(|| std::sync::Mutex::new(()));

/// boolの組み合わせ爆発を避けるための2値トグル。
///
/// Clippyの `struct_excessive_bools` 対策として、設定フラグは可能な限り
/// 2値enum（Enabled/Disabled）や状態enumで表現する。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum FeatureToggle {
    Enabled,
    Disabled,
}

impl FeatureToggle {
    #[must_use]
    pub const fn is_enabled(self) -> bool {
        matches!(self, FeatureToggle::Enabled)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RequireTagsPolicy {
    Require,
    Optional,
}

impl RequireTagsPolicy {
    #[must_use]
    pub const fn requires_tags(self) -> bool {
        matches!(self, RequireTagsPolicy::Require)
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BootstrapEvalConfig {
    Disabled,
    Enabled { n_bootstrap: i32 },
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CrossValidationEvalConfig {
    Disabled,
    Enabled,
}

/// 分類評価の設定（不正な組合せを型で禁止）。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClassificationEvalConfig {
    Disabled,
    Enabled {
        bootstrap: BootstrapEvalConfig,
        cv: CrossValidationEvalConfig,
    },
}

#[derive(Debug, Clone, PartialEq)]
pub struct Config {
    http_bind: SocketAddr,
    llm_max_concurrency: NonZeroUsize,
    llm_prompt_version: String,
    recap_db_dsn: String,
    news_creator_base_url: String,
    subworker_base_url: String,
    alt_backend_base_url: String,
    alt_backend_service_token: Option<String>,
    alt_backend_connect_timeout: Duration,
    alt_backend_read_timeout: Duration,
    alt_backend_total_timeout: Duration,
    http_max_retries: usize,
    http_backoff_base_ms: u64,
    http_backoff_cap_ms: u64,
    otel_exporter_endpoint: Option<String>,
    otel_sampling_ratio: f64,
    recap_window_days: u32,
    recap_genres: Vec<String>,
    genre_classifier_weights_path: Option<String>,
    genre_classifier_weights_path_ja: Option<String>,
    genre_classifier_weights_path_en: Option<String>,
    genre_classifier_threshold: f32,
    genre_refine: FeatureToggle,
    genre_refine_require_tags: RequireTagsPolicy,
    genre_refine_rollout_pct: u8,
    lang_detect_min_chars: usize,
    lang_detect_min_confidence: f64,
    tag_label_graph_window: String,
    tag_label_graph_ttl: Duration,
    tag_generator_base_url: String,
    tag_generator_service_token: Option<String>,
    tag_generator_connect_timeout: Duration,
    tag_generator_total_timeout: Duration,
    min_documents_per_genre: usize,
    coherence_similarity_threshold: f32,
    subgenre_max_docs_per_genre: usize,
    subgenre_target_docs_per_subgenre: usize,
    subgenre_max_k: usize,
    recap_pre_refresh_graph: FeatureToggle,
    recap_pre_refresh_timeout: Duration,
    llm_summary_timeout: Duration,
    recap_db_max_connections: u32,
    recap_db_min_connections: u32,
    recap_db_acquire_timeout: Duration,
    recap_db_idle_timeout: Duration,
    recap_db_max_lifetime: Duration,
    classification_queue_concurrency: usize,
    classification_queue_chunk_size: usize,
    classification_queue_max_retries: i32,
    classification_queue_retry_delay_ms: u64,
    job_retention_days: i64,
    classification_eval: ClassificationEvalConfig,
    clustering_genre_timeout: Duration,
    clustering_job_timeout: Duration,
    clustering_min_success_genres: usize,
}

#[derive(Debug, Error)]
pub enum ConfigError {
    #[error("missing environment variable: {0}")]
    Missing(&'static str),
    #[error("invalid value for {name}: {source}")]
    Invalid {
        name: &'static str,
        #[source]
        source: anyhow::Error,
    },
}

struct BasicConfig {
    recap_db_dsn: String,
    http_bind: SocketAddr,
    llm_prompt_version: String,
    llm_max_concurrency: NonZeroUsize,
    llm_summary_timeout: Duration,
}

struct ExternalServiceConfig {
    news_creator_base_url: String,
    subworker_base_url: String,
    alt_backend_base_url: String,
    alt_backend_service_token: Option<String>,
    alt_backend_connect_timeout: Duration,
    alt_backend_read_timeout: Duration,
    alt_backend_total_timeout: Duration,
    http_max_retries: usize,
    http_backoff_base_ms: u64,
    http_backoff_cap_ms: u64,
    otel_exporter_endpoint: Option<String>,
    otel_sampling_ratio: f64,
}

struct BatchConfig {
    recap_window_days: u32,
    recap_genres: Vec<String>,
    genre_classifier_weights_path: Option<String>,
    genre_classifier_weights_path_ja: Option<String>,
    genre_classifier_weights_path_en: Option<String>,
    genre_classifier_threshold: f32,
    genre_refine: FeatureToggle,
    genre_refine_require_tags: RequireTagsPolicy,
    genre_refine_rollout_pct: u8,
    lang_detect_min_chars: usize,
    lang_detect_min_confidence: f64,
}

struct GraphConfig {
    tag_label_graph_window: String,
    tag_label_graph_ttl: Duration,
}

#[allow(clippy::struct_field_names)] // Field names match Config struct for clarity
struct TagConfig {
    tag_generator_base_url: String,
    tag_generator_service_token: Option<String>,
    tag_generator_connect_timeout: Duration,
    tag_generator_total_timeout: Duration,
}

struct SubworkerConfig {
    min_documents_per_genre: usize,
    coherence_similarity_threshold: f32,
    subgenre_max_docs_per_genre: usize,
    subgenre_target_docs_per_subgenre: usize,
    subgenre_max_k: usize,
}

struct PreRefreshConfig {
    recap_pre_refresh_graph: FeatureToggle,
    recap_pre_refresh_timeout: Duration,
}

#[allow(clippy::struct_field_names)] // Field names match Config struct for clarity
struct DbPoolConfig {
    recap_db_max_connections: u32,
    recap_db_min_connections: u32,
    recap_db_acquire_timeout: Duration,
    recap_db_idle_timeout: Duration,
    recap_db_max_lifetime: Duration,
}

#[allow(clippy::struct_field_names)] // Field names match Config struct for clarity
struct QueueConfig {
    classification_queue_concurrency: usize,
    classification_queue_chunk_size: usize,
    classification_queue_max_retries: i32,
    classification_queue_retry_delay_ms: u64,
}

impl Config {
    /// 環境変数から Recap Worker の設定値を読み込み、検証する。
    ///
    /// 必須の環境変数が揃っていない場合や、数値／アドレスのパースに失敗した場合はエラーを返す。
    ///
    /// # Errors
    /// `RECAP_DB_DSN` が未設定、もしくは各種値のパースに失敗した場合は [`ConfigError`] を返す。
    pub fn from_env() -> Result<Self, ConfigError> {
        let basic = load_basic_config()?;
        let external_services = load_external_service_config()?;
        let batch = load_batch_config()?;
        let graph = load_graph_config()?;
        let tag = load_tag_config()?;
        let subworker = load_subworker_config()?;
        let pre_refresh = load_pre_refresh_config()?;
        let db_pool = load_db_pool_config()?;
        let queue = load_classification_queue_config()?;
        let job_retention_days = parse_i64("RECAP_JOB_RETENTION_DAYS", 14)?;
        let classification_eval_enabled = parse_bool("RECAP_CLASSIFICATION_EVAL_ENABLED", true)?;
        let classification_eval_use_bootstrap =
            parse_bool("RECAP_CLASSIFICATION_EVAL_USE_BOOTSTRAP", true)?;
        let classification_eval_n_bootstrap =
            parse_i32("RECAP_CLASSIFICATION_EVAL_N_BOOTSTRAP", 200)?;
        let classification_eval_use_cv = parse_bool("RECAP_CLASSIFICATION_EVAL_USE_CV", false)?;
        let classification_eval = if classification_eval_enabled {
            ClassificationEvalConfig::Enabled {
                bootstrap: if classification_eval_use_bootstrap {
                    BootstrapEvalConfig::Enabled {
                        n_bootstrap: classification_eval_n_bootstrap,
                    }
                } else {
                    BootstrapEvalConfig::Disabled
                },
                cv: if classification_eval_use_cv {
                    CrossValidationEvalConfig::Enabled
                } else {
                    CrossValidationEvalConfig::Disabled
                },
            }
        } else {
            ClassificationEvalConfig::Disabled
        };
        let clustering_genre_timeout =
            parse_duration_secs("RECAP_CLUSTERING_GENRE_TIMEOUT_SECS", 300)?; // 5分
        let clustering_job_timeout =
            parse_duration_secs("RECAP_CLUSTERING_JOB_TIMEOUT_SECS", 1800)?; // 30分
        let clustering_min_success_genres = parse_usize("RECAP_CLUSTERING_MIN_SUCCESS_GENRES", 1)?;

        Ok(Self::from_components(
            basic,
            external_services,
            batch,
            graph,
            tag,
            subworker,
            pre_refresh,
            db_pool,
            queue,
            job_retention_days,
            classification_eval,
            clustering_genre_timeout,
            clustering_job_timeout,
            clustering_min_success_genres,
        ))
    }

    #[allow(clippy::too_many_arguments)] // Internal helper method that groups related configs
    fn from_components(
        basic: BasicConfig,
        external_services: ExternalServiceConfig,
        batch: BatchConfig,
        graph: GraphConfig,
        tag: TagConfig,
        subworker: SubworkerConfig,
        pre_refresh: PreRefreshConfig,
        db_pool: DbPoolConfig,
        queue: QueueConfig,
        job_retention_days: i64,
        classification_eval: ClassificationEvalConfig,
        clustering_genre_timeout: Duration,
        clustering_job_timeout: Duration,
        clustering_min_success_genres: usize,
    ) -> Self {
        Self {
            http_bind: basic.http_bind,
            llm_max_concurrency: basic.llm_max_concurrency,
            llm_prompt_version: basic.llm_prompt_version,
            recap_db_dsn: basic.recap_db_dsn,
            news_creator_base_url: external_services.news_creator_base_url,
            subworker_base_url: external_services.subworker_base_url,
            alt_backend_base_url: external_services.alt_backend_base_url,
            alt_backend_service_token: external_services.alt_backend_service_token,
            alt_backend_connect_timeout: external_services.alt_backend_connect_timeout,
            alt_backend_read_timeout: external_services.alt_backend_read_timeout,
            alt_backend_total_timeout: external_services.alt_backend_total_timeout,
            http_max_retries: external_services.http_max_retries,
            http_backoff_base_ms: external_services.http_backoff_base_ms,
            http_backoff_cap_ms: external_services.http_backoff_cap_ms,
            otel_exporter_endpoint: external_services.otel_exporter_endpoint,
            otel_sampling_ratio: external_services.otel_sampling_ratio,
            recap_window_days: batch.recap_window_days,
            recap_genres: batch.recap_genres,
            genre_classifier_weights_path: batch.genre_classifier_weights_path,
            genre_classifier_weights_path_ja: batch.genre_classifier_weights_path_ja,
            genre_classifier_weights_path_en: batch.genre_classifier_weights_path_en,
            genre_classifier_threshold: batch.genre_classifier_threshold,
            genre_refine: batch.genre_refine,
            lang_detect_min_chars: batch.lang_detect_min_chars,
            lang_detect_min_confidence: batch.lang_detect_min_confidence,
            genre_refine_require_tags: batch.genre_refine_require_tags,
            genre_refine_rollout_pct: batch.genre_refine_rollout_pct,
            tag_label_graph_window: graph.tag_label_graph_window,
            tag_label_graph_ttl: graph.tag_label_graph_ttl,
            tag_generator_base_url: tag.tag_generator_base_url,
            tag_generator_service_token: tag.tag_generator_service_token,
            tag_generator_connect_timeout: tag.tag_generator_connect_timeout,
            tag_generator_total_timeout: tag.tag_generator_total_timeout,
            min_documents_per_genre: subworker.min_documents_per_genre,
            coherence_similarity_threshold: subworker.coherence_similarity_threshold,
            subgenre_max_docs_per_genre: subworker.subgenre_max_docs_per_genre,
            subgenre_target_docs_per_subgenre: subworker.subgenre_target_docs_per_subgenre,
            subgenre_max_k: subworker.subgenre_max_k,
            recap_pre_refresh_graph: pre_refresh.recap_pre_refresh_graph,
            recap_pre_refresh_timeout: pre_refresh.recap_pre_refresh_timeout,
            llm_summary_timeout: basic.llm_summary_timeout,
            recap_db_max_connections: db_pool.recap_db_max_connections,
            recap_db_min_connections: db_pool.recap_db_min_connections,
            recap_db_acquire_timeout: db_pool.recap_db_acquire_timeout,
            recap_db_idle_timeout: db_pool.recap_db_idle_timeout,
            recap_db_max_lifetime: db_pool.recap_db_max_lifetime,
            classification_queue_concurrency: queue.classification_queue_concurrency,
            classification_queue_chunk_size: queue.classification_queue_chunk_size,
            classification_queue_max_retries: queue.classification_queue_max_retries,
            classification_queue_retry_delay_ms: queue.classification_queue_retry_delay_ms,
            job_retention_days,
            classification_eval,
            clustering_genre_timeout,
            clustering_job_timeout,
            clustering_min_success_genres,
        }
    }

    #[must_use]
    pub fn http_bind(&self) -> SocketAddr {
        self.http_bind
    }

    #[must_use]
    pub fn llm_max_concurrency(&self) -> NonZeroUsize {
        self.llm_max_concurrency
    }

    #[must_use]
    pub fn llm_prompt_version(&self) -> &str {
        &self.llm_prompt_version
    }

    #[must_use]
    pub fn recap_db_dsn(&self) -> &str {
        &self.recap_db_dsn
    }

    #[must_use]
    pub fn news_creator_base_url(&self) -> &str {
        &self.news_creator_base_url
    }

    #[must_use]
    pub fn subworker_base_url(&self) -> &str {
        &self.subworker_base_url
    }

    #[must_use]
    pub fn alt_backend_base_url(&self) -> &str {
        &self.alt_backend_base_url
    }

    #[must_use]
    pub fn alt_backend_service_token(&self) -> Option<&str> {
        self.alt_backend_service_token.as_deref()
    }

    #[must_use]
    pub fn alt_backend_connect_timeout(&self) -> Duration {
        self.alt_backend_connect_timeout
    }

    #[must_use]
    pub fn alt_backend_read_timeout(&self) -> Duration {
        self.alt_backend_read_timeout
    }

    #[must_use]
    pub fn alt_backend_total_timeout(&self) -> Duration {
        self.alt_backend_total_timeout
    }

    #[must_use]
    pub fn http_max_retries(&self) -> usize {
        self.http_max_retries
    }

    #[must_use]
    pub fn http_backoff_base_ms(&self) -> u64 {
        self.http_backoff_base_ms
    }

    #[must_use]
    pub fn http_backoff_cap_ms(&self) -> u64 {
        self.http_backoff_cap_ms
    }

    #[must_use]
    pub fn otel_exporter_endpoint(&self) -> Option<&str> {
        self.otel_exporter_endpoint.as_deref()
    }

    #[must_use]
    pub fn otel_sampling_ratio(&self) -> f64 {
        self.otel_sampling_ratio
    }

    #[must_use]
    pub fn recap_window_days(&self) -> u32 {
        self.recap_window_days
    }

    #[must_use]
    pub fn recap_genres(&self) -> &[String] {
        &self.recap_genres
    }

    #[must_use]
    pub fn genre_classifier_weights_path(&self) -> Option<&str> {
        self.genre_classifier_weights_path.as_deref()
    }

    #[must_use]
    pub fn genre_classifier_weights_path_ja(&self) -> Option<&str> {
        self.genre_classifier_weights_path_ja.as_deref()
    }

    #[must_use]
    pub fn genre_classifier_weights_path_en(&self) -> Option<&str> {
        self.genre_classifier_weights_path_en.as_deref()
    }

    #[must_use]
    pub fn genre_classifier_threshold(&self) -> f32 {
        self.genre_classifier_threshold
    }

    #[must_use]
    pub fn lang_detect_min_chars(&self) -> usize {
        self.lang_detect_min_chars
    }

    #[must_use]
    pub fn lang_detect_min_confidence(&self) -> f64 {
        self.lang_detect_min_confidence
    }

    #[must_use]
    pub fn genre_refine_enabled(&self) -> bool {
        self.genre_refine.is_enabled()
    }

    #[must_use]
    pub fn genre_refine_require_tags(&self) -> bool {
        self.genre_refine_require_tags.requires_tags()
    }

    #[must_use]
    pub fn genre_refine_rollout_pct(&self) -> u8 {
        self.genre_refine_rollout_pct
    }

    #[must_use]
    pub fn tag_label_graph_window(&self) -> &str {
        &self.tag_label_graph_window
    }

    #[must_use]
    pub fn tag_label_graph_ttl(&self) -> Duration {
        self.tag_label_graph_ttl
    }

    #[must_use]
    pub fn tag_generator_base_url(&self) -> &str {
        &self.tag_generator_base_url
    }

    #[must_use]
    pub fn tag_generator_service_token(&self) -> Option<&str> {
        self.tag_generator_service_token.as_deref()
    }

    #[must_use]
    pub fn tag_generator_connect_timeout(&self) -> Duration {
        self.tag_generator_connect_timeout
    }

    #[must_use]
    pub fn tag_generator_total_timeout(&self) -> Duration {
        self.tag_generator_total_timeout
    }

    #[must_use]
    pub fn min_documents_per_genre(&self) -> usize {
        self.min_documents_per_genre
    }

    #[must_use]
    pub fn coherence_similarity_threshold(&self) -> f32 {
        self.coherence_similarity_threshold
    }

    #[must_use]
    pub fn subgenre_max_docs_per_genre(&self) -> usize {
        self.subgenre_max_docs_per_genre
    }

    #[must_use]
    pub fn subgenre_target_docs_per_subgenre(&self) -> usize {
        self.subgenre_target_docs_per_subgenre
    }

    #[must_use]
    pub fn subgenre_max_k(&self) -> usize {
        self.subgenre_max_k
    }

    pub fn recap_pre_refresh_graph_enabled(&self) -> bool {
        self.recap_pre_refresh_graph.is_enabled()
    }

    pub fn recap_pre_refresh_timeout(&self) -> Duration {
        self.recap_pre_refresh_timeout
    }

    #[must_use]
    pub fn llm_summary_timeout(&self) -> Duration {
        self.llm_summary_timeout
    }

    #[must_use]
    pub fn recap_db_max_connections(&self) -> u32 {
        self.recap_db_max_connections
    }

    #[must_use]
    pub fn recap_db_min_connections(&self) -> u32 {
        self.recap_db_min_connections
    }

    #[must_use]
    pub fn recap_db_acquire_timeout(&self) -> Duration {
        self.recap_db_acquire_timeout
    }

    #[must_use]
    pub fn recap_db_idle_timeout(&self) -> Duration {
        self.recap_db_idle_timeout
    }

    #[must_use]
    pub fn recap_db_max_lifetime(&self) -> Duration {
        self.recap_db_max_lifetime
    }

    #[must_use]
    pub fn classification_queue_concurrency(&self) -> usize {
        self.classification_queue_concurrency
    }

    #[must_use]
    pub fn classification_queue_chunk_size(&self) -> usize {
        self.classification_queue_chunk_size
    }

    #[must_use]
    pub fn classification_queue_max_retries(&self) -> i32 {
        self.classification_queue_max_retries
    }

    #[must_use]
    pub fn classification_queue_retry_delay_ms(&self) -> u64 {
        self.classification_queue_retry_delay_ms
    }

    #[must_use]
    pub fn job_retention_days(&self) -> i64 {
        self.job_retention_days
    }

    #[must_use]
    pub fn classification_eval_enabled(&self) -> bool {
        matches!(
            self.classification_eval,
            ClassificationEvalConfig::Enabled { .. }
        )
    }

    #[must_use]
    pub fn classification_eval_use_bootstrap(&self) -> bool {
        matches!(
            self.classification_eval,
            ClassificationEvalConfig::Enabled {
                bootstrap: BootstrapEvalConfig::Enabled { .. },
                ..
            }
        )
    }

    #[must_use]
    pub fn classification_eval_n_bootstrap(&self) -> i32 {
        match self.classification_eval {
            ClassificationEvalConfig::Enabled {
                bootstrap: BootstrapEvalConfig::Enabled { n_bootstrap },
                ..
            } => n_bootstrap,
            _ => 0,
        }
    }

    #[must_use]
    pub fn classification_eval_use_cv(&self) -> bool {
        matches!(
            self.classification_eval,
            ClassificationEvalConfig::Enabled {
                cv: CrossValidationEvalConfig::Enabled,
                ..
            }
        )
    }

    #[must_use]
    pub fn clustering_genre_timeout(&self) -> Duration {
        self.clustering_genre_timeout
    }

    #[must_use]
    pub fn clustering_job_timeout(&self) -> Duration {
        self.clustering_job_timeout
    }

    #[must_use]
    pub fn clustering_min_success_genres(&self) -> usize {
        self.clustering_min_success_genres
    }
}

fn load_basic_config() -> Result<BasicConfig, ConfigError> {
    let recap_db_dsn = load_database_dsn()?;
    let http_bind = parse_socket_addr("RECAP_WORKER_HTTP_BIND", "0.0.0.0:9005")?;
    let llm_prompt_version =
        env::var("LLM_PROMPT_VERSION").unwrap_or_else(|_| "recap-ja-v2".to_string());
    let llm_max_concurrency = parse_non_zero_usize("LLM_MAX_CONCURRENCY", 1)?;
    let llm_summary_timeout = parse_duration_secs("LLM_SUMMARY_TIMEOUT_SECS", 600)?;

    Ok(BasicConfig {
        recap_db_dsn,
        http_bind,
        llm_prompt_version,
        llm_max_concurrency,
        llm_summary_timeout,
    })
}

fn load_external_service_config() -> Result<ExternalServiceConfig, ConfigError> {
    let news_creator_base_url = env_var("NEWS_CREATOR_BASE_URL")?;
    let subworker_base_url = env_var("SUBWORKER_BASE_URL")?;
    let alt_backend_base_url = env_var("ALT_BACKEND_BASE_URL")?;
    let alt_backend_service_token = env_var_optional("ALT_BACKEND_SERVICE_TOKEN");

    let (alt_backend_connect_timeout, alt_backend_read_timeout, alt_backend_total_timeout) =
        load_http_timeout_config()?;
    let (http_max_retries, http_backoff_base_ms, http_backoff_cap_ms) = load_retry_config()?;
    let (otel_exporter_endpoint, otel_sampling_ratio) = load_observability_config()?;

    Ok(ExternalServiceConfig {
        news_creator_base_url,
        subworker_base_url,
        alt_backend_base_url,
        alt_backend_service_token,
        alt_backend_connect_timeout,
        alt_backend_read_timeout,
        alt_backend_total_timeout,
        http_max_retries,
        http_backoff_base_ms,
        http_backoff_cap_ms,
        otel_exporter_endpoint,
        otel_sampling_ratio,
    })
}

fn load_database_dsn() -> Result<String, ConfigError> {
    match env_var("RECAP_DB_DSN") {
        Ok(dsn) => Ok(dsn),
        Err(ConfigError::Missing(_)) => {
            // Try to build from components
            // Check all components exist before building DSN
            // This ensures we return RECAP_DB_DSN error if all components are missing
            if env_var("RECAP_DB_HOST").is_err()
                && env_var("RECAP_DB_PORT").is_err()
                && env_var("RECAP_DB_USER").is_err()
                && env_var("RECAP_DB_NAME").is_err()
                && env_var("RECAP_DB_PASSWORD").is_err()
            {
                return Err(ConfigError::Missing("RECAP_DB_DSN"));
            }
            let host = env_var("RECAP_DB_HOST")?;
            let port = env_var("RECAP_DB_PORT")?;
            let user = env_var("RECAP_DB_USER")?;
            let dbname = env_var("RECAP_DB_NAME")?;
            let password = env_var("RECAP_DB_PASSWORD")?;
            Ok(format!(
                "postgres://{}:{}@{}:{}/{}",
                user, password, host, port, dbname
            ))
        }
        Err(e) => Err(e),
    }
}

fn load_http_timeout_config() -> Result<(Duration, Duration, Duration), ConfigError> {
    let connect_timeout = parse_duration_ms("ALT_BACKEND_CONNECT_TIMEOUT_MS", 3000)?;
    let read_timeout = parse_duration_ms("ALT_BACKEND_READ_TIMEOUT_MS", 20000)?;
    let total_timeout = parse_duration_ms("ALT_BACKEND_TOTAL_TIMEOUT_MS", 30000)?;
    Ok((connect_timeout, read_timeout, total_timeout))
}

fn load_retry_config() -> Result<(usize, u64, u64), ConfigError> {
    let max_retries = parse_usize("HTTP_MAX_RETRIES", 3)?;
    let backoff_base_ms = parse_u64("HTTP_BACKOFF_BASE_MS", 250)?;
    let backoff_cap_ms = parse_u64("HTTP_BACKOFF_CAP_MS", 10000)?;
    Ok((max_retries, backoff_base_ms, backoff_cap_ms))
}

fn load_observability_config() -> Result<(Option<String>, f64), ConfigError> {
    let exporter_endpoint = env_var_optional("OTEL_EXPORTER_ENDPOINT");
    let sampling_ratio = parse_f64("OTEL_SAMPLING_RATIO", 1.0)?;
    Ok((exporter_endpoint, sampling_ratio))
}

fn load_batch_config() -> Result<BatchConfig, ConfigError> {
    let window_days = parse_u32("RECAP_WINDOW_DAYS", 7)?;
    let genres = parse_csv(
        "RECAP_GENRES",
        "ai_data,climate_environment,consumer_products,consumer_tech,culture_arts,cybersecurity,diplomacy_security,economics_macro,education,energy_transition,film_tv,food_cuisine,games_esports,health_medicine,home_living,industry_logistics,internet_platforms,labor_workplace,law_crime,life_science,markets_finance,mobility_automotive,music_audio,politics_government,society_demographics,software_dev,space_astronomy,sports,startups_innovation,travel_places",
    );
    // レガシー: RECAP_GENRE_MODEL_WEIGHTSはJAのデフォルトとして扱う
    let classifier_weights_path = env_var_optional("RECAP_GENRE_MODEL_WEIGHTS");
    // 新規: 言語別の重みファイルパス
    let classifier_weights_path_ja =
        env_var_optional("RECAP_GENRE_MODEL_WEIGHTS_JA").or(classifier_weights_path.clone());
    let classifier_weights_path_en = env_var_optional("RECAP_GENRE_MODEL_WEIGHTS_EN");
    let classifier_threshold = parse_f64("RECAP_GENRE_MODEL_THRESHOLD", 0.5)? as f32;
    let refine_enabled = parse_bool("RECAP_GENRE_REFINE_ENABLED", false)?;
    let refine_require_tags = parse_bool("RECAP_GENRE_REFINE_REQUIRE_TAGS", true)?;
    let refine_rollout_pct = parse_percentage("RECAP_GENRE_REFINE_ROLLOUT_PERCENT", 100)?;
    let lang_detect_min_chars = parse_usize("RECAP_LANG_DETECT_MIN_CHARS", 50)?;
    let lang_detect_min_confidence = parse_f64("RECAP_LANG_DETECT_MIN_CONFIDENCE", 0.65)?;
    Ok(BatchConfig {
        recap_window_days: window_days,
        recap_genres: genres,
        genre_classifier_weights_path: classifier_weights_path,
        genre_classifier_weights_path_ja: classifier_weights_path_ja,
        genre_classifier_weights_path_en: classifier_weights_path_en,
        genre_classifier_threshold: classifier_threshold,
        genre_refine: if refine_enabled {
            FeatureToggle::Enabled
        } else {
            FeatureToggle::Disabled
        },
        genre_refine_require_tags: if refine_require_tags {
            RequireTagsPolicy::Require
        } else {
            RequireTagsPolicy::Optional
        },
        genre_refine_rollout_pct: refine_rollout_pct,
        lang_detect_min_chars,
        lang_detect_min_confidence,
    })
}

fn load_graph_config() -> Result<GraphConfig, ConfigError> {
    let window = env::var("TAG_LABEL_GRAPH_WINDOW").unwrap_or_else(|_| "7d".to_string());
    let ttl = Duration::from_secs(parse_u64("TAG_LABEL_GRAPH_TTL_SECONDS", 900)?);
    Ok(GraphConfig {
        tag_label_graph_window: window,
        tag_label_graph_ttl: ttl,
    })
}

fn load_tag_config() -> Result<TagConfig, ConfigError> {
    let base_url = env::var("TAG_GENERATOR_BASE_URL")
        .unwrap_or_else(|_| "http://tag-generator:9400".to_string());
    let service_token = env_var_optional("TAG_GENERATOR_SERVICE_TOKEN");
    let connect_timeout = parse_duration_ms("TAG_GENERATOR_CONNECT_TIMEOUT_MS", 3000)?;
    let total_timeout = parse_duration_ms("TAG_GENERATOR_TOTAL_TIMEOUT_MS", 30000)?;
    Ok(TagConfig {
        tag_generator_base_url: base_url,
        tag_generator_service_token: service_token,
        tag_generator_connect_timeout: connect_timeout,
        tag_generator_total_timeout: total_timeout,
    })
}

fn load_subworker_config() -> Result<SubworkerConfig, ConfigError> {
    let min_documents = parse_usize("RECAP_MIN_DOCUMENTS_PER_GENRE", 3)?;
    let similarity_threshold = parse_f64("RECAP_COHERENCE_SIMILARITY_THRESHOLD", 0.5)? as f32;
    let subgenre_max_docs = parse_usize("RECAP_SUBGENRE_MAX_DOCS_PER_GENRE", 200)?;
    let subgenre_target_docs = parse_usize("RECAP_SUBGENRE_TARGET_DOCS_PER_SUBGENRE", 50)?;
    let subgenre_max_k = parse_usize("RECAP_SUBGENRE_MAX_K", 10)?;
    Ok(SubworkerConfig {
        min_documents_per_genre: min_documents,
        coherence_similarity_threshold: similarity_threshold,
        subgenre_max_docs_per_genre: subgenre_max_docs,
        subgenre_target_docs_per_subgenre: subgenre_target_docs,
        subgenre_max_k,
    })
}

fn load_pre_refresh_config() -> Result<PreRefreshConfig, ConfigError> {
    let enabled = parse_bool("RECAP_PRE_REFRESH_GRAPH_ENABLED", true)?;
    let timeout = parse_duration_secs("RECAP_PRE_REFRESH_TIMEOUT_SECS", 300)?;
    Ok(PreRefreshConfig {
        recap_pre_refresh_graph: if enabled {
            FeatureToggle::Enabled
        } else {
            FeatureToggle::Disabled
        },
        recap_pre_refresh_timeout: timeout,
    })
}

fn load_db_pool_config() -> Result<DbPoolConfig, ConfigError> {
    let max_connections = parse_u32("RECAP_DB_MAX_CONNECTIONS", 50)?;
    let min_connections = parse_u32("RECAP_DB_MIN_CONNECTIONS", 5)?;
    let acquire_timeout = parse_duration_secs("RECAP_DB_ACQUIRE_TIMEOUT_SECS", 60)?;
    let idle_timeout = parse_duration_secs("RECAP_DB_IDLE_TIMEOUT_SECS", 600)?;
    let max_lifetime = parse_duration_secs("RECAP_DB_MAX_LIFETIME_SECS", 1800)?;
    Ok(DbPoolConfig {
        recap_db_max_connections: max_connections,
        recap_db_min_connections: min_connections,
        recap_db_acquire_timeout: acquire_timeout,
        recap_db_idle_timeout: idle_timeout,
        recap_db_max_lifetime: max_lifetime,
    })
}

fn load_classification_queue_config() -> Result<QueueConfig, ConfigError> {
    let concurrency = parse_usize("CLASSIFICATION_QUEUE_CONCURRENCY", 8)?;
    let chunk_size = parse_usize("CLASSIFICATION_QUEUE_CHUNK_SIZE", CLASSIFY_CHUNK_SIZE)?;
    let max_retries = parse_u64("CLASSIFICATION_QUEUE_MAX_RETRIES", 3)? as i32;
    let retry_delay_ms = parse_u64("CLASSIFICATION_QUEUE_RETRY_DELAY_MS", 5000)?;
    Ok(QueueConfig {
        classification_queue_concurrency: concurrency,
        classification_queue_chunk_size: chunk_size,
        classification_queue_max_retries: max_retries,
        classification_queue_retry_delay_ms: retry_delay_ms,
    })
}

fn env_var(name: &'static str) -> Result<String, ConfigError> {
    // Check for _FILE suffix
    let file_var = format!("{}_FILE", name);
    if let Ok(path) = env::var(&file_var) {
        if let Ok(content) = std::fs::read_to_string(path) {
            return Ok(content.trim().to_string());
        }
    }
    env::var(name).map_err(|_| ConfigError::Missing(name))
}

fn env_var_optional(name: &'static str) -> Option<String> {
    // Check for _FILE suffix
    let file_var = format!("{}_FILE", name);
    if let Ok(path) = env::var(&file_var) {
        if let Ok(content) = std::fs::read_to_string(path) {
            return Some(content.trim().to_string());
        }
    }
    env::var(name).ok()
}

fn parse_socket_addr(name: &'static str, default: &str) -> Result<SocketAddr, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());

    raw.parse().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_non_zero_usize(name: &'static str, default: usize) -> Result<NonZeroUsize, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    let parsed = raw.parse::<usize>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })?;
    NonZeroUsize::new(parsed).ok_or_else(|| ConfigError::Invalid {
        name,
        source: anyhow::anyhow!("must be greater than zero"),
    })
}

fn parse_duration_secs(name: &'static str, default_secs: u64) -> Result<Duration, ConfigError> {
    let value = parse_u64(name, default_secs)?;
    Ok(Duration::from_secs(value))
}

fn parse_duration_ms(name: &'static str, default_ms: u64) -> Result<Duration, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default_ms.to_string());
    let ms = raw.parse::<u64>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })?;
    Ok(Duration::from_millis(ms))
}

fn parse_usize(name: &'static str, default: usize) -> Result<usize, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<usize>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_u32(name: &'static str, default: u32) -> Result<u32, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<u32>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_u64(name: &'static str, default: u64) -> Result<u64, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<u64>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_i32(name: &'static str, default: i32) -> Result<i32, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<i32>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_i64(name: &'static str, default: i64) -> Result<i64, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<i64>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_percentage(name: &'static str, default: u8) -> Result<u8, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    let parsed = raw.parse::<u8>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })?;
    if parsed > 100 {
        return Err(ConfigError::Invalid {
            name,
            source: anyhow::anyhow!("value must be between 0 and 100"),
        });
    }
    Ok(parsed)
}

fn parse_f64(name: &'static str, default: f64) -> Result<f64, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<f64>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_bool(name: &'static str, default: bool) -> Result<bool, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    match raw.to_lowercase().as_str() {
        "true" | "1" | "yes" | "on" => Ok(true),
        "false" | "0" | "no" | "off" => Ok(false),
        _ => Err(ConfigError::Invalid {
            name,
            source: anyhow::anyhow!("invalid boolean value: {raw}"),
        }),
    }
}

fn parse_csv(name: &'static str, default: &str) -> std::vec::Vec<std::string::String> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.split(',')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn set_env(name: &str, value: &str) {
        // SAFETY: tests run sequentially and assign valid UTF-8 values.
        unsafe {
            env::set_var(name, value);
        }
    }

    fn remove_env(name: &str) {
        // SAFETY: tests run sequentially and clean up deterministic keys.
        unsafe {
            env::remove_var(name);
        }
    }

    fn reset_env() {
        remove_env("RECAP_DB_DSN");
        remove_env("RECAP_WORKER_HTTP_BIND");
        remove_env("LLM_PROMPT_VERSION");
        remove_env("LLM_MAX_CONCURRENCY");
        remove_env("NEWS_CREATOR_BASE_URL");
        remove_env("SUBWORKER_BASE_URL");
        remove_env("ALT_BACKEND_BASE_URL");
        remove_env("ALT_BACKEND_CONNECT_TIMEOUT_MS");
        remove_env("ALT_BACKEND_READ_TIMEOUT_MS");
        remove_env("ALT_BACKEND_TOTAL_TIMEOUT_MS");
        remove_env("HTTP_MAX_RETRIES");
        remove_env("HTTP_BACKOFF_BASE_MS");
        remove_env("HTTP_BACKOFF_CAP_MS");
        remove_env("TAG_LABEL_GRAPH_WINDOW");
        remove_env("TAG_LABEL_GRAPH_TTL_SECONDS");
        remove_env("OTEL_EXPORTER_ENDPOINT");
        remove_env("OTEL_SAMPLING_RATIO");
        remove_env("RECAP_WINDOW_DAYS");
        remove_env("RECAP_GENRES");
        remove_env("LLM_SUMMARY_TIMEOUT_SECS");
    }

    #[test]
    fn from_env_uses_defaults_when_optional_missing() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env(
            "RECAP_DB_DSN",
            "postgres://recap:recap@localhost:5555/recap_db",
        );
        set_env("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
        set_env("SUBWORKER_BASE_URL", "http://localhost:8002/");
        set_env("ALT_BACKEND_BASE_URL", "http://localhost:9000/");

        let config = Config::from_env().expect("config should load");

        assert_eq!(
            config.recap_db_dsn(),
            "postgres://recap:recap@localhost:5555/recap_db"
        );
        assert_eq!(config.llm_prompt_version(), "recap-ja-v2");
        assert_eq!(config.llm_max_concurrency().get(), 1);
        assert_eq!(config.http_bind(), "0.0.0.0:9005".parse().unwrap());
        assert_eq!(config.news_creator_base_url(), "http://localhost:8001/");
        assert_eq!(config.subworker_base_url(), "http://localhost:8002/");
        assert_eq!(config.alt_backend_base_url(), "http://localhost:9000/");
        assert_eq!(
            config.alt_backend_connect_timeout(),
            Duration::from_millis(3000)
        );
        assert_eq!(
            config.alt_backend_read_timeout(),
            Duration::from_millis(20000)
        );
        assert_eq!(
            config.alt_backend_total_timeout(),
            Duration::from_millis(30000)
        );
        assert_eq!(config.http_max_retries(), 3);
        assert_eq!(config.http_backoff_base_ms(), 250);
        assert_eq!(config.http_backoff_cap_ms(), 10000);
        assert!(config.otel_exporter_endpoint().is_none());
        assert!((config.otel_sampling_ratio() - 1.0).abs() < f64::EPSILON);
        assert_eq!(config.recap_window_days(), 7);
        assert_eq!(config.tag_label_graph_window(), "7d");
        assert_eq!(config.tag_label_graph_ttl(), Duration::from_secs(900));
        assert_eq!(
            config.recap_genres(),
            &[
                "ai_data",
                "climate_environment",
                "consumer_products",
                "consumer_tech",
                "culture_arts",
                "cybersecurity",
                "diplomacy_security",
                "economics_macro",
                "education",
                "energy_transition",
                "film_tv",
                "food_cuisine",
                "games_esports",
                "health_medicine",
                "home_living",
                "industry_logistics",
                "internet_platforms",
                "labor_workplace",
                "law_crime",
                "life_science",
                "markets_finance",
                "mobility_automotive",
                "music_audio",
                "politics_government",
                "society_demographics",
                "software_dev",
                "space_astronomy",
                "sports",
                "startups_innovation",
                "travel_places",
            ]
        );
    }

    #[test]
    fn from_env_overrides_values() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env(
            "RECAP_DB_DSN",
            "postgres://recap:recap@localhost:5999/recap_db",
        );
        set_env("RECAP_WORKER_HTTP_BIND", "127.0.0.1:8088");
        set_env("LLM_PROMPT_VERSION", "recap-ja-v3");
        set_env("LLM_MAX_CONCURRENCY", "2");
        set_env("NEWS_CREATOR_BASE_URL", "https://news.example.com/");
        set_env("SUBWORKER_BASE_URL", "https://subworker.example.com/");
        set_env("ALT_BACKEND_BASE_URL", "https://backend.example.com/");
        set_env("ALT_BACKEND_CONNECT_TIMEOUT_MS", "5000");
        set_env("HTTP_MAX_RETRIES", "5");
        set_env("OTEL_EXPORTER_ENDPOINT", "http://otel:4317");
        set_env("RECAP_WINDOW_DAYS", "14");
        set_env("RECAP_GENRES", "ai,tech");
        set_env("TAG_LABEL_GRAPH_WINDOW", "30d");
        set_env("TAG_LABEL_GRAPH_TTL_SECONDS", "600");

        let config = Config::from_env().expect("config should load");

        assert_eq!(
            config.recap_db_dsn(),
            "postgres://recap:recap@localhost:5999/recap_db"
        );
        assert_eq!(config.llm_prompt_version(), "recap-ja-v3");
        assert_eq!(config.llm_max_concurrency().get(), 2);
        assert_eq!(config.http_bind(), "127.0.0.1:8088".parse().unwrap());
        assert_eq!(config.news_creator_base_url(), "https://news.example.com/");
        assert_eq!(
            config.subworker_base_url(),
            "https://subworker.example.com/"
        );
        assert_eq!(
            config.alt_backend_base_url(),
            "https://backend.example.com/"
        );
        assert_eq!(
            config.alt_backend_connect_timeout(),
            Duration::from_millis(5000)
        );
        assert_eq!(config.http_max_retries(), 5);
        assert_eq!(config.otel_exporter_endpoint(), Some("http://otel:4317"));
        assert_eq!(config.recap_window_days(), 14);
        assert_eq!(config.recap_genres(), &["ai", "tech"]);
        assert_eq!(config.tag_label_graph_window(), "30d");
        assert_eq!(config.tag_label_graph_ttl(), Duration::from_secs(600));
    }

    #[test]
    fn from_env_errors_when_required_missing() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
        set_env("SUBWORKER_BASE_URL", "http://localhost:8002/");
        set_env("ALT_BACKEND_BASE_URL", "http://localhost:9000/");

        let error = Config::from_env().expect_err("missing DSN should fail");

        assert!(matches!(error, ConfigError::Missing("RECAP_DB_DSN")));
    }

    #[test]
    fn from_env_errors_when_news_creator_missing() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env(
            "RECAP_DB_DSN",
            "postgres://recap:recap@localhost:5555/recap_db",
        );
        set_env("SUBWORKER_BASE_URL", "http://localhost:8002/");
        set_env("ALT_BACKEND_BASE_URL", "http://localhost:9000/");

        let error = Config::from_env().expect_err("missing news creator should fail");

        assert!(matches!(
            error,
            ConfigError::Missing("NEWS_CREATOR_BASE_URL")
        ));
    }

    #[test]
    fn from_env_errors_when_subworker_missing() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env(
            "RECAP_DB_DSN",
            "postgres://recap:recap@localhost:5555/recap_db",
        );
        set_env("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
        set_env("ALT_BACKEND_BASE_URL", "http://localhost:9000/");

        let error = Config::from_env().expect_err("missing subworker should fail");

        assert!(matches!(error, ConfigError::Missing("SUBWORKER_BASE_URL")));
    }

    #[test]
    fn from_env_errors_when_alt_backend_missing() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env(
            "RECAP_DB_DSN",
            "postgres://recap:recap@localhost:5555/recap_db",
        );
        set_env("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
        set_env("SUBWORKER_BASE_URL", "http://localhost:8002/");

        let error = Config::from_env().expect_err("missing alt backend should fail");

        assert!(matches!(
            error,
            ConfigError::Missing("ALT_BACKEND_BASE_URL")
        ));
    }
}
