//! トークン列から特徴量を抽出する。
use std::collections::HashMap;
use tracing;
use xxhash_rust::xxh3::xxh3_64;

pub const EMBEDDING_DIM: usize = 6;
pub(crate) const FALLBACK_BM25_K1: f32 = 1.6;
pub(crate) const FALLBACK_BM25_B: f32 = 0.75;
pub(crate) const FALLBACK_AVG_DOC_LEN: f32 = 320.0;

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
    pub bm25: Vec<f32>,
    pub embedding: Vec<f32>,
}

/// Embedding統計情報（Z-score正規化用）
#[derive(Debug, Clone)]
pub struct EmbeddingStats {
    /// 各次元の平均値
    pub mean: Vec<f32>,
    /// 各次元の標準偏差
    pub std: Vec<f32>,
}

impl EmbeddingStats {
    /// 新しいEmbeddingStatsを作成する（統計なし = 正規化なし）
    #[must_use]
    pub fn empty(dim: usize) -> Self {
        Self {
            mean: vec![0.0; dim],
            std: vec![1.0; dim], // 標準偏差1.0 = 正規化なし
        }
    }

    /// Golden Datasetから統計を計算する
    ///
    /// # Arguments
    /// * `embeddings` - Embeddingベクトルのリスト
    ///
    /// # Returns
    /// 計算された統計情報
    pub fn from_embeddings(embeddings: &[Vec<f32>]) -> Self {
        if embeddings.is_empty() {
            return Self::empty(EMBEDDING_DIM);
        }

        let dim = embeddings[0].len();
        let n = embeddings.len() as f32;

        // 平均を計算
        let mut mean = vec![0.0; dim];
        for emb in embeddings {
            for (i, &val) in emb.iter().enumerate() {
                mean[i] += val;
            }
        }
        for m in &mut mean {
            *m /= n;
        }

        // 標準偏差を計算
        let mut std = vec![0.0; dim];
        for emb in embeddings {
            for (i, &val) in emb.iter().enumerate() {
                let diff = val - mean[i];
                std[i] += diff * diff;
            }
        }
        for s in &mut std {
            *s = (*s / n).sqrt();
            // ゼロ除算を防ぐため、最小値を設定
            if *s < 1e-6 {
                *s = 1e-6;
            }
        }

        Self { mean, std }
    }

    /// EmbeddingベクトルにZ-score正規化を適用する
    ///
    /// # Arguments
    /// * `embedding` - 正規化するEmbeddingベクトル（in-place）
    pub fn normalize(&self, embedding: &mut [f32]) {
        for (i, val) in embedding.iter_mut().enumerate() {
            if i < self.mean.len() && i < self.std.len() {
                *val = (*val - self.mean[i]) / self.std[i];
            }
        }
    }
}

#[derive(Debug, Clone)]
pub struct FeatureExtractor {
    vocab_index: HashMap<String, usize>,
    idf: Vec<f32>,
    bm25_k1: f32,
    bm25_b: f32,
    average_doc_len: f32,
    embedding_index: HashMap<&'static str, [f32; EMBEDDING_DIM]>,
    /// Embedding統計情報（Z-score正規化用）
    embedding_stats: EmbeddingStats,
}

impl FeatureExtractor {
    #[must_use]
    pub fn from_metadata(
        vocab: &[String],
        idf: &[f32],
        bm25_k1: f32,
        bm25_b: f32,
        average_doc_len: f32,
    ) -> Self {
        let vocab_index = vocab
            .iter()
            .enumerate()
            .map(|(idx, term)| (term.clone(), idx))
            .collect();
        let embedding_index = EMBEDDING_LOOKUP
            .iter()
            .map(|(term, vec)| (*term, *vec))
            .collect();
        Self {
            vocab_index,
            idf: idf.to_vec(),
            bm25_k1,
            bm25_b,
            average_doc_len,
            embedding_index,
            embedding_stats: EmbeddingStats::empty(EMBEDDING_DIM),
        }
    }

