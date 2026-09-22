//! Pure evaluation metrics calculations and reporting for topic cards.
//!
//! Evaluates candidate cluster selection (Precision@k, nDCG@k with binary gains)
//! and topic card quality (0/1/2 ratings, defect flags, execution stats).

use crate::store::dao::cards::{
    RecapCard, RecapCardCandidate, RecapCardJobStats, RecapCardRating, RecapStoryJudgment,
};
use serde::{Deserialize, Serialize};
use std::collections::{HashMap, HashSet};
use std::fmt::Write;
use uuid::Uuid;

/// Selection metrics for top-k candidate clusters.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct SelectionMetrics {
    pub k: usize,
    pub precision_at_k: f64,
    pub ndcg_at_k: f64,
    pub top_count: usize,
    pub not_top_count: usize,
    pub noise_count: usize,
    pub unjudged_count: usize,
    pub total_evaluated: usize,
}

/// Quality metrics for generated topic cards.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct QualityMetrics {
    pub total_cards: usize,
    pub total_rated: usize,
    pub score_0_count: usize,
    pub score_1_count: usize,
    pub score_2_count: usize,
    pub unrated_count: usize,
    pub score_ge_1_pct: f64,
    pub defect_flags: HashMap<String, usize>,
    pub hallucination_or_wrong_source_pct: f64,
    pub filler_pct: f64,
}

/// Pipeline execution timings and dropped metrics.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct PipelineExecutionMetrics {
    pub items_fetched: i32,
    pub items_after_noise: i32,
    pub items_after_dedup: i32,
    pub clusters: i32,
    pub candidates: i32,
    pub cards_selected: i32,
    pub cards_dropped: serde_json::Value,
    pub total_cards_dropped: i32,
    pub drop_rate: f64,
    pub is_degraded: bool,
    pub embed_ms: i64,
    pub cluster_ms: i64,
    pub llm_ms: i64,
    pub total_ms: i64,
}

/// Drop and degraded metrics computed across jobs.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct DropMetrics {
    pub total_cards_dropped: i32,
    pub total_cards_selected: i32,
    pub drop_rate: f64,
    pub degraded_jobs_count: usize,
    pub total_jobs_count: usize,
    pub degraded_share: f64,
}

/// Consolidated evaluation report.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
pub struct EvalReport {
    pub window_id: Uuid,
    pub selection: SelectionMetrics,
    pub quality: QualityMetrics,
    pub execution: Option<PipelineExecutionMetrics>,
    pub drop_metrics: Option<DropMetrics>,
}

impl EvalReport {
    /// Format report as pretty JSON string.
    pub fn to_json(&self) -> serde_json::Result<String> {
        serde_json::to_string_pretty(self)
    }

    /// Format report as human-readable Markdown.
    pub fn to_markdown(&self) -> String {
        let mut out = String::new();
        let _ = writeln!(out, "# Evaluation Report: Window `{}`\n", self.window_id);

        format_selection_markdown(&mut out, &self.selection);
        format_quality_markdown(&mut out, &self.quality);
        if let Some(exec) = &self.execution {
            format_execution_markdown(&mut out, exec);
        }
        if let Some(drop) = &self.drop_metrics {
            format_drop_markdown(&mut out, drop);
        }

        out
    }
}

fn format_selection_markdown(out: &mut String, selection: &SelectionMetrics) {
    let _ = writeln!(out, "## 1. Topic Selection Metrics\n");
    let _ = writeln!(out, "- **k**: {}", selection.k);
    let _ = writeln!(
        out,
        "- **Precision@{}**: {:.4}",
        selection.k, selection.precision_at_k
    );
    let _ = writeln!(
        out,
        "- **nDCG@{}**: {:.4}",
        selection.k, selection.ndcg_at_k
    );
    let _ = writeln!(
        out,
        "- **Judgments (top-{})**: {} top, {} not_top, {} noise, {} unjudged (total: {})\n",
        selection.k,
        selection.top_count,
        selection.not_top_count,
        selection.noise_count,
        selection.unjudged_count,
        selection.total_evaluated
    );
}

