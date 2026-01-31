//! Scoring functions for genre refinement.

use std::collections::HashMap;

use rustc_hash::FxHashMap;

use crate::pipeline::genre::GenreCandidate;
use crate::pipeline::tag_signal::TagSignal;

use super::cache::TagLabelGraphCache;
use super::config::RefineConfig;

/// 候補拡大の最小重み閾値（タグの合計重みがこの値以上のジャンルのみ候補に追加）
pub(crate) const CANDIDATE_EXPANSION_MIN_WEIGHT: f32 = 0.3;

/// Compute entropy from tag signals.
pub(crate) fn compute_entropy(tags: &[TagSignal]) -> f32 {
    if tags.is_empty() {
        return 0.0;
    }
    let total: f32 = tags.iter().map(|t| t.confidence.max(0.0)).sum();
    if total <= f32::EPSILON {
        return 0.0;
    }

    tags.iter()
        .filter_map(|tag| {
            let p = (tag.confidence.max(0.0)) / total;
            if p <= 0.0 {
                None
            } else {
                Some(-p * (p.ln() / std::f32::consts::LN_2))
            }
        })
        .sum::<f32>()
}

/// Compute graph boosts for candidates based on tag signals.
pub(crate) fn compute_graph_boosts(
    graph: &TagLabelGraphCache,
    candidates: &[(String, &GenreCandidate)],
    tags: &[TagSignal],
) -> FxHashMap<String, f32> {
    let mut boosts: FxHashMap<String, f32> = FxHashMap::default();

    // Debug: Log tag matching attempts (only for first candidate to avoid log spam)
    let mut logged_debug = false;

    for (normalized, candidate) in candidates {
        let mut candidate_boost = 0.0;
        let mut matched_tags = Vec::new();
        let mut unmatched_tags = Vec::new();

        for tag in tags {
            let tag_norm = normalize(&tag.label);
            // Normalize genre name for matching (graph stores lowercase)
            let genre_norm = normalize(&candidate.name);
            let weight = graph.weight(&genre_norm, &tag_norm).unwrap_or(0.0);
            let contribution = weight * tag.confidence;
            candidate_boost += contribution;

            if !logged_debug {
                if weight > 0.0 {
                    matched_tags.push((tag.label.clone(), tag_norm, weight, tag.confidence));
                } else {
                    unmatched_tags.push((tag.label.clone(), tag_norm));
                }
            }
        }

        if !logged_debug && !tags.is_empty() {
            if matched_tags.is_empty() && !unmatched_tags.is_empty() {
                // Debug: Check if graph is empty
                let (graph_genre_count, graph_total_tags, graph_sample_tags) = graph.debug_stats();
                tracing::debug!(
                    genre = %normalized,
                    graph_genre_count = graph_genre_count,
                    graph_total_tags = graph_total_tags,
                    graph_sample_tags = ?graph_sample_tags,
                    "no graph boost matches found for genre"
                );
            } else if !matched_tags.is_empty() {
                tracing::debug!(
                    genre = %normalized,
                    matched_count = matched_tags.len(),
                    "graph boost matches found"
                );
            }
            logged_debug = true;
        }

        boosts.insert(normalized.clone(), candidate_boost);
    }
    boosts
}

/// Normalize a string value for comparison.
pub(crate) fn normalize(value: &str) -> String {
    value.trim().to_lowercase()
}

/// タグから候補ジャンルを拡大する
pub(crate) fn expand_candidates_from_tags(
    graph: &TagLabelGraphCache,
    existing_candidates: &[GenreCandidate],
    tags: &[TagSignal],
) -> Vec<GenreCandidate> {
    // 既存候補のジャンル名を正規化してセットに保存
    let existing_genres: std::collections::HashSet<String> = existing_candidates
        .iter()
        .map(|c| normalize(&c.name))
        .collect();

    // タグからジャンルへの重みマッピングを集計
    let mut genre_weights: FxHashMap<String, f32> = FxHashMap::default();

    for tag in tags {
        let tag_norm = normalize(&tag.label);
        let genres = graph.genres_for_tag(&tag_norm);

        for (genre, weight) in genres {
            // タグのconfidenceを考慮して重みを計算
            let contribution = weight * tag.confidence;
            *genre_weights.entry(genre).or_insert(0.0) += contribution;
        }
    }

    // 閾値を超えるジャンルを候補に追加
    let mut expanded: Vec<GenreCandidate> = existing_candidates.to_vec();
    let mut added_genres = Vec::new();
    let mut skipped_below_threshold = Vec::new();

    for (genre, total_weight) in genre_weights {
        // 既存候補に含まれている場合はスキップ
        if existing_genres.contains(&genre) {
            continue;
        }

        // 閾値を超えている場合のみ追加
        if total_weight >= CANDIDATE_EXPANSION_MIN_WEIGHT {
            // 新しい候補を作成（初期スコアは0.0、confidenceは重みベース）
            expanded.push(GenreCandidate {
                name: genre.clone(),
                classifier_confidence: total_weight.min(1.0),
                score: 0.0,
                keyword_support: 0, // タグベースの拡張なのでkeyword_supportは0
            });
            added_genres.push((genre, total_weight));
        } else {
            skipped_below_threshold.push((genre, total_weight));
        }
    }

    if !added_genres.is_empty() || !skipped_below_threshold.is_empty() {
        tracing::debug!(
            added_count = added_genres.len(),
            skipped_count = skipped_below_threshold.len(),
            "candidate expansion from tags"
        );
    }

    expanded
}

