//! Deduplication stage for topic card candidate items.

use anyhow::Result;
use std::collections::HashSet;
use tracing::info;

use super::normalize::NormalizedItem;
use crate::util::text::hash_text;

/// Normalize title for deduplication: lowercase and whitespace-collapsed.
pub fn normalize_title(title: &str) -> String {
    title
        .to_lowercase()
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ")
}

/// Deduplicate items:
/// Drops duplicates matching any of:
/// 1. Identical `websiteUrl`
/// 2. Exact XXH3 hash of normalized `title + lede`
/// 3. Identical normalized title (lowercased, whitespace-collapsed)
///
/// Keeps the item with the earliest pub_date (tie-broken deterministically by feed_id).
pub fn deduplicate(mut items: Vec<NormalizedItem>) -> (Vec<NormalizedItem>, usize) {
    let initial_count = items.len();

    // Sort by pub_date ascending so the earliest comes first.
    // Tie-break by feed_id for determinism.
    items.sort_by(|a, b| {
        a.pub_date
            .cmp(&b.pub_date)
            .then_with(|| a.feed_id.cmp(&b.feed_id))
    });

    let mut seen_hashes: HashSet<u64> = HashSet::with_capacity(items.len());
    let mut seen_urls: HashSet<String> = HashSet::with_capacity(items.len());
    let mut seen_titles: HashSet<String> = HashSet::with_capacity(items.len());
    let mut kept: Vec<NormalizedItem> = Vec::with_capacity(items.len());

    for item in items {
        let norm_title = normalize_title(&item.title);
        let content_to_hash = format!("{}{}", norm_title, item.lede);
        let content_hash = hash_text(&content_to_hash);

        if seen_hashes.contains(&content_hash)
            || seen_urls.contains(&item.url)
            || seen_titles.contains(&norm_title)
        {
            // Drop duplicate (earlier item was already kept)
            continue;
        }

        seen_hashes.insert(content_hash);
        seen_urls.insert(item.url.clone());
        seen_titles.insert(norm_title);
        kept.push(item);
    }

    let duplicates_removed = initial_count.saturating_sub(kept.len());
    info!(
        items_before_dedup = initial_count,
        items_after_dedup = kept.len(),
        duplicates_removed,
        "applied deduplication"
    );

    (kept, duplicates_removed)
}

/// Compute cosine similarity between two vectors.
pub fn cosine_similarity(a: &[f32], b: &[f32]) -> Result<f32> {
    if a.is_empty() || b.is_empty() {
        anyhow::bail!("cannot compute cosine similarity for empty vector");
    }
    if a.len() != b.len() {
        anyhow::bail!(
            "vector dimension mismatch in cosine similarity: {} vs {}",
            a.len(),
            b.len()
        );
    }
    let mut dot = 0.0_f32;
    let mut norm_a = 0.0_f32;
    let mut norm_b = 0.0_f32;
    for (x, y) in a.iter().zip(b.iter()) {
        dot += x * y;
        norm_a += x * x;
        norm_b += y * y;
    }
    if norm_a <= 1e-9 || norm_b <= 1e-9 {
        return Ok(0.0);
    }
    Ok((dot / (norm_a.sqrt() * norm_b.sqrt())).clamp(-1.0, 1.0))
}

