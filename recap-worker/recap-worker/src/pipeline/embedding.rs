use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use candle_core::{DType, Device, Tensor};
use candle_nn::VarBuilder;
use candle_transformers::models::bert::{BertModel, Config};
use rand::{RngExt, SeedableRng, rngs::StdRng};
use tokenizers::{PaddingParams, PaddingStrategy, Tokenizer, TruncationParams};
use tracing::warn;

/// Where the image bakes the sentence-transformers model directory.
const DEFAULT_MODEL_DIR: &str = "/opt/models/all-MiniLM-L12-v2";

/// sentence-transformers truncates all-MiniLM-L12-v2 at 128 tokens; a longer
/// window changes the pooled vector and breaks parity with the reference.
const MAX_SEQUENCE_LENGTH: usize = 128;

/// The files the candle BERT backend reads out of the model directory.
///
/// Listing them lets a mis-built image fail with a message naming the missing
/// file, rather than with a bare serde or safetensors error from deep inside
/// the load path.
pub(crate) const REQUIRED_MODEL_FILES: &[&str] =
    &["config.json", "tokenizer.json", "model.safetensors"];

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

/// Whether the embedding service is a hard requirement at startup.
///
/// `Required` mirrors the Settings-validator fail-closed pattern established in
/// ADR-000825 (recap-subworker joblib artefacts): the runtime must refuse to
/// start when the embedding model cannot initialize. The alternative — `Optional`
/// — keeps the pre-existing degraded keyword-only behaviour for dev/test stacks
/// whose image does not bake the model directory.
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

struct Inner {
    model: BertModel,
    tokenizer: Tokenizer,
    dir: PathBuf,
}

impl Inner {
    /// Tokenize, run BERT and pool, all on the calling (blocking) thread.
    fn embed(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        let inputs: Vec<&str> = texts.iter().map(String::as_str).collect();
        let encodings = self
            .tokenizer
            .encode_batch(inputs, true)
            .map_err(|e| anyhow::anyhow!("{e}"))
            .context("tokenizer failed to encode batch")?;

        let batch = encodings.len();
        let sequence = encodings.first().map_or(0, |e| e.get_ids().len());
        let mut ids = Vec::with_capacity(batch * sequence);
        let mut mask = Vec::with_capacity(batch * sequence);
        for encoding in &encodings {
            ids.extend_from_slice(encoding.get_ids());
            mask.extend_from_slice(encoding.get_attention_mask());
        }

        let device = Device::Cpu;
        let input_ids = Tensor::from_vec(ids, (batch, sequence), &device)?;
        let attention_mask = Tensor::from_vec(mask, (batch, sequence), &device)?;
        let token_type_ids = input_ids.zeros_like()?;

        let token_embeddings =
            self.model
                .forward(&input_ids, &token_type_ids, Some(&attention_mask))?;

        Ok(masked_mean_pool_l2(&token_embeddings, &attention_mask)?)
    }
}

/// Embedding generation service backed by candle. This runs on CPU.
#[derive(Clone)]
pub struct EmbeddingService {
    inner: Arc<Inner>,
}

impl std::fmt::Debug for EmbeddingService {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("EmbeddingService")
            .field("model_dir", &self.inner.dir)
            .finish()
    }
}

impl EmbeddingService {
    /// Load the embedding model from the directory baked into the image.
    ///
    /// Blocking and CPU-heavy (~130 MB of weights) — call it from
    /// `spawn_blocking`, never directly on an async worker. Everything is read
    /// from disk; this path never touches the network.
    pub fn new() -> Result<Self> {
        Self::from_dir(&configured_model_dir())
    }