/// Find a tag-consistency winner among candidates.
pub(crate) fn tag_consistency_winner(
    config: &RefineConfig,
    candidates: &[(String, &GenreCandidate)],
    tags: &[TagSignal],
) -> Option<(String, f32)> {
    let mut matched: Vec<(String, f32)> = Vec::new();
    for tag in tags {
        if tag.confidence < config.tag_confidence_gate {
            continue;
        }
        let normalized_tag = normalize(&tag.label);
        if let Some((candidate_name, _)) = candidates
            .iter()
            .find(|(normalized, _)| normalized == &normalized_tag)
        {
            matched.push((candidate_name.clone(), tag.confidence));
        }
    }

    if matched.is_empty() {
        return None;
    }

    let unique: FxHashMap<String, f32> =
        matched
            .into_iter()
            .fold(FxHashMap::default(), |mut acc, (name, conf)| {
                let entry = acc.entry(name).or_insert(0.0f32);
                *entry = entry.max(conf);
                acc
            });

    if unique.len() == 1 {
        let (name, confidence) = unique.into_iter().next().unwrap();
        return Some((name, confidence));
    }

    None
}

/// タグ一貫性スコアを計算する（強化版）。
/// タグの信頼度、出現頻度、ジャンル名との部分一致も考慮する。
pub(crate) fn tag_consistency_score(
    config: &RefineConfig,
    candidate_name: &str,
    tags: &[TagSignal],
) -> f32 {
    let normalized_candidate = normalize(candidate_name);
    let mut score = 0.0;
    let mut match_count = 0;

    for tag in tags {
        if tag.confidence < config.tag_confidence_gate {
            continue;
        }
        let normalized_tag = normalize(&tag.label);

        // 完全一致
        if normalized_tag == normalized_candidate {
            score += tag.confidence;
            match_count += 1;
        }
        // 部分一致（タグがジャンル名を含む、またはジャンル名がタグを含む）
        else if normalized_tag.contains(&normalized_candidate)
            || normalized_candidate.contains(&normalized_tag)
        {
            score += tag.confidence * 0.5;
            match_count += 1;
        }
    }

    // 複数のタグが一致する場合、信頼度の合計を返す（最大1.0にクランプ）
    if match_count > 0 {
        score.min(1.0)
    } else {
        0.0
    }
}

/// 重み付きスコアリングでタイブレークを決定する。
/// 各候補に対して以下の要素を重み付きで評価：
/// - keyword_support (重み: 0.25)
/// - classifier_confidence (重み: 0.30)
/// - graph_boost (重み: 0.25)
/// - tag_consistency_score (重み: 0.20)
pub(crate) fn compute_weighted_tie_break_score(
    _config: &RefineConfig,
    candidate: &GenreCandidate,
    graph_boost: f32,
    tag_consistency: f32,
) -> f32 {
    // 各要素を正規化
    let keyword_score = (candidate.keyword_support as f32 / 10.0).min(1.0); // 最大10で正規化
    let classifier_score = candidate.classifier_confidence.clamp(0.0, 1.0);
    let graph_score = graph_boost.clamp(0.0, 1.0);
    let tag_score = tag_consistency.clamp(0.0, 1.0);

    // 重み付き合計
    keyword_score * 0.25 + classifier_score * 0.30 + graph_score * 0.25 + tag_score * 0.20
}

/// Convert FxHashMap boosts to owned HashMap.
pub(crate) fn graph_boosts_to_owned(boosts: &FxHashMap<String, f32>) -> HashMap<String, f32> {
    boosts
        .iter()
        .map(|(k, v)| (k.clone(), *v))
        .collect::<HashMap<_, _>>()
}
