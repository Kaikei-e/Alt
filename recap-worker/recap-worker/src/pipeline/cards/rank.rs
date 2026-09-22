//! Candidate ranking and candidate DTO formation for CardsPipeline.

use anyhow::Result;
use chrono::{DateTime, Utc};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::collections::HashMap;
use uuid::Uuid;

use super::dedup::cosine_similarity;
use super::normalize::NormalizedItem;
use crate::clients::subworker::cards::ClusterOutput;
use crate::store::dao::cards::{PreviousCardSummary, RecapCardCandidate};

/// Compute cluster fingerprint: lowercase hex sha256 of sorted member feed ids joined by `","`.
pub fn compute_cluster_fingerprint(member_ids: &[Uuid]) -> String {
    let mut id_strings: Vec<String> = member_ids.iter().map(ToString::to_string).collect();
    id_strings.sort();
    let joined = id_strings.join(",");
    let mut hasher = Sha256::new();
    hasher.update(joined.as_bytes());
    format!("{:x}", hasher.finalize())
}

/// Intermediate scored cluster candidate before truncation and ranking.
pub struct ScoredCluster {
    pub fingerprint: String,
    pub size: usize,
    pub total: f64,
    pub newest_pub_date: DateTime<Utc>,
    pub centroid: Vec<f32>,
    pub domains: Value,
    pub items: Value,
    pub scores: Value,
    pub genre: Option<String>,
}

fn build_domains_json(cluster_host_counts: &HashMap<String, usize>) -> (Vec<String>, Value) {
    let mut domain_entries: Vec<(&String, &usize)> = cluster_host_counts.iter().collect();
    domain_entries.sort_by(|a, b| b.1.cmp(a.1).then_with(|| a.0.cmp(b.0)));
    let domains_json = Value::Array(
        domain_entries
            .iter()
            .map(|(host, count)| {
                json!({
                    "host": (*host).clone(),
                    "count": **count,
                })
            })
            .collect(),
    );
    let sorted_hosts: Vec<String> = domain_entries
        .into_iter()
        .map(|(host, _)| (*host).clone())
        .collect();
    (sorted_hosts, domains_json)
}

fn select_diverse_items(member_items: &[&NormalizedItem], sorted_hosts: &[String]) -> Value {
    let mut host_item_map: HashMap<&str, Vec<&NormalizedItem>> = HashMap::new();
    for item in member_items {
        host_item_map.entry(&item.host).or_default().push(item);
    }
    for items in host_item_map.values_mut() {
        items.sort_by(|a, b| {
            b.pub_date
                .cmp(&a.pub_date)
                .then_with(|| a.feed_id.cmp(&b.feed_id))
        });
    }

    let mut chosen_items: Vec<&NormalizedItem> = Vec::with_capacity(8);
    let mut round = 0;
    'round_robin: loop {
        let mut any_taken = false;
        for host in sorted_hosts {
            if let Some(items) = host_item_map.get(host.as_str()) {
                if round < items.len() {
                    chosen_items.push(items[round]);
                    any_taken = true;
                    if chosen_items.len() == 8 {
                        break 'round_robin;
                    }
                }
            }
        }
        if !any_taken {
            break;
        }
        round += 1;
    }

    Value::Array(
        chosen_items
            .into_iter()
            .map(|it| {
                json!({
                    "feed_id": it.feed_id,
                    "title": it.title,
                    "host": it.host,
                    "url": it.url,
                    "pub_date": it.pub_date.to_rfc3339(),
                    "genre": it.genre,
                })
            })
            .collect(),
    )
}

struct ClusterScoreContext<'a> {
    item_by_id: &'a HashMap<Uuid, &'a NormalizedItem>,
    host_counts_in_window: &'a HashMap<&'a str, usize>,
    personal_vector: Option<&'a [f32]>,
    alpha: f32,
    previous_cards: &'a [PreviousCardSummary],
    previous_job_to: Option<DateTime<Utc>>,
    theta_novelty: f32,
}

struct NoveltyContinuation {
    novelty_score: f64,
    story_id: Uuid,
    continues_card_id: Option<Uuid>,
    /// Previous card IDs merged into this continuation (same ID space as continues_card_id).
    merged_from: Option<Vec<Uuid>>,
}

