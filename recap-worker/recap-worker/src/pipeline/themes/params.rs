//! Theme pipeline parameters and defaults.

use serde::{Deserialize, Serialize};

pub const DEFAULT_PARAMS_VERSION: &str = "themes-v0.1";
pub const DEFAULT_THRESHOLD: f32 = 0.65;
pub const DEFAULT_LINKAGE: &str = "average";
pub const DEFAULT_TIME_DECAY_PER_DAY: f32 = 0.0;
pub const DEFAULT_THETA_CONTINUATION: f32 = 0.80;
pub const DEFAULT_MIN_THEME_ARTICLES: usize = 3;
pub const DEFAULT_MAX_GENERATIONS_PER_RUN: usize = 5;
pub const DEFAULT_MAX_ITEMS_PER_GENERATION: usize = 8;
pub const DEFAULT_EXPECTED_EMBED_MODEL: &str = "bge-m3";
pub const DEFAULT_EXPECTED_EMBED_DIM: usize = 1024;
pub const DEFAULT_TAU_A: f32 = 0.55;

pub const MAX_TITLE_CHARS: usize = 120;
pub const MAX_LEDE_CHARS: usize = 400;
pub const MAX_PREV_KEY_POINTS: usize = 7;
pub const MAX_PREV_OPEN_QUESTIONS: usize = 4;
pub const MAX_PREV_ENTRY_CHARS: usize = 200;
pub const MAX_FAILURE_NOTES: usize = 5;
pub const MAX_FAILURE_NOTE_CHARS: usize = 120;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct ThemesParams {
    pub params_version: String,
    pub threshold: f32,
    pub linkage: String,
    pub time_decay_per_day: f32,
    pub theta_continuation: f32,
    pub min_theme_articles: usize,
    pub max_generations_per_run: usize,
    pub max_items_per_generation: usize,
    pub expected_embed_model: String,
    pub expected_embed_dim: usize,
    pub tau_a: f32,
}

impl Default for ThemesParams {
    fn default() -> Self {
        Self {
            params_version: DEFAULT_PARAMS_VERSION.to_string(),
            threshold: DEFAULT_THRESHOLD,
            linkage: DEFAULT_LINKAGE.to_string(),
            time_decay_per_day: DEFAULT_TIME_DECAY_PER_DAY,
            theta_continuation: DEFAULT_THETA_CONTINUATION,
            min_theme_articles: DEFAULT_MIN_THEME_ARTICLES,
            max_generations_per_run: DEFAULT_MAX_GENERATIONS_PER_RUN,
            max_items_per_generation: DEFAULT_MAX_ITEMS_PER_GENERATION,
            expected_embed_model: DEFAULT_EXPECTED_EMBED_MODEL.to_string(),
            expected_embed_dim: DEFAULT_EXPECTED_EMBED_DIM,
            tau_a: DEFAULT_TAU_A,
        }
    }
}

impl ThemesParams {
    #[must_use]
    pub fn version(&self) -> &str {
        &self.params_version
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_params_match_expected_values() {
        let params = ThemesParams::default();
        assert_eq!(params.version(), "themes-v0.1");
        assert!((params.threshold - 0.65).abs() < f32::EPSILON);
        assert_eq!(params.linkage, "average");
        assert!(params.time_decay_per_day.abs() < f32::EPSILON);
        assert!((params.theta_continuation - 0.80).abs() < f32::EPSILON);
        assert_eq!(params.min_theme_articles, 3);
        assert_eq!(params.max_generations_per_run, 5);
        assert_eq!(params.max_items_per_generation, 8);
        assert_eq!(params.expected_embed_model, "bge-m3");
        assert_eq!(params.expected_embed_dim, 1024);
        assert!((params.tau_a - 0.55).abs() < f32::EPSILON);
    }
}
