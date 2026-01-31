//! Refine strategy types for genre refinement.

use std::collections::HashMap;

/// Refine戦略。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum RefineStrategy {
    TagConsistency,
    GraphBoost,
    WeightedScore,
    #[allow(dead_code)]
    LlmTieBreak, // 後方互換性のため保持
    FallbackOther,
    CoarseOnly,
}

/// Refine結果。
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct RefineOutcome {
    pub(crate) final_genre: String,
    pub(crate) confidence: f32,
    pub(crate) strategy: RefineStrategy,
    pub(crate) llm_trace_id: Option<String>,
    pub(crate) graph_boosts: HashMap<String, f32>,
}

impl RefineOutcome {
    #[must_use]
    pub(crate) fn new(
        final_genre: impl Into<String>,
        confidence: f32,
        strategy: RefineStrategy,
        llm_trace_id: Option<String>,
        graph_boosts: HashMap<String, f32>,
    ) -> Self {
        Self {
            final_genre: final_genre.into(),
            confidence,
            strategy,
            llm_trace_id,
            graph_boosts,
        }
    }

    #[must_use]
    pub(crate) fn graph_boosts(&self) -> &HashMap<String, f32> {
        &self.graph_boosts
    }
}
