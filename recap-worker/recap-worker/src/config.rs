use std::{env, net::SocketAddr, num::NonZeroUsize, time::Duration};

use thiserror::Error;

#[cfg(test)]
use once_cell::sync::Lazy;
#[cfg(test)]
pub(crate) static ENV_MUTEX: Lazy<std::sync::Mutex<()>> = Lazy::new(|| std::sync::Mutex::new(()));

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
    genre_classifier_threshold: f32,
    genre_refine_enabled: bool,
    genre_refine_require_tags: bool,
    genre_refine_rollout_pct: u8,
    tag_label_graph_window: String,
    tag_label_graph_ttl: Duration,
    tag_generator_base_url: String,
    tag_generator_service_token: Option<String>,
    tag_generator_connect_timeout: Duration,
    tag_generator_total_timeout: Duration,
    min_documents_per_genre: usize,
    coherence_similarity_threshold: f32,
    recap_pre_refresh_graph_enabled: bool,
    recap_pre_refresh_timeout: Duration,
    llm_summary_timeout: Duration,
    recap_db_max_connections: u32,
    recap_db_min_connections: u32,
    recap_db_acquire_timeout: Duration,
    recap_db_idle_timeout: Duration,
    recap_db_max_lifetime: Duration,
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

impl Config {
    /// 環境変数から Recap Worker の設定値を読み込み、検証する。
    ///
    /// 必須の環境変数が揃っていない場合や、数値／アドレスのパースに失敗した場合はエラーを返す。
    ///
    /// # Errors
    /// `RECAP_DB_DSN` が未設定、もしくは各種値のパースに失敗した場合は [`ConfigError`] を返す。
    pub fn from_env() -> Result<Self, ConfigError> {
        let recap_db_dsn = load_database_dsn()?;
        let http_bind = parse_socket_addr("RECAP_WORKER_HTTP_BIND", "0.0.0.0:9005")?;
        let llm_prompt_version =
            env::var("LLM_PROMPT_VERSION").unwrap_or_else(|_| "recap-ja-v2".to_string());
        let llm_max_concurrency = parse_non_zero_usize("LLM_MAX_CONCURRENCY", 1)?;
        let news_creator_base_url = env_var("NEWS_CREATOR_BASE_URL")?;
        let subworker_base_url = env_var("SUBWORKER_BASE_URL")?;
        let alt_backend_base_url = env_var("ALT_BACKEND_BASE_URL")?;
        let alt_backend_service_token = env_var_optional("ALT_BACKEND_SERVICE_TOKEN");

        let (alt_backend_connect_timeout, alt_backend_read_timeout, alt_backend_total_timeout) =
            load_http_timeout_config()?;
        let (http_max_retries, http_backoff_base_ms, http_backoff_cap_ms) = load_retry_config()?;
        let (otel_exporter_endpoint, otel_sampling_ratio) = load_observability_config()?;
        let (
            recap_window_days,
            recap_genres,
            genre_classifier_weights_path,
            genre_classifier_threshold,
            genre_refine_enabled,
            genre_refine_require_tags,
            genre_refine_rollout_pct,
        ) = load_batch_config()?;
        let (tag_label_graph_window, tag_label_graph_ttl) = load_graph_config()?;
        let (
            tag_generator_base_url,
            tag_generator_service_token,
            tag_generator_connect_timeout,
            tag_generator_total_timeout,
        ) = load_tag_config()?;
        let (min_documents_per_genre, coherence_similarity_threshold) = load_subworker_config()?;
        let (recap_pre_refresh_graph_enabled, recap_pre_refresh_timeout) =
            load_pre_refresh_config()?;
        let llm_summary_timeout = parse_duration_secs("LLM_SUMMARY_TIMEOUT_SECS", 600)?;
        let (
            recap_db_max_connections,
            recap_db_min_connections,
            recap_db_acquire_timeout,
            recap_db_idle_timeout,
            recap_db_max_lifetime,
        ) = load_db_pool_config()?;

        Ok(Self {
            http_bind,
            llm_max_concurrency,
            llm_prompt_version,
            recap_db_dsn,
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
            recap_window_days,
            recap_genres,
            genre_classifier_weights_path,
            genre_classifier_threshold,
            genre_refine_enabled,
            genre_refine_require_tags,
            genre_refine_rollout_pct,
            tag_label_graph_window,
            tag_label_graph_ttl,
            tag_generator_base_url,
            tag_generator_service_token,
            tag_generator_connect_timeout,
            tag_generator_total_timeout,
            min_documents_per_genre,
            coherence_similarity_threshold,
            recap_pre_refresh_graph_enabled,
            recap_pre_refresh_timeout,
            llm_summary_timeout,
            recap_db_max_connections,
            recap_db_min_connections,
            recap_db_acquire_timeout,
            recap_db_idle_timeout,
            recap_db_max_lifetime,
        })
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
    pub fn genre_classifier_threshold(&self) -> f32 {
        self.genre_classifier_threshold
    }

    #[must_use]
    pub fn genre_refine_enabled(&self) -> bool {
        self.genre_refine_enabled
    }

    #[must_use]
    pub fn genre_refine_require_tags(&self) -> bool {
        self.genre_refine_require_tags
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

    pub fn recap_pre_refresh_graph_enabled(&self) -> bool {
        self.recap_pre_refresh_graph_enabled
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

type BatchConfig = (u32, Vec<String>, Option<String>, f32, bool, bool, u8);

fn load_batch_config() -> Result<BatchConfig, ConfigError> {
    let window_days = parse_u32("RECAP_WINDOW_DAYS", 7)?;
    let genres = parse_csv(
        "RECAP_GENRES",
        "ai,tech,business,politics,health,sports,science,entertainment,world,security,product,design,culture,environment,lifestyle,art_culture,developer_insights,pro_it_media,consumer_tech,global_politics,environment_policy,society_justice,travel_lifestyle,security_policy,business_finance,ai_research,ai_policy,games_puzzles,other",
    );
    let classifier_weights_path = env_var_optional("RECAP_GENRE_MODEL_WEIGHTS");
    let classifier_threshold = parse_f64("RECAP_GENRE_MODEL_THRESHOLD", 0.5)? as f32;
    let refine_enabled = parse_bool("RECAP_GENRE_REFINE_ENABLED", false)?;
    let refine_require_tags = parse_bool("RECAP_GENRE_REFINE_REQUIRE_TAGS", true)?;
    let refine_rollout_pct = parse_percentage("RECAP_GENRE_REFINE_ROLLOUT_PERCENT", 100)?;
    Ok((
        window_days,
        genres,
        classifier_weights_path,
        classifier_threshold,
        refine_enabled,
        refine_require_tags,
        refine_rollout_pct,
    ))
}

fn load_graph_config() -> Result<(String, Duration), ConfigError> {
    let window = env::var("TAG_LABEL_GRAPH_WINDOW").unwrap_or_else(|_| "7d".to_string());
    let ttl = Duration::from_secs(parse_u64("TAG_LABEL_GRAPH_TTL_SECONDS", 900)?);
    Ok((window, ttl))
}

fn load_tag_config() -> Result<(String, Option<String>, Duration, Duration), ConfigError> {
    let base_url = env::var("TAG_GENERATOR_BASE_URL")
        .unwrap_or_else(|_| "http://tag-generator:9400".to_string());
    let service_token = env_var_optional("TAG_GENERATOR_SERVICE_TOKEN");
    let connect_timeout = parse_duration_ms("TAG_GENERATOR_CONNECT_TIMEOUT_MS", 3000)?;
    let total_timeout = parse_duration_ms("TAG_GENERATOR_TOTAL_TIMEOUT_MS", 30000)?;
    Ok((base_url, service_token, connect_timeout, total_timeout))
}

fn load_subworker_config() -> Result<(usize, f32), ConfigError> {
    let min_documents = parse_usize("RECAP_MIN_DOCUMENTS_PER_GENRE", 10)?;
    let similarity_threshold = parse_f64("RECAP_COHERENCE_SIMILARITY_THRESHOLD", 0.5)? as f32;
    Ok((min_documents, similarity_threshold))
}

fn load_pre_refresh_config() -> Result<(bool, Duration), ConfigError> {
    let enabled = parse_bool("RECAP_PRE_REFRESH_GRAPH_ENABLED", true)?;
    let timeout = parse_duration_secs("RECAP_PRE_REFRESH_TIMEOUT_SECS", 300)?;
    Ok((enabled, timeout))
}

fn load_db_pool_config() -> Result<(u32, u32, Duration, Duration, Duration), ConfigError> {
    let max_connections = parse_u32("RECAP_DB_MAX_CONNECTIONS", 50)?;
    let min_connections = parse_u32("RECAP_DB_MIN_CONNECTIONS", 5)?;
    let acquire_timeout = parse_duration_secs("RECAP_DB_ACQUIRE_TIMEOUT_SECS", 60)?;
    let idle_timeout = parse_duration_secs("RECAP_DB_IDLE_TIMEOUT_SECS", 600)?;
    let max_lifetime = parse_duration_secs("RECAP_DB_MAX_LIFETIME_SECS", 1800)?;
    Ok((
        max_connections,
        min_connections,
        acquire_timeout,
        idle_timeout,
        max_lifetime,
    ))
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
                "ai",
                "tech",
                "business",
                "politics",
                "health",
                "sports",
                "science",
                "entertainment",
                "world",
                "security",
                "product",
                "design",
                "culture",
                "environment",
                "lifestyle",
                "art_culture",
                "developer_insights",
                "pro_it_media",
                "consumer_tech",
                "global_politics",
                "environment_policy",
                "society_justice",
                "travel_lifestyle",
                "security_policy",
                "business_finance",
                "ai_research",
                "ai_policy",
                "games_puzzles",
                "other"
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
