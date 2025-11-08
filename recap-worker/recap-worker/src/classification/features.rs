//! トークン列から特徴量を抽出する。
use std::collections::HashMap;

const FEATURE_VOCAB: [&str; 19] = [
    "人工知能",
    "自動運転",
    "資金調達",
    "投資",
    "決算",
    "政策",
    "政府",
    "遺伝子",
    "医療",
    "量子",
    "サッカー",
    "音楽",
    "confidential computing",
    "cybersecurity",
    "transformer",
    "diplomacy",
    "treaty",
    "economy",
    "business",
];

const FEATURE_IDF: [f32; FEATURE_VOCAB.len()] = [
    1.6, 1.5, 1.4, 1.3, 1.2, 1.3, 1.2, 1.5, 1.4, 1.5, 1.3, 1.3, 1.2, 1.2, 1.5, 1.4, 1.4, 1.2, 1.2,
];

pub const EMBEDDING_DIM: usize = 6;

const EMBEDDING_LOOKUP: [(&str, [f32; EMBEDDING_DIM]); 19] = [
    ("人工知能", [1.0, 0.0, 0.0, 0.0, 0.0, 0.0]),
    ("自動運転", [1.0, 0.0, 0.0, 0.0, 0.0, 0.0]),
    ("transformer", [1.0, 0.0, 0.0, 0.0, 0.0, 0.0]),
    ("資金調達", [0.0, 1.0, 0.0, 0.0, 0.0, 0.0]),
    ("投資", [0.0, 1.0, 0.0, 0.0, 0.0, 0.0]),
    ("決算", [0.0, 1.0, 0.0, 0.0, 0.0, 0.0]),
    ("economy", [0.0, 1.0, 0.0, 0.0, 0.0, 0.0]),
    ("business", [0.0, 1.0, 0.0, 0.0, 0.0, 0.0]),
    ("政策", [0.0, 0.0, 1.0, 0.0, 0.0, 0.0]),
    ("政府", [0.0, 0.0, 1.0, 0.0, 0.0, 0.0]),
    ("diplomacy", [0.0, 0.3, 0.8, 0.0, 0.0, 0.0]),
    ("treaty", [0.0, 0.3, 0.8, 0.0, 0.0, 0.0]),
    ("遺伝子", [0.0, 0.0, 0.0, 1.0, 0.0, 0.0]),
    ("医療", [0.0, 0.0, 0.0, 1.0, 0.0, 0.0]),
    ("量子", [0.4, 0.1, 0.0, 0.9, 0.0, 0.0]),
    ("サッカー", [0.0, 0.0, 0.0, 0.0, 1.0, 0.0]),
    ("音楽", [0.0, 0.0, 0.0, 0.0, 0.0, 1.0]),
    ("confidential computing", [0.8, 0.3, 0.0, 0.0, 0.0, 0.0]),
    ("cybersecurity", [0.8, 0.2, 0.0, 0.0, 0.0, 0.0]),
];

#[derive(Debug, Clone)]
pub struct FeatureVector {
    pub tfidf: Vec<f32>,
    pub embedding: Vec<f32>,
}

#[derive(Debug, Default, Clone)]
pub struct FeatureExtractor {
    vocab_index: HashMap<&'static str, usize>,
    embedding_index: HashMap<&'static str, [f32; EMBEDDING_DIM]>,
}

impl FeatureExtractor {
    #[must_use]
    pub fn new() -> Self {
        let vocab_index = FEATURE_VOCAB
            .iter()
            .enumerate()
            .map(|(idx, term)| (*term, idx))
            .collect();
        let embedding_index = EMBEDDING_LOOKUP
            .iter()
            .map(|(term, vec)| (*term, *vec))
            .collect();
        Self {
            vocab_index,
            embedding_index,
        }
    }

    #[must_use]
    pub fn extract(&self, tokens: &[String]) -> FeatureVector {
        let mut counts = vec![0.0f32; FEATURE_VOCAB.len()];
        let mut hits = 0.0f32;
        let mut embedding = vec![0.0f32; EMBEDDING_DIM];
        let mut embedding_hits = 0.0f32;

        for token in tokens {
            let lowered = token.to_lowercase();
            if let Some(&index) = self.vocab_index.get(lowered.as_str()) {
                counts[index] += 1.0;
                hits += 1.0;
            }
            if let Some(vector) = self.embedding_index.get(lowered.as_str()) {
                for (slot, value) in embedding.iter_mut().zip(vector.iter()) {
                    *slot += value;
                }
                embedding_hits += 1.0;
            }
        }

        if hits > 0.0 {
            for (idx, value) in counts.iter_mut().enumerate() {
                let tf = *value / hits;
                *value = tf * FEATURE_IDF[idx];
            }
        }

        if embedding_hits > 0.0 {
            for value in &mut embedding {
                *value /= embedding_hits;
            }
        }

        FeatureVector {
            tfidf: counts,
            embedding,
        }
    }
}
