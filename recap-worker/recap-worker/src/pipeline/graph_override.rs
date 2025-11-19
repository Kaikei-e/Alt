use serde::Deserialize;
use sqlx::PgPool;
use std::{
    env, fs,
    path::{Path, PathBuf},
};

/// Graph-related tuning overrides loaded from database (latest) or YAML fallback.
#[derive(Debug, Clone, Default, Deserialize, PartialEq)]
#[serde(rename_all = "snake_case")]
pub(crate) struct GraphOverrideSettings {
    pub graph_margin: Option<f32>,
    pub weighted_tie_break_margin: Option<f32>,
    pub tag_confidence_gate: Option<f32>,
    pub boost_threshold: Option<f32>,
    pub tag_count_threshold: Option<usize>,
}

impl GraphOverrideSettings {
    /// Load from database first, fallback to YAML if not found.
    pub(crate) async fn load_with_fallback(
        pool: &PgPool,
    ) -> Result<Self, GraphOverrideError> {
        // Try database first
        if let Ok(Some(config_json)) = crate::store::dao::RecapDao::new(pool.clone())
            .get_latest_worker_config("graph_override")
            .await
        {
            if let Ok(settings) = serde_json::from_value::<GraphOverrideSettings>(config_json) {
                tracing::info!("loaded graph override settings from database");
                return Ok(settings);
            }
        }

        // Fallback to YAML
        tracing::debug!("no database config found, falling back to YAML");
        Self::load_from_env()
    }

    pub(crate) fn load_from_env() -> Result<Self, GraphOverrideError> {
        let path = match env::var("GRAPH_CONFIG") {
            Ok(raw) if !raw.trim().is_empty() => PathBuf::from(raw),
            _ => return Ok(Self::default()),
        };
        Self::load_from_path(&path)
    }

    fn load_from_path(path: &Path) -> Result<Self, GraphOverrideError> {
        let contents = fs::read_to_string(path).map_err(|source| GraphOverrideError::Io {
            path: path.to_path_buf(),
            source,
        })?;
        serde_yaml::from_str(&contents).map_err(|source| GraphOverrideError::Deserialize {
            path: path.to_path_buf(),
            source,
        })
    }
}

#[derive(Debug, thiserror::Error)]
pub(crate) enum GraphOverrideError {
    #[error("failed to read graph config at {path}: {source}")]
    Io {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("failed to parse graph config at {path}: {source}")]
    Deserialize {
        path: PathBuf,
        #[source]
        source: serde_yaml::Error,
    },
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::env;

    fn fixtures_path() -> PathBuf {
        PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("config/graph.local.yaml")
    }

    #[test]
    fn load_from_env_defaults_when_missing() {
        unsafe {
            env::remove_var("GRAPH_CONFIG");
        }
        let overrides = GraphOverrideSettings::load_from_env().expect("should load defaults");
        assert_eq!(overrides, GraphOverrideSettings::default());
    }

    #[test]
    fn load_from_env_reads_yaml() {
        let fixture = fixtures_path();
        unsafe {
            env::set_var("GRAPH_CONFIG", &fixture);
        }
        let overrides = GraphOverrideSettings::load_from_env().expect("should parse fixture");
        unsafe {
            env::remove_var("GRAPH_CONFIG");
        }
        assert_eq!(overrides.graph_margin, Some(0.12));
        assert_eq!(overrides.weighted_tie_break_margin, Some(0.03));
        assert_eq!(overrides.tag_confidence_gate, Some(0.65));
        assert_eq!(overrides.boost_threshold, Some(0.0));
        assert_eq!(overrides.tag_count_threshold, Some(0));
    }

    #[test]
    fn load_from_path_errors_for_missing_file() {
        let missing = fixtures_path().with_file_name("does-not-exist.yaml");
        let err = GraphOverrideSettings::load_from_path(&missing).unwrap_err();
        match err {
            GraphOverrideError::Io { path, .. } => {
                assert!(path.ends_with("does-not-exist.yaml"));
            }
            _ => panic!("expected Io error"),
        }
    }
}
