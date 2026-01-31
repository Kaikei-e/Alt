//! Configuration for genre refinement.

/// Refine用設定値。
#[derive(Debug, Clone)]
pub(crate) struct RefineConfig {
    pub(crate) require_tags: bool,
    pub(crate) tag_confidence_gate: f32,
    pub(crate) graph_margin: f32,
    pub(crate) boost_threshold: f32,
    pub(crate) tag_count_threshold: usize,
    pub(crate) weighted_tie_break_margin: f32,
    pub(crate) fallback_genre: String,
}

impl RefineConfig {
    #[must_use]
    pub(crate) fn new(require_tags: bool) -> Self {
        Self {
            require_tags,
            tag_confidence_gate: 0.6,
            graph_margin: 0.15,
            boost_threshold: 0.0,
            tag_count_threshold: 0,
            weighted_tie_break_margin: 0.05,
            fallback_genre: "other".to_string(),
        }
    }
}
