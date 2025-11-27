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
    /// Temperature Scalingパラメータ（信頼度較正用）
    /// デフォルトは1.0（較正なし）。小さいほど分布がシャープ、大きいほどソフト
    temperature: f32,
}

impl CentroidClassifier {
    /// 新しいCentroidClassifierを作成する。
    #[must_use]
    pub fn new(feature_dim: usize) -> Self {
        Self {
            centroids: HashMap::new(),
            thresholds: HashMap::new(),
            feature_dim,
            temperature: 1.0, // デフォルトは較正なし
        }
    }

    /// Temperature Scalingパラメータを設定する。
    ///
    /// # Arguments
    /// * `temperature` - 温度パラメータ（0.05以上、2.0以下を推奨）
    pub fn set_temperature(&mut self, temperature: f32) {
        self.temperature = temperature.max(0.01); // ゼロ除算を防ぐ
    }

    /// Temperature Scalingパラメータを取得する。
    #[must_use]
    pub fn temperature(&self) -> f32 {
        self.temperature
    }

    /// Validation Setを用いて最適な温度パラメータを探索する。
    ///
    /// # Arguments
    /// * `validation_articles` - 検証用記事のリスト（ラベル付き）
    /// * `temperature_range` - 探索する温度の範囲（開始、終了、刻み）
    ///
    /// # Returns
    /// 最適な温度パラメータ
    ///
    /// # Note
    /// Negative Log Likelihood (NLL) を最小化する温度を探索する。
    /// グリッドサーチで実装（0.05刻みで0.05から2.0まで探索）
    pub fn find_optimal_temperature(
        &self,
        validation_articles: &[Article],
        temperature_range: Option<(f32, f32, f32)>,
    ) -> f32 {
        let (min_temp, max_temp, step) = temperature_range.unwrap_or((0.05, 2.0, 0.05));

        let mut best_temp = 1.0;
        let mut best_nll = f32::INFINITY;

        let mut temp = min_temp;
        while temp <= max_temp {
            let mut nll_sum = 0.0;
            let mut valid_count = 0;

            for article in validation_articles {
                if let Some(ref fv) = article.feature_vector {
                    if let Some((predicted_genre, calibrated_score)) =
                        self.predict_with_temp(fv, temp)
                    {
                        // 正解ラベルがある場合のみNLLを計算
                        if !article.genres.is_empty() {
                            let is_correct = article.genres.contains(&predicted_genre);
                            // 正解の場合は -log(p)、不正解の場合は -log(1-p)
                            let nll = if is_correct {
                                -calibrated_score.ln()
                            } else {
                                -(1.0 - calibrated_score).ln()
                            };
                            nll_sum += nll;
                            valid_count += 1;
                        }
                    }
                }
            }

            if valid_count > 0 {
                let avg_nll = nll_sum / (valid_count as f32);
                if avg_nll < best_nll {
                    best_nll = avg_nll;
                    best_temp = temp;
                }
            }

            temp += step;
        }

        best_temp
    }

    /// 指定された温度で予測を行う（内部メソッド）
    fn predict_with_temp(&self, target_vector: &FeatureVector, temp: f32) -> Option<(String, f32)> {
        let combined = Self::combine_feature_vector(target_vector);
        if combined.len() != self.feature_dim {
            return None;
        }

        let norm = combined.dot(&combined).sqrt();
        if norm <= 0.0 {
            return None;
        }
        let normalized = &combined / norm;

        let mut best_genre: Option<(String, f32)> = None;

        for (genre, centroids) in &self.centroids {
            let mut max_similarity = -1.0;
            for centroid in centroids {
                let sim = normalized.dot(centroid);
                if sim > max_similarity {
                    max_similarity = sim;
                }
            }

            let calibrated_score = Self::calibrate_score(max_similarity, temp);
            let threshold = self.thresholds.get(genre).copied().unwrap_or(0.6);

            if max_similarity >= threshold {
                if let Some((_, best_score)) = best_genre {
                    if calibrated_score > best_score {
                        best_genre = Some((genre.clone(), calibrated_score));
                    }
                } else {
                    best_genre = Some((genre.clone(), calibrated_score));
                }
            }
        }

        best_genre
    }

    /// Golden Datasetから重心を計算して学習する。
    ///
    /// # Errors
    /// ベクトルの次元が一致しない場合にエラーを返す。
    pub fn train(&mut self, labeled_articles: &[Article]) -> Result<()> {
        self.train_with_robust(labeled_articles, 0.2)
    }

