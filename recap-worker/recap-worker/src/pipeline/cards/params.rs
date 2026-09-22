//! Cards pipeline parameters and defaults.

use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap};

pub const DEFAULT_PARAMS_VERSION: &str = "cards-v0.1";
pub const DEFAULT_THRESHOLD: f32 = 0.78;
pub const DEFAULT_LINKAGE: &str = "average";
pub const DEFAULT_TIME_DECAY_PER_DAY: f32 = 0.02;
pub const DEFAULT_MIN_CLUSTER_SIZE: usize = 1;
pub const DEFAULT_ALPHA: f32 = 0.5;
pub const DEFAULT_THETA_NOVELTY: f32 = 0.80;
pub const DEFAULT_NEAR_DUP_THRESHOLD: f32 = 0.95;
pub const DEFAULT_RECENCY_TAU_DAYS: f32 = 10.0;
pub const DEFAULT_EXPECTED_EMBED_MODEL: &str = "bge-m3";
pub const DEFAULT_EXPECTED_EMBED_DIM: usize = 1024;
pub const DEFAULT_TAU_A: f32 = 0.55;
pub const DEFAULT_GENRE_TAGGING: bool = true;
pub const DEFAULT_GENRE_MIN_CONFIDENCE: f32 = 0.5;
/// Default concurrency for calling coarse classifier during genre tagging.
pub const DEFAULT_GENRE_CONCURRENCY: usize = 8;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct CardsParams {
    pub threshold: f32,
    pub linkage: String,
    pub time_decay_per_day: f32,
    pub min_cluster_size: usize,
    pub min_cluster_size_by_language: HashMap<String, usize>,
    pub params_version: String,
    pub alpha: f32,
    pub theta_novelty: f32,
    pub near_dup_threshold: f32,
    pub recency_tau_days: f32,
    pub expected_embed_model: String,
    pub expected_embed_dim: usize,
    pub tau_a: f32,
    pub genre_tagging: bool,
    pub genre_min_confidence: f32,
    /// Concurrency bound for classifying candidate item genres via the subworker.
    pub genre_concurrency: usize,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub overrides: BTreeMap<String, String>,
}

impl Default for CardsParams {
    fn default() -> Self {
        let mut min_cluster_size_by_language = HashMap::new();
        min_cluster_size_by_language.insert("ja".to_string(), 1);
        min_cluster_size_by_language.insert("en".to_string(), 1);

        Self {
            threshold: DEFAULT_THRESHOLD,
            linkage: DEFAULT_LINKAGE.to_string(),
            time_decay_per_day: DEFAULT_TIME_DECAY_PER_DAY,
            min_cluster_size: DEFAULT_MIN_CLUSTER_SIZE,
            min_cluster_size_by_language,
            params_version: DEFAULT_PARAMS_VERSION.to_string(),
            alpha: DEFAULT_ALPHA,
            theta_novelty: DEFAULT_THETA_NOVELTY,
            near_dup_threshold: DEFAULT_NEAR_DUP_THRESHOLD,
            recency_tau_days: DEFAULT_RECENCY_TAU_DAYS,
            expected_embed_model: DEFAULT_EXPECTED_EMBED_MODEL.to_string(),
            expected_embed_dim: DEFAULT_EXPECTED_EMBED_DIM,
            tau_a: DEFAULT_TAU_A,
            genre_tagging: DEFAULT_GENRE_TAGGING,
            genre_min_confidence: DEFAULT_GENRE_MIN_CONFIDENCE,
            genre_concurrency: DEFAULT_GENRE_CONCURRENCY,
            overrides: BTreeMap::new(),
        }
    }
}

fn parse_f32(value: &serde_json::Value, name: &str) -> anyhow::Result<f32> {
    if let Some(n) = value.as_f64() {
        Ok(n as f32)
    } else if let Some(s) = value.as_str() {
        s.parse::<f32>()
            .map_err(|_| anyhow::anyhow!("invalid float string for {name}: {s}"))
    } else {
        anyhow::bail!("{name} must be a number or string")
    }
}

