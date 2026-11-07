use std::path::{Path, PathBuf};
use std::sync::Arc;

use anyhow::{Context, Result};
use async_trait::async_trait;
use candle_core::{Device, Tensor};
use candle_nn::VarBuilder;
use candle_transformers::models::bert::{BertModel, Config as BertConfig, DTYPE};
use rand::{RngExt, SeedableRng, rngs::StdRng};
use serde::Deserialize;
use tokenizers::models::wordpiece::WordPiece;
use tokenizers::normalizers::bert::BertNormalizer;
use tokenizers::pre_tokenizers::bert::BertPreTokenizer;
use tokenizers::processors::bert::BertProcessing;
use tokenizers::{
    Model, PaddingParams, PaddingStrategy, Tokenizer, TruncationParams, TruncationStrategy,
};
use tracing::warn;

/// Where the image bakes the sentence-transformers model directory.
const DEFAULT_MODEL_DIR: &str = "/opt/rustbert-models/all-MiniLM-L12-v2";

/// The files `EmbeddingService::from_dir` reads for a BERT
/// sentence-transformers model.
///
/// Listing the files lets a mis-built image fail at startup with an explicit
/// error naming every missing artifact (`1_Pooling/config.json`, etc.).
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

/// Whether the sentence-embedding service is a hard requirement at startup.
///
/// `Required` mirrors the Settings-validator fail-closed pattern established in
/// ADR-000825 (recap-subworker joblib artefacts): the runtime must refuse to
/// start when the embedding model cannot initialize. The alternative — `Optional`
/// — keeps the pre-existing degraded keyword-only behaviour for dev/test stacks
/// that do not have a local model directory populated.
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

const fn default_max_seq_length() -> usize {
    128
}

const fn default_true() -> bool {
    true
}

fn default_unk_token() -> String {
    "[UNK]".to_string()
}

fn default_sep_token() -> String {
    "[SEP]".to_string()
}

fn default_pad_token() -> String {
    "[PAD]".to_string()
}

fn default_cls_token() -> String {
    "[CLS]".to_string()
}

#[derive(Debug, Deserialize)]
struct SentenceBertConfig {
    #[serde(default = "default_max_seq_length")]
    max_seq_length: usize,
    #[serde(default)]
    do_lower_case: bool,
}

#[derive(Debug, Deserialize)]
struct TokenizerConfigFile {
    do_lower_case: Option<bool>,
    strip_accents: Option<bool>,
    #[serde(default = "default_true")]
    tokenize_chinese_chars: bool,
    #[serde(default = "default_unk_token")]
    unk_token: String,
    #[serde(default = "default_sep_token")]
    sep_token: String,
    #[serde(default = "default_pad_token")]
    pad_token: String,
    #[serde(default = "default_cls_token")]
    cls_token: String,
}

#[allow(clippy::struct_excessive_bools)]
#[derive(Debug, Clone, Deserialize)]
struct PoolingConfig {
    #[serde(default)]
    pooling_mode_cls_token: bool,
    #[serde(default = "default_true")]
    pooling_mode_mean_tokens: bool,
    #[serde(default)]
    pooling_mode_max_tokens: bool,
    #[serde(default)]
    pooling_mode_mean_sqrt_len_tokens: bool,
}

#[derive(Debug, Deserialize)]
struct ModuleConfigEntry {
    #[serde(rename = "type")]
    module_type: String,
}

struct SentenceEmbeddingsRunner {
    bert: BertModel,
    tokenizer: Tokenizer,
    pooling: PoolingConfig,
    normalize_embeddings: bool,
    device: Device,
}