    /// Golden Datasetから重心を計算して学習する（ロバスト版）。
    /// 外れ値を除外したロバストな重心計算を行う。
    ///
    /// # Arguments
    /// * `labeled_articles` - ラベル付き記事のリスト
    /// * `trim_ratio` - 除外する外れ値の割合（デフォルト: 0.2 = 20%）
    ///
    /// # Errors
    /// ベクトルの次元が一致しない場合にエラーを返す。
    pub fn train_with_robust(
        &mut self,
        labeled_articles: &[Article],
        trim_ratio: f32,
    ) -> Result<()> {
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
                // ロバストな単一重心計算（外れ値除外）
                Self::train_robust_centroid(articles, self.feature_dim, trim_ratio)
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

    /// ロバストな重心計算（外れ値除外版）
    /// Geometric Trimmed Mean: クラスタ内平均類似度が低い下位k%を除外してから平均を計算
    ///
    /// # Arguments
    /// * `articles` - 記事のリスト
    /// * `feature_dim` - 特徴ベクトルの次元数
    /// * `trim_ratio` - 除外する外れ値の割合（例: 0.2 = 20%）
    ///
    /// # Returns
    /// ロバストな重心ベクトルのリスト（通常は1つ）
    fn train_robust_centroid(
        articles: &[&Article],
        feature_dim: usize,
        trim_ratio: f32,
    ) -> Vec<Array1<f32>> {
        // 1. 有効なベクトルを収集
        let mut valid_vectors = Vec::new();
        for article in articles {
            if let Some(ref fv) = article.feature_vector {
                let combined = Self::combine_feature_vector(fv);
                if combined.len() == feature_dim {
                    let norm = combined.dot(&combined).sqrt();
                    if norm > 0.0 {
                        valid_vectors.push((&combined / norm).to_owned());
                    }
                }
            }
        }

        if valid_vectors.is_empty() {
            return Vec::new();
        }

        // 記事数が少ない場合は、外れ値除外を行わず通常の平均を計算
        if valid_vectors.len() <= 3 {
            let mut centroid_sum = Array1::<f32>::zeros(feature_dim);
            for v in &valid_vectors {
                centroid_sum = &centroid_sum + v;
            }
            let centroid = &centroid_sum / (valid_vectors.len() as f32);
            let norm = centroid.dot(&centroid).sqrt();
            if norm > 0.0 {
                return vec![&centroid / norm];
            }
            return vec![centroid];
        }

        // 2. 各ベクトルの「クラスタ内平均類似度」を計算
        // 計算量削減のため、全点対全点ではなく、サンプリングまたはバッチ処理を検討可
        let mut scores: Vec<(usize, f32)> = valid_vectors
            .iter()
            .enumerate()
            .map(|(i, v)| {
                let v_view = v.view();
                // クラスタ内の他の全ベクトルとの類似度の合計を計算
                let sim_sum: f32 = valid_vectors
                    .iter()
                    .map(|other| {
                        // コサイン類似度（既にL2正規化されているので、内積がそのまま類似度）
                        v_view.dot(&other.view())
                    })
                    .sum();
                (i, sim_sum) // 平均をとる必要はなく、合計で比較可能
            })
            .collect();

        // 3. スコア順にソート (降順: 類似度が高い順)
        scores.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

        // 4. 上位 (1.0 - trim_ratio) の要素のみを残す
        #[allow(clippy::cast_sign_loss)]
        let keep_count = (valid_vectors.len() as f32 * (1.0 - trim_ratio)).ceil() as usize;
        let keep_count = keep_count.max(1); // 少なくとも1つは残す
        let keep_count = keep_count.min(valid_vectors.len()); // 上限チェック

        let valid_indices: Vec<usize> = scores.iter().take(keep_count).map(|x| x.0).collect();

        // 5. 平均を計算 (Trimmed Mean)
        // valid_indicesに対応するベクトルを足し合わせて平均をとる
        let mut sum_vec = Array1::<f32>::zeros(feature_dim);
        for &idx in &valid_indices {
            sum_vec = &sum_vec + &valid_vectors[idx];
        }

        let centroid = &sum_vec / (valid_indices.len() as f32);
        let norm = centroid.dot(&centroid).sqrt();
        if norm > 0.0 {
            vec![&centroid / norm]
        } else {
            vec![centroid.clone()]
        }
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
        self.predict_top_k(target_vector, 1)
            .first()
            .map(|(g, s)| (g.clone(), *s))
    }

    /// ターゲットベクトルを分類し、上位k個の候補を返す。
    ///
    /// # Arguments
    /// * `target_vector` - 分類対象の特徴ベクトル
    /// * `k` - 取得する上位候補の数
    ///
    /// # Returns
    /// (ジャンル, スコア) のリスト。スコア降順。
    pub fn predict_top_k(&self, target_vector: &FeatureVector, k: usize) -> Vec<(String, f32)> {
        let combined = Self::combine_feature_vector(target_vector);
        if combined.len() != self.feature_dim {
            return Vec::new();
        }

        // L2正規化
        let norm = combined.dot(&combined).sqrt();
        if norm <= 0.0 {
            return Vec::new();
        }
        let normalized = &combined / norm;

        let mut candidates: Vec<(String, f32)> = Vec::new();

        for (genre, centroids) in &self.centroids {
            // 各サブ重心との最大類似度を計算
            let mut max_similarity = -1.0;
            for centroid in centroids {
                let sim = normalized.dot(centroid);
                if sim > max_similarity {
                    max_similarity = sim;
                }
            }

            // Temperature Scalingによる較正
            let calibrated_score = Self::calibrate_score(max_similarity, self.temperature);

            // 閾値を取得
            let threshold = self.thresholds.get(genre).copied().unwrap_or(0.6);

            // 閾値を超えている場合のみ候補に追加（較正前の類似度で判定）
            if max_similarity >= threshold {
                candidates.push((genre.clone(), calibrated_score));
            }
        }

        // スコア降順にソート
        candidates.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

        // 上位k個を返す
        candidates.into_iter().take(k).collect()
    }

    /// Temperature Scalingによる信頼度較正
    ///
    /// # Arguments
    /// * `similarity` - コサイン類似度（-1.0 ～ 1.0）
    /// * `temperature` - 温度パラメータ（T > 0）
    ///
    /// # Returns
    /// 較正後の確率スコア（0.0 ～ 1.0）
    ///
    /// # Note
    /// コサイン類似度をシグモイド関数で確率に変換する。
    /// 数式: P = 1 / (1 + exp(-s / T))
    /// ここで s は類似度、T は温度パラメータ
    fn calibrate_score(similarity: f32, temperature: f32) -> f32 {
        // 類似度を[-1, 1]から[0, 1]にシフト（オプション）
        // または、そのままシグモイドを適用
        // ここでは、類似度をそのまま使用し、シグモイドで確率に変換
        let scaled = similarity / temperature;
        1.0 / (1.0 + (-scaled).exp())
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
