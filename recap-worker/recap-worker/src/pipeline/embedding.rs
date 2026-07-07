use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use rand::{RngExt, SeedableRng, rngs::StdRng};
use rust_bert::RustBertError;
use rust_bert::pipelines::sentence_embeddings::{
    SentenceEmbeddingsBuilder, SentenceEmbeddingsModel, SentenceEmbeddingsModelType,
};
use tokio::sync::Mutex;
use tracing::warn;

#[async_trait]
pub trait Embedder: Send + Sync + std::fmt::Debug {
    async fn encode(&self, texts: &[String]) -> Result<Vec<Vec<f32>>>;
}

/// Whether the rust-bert embedding service is a hard requirement at startup.
///
/// `Required` mirrors the Settings-validator fail-closed pattern established in
/// ADR-000825 (recap-subworker joblib artefacts): the runtime must refuse to
/// start when the embedding model cannot initialize. The alternative — `Optional`
/// — keeps the pre-existing degraded keyword-only behaviour for dev/test stacks
/// that do not have a rust-bert cache populated.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EmbeddingAvailability {
    Required,
    Optional,
}

/// Apply the configured availability policy to an embedding-init result.
///
/// - `(Required, Err)` → surfaces the error so the caller can fail-closed at
///   startup, rather than silently degrading the pipeline to keyword-only
///   filtering (the silent-failure footgun described in PM-2026-038).
/// - `(Optional, Err)` → returns `Ok(None)`; callers log a warning and continue
///   with the fallback path that already has unit-test coverage
///   (`subcluster_large_genres_handles_no_embedding_service`).
/// - Any `Ok(v)` → `Ok(Some(v))`.
pub fn require_or_degrade<T, E>(
    result: std::result::Result<T, E>,
    policy: EmbeddingAvailability,
) -> std::result::Result<Option<T>, E> {
    match (policy, result) {
        (_, Ok(v)) => Ok(Some(v)),
        (EmbeddingAvailability::Required, Err(e)) => Err(e),
        (EmbeddingAvailability::Optional, Err(_)) => Ok(None),
    }
}

/// Embedding generation service using rust-bert.
/// This runs on CPU.
#[derive(Clone)]
pub struct EmbeddingService {
    model: Arc<Mutex<SentenceEmbeddingsModel>>,
}

impl std::fmt::Debug for EmbeddingService {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("EmbeddingService")
            .field("model", &"<SentenceEmbeddingsModel>")
            .finish()
    }
}

impl EmbeddingService {
    /// Initialize the embedding model.
    /// This might take a while to download the model on first run.
    pub fn new() -> Result<Self> {
        // Use a separate thread to initialize the model because it's blocking and heavy
        let model = std::thread::spawn(|| {
            SentenceEmbeddingsBuilder::remote(SentenceEmbeddingsModelType::AllMiniLmL12V2)
                .create_model()
        })
        .join()
        .map_err(|_| anyhow::anyhow!("Failed to join model creation thread"))??;

        Ok(Self {
            model: Arc::new(Mutex::new(model)),
        })
    }

    /// Generate a deterministic fallback embedding using MD5 hashing.
    fn fallback_embedding(text: &str) -> Vec<f32> {
        let digest = md5::compute(text);
        // Use the MD5 hash as a seed for a random number generator
        // MD5 produces 16 bytes, which is enough for a seed (u64 needs 8 bytes, StdRng::from_seed needs 32 bytes)
        // We'll pad the seed.
        let mut seed = [0u8; 32];
        for (i, &byte) in digest.iter().enumerate() {
            seed[i] = byte;
            seed[i + 16] = byte; // Simple padding
        }

        let mut rng = StdRng::from_seed(seed);
        // AllMiniLmL12V2 dimension is 384
        let mut embedding = Vec::with_capacity(384);
        for _ in 0..384 {
            embedding.push(rng.random_range(-1.0..1.0));
        }

        // Normalize
        let norm: f32 = embedding.iter().map(|x| x * x).sum::<f32>().sqrt();
        if norm > 0.0 {
            for x in &mut embedding {
                *x /= norm;
            }
        }

        embedding
    }

    /// Generate embeddings for a batch of texts.
    ///
    /// Model failure or a panicked/cancelled blocking task surfaces as `Err`
    /// (PM-2026-038: this previously fell back to an MD5-seeded random
    /// vector for every text in the batch and returned `Ok`, so downstream
    /// clustering/subgenre-splitting silently ran on meaningless noise).
    /// Callers already treat a failed `encode` as "embeddings unavailable"
    /// and degrade gracefully (skip clustering, keep the coarse genre).
    pub async fn encode(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        let model = self.model.clone();
        let texts_clone = texts.to_vec();

        // Offload to blocking thread
        let batch_result = tokio::task::spawn_blocking(move || {
            let model = model.blocking_lock();
            model.encode(&texts_clone)
        })
        .await
        .context("embedding task panicked or was cancelled")?;

        Self::resolve_batch_result(batch_result, texts)
    }

