//! Shared embedding cache lookup and population helpers.

use anyhow::{Context, Result};
use std::collections::{HashMap, HashSet};
use uuid::Uuid;

use crate::pipeline::cards::ports::EmbedCluster;

/// Compute cache key hash for an embedding input string (xxh3-64 hex).
#[must_use]
pub fn compute_embedding_text_hash(text: &str) -> String {
    format!("{:016x}", xxhash_rust::xxh3::xxh3_64(text.as_bytes()))
}

/// Persistence interface for reading and storing cached text embeddings.
#[async_trait::async_trait]
pub trait EmbeddingCacheDao: Send + Sync {
    async fn get_cached_embeddings(
        &self,
        model: &str,
        text_hashes: &[String],
    ) -> Result<HashMap<String, Vec<f32>>>;

    async fn insert_embeddings(
        &self,
        model: &str,
        dim: usize,
        entries: &[(String, Vec<f32>)],
    ) -> Result<()>;
}

/// Embed `texts` using the persistent cache for hits and `ml_port` for misses.
pub async fn embed_with_cache(
    dao: &dyn EmbeddingCacheDao,
    ml_port: &dyn EmbedCluster,
    run_id: Uuid,
    texts: &[String],
    expected_model: &str,
    expected_dim: usize,
) -> Result<(Vec<Vec<f32>>, usize, usize)> {
    if texts.is_empty() {
        return Ok((Vec::new(), 0, 0));
    }

    let text_hashes: Vec<String> = texts
        .iter()
        .map(|t| compute_embedding_text_hash(t))
        .collect();

    let cached = dao
        .get_cached_embeddings(expected_model, &text_hashes)
        .await
        .context("failed to query embedding cache")?;

    for (hash, vec) in &cached {
        if vec.len() != expected_dim {
            anyhow::bail!(
                "cached embedding dimension mismatch for hash {hash}: expected {expected_dim}, got {}",
                vec.len()
            );
        }
    }

    let mut misses_indices = Vec::new();
    let mut misses_texts = Vec::new();
    for (i, hash) in text_hashes.iter().enumerate() {
        if !cached.contains_key(hash) {
            misses_indices.push(i);
            misses_texts.push(texts[i].clone());
        }
    }

    let hits_count = texts.len() - misses_texts.len();
    let misses_count = misses_texts.len();

    let new_embeddings = if misses_texts.is_empty() {
        Vec::new()
    } else {
        let embs = ml_port
            .embed(&misses_texts)
            .await
            .context("failed to generate ML embeddings for cache misses")?;

        if embs.len() != misses_texts.len() {
            anyhow::bail!(
                "subworker embedding count mismatch for cache misses: expected {}, got {}",
                misses_texts.len(),
                embs.len()
            );
        }
        embs
    };

    if !new_embeddings.is_empty() {
        let mut entries_to_insert = Vec::with_capacity(new_embeddings.len());
        let mut seen_hashes = HashSet::new();
        for (&idx, emb) in misses_indices.iter().zip(new_embeddings.iter()) {
            let hash = &text_hashes[idx];
            if seen_hashes.insert(hash.clone()) {
                entries_to_insert.push((hash.clone(), emb.clone()));
            }
        }
        let insert_dim = new_embeddings.first().map_or(expected_dim, Vec::len);
        dao.insert_embeddings(expected_model, insert_dim, &entries_to_insert)
            .await
            .context("failed to insert new embeddings into cache")?;
    }

    let mut final_embeddings = Vec::with_capacity(texts.len());
    let mut miss_map: HashMap<usize, Vec<f32>> = HashMap::new();
    for (&idx, emb) in misses_indices.iter().zip(new_embeddings) {
        miss_map.insert(idx, emb);
    }

    for (i, hash) in text_hashes.iter().enumerate() {
        if let Some(emb) = cached.get(hash) {
            final_embeddings.push(emb.clone());
        } else if let Some(emb) = miss_map.remove(&i) {
            final_embeddings.push(emb);
        } else {
            anyhow::bail!("internal error: missing embedding for index {i}");
        }
    }

    tracing::debug!(
        run_id = %run_id,
        total_texts = texts.len(),
        cache_hits = hits_count,
        cache_misses = misses_count,
        "embed_with_cache completed"
    );

    Ok((final_embeddings, hits_count, misses_count))
}
