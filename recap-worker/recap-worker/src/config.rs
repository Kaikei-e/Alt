use std::{env, net::SocketAddr, num::NonZeroUsize};

use thiserror::Error;

#[cfg(test)]
use once_cell::sync::Lazy;
#[cfg(test)]
pub(crate) static ENV_MUTEX: Lazy<std::sync::Mutex<()>> = Lazy::new(|| std::sync::Mutex::new(()));

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Config {
    http_bind: SocketAddr,
    llm_max_concurrency: NonZeroUsize,
    llm_prompt_version: String,
    recap_db_dsn: String,
    news_creator_base_url: String,
    subworker_base_url: String,
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

        Ok(Self {
            http_bind,
            llm_max_concurrency,
            llm_prompt_version,
            recap_db_dsn,
            news_creator_base_url,
            subworker_base_url,
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
    }

    #[test]
    fn from_env_errors_when_required_missing() {
        let _lock = ENV_MUTEX.lock().expect("env mutex");
        reset_env();
        set_env("NEWS_CREATOR_BASE_URL", "http://localhost:8001/");
        set_env("SUBWORKER_BASE_URL", "http://localhost:8002/");

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

        let error = Config::from_env().expect_err("missing subworker should fail");

        assert!(matches!(error, ConfigError::Missing("SUBWORKER_BASE_URL")));
    }
}
