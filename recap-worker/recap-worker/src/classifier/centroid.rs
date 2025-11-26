//! Centroid-based Classification (Rocchio) の実装。
//! Golden Datasetから各ジャンルの重心ベクトルを計算し、コサイン類似度で分類を行う。

use std::collections::HashMap;

use anyhow::Result;
use ndarray::Array1;

use crate::classification::FeatureVector;

/// 記事データ（Golden Dataset用）
#[derive(Debug, Clone)]
pub struct Article {
    pub id: String,
    pub content: String,
    pub genres: Vec<String>,
    pub feature_vector: Option<FeatureVector>,
}

/// Centroid Classifier
/// 各ジャンルの重心ベクトルを保持し、コサイン類似度で分類を行う。
#[derive(Debug, Clone)]
pub struct CentroidClassifier {
    /// 各ジャンルの重心ベクトル（結合された特徴ベクトル）
    centroids: HashMap<String, Array1<f32>>,
    /// 各ジャンルの適応型閾値
    thresholds: HashMap<String, f32>,
    /// 特徴ベクトルの次元数
    feature_dim: usize,
}

impl CentroidClassifier {
    /// 新しいCentroidClassifierを作成する。
    #[must_use]
    pub fn new(feature_dim: usize) -> Self {
        Self {
            centroids: HashMap::new(),
            thresholds: HashMap::new(),
            feature_dim,
        }
    }

    /// Golden Datasetから重心を計算して学習する。
    ///
    /// # Errors
    /// ベクトルの次元が一致しない場合にエラーを返す。
    pub fn train(&mut self, labeled_articles: &[Article]) -> Result<()> {
        // ジャンルごとに記事をグループ化
        let mut genre_articles: HashMap<String, Vec<&Article>> = HashMap::new();
        for article in labeled_articles {
            if article.feature_vector.is_some() {
                for genre in &article.genres {
                    genre_articles
                        .entry(genre.clone())
                        .or_default()
                        .push(article);
                }
            }
        }

        // 各ジャンルの重心を計算
        for (genre, articles) in genre_articles.iter() {
            if articles.is_empty() {
                continue;
            }

            // ベクトルを結合して正規化
            let mut centroid_sum = Array1::<f32>::zeros(self.feature_dim);
            let mut count = 0;

            for article in articles {
                if let Some(ref feature_vector) = article.feature_vector {
                    let combined = Self::combine_feature_vector(feature_vector);
                    anyhow::ensure!(
                        combined.len() == self.feature_dim,
                        "feature dimension mismatch: expected {}, got {}",
                        self.feature_dim,
                        combined.len()
                    );

                    // L2正規化
                    let norm = combined.dot(&combined).sqrt();
                    if norm > 0.0 {
                        let normalized = &combined / norm;
                        centroid_sum = &centroid_sum + &normalized;
                        count += 1;
                    }
                }
            }

            if count > 0 {
                // 平均を計算（既に正規化されているので、平均も正規化する）
                let centroid = &centroid_sum / (count as f32);
                let norm = centroid.dot(&centroid).sqrt();
                let normalized_centroid = if norm > 0.0 {
                    &centroid / norm
                } else {
                    centroid
                };

                self.centroids
                    .insert(genre.clone(), normalized_centroid.to_owned());

                // デフォルト閾値を設定（society_justiceは0.75、その他は0.6）
                let default_threshold = if genre == "society_justice" {
                    0.75
                } else {
                    0.6
                };
                self.thresholds.insert(genre.clone(), default_threshold);
            }
        }

        Ok(())
    }

    /// ターゲットベクトルを分類する。
    ///
    /// # Returns
    /// 閾値を超える中で最大のコサイン類似度を持つジャンルとスコアを返す。
    /// 該当するジャンルがない場合は `None` を返す。
    pub fn predict(&self, target_vector: &FeatureVector) -> Option<(String, f32)> {
        let combined = Self::combine_feature_vector(target_vector);
        if combined.len() != self.feature_dim {
            return None;
        }

        // L2正規化
        let norm = combined.dot(&combined).sqrt();
        if norm <= 0.0 {
            return None;
        }
        let normalized = &combined / norm;

        let mut best_genre: Option<(String, f32)> = None;

        for (genre, centroid) in &self.centroids {
            // コサイン類似度を計算
            let similarity = normalized.dot(centroid);

            // 閾値を取得
            let threshold = self.thresholds.get(genre).copied().unwrap_or(0.6);

            // 閾値を超えている場合のみ考慮
            if similarity >= threshold {
                if let Some((_, best_score)) = best_genre {
                    if similarity > best_score {
                        best_genre = Some((genre.clone(), similarity));
                    }
                } else {
                    best_genre = Some((genre.clone(), similarity));
                }
            }
        }

        best_genre
    }

    /// FeatureVectorを結合して1つのベクトルに変換する。
    /// tfidf, bm25, embeddingを結合する。
    fn combine_feature_vector(feature_vector: &FeatureVector) -> Array1<f32> {
        let total_dim =
            feature_vector.tfidf.len() + feature_vector.bm25.len() + feature_vector.embedding.len();
        let mut combined = Vec::with_capacity(total_dim);

        combined.extend_from_slice(&feature_vector.tfidf);
        combined.extend_from_slice(&feature_vector.bm25);
        combined.extend_from_slice(&feature_vector.embedding);

        Array1::from_vec(combined)
    }

    /// 閾値を設定する。
    pub fn set_threshold(&mut self, genre: &str, threshold: f32) {
        self.thresholds.insert(genre.to_string(), threshold);
    }

    /// 重心を取得する（デバッグ用）。
    #[allow(dead_code)]
    pub fn get_centroid(&self, genre: &str) -> Option<&Array1<f32>> {
        self.centroids.get(genre)
    }

    /// 学習済みのジャンル一覧を取得する。
    #[must_use]
    pub fn trained_genres(&self) -> Vec<String> {
        self.centroids.keys().cloned().collect()
    }
}