fn parse_usize(value: &serde_json::Value, name: &str) -> anyhow::Result<usize> {
    if let Some(n) = value.as_u64() {
        usize::try_from(n).map_err(|e| anyhow::anyhow!("value too large for usize {name}: {e}"))
    } else if let Some(s) = value.as_str() {
        s.parse::<usize>()
            .map_err(|_| anyhow::anyhow!("invalid integer string for {name}: {s}"))
    } else {
        anyhow::bail!("{name} must be an integer or string")
    }
}

impl CardsParams {
    pub fn version(&self) -> &str {
        &self.params_version
    }

    pub fn with_override(&self, key: &str, value: &serde_json::Value) -> anyhow::Result<Self> {
        let mut cloned = self.clone();
        cloned.apply_override(key, value)?;
        Ok(cloned)
    }

    #[allow(clippy::too_many_lines)]
    pub fn apply_override(&mut self, key: &str, value: &serde_json::Value) -> anyhow::Result<()> {
        let val_str = match key {
            "threshold" => {
                let v = parse_f32(value, "threshold")?;
                self.threshold = v;
                format!("{v}")
            }
            "linkage" => {
                let v = value
                    .as_str()
                    .ok_or_else(|| anyhow::anyhow!("linkage must be a string"))?;
                self.linkage = v.to_string();
                v.to_string()
            }
            "time_decay_per_day" => {
                let v = parse_f32(value, "time_decay_per_day")?;
                self.time_decay_per_day = v;
                format!("{v}")
            }
            "min_cluster_size" => {
                let v = parse_usize(value, "min_cluster_size")?;
                self.min_cluster_size = v;
                v.to_string()
            }
            "alpha" => {
                let v = parse_f32(value, "alpha")?;
                self.alpha = v;
                format!("{v}")
            }
            "theta_novelty" => {
                let v = parse_f32(value, "theta_novelty")?;
                self.theta_novelty = v;
                format!("{v}")
            }
            "near_dup_threshold" => {
                let v = parse_f32(value, "near_dup_threshold")?;
                self.near_dup_threshold = v;
                format!("{v}")
            }
            "recency_tau_days" => {
                let v = parse_f32(value, "recency_tau_days")?;
                self.recency_tau_days = v;
                format!("{v}")
            }
            "expected_embed_model" => {
                let v = value
                    .as_str()
                    .ok_or_else(|| anyhow::anyhow!("expected_embed_model must be a string"))?;
                self.expected_embed_model = v.to_string();
                v.to_string()
            }
            "expected_embed_dim" => {
                let v = parse_usize(value, "expected_embed_dim")?;
                self.expected_embed_dim = v;
                v.to_string()
            }
            "tau_a" => {
                let v = parse_f32(value, "tau_a")?;
                self.tau_a = v;
                format!("{v}")
            }
            "genre_tagging" => {
                let v = if let Some(b) = value.as_bool() {
                    b
                } else if let Some(s) = value.as_str() {
                    s.parse::<bool>().map_err(|_| {
                        anyhow::anyhow!("invalid bool string for genre_tagging: {s}")
                    })?
                } else {
                    anyhow::bail!("genre_tagging must be a bool");
                };
                self.genre_tagging = v;
                v.to_string()
            }
            "genre_min_confidence" => {
                let v = parse_f32(value, "genre_min_confidence")?;
                self.genre_min_confidence = v;
                format!("{v}")
            }
            "genre_concurrency" => {
                let v = parse_usize(value, "genre_concurrency")?;
                self.genre_concurrency = v;
                v.to_string()
            }
            "min_cluster_size_by_language" => {
                let map: HashMap<String, usize> = serde_json::from_value(value.clone())
                    .map_err(|e| anyhow::anyhow!("invalid min_cluster_size_by_language: {e}"))?;
                let mut entries: Vec<_> = map.iter().collect();
                entries.sort_by_key(|e| e.0);
                let formatted = entries
                    .iter()
                    .map(|(k, v)| format!("{k}:{v}"))
                    .collect::<Vec<_>>()
                    .join(";");
                self.min_cluster_size_by_language = map;
                formatted
            }
            "params_version" => {
                let v = value
                    .as_str()
                    .ok_or_else(|| anyhow::anyhow!("params_version must be a string"))?;
                let (base, _) = v.split_once('+').unwrap_or((v, ""));
                self.params_version = base.to_string();
                self.recompute_params_version();
                return Ok(());
            }
            unknown => anyhow::bail!("unknown CardsParams override key: {unknown}"),
        };

        self.overrides.insert(key.to_string(), val_str);
        self.recompute_params_version();
        Ok(())
    }

