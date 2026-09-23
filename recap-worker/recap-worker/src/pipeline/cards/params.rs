//! Cards pipeline parameters and defaults.

use serde::{Deserialize, Serialize};
use std::collections::{BTreeMap, HashMap, HashSet};

pub const DEFAULT_PARAMS_VERSION: &str = "cards-v0.5";
pub const DEFAULT_THRESHOLD: f32 = 0.65;
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
pub const DEFAULT_AGGREGATOR_HOST_WEIGHT: f32 = 0.5;
pub const DEFAULT_MAX_ARTICLES_PER_HOST: usize = 4;
pub const DEFAULT_JA_RATIO_MIN: f32 = 0.6;

pub fn default_aggregator_hosts() -> Vec<String> {
    vec![
        "dev.to".to_string(),
        "zenn.dev".to_string(),
        "qiita.com".to_string(),
        "medium.com".to_string(),
        "note.com".to_string(),
        "hatenablog.com".to_string(),
    ]
}

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
    pub aggregator_hosts: Vec<String>,
    pub aggregator_host_weight: f32,
    pub max_articles_per_host: usize,
    pub ja_ratio_min: f32,
    #[serde(default, skip_serializing_if = "BTreeMap::is_empty")]
    pub overrides: BTreeMap<String, String>,
}