    /// コーパスから動的に語彙を構築してFeatureExtractorを作成する。
    ///
    /// # Arguments
    /// * `tokenized_corpus` - トークン化された文書のリスト（各文書はトークンのベクトル）
    /// * `vocab_size` - 構築する語彙のサイズ（上位N個のトークンを選択）
    ///
    /// # Returns
    /// 動的に構築されたFeatureExtractor
    pub fn build_from_corpus(tokenized_corpus: &[Vec<String>], vocab_size: usize) -> Self {
        use std::collections::HashMap;

        // 1. 各トークンの文書頻度（DF）を計算
        let mut doc_freq: HashMap<String, usize> = HashMap::new();
        let total_docs = tokenized_corpus.len();

        for doc_tokens in tokenized_corpus {
            let unique_tokens: std::collections::HashSet<String> =
                doc_tokens.iter().map(|t| t.to_lowercase()).collect();
            for token in unique_tokens {
                *doc_freq.entry(token).or_insert(0) += 1;
            }
        }

        // 選択前のユニークトークン数を保持
        let unique_tokens_before_selection = doc_freq.len();

        // 2. 上位vocab_size個のトークンを選択（DF降順）
        let mut token_df_pairs: Vec<(String, usize)> = doc_freq.into_iter().collect();
        token_df_pairs.sort_by(|a, b| b.1.cmp(&a.1)); // DF降順でソート
        token_df_pairs.truncate(vocab_size);

        // 3. 語彙とIDFを構築
        let vocab: Vec<String> = token_df_pairs
            .iter()
            .map(|(token, _)| token.clone())
            .collect();
        let idf: Vec<f32> = token_df_pairs
            .iter()
            .map(|(_, df)| {
                // IDF(t) = log((N + 1) / (DF(t) + 1)) + 1
                let n = total_docs as f32;
                let df_val = *df as f32;
                ((n + 1.0) / (df_val + 1.0)).ln() + 1.0
            })
            .collect();

        // 4. 平均文書長を計算
        let total_tokens: usize = tokenized_corpus.iter().map(Vec::len).sum();
        let average_doc_len = if total_docs > 0 {
            total_tokens as f32 / total_docs as f32
        } else {
            FALLBACK_AVG_DOC_LEN
        };

        // ログ出力: FeatureExtractor構築時の統計情報
        tracing::info!(
            "FeatureExtractor corpus analysis: total_docs={}, unique_tokens={}, selected_vocab_size={}, avg_doc_len={:.2}",
            total_docs,
            unique_tokens_before_selection,
            vocab.len(),
            average_doc_len
        );

        // ログ出力: FeatureExtractor構築時の統計情報
        tracing::info!(
            "FeatureExtractor corpus analysis: total_docs={}, unique_tokens={}, selected_vocab_size={}, avg_doc_len={:.2}",
            total_docs,
            unique_tokens_before_selection,
            vocab.len(),
            average_doc_len
        );

        // 5. FeatureExtractorを初期化
        // 6. Embedding統計は空の統計を設定（後でtrain_embedding_statsで更新可能）
        Self::from_metadata(
            &vocab,
            &idf,
            FALLBACK_BM25_K1,
            FALLBACK_BM25_B,
            average_doc_len,
        )
    }

    #[must_use]
    pub fn fallback() -> Self {
        let vocab: Vec<String> = FALLBACK_VOCAB.iter().map(ToString::to_string).collect();
        let idf: Vec<f32> = FALLBACK_IDF.to_vec();
        Self::from_metadata(
            &vocab,
            &idf,
            FALLBACK_BM25_K1,
            FALLBACK_BM25_B,
            FALLBACK_AVG_DOC_LEN,
        )
    }