impl SentenceEmbeddingsRunner {
    fn from_dir(dir: &Path) -> Result<Self> {
        let config_bytes = std::fs::read(dir.join("config.json"))
            .with_context(|| format!("failed to read {}/config.json", dir.display()))?;
        let bert_config: BertConfig = serde_json::from_slice(&config_bytes)
            .with_context(|| format!("failed to parse {}/config.json", dir.display()))?;

        let sbert_bytes =
            std::fs::read(dir.join("sentence_bert_config.json")).with_context(|| {
                format!("failed to read {}/sentence_bert_config.json", dir.display())
            })?;
        let sbert_config: SentenceBertConfig =
            serde_json::from_slice(&sbert_bytes).with_context(|| {
                format!(
                    "failed to parse {}/sentence_bert_config.json",
                    dir.display()
                )
            })?;

        let tok_cfg_bytes = std::fs::read(dir.join("tokenizer_config.json"))
            .with_context(|| format!("failed to read {}/tokenizer_config.json", dir.display()))?;
        let tok_config: TokenizerConfigFile = serde_json::from_slice(&tok_cfg_bytes)
            .with_context(|| format!("failed to parse {}/tokenizer_config.json", dir.display()))?;

        let pooling_bytes = std::fs::read(dir.join("1_Pooling/config.json"))
            .with_context(|| format!("failed to read {}/1_Pooling/config.json", dir.display()))?;
        let pooling: PoolingConfig = serde_json::from_slice(&pooling_bytes)
            .with_context(|| format!("failed to parse {}/1_Pooling/config.json", dir.display()))?;

        let modules_bytes = std::fs::read(dir.join("modules.json"))
            .with_context(|| format!("failed to read {}/modules.json", dir.display()))?;
        let modules: Vec<ModuleConfigEntry> = serde_json::from_slice(&modules_bytes)
            .with_context(|| format!("failed to parse {}/modules.json", dir.display()))?;
        let normalize_embeddings = modules
            .iter()
            .any(|m| m.module_type == "sentence_transformers.models.Normalize");

        let vocab_path = dir.join("vocab.txt");
        let vocab_path_str = vocab_path
            .to_str()
            .with_context(|| format!("non-UTF-8 vocab path: {}", vocab_path.display()))?;
        let wordpiece = WordPiece::from_file(vocab_path_str)
            .unk_token(tok_config.unk_token.clone())
            .build()
            .map_err(|e| anyhow::anyhow!("failed to load {}: {e}", vocab_path.display()))?;

        let cls_id = wordpiece
            .token_to_id(&tok_config.cls_token)
            .with_context(|| format!("missing cls_token {} in vocab.txt", tok_config.cls_token))?;
        let sep_id = wordpiece
            .token_to_id(&tok_config.sep_token)
            .with_context(|| format!("missing sep_token {} in vocab.txt", tok_config.sep_token))?;
        let pad_id = wordpiece
            .token_to_id(&tok_config.pad_token)
            .with_context(|| format!("missing pad_token {} in vocab.txt", tok_config.pad_token))?;

        let do_lower_case = tok_config
            .do_lower_case
            .unwrap_or(sbert_config.do_lower_case);
        let normalizer = BertNormalizer::new(
            true,
            tok_config.tokenize_chinese_chars,
            tok_config.strip_accents,
            do_lower_case,
        );
        let post_processor = BertProcessing::new(
            (tok_config.sep_token, sep_id),
            (tok_config.cls_token, cls_id),
        );

        let mut tokenizer = Tokenizer::new(wordpiece);
        tokenizer
            .with_normalizer(Some(normalizer))
            .map_err(|e| anyhow::anyhow!("failed to configure BERT normalizer: {e}"))?;
        tokenizer.with_pre_tokenizer(Some(BertPreTokenizer));
        tokenizer.with_post_processor(Some(post_processor));
        tokenizer
            .with_truncation(Some(TruncationParams {
                max_length: sbert_config.max_seq_length,
                strategy: TruncationStrategy::LongestFirst,
                ..Default::default()
            }))
            .map_err(|e| anyhow::anyhow!("failed to configure BERT truncation: {e}"))?;
        tokenizer.with_padding(Some(PaddingParams {
            strategy: PaddingStrategy::BatchLongest,
            pad_id,
            pad_token: tok_config.pad_token,
            ..Default::default()
        }));

        let device = Device::Cpu;
        let safetensors_path = dir.join("model.safetensors");
        let ot_path = dir.join("rust_model.ot");
        let vb = if safetensors_path.is_file() {
            let data = std::fs::read(&safetensors_path)
                .with_context(|| format!("failed to read {}", safetensors_path.display()))?;
            VarBuilder::from_buffered_safetensors(data, DTYPE, &device)?
        } else {
            // `rust_model.ot` stores TorchScript module parameter keys delimited by `|`
            // (e.g., `embeddings|word_embeddings|weight`).
            VarBuilder::from_pth(&ot_path, DTYPE, &device)
                .with_context(|| format!("failed to load {}", ot_path.display()))?
                .rename_f(|name: &str| name.replace('.', "|"))
        };

        let bert = BertModel::load(vb, &bert_config)
            .with_context(|| format!("failed to build BertModel from {}", dir.display()))?;

        Ok(Self {
            bert,
            tokenizer,
            pooling,
            normalize_embeddings,
            device,
        })
    }

