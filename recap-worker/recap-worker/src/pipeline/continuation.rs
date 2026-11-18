//! Shared centroid continuation matching for cards and themes.

use anyhow::Result;
use uuid::Uuid;

/// Match result identifying a previous entity and its cosine similarity.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct ContinuationMatch {
    pub id: Uuid,
    pub similarity: f32,
}

/// Compute cosine similarity between two non-empty equal-length vectors.
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

/// Return all previous items with cosine similarity at or above `threshold`,
/// ordered by highest similarity then lowest id.
pub fn match_ranked_continuations<'a>(
    centroid: &[f32],
    previous: impl IntoIterator<Item = (Uuid, &'a [f32])>,
    threshold: f32,
) -> Result<Vec<ContinuationMatch>> {
    let mut matches = Vec::new();
    for (id, prev_centroid) in previous {
        let similarity = cosine_similarity(centroid, prev_centroid)?;
        if similarity >= threshold {
            matches.push(ContinuationMatch { id, similarity });
        }
    }

    matches.sort_by(|a, b| {
        b.similarity
            .partial_cmp(&a.similarity)
            .unwrap_or(std::cmp::Ordering::Equal)
            .then_with(|| a.id.cmp(&b.id))
    });

    Ok(matches)
}

/// Return the single best matching previous item at or above `threshold`,
/// breaking ties by lowest id.
pub fn match_best_continuation<'a>(
    centroid: &[f32],
    previous: impl IntoIterator<Item = (Uuid, &'a [f32])>,
    threshold: f32,
) -> Result<Option<ContinuationMatch>> {
    Ok(match_ranked_continuations(centroid, previous, threshold)?
        .into_iter()
        .next())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn no_match_when_previous_is_empty() {
        let centroid = [1.0_f32, 0.0];
        let previous: Vec<(Uuid, &[f32])> = Vec::new();
        let best = match_best_continuation(&centroid, previous, 0.80).unwrap();
        assert_eq!(best, None);
    }

    #[test]
    fn no_match_when_all_below_threshold() {
        let centroid = [1.0_f32, 0.0];
        let p1 = [0.5_f32, 0.866_025_4];
        let p2 = [0.0_f32, 1.0];
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let best = match_best_continuation(
            &centroid,
            [(id1, p1.as_slice()), (id2, p2.as_slice())],
            0.80,
        )
        .unwrap();
        assert_eq!(best, None);
    }

    #[test]
    fn matches_single_candidate_at_or_above_threshold() {
        let centroid = [1.0_f32, 0.0];
        let p1 = [0.8_f32, 0.6];
        let p2 = [0.0_f32, 1.0];
        let id1 = Uuid::new_v4();
        let id2 = Uuid::new_v4();
        let best = match_best_continuation(
            &centroid,
            [(id1, p1.as_slice()), (id2, p2.as_slice())],
            0.80,
        )
        .unwrap()
        .expect("expected threshold match");
        assert_eq!(best.id, id1);
        assert!((best.similarity - 0.8).abs() < 1e-6);
    }

    #[test]
    fn picks_highest_similarity_over_lower_id() {
        let centroid = [1.0_f32, 0.0];
        let p_low_id = [0.85_f32, 0.526_782_7];
        let p_high_sim = [0.99_f32, 0.141_067_4];
        let id_low = Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap();
        let id_high = Uuid::parse_str("00000000-0000-0000-0000-000000000002").unwrap();

        let best = match_best_continuation(
            &centroid,
            [
                (id_low, p_low_id.as_slice()),
                (id_high, p_high_sim.as_slice()),
            ],
            0.80,
        )
        .unwrap()
        .expect("expected match");
        assert_eq!(best.id, id_high);
    }

    #[test]
    fn tie_breaks_by_lowest_id_on_equal_similarity() {
        let centroid = [1.0_f32, 0.0];
        let p = [0.95_f32, 0.312_249_9];
        let id_high = Uuid::parse_str("ffffffff-ffff-ffff-ffff-ffffffffffff").unwrap();
        let id_low = Uuid::parse_str("00000000-0000-0000-0000-000000000001").unwrap();

        let best = match_best_continuation(
            &centroid,
            [(id_high, p.as_slice()), (id_low, p.as_slice())],
            0.80,
        )
        .unwrap()
        .expect("expected tie-break match");
        assert_eq!(best.id, id_low);

        let ranked = match_ranked_continuations(
            &centroid,
            [(id_high, p.as_slice()), (id_low, p.as_slice())],
            0.80,
        )
        .unwrap();
        assert_eq!(ranked.len(), 2);
        assert_eq!(ranked[0].id, id_low);
        assert_eq!(ranked[1].id, id_high);
    }

    #[test]
    fn dimension_mismatch_returns_error() {
        let centroid = [1.0_f32, 0.0];
        let mismatch = [1.0_f32, 0.0, 0.0];
        let id = Uuid::new_v4();
        let err = match_best_continuation(&centroid, [(id, mismatch.as_slice())], 0.80)
            .expect_err("dimension mismatch must error");
        assert!(err.to_string().contains("dimension mismatch"));
    }

    #[test]
    fn empty_vector_returns_error() {
        let centroid = [1.0_f32, 0.0];
        let empty: [f32; 0] = [];
        let id = Uuid::new_v4();
        let err = match_best_continuation(&centroid, [(id, empty.as_slice())], 0.80)
            .expect_err("empty vector must error");
        assert!(err.to_string().contains("empty vector"));
    }
}
