// Candle を用いた軽量ハイブリッド分類モデル。
use std::fs;
use std::path::Path;

use anyhow::{Context, Result};
use serde::Deserialize;

use super::features::{
    FeatureVector, EMBEDDING_DIM, FALLBACK_AVG_DOC_LEN, FALLBACK_BM25_B, FALLBACK_BM25_K1,
    FALLBACK_IDF, FALLBACK_VOCAB,
};

const DEFAULT_WEIGHTS_JSON: &str = include_str!("../resources/genre_classifier_weights.json");

#[derive(Debug, Deserialize)]
struct ModelWeights {
    feature_dim: usize,
    embedding_dim: usize,
    #[serde(default)]
    feature_vocab: Vec<String>,
    #[serde(default)]
    feature_idf: Vec<f32>,
    #[serde(default)]
    bm25_k1: Option<f32>,
    #[serde(default)]
    bm25_b: Option<f32>,
    #[serde(default)]
    average_doc_len: Option<f32>,
    genres: Vec<String>,
    tfidf_weights: Vec<Vec<f32>>,
    embedding_weights: Vec<Vec<f32>>,
    bias: Vec<f32>,
}

impl ModelWeights {
    fn validate(&self) -> Result<()> {
        anyhow::ensure!(
            self.tfidf_weights.len() == self.genres.len(),
            "tfidf weight matrix row count mismatch"
        );
        anyhow::ensure!(
            self.embedding_weights.len() == self.genres.len(),
            "embedding weight matrix row count mismatch"
        );
        for row in &self.tfidf_weights {
            anyhow::ensure!(
                row.len() == self.feature_dim,
                "tfidf weight row length mismatch"
            );
        }
        for row in &self.embedding_weights {
            anyhow::ensure!(
                row.len() == self.embedding_dim,
                "embedding weight row length mismatch"
            );
        }
        anyhow::ensure!(self.bias.len() == self.genres.len(), "bias length mismatch");
        if !self.feature_vocab.is_empty() {
            anyhow::ensure!(
                self.feature_vocab.len() == self.feature_dim,
                "feature vocab length mismatch"
            );
        }
        if !self.feature_idf.is_empty() {
            anyhow::ensure!(
                self.feature_idf.len() == self.feature_dim,
                "feature idf length mismatch"
            );
        }
        Ok(())
    }
}

#[derive(Debug)]
pub struct HybridModel {
    genres: Vec<String>,
    feature_dim: usize,
    feature_vocab: Vec<String>,
    feature_idf: Vec<f32>,
    bm25_k1: f32,
    bm25_b: f32,
    average_doc_len: f32,
    tfidf_weight: Vec<Vec<f32>>,
    embedding_weight: Vec<Vec<f32>>,
    bias: Vec<f32>,
}

impl HybridModel {
    pub fn new() -> Result<Self> {
        Self::from_path::<&Path>(None)
    }

    pub fn from_path<P: AsRef<Path>>(path: Option<P>) -> Result<Self> {
        let raw = if let Some(path) = path {
            fs::read_to_string(path.as_ref())
                .with_context(|| format!("failed to read weights from {:?}", path.as_ref()))?
        } else {
            DEFAULT_WEIGHTS_JSON.to_string()
        };
        let weights: ModelWeights =
            serde_json::from_str(&raw).context("failed to parse classifier weights json")?;
        weights.validate()?;

        let feature_vocab = if weights.feature_vocab.is_empty() {
            FALLBACK_VOCAB.iter().map(|s| s.to_string()).collect()
        } else {
            weights.feature_vocab.clone()
        };
        let feature_idf = if weights.feature_idf.is_empty() {
            FALLBACK_IDF.to_vec()
        } else {
            weights.feature_idf.clone()
        };
        let bm25_k1 = weights.bm25_k1.unwrap_or(FALLBACK_BM25_K1);
        let bm25_b = weights.bm25_b.unwrap_or(FALLBACK_BM25_B);
        let average_doc_len = weights.average_doc_len.unwrap_or(FALLBACK_AVG_DOC_LEN);

        Ok(Self {
            genres: weights.genres,
            feature_dim: weights.feature_dim,
            feature_vocab,
            feature_idf,
            bm25_k1,
            bm25_b,
            average_doc_len,
            tfidf_weight: weights.tfidf_weights,
            embedding_weight: weights.embedding_weights,
            bias: weights.bias,
        })
    }

    pub fn predict(&self, features: &FeatureVector) -> Result<Vec<(String, f32)>> {
        anyhow::ensure!(
            features.tfidf.len() == self.feature_dim,
            "feature dimension mismatch: expected {}, got {}",
            self.feature_dim,
            features.tfidf.len()
        );
        anyhow::ensure!(
            features.embedding.len() == EMBEDDING_DIM,
            "embedding dimension mismatch"
        );

        let mut paired = Vec::with_capacity(self.genres.len());
        for (idx, genre) in self.genres.iter().enumerate() {
            let mut score = self.bias[idx];
            for (feature_value, weight) in features.tfidf.iter().zip(&self.tfidf_weight[idx]) {
                score += feature_value * weight;
            }
            for (embed_value, weight) in features.embedding.iter().zip(&self.embedding_weight[idx])
            {
                score += embed_value * weight;
            }
            paired.push((genre.clone(), score));
        }
        Ok(paired)
    }

    #[must_use]
    pub fn feature_vocab(&self) -> &[String] {
        &self.feature_vocab
    }

    #[must_use]
    pub fn feature_idf(&self) -> &[f32] {
        &self.feature_idf
    }

    #[must_use]
    pub fn bm25_k1(&self) -> f32 {
        self.bm25_k1
    }

    #[must_use]
    pub fn bm25_b(&self) -> f32 {
        self.bm25_b
    }

    #[must_use]
    pub fn average_doc_len(&self) -> f32 {
        self.average_doc_len
    }
}