    /// Turn a raw model-encode outcome into the batch's embeddings.
    ///
    /// Extracted from `encode` so the failure-propagation behaviour is
    /// testable without spinning up the real rust-bert model. A per-text
    /// zero-norm output still gets an individual deterministic repair (a
    /// narrow numerical-stability guard on an otherwise-successful batch,
    /// not a blanket "pretend the model worked" fallback for the whole
    /// request).
    fn resolve_batch_result(
        batch_result: std::result::Result<Vec<Vec<f32>>, RustBertError>,
        texts: &[String],
    ) -> Result<Vec<Vec<f32>>> {
        let embeddings = batch_result.context("embedding model failed to encode batch")?;

        let mut valid_embeddings = Vec::with_capacity(embeddings.len());
        let mut fallback_count = 0;

        for (i, embedding) in embeddings.into_iter().enumerate() {
            let norm: f32 = embedding.iter().map(|x| x * x).sum();
            if norm.abs() < 1e-6 {
                // Zero vector detected, use fallback
                valid_embeddings.push(Self::fallback_embedding(&texts[i]));
                fallback_count += 1;
            } else {
                valid_embeddings.push(embedding);
            }
        }

        if fallback_count > 0 {
            warn!(
                fallback_count,
                total_count = texts.len(),
                "generated fallback embeddings due to zero-norm output"
            );
        }

        Ok(valid_embeddings)
    }
}

#[async_trait]
impl Embedder for EmbeddingService {
    async fn encode(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        self.encode(texts).await
    }
}

/// Compute cosine similarity between two vectors.
pub fn cosine_similarity(a: &[f32], b: &[f32]) -> f32 {
    let dot_product: f32 = a.iter().zip(b).map(|(x, y)| x * y).sum();
    let norm_a: f32 = a.iter().map(|x| x * x).sum::<f32>().sqrt();
    let norm_b: f32 = b.iter().map(|x| x * x).sum::<f32>().sqrt();

    if norm_a == 0.0 || norm_b == 0.0 {
        return 0.0;
    }

    dot_product / (norm_a * norm_b)
}

#[cfg(test)]
mod tests {
    use super::{EmbeddingAvailability, EmbeddingService, require_or_degrade};
    use rust_bert::RustBertError;

    /// PM-2026-038 regression: a model failure must surface as `Err`, never
    /// as a batch of fabricated MD5-seeded random vectors dressed up as
    /// `Ok`. Downstream clustering must be able to tell "no embedding" from
    /// "embedding is noise".
    #[test]
    fn resolve_batch_result_propagates_model_failure_as_err() {
        let texts = vec!["a".to_string(), "b".to_string()];
        let outcome: std::result::Result<Vec<Vec<f32>>, RustBertError> =
            Err(RustBertError::ValueError("model unavailable".to_string()));

        let result = EmbeddingService::resolve_batch_result(outcome, &texts);

        assert!(
            result.is_err(),
            "model failure must propagate as Err, not a fabricated Ok(..) batch"
        );
    }

    #[test]
    fn resolve_batch_result_passes_through_healthy_embeddings() {
        let texts = vec!["a".to_string()];
        let outcome: std::result::Result<Vec<Vec<f32>>, RustBertError> = Ok(vec![vec![1.0, 0.0]]);

        let result = EmbeddingService::resolve_batch_result(outcome, &texts).unwrap();

        assert_eq!(result, vec![vec![1.0, 0.0]]);
    }

    #[test]
    fn resolve_batch_result_repairs_only_zero_norm_rows() {
        let texts = vec!["a".to_string(), "b".to_string()];
        // First row is a legitimate embedding; second is the zero vector a
        // model can emit for degenerate input. Only the zero row should be
        // replaced — the healthy row must survive untouched.
        let outcome: std::result::Result<Vec<Vec<f32>>, RustBertError> =
            Ok(vec![vec![0.6, 0.8], vec![0.0, 0.0]]);

        let result = EmbeddingService::resolve_batch_result(outcome, &texts).unwrap();

        assert_eq!(result.len(), 2);
        assert_eq!(result[0], vec![0.6, 0.8]);
        let repaired_norm: f32 = result[1].iter().map(|x| x * x).sum::<f32>().sqrt();
        assert!(
            (repaired_norm - 1.0).abs() < 1e-3,
            "repaired zero-norm row must be renormalized: {result:?}"
        );
    }

    #[test]
    fn required_surfaces_init_error() {
        let init: std::result::Result<&'static str, &'static str> = Err("cache empty");
        let out = require_or_degrade(init, EmbeddingAvailability::Required);
        assert_eq!(out, Err("cache empty"));
    }

    #[test]
    fn optional_degrades_init_error_to_none() {
        let init: std::result::Result<&'static str, &'static str> = Err("cache empty");
        let out = require_or_degrade(init, EmbeddingAvailability::Optional);
        assert_eq!(out, Ok(None));
    }

    #[test]
    fn required_passes_through_success() {
        let init: std::result::Result<&'static str, &'static str> = Ok("model");
        let out = require_or_degrade(init, EmbeddingAvailability::Required);
        assert_eq!(out, Ok(Some("model")));
    }

    #[test]
    fn optional_passes_through_success() {
        let init: std::result::Result<&'static str, &'static str> = Ok("model");
        let out = require_or_degrade(init, EmbeddingAvailability::Optional);
        assert_eq!(out, Ok(Some("model")));
    }
}
