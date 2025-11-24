use std::sync::Arc;

use anyhow::{Context, Result};
use rust_bert::pipelines::sentence_embeddings::{
    SentenceEmbeddingsBuilder, SentenceEmbeddingsModel, SentenceEmbeddingsModelType,
};
use tokio::sync::Mutex;

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
        let texts = texts.to_vec();

        // Offload to blocking thread
        tokio::task::spawn_blocking(move || {
            let model = model.blocking_lock();
            model.encode(&texts)
        })
        .await
        .context("Failed to join embedding task")?
        .context("Failed to encode texts")
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