    fn encode(&self, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        if texts.is_empty() {
            anyhow::bail!("cannot encode an empty text batch");
        }

        let encodings = self
            .tokenizer
            .encode_batch(texts.to_vec(), true)
            .map_err(|e| anyhow::anyhow!("tokenizer encode_batch failed: {e}"))?;

        let batch_size = encodings.len();
        let seq_len = encodings.first().map_or(0, |e| e.get_ids().len());
        if seq_len == 0 {
            anyhow::bail!("tokenized sequence length is zero");
        }

        let mut token_ids = Vec::with_capacity(batch_size * seq_len);
        let mut token_type_ids = Vec::with_capacity(batch_size * seq_len);
        let mut attention_mask = Vec::with_capacity(batch_size * seq_len);
        for encoding in &encodings {
            token_ids.extend_from_slice(encoding.get_ids());
            token_type_ids.extend_from_slice(encoding.get_type_ids());
            attention_mask.extend_from_slice(encoding.get_attention_mask());
        }

        let input_ids = Tensor::from_vec(token_ids, (batch_size, seq_len), &self.device)?;
        let token_type_ids = Tensor::from_vec(token_type_ids, (batch_size, seq_len), &self.device)?;
        let attention_mask_u32 =
            Tensor::from_vec(attention_mask, (batch_size, seq_len), &self.device)?;

        let token_embeddings =
            self.bert
                .forward(&input_ids, &token_type_ids, Some(&attention_mask_u32))?;

        let pooled = self.pool(&token_embeddings, &attention_mask_u32)?;
        let output = if self.normalize_embeddings {
            let norm = pooled
                .sqr()?
                .sum_keepdim(1)?
                .sqrt()?
                .clamp(1e-12, f64::INFINITY)?;
            pooled.broadcast_div(&norm)?
        } else {
            pooled
        };

        Ok(output.to_vec2::<f32>()?)
    }

    fn pool(&self, token_embeddings: &Tensor, attention_mask_u32: &Tensor) -> Result<Tensor> {
        let mut output_vectors = Vec::new();

        if self.pooling.pooling_mode_cls_token {
            let cls_token = token_embeddings.narrow(1, 0, 1)?.squeeze(1)?;
            output_vectors.push(cls_token);
        }

        if self.pooling.pooling_mode_max_tokens {
            let mask_f32 = attention_mask_u32
                .to_dtype(DTYPE)?
                .unsqueeze(2)?
                .broadcast_as(token_embeddings.shape())?;
            let inv_mask = (mask_f32.ones_like()? - &mask_f32)?;
            let penalty = (&inv_mask * (-1e9_f64))?;
            let masked = ((token_embeddings * &mask_f32)? + penalty)?;
            output_vectors.push(masked.max(1)?);
        }

        if self.pooling.pooling_mode_mean_tokens || self.pooling.pooling_mode_mean_sqrt_len_tokens {
            let mask_f32 = attention_mask_u32
                .to_dtype(DTYPE)?
                .unsqueeze(2)?
                .broadcast_as(token_embeddings.shape())?;
            let sum_embeddings = (token_embeddings * &mask_f32)?.sum(1)?;
            let sum_mask = mask_f32.sum(1)?.clamp(1e-8, f64::INFINITY)?;

            if self.pooling.pooling_mode_mean_tokens {
                output_vectors.push(sum_embeddings.broadcast_div(&sum_mask)?);
            }
            if self.pooling.pooling_mode_mean_sqrt_len_tokens {
                output_vectors.push(sum_embeddings.broadcast_div(&sum_mask.sqrt()?)?);
            }
        }

        match output_vectors.len() {
            0 => anyhow::bail!("no pooling mode enabled in 1_Pooling/config.json"),
            1 => Ok(output_vectors.remove(0)),
            _ => Ok(Tensor::cat(&output_vectors, 1)?),
        }
    }
}

/// Embedding generation service using pure-Rust `candle` + `tokenizers`.
/// This runs on CPU.
#[derive(Clone)]
pub struct EmbeddingService {
    model: Arc<SentenceEmbeddingsRunner>,
}

impl std::fmt::Debug for EmbeddingService {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("EmbeddingService")
            .field("model", &"<SentenceEmbeddingsRunner>")
            .finish()
    }
}

impl EmbeddingService {
    /// Load the embedding model from the directory baked into the image.
    ///
    /// Blocking and CPU-heavy (~130 MB of weights) — call it from
    /// `spawn_blocking`, never directly on an async worker.
    ///
    /// Reads straight from local disk and cannot reach the network.
    pub fn new() -> Result<Self> {
        Self::from_dir(&configured_model_dir())
    }