fn format_quality_markdown(out: &mut String, quality: &QualityMetrics) {
    let _ = writeln!(out, "## 2. Card Quality Metrics\n");
    let _ = writeln!(out, "- **Total Cards**: {}", quality.total_cards);
    let _ = writeln!(out, "- **Rated Cards**: {}", quality.total_rated);
    let _ = writeln!(out, "- **Unrated Cards**: {}", quality.unrated_count);
    let _ = writeln!(
        out,
        "- **Score Distribution**: Score 2: {}, Score 1: {}, Score 0: {}",
        quality.score_2_count, quality.score_1_count, quality.score_0_count
    );
    let _ = writeln!(out, "- **Score >= 1 Rate**: {:.1}%", quality.score_ge_1_pct);
    let _ = writeln!(
        out,
        "- **Hallucination / Wrong Source Rate**: {:.1}%",
        quality.hallucination_or_wrong_source_pct
    );
    let _ = writeln!(out, "- **Filler Rate**: {:.1}%", quality.filler_pct);

    if !quality.defect_flags.is_empty() {
        let _ = writeln!(out, "\n### Defect Flags\n");
        let mut sorted_flags: Vec<_> = quality.defect_flags.iter().collect();
        sorted_flags.sort_by_key(|(k, _)| *k);
        for (flag, count) in sorted_flags {
            let _ = writeln!(out, "- `{flag}`: {count}");
        }
    }
    out.push('\n');
}

fn format_execution_markdown(out: &mut String, exec: &PipelineExecutionMetrics) {
    let _ = writeln!(out, "## 3. Pipeline Execution Stats\n");
    let _ = writeln!(out, "- **Items Fetched**: {}", exec.items_fetched);
    let _ = writeln!(
        out,
        "- **After Noise Filtering**: {}",
        exec.items_after_noise
    );
    let _ = writeln!(out, "- **After Dedup**: {}", exec.items_after_dedup);
    let _ = writeln!(out, "- **Clusters**: {}", exec.clusters);
    let _ = writeln!(out, "- **Candidates**: {}", exec.candidates);
    let _ = writeln!(out, "- **Cards Selected**: {}", exec.cards_selected);
    let _ = writeln!(
        out,
        "- **Cards Dropped (Total)**: {}",
        exec.total_cards_dropped
    );
    let _ = writeln!(out, "- **Drop Rate**: {:.1}%", exec.drop_rate * 100.0);
    let _ = writeln!(
        out,
        "- **Degraded (< 5 cards)**: {}",
        if exec.is_degraded { "Yes" } else { "No" },
    );
    let _ = writeln!(
        out,
        "- **Timings (ms)**: Total {} ms (Embed: {} ms, Cluster: {} ms, LLM: {} ms)",
        exec.total_ms, exec.embed_ms, exec.cluster_ms, exec.llm_ms
    );
    if !exec.cards_dropped.is_null() && exec.cards_dropped != serde_json::json!({}) {
        let _ = writeln!(
            out,
            "- **Cards Dropped by Reason**: `{}`",
            exec.cards_dropped
        );
    }
    out.push('\n');
}

fn format_drop_markdown(out: &mut String, drop: &DropMetrics) {
    out.push_str("## 4. Aggregate Drop & Degraded Metrics\n\n");
    let _ = writeln!(
        out,
        "- **Total Cards Dropped**: {}",
        drop.total_cards_dropped
    );
    let _ = writeln!(
        out,
        "- **Total Cards Selected**: {}",
        drop.total_cards_selected
    );
    let _ = writeln!(
        out,
        "- **Aggregate Drop Rate**: {:.1}%",
        drop.drop_rate * 100.0
    );
    let _ = writeln!(
        out,
        "- **Degraded Jobs (< 5 cards)**: {} / {} ({:.1}%)\n",
        drop.degraded_jobs_count,
        drop.total_jobs_count,
        drop.degraded_share * 100.0
    );
}

