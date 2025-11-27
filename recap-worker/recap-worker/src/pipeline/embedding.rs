use std::sync::Arc;

use anyhow::{Context, Result};
use rand::{Rng, SeedableRng, rngs::StdRng};
use rust_bert::pipelines::sentence_embeddings::{
    SentenceEmbeddingsBuilder, SentenceEmbeddingsModel, SentenceEmbeddingsModelType,
};
use tokio::sync::Mutex;
use tracing::warn;

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

    /// Generate embeddings for a batch of texts.
    pub async fn encode(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        let model = self.model.clone();
        let texts_clone = texts.to_vec();

        // Offload to blocking thread
        let result = tokio::task::spawn_blocking(move || {
            let model = model.blocking_lock();
            model.encode(&texts_clone)
        })
        .await
        .context("Failed to join embedding task");

        match result {
            Ok(Ok(embeddings)) => {
                // Check for zero-norm vectors
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
            Ok(Err(e)) => {
                warn!(error = ?e, "embedding model failed, using fallback for all texts");
                Ok(texts.iter().map(|t| Self::fallback_embedding(t)).collect())
            }
            Err(e) => {
                warn!(error = ?e, "embedding task failed, using fallback for all texts");
                Ok(texts.iter().map(|t| Self::fallback_embedding(t)).collect())
            }
        }
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
            embedding.push(rng.gen_range(-1.0..1.0));
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