fn determine_novelty_and_continuation(
    centroid: &[f32],
    fingerprint: &str,
    member_items: &[&NormalizedItem],
    previous_cards: &[PreviousCardSummary],
    previous_job_to: Option<DateTime<Utc>>,
    theta_novelty: f32,
) -> Result<Option<NoveltyContinuation>> {
    if previous_cards.is_empty() {
        let story_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, fingerprint.as_bytes());
        return Ok(Some(NoveltyContinuation {
            novelty_score: 1.0_f64,
            story_id,
            continues_card_id: None,
            merged_from: None,
        }));
    }

    let mut matches: Vec<(&PreviousCardSummary, f32)> = Vec::new();
    for prev in previous_cards {
        let sim = cosine_similarity(centroid, &prev.centroid)?;
        if sim >= theta_novelty {
            matches.push((prev, sim));
        }
    }

    if matches.is_empty() {
        let story_id = Uuid::new_v5(&Uuid::NAMESPACE_OID, fingerprint.as_bytes());
        Ok(Some(NoveltyContinuation {
            novelty_score: 1.0_f64,
            story_id,
            continues_card_id: None,
            merged_from: None,
        }))
    } else {
        // Check if any member has pub_date > previous_job_to
        let prev_to = previous_job_to.unwrap_or(DateTime::<Utc>::MIN_UTC);
        let has_new_item = member_items.iter().any(|it| it.pub_date > prev_to);
        if !has_new_item {
            // Drop cluster: old/duplicate story with no new items
            return Ok(None);
        }

        matches.sort_by(|a, b| {
            b.1.partial_cmp(&a.1)
                .unwrap_or(std::cmp::Ordering::Equal)
                .then_with(|| a.0.id.cmp(&b.0.id))
        });

        let nearest = matches[0].0;
        let merged = if matches.len() > 1 {
            Some(
                matches[1..]
                    .iter()
                    .map(|(c, _)| c.id)
                    .collect::<Vec<Uuid>>(),
            )
        } else {
            None
        };
        Ok(Some(NoveltyContinuation {
            novelty_score: 1.0_f64,
            story_id: nearest.story_id,
            continues_card_id: Some(nearest.id),
            merged_from: merged,
        }))
    }
}

fn score_cluster(
    cluster: &ClusterOutput,
    ctx: &ClusterScoreContext<'_>,
) -> Result<Option<ScoredCluster>> {
    let member_items: Vec<&NormalizedItem> = cluster
        .member_ids
        .iter()
        .filter_map(|id| ctx.item_by_id.get(id).copied())
        .collect();

    if member_items.is_empty() {
        return Ok(None);
    }

    let fingerprint = compute_cluster_fingerprint(&cluster.member_ids);

    let Some(continuation) = determine_novelty_and_continuation(
        &cluster.centroid,
        &fingerprint,
        &member_items,
        ctx.previous_cards,
        ctx.previous_job_to,
        ctx.theta_novelty,
    )?
    else {
        return Ok(None);
    };

    let newest_pub_date = member_items
        .iter()
        .map(|it| it.pub_date)
        .max()
        .unwrap_or(DateTime::<Utc>::MIN_UTC);

    let mut cluster_host_counts: HashMap<String, usize> = HashMap::new();
    let mut genre_counts: HashMap<String, usize> = HashMap::new();
    for item in &member_items {
        *cluster_host_counts.entry(item.host.clone()).or_insert(0) += 1;
        if let Some(ref g) = item.genre {
            *genre_counts.entry(g.clone()).or_insert(0) += 1;
        }
    }

    let cluster_genre = genre_counts
        .into_iter()
        .max_by(|a, b| a.1.cmp(&b.1).then_with(|| b.0.cmp(&a.0)))
        .map(|(g, _)| g);

    let mut corroboration = 0.0_f64;
    for host in cluster_host_counts.keys() {
        if let Some(&n_d) = ctx.host_counts_in_window.get(host.as_str()) {
            if n_d > 0 {
                corroboration += 1.0 / (n_d as f64).sqrt();
            }
        }
    }

    let (personal_score, personal_multiplier) = match ctx.personal_vector {
        Some(u) if ctx.alpha > 0.0 => {
            let cos = f64::from(cosine_similarity(u, &cluster.centroid)?);
            (Some(cos), 1.0 + f64::from(ctx.alpha) * cos)
        }
        _ => (None, 1.0),
    };

    let total = corroboration * personal_multiplier * continuation.novelty_score;

    let (sorted_hosts, domains_json) = build_domains_json(&cluster_host_counts);
    let items_json = select_diverse_items(&member_items, &sorted_hosts);

    let scores_json = json!({
        "corroboration": corroboration,
        "personal": personal_score,
        "novelty": continuation.novelty_score,
        "total": total,
        "story_id": continuation.story_id,
        "continues_card_id": continuation.continues_card_id,
        "merged_from": continuation.merged_from,
        "genre": cluster_genre,
    });

    Ok(Some(ScoredCluster {
        fingerprint,
        size: member_items.len(),
        total,
        newest_pub_date,
        centroid: cluster.centroid.clone(),
        domains: domains_json,
        items: items_json,
        scores: scores_json,
        genre: cluster_genre,
    }))
}

