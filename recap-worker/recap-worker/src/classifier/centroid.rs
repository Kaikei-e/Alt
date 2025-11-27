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
    // Multi-Centroid: 1ジャンルにつき複数の重心を持つ
    centroids: HashMap<String, Vec<Array1<f32>>>,
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
        let mut articles_with_features = 0;

        for article in labeled_articles {
            if article.feature_vector.is_some() {
                articles_with_features += 1;
            }
            if article.feature_vector.is_some() {
                for genre in &article.genres {
                    genre_articles
                        .entry(genre.clone())
                        .or_default()
                        .push(article);
                }
            }
        }

        tracing::info!(
            "CentroidClassifier training started: total_articles={}, articles_with_features={}, unique_genres={}",
            labeled_articles.len(),
            articles_with_features,
            genre_articles.len()
        );

        for (genre, articles) in genre_articles.iter() {
            if articles.is_empty() {
                continue;
            }

            // k-Means クラスタリング (k=3) または単一重心
            // 記事数が少ない場合は単一重心、多い場合はk-Means
            let k = if articles.len() >= 10 { 3 } else { 1 };
            let centroids = if k > 1 {
                Self::perform_kmeans(articles, k, self.feature_dim)
            } else {
                // 従来の単一重心計算
                let mut centroid_sum = Array1::<f32>::zeros(self.feature_dim);
                let mut valid_count = 0;
                for article in articles {
                    if let Some(ref fv) = article.feature_vector {
                        let combined = Self::combine_feature_vector(fv);
                        anyhow::ensure!(
                            combined.len() == self.feature_dim,
                            "feature dimension mismatch: expected {}, got {}",
                            self.feature_dim,
                            combined.len()
                        );
                        let norm = combined.dot(&combined).sqrt();
                        if norm > 0.0 {
                            let normalized = &combined / norm;
                            centroid_sum = &centroid_sum + &normalized;
                            valid_count += 1;
                        }
                    }
                }
                if valid_count > 0 {
                    let centroid = &centroid_sum / (valid_count as f32);
                    let norm = centroid.dot(&centroid).sqrt();
                    if norm > 0.0 {
                        vec![&centroid / norm]
                    } else {
                        vec![centroid]
                    }
                } else {
                    vec![]
                }
            };

            if centroids.is_empty() {
                tracing::warn!(
                    "CentroidClassifier: genre '{}' has no valid feature vectors ({} articles)",
                    genre,
                    articles.len()
                );
            } else {
                // 重心を保存
                self.centroids.insert(genre.clone(), centroids.clone());

                // 適応型閾値の計算 (最大の重心を使用)
                // 簡易的に、最も多くの記事が属するクラスタ（または全体の重心）を使うべきだが、
                // ここでは全記事と全サブ重心の最大類似度の平均を使う
                let (threshold, mean, std_dev) =
                    Self::calculate_adaptive_threshold_multi(articles, &centroids);

                self.thresholds.insert(genre.clone(), threshold);

                tracing::info!(
                    "CentroidClassifier: trained genre '{}' with {} articles, {} centroids, threshold={:.4} (mean={:.4}, std_dev={:.4})",
                    genre,
                    articles.len(), // Use articles.len() for total articles in genre
                    centroids.len(),
                    threshold,
                    mean,
                    std_dev
                );
            }
        }

        Ok(())
    }

    /// k-Means クラスタリングを実行する
    fn perform_kmeans(articles: &[&Article], k: usize, feature_dim: usize) -> Vec<Array1<f32>> {
        use rand::seq::SliceRandom;
        use rand::thread_rng;

        let mut valid_vectors = Vec::new();
        for article in articles {
            if let Some(ref fv) = article.feature_vector {
                let combined = Self::combine_feature_vector(fv);
                let norm = combined.dot(&combined).sqrt();
                if norm > 0.0 {
                    valid_vectors.push(&combined / norm);
                }
            }
        }

        if valid_vectors.is_empty() {
            return Vec::new();
        }
        if valid_vectors.len() <= k {
            return valid_vectors;
        }

        // 初期化: ランダムにk個選択
        let mut rng = thread_rng();
        let mut centroids: Vec<Array1<f32>> = valid_vectors
            .choose_multiple(&mut rng, k)
            .cloned()
            .collect();

        // k-Means ループ (最大10回)
        for _ in 0..10 {
            let mut clusters: Vec<Vec<&Array1<f32>>> = vec![Vec::new(); k];

            // E-step: 各点を最も近い重心に割り当て
            for v in &valid_vectors {
                let mut best_idx = 0;
                let mut max_sim = -1.0;
                for (i, c) in centroids.iter().enumerate() {
                    let sim = v.dot(c);
                    if sim > max_sim {
                        max_sim = sim;
                        best_idx = i;
                    }
                }
                clusters[best_idx].push(v);
            }

            // M-step: 重心を更新
            let mut new_centroids = Vec::new();
            let mut changed = false;
            for (i, cluster) in clusters.iter().enumerate() {
                if cluster.is_empty() {
                    new_centroids.push(centroids[i].clone()); // 変更なし
                    continue;
                }

                let mut sum = Array1::<f32>::zeros(feature_dim);
                for v in cluster {
                    sum = &sum + *v;
                }
                let mean = &sum / (cluster.len() as f32);
                let norm = mean.dot(&mean).sqrt();
                let new_c = if norm > 0.0 { &mean / norm } else { mean };

                // 収束判定用の差分チェック（簡易）
                if (new_c.clone() - &centroids[i]).mapv(f32::abs).sum() > 1e-4 {
                    changed = true;
                }
                new_centroids.push(new_c);
            }
            centroids = new_centroids;

            if !changed {
                break;
            }
        }

        centroids
    }

    /// 適応型閾値を計算するヘルパーメソッド（Multi-Centroid対応）。
    fn calculate_adaptive_threshold_multi(
        articles: &[&Article],
        centroids: &[Array1<f32>],
    ) -> (f32, f32, f32) {
        let mut max_similarities = Vec::new();
        for article in articles {
            if let Some(ref fv) = article.feature_vector {
                let combined = Self::combine_feature_vector(fv);
                let norm = combined.dot(&combined).sqrt();
                if norm > 0.0 {
                    let normalized_article = &combined / norm;
                    // 各重心との最大類似度を取る
                    let mut max_sim = -1.0;
                    for c in centroids {
                        let sim = normalized_article.dot(c);
                        if sim > max_sim {
                            max_sim = sim;
                        }
                    }
                    if max_sim > -1.0 {
                        max_similarities.push(max_sim);
                    }
                }
            }
        }

        if max_similarities.is_empty() {
            return (0.6, 0.0, 0.0);
        }

        let mean: f32 = max_similarities.iter().sum::<f32>() / max_similarities.len() as f32;
        let variance: f32 = max_similarities
            .iter()
            .map(|s| (s - mean).powi(2))
            .sum::<f32>()
            / max_similarities.len() as f32;
        // 平均 - 1.5 * 標準偏差 を閾値とする（ただし下限は0.3）
        // 分散が極端に小さい場合の対策として、標準偏差に小さな値を加えるか、閾値の上限を設定する
        let std_dev = variance.sqrt().max(0.05); // 最低でも0.05の標準偏差を仮定
        let adaptive = mean - 1.5 * std_dev;

        // 閾値の上限を0.95、下限を0.3とする
        (adaptive.clamp(0.3, 0.95), mean, std_dev)
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

        for (genre, centroids) in &self.centroids {
            // 各サブ重心との最大類似度を計算
            let mut max_similarity = -1.0;
            for centroid in centroids {
                let sim = normalized.dot(centroid);
                if sim > max_similarity {
                    max_similarity = sim;
                }
            }

            // 閾値を取得
            let threshold = self.thresholds.get(genre).copied().unwrap_or(0.6);

            // 閾値を超えている場合のみ考慮
            if max_similarity >= threshold {
                if let Some((_, best_score)) = best_genre {
                    if max_similarity > best_score {
                        best_genre = Some((genre.clone(), max_similarity));
                    }
                } else {
                    best_genre = Some((genre.clone(), max_similarity));
                }
            }
        }

        best_genre
    }

    /// 全ジャンル中の最大類似度とそのジャンル、閾値を取得する（デバッグ用）。
    ///
    /// # Returns
    /// (ジャンル, 類似度, 閾値) のタプル。該当するジャンルがない場合は `None` を返す。
    pub fn get_top_similarity(&self, target_vector: &FeatureVector) -> Option<(String, f32, f32)> {
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

        let mut best_genre: Option<(String, f32, f32)> = None;

        for (genre, centroids) in &self.centroids {
            // 各サブ重心との最大類似度を計算
            let mut max_similarity = -1.0;
            for centroid in centroids {
                let sim = normalized.dot(centroid);
                if sim > max_similarity {
                    max_similarity = sim;
                }
            }

            // 閾値を取得
            let threshold = self.thresholds.get(genre).copied().unwrap_or(0.6);

            // 最大類似度を更新
            if let Some((_, best_score, _)) = best_genre {
                if max_similarity > best_score {
                    best_genre = Some((genre.clone(), max_similarity, threshold));
                }
            } else {
                best_genre = Some((genre.clone(), max_similarity, threshold));
            }
        }

        best_genre
    }

    /// FeatureVectorを結合して1つのベクトルに変換する。
    /// tfidf, bm25, embeddingを結合する。
    pub(crate) fn combine_feature_vector(feature_vector: &FeatureVector) -> Array1<f32> {
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
    pub fn get_centroid(&self, genre: &str) -> Option<&Vec<Array1<f32>>> {
        self.centroids.get(genre)
    }

    /// 学習された閾値のマップを取得する。
    pub fn get_thresholds(&self) -> &HashMap<String, f32> {
        &self.thresholds
    }

    /// 学習済みのジャンル一覧を取得する。
    #[must_use]
    pub fn trained_genres(&self) -> Vec<String> {
        self.centroids.keys().cloned().collect()
    }
}
