use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use rand::{RngExt, SeedableRng, rngs::StdRng};
use rust_bert::RustBertError;
use rust_bert::pipelines::sentence_embeddings::{
    SentenceEmbeddingsBuilder, SentenceEmbeddingsModel,
};
use tokio::sync::Mutex;
use tracing::warn;

/// Where the image bakes the sentence-transformers model directory.
const DEFAULT_MODEL_DIR: &str = "/opt/rustbert-models/all-MiniLM-L12-v2";

/// The files `SentenceEmbeddingsBuilder::local` reads for a BERT
/// sentence-transformers model.
///
/// rust-bert opens the JSON configs through `Config::from_file`, which
/// `expect`s the open to succeed — a missing file is a panic, and with
/// `panic = "abort"` that is process death with no named cause. Listing the
/// files lets a mis-built image fail with a message that says which one.
pub(crate) const REQUIRED_MODEL_FILES: &[&str] = &[
    "modules.json",
    "config.json",
    "rust_model.ot",
    "tokenizer_config.json",
    "sentence_bert_config.json",
    "vocab.txt",
    "1_Pooling/config.json",
];

/// Which of [`REQUIRED_MODEL_FILES`] are absent from `dir`.
pub(crate) fn missing_model_files(dir: &Path) -> Vec<&'static str> {
    REQUIRED_MODEL_FILES
        .iter()
        .copied()
        .filter(|rel| !dir.join(rel).is_file())
        .collect()
}

fn configured_model_dir() -> PathBuf {
    std::env::var("RECAP_WORKER_EMBEDDING_MODEL_DIR")
        .map_or_else(|_| PathBuf::from(DEFAULT_MODEL_DIR), PathBuf::from)
}

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
    /// Load the embedding model from the directory baked into the image.
    ///
    /// Blocking and CPU-heavy (~130 MB of weights) — call it from
    /// `spawn_blocking`, never directly on an async worker.
    ///
    /// Loads through `SentenceEmbeddingsBuilder::local` rather than `::remote`.
    /// The remote builder resolves each file through `cached_path`, which
    /// records no expiry for cached entries and therefore treats every entry as
    /// stale: it issues an HTTP HEAD to huggingface.co on *every* start, warm
    /// cache or not, using a client built with no read timeout. One HEAD that
    /// connected and then went silent left this process wedged for 16 hours
    /// before it ever bound its listener. `local` reads straight from disk and
    /// cannot reach the network.
    pub fn new() -> Result<Self> {
        Self::from_dir(&configured_model_dir())
    }

    /// Load the model from an explicit directory. Fails with the names of any
    /// missing files rather than letting rust-bert panic on the first one.
    pub(crate) fn from_dir(dir: &Path) -> Result<Self> {
        let missing = missing_model_files(dir);
        if !missing.is_empty() {
            anyhow::bail!(
                "sentence-embeddings model directory {} is incomplete; missing: {}",
                dir.display(),
                missing.join(", ")
            );
        }

        let model = SentenceEmbeddingsBuilder::local(dir)
            .create_model()
            .with_context(|| {
                format!(
                    "failed to load sentence-embeddings model from {}",
                    dir.display()
                )
            })?;

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
    use super::{
        EmbeddingAvailability, EmbeddingService, REQUIRED_MODEL_FILES, missing_model_files,
        require_or_degrade,
    };
    use rust_bert::RustBertError;

    fn populate(dir: &std::path::Path, files: &[&str]) {
        for rel in files {
            let path = dir.join(rel);
            std::fs::create_dir_all(path.parent().unwrap()).unwrap();
            std::fs::write(&path, b"x").unwrap();
        }
    }

    #[test]
    fn missing_model_files_lists_everything_for_an_empty_dir() {
        let dir = tempfile::tempdir().unwrap();

        assert_eq!(missing_model_files(dir.path()), REQUIRED_MODEL_FILES);
    }

    #[test]
    fn missing_model_files_is_empty_for_a_complete_dir() {
        let dir = tempfile::tempdir().unwrap();
        populate(dir.path(), REQUIRED_MODEL_FILES);

        assert!(missing_model_files(dir.path()).is_empty());
    }

    /// `1_Pooling/config.json` sits in a subdirectory, so a flat wildcard copy
    /// in the image build misses it while every other file lands. rust-bert
    /// would then panic inside `Config::from_file` instead of returning an
    /// error, so the pre-flight check has to name it.
    #[test]
    fn missing_model_files_names_the_nested_pooling_config() {
        let dir = tempfile::tempdir().unwrap();
        let flat: Vec<&str> = REQUIRED_MODEL_FILES
            .iter()
            .copied()
            .filter(|f| !f.contains('/'))
            .collect();
        populate(dir.path(), &flat);

        assert_eq!(
            missing_model_files(dir.path()),
            vec!["1_Pooling/config.json"]
        );
    }

    #[test]
    fn from_dir_reports_the_missing_files_instead_of_loading() {
        let dir = tempfile::tempdir().unwrap();

        let err = EmbeddingService::from_dir(dir.path()).expect_err("incomplete dir must fail");
        let message = format!("{err:#}");

        assert!(message.contains("rust_model.ot"), "got: {message}");
        assert!(message.contains("1_Pooling/config.json"), "got: {message}");
    }

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