/// Apply genre soft cap:
/// Upper limit of 4 candidates per non-null genre among top 12.
/// Overflow candidates replaced by next highest candidate with another genre.
pub fn apply_genre_soft_cap(
    scored_clusters: Vec<ScoredCluster>,
    max_per_genre: usize,
    top_k: usize,
) -> Vec<ScoredCluster> {
    let mut top = Vec::with_capacity(top_k);
    let mut overflow = Vec::new();
    let mut genre_counts: HashMap<String, usize> = HashMap::new();

    for cluster in scored_clusters {
        if top.len() < top_k {
            if let Some(ref g) = cluster.genre {
                let count = genre_counts.entry(g.clone()).or_insert(0);
                if *count < max_per_genre {
                    *count += 1;
                    top.push(cluster);
                } else {
                    overflow.push(cluster);
                }
            } else {
                top.push(cluster);
            }
        } else {
            overflow.push(cluster);
        }
    }

    // If top has fewer than top_k items and overflow is not empty, fill remaining slots
    while top.len() < top_k && !overflow.is_empty() {
        top.push(overflow.remove(0));
    }

    top.extend(overflow);
    top
}

/// Arguments for pure candidate ranking.
pub struct RankCandidatesArgs<'a> {
    pub job_id: Uuid,
    pub created_at: DateTime<Utc>,
    pub clusters: &'a [ClusterOutput],
    pub deduped_items: &'a [NormalizedItem],
    pub personal_vector: Option<&'a [f32]>,
    pub alpha: f32,
    pub previous_cards: &'a [PreviousCardSummary],
    pub previous_job_to: Option<DateTime<Utc>>,
    pub theta_novelty: f32,
}

impl<'a> RankCandidatesArgs<'a> {
    pub fn new(
        job_id: Uuid,
        created_at: DateTime<Utc>,
        clusters: &'a [ClusterOutput],
        deduped_items: &'a [NormalizedItem],
    ) -> Self {
        Self {
            job_id,
            created_at,
            clusters,
            deduped_items,
            personal_vector: None,
            alpha: 0.0,
            previous_cards: &[],
            previous_job_to: None,
            theta_novelty: 0.80,
        }
    }
}