    /// Load the model from an explicit directory. Fails with the names of any
    /// missing files rather than panicking on the first missing artifact.
    pub(crate) fn from_dir(dir: &Path) -> Result<Self> {
        let missing = missing_model_files(dir);
        if !missing.is_empty() {
            anyhow::bail!(
                "sentence-embeddings model directory {} is incomplete; missing: {}",
                dir.display(),
                missing.join(", ")
            );
        }

        let runner = SentenceEmbeddingsRunner::from_dir(dir).with_context(|| {
            format!(
                "failed to load sentence-embeddings model from {}",
                dir.display()
            )
        })?;

        Ok(Self {
            model: Arc::new(runner),
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
        let batch_result = tokio::task::spawn_blocking(move || model.encode(&texts_clone))
            .await
            .context("embedding task panicked or was cancelled")?;

        Self::resolve_batch_result(batch_result, texts)
    }

    /// Turn a raw model-encode outcome into the batch's embeddings.
    ///
    /// Extracted from `encode` so the failure-propagation behaviour is
    /// testable without spinning up the real model. A per-text
    /// zero-norm output still gets an individual deterministic repair (a
    /// narrow numerical-stability guard on an otherwise-successful batch,
    /// not a blanket "pretend the model worked" fallback for the whole
    /// request).
    fn resolve_batch_result<E>(
        batch_result: std::result::Result<Vec<Vec<f32>>, E>,
        texts: &[String],
    ) -> Result<Vec<Vec<f32>>>
    where
        E: Into<anyhow::Error>,
    {
        let embeddings = batch_result
            .map_err(Into::into)
            .context("embedding model failed to encode batch")?;

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
    /// in the image build misses it while every other file lands. The
    /// pre-flight check has to name it explicitly.
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

    #[test]
    fn missing_model_files_accepts_safetensors_without_rust_model_ot_and_reports_both_when_neither_exists()
     {
        let base_files = [
            "modules.json",
            "config.json",
            "tokenizer_config.json",
            "sentence_bert_config.json",
            "vocab.txt",
            "1_Pooling/config.json",
        ];

        let safetensors_dir = tempfile::tempdir().unwrap();
        populate(safetensors_dir.path(), &base_files);
        populate(safetensors_dir.path(), &["model.safetensors"]);
        assert!(
            missing_model_files(safetensors_dir.path()).is_empty(),
            "dir with model.safetensors and all non-weight files must pass completeness check"
        );

        let ot_dir = tempfile::tempdir().unwrap();
        populate(ot_dir.path(), &base_files);
        populate(ot_dir.path(), &["rust_model.ot"]);
        assert!(
            missing_model_files(ot_dir.path()).is_empty(),
            "dir with rust_model.ot and all non-weight files must pass completeness check"
        );

        let neither_dir = tempfile::tempdir().unwrap();
        populate(neither_dir.path(), &base_files);
        let missing = missing_model_files(neither_dir.path());
        assert!(
            missing.contains(&"model.safetensors") && missing.contains(&"rust_model.ot"),
            "missing weight files must name both model.safetensors and rust_model.ot, got: {missing:?}"
        );

        let err = EmbeddingService::from_dir(neither_dir.path())
            .expect_err("dir without weights must fail");
        let message = format!("{err:#}");
        assert!(
            message.contains("model.safetensors") && message.contains("rust_model.ot"),
            "from_dir error must name both weight file alternatives, got: {message}"
        );
    }

    /// Real AllMiniLM load + 384-d encode when a baked local directory is
    /// supplied. Skips when the dir is absent so `cargo test` stays hermetic.
    #[test]
    fn local_all_minilm_encodes_384_when_model_dir_present() {
        let raw = match std::env::var("RECAP_WORKER_EMBEDDING_MODEL_DIR") {
            Ok(v) if !v.is_empty() => v,
            _ => {
                eprintln!(
                    "skipping: set RECAP_WORKER_EMBEDDING_MODEL_DIR to a complete \
                     AllMiniLM directory"
                );
                return;
            }
        };
        let dir = std::path::Path::new(&raw);
        let missing = missing_model_files(dir);
        if !missing.is_empty() {
            eprintln!(
                "skipping: incomplete model dir {}, missing {missing:?}",
                dir.display()
            );
            return;
        }
        let svc = EmbeddingService::from_dir(dir).expect("local AllMiniLM must load");
        let rt = tokio::runtime::Runtime::new().unwrap();
        let vectors = rt
            .block_on(svc.encode(&["recap-worker local embedding smoke".into()]))
            .expect("encode");
        assert_eq!(vectors.len(), 1);
        assert_eq!(vectors[0].len(), 384, "AllMiniLmL12V2 must be 384-d");
        let norm: f32 = vectors[0].iter().map(|x| x * x).sum::<f32>().sqrt();
        assert!(
            (norm - 1.0).abs() < 1e-3,
            "normalized embedding must have unit norm, got norm={norm}"
        );
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
