//! Assignment trimming for the select stage.

use std::collections::HashMap;

use crate::pipeline::genre::{GenreAssignment, GenreBundle};

use super::scoring::calculate_score;

/// Trim assignments based on max/min document thresholds.
pub(crate) fn trim_assignments(
    max_articles_per_genre: usize,
    min_documents_per_genre: usize,
    bundle: GenreBundle,
    thresholds: &HashMap<String, usize>,
) -> Vec<GenreAssignment> {
    let mut selected = Vec::new();

    // Group by genre
    let mut by_genre: HashMap<String, Vec<GenreAssignment>> = HashMap::new();
    for assignment in bundle.assignments {
        let g = assignment
            .primary_genre()
            .map_or("other".to_string(), ToString::to_string);
        by_genre.entry(g).or_default().push(assignment);
    }

    for (genre, mut candidates) in by_genre {
        // Sort by score descending
        candidates.sort_by(|a, b| {
            let score_a = calculate_score(a);
            let score_b = calculate_score(b);
            score_b
                .partial_cmp(&score_a)
                .unwrap_or(std::cmp::Ordering::Equal)
        });

        // Determine limits
        let total_in_genre = candidates.len() as f64;
        let computed_f64 = (total_in_genre * 0.1).ceil().max(0.0);
        // computed_f64は既に非負（.max(0.0)で保証）かつusize::MAX以下であることを確認済み
        let dynamic_min = if computed_f64 <= usize::MAX as f64 && computed_f64 >= 0.0 {
            // f64をfloor()で整数に丸めてからi64に変換し、usizeに変換（符号損失を回避）
            let value_i64 = computed_f64.floor() as i64;
            usize::try_from(value_i64).unwrap_or(0)
        } else {
            usize::MAX
        };

        let base_min = thresholds
            .get(&genre)
            .copied()
            .unwrap_or(min_documents_per_genre);
        let effective_min = base_min.max(dynamic_min);
        let adjusted_max = max_articles_per_genre.max(effective_min * 2);

        // Group by source for Round-Robin
        let mut by_source: HashMap<String, std::collections::VecDeque<GenreAssignment>> =
            HashMap::new();
        for c in candidates {
            // simple hostname extraction or just use source_url as is
            let source = c
                .article
                .source_url
                .as_deref()
                .unwrap_or("unknown")
                .to_string();
            by_source.entry(source).or_default().push_back(c);
        }

        // Round-robin selection
        let mut genre_selected = Vec::new();
        let mut sources: Vec<String> = by_source.keys().cloned().collect();
        // Sort sources to be deterministic? Or random?
        // Deterministic is better. Sort by best article score in that source?
        // For now just sort by name for stability.
        sources.sort();

        while genre_selected.len() < adjusted_max && !by_source.is_empty() {
            let mut articles_picked_this_round = 0;
            let mut empty_sources = Vec::new();

            for source in &sources {
                if let Some(deque) = by_source.get_mut(source) {
                    if let Some(article) = deque.pop_front() {
                        genre_selected.push(article);
                        articles_picked_this_round += 1;
                    }
                    if deque.is_empty() {
                        empty_sources.push(source.clone());
                    }
                }
                if genre_selected.len() >= adjusted_max {
                    break;
                }
            }

            // Cleanup empty sources
            for s in empty_sources {
                by_source.remove(&s);
            }

            // Remove keys from sources list effectively?
            // We just rebuild sources list or ignore missing ones?
            // Efficiency: sources list is small (dozens).
            sources.retain(|s| by_source.contains_key(s));

            if articles_picked_this_round == 0 {
                break;
            }
        }
        selected.append(&mut genre_selected);
    }
    selected
}