/// Near-duplicate deduplication using embedding cosine similarity.
///
/// After embedding and before clustering, drops any item whose embedding has
/// cosine similarity >= `threshold` (default 0.95) with an already-kept item.
/// Keeps the earliest item (tie-broken by feed_id) deterministically.
pub fn deduplicate_near_duplicates(
    items: Vec<NormalizedItem>,
    embeddings: Vec<Vec<f32>>,
    threshold: f32,
) -> Result<(Vec<NormalizedItem>, Vec<Vec<f32>>, usize)> {
    if items.len() != embeddings.len() {
        anyhow::bail!(
            "mismatch between items and embeddings in near-duplicate deduplication: {} items vs {} embeddings",
            items.len(),
            embeddings.len()
        );
    }

    let initial_count = items.len();
    let mut kept_items: Vec<NormalizedItem> = Vec::with_capacity(initial_count);
    let mut kept_embeddings: Vec<Vec<f32>> = Vec::with_capacity(initial_count);

    for (item, emb) in items.into_iter().zip(embeddings) {
        let mut is_near_dup = false;
        for kept_emb in &kept_embeddings {
            if cosine_similarity(kept_emb, &emb)? >= threshold {
                is_near_dup = true;
                break;
            }
        }

        if is_near_dup {
            continue;
        }

        kept_items.push(item);
        kept_embeddings.push(emb);
    }

    let duplicates_removed = initial_count.saturating_sub(kept_items.len());
    info!(
        items_before_near_dedup = initial_count,
        items_after_near_dedup = kept_items.len(),
        near_duplicates_removed = duplicates_removed,
        threshold,
        "applied near-duplicate deduplication"
    );

    Ok((kept_items, kept_embeddings, duplicates_removed))
}

#[cfg(test)]
mod tests {
    use super::*;
    use uuid::Uuid;

    fn make_item(
        id: Uuid,
        title: &str,
        lede: &str,
        url: &str,
        pub_date_rfc3339: &str,
    ) -> NormalizedItem {
        NormalizedItem {
            feed_id: id,
            title: title.to_string(),
            lede: lede.to_string(),
            host: "example.com".to_string(),
            url: url.to_string(),
            pub_date: chrono::DateTime::parse_from_rfc3339(pub_date_rfc3339)
                .unwrap()
                .with_timezone(&chrono::Utc),
            language: "en".to_string(),
            genre: None,
        }
    }

    #[test]
    fn test_dedup_by_identical_url_keeps_earliest() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let item1 = make_item(
            id1,
            "Example headline 1",
            "Example lede 1 longer than twenty characters.",
            "https://example.com/post",
            "2026-03-20T10:00:00Z",
        );
        let item2 = make_item(
            id2,
            "Example headline 2",
            "Example lede 2 longer than twenty characters.",
            "https://example.com/post",
            "2026-03-20T12:00:00Z",
        );