/// Compute topic candidate selection metrics (Precision@k, nDCG@k with binary gains and E25 cluster dedup).
pub fn compute_selection_metrics(
    candidates: &[RecapCardCandidate],
    judgments: &[RecapStoryJudgment],
    k: usize,
) -> SelectionMetrics {
    let mut judgment_map: HashMap<&str, &str> = HashMap::new();
    for j in judgments {
        judgment_map.insert(&j.cluster_fingerprint, &j.decision);
    }

    let mut sorted_candidates = candidates.to_vec();
    sorted_candidates.sort_by_key(|c| c.rank);

    let k_eff = k.max(1);
    let top_k_candidates: Vec<&RecapCardCandidate> = sorted_candidates.iter().take(k_eff).collect();

    let mut seen_clusters = HashSet::new();
    let mut dcg = 0.0;
    let mut relevant_count: u32 = 0;
    let mut top_count = 0;
    let mut not_top_count = 0;
    let mut noise_count = 0;
    let mut unjudged_count = 0;

    for (idx, c) in top_k_candidates.iter().enumerate() {
        let is_first_seen = seen_clusters.insert(c.cluster_fingerprint.as_str());
        let decision = judgment_map.get(c.cluster_fingerprint.as_str()).copied();

        match decision {
            Some("top") => {
                top_count += 1;
                if is_first_seen {
                    relevant_count += 1;
                    let discount = (idx as f64 + 2.0).log2();
                    dcg += 1.0 / discount;
                }
            }
            Some("not_top") => {
                not_top_count += 1;
            }
            Some("noise") => {
                noise_count += 1;
            }
            _ => {
                unjudged_count += 1;
            }
        }
    }

    let precision_at_k = f64::from(relevant_count) / k_eff as f64;

    let total_top_in_window = judgments
        .iter()
        .filter(|j| j.decision == "top")
        .map(|j| j.cluster_fingerprint.as_str())
        .collect::<HashSet<_>>()
        .len();

    let ideal_hits = total_top_in_window.min(k_eff);
    let mut idcg = 0.0;
    for idx in 0..ideal_hits {
        let discount = (idx as f64 + 2.0).log2();
        idcg += 1.0 / discount;
    }

    let ndcg_at_k = if idcg > 0.0 { dcg / idcg } else { 0.0 };

    SelectionMetrics {
        k: k_eff,
        precision_at_k,
        ndcg_at_k,
        top_count,
        not_top_count,
        noise_count,
        unjudged_count,
        total_evaluated: top_k_candidates.len(),
    }
}

/// Compute card quality metrics based on 0/1/2 ratings and defect flags.
pub fn compute_quality_metrics(cards: &[RecapCard], ratings: &[RecapCardRating]) -> QualityMetrics {
    let mut rating_map: HashMap<Uuid, &RecapCardRating> = HashMap::new();
    for r in ratings {
        rating_map.insert(r.card_id, r);
    }

    let total_cards = cards.len();
    let mut score_0_count = 0;
    let mut score_1_count = 0;
    let mut score_2_count = 0;
    let mut unrated_count = 0;
    let mut defect_flags: HashMap<String, usize> = HashMap::new();

    for card in cards {
        match rating_map.get(&card.id) {
            Some(r) => {
                match r.score {
                    0 => score_0_count += 1,
                    1 => score_1_count += 1,
                    2 => score_2_count += 1,
                    _ => {}
                }
                for flag in &r.flags {
                    *defect_flags.entry(flag.clone()).or_insert(0) += 1;
                }
            }
            None => {
                unrated_count += 1;
            }
        }
    }

    let total_rated = score_0_count + score_1_count + score_2_count;
    let score_ge_1_pct = if total_rated > 0 {
        ((score_1_count + score_2_count) as f64 / total_rated as f64) * 100.0
    } else {
        0.0
    };

    let hallucination_or_wrong_source_count =
        defect_flags.get("hallucination").copied().unwrap_or(0)
            + defect_flags.get("wrong_source").copied().unwrap_or(0);

    let hallucination_or_wrong_source_pct = if total_rated > 0 {
        (hallucination_or_wrong_source_count as f64 / total_rated as f64) * 100.0
    } else {
        0.0
    };

    let filler_count = defect_flags.get("filler").copied().unwrap_or(0);
    let filler_pct = if total_rated > 0 {
        (filler_count as f64 / total_rated as f64) * 100.0
    } else {
        0.0
    };

    QualityMetrics {
        total_cards,
        total_rated,
        score_0_count,
        score_1_count,
        score_2_count,
        unrated_count,
        score_ge_1_pct,
        defect_flags,
        hallucination_or_wrong_source_pct,
        filler_pct,
    }
}