    /// Embedding統計を設定する（訓練時に呼び出す）
    ///
    /// # Arguments
    /// * `stats` - 計算済みのEmbedding統計情報
    pub fn set_embedding_stats(&mut self, stats: EmbeddingStats) {
        self.embedding_stats = stats;
    }

    /// Embedding統計を取得する
    #[must_use]
    pub fn embedding_stats(&self) -> &EmbeddingStats {
        &self.embedding_stats
    }

    /// 語彙サイズを取得する。
    #[must_use]
    pub fn vocab_len(&self) -> usize {
        self.idf.len()
    }

    #[must_use]
    pub fn extract(&self, tokens: &[String]) -> FeatureVector {
        let vocab_len = self.idf.len();
        let mut raw_counts = vec![0.0f32; vocab_len];
        let mut total_hits = 0.0f32;
        let mut embedding = vec![0.0f32; EMBEDDING_DIM];
        let mut embedding_hits = 0.0f32;

        for token in tokens {
            let lowered = token.to_lowercase();
            if let Some(&index) = self.vocab_index.get(&lowered) {
                raw_counts[index] += 1.0;
                total_hits += 1.0;
            }
            if let Some(vector) = self.embedding_index.get(lowered.as_str()) {
                for (slot, value) in embedding.iter_mut().zip(vector.iter()) {
                    *slot += value;
                }
                embedding_hits += 1.0;
            } else {
                // Fallback: use hashing to generate a deterministic embedding
                // This ensures that even unknown words contribute to the embedding vector
                // and prevents zero-norm vectors which cause classification failures.
                let hash = xxh3_64(lowered.as_bytes());
                for (i, slot) in embedding.iter_mut().enumerate() {
                    // Generate a pseudo-random float in [0.0, 1.0] from the hash
                    // Use different bit shifts for each dimension to decorrelate
                    let shift = i * 8;
                    let val = ((hash >> shift) & 0xFF) as f32 / 255.0;
                    *slot += val;
                }
                embedding_hits += 1.0;
            }
        }

        let mut tfidf = vec![0.0f32; vocab_len];
        let mut bm25 = vec![0.0f32; vocab_len];

        #[allow(clippy::cast_precision_loss)]
        let doc_len = tokens.len() as f32;
        let length_norm = if doc_len > 0.0 {
            1.0 - self.bm25_b + self.bm25_b * (doc_len / self.average_doc_len)
        } else {
            1.0
        };

        if total_hits > 0.0 {
            for (idx, raw) in raw_counts.iter().enumerate() {
                if *raw == 0.0 {
                    continue;
                }
                let tf = *raw / total_hits;
                tfidf[idx] = tf * self.idf[idx];

                let numerator = (*raw) * (self.bm25_k1 + 1.0);
                let denominator = *raw + self.bm25_k1 * length_norm;
                bm25[idx] = self.idf[idx] * (numerator / denominator);
            }
        }

        if embedding_hits > 0.0 {
            for value in &mut embedding {
                *value /= embedding_hits;
            }
        }

        // EmbeddingにZ-score正規化を適用
        self.embedding_stats.normalize(&mut embedding);

        FeatureVector {
            tfidf,
            bm25,
            embedding,
        }
    }
}

pub(crate) const FALLBACK_VOCAB: [&str; 19] = [
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

pub(crate) const FALLBACK_IDF: [f32; FALLBACK_VOCAB.len()] = [
    1.6, 1.5, 1.4, 1.3, 1.2, 1.3, 1.2, 1.5, 1.4, 1.5, 1.3, 1.3, 1.2, 1.2, 1.5, 1.4, 1.4, 1.2, 1.2,
];

impl FeatureVector {
    #[must_use]
    pub fn max_bm25(&self) -> Option<f32> {
        self.bm25
            .iter()
            .copied()
            .fold(None, |acc, value| match acc {
                Some(existing) if existing >= value => Some(existing),
                _ => Some(value),
            })
    }
}
