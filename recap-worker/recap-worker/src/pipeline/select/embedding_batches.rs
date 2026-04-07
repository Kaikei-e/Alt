use anyhow::{Result, ensure};

use crate::pipeline::embedding::Embedder;

pub(super) const EMBEDDING_REQUEST_BATCH_SIZE: usize = 32;

pub(super) async fn encode_batched(
    service: &dyn Embedder,
    texts: &[String],
    batch_size: usize,
) -> Result<Vec<Vec<f32>>> {
    if texts.is_empty() {
        return Ok(Vec::new());
    }

    let effective_batch_size = batch_size.max(1);
    let mut embeddings = Vec::with_capacity(texts.len());

    for chunk in texts.chunks(effective_batch_size) {
        let encoded = service.encode(chunk).await?;
        ensure!(
            encoded.len() == chunk.len(),
            "embedding count mismatch: expected {}, got {}",
            chunk.len(),
            encoded.len()
        );
        embeddings.extend(encoded);
    }

    Ok(embeddings)
}
