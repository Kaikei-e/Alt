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
        let recap_db_dsn = env_var("RECAP_DB_DSN")?;
        let http_bind = parse_socket_addr("RECAP_WORKER_HTTP_BIND", "0.0.0.0:9005")?;
        let llm_prompt_version =
            env::var("LLM_PROMPT_VERSION").unwrap_or_else(|_| "recap-ja-v2".to_string());
        let llm_max_concurrency = parse_non_zero_usize("LLM_MAX_CONCURRENCY", 4)?;
        let news_creator_base_url = env_var("NEWS_CREATOR_BASE_URL")?;
        let subworker_base_url = env_var("SUBWORKER_BASE_URL")?;
        let alt_backend_base_url = env_var("ALT_BACKEND_BASE_URL")?;
        let alt_backend_service_token = env::var("ALT_BACKEND_SERVICE_TOKEN").ok();

        // HTTP timeout settings (defaults based on memo.md recommendations)
        let alt_backend_connect_timeout =
            parse_duration_ms("ALT_BACKEND_CONNECT_TIMEOUT_MS", 3000)?;
        let alt_backend_read_timeout = parse_duration_ms("ALT_BACKEND_READ_TIMEOUT_MS", 20000)?;
        let alt_backend_total_timeout = parse_duration_ms("ALT_BACKEND_TOTAL_TIMEOUT_MS", 30000)?;

        // Retry settings (exponential backoff + jitter)
        let http_max_retries = parse_usize("HTTP_MAX_RETRIES", 3)?;
        let http_backoff_base_ms = parse_u64("HTTP_BACKOFF_BASE_MS", 250)?;
        let http_backoff_cap_ms = parse_u64("HTTP_BACKOFF_CAP_MS", 10000)?;

        // OpenTelemetry settings
        let otel_exporter_endpoint = env::var("OTEL_EXPORTER_ENDPOINT").ok();
        let otel_sampling_ratio = parse_f64("OTEL_SAMPLING_RATIO", 1.0)?;

        // Batch processing settings
        let recap_window_days = parse_u32("RECAP_WINDOW_DAYS", 7)?;
        let recap_genres = parse_csv(
            "RECAP_GENRES",
            "tech,ai,business,science,politics,health,sports,entertainment,world,security,product,design,culture,environment,lifestyle",
        )?;
        let genre_classifier_weights_path = env::var("RECAP_GENRE_MODEL_WEIGHTS").ok();
        let genre_classifier_threshold = parse_f64("RECAP_GENRE_MODEL_THRESHOLD", 0.5)? as f32;

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
}

fn env_var(name: &'static str) -> Result<String, ConfigError> {
    env::var(name).map_err(|_| ConfigError::Missing(name))
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

fn parse_f64(name: &'static str, default: f64) -> Result<f64, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    raw.parse::<f64>().map_err(|error| ConfigError::Invalid {
        name,
        source: anyhow::Error::new(error),
    })
}

fn parse_csv(name: &'static str, default: &str) -> Result<Vec<String>, ConfigError> {
    let raw = env::var(name).unwrap_or_else(|_| default.to_string());
    Ok(raw
        .split(',')
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect())
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
        remove_env("OTEL_EXPORTER_ENDPOINT");
        remove_env("OTEL_SAMPLING_RATIO");
        remove_env("RECAP_WINDOW_DAYS");
        remove_env("RECAP_GENRES");
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
        assert_eq!(config.llm_max_concurrency().get(), 4);
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
        assert_eq!(config.otel_sampling_ratio(), 1.0);
        assert_eq!(config.recap_window_days(), 7);
        assert_eq!(
            config.recap_genres(),
            &[
                "tech", "economy", "ai", "policy", "security", "science", "product", "design",
                "devops", "culture"
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