    fn recompute_params_version(&mut self) {
        let (base, _) = self
            .params_version
            .split_once('+')
            .unwrap_or((&self.params_version, ""));
        if self.overrides.is_empty() {
            self.params_version = base.to_string();
        } else {
            let joined = self
                .overrides
                .iter()
                .map(|(k, v)| format!("{k}={v}"))
                .collect::<Vec<_>>()
                .join(",");
            self.params_version = format!("{base}+{joined}");
        }
    }
}

#[cfg(test)]
#[allow(clippy::float_cmp)]
mod tests {
    use super::*;

    #[test]
    fn test_default_params_values() {
        let params = CardsParams::default();
        assert_eq!(params.params_version, "cards-v0.1");
        assert_eq!(params.alpha, 0.5);
        assert!(params.genre_tagging);
        assert_eq!(params.genre_min_confidence, 0.5);
        assert_eq!(params.genre_concurrency, 8);
        assert_eq!(params.min_cluster_size_by_language.get("ja"), Some(&1));
        assert_eq!(params.min_cluster_size_by_language.get("en"), Some(&1));
    }

    #[test]
    fn test_with_override_updates_field_and_derives_version_label() {
        let params = CardsParams::default();
        let overridden = params
            .with_override("alpha", &serde_json::json!(0.7))
            .expect("valid override");

        assert_eq!(overridden.alpha, 0.7);
        assert_eq!(overridden.params_version, "cards-v0.1+alpha=0.7");
        assert_eq!(overridden.version(), "cards-v0.1+alpha=0.7");
    }

    #[test]
    fn test_multiple_overrides_sorted_deterministically() {
        let mut params = CardsParams::default();
        params
            .apply_override("theta_novelty", &serde_json::json!(0.85))
            .unwrap();
        params
            .apply_override("alpha", &serde_json::json!(0.7))
            .unwrap();

        assert_eq!(params.alpha, 0.7);
        assert_eq!(params.theta_novelty, 0.85);
        assert_eq!(
            params.params_version,
            "cards-v0.1+alpha=0.7,theta_novelty=0.85"
        );
    }

    #[test]
    fn test_params_version_override_recomputes_with_recorded_overrides() {
        let mut params = CardsParams::default();
        params
            .apply_override("alpha", &serde_json::json!(0.7))
            .unwrap();
        assert_eq!(params.params_version, "cards-v0.1+alpha=0.7");

        params
            .apply_override("params_version", &serde_json::json!("cards-v0.2"))
            .unwrap();
        assert_eq!(params.params_version, "cards-v0.2+alpha=0.7");

        params
            .apply_override("genre_concurrency", &serde_json::json!(16))
            .unwrap();
        assert_eq!(params.genre_concurrency, 16);
        assert_eq!(
            params.params_version,
            "cards-v0.2+alpha=0.7,genre_concurrency=16"
        );
    }

    #[test]
    fn test_with_override_rejects_bad_values() {
        let params = CardsParams::default();

        // Bad float
        assert!(
            params
                .with_override("alpha", &serde_json::json!("not_a_float"))
                .is_err()
        );
        assert!(
            params
                .with_override("threshold", &serde_json::json!(true))
                .is_err()
        );

        // Bad integer
        assert!(
            params
                .with_override("min_cluster_size", &serde_json::json!("not_an_int"))
                .is_err()
        );
        assert!(
            params
                .with_override("genre_concurrency", &serde_json::json!(-5))
                .is_err()
        );

        // Bad bool
        assert!(
            params
                .with_override("genre_tagging", &serde_json::json!("not_a_bool"))
                .is_err()
        );
        assert!(
            params
                .with_override("genre_tagging", &serde_json::json!(123))
                .is_err()
        );

        // Bad language map
        assert!(
            params
                .with_override(
                    "min_cluster_size_by_language",
                    &serde_json::json!("not_a_map")
                )
                .is_err()
        );

        // Unknown key
        assert!(
            params
                .with_override("unknown_key", &serde_json::json!(123))
                .is_err()
        );
    }
}
