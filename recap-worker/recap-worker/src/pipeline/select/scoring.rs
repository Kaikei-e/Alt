//! Scoring functions for the select stage.

use chrono::Utc;

use crate::pipeline::genre::GenreAssignment;

/// Calculate a score for an assignment based on confidence, freshness, and tags.
pub(crate) fn calculate_score(assignment: &GenreAssignment) -> f32 {
    let Some(primary) = assignment.primary_genre() else {
        return 0.0;
    };

    // 1. Classification Confidence
    let keyword_component =
        assignment.genre_scores.get(primary).copied().unwrap_or(0) as f32 / 100.0;
    let classifier_component = assignment
        .genre_confidence
        .get(primary)
        .copied()
        .unwrap_or(keyword_component);
    let base_confidence = classifier_component.max(keyword_component);

    // 2. Freshness (Exponential decay)
    // age in hours. If unknown, assume 24h (neutral) or 0 (fresh). Let's assume 24h.
    let age_hours = if let Some(published_at) = assignment.article.published_at {
        (Utc::now() - published_at).num_hours().max(0) as f32
    } else {
        24.0
    };
    // Decay score: exp(-0.01 * hours) -> decreases slowly.
    // 24h -> 0.78, 48h -> 0.61, 1 week -> 0.18
    // Let's use a milder decay: exp(-0.005 * hours) => 24h -> 0.88, 1 week -> 0.43
    let freshness_score = (-0.005 * age_hours).exp();

    // 3. Tag Match Score
    // Normalize overlap count. 5+ tags = 1.0
    let tag_score = (assignment.feature_profile.tag_overlap_count as f32 / 5.0).min(1.0);

    // Weighted Sum
    // Weights: Confidence 0.5, Freshness 0.3, Tags 0.2

    (base_confidence * 0.5) + (freshness_score * 0.3) + (tag_score * 0.2)
}