/// Pure ranking function: rank clusters and produce top ≤60 RecapCardCandidate records.
pub fn rank_candidates(args: RankCandidatesArgs<'_>) -> Result<Vec<RecapCardCandidate>> {
    let mut host_counts_in_window: HashMap<&str, usize> = HashMap::new();
    let mut item_by_id: HashMap<Uuid, &NormalizedItem> = HashMap::new();

    for item in args.deduped_items {
        *host_counts_in_window.entry(&item.host).or_insert(0) += 1;
        item_by_id.insert(item.feed_id, item);
    }

    let score_ctx = ClusterScoreContext {
        item_by_id: &item_by_id,
        host_counts_in_window: &host_counts_in_window,
        personal_vector: args.personal_vector,
        alpha: args.alpha,
        previous_cards: args.previous_cards,
        previous_job_to: args.previous_job_to,
        theta_novelty: args.theta_novelty,
    };

    let mut scored_clusters = Vec::new();
    for c in args.clusters {
        if let Some(sc) = score_cluster(c, &score_ctx)? {
            scored_clusters.push(sc);
        }
    }

    scored_clusters.sort_by(|a, b| {
        b.total
            .partial_cmp(&a.total)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| b.size.cmp(&a.size))
            .then_with(|| b.newest_pub_date.cmp(&a.newest_pub_date))
            .then_with(|| a.fingerprint.cmp(&b.fingerprint))
    });

    let capped_clusters = apply_genre_soft_cap(scored_clusters, 4, 12);

    Ok(capped_clusters
        .into_iter()
        .take(60)
        .enumerate()
        .map(|(idx, sc)| {
            let id = Uuid::new_v5(&args.job_id, sc.fingerprint.as_bytes());
            RecapCardCandidate {
                id,
                job_id: args.job_id,
                rank: i32::try_from(idx + 1).unwrap_or(i32::MAX),
                cluster_fingerprint: sc.fingerprint,
                size: i32::try_from(sc.size).unwrap_or(i32::MAX),
                domains: sc.domains,
                items: sc.items,
                scores: sc.scores,
                centroid: Some(sc.centroid),
                created_at: args.created_at,
            }
        })
        .collect())
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    fn make_item(id: Uuid, host: &str, pub_date_rfc3339: &str) -> NormalizedItem {
        NormalizedItem {
            feed_id: id,
            title: format!("Example headline for {id}"),
            lede: "Example lede text that is long enough to satisfy all requirements.".to_string(),
            host: host.to_string(),
            url: format!("https://{host}/post/{id}"),
            pub_date: chrono::DateTime::parse_from_rfc3339(pub_date_rfc3339)
                .unwrap()
                .with_timezone(&Utc),
            language: "en".to_string(),
            genre: None,
        }
    }

    #[test]
    fn test_corroboration_ordering_two_hosts_outranks_single_host_with_1300_items() {
        let job_id = Uuid::new_v4();
        let mut deduped = Vec::new();

        // Host A has 1,300 items in the window after dedup
        let mut host_a_ids = Vec::with_capacity(1300);
        for _ in 0..1300 {
            let id = Uuid::new_v4();
            host_a_ids.push(id);
            deduped.push(make_item(
                id,
                "example-high-volume.com",
                "2026-03-20T10:00:00Z",
            ));
        }

        // Host B has 2 items in the window
        let first_b = Uuid::new_v4();
        let second_b = Uuid::new_v4();
        deduped.push(make_item(first_b, "example-b.com", "2026-03-20T11:00:00Z"));
        deduped.push(make_item(second_b, "example-b.com", "2026-03-20T11:30:00Z"));

        // Host C has 2 items in the window
        let primary_c = Uuid::new_v4();
        let secondary_c = Uuid::new_v4();
        deduped.push(make_item(
            primary_c,
            "example-c.com",
            "2026-03-20T12:00:00Z",
        ));
        deduped.push(make_item(
            secondary_c,
            "example-c.com",
            "2026-03-20T12:30:00Z",
        ));

        // Cluster 1: 10 items, all from Host A
        // corroboration = 1 / sqrt(1300) ≈ 0.0277
        let cluster1_single_host = ClusterOutput {
            cluster_id: 1,
            member_ids: host_a_ids[0..10].to_vec(),
            centroid: vec![0.1],
        };

        // Cluster 2: 2 items, 1 from Host B and 1 from Host C
        // corroboration = 1 / sqrt(2) + 1 / sqrt(2) ≈ 1.4142
        let cluster2_two_hosts = ClusterOutput {
            cluster_id: 2,
            member_ids: vec![first_b, primary_c],
            centroid: vec![0.2],
        };

        let clusters = vec![cluster1_single_host, cluster2_two_hosts];
        let now = Utc::now();
        let args = RankCandidatesArgs::new(job_id, now, &clusters, &deduped);
        let candidates = rank_candidates(args).unwrap();

        assert_eq!(candidates.len(), 2);
        // Cluster 2 must outrank Cluster 1 because corroboration (1.4142 > 0.0277)
        assert_eq!(candidates[0].rank, 1);
        assert_eq!(candidates[0].size, 2);
        let corrob_rank1 = candidates[0].scores["corroboration"].as_f64().unwrap();
        let expected_corrob_rank1 = 1.0 / (2.0_f64).sqrt() + 1.0 / (2.0_f64).sqrt();
        assert!((corrob_rank1 - expected_corrob_rank1).abs() < 1e-4);

        assert_eq!(candidates[1].rank, 2);
        assert_eq!(candidates[1].size, 10);
        let corrob_rank2 = candidates[1].scores["corroboration"].as_f64().unwrap();
        let expected_corrob_rank2 = 1.0 / (1300.0_f64).sqrt();
        assert!((corrob_rank2 - expected_corrob_rank2).abs() < 1e-4);
    }

    #[test]
    fn test_fingerprint_independent_of_member_order() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let id3 = Uuid::new_v4();

        let fp1 = compute_cluster_fingerprint(&[id1, id2, id3]);
        let fp2 = compute_cluster_fingerprint(&[id3, id1, id2]);
        let fp3 = compute_cluster_fingerprint(&[id2, id3, id1]);

        assert_eq!(fp1, fp2);
        assert_eq!(fp2, fp3);
        assert_eq!(fp1.len(), 64, "full lowercase hex sha256 is 64 characters");
    }

    #[test]
    fn test_item_selection_host_diversity_round_robin() {
        let job_id = Uuid::new_v4();
        let mut deduped = Vec::new();
        let mut cluster_members = Vec::new();

        // Host A has 6 items, Host B has 6 items
        for i in 0..6 {
            let id = Uuid::new_v4();
            let date = format!("2026-03-20T{:02}:00:00Z", 10 + i);
            let it = make_item(id, "example-a.com", &date);
            deduped.push(it);
            cluster_members.push(id);
        }
        for i in 0..6 {
            let id = Uuid::new_v4();
            let date = format!("2026-03-20T{:02}:00:00Z", 16 + i);
            let it = make_item(id, "example-b.com", &date);
            deduped.push(it);
            cluster_members.push(id);
        }

        let cluster = ClusterOutput {
            cluster_id: 0,
            member_ids: cluster_members,
            centroid: vec![0.1],
        };

        let now = Utc::now();
        let args = RankCandidatesArgs::new(job_id, now, std::slice::from_ref(&cluster), &deduped);
        let candidates = rank_candidates(args).unwrap();
        assert_eq!(candidates.len(), 1);

        let items = candidates[0].items.as_array().unwrap();
        assert_eq!(items.len(), 8, "must select exactly 8 items (max limit)");

        // Alternates between hosts (round robin):
        let hosts: Vec<&str> = items
            .iter()
            .map(|it| it["host"].as_str().unwrap())
            .collect();
        let a_count = hosts.iter().filter(|&&h| h == "example-a.com").count();
        let b_count = hosts.iter().filter(|&&h| h == "example-b.com").count();
        assert_eq!(a_count, 4);
        assert_eq!(b_count, 4);
    }

    #[test]
    fn test_determinism_pure_function() {
        let job_id = Uuid::new_v4();
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let item1 = make_item(id1, "example-a.com", "2026-03-20T10:00:00Z");
        let item2 = make_item(id2, "example-b.com", "2026-03-20T11:00:00Z");

        let cluster = ClusterOutput {
            cluster_id: 0,
            member_ids: vec![id1, id2],
            centroid: vec![0.1, 0.2],
        };

        let deduped = vec![item1, item2];
        let now = chrono::DateTime::parse_from_rfc3339("2026-03-20T12:00:00Z")
            .unwrap()
            .with_timezone(&Utc);

        let args1 = RankCandidatesArgs::new(job_id, now, std::slice::from_ref(&cluster), &deduped);
        let run1 = rank_candidates(args1).unwrap();

        let args2 = RankCandidatesArgs::new(job_id, now, std::slice::from_ref(&cluster), &deduped);
        let run2 = rank_candidates(args2).unwrap();

        assert_eq!(run1.len(), run2.len());
        for (c1, c2) in run1.iter().zip(run2.iter()) {
            assert_eq!(c1.id, c2.id);
            assert_eq!(c1.rank, c2.rank);
            assert_eq!(c1.cluster_fingerprint, c2.cluster_fingerprint);
            assert_eq!(c1.scores, c2.scores);
            assert_eq!(c1.domains, c2.domains);
            assert_eq!(c1.items, c2.items);
            assert_eq!(c1.created_at, c2.created_at);
        }
    }

    #[test]
    fn test_personal_vector_boosting() {
        let job_id = Uuid::new_v4();
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let item1 = make_item(id1, "example-a.com", "2026-03-20T10:00:00Z");
        let item2 = make_item(id2, "example-b.com", "2026-03-20T11:00:00Z");
        let deduped = vec![item1, item2];

        // Cluster 1 aligns with personal vector u = [1.0, 0.0]
        let first_cluster = ClusterOutput {
            cluster_id: 1,
            member_ids: vec![id1],
            centroid: vec![1.0, 0.0],
        };

        // Cluster 2 is orthogonal to personal vector
        let second_cluster = ClusterOutput {
            cluster_id: 2,
            member_ids: vec![id2],
            centroid: vec![0.0, 1.0],
        };

        let clusters = vec![first_cluster, second_cluster];
        let personal_vector = vec![1.0, 0.0];
        let now = Utc::now();

        let mut args = RankCandidatesArgs::new(job_id, now, &clusters, &deduped);
        args.personal_vector = Some(&personal_vector);
        args.alpha = 0.5;

        let candidates = rank_candidates(args).unwrap();
        assert_eq!(candidates.len(), 2);
        // Cluster 1 is boosted by (1.0 + 0.5 * 1.0) = 1.5, so it outranks Cluster 2
        assert_eq!(
            candidates[0].cluster_fingerprint,
            compute_cluster_fingerprint(&[id1])
        );
        assert_eq!(
            candidates[1].cluster_fingerprint,
            compute_cluster_fingerprint(&[id2])
        );
        assert!(candidates[0].scores["personal"].as_f64().unwrap() > 0.99);
    }

    #[test]
    fn test_novelty_continuation_and_duplicate_drop() {
        let job_id = Uuid::new_v4();
        let prev_card_id = Uuid::new_v4();
        let prev_story_id = Uuid::new_v4();
        let prev_cards = vec![PreviousCardSummary {
            id: prev_card_id,
            story_id: prev_story_id,
            centroid: vec![1.0, 0.0],
        }];

        let prev_job_to = chrono::DateTime::parse_from_rfc3339("2026-03-18T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);

        // Item 1 is published AFTER previous job end (2026-03-19 > 2026-03-18) -> continuing story
        let id1 = Uuid::new_v4();
        let item1 = make_item(id1, "example-a.com", "2026-03-19T10:00:00Z");

        // Item 2 is published BEFORE previous job end (2026-03-17 < 2026-03-18) -> old duplicate story
        let id2 = Uuid::new_v4();
        let item2 = make_item(id2, "example-b.com", "2026-03-17T10:00:00Z");

        let deduped = vec![item1, item2];

        let cluster_continuing = ClusterOutput {
            cluster_id: 1,
            member_ids: vec![id1],
            centroid: vec![0.99, 0.01], // sim >= 0.80
        };

        let cluster_duplicate = ClusterOutput {
            cluster_id: 2,
            member_ids: vec![id2],
            centroid: vec![0.99, 0.01], // sim >= 0.80
        };

        let clusters = vec![cluster_continuing, cluster_duplicate];
        let now = Utc::now();

        let mut args = RankCandidatesArgs::new(job_id, now, &clusters, &deduped);
        args.previous_cards = &prev_cards;
        args.previous_job_to = Some(prev_job_to);
        args.theta_novelty = 0.80;

        let candidates = rank_candidates(args).unwrap();
        // Only the continuing cluster should survive; the duplicate cluster must be dropped!
        assert_eq!(candidates.len(), 1);
        assert_eq!(
            candidates[0].cluster_fingerprint,
            compute_cluster_fingerprint(&[id1])
        );
        assert_eq!(
            candidates[0].scores["continues_card_id"].as_str().unwrap(),
            prev_card_id.to_string()
        );
        assert_eq!(
            candidates[0].scores["story_id"].as_str().unwrap(),
            prev_story_id.to_string()
        );
    }

    #[test]
    fn test_novelty_merged_story() {
        let job_id = Uuid::new_v4();
        let card1_id = Uuid::new_v4();
        let card2_id = Uuid::new_v4();
        let story1_id = Uuid::new_v4();
        let story2_id = Uuid::new_v4();

        let prev_cards = vec![
            PreviousCardSummary {
                id: card1_id,
                story_id: story1_id,
                centroid: vec![1.0, 0.0],
            },
            PreviousCardSummary {
                id: card2_id,
                story_id: story2_id,
                centroid: vec![0.9, 0.435_889_9], // approx unit vector
            },
        ];

        let prev_job_to = chrono::DateTime::parse_from_rfc3339("2026-03-18T00:00:00Z")
            .unwrap()
            .with_timezone(&Utc);

        let id1 = Uuid::new_v4();
        let item1 = make_item(id1, "example-a.com", "2026-03-19T10:00:00Z");
        let deduped = vec![item1];

        // Cluster is closer to card 1 (cos = 0.999) and also matches card 2 (cos = 0.89)
        let cluster = ClusterOutput {
            cluster_id: 1,
            member_ids: vec![id1],
            centroid: vec![0.999, 0.044],
        };

        let now = Utc::now();
        let mut args =
            RankCandidatesArgs::new(job_id, now, std::slice::from_ref(&cluster), &deduped);
        args.previous_cards = &prev_cards;
        args.previous_job_to = Some(prev_job_to);
        args.theta_novelty = 0.80;

        let candidates = rank_candidates(args).unwrap();
        assert_eq!(candidates.len(), 1);
        assert_eq!(
            candidates[0].scores["story_id"].as_str().unwrap(),
            story1_id.to_string()
        );
        assert_eq!(
            candidates[0].scores["continues_card_id"].as_str().unwrap(),
            card1_id.to_string()
        );
        let merged: Vec<String> = candidates[0].scores["merged_from"]
            .as_array()
            .unwrap()
            .iter()
            .map(|v| v.as_str().unwrap().to_string())
            .collect();
        assert_eq!(merged, vec![card2_id.to_string()]);
    }

    #[test]
    fn test_genre_soft_cap_limits_to_4_per_genre_in_top_12() {
        let job_id = Uuid::new_v4();
        let mut deduped = Vec::new();
        let mut clusters = Vec::new();

        // Create 6 items with genre "tech" (they will have high corroboration)
        for i in 0..6 {
            let id = Uuid::new_v4();
            let mut it = make_item(id, &format!("host-tech-{i}.com"), "2026-03-20T10:00:00Z");
            it.genre = Some("tech".to_string());
            deduped.push(it);
            clusters.push(ClusterOutput {
                cluster_id: i,
                member_ids: vec![id],
                centroid: vec![0.1],
            });
        }

        // Create 2 items with genre "politics"
        for i in 0..2 {
            let id = Uuid::new_v4();
            let mut it = make_item(id, &format!("host-pol-{i}.com"), "2026-03-20T10:00:00Z");
            it.genre = Some("politics".to_string());
            deduped.push(it);
            clusters.push(ClusterOutput {
                cluster_id: 6 + i,
                member_ids: vec![id],
                centroid: vec![0.1],
            });
        }

        // Create 6 items with genre None (below threshold)
        for i in 0..6 {
            let id = Uuid::new_v4();
            let it = make_item(id, &format!("host-none-{i}.com"), "2026-03-20T10:00:00Z");
            deduped.push(it);
            clusters.push(ClusterOutput {
                cluster_id: 8 + i,
                member_ids: vec![id],
                centroid: vec![0.1],
            });
        }

        let now = Utc::now();
        let args = RankCandidatesArgs::new(job_id, now, &clusters, &deduped);
        let candidates = rank_candidates(args).unwrap();

        // Top 12 must contain at most 4 "tech"
        let top_12 = &candidates[0..12];
        let tech_in_top12 = top_12
            .iter()
            .filter(|c| c.scores["genre"].as_str() == Some("tech"))
            .count();
        assert_eq!(tech_in_top12, 4, "top 12 must cap tech candidates to 4");

        // Candidates beyond rank 12 should include the 5th and 6th tech candidates
        let overflow = &candidates[12..];
        let tech_in_overflow = overflow
            .iter()
            .filter(|c| c.scores["genre"].as_str() == Some("tech"))
            .count();
        assert_eq!(
            tech_in_overflow, 2,
            "overflow contains remaining tech candidates"
        );

        // Candidates with genre = None are not capped and appear in top 12
        let none_in_top12 = top_12
            .iter()
            .filter(|c| c.scores["genre"].is_null())
            .count();
        assert!(
            none_in_top12 >= 6,
            "genre = null candidates remain in candidates"
        );
    }

    #[test]
    fn test_apply_genre_soft_cap_overflow_readmission() {
        // Document and verify: overflow re-admission happens only when fewer than
        // top_k other-genre candidates exist.
        let make_scored = |fp: &str, genre: Option<&str>, total: f64| ScoredCluster {
            fingerprint: fp.to_string(),
            size: 1,
            total,
            newest_pub_date: Utc::now(),
            centroid: vec![1.0, 0.0],
            domains: serde_json::json!({}),
            items: serde_json::json!([]),
            scores: serde_json::json!({}),
            genre: genre.map(String::from),
        };

        // Case A: 6 "tech" candidates and 2 "finance" candidates (total 8 < 12).
        // Max 4 per genre, top_k = 12.
        // The 2 overflow "tech" candidates are re-admitted to fill the remaining slots.
        let mut clusters_a = Vec::new();
        for i in 0..6 {
            clusters_a.push(make_scored(
                &format!("tech_{i}"),
                Some("tech"),
                10.0 - f64::from(i),
            ));
        }
        for i in 0..2 {
            clusters_a.push(make_scored(
                &format!("fin_{i}"),
                Some("finance"),
                5.0 - f64::from(i),
            ));
        }
        let res_a = apply_genre_soft_cap(clusters_a, 4, 12);
        assert_eq!(res_a.len(), 8);
        let tech_count_a = res_a
            .iter()
            .filter(|c| c.genre.as_deref() == Some("tech"))
            .count();
        assert_eq!(
            tech_count_a, 6,
            "all 6 tech candidates admitted because total (8) < top_k (12)"
        );

        // Case B: 6 "tech" candidates and 10 other candidates (4 finance, 6 science).
        // Total other = 10. Top 12 can hold 4 tech + 8 others.
        // The remaining 2 tech overflow candidates are NOT in top 12.
        let mut clusters_b = Vec::new();
        for i in 0..6 {
            clusters_b.push(make_scored(
                &format!("tech_{i}"),
                Some("tech"),
                10.0 - f64::from(i),
            ));
        }
        for i in 0..4 {
            clusters_b.push(make_scored(
                &format!("fin_{i}"),
                Some("finance"),
                8.0 - f64::from(i),
            ));
        }
        for i in 0..6 {
            clusters_b.push(make_scored(
                &format!("sci_{i}"),
                Some("science"),
                7.0 - f64::from(i),
            ));
        }
        let res_b = apply_genre_soft_cap(clusters_b, 4, 12);
        let top_12_b = &res_b[..12];
        let tech_in_top_b = top_12_b
            .iter()
            .filter(|c| c.genre.as_deref() == Some("tech"))
            .count();
        assert_eq!(
            tech_in_top_b, 4,
            "tech is strictly capped to 4 when >= top_k slots filled by other genres"
        );
    }

    #[test]
    fn test_rank_candidates_dimension_mismatch_fails() {
        let item_id = Uuid::new_v4();
        let args = RankCandidatesArgs {
            job_id: Uuid::new_v4(),
            created_at: Utc::now(),
            clusters: &[ClusterOutput {
                cluster_id: 0,
                member_ids: vec![item_id],
                centroid: vec![1.0, 0.0],
            }],
            deduped_items: &[make_item(item_id, "example.com", "2026-03-20T10:00:00Z")],
            personal_vector: None,
            alpha: 0.0,
            previous_cards: &[PreviousCardSummary {
                id: Uuid::new_v4(),
                story_id: Uuid::new_v4(),
                centroid: vec![1.0, 0.0, 0.0], // 3D vs 2D mismatch!
            }],
            previous_job_to: None,
            theta_novelty: 0.8,
        };
        let res = rank_candidates(args);
        assert!(res.is_err());
        assert!(res.unwrap_err().to_string().contains("dimension mismatch"));
    }
}