/// Count dropped cards from cards_dropped jsonb (object with reason counts or numeric value).
pub fn count_dropped_cards(cards_dropped: &serde_json::Value) -> i32 {
    match cards_dropped {
        serde_json::Value::Object(map) => map
            .values()
            .filter_map(|v| v.as_i64().or_else(|| v.as_u64().map(|u| u as i64)))
            .sum::<i64>() as i32,
        serde_json::Value::Number(n) => n.as_i64().unwrap_or(0) as i32,
        _ => 0,
    }
}

/// Compute aggregate drop and degraded metrics across jobs.
pub fn compute_drop_metrics(
    job_stats: &[&RecapCardJobStats],
    cards_per_job: &[usize],
) -> DropMetrics {
    let mut total_dropped = 0;
    let mut total_selected = 0;
    for s in job_stats {
        total_dropped += count_dropped_cards(&s.cards_dropped);
        total_selected += s.cards_selected;
    }
    let total_generated = total_dropped + total_selected;
    let drop_rate = if total_generated > 0 {
        f64::from(total_dropped) / f64::from(total_generated)
    } else {
        0.0
    };

    let total_jobs = cards_per_job.len();
    let degraded_jobs = cards_per_job.iter().filter(|&&count| count < 5).count();
    let degraded_share = if total_jobs > 0 {
        degraded_jobs as f64 / total_jobs as f64
    } else {
        0.0
    };

    DropMetrics {
        total_cards_dropped: total_dropped,
        total_cards_selected: total_selected,
        drop_rate,
        degraded_jobs_count: degraded_jobs,
        total_jobs_count: total_jobs,
        degraded_share,
    }
}

/// Compute pipeline execution stats for a job.
pub fn compute_execution_metrics(
    stats: Option<&RecapCardJobStats>,
    cards: &[RecapCard],
) -> Option<PipelineExecutionMetrics> {
    stats.map(|s| {
        let total_cards_dropped = count_dropped_cards(&s.cards_dropped);
        let total_generated = total_cards_dropped + s.cards_selected;
        let drop_rate = if total_generated > 0 {
            f64::from(total_cards_dropped) / f64::from(total_generated)
        } else {
            0.0
        };
        let is_degraded = cards.len() < 5;

        PipelineExecutionMetrics {
            items_fetched: s.items_fetched,
            items_after_noise: s.items_after_noise,
            items_after_dedup: s.items_after_dedup,
            clusters: s.clusters,
            candidates: s.candidates,
            cards_selected: s.cards_selected,
            cards_dropped: s.cards_dropped.clone(),
            total_cards_dropped,
            drop_rate,
            is_degraded,
            embed_ms: s.embed_ms,
            cluster_ms: s.cluster_ms,
            llm_ms: s.llm_ms,
            total_ms: s.total_ms,
        }
    })
}

/// Compute complete evaluation report given candidate clusters, judgments, generated cards, ratings, and optional aggregate drop metrics.
#[allow(clippy::too_many_arguments)]
pub fn compute_eval_report(
    window_id: Uuid,
    candidates: &[RecapCardCandidate],
    judgments: &[RecapStoryJudgment],
    cards: &[RecapCard],
    ratings: &[RecapCardRating],
    stats: Option<&RecapCardJobStats>,
    drop_metrics: Option<DropMetrics>,
    k: usize,
) -> EvalReport {
    EvalReport {
        window_id,
        selection: compute_selection_metrics(candidates, judgments, k),
        quality: compute_quality_metrics(cards, ratings),
        execution: compute_execution_metrics(stats, cards),
        drop_metrics,
    }
}