    /// Load the model from an explicit directory. Fails with the names of any
    /// missing files rather than with an error from deep inside the load path.
    pub(crate) fn from_dir(dir: &Path) -> Result<Self> {
        let missing = missing_model_files(dir);
        if !missing.is_empty() {
            anyhow::bail!(
                "sentence-embeddings model directory {} is incomplete; missing: {}",
                dir.display(),
                missing.join(", ")
            );
        }

        let config_path = dir.join("config.json");
        let config_json = std::fs::read_to_string(&config_path)
            .with_context(|| format!("failed to read {}", config_path.display()))?;
        let config: Config = serde_json::from_str(&config_json)
            .with_context(|| format!("failed to parse {}", config_path.display()))?;

        let tokenizer_path = dir.join("tokenizer.json");
        let mut tokenizer = Tokenizer::from_file(&tokenizer_path)
            .map_err(|e| anyhow::anyhow!("{e}"))
            .with_context(|| format!("failed to load {}", tokenizer_path.display()))?;
        configure_tokenizer(&mut tokenizer)
            .with_context(|| format!("failed to configure {}", tokenizer_path.display()))?;

        let weights_path = dir.join("model.safetensors");
        // Sound only while nothing writes the file under the mapping: the
        // image bakes it read-only, and the env override exists for tests that
        // point at a directory nothing else touches.
        let vb = unsafe {
            VarBuilder::from_mmaped_safetensors(&[&weights_path], DType::F32, &Device::Cpu)
        }
        .with_context(|| format!("failed to mmap {}", weights_path.display()))?;
        let model = BertModel::load(vb, &config).with_context(|| {
            format!(
                "failed to load BERT weights from {}",
                weights_path.display()
            )
        })?;

        Ok(Self {
            inner: Arc::new(Inner {
                model,
                tokenizer,
                dir: dir.to_path_buf(),
            }),
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
        if texts.is_empty() {
            return Ok(Vec::new());
        }

        let inner = self.inner.clone();
        let texts_clone = texts.to_vec();

        // Offload to blocking thread
        let batch_result = tokio::task::spawn_blocking(move || inner.embed(&texts_clone))
            .await
            .context("embedding task panicked or was cancelled")?;

        Self::resolve_batch_result(batch_result, texts)
    }

    /// Turn a raw model-encode outcome into the batch's embeddings.
    ///
    /// Extracted from `encode` so the failure-propagation behaviour is
    /// testable without loading the real model. A per-text zero-norm output
    /// still gets an individual deterministic repair (a narrow
    /// numerical-stability guard on an otherwise-successful batch, not a
    /// blanket "pretend the model worked" fallback for the whole request).
    fn resolve_batch_result(
        batch_result: Result<Vec<Vec<f32>>>,
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

/// Pin the batch tokenizer to the sentence-transformers geometry.
///
/// The shipped `tokenizer.json` pads every input to a fixed 128 tokens. The
/// attention mask already keeps pad positions out of both attention and
/// pooling, so padding to the batch's longest sequence changes no vector; it
/// only stops BERT from running over ~120 pad positions per short text.
fn configure_tokenizer(tokenizer: &mut Tokenizer) -> Result<()> {
    let padding = tokenizer.get_padding().map_or_else(
        || PaddingParams {
            strategy: PaddingStrategy::BatchLongest,
            ..PaddingParams::default()
        },
        |existing| PaddingParams {
            strategy: PaddingStrategy::BatchLongest,
            ..existing.clone()
        },
    );
    tokenizer.with_padding(Some(padding));

    let truncation = tokenizer.get_truncation().map_or_else(
        || TruncationParams {
            max_length: MAX_SEQUENCE_LENGTH,
            ..TruncationParams::default()
        },
        |existing| TruncationParams {
            max_length: MAX_SEQUENCE_LENGTH,
            ..existing.clone()
        },
    );
    tokenizer
        .with_truncation(Some(truncation))
        .map_err(|e| anyhow::anyhow!("{e}"))?;

    Ok(())
}

/// Mean-pool `token_embeddings` `[batch, seq, hidden]` over the positions where
/// `attention_mask` `[batch, seq]` is 1, then L2-normalize each row — the
/// pooling head sentence-transformers applies to all-MiniLM-L12-v2.
///
/// Both divisors are floored so an all-padding row yields an exact zero vector
/// instead of NaN.
pub(crate) fn masked_mean_pool_l2(
    token_embeddings: &Tensor,
    attention_mask: &Tensor,
) -> candle_core::Result<Vec<Vec<f32>>> {
    let mask = attention_mask.to_dtype(DType::F32)?;
    let summed = token_embeddings
        .broadcast_mul(&mask.unsqueeze(2)?)?
        .sum(1)?;
    let counts = mask.sum_keepdim(1)?.maximum(1e-9f32)?;
    let mean = summed.broadcast_div(&counts)?;
    let norms = mean.sqr()?.sum_keepdim(1)?.sqrt()?.maximum(1e-12f32)?;

    mean.broadcast_div(&norms)?.to_vec2::<f32>()
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
        EmbeddingAvailability, EmbeddingService, REQUIRED_MODEL_FILES, ReferenceFixture,
        cosine_similarity, masked_mean_pool_l2, missing_model_files, reference_fixture,
        require_or_degrade, verify_against_reference,
    };
    use candle_core::{Device, Tensor};
    use std::path::{Path, PathBuf};

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
        for value in &pooled[0] {
            assert!(
                (value - std::f32::consts::FRAC_1_SQRT_2).abs() < 1e-4,
                "got: {:?}",
                pooled[0]
            );
        }
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
        assert!(
            pooled[1].iter().all(|v| v.is_finite()),
            "got: {:?}",
            pooled[1]
        );
        assert_eq!(pooled[1], vec![0.0f32; 2]);
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

    #[test]
    fn reference_fixture_parses_the_checked_in_json() {
        let fixture: ReferenceFixture = reference_fixture().expect("checked-in fixture must parse");

        assert_eq!(fixture.dimension, 384);
        assert_eq!(fixture.items.len(), 8);
        for (i, item) in fixture.items.iter().enumerate() {
            assert_eq!(item.embedding.len(), 384, "item {i}");
            let norm = l2_norm(&item.embedding);
            assert!((norm - 1.0).abs() < 1e-3, "item {i} norm {norm}");
        }
    }

    #[test]
    fn verify_against_reference_accepts_the_reference_itself() {
        let fixture = reference_fixture().unwrap();
        let rows: Vec<Vec<f32>> = fixture.items.iter().map(|i| i.embedding.clone()).collect();

        verify_against_reference(&rows, &fixture)
            .expect("the reference must verify against itself");
    }

    #[test]
    fn verify_against_reference_rejects_a_row_count_mismatch() {
        let fixture = reference_fixture().unwrap();
        let rows: Vec<Vec<f32>> = fixture
            .items
            .iter()
            .skip(1)
            .map(|i| i.embedding.clone())
            .collect();

        let err = verify_against_reference(&rows, &fixture).expect_err("row count must be checked");
        let message = format!("{err:#}");

        assert!(
            message.contains(&fixture.items.len().to_string()),
            "got: {message}"
        );
    }

    #[test]
    fn verify_against_reference_rejects_a_dimension_mismatch() {
        let fixture = reference_fixture().unwrap();
        let mut rows: Vec<Vec<f32>> = fixture.items.iter().map(|i| i.embedding.clone()).collect();
        rows[2].truncate(128);

        let err = verify_against_reference(&rows, &fixture).expect_err("dimension must be checked");
        let message = format!("{err:#}");

        assert!(
            message.contains('2'),
            "message must name the item index, got: {message}"
        );
        assert!(message.contains("128"), "got: {message}");
    }

    #[test]
    fn verify_against_reference_rejects_a_drifted_row() {
        let fixture = reference_fixture().unwrap();
        let mut rows: Vec<Vec<f32>> = fixture.items.iter().map(|i| i.embedding.clone()).collect();
        for value in &mut rows[3] {
            *value = -*value;
        }

        let err = verify_against_reference(&rows, &fixture).expect_err("drift must be rejected");
        let message = format!("{err:#}");

        assert!(
            message.contains('3'),
            "message must name the item index, got: {message}"
        );
        assert!(message.contains("cosine"), "got: {message}");
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
        let fixture = reference_fixture().expect("parity fixture must parse");

        let svc = EmbeddingService::from_dir(&dir).expect("local all-MiniLM must load");
        let rt = tokio::runtime::Runtime::new().unwrap();
        let texts: Vec<String> = fixture.items.iter().map(|i| i.text.clone()).collect();

        let batched = rt.block_on(svc.encode(&texts)).expect("batched encode");
        verify_against_reference(&batched, &fixture).expect("batched encode must match reference");

        for actual in &batched {
            let norm = l2_norm(actual);
            assert!((norm - 1.0).abs() < 1e-3, "norm {norm} must be unit");
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
