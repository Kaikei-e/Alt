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
        EmbeddingAvailability, EmbeddingService, REQUIRED_MODEL_FILES, cosine_similarity,
        masked_mean_pool_l2, missing_model_files, require_or_degrade,
    };
    use candle_core::{Device, Tensor};
    use std::path::{Path, PathBuf};

    #[derive(serde::Deserialize)]
    struct ReferenceItem {
        text: String,
        embedding: Vec<f32>,
    }

    #[derive(serde::Deserialize)]
    struct ReferenceFixture {
        dimension: usize,
        items: Vec<ReferenceItem>,
    }

    fn populate(dir: &Path, files: &[&str]) {
        for rel in files {
            let path = dir.join(rel);
            std::fs::create_dir_all(path.parent().unwrap()).unwrap();
            std::fs::write(&path, b"x").unwrap();
        }
    }

    /// A complete model directory from the environment, or `None` after printing
    /// why the caller should skip.
    fn model_dir_from_env() -> Option<PathBuf> {
        let raw = match std::env::var("RECAP_WORKER_EMBEDDING_MODEL_DIR") {
            Ok(v) if !v.is_empty() => v,
            _ => {
                eprintln!(
                    "skipping: set RECAP_WORKER_EMBEDDING_MODEL_DIR to a complete \
                     all-MiniLM-L12-v2 directory"
                );
                return None;
            }
        };
        let dir = PathBuf::from(raw);
        let missing = missing_model_files(&dir);
        if !missing.is_empty() {
            eprintln!(
                "skipping: incomplete model dir {}, missing {missing:?}",
                dir.display()
            );
            return None;
        }
        Some(dir)
    }

    fn l2_norm(v: &[f32]) -> f32 {
        v.iter().map(|x| x * x).sum::<f32>().sqrt()
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

    #[test]
    fn from_dir_reports_the_missing_files_instead_of_loading() {
        let dir = tempfile::tempdir().unwrap();

        let err = EmbeddingService::from_dir(dir.path()).expect_err("incomplete dir must fail");
        let message = format!("{err:#}");

        assert!(message.contains("model.safetensors"), "got: {message}");
        assert!(message.contains("tokenizer.json"), "got: {message}");
    }

    /// Padding tokens must contribute nothing to the mean, and the result must
    /// come back L2-normalized the way sentence-transformers emits it.
    #[test]
    fn masked_mean_pool_l2_ignores_padding_and_normalizes() {
        let token_embeddings =
            Tensor::new(&[[[1f32, 0.0], [0.0, 1.0], [100.0, 100.0]]], &Device::Cpu).unwrap();
        let attention_mask = Tensor::new(&[[1u32, 1, 0]], &Device::Cpu).unwrap();

        let pooled = masked_mean_pool_l2(&token_embeddings, &attention_mask).unwrap();

        assert_eq!(pooled.len(), 1);
        assert_eq!(pooled[0].len(), 2);
        assert!(
            (pooled[0][0] - 0.707_106_8).abs() < 1e-4,
            "got: {:?}",
            pooled[0]
        );
        assert!(
            (pooled[0][1] - 0.707_106_8).abs() < 1e-4,
            "got: {:?}",
            pooled[0]
        );
    }

    /// Rows with different real-token counts must be pooled independently: one
    /// row's padding must never bleed into another row's mean.
    #[test]
    fn masked_mean_pool_l2_handles_rows_independently() {
        let token_embeddings = Tensor::new(
            &[
                [[3f32, 0.0], [0.0, 0.0], [9.0, 9.0]],
                [[0f32, 2.0], [7.0, 7.0], [7.0, 7.0]],
            ],
            &Device::Cpu,
        )
        .unwrap();
        let attention_mask = Tensor::new(&[[1u32, 1, 0], [1, 0, 0]], &Device::Cpu).unwrap();

        let pooled = masked_mean_pool_l2(&token_embeddings, &attention_mask).unwrap();

        assert_eq!(pooled.len(), 2);
        assert!((pooled[0][0] - 1.0).abs() < 1e-4, "got: {:?}", pooled[0]);
        assert!(pooled[0][1].abs() < 1e-4, "got: {:?}", pooled[0]);
        assert!(pooled[1][0].abs() < 1e-4, "got: {:?}", pooled[1]);
        assert!((pooled[1][1] - 1.0).abs() < 1e-4, "got: {:?}", pooled[1]);
        for row in &pooled {
            assert!((l2_norm(row) - 1.0).abs() < 1e-4, "got: {row:?}");
        }
    }

    /// An all-padding row has no tokens to average, so the guard against the
    /// zero divisor must yield a finite zero vector rather than NaN.
    #[test]
    fn masked_mean_pool_l2_yields_zero_vector_for_all_padding_row() {
        let token_embeddings = Tensor::new(
            &[
                [[1f32, 0.0], [0.0, 1.0], [0.0, 0.0]],
                [[5f32, 5.0], [5.0, 5.0], [5.0, 5.0]],
            ],
            &Device::Cpu,
        )
        .unwrap();
        let attention_mask = Tensor::new(&[[1u32, 1, 0], [0, 0, 0]], &Device::Cpu).unwrap();

        let pooled = masked_mean_pool_l2(&token_embeddings, &attention_mask).unwrap();

        assert_eq!(pooled.len(), 2);
        for value in &pooled[1] {
            assert!(value.is_finite(), "got: {:?}", pooled[1]);
            assert_eq!(*value, 0.0, "got: {:?}", pooled[1]);
        }
    }

    #[test]
    fn encode_empty_batch_returns_empty() {
        let Some(dir) = model_dir_from_env() else {
            return;
        };
        let svc = EmbeddingService::from_dir(&dir).expect("local all-MiniLM must load");
        let rt = tokio::runtime::Runtime::new().unwrap();

        let vectors = rt
            .block_on(svc.encode(&[]))
            .expect("empty batch must be Ok");

        assert!(vectors.is_empty(), "got: {vectors:?}");
    }

    /// Numerical parity with the sentence-transformers reference vectors, plus
    /// the batched-vs-single check that catches padding leaking through pooling.
    /// Skips when the baked model directory or the fixture is absent so
    /// `cargo test` stays hermetic.
    #[test]
    fn local_all_minilm_matches_reference_when_model_dir_present() {
        let Some(dir) = model_dir_from_env() else {
            return;
        };
        let fixture_path = concat!(
            env!("CARGO_MANIFEST_DIR"),
            "/tests/data/all_minilm_l12_v2_reference.json"
        );
        let Ok(raw) = std::fs::read_to_string(fixture_path) else {
            eprintln!("skipping: parity fixture {fixture_path} is absent");
            return;
        };
        let fixture: ReferenceFixture =
            serde_json::from_str(&raw).expect("parity fixture must parse");
        assert_eq!(fixture.dimension, 384);
        assert!(!fixture.items.is_empty(), "fixture must carry items");

        let svc = EmbeddingService::from_dir(&dir).expect("local all-MiniLM must load");
        let rt = tokio::runtime::Runtime::new().unwrap();
        let texts: Vec<String> = fixture.items.iter().map(|i| i.text.clone()).collect();

        let batched = rt.block_on(svc.encode(&texts)).expect("batched encode");
        assert_eq!(batched.len(), fixture.items.len());

        for (item, actual) in fixture.items.iter().zip(&batched) {
            assert_eq!(actual.len(), 384, "text: {}", item.text);
            let norm = l2_norm(actual);
            assert!(
                (norm - 1.0).abs() < 1e-3,
                "norm {norm} for text: {}",
                item.text
            );
            let similarity = cosine_similarity(actual, &item.embedding);
            assert!(
                similarity >= 0.999,
                "cosine {similarity} below parity floor for text: {}",
                item.text
            );
        }

        for (i, text) in texts.iter().enumerate() {
            let single = rt
                .block_on(svc.encode(std::slice::from_ref(text)))
                .expect("single encode");
            let similarity = cosine_similarity(&batched[i], &single[0]);
            assert!(
                similarity >= 0.9999,
                "batched/single cosine {similarity} means padding leaked for text: {text}"
            );
        }
    }

    /// PM-2026-038 regression: a model failure must surface as `Err`, never
    /// as a batch of fabricated MD5-seeded random vectors dressed up as
    /// `Ok`. Downstream clustering must be able to tell "no embedding" from
    /// "embedding is noise".
    #[test]
    fn resolve_batch_result_propagates_model_failure_as_err() {
        let texts = vec!["a".to_string(), "b".to_string()];
        let outcome: std::result::Result<Vec<Vec<f32>>, anyhow::Error> =
            Err(anyhow::anyhow!("model unavailable"));

        let result = EmbeddingService::resolve_batch_result(outcome, &texts);

        assert!(
            result.is_err(),
            "model failure must propagate as Err, not a fabricated Ok(..) batch"
        );
    }

    #[test]
    fn resolve_batch_result_passes_through_healthy_embeddings() {
        let texts = vec!["a".to_string()];
        let outcome: std::result::Result<Vec<Vec<f32>>, anyhow::Error> = Ok(vec![vec![1.0, 0.0]]);

        let result = EmbeddingService::resolve_batch_result(outcome, &texts).unwrap();

        assert_eq!(result, vec![vec![1.0, 0.0]]);
    }

    #[test]
    fn resolve_batch_result_repairs_only_zero_norm_rows() {
        let texts = vec!["a".to_string(), "b".to_string()];
        // First row is a legitimate embedding; second is the zero vector a
        // model can emit for degenerate input. Only the zero row should be
        // replaced — the healthy row must survive untouched.
        let outcome: std::result::Result<Vec<Vec<f32>>, anyhow::Error> =
            Ok(vec![vec![0.6, 0.8], vec![0.0, 0.0]]);

        let result = EmbeddingService::resolve_batch_result(outcome, &texts).unwrap();

        assert_eq!(result.len(), 2);
        assert_eq!(result[0], vec![0.6, 0.8]);
        let repaired_norm = l2_norm(&result[1]);
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