/// Fetch data from DB and compute evaluation report for a window.
pub async fn generate_window_report(
    pool: &sqlx::PgPool,
    window_id: Uuid,
    k: usize,
) -> Result<EvalReport, String> {
    let window = match crate::store::dao::cards::CardsDaoOps::get_eval_window(pool, window_id).await
    {
        Ok(Some(w)) => w,
        Ok(None) => return Err(format!("eval window '{window_id}' not found")),
        Err(e) => return Err(format!("failed to fetch eval window: {e}")),
    };

    let candidates =
        crate::store::dao::cards::CardsDaoOps::get_candidates_for_window(pool, window_id)
            .await
            .map_err(|e| format!("failed to fetch candidates: {e}"))?;

    let judgments =
        crate::store::dao::cards::CardsDaoOps::latest_judgments_for_window(pool, window_id)
            .await
            .map_err(|e| format!("failed to fetch judgments: {e}"))?;

    let cards =
        crate::store::dao::cards::CardsDaoOps::get_cards_for_job(pool, window.snapshot_job_id)
            .await
            .map_err(|e| format!("failed to fetch cards: {e}"))?;

    let ratings =
        crate::store::dao::cards::CardsDaoOps::latest_rating_per_card(pool, window.snapshot_job_id)
            .await
            .map_err(|e| format!("failed to fetch ratings: {e}"))?;

    let stats = crate::store::dao::cards::CardsDaoOps::get_job_stats(pool, window.snapshot_job_id)
        .await
        .map_err(|e| format!("failed to fetch job stats: {e}"))?;

    // Compute aggregate drop metrics across cards jobs in the DB
    let drop_metrics = match crate::store::dao::cards::CardsDaoOps::list_cards_jobs(pool, 100).await
    {
        Ok(jobs) if !jobs.is_empty() => {
            let cards_per_job: Vec<usize> = jobs
                .iter()
                .map(|j| usize::try_from(j.card_count.max(0)).unwrap_or(0))
                .collect();
            let mut job_stats_owned = Vec::new();
            for j in &jobs {
                if let Ok(Some(s)) =
                    crate::store::dao::cards::CardsDaoOps::get_job_stats(pool, j.job_id).await
                {
                    job_stats_owned.push(s);
                }
            }
            let job_stats_refs: Vec<&RecapCardJobStats> = job_stats_owned.iter().collect();
            Some(compute_drop_metrics(&job_stats_refs, &cards_per_job))
        }
        _ => stats
            .as_ref()
            .map(|s| compute_drop_metrics(&[s], &[cards.len()])),
    };

    Ok(compute_eval_report(
        window_id,
        &candidates,
        &judgments,
        &cards,
        &ratings,
        stats.as_ref(),
        drop_metrics,
        k,
    ))
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    #[test]
    fn test_compute_eval_report_precision_and_ndcg() {
        let window_id = Uuid::new_v4();
        let job_id = Uuid::new_v4();

        let candidates = vec![
            RecapCardCandidate {
                id: Uuid::new_v4(),
                job_id,
                rank: 1,
                cluster_fingerprint: "fp-1".to_string(),
                size: 3,
                domains: serde_json::json!([{"host": "example.com", "count": 2}]),
                items: serde_json::json!([]),
                scores: serde_json::json!({}),
                centroid: None,
                created_at: Utc::now(),
            },
            RecapCardCandidate {
                id: Uuid::new_v4(),
                job_id,
                rank: 2,
                cluster_fingerprint: "fp-2".to_string(),
                size: 2,
                domains: serde_json::json!([{"host": "example.com", "count": 1}]),
                items: serde_json::json!([]),
                scores: serde_json::json!({}),
                centroid: None,
                created_at: Utc::now(),
            },
            RecapCardCandidate {
                id: Uuid::new_v4(),
                job_id,
                rank: 3,
                cluster_fingerprint: "fp-3".to_string(),
                size: 1,
                domains: serde_json::json!([]),
                items: serde_json::json!([]),
                scores: serde_json::json!({}),
                centroid: None,
                created_at: Utc::now(),
            },
        ];

        let judgments = vec![
            RecapStoryJudgment {
                id: Uuid::new_v4(),
                window_id,
                cluster_fingerprint: "fp-1".to_string(),
                decision: "top".to_string(),
                rated_at: Utc::now(),
            },
            RecapStoryJudgment {
                id: Uuid::new_v4(),
                window_id,
                cluster_fingerprint: "fp-2".to_string(),
                decision: "not_top".to_string(),
                rated_at: Utc::now(),
            },
            RecapStoryJudgment {
                id: Uuid::new_v4(),
                window_id,
                cluster_fingerprint: "fp-3".to_string(),
                decision: "top".to_string(),
                rated_at: Utc::now(),
            },
        ];

        let report =
            compute_eval_report(window_id, &candidates, &judgments, &[], &[], None, None, 3);

        // Top 3 has fp-1 (top), fp-2 (not_top), fp-3 (top)
        // Precision@3 = 2 / 3
        assert!((report.selection.precision_at_k - (2.0 / 3.0)).abs() < 1e-4);

        // DCG@3:
        // rank 1: rel=1 -> 1 / log2(2) = 1.0
        // rank 2: rel=0 -> 0
        // rank 3: rel=1 -> 1 / log2(4) = 0.5
        // DCG = 1.5
        // IDCG@3 (total 2 top items in window):
        // rank 1: 1.0, rank 2: 1 / log2(3)
        let ideal_dcg = 1.0 + 1.0 / 3.0_f64.log2();
        let actual_dcg = 1.0 + 0.5;
        let want_ndcg = actual_dcg / ideal_dcg;
        assert!((report.selection.ndcg_at_k - want_ndcg).abs() < 1e-4);
    }

    fn assert_approx_eq(actual: f64, expected: f64) {
        assert!(
            (actual - expected).abs() < 1e-6,
            "expected {expected}, got {actual}"
        );
    }

    fn make_test_card(id: Uuid, job_id: Uuid, rank: i32, headline: &str) -> RecapCard {
        RecapCard {
            id,
            job_id,
            rank,
            story_id: Uuid::new_v4(),
            continues_card_id: None,
            merged_from: None,
            headline_ja: headline.to_string(),
            what_ja: format!("What {rank}"),
            why_ja: None,
            genre: None,
            member_feed_ids: vec![],
            sources: serde_json::json!([]),
            centroid: None,
            scores: serde_json::json!({}),
            gates: serde_json::json!({}),
            generation: serde_json::json!({}),
            created_at: Utc::now(),
        }
    }

    #[test]
    fn test_duplicate_clusters_receive_zero_credit() {
        let window_id = Uuid::new_v4();
        let job_id = Uuid::new_v4();

        // 2 candidates from same cluster "fp-1"
        let candidates = vec![
            RecapCardCandidate {
                id: Uuid::new_v4(),
                job_id,
                rank: 1,
                cluster_fingerprint: "fp-1".to_string(),
                size: 3,
                domains: serde_json::json!([]),
                items: serde_json::json!([]),
                scores: serde_json::json!({}),
                centroid: None,
                created_at: Utc::now(),
            },
            RecapCardCandidate {
                id: Uuid::new_v4(),
                job_id,
                rank: 2,
                cluster_fingerprint: "fp-1".to_string(),
                size: 3,
                domains: serde_json::json!([]),
                items: serde_json::json!([]),
                scores: serde_json::json!({}),
                centroid: None,
                created_at: Utc::now(),
            },
        ];

        let judgments = vec![RecapStoryJudgment {
            id: Uuid::new_v4(),
            window_id,
            cluster_fingerprint: "fp-1".to_string(),
            decision: "top".to_string(),
            rated_at: Utc::now(),
        }];

        let report =
            compute_eval_report(window_id, &candidates, &judgments, &[], &[], None, None, 2);

        // Only 1 relevant unique cluster in top 2 -> precision@2 = 1 / 2 = 0.5
        assert_approx_eq(report.selection.precision_at_k, 0.5);
        // DCG@2 has gain only at rank 1 (1.0), rank 2 gets 0 credit.
        // IDCG@2 (total 1 top item) has gain at rank 1 (1.0).
        // nDCG@2 = 1.0 / 1.0 = 1.0
        assert_approx_eq(report.selection.ndcg_at_k, 1.0);
    }

    #[test]
    fn test_card_quality_metrics() {
        let window_id = Uuid::new_v4();
        let job_id = Uuid::new_v4();
        let card1_id = Uuid::new_v4();
        let card2_id = Uuid::new_v4();
        let card3_id = Uuid::new_v4();

        let cards = vec![
            make_test_card(card1_id, job_id, 1, "Headline 1"),
            make_test_card(card2_id, job_id, 2, "Headline 2"),
            make_test_card(card3_id, job_id, 3, "Headline 3"),
        ];

        let ratings = vec![
            RecapCardRating {
                id: Uuid::new_v4(),
                card_id: card1_id,
                score: 2,
                flags: vec![],
                comment: Some("good card".to_string()),
                rated_at: Utc::now(),
            },
            RecapCardRating {
                id: Uuid::new_v4(),
                card_id: card2_id,
                score: 0,
                flags: vec!["hallucination".to_string(), "filler".to_string()],
                comment: None,
                rated_at: Utc::now(),
            },
            // card3 is unrated
        ];

        let stats = RecapCardJobStats {
            job_id,
            items_fetched: 100,
            items_after_noise: 80,
            items_after_dedup: 60,
            clusters: 15,
            candidates: 10,
            cards_selected: 3,
            cards_dropped: serde_json::json!({"g3_attribution": 1}),
            embed_ms: 1200,
            cluster_ms: 300,
            llm_ms: 5000,
            total_ms: 6500,
            params_version: "v1".to_string(),
            created_at: Utc::now(),
        };

        let report = compute_eval_report(
            window_id,
            &[],
            &[],
            &cards,
            &ratings,
            Some(&stats),
            None,
            10,
        );

        assert_eq!(report.quality.total_cards, 3);
        assert_eq!(report.quality.total_rated, 2);
        assert_eq!(report.quality.unrated_count, 1);
        assert_eq!(report.quality.score_2_count, 1);
        assert_eq!(report.quality.score_0_count, 1);
        assert_approx_eq(report.quality.score_ge_1_pct, 50.0);
        assert_approx_eq(report.quality.hallucination_or_wrong_source_pct, 50.0);
        assert_approx_eq(report.quality.filler_pct, 50.0);
    }

    #[test]
    fn test_card_quality_and_drop_formatting() {
        let window_id = Uuid::new_v4();
        let job_id = Uuid::new_v4();
        let card1_id = Uuid::new_v4();

        let cards = vec![make_test_card(card1_id, job_id, 1, "Headline 1")];
        let ratings = vec![RecapCardRating {
            id: Uuid::new_v4(),
            card_id: card1_id,
            score: 2,
            flags: vec!["hallucination".to_string()],
            comment: Some("ok".to_string()),
            rated_at: Utc::now(),
        }];

        // 2 dropped, 2 kept -> drop_rate = 2 / (2 + 2) = 0.5
        let stats = RecapCardJobStats {
            job_id,
            items_fetched: 100,
            items_after_noise: 80,
            items_after_dedup: 60,
            clusters: 15,
            candidates: 10,
            cards_selected: 2,
            cards_dropped: serde_json::json!({"g3_attribution": 1, "g4_filler": 1}),
            embed_ms: 1200,
            cluster_ms: 300,
            llm_ms: 5000,
            total_ms: 6500,
            params_version: "v1".to_string(),
            created_at: Utc::now(),
        };

        let drop_metrics = compute_drop_metrics(&[&stats], &[1]);
        let report = compute_eval_report(
            window_id,
            &[],
            &[],
            &cards,
            &ratings,
            Some(&stats),
            Some(drop_metrics),
            10,
        );

        // JSON format
        let json_str = report.to_json().expect("to_json");
        assert!(json_str.contains("\"total_cards\": 1"));
        assert!(json_str.contains("\"drop_rate\": 0.5"));
        assert!(json_str.contains("\"is_degraded\": true"));
        assert!(json_str.contains("\"drop_metrics\":"));

        // Markdown format
        let md_str = report.to_markdown();
        assert!(md_str.contains("# Evaluation Report"));
        assert!(md_str.contains("Total Cards"));
        assert!(md_str.contains("hallucination"));
        assert!(md_str.contains("Drop Rate"));
        assert!(md_str.contains("50.0%"));
        assert!(md_str.contains("Degraded (< 5 cards)"));
        assert!(md_str.contains("Yes"));
        assert!(md_str.contains("Aggregate Drop & Degraded Metrics"));
    }

    #[test]
    fn test_compute_drop_metrics_with_synthetic_rows() {
        let stats1 = RecapCardJobStats {
            job_id: Uuid::new_v4(),
            items_fetched: 200,
            items_after_noise: 150,
            items_after_dedup: 120,
            clusters: 20,
            candidates: 15,
            cards_selected: 10,
            cards_dropped: serde_json::json!({"g3_attribution": 2, "g4_filler": 1}),
            embed_ms: 1000,
            cluster_ms: 500,
            llm_ms: 4000,
            total_ms: 5500,
            params_version: "v1".to_string(),
            created_at: Utc::now(),
        };

        let stats2 = RecapCardJobStats {
            job_id: Uuid::new_v4(),
            items_fetched: 150,
            items_after_noise: 100,
            items_after_dedup: 80,
            clusters: 12,
            candidates: 8,
            cards_selected: 6,
            cards_dropped: serde_json::json!({"g3_attribution": 3}),
            embed_ms: 800,
            cluster_ms: 400,
            llm_ms: 3000,
            total_ms: 4200,
            params_version: "v1".to_string(),
            created_at: Utc::now(),
        };

        // Job 1 has 7 cards (>= 5, not degraded); Job 2 has 3 cards (< 5, degraded)
        let drop_metrics = compute_drop_metrics(&[&stats1, &stats2], &[7, 3]);

        // Job 1 dropped 3, Job 2 dropped 3 -> total dropped = 6
        assert_eq!(drop_metrics.total_cards_dropped, 6);
        // Total selected: 10 + 6 = 16
        assert_eq!(drop_metrics.total_cards_selected, 16);
        // Drop rate = 6 / (16 + 6) = 6 / 22 = 0.272727...
        assert_approx_eq(drop_metrics.drop_rate, 6.0 / 22.0);
        // 1 of 2 jobs degraded -> degraded_share = 0.5
        assert_eq!(drop_metrics.degraded_jobs_count, 1);
        assert_eq!(drop_metrics.total_jobs_count, 2);
        assert_approx_eq(drop_metrics.degraded_share, 0.5);
    }

    #[test]
    fn test_drop_rate_calculation_proportions() {
        // 2 dropped / 2 kept -> 0.5
        let stats_2_kept = RecapCardJobStats {
            job_id: Uuid::new_v4(),
            items_fetched: 10,
            items_after_noise: 10,
            items_after_dedup: 10,
            clusters: 5,
            candidates: 4,
            cards_selected: 2,
            cards_dropped: serde_json::json!({"g3_attribution": 1, "g4_filler": 1}),
            embed_ms: 0,
            cluster_ms: 0,
            llm_ms: 0,
            total_ms: 0,
            params_version: "v1".to_string(),
            created_at: Utc::now(),
        };
        let exec_2_kept = compute_execution_metrics(Some(&stats_2_kept), &[]).unwrap();
        assert_approx_eq(exec_2_kept.drop_rate, 0.5);

        // 2 dropped / 4 kept -> 0.33
        let stats_4_kept = RecapCardJobStats {
            cards_selected: 4,
            ..stats_2_kept
        };
        let exec_4_kept = compute_execution_metrics(Some(&stats_4_kept), &[]).unwrap();
        assert!((exec_4_kept.drop_rate - (2.0 / 6.0)).abs() < 1e-4);
        assert!((exec_4_kept.drop_rate - 0.3333).abs() < 1e-3);
    }
}