        let (deduped, dropped) = deduplicate(vec![item2, item1]);
        assert_eq!(deduped.len(), 1);
        assert_eq!(dropped, 1);
        assert_eq!(
            deduped[0].feed_id, id1,
            "earliest pub_date item must be kept"
        );
    }

    #[test]
    fn test_dedup_by_identical_title_and_lede_hash_keeps_earliest() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let item1 = make_item(
            id1,
            "Example headline 1",
            "Identical lede text content longer than 20 chars.",
            "https://example.com/url1",
            "2026-03-20T08:00:00Z",
        );
        let item2 = make_item(
            id2,
            "Example headline 1",
            "Identical lede text content longer than 20 chars.",
            "https://example.com/url2",
            "2026-03-20T14:00:00Z",
        );

        let (deduped, dropped) = deduplicate(vec![item2, item1]);
        assert_eq!(deduped.len(), 1);
        assert_eq!(dropped, 1);
        assert_eq!(
            deduped[0].feed_id, id1,
            "earliest pub_date item must be kept"
        );
    }

    #[test]
    fn test_dedup_by_identical_normalized_title_keeps_earliest() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let item1 = make_item(
            id1,
            "Example Headline 1",
            "Lede text variant A that is longer than 20 characters.",
            "https://example.com/url1",
            "2026-03-20T07:00:00Z",
        );
        let item2 = make_item(
            id2,
            "  example   headline  1  ",
            "Lede text variant B that is longer than 20 characters.",
            "https://example.com/url2",
            "2026-03-20T15:00:00Z",
        );

        let (deduped, dropped) = deduplicate(vec![item2, item1]);
        assert_eq!(deduped.len(), 1);
        assert_eq!(dropped, 1);
        assert_eq!(
            deduped[0].feed_id, id1,
            "earliest pub_date item must be kept"
        );
    }

    #[test]
    fn test_dedup_distinct_items_kept() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();

        let item1 = make_item(
            id1,
            "Example headline 1",
            "Lede text A is sufficiently long for testing.",
            "https://example.com/a",
            "2026-03-20T10:00:00Z",
        );
        let item2 = make_item(
            id2,
            "Example headline 2",
            "Lede text B is sufficiently long for testing.",
            "https://example.com/b",
            "2026-03-20T11:00:00Z",
        );

        let (deduped, dropped) = deduplicate(vec![item1, item2]);
        assert_eq!(deduped.len(), 2);
        assert_eq!(dropped, 0);
    }

    #[test]
    fn test_cosine_similarity_computation() {
        let v1 = vec![1.0, 0.0, 0.0];
        let v2 = vec![1.0, 0.0, 0.0];
        assert!((cosine_similarity(&v1, &v2).unwrap() - 1.0).abs() < 1e-6);

        let v3 = vec![0.0, 1.0, 0.0];
        assert!(cosine_similarity(&v1, &v3).unwrap().abs() < 1e-6);

        let v4 = vec![-1.0, 0.0, 0.0];
        assert!((cosine_similarity(&v1, &v4).unwrap() - (-1.0)).abs() < 1e-6);
    }

    #[test]
    fn test_cosine_similarity_dimension_mismatch_errors() {
        let v1 = vec![1.0, 0.0];
        let v2 = vec![1.0, 0.0, 0.0];
        let res = cosine_similarity(&v1, &v2);
        assert!(res.is_err());
        assert!(res.unwrap_err().to_string().contains("dimension mismatch"));
    }

    #[test]
    fn test_deduplicate_near_duplicates_drops_high_similarity() {
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let id3 = Uuid::new_v4();

        let item1 = make_item(
            id1,
            "Major Earthquake in Japan",
            "A magnitude 7 earthquake struck near the coast today.",
            "https://example.com/item1",
            "2026-03-20T10:00:00Z",
        );
        let item2 = make_item(
            id2,
            "Massive Japan Quake Shakes Coast",
            "A magnitude 7 quake has struck coastal Japan today.",
            "https://example.com/item2",
            "2026-03-20T10:05:00Z",
        );
        let item3 = make_item(
            id3,
            "Completely Different Topic on Space",
            "James Webb Space Telescope finds distant galaxy cluster.",
            "https://example.com/item3",
            "2026-03-20T10:10:00Z",
        );

        // item1 and item2 have cos sim = 0.96 (above 0.95)
        // item3 has cos sim = 0.1
        let emb1 = vec![1.0, 0.0, 0.0];
        let emb2 = vec![0.98, 0.198_997_5, 0.0]; // cos(emb1, emb2) ≈ 0.98
        let emb3 = vec![0.0, 1.0, 0.0]; // orthogonal to emb1

        let items = vec![item1, item2, item3];
        let embeddings = vec![emb1, emb2, emb3];

        let (kept_items, kept_embs, dropped) =
            deduplicate_near_duplicates(items, embeddings, 0.95).expect("dedup succeeds");

        assert_eq!(dropped, 1);
        assert_eq!(kept_items.len(), 2);
        assert_eq!(kept_embs.len(), 2);
        assert_eq!(kept_items[0].feed_id, id1);
        assert_eq!(kept_items[1].feed_id, id3);
    }

    #[test]
    fn test_deduplicate_near_duplicates_mismatch_bails() {
        let item1 = make_item(
            Uuid::new_v4(),
            "Title 1",
            "Lede text variant that is long enough.",
            "https://example.com/1",
            "2026-03-20T10:00:00Z",
        );
        let items = vec![item1];
        let embeddings = vec![]; // mismatch
        let res = deduplicate_near_duplicates(items, embeddings, 0.95);
        assert!(res.is_err());
        assert!(res.unwrap_err().to_string().contains("mismatch"));
    }
}