impl Default for CardsParams {
    fn default() -> Self {
        Self {
            threshold: DEFAULT_THRESHOLD,
            linkage: DEFAULT_LINKAGE.to_string(),
            time_decay_per_day: DEFAULT_TIME_DECAY_PER_DAY,
            min_cluster_size: DEFAULT_MIN_CLUSTER_SIZE,
            min_cluster_size_by_language: HashMap::new(),
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
            aggregator_hosts: default_aggregator_hosts(),
            aggregator_host_weight: DEFAULT_AGGREGATOR_HOST_WEIGHT,
            max_articles_per_host: DEFAULT_MAX_ARTICLES_PER_HOST,
            ja_ratio_min: DEFAULT_JA_RATIO_MIN,
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

fn parse_aggregator_hosts(value: &serde_json::Value) -> anyhow::Result<Vec<String>> {
    let raw_parts: Vec<&str> = if let Some(s) = value.as_str() {
        s.split(',').collect()
    } else if let Some(arr) = value.as_array() {
        let mut parts = Vec::with_capacity(arr.len());
        for item in arr {
            let s = item
                .as_str()
                .ok_or_else(|| anyhow::anyhow!("aggregator_hosts entries must be strings"))?;
            parts.push(s);
        }
        parts
    } else {
        anyhow::bail!("aggregator_hosts must be a comma-separated string or array of strings");
    };

    if raw_parts.is_empty() {
        anyhow::bail!("aggregator_hosts cannot be empty");
    }

    let mut list = Vec::new();
    let mut seen = HashSet::new();

    for raw in raw_parts {
        let trimmed = raw.trim();
        if trimmed.is_empty() {
            anyhow::bail!("aggregator_hosts entry cannot be empty");
        }
        if trimmed.chars().any(char::is_whitespace) {
            anyhow::bail!("aggregator_hosts entry cannot contain whitespace: '{trimmed}'");
        }
        let lower = trimmed.to_ascii_lowercase();
        let stripped = lower.strip_prefix('.').unwrap_or(&lower);
        if stripped.is_empty() {
            anyhow::bail!("aggregator_hosts entry cannot be empty after stripping dot");
        }
        if seen.insert(stripped.to_string()) {
            list.push(stripped.to_string());
        }
    }

    if list.is_empty() {
        anyhow::bail!("aggregator_hosts cannot be empty");
    }

    Ok(list)
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
            "aggregator_host_weight" => {
                let v = parse_f32(value, "aggregator_host_weight")?;
                if !v.is_finite() || v < 0.0 {
                    anyhow::bail!(
                        "invalid aggregator_host_weight '{v}': must be a finite number >= 0"
                    );
                }
                self.aggregator_host_weight = v;
                format!("{v}")
            }
            "aggregator_hosts" => {
                let hosts = parse_aggregator_hosts(value)?;
                let formatted = hosts.join(";");
                self.aggregator_hosts = hosts;
                formatted
            }
            "max_articles_per_host" => {
                let v = if let Some(i) = value.as_i64() {
                    if i < 1 {
                        anyhow::bail!("invalid max_articles_per_host '{i}': must be >= 1");
                    }
                    usize::try_from(i).map_err(|e| {
                        anyhow::anyhow!("value too large for max_articles_per_host: {e}")
                    })?
                } else if let Some(u) = value.as_u64() {
                    if u < 1 {
                        anyhow::bail!("invalid max_articles_per_host '{u}': must be >= 1");
                    }
                    usize::try_from(u).map_err(|e| {
                        anyhow::anyhow!("value too large for max_articles_per_host: {e}")
                    })?
                } else if let Some(s) = value.as_str() {
                    if let Ok(i) = s.parse::<i64>() {
                        if i < 1 {
                            anyhow::bail!("invalid max_articles_per_host '{s}': must be >= 1");
                        }
                        usize::try_from(i).map_err(|e| {
                            anyhow::anyhow!("value too large for max_articles_per_host: {e}")
                        })?
                    } else {
                        anyhow::bail!(
                            "invalid max_articles_per_host '{s}': must be an integer >= 1"
                        );
                    }
                } else {
                    anyhow::bail!(
                        "invalid max_articles_per_host '{value}': must be an integer >= 1"
                    );
                };
                self.max_articles_per_host = v;
                v.to_string()
            }
            "ja_ratio_min" => {
                let v = parse_f32(value, "ja_ratio_min")?;
                if !v.is_finite() || !(0.0..=1.0).contains(&v) {
                    anyhow::bail!(
                        "invalid ja_ratio_min '{v}': must be a finite number between 0.0 and 1.0"
                    );
                }
                self.ja_ratio_min = v;
                format!("{v}")
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
        assert_eq!(params.params_version, DEFAULT_PARAMS_VERSION);
        assert_eq!(params.threshold, 0.65);
        assert_eq!(params.alpha, 0.5);
        assert!(params.genre_tagging);
        assert_eq!(params.genre_min_confidence, 0.5);
        assert_eq!(params.genre_concurrency, 8);
        assert!(params.min_cluster_size_by_language.is_empty());
        assert_eq!(params.aggregator_host_weight, 0.5);
        assert_eq!(params.aggregator_hosts, default_aggregator_hosts());
        assert_eq!(params.max_articles_per_host, 4);
        assert_eq!(params.ja_ratio_min, 0.6);
    }

    #[test]
    fn test_with_override_updates_field_and_derives_version_label() {
        let params = CardsParams::default();
        let overridden = params
            .with_override("alpha", &serde_json::json!(0.7))
            .expect("valid override");

        assert_eq!(overridden.alpha, 0.7);
        assert_eq!(
            overridden.params_version,
            format!("{DEFAULT_PARAMS_VERSION}+alpha=0.7")
        );
        assert_eq!(
            overridden.version(),
            format!("{DEFAULT_PARAMS_VERSION}+alpha=0.7")
        );
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
            format!("{DEFAULT_PARAMS_VERSION}+alpha=0.7,theta_novelty=0.85")
        );
    }

    #[test]
    fn test_params_version_override_recomputes_with_recorded_overrides() {
        let mut params = CardsParams::default();
        params
            .apply_override("alpha", &serde_json::json!(0.7))
            .unwrap();
        assert_eq!(
            params.params_version,
            format!("{DEFAULT_PARAMS_VERSION}+alpha=0.7")
        );

        params
            .apply_override("params_version", &serde_json::json!("cards-v0.5"))
            .unwrap();
        assert_eq!(params.params_version, "cards-v0.5+alpha=0.7");

        params
            .apply_override("genre_concurrency", &serde_json::json!(16))
            .unwrap();
        assert_eq!(params.genre_concurrency, 16);
        assert_eq!(
            params.params_version,
            "cards-v0.5+alpha=0.7,genre_concurrency=16"
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

    #[test]
    fn test_aggregator_hosts_and_weight_overrides() {
        let params = CardsParams::default();
        assert_eq!(params.aggregator_host_weight, 0.5);
        assert_eq!(
            params.aggregator_hosts,
            vec![
                "dev.to",
                "zenn.dev",
                "qiita.com",
                "medium.com",
                "note.com",
                "hatenablog.com"
            ]
        );

        // Test aggregator_host_weight override
        let overridden = params
            .with_override("aggregator_host_weight", &serde_json::json!(0.25))
            .expect("valid aggregator_host_weight");
        assert_eq!(overridden.aggregator_host_weight, 0.25);
        assert!(
            overridden
                .params_version
                .contains("aggregator_host_weight=0.25")
        );

        // Test aggregator_hosts override
        let overridden = params
            .with_override("aggregator_hosts", &serde_json::json!("dev.to,zenn.dev"))
            .expect("valid aggregator_hosts");
        assert_eq!(overridden.aggregator_hosts, vec!["dev.to", "zenn.dev"]);
        assert!(
            overridden
                .params_version
                .contains("aggregator_hosts=dev.to;zenn.dev")
        );

        // Case-insensitivity (lower-cased), leading dot stripped, trimming, and deduplication
        let overridden = params
            .with_override(
                "aggregator_hosts",
                &serde_json::json!(" .DEV.TO, .zenn.dev , dev.to "),
            )
            .expect("valid uppercase, dotted, deduped aggregator_hosts");
        assert_eq!(overridden.aggregator_hosts, vec!["dev.to", "zenn.dev"]);
        assert!(
            overridden
                .params_version
                .contains("aggregator_hosts=dev.to;zenn.dev")
        );

        // Internal whitespace rejected
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!("dev .to"))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!("dev to"))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!("dev\tto"))
                .is_err()
        );

        // Empty list entry rejected
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!("dev.to,,zenn.dev"))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!("dev.to,"))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!(",dev.to"))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!(""))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!("."))
                .is_err()
        );
        assert!(
            params
                .with_override("aggregator_hosts", &serde_json::json!(["dev.to", ""]))
                .is_err()
        );
    }

    #[test]
    fn test_aggregator_host_weight_validation() {
        let params = CardsParams::default();

        // 0 accepted (int or float)
        let p0 = params
            .with_override("aggregator_host_weight", &serde_json::json!(0))
            .expect("0 is accepted");
        assert_eq!(p0.aggregator_host_weight, 0.0);

        let p0_float = params
            .with_override("aggregator_host_weight", &serde_json::json!(0.0))
            .expect("0.0 is accepted");
        assert_eq!(p0_float.aggregator_host_weight, 0.0);

        // -1 rejected
        let err_neg = params
            .with_override("aggregator_host_weight", &serde_json::json!(-1))
            .unwrap_err();
        let msg_neg = err_neg.to_string();
        assert!(msg_neg.contains("aggregator_host_weight"));
        assert!(msg_neg.contains("-1"));

        // NaN rejected
        let err_nan = params
            .with_override("aggregator_host_weight", &serde_json::json!("NaN"))
            .unwrap_err();
        let msg_nan = err_nan.to_string();
        assert!(msg_nan.contains("aggregator_host_weight"));
        assert!(msg_nan.contains("NaN"));

        // inf rejected
        let err_inf = params
            .with_override("aggregator_host_weight", &serde_json::json!("inf"))
            .unwrap_err();
        let msg_inf = err_inf.to_string();
        assert!(msg_inf.contains("aggregator_host_weight"));
        assert!(msg_inf.contains("inf"));
    }

    #[test]
    fn test_max_articles_per_host_validation() {
        let params = CardsParams::default();

        // 4 accepted (default)
        assert_eq!(params.max_articles_per_host, 4);

        // 8 accepted via override
        let p8 = params
            .with_override("max_articles_per_host", &serde_json::json!(8))
            .expect("8 is accepted");
        assert_eq!(p8.max_articles_per_host, 8);
        assert!(p8.params_version.contains("max_articles_per_host=8"));

        let p8_str = params
            .with_override("max_articles_per_host", &serde_json::json!("8"))
            .expect("string '8' is accepted");
        assert_eq!(p8_str.max_articles_per_host, 8);

        // 0 rejected (naming key and value)
        let err0 = params
            .with_override("max_articles_per_host", &serde_json::json!(0))
            .unwrap_err();
        let msg0 = err0.to_string();
        assert!(msg0.contains("max_articles_per_host"));
        assert!(msg0.contains("'0'"));

        let err0_str = params
            .with_override("max_articles_per_host", &serde_json::json!("0"))
            .unwrap_err();
        let msg0_str = err0_str.to_string();
        assert!(msg0_str.contains("max_articles_per_host"));
        assert!(msg0_str.contains("'0'"));

        // -1 rejected (naming key and value)
        let err_neg = params
            .with_override("max_articles_per_host", &serde_json::json!(-1))
            .unwrap_err();
        let msg_neg = err_neg.to_string();
        assert!(msg_neg.contains("max_articles_per_host"));
        assert!(msg_neg.contains("-1"));

        let err_neg_str = params
            .with_override("max_articles_per_host", &serde_json::json!("-1"))
            .unwrap_err();
        let msg_neg_str = err_neg_str.to_string();
        assert!(msg_neg_str.contains("max_articles_per_host"));
        assert!(msg_neg_str.contains("-1"));

        // abc rejected (naming key and value)
        let err_abc = params
            .with_override("max_articles_per_host", &serde_json::json!("abc"))
            .unwrap_err();
        let msg_abc = err_abc.to_string();
        assert!(msg_abc.contains("max_articles_per_host"));
        assert!(msg_abc.contains("abc"));
    }

    #[test]
    fn test_ja_ratio_min_validation() {
        let params = CardsParams::default();

        let overridden = params
            .with_override("ja_ratio_min", &serde_json::json!(0.75))
            .expect("valid ja_ratio_min");
        assert_eq!(overridden.ja_ratio_min, 0.75);
        assert!(overridden.params_version.contains("ja_ratio_min=0.75"));

        let zero = params
            .with_override("ja_ratio_min", &serde_json::json!(0.0))
            .expect("0.0 is valid");
        assert_eq!(zero.ja_ratio_min, 0.0);

        let one = params
            .with_override("ja_ratio_min", &serde_json::json!(1.0))
            .expect("1.0 is valid");
        assert_eq!(one.ja_ratio_min, 1.0);

        let err_neg = params
            .with_override("ja_ratio_min", &serde_json::json!(-0.1))
            .unwrap_err();
        assert!(err_neg.to_string().contains("invalid ja_ratio_min"));

        let err_large = params
            .with_override("ja_ratio_min", &serde_json::json!(1.1))
            .unwrap_err();
        assert!(err_large.to_string().contains("invalid ja_ratio_min"));

        let err_nan = params
            .with_override("ja_ratio_min", &serde_json::json!("NaN"))
            .unwrap_err();
        assert!(err_nan.to_string().contains("invalid ja_ratio_min"));

        let err_inf = params
            .with_override("ja_ratio_min", &serde_json::json!("inf"))
            .unwrap_err();
        assert!(err_inf.to_string().contains("invalid ja_ratio_min"));

        let err_str = params
            .with_override("ja_ratio_min", &serde_json::json!("abc"))
            .unwrap_err();
        assert!(err_str.to_string().contains("invalid float string"));
    }
}
