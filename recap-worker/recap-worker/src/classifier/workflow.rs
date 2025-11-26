//! 分類パイプラインのワークフロー統合
//! Centroid Classifier + Graph Label Propagation の統合処理

use std::collections::HashSet;
use std::fs;
use std::path::Path;

use anyhow::{Context, Result};
use serde::Deserialize;

use std::collections::HashMap;

use crate::classification::{
    ClassificationLanguage, ClassificationResult, FeatureExtractor, GenreClassifier, TokenPipeline,
};
use crate::classifier::{CentroidClassifier, GraphPropagator, centroid::Article};

/// Golden Datasetのアイテム（JSON形式）
#[derive(Debug, Deserialize, Clone)]
pub struct GoldenItem {
    pub id: String,
    pub content: String,
    #[serde(rename = "expected_genres")]
    pub genres: Vec<String>,
}

/// 特徴ベクトル抽出の統計情報
struct FeatureExtractionStats {
    zero_vector_count: usize,
    avg_tfidf_norm: f32,
    avg_bm25_norm: f32,
    avg_embedding_norm: f32,
}

/// 分類パイプライン
/// Centroid ClassifierとGraph Label Propagationを統合した分類器
/// Golden Datasetが読み込めない場合は、既存のGenreClassifierにフォールバック
#[derive(Debug)]
pub struct ClassificationPipeline {
    centroid_classifier: Option<CentroidClassifier>,
    graph_propagator: Option<GraphPropagator>,
    fallback_classifier: Option<GenreClassifier>,
    feature_extractor: FeatureExtractor,
    token_pipeline: TokenPipeline,
}

impl Default for ClassificationPipeline {
    fn default() -> Self {
        Self::new()
    }
}

impl ClassificationPipeline {
    /// デフォルトのGolden Datasetパスからパイプラインを初期化する。
    /// Golden Datasetが見つからない場合は、既存のGenreClassifierにフォールバックする。
    ///
    /// # Note
    /// Dockerfileと同じように、`/app/data/golden_classification.json` に配置されていることを前提とします。
    /// 開発環境では `tests/data/golden_classification.json` も試行します。
    ///
    /// # Returns
    /// 常に成功する（フォールバック処理により）
    #[must_use]
    pub fn new() -> Self {
        // Dockerfileと同じパスを優先（本番環境）
        let default_path = Path::new("/app/data/golden_classification.json");
        if default_path.exists() {
            match Self::from_golden_dataset(default_path) {
                Ok(pipeline) => {
                    tracing::info!(
                        "ClassificationPipeline initialized with CentroidClassifier from {}",
                        default_path.display()
                    );
                    return pipeline;
                }
                Err(e) => {
                    tracing::warn!(
                        "Failed to load golden dataset from {}: {}. Falling back to GenreClassifier.",
                        default_path.display(),
                        e
                    );
                }
            }
        }

        // 開発環境用: tests/data/golden_classification.json を試行
        let dev_candidates = vec![
            // CARGO_MANIFEST_DIRからの相対パス（ビルド時）
            std::env::var("CARGO_MANIFEST_DIR")
                .ok()
                .map(|d| format!("{}/../tests/data/golden_classification.json", d)),
            // 現在の作業ディレクトリからの相対パス
            Some("tests/data/golden_classification.json".to_string()),
            // 絶対パス（recap-workerディレクトリからの相対）
            Some("../tests/data/golden_classification.json".to_string()),
        ];

        for candidate in dev_candidates.into_iter().flatten() {
            let path = Path::new(&candidate);
            if path.exists() {
                match Self::from_golden_dataset(path) {
                    Ok(pipeline) => {
                        tracing::info!(
                            "ClassificationPipeline initialized with CentroidClassifier from {}",
                            path.display()
                        );
                        return pipeline;
                    }
                    Err(e) => {
                        tracing::warn!(
                            "Failed to load golden dataset from {}: {}",
                            path.display(),
                            e
                        );
                    }
                }
            }
        }

        // Golden Datasetが見つからない場合は、既存のGenreClassifierにフォールバック
        // ログは1回だけ出力（静的変数で制御）
        static WARNED: std::sync::Once = std::sync::Once::new();
        WARNED.call_once(|| {
            tracing::warn!(
                "Golden dataset not found at /app/data/golden_classification.json or tests/data/golden_classification.json. \
                 Falling back to GenreClassifier. \
                 See README.md for details on where to place the golden dataset file."
            );
        });

        let feature_extractor = FeatureExtractor::fallback();
        let token_pipeline = TokenPipeline::new();

        Self {
            centroid_classifier: None,
            graph_propagator: None,
            fallback_classifier: Some(GenreClassifier::new_default()),
            feature_extractor,
            token_pipeline,
        }
    }

    /// Golden Datasetからパイプラインを初期化する。
    ///
    /// # Arguments
    /// * `golden_dataset_path` - Golden Dataset JSONファイルのパス
    ///
    /// # Errors
    /// ファイルの読み込みやパースに失敗した場合にエラーを返す。
    pub fn from_golden_dataset(golden_dataset_path: &Path) -> Result<Self> {
        // Golden Datasetをロード
        let content = fs::read_to_string(golden_dataset_path).with_context(|| {
            format!(
                "failed to read golden dataset: {}",
                golden_dataset_path.display()
            )
        })?;
        let golden_items: Vec<GoldenItem> =
            serde_json::from_str(&content).context("failed to parse golden dataset JSON")?;

        let token_pipeline = TokenPipeline::new();

        // 1. すべての記事をトークン化してコーパスを構築
        let mut tokenized_corpus = Vec::new();
        for item in &golden_items {
            let language = ClassificationLanguage::Unknown;
            let normalized = token_pipeline.preprocess(&item.content, "", language);
            tokenized_corpus.push(normalized.tokens);
        }

        // 2. 動的にFeatureExtractorを構築（語彙サイズ1000）
        let vocab_size = 1000;
        let feature_extractor = FeatureExtractor::build_from_corpus(&tokenized_corpus, vocab_size);
        let actual_vocab_len = feature_extractor.vocab_len();
        tracing::info!(
            "FeatureExtractor built from corpus: vocab_size={} (requested={})",
            actual_vocab_len,
            vocab_size
        );

        // 3. 特徴ベクトルの次元数を動的に計算
        // tfidf + bm25 + embedding
        let embedding_dim = 6; // EMBEDDING_DIM
        let feature_dim = actual_vocab_len + actual_vocab_len + embedding_dim;
        tracing::info!(
            "Feature dimension: vocab_len={}, embedding_dim={}, total={}",
            actual_vocab_len,
            embedding_dim,
            feature_dim
        );

        // 4. Golden Datasetから特徴ベクトルを抽出
        let (labeled_articles, stats) =
            Self::extract_feature_vectors(&golden_items, &tokenized_corpus, &feature_extractor);

        // 統計ログを出力
        tracing::info!(
            "Golden Dataset feature extraction: total_articles={}, zero_norm_count={}, avg_tfidf_norm={:.4}, avg_bm25_norm={:.4}, avg_embedding_norm={:.4}",
            golden_items.len(),
            stats.zero_vector_count,
            stats.avg_tfidf_norm,
            stats.avg_bm25_norm,
            stats.avg_embedding_norm
        );

        if stats.zero_vector_count > 0 {
            tracing::warn!(
                "Golden Dataset: {} out of {} articles have zero-norm feature vectors",
                stats.zero_vector_count,
                golden_items.len()
            );
        } else {
            tracing::info!(
                "Golden Dataset: all {} articles have non-zero feature vectors",
                golden_items.len()
            );
        }

        // 5. CentroidClassifierを学習
        let mut centroid_classifier = CentroidClassifier::new(feature_dim);
        centroid_classifier.train(&labeled_articles)?;

        let trained_genres = centroid_classifier.trained_genres();
        tracing::info!(
            "CentroidClassifier trained successfully: {} genres, {} articles",
            trained_genres.len(),
            labeled_articles.len()
        );

        // GraphPropagatorの構築
        let mut graph_propagator = GraphPropagator::new(0.85); // エッジ構築用閾値
        if let Err(e) = graph_propagator.build_graph(&labeled_articles, &HashSet::new()) {
            tracing::warn!("Failed to build GraphPropagator: {}", e);
        } else {
            tracing::info!(
                "GraphPropagator built successfully: {} nodes, {} edges",
                graph_propagator.graph_stats().0,
                graph_propagator.graph_stats().1
            );
        }

        Ok(Self {
            centroid_classifier: Some(centroid_classifier),
            graph_propagator: Some(graph_propagator),
            fallback_classifier: None,
            feature_extractor,
            token_pipeline: TokenPipeline::new(),
        })
    }

    /// 新着記事を分類する。
    ///
    /// # Arguments
    /// * `article_id` - 記事ID
    /// * `content` - 記事の内容
    /// * `all_articles` - 全記事（Labeled + Unlabeled）のリスト（Graph Propagation用）
    ///
    /// # Returns
    /// 分類されたジャンル。分類できない場合は "other" を返す。
    pub fn classify(
        &self,
        article_id: &str,
        content: &str,
        all_articles: &[Article],
    ) -> Result<String> {
        // サンプリング用のカウンター（静的変数）
        static SAMPLE_COUNT: std::sync::atomic::AtomicUsize =
            std::sync::atomic::AtomicUsize::new(0);
        let sample_count = SAMPLE_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed);

        // 特徴ベクトルを抽出
        let language = ClassificationLanguage::Unknown;
        let normalized = self.token_pipeline.preprocess(content, "", language);
        let feature_vector = self.feature_extractor.extract(&normalized.tokens);

        // ゼロベクトルチェック
        Self::check_zero_vector(article_id, &normalized, &feature_vector);

        // Fast Pass: Centroid Classifierで分類
        if let Some(genre) =
            self.try_fast_pass(article_id, content, &feature_vector, language, sample_count)?
        {
            return Ok(genre);
        }

        // Rescue Pass: Graph Label Propagation
        if let Some(genre) = self.try_rescue_pass(
            article_id,
            content,
            all_articles,
            &feature_vector,
            sample_count,
        )? {
            return Ok(genre);
        }

        // Fallback: 分類できない場合は "other"
        Ok("other".to_string())
    }

    /// Golden Datasetから特徴ベクトルを抽出する。
    fn extract_feature_vectors(
        golden_items: &[GoldenItem],
        tokenized_corpus: &[Vec<String>],
        feature_extractor: &FeatureExtractor,
    ) -> (Vec<Article>, FeatureExtractionStats) {
        let mut labeled_articles = Vec::new();
        let mut zero_vector_count = 0;
        let mut tfidf_norms = Vec::new();
        let mut bm25_norms = Vec::new();
        let mut embedding_norms = Vec::new();

        for (item, tokens) in golden_items.iter().zip(tokenized_corpus.iter()) {
            let feature_vector = feature_extractor.extract(tokens);

            // 各特徴ベクトルのノルムを計算
            let tfidf_norm = {
                let tfidf_vec = ndarray::Array1::from_vec(feature_vector.tfidf.clone());
                tfidf_vec.dot(&tfidf_vec).sqrt()
            };
            let bm25_norm = {
                let bm25_vec = ndarray::Array1::from_vec(feature_vector.bm25.clone());
                bm25_vec.dot(&bm25_vec).sqrt()
            };
            let embedding_norm = {
                let embedding_vec = ndarray::Array1::from_vec(feature_vector.embedding.clone());
                embedding_vec.dot(&embedding_vec).sqrt()
            };

            tfidf_norms.push(tfidf_norm);
            bm25_norms.push(bm25_norm);
            embedding_norms.push(embedding_norm);

            // ベクトルのノルムをチェック
            let combined = {
                let total_dim = feature_vector.tfidf.len()
                    + feature_vector.bm25.len()
                    + feature_vector.embedding.len();
                let mut combined_vec = Vec::with_capacity(total_dim);
                combined_vec.extend_from_slice(&feature_vector.tfidf);
                combined_vec.extend_from_slice(&feature_vector.bm25);
                combined_vec.extend_from_slice(&feature_vector.embedding);
                ndarray::Array1::from_vec(combined_vec)
            };
            let norm = combined.dot(&combined).sqrt();
            if norm <= 0.0 {
                zero_vector_count += 1;
                if zero_vector_count <= 3 {
                    tracing::warn!(
                        "Golden Dataset article '{}' has zero-norm feature vector (tokens={}, tfidf_sum={:.4}, bm25_sum={:.4}, embedding_sum={:.4})",
                        item.id,
                        tokens.len(),
                        feature_vector.tfidf.iter().sum::<f32>(),
                        feature_vector.bm25.iter().sum::<f32>(),
                        feature_vector.embedding.iter().sum::<f32>()
                    );
                }
            }

            labeled_articles.push(Article {
                id: item.id.clone(),
                content: item.content.clone(),
                genres: item.genres.clone(),
                feature_vector: Some(feature_vector),
            });
        }

        // 統計情報を計算
        let avg_tfidf_norm = if tfidf_norms.is_empty() {
            0.0
        } else {
            tfidf_norms.iter().sum::<f32>() / tfidf_norms.len() as f32
        };
        let avg_bm25_norm = if bm25_norms.is_empty() {
            0.0
        } else {
            bm25_norms.iter().sum::<f32>() / bm25_norms.len() as f32
        };
        let avg_embedding_norm = if embedding_norms.is_empty() {
            0.0
        } else {
            embedding_norms.iter().sum::<f32>() / embedding_norms.len() as f32
        };

        let stats = FeatureExtractionStats {
            zero_vector_count,
            avg_tfidf_norm,
            avg_bm25_norm,
            avg_embedding_norm,
        };

        (labeled_articles, stats)
    }

    /// ゼロベクトルをチェックしてログを出力する。
    fn check_zero_vector(
        article_id: &str,
        normalized: &crate::classification::NormalizedDocument,
        feature_vector: &crate::classification::FeatureVector,
    ) {
        let combined = {
            let total_dim = feature_vector.tfidf.len()
                + feature_vector.bm25.len()
                + feature_vector.embedding.len();
            let mut combined_vec = Vec::with_capacity(total_dim);
            combined_vec.extend_from_slice(&feature_vector.tfidf);
            combined_vec.extend_from_slice(&feature_vector.bm25);
            combined_vec.extend_from_slice(&feature_vector.embedding);
            ndarray::Array1::from_vec(combined_vec)
        };
        let norm = combined.dot(&combined).sqrt();
        if norm <= 0.0 {
            tracing::warn!(
                "CentroidClassifier: zero-norm feature vector for article_id={}, tokens={}, tfidf_sum={:.4}, bm25_sum={:.4}, embedding_sum={:.4}",
                article_id,
                normalized.tokens.len(),
                feature_vector.tfidf.iter().sum::<f32>(),
                feature_vector.bm25.iter().sum::<f32>(),
                feature_vector.embedding.iter().sum::<f32>()
            );
        }
    }

    /// Fast Pass（Centroid Classifier）で分類を試みる。
    fn try_fast_pass(
        &self,
        article_id: &str,
        content: &str,
        feature_vector: &crate::classification::FeatureVector,
        language: ClassificationLanguage,
        sample_count: usize,
    ) -> Result<Option<String>> {
        if let Some(ref centroid_classifier) = self.centroid_classifier {
            if let Some((genre, score)) = centroid_classifier.predict(feature_vector) {
                if sample_count < 10 {
                    tracing::info!(
                        "ClassificationPipeline Fast Pass: article_id={}, genre={}, score={:.4}",
                        article_id,
                        genre,
                        score
                    );
                }
                return Ok(Some(genre));
            }
            // 予測失敗時: 全ジャンル中の最大類似度を計算
            if sample_count < 10 {
                if let Some((genre, similarity, thresh)) =
                    centroid_classifier.get_top_similarity(feature_vector)
                {
                    tracing::warn!(
                        "CentroidClassifier prediction failed: article_id={}, top_genre={}, top_score={:.4}, threshold={:.4}, gap={:.4}",
                        article_id,
                        genre,
                        similarity,
                        thresh,
                        thresh - similarity
                    );
                }
            }
            if sample_count < 10 {
                tracing::warn!(
                    "ClassificationPipeline Fast Pass failed: article_id={}, falling back to GraphPropagator",
                    article_id
                );
            }
        } else {
            // Centroid Classifierが利用できない場合は、フォールバックを使用
            if let Some(ref fallback) = self.fallback_classifier {
                let result = fallback.predict("", content, language)?;
                if let Some(genre) = result.top_genres.first() {
                    return Ok(Some(genre.clone()));
                }
            }
            return Ok(Some("other".to_string()));
        }
        Ok(None)
    }

    /// Rescue Pass（Graph Label Propagation）で分類を試みる。
    fn try_rescue_pass(
        &self,
        article_id: &str,
        content: &str,
        all_articles: &[Article],
        feature_vector: &crate::classification::FeatureVector,
        sample_count: usize,
    ) -> Result<Option<String>> {
        // Centroid Classifierで候補に絞る
        let mut centroid_candidates = HashSet::new();
        if let Some(ref centroid_classifier) = self.centroid_classifier {
            for article in all_articles {
                if let Some(ref fv) = article.feature_vector {
                    if centroid_classifier.predict(fv).is_some() {
                        centroid_candidates.insert(article.id.clone());
                    }
                }
            }
        }

        // 候補数が少ない場合のWARNログ
        if centroid_candidates.len() < 5 {
            tracing::warn!(
                "GraphPropagator: very few centroid candidates ({}), may limit label propagation effectiveness",
                centroid_candidates.len()
            );
        }

        // 現在の記事も追加
        let current_article = Article {
            id: article_id.to_string(),
            content: content.to_string(),
            genres: Vec::new(),
            feature_vector: Some(feature_vector.clone()),
        };

        let mut all_articles_with_current = all_articles.to_vec();
        all_articles_with_current.push(current_article);

        // Graphを構築してラベル伝播
        let mut propagator = GraphPropagator::default();
        propagator.build_graph(&all_articles_with_current, &centroid_candidates)?;
        let propagated_labels = propagator.propagate_labels();

        // 伝播されたラベルを確認
        if let Some(genre) = propagated_labels.get(article_id) {
            if sample_count < 10 {
                tracing::info!(
                    "ClassificationPipeline Rescue Pass: article_id={}, genre={}",
                    article_id,
                    genre
                );
            }
            return Ok(Some(genre.clone()));
        }
        if sample_count < 10 {
            tracing::warn!(
                "ClassificationPipeline Rescue Pass failed: article_id={}, falling back to 'other'",
                article_id
            );
        }
        Ok(None)
    }

    /// 学習済みのジャンル一覧を取得する。
    #[must_use]
    pub fn trained_genres(&self) -> Vec<String> {
        if let Some(ref classifier) = self.centroid_classifier {
            classifier.trained_genres()
        } else {
            Vec::new()
        }
    }

    /// 既存のGenreClassifierインターフェースに合わせたpredictメソッド。
    ///
    /// # Arguments
    /// * `title` - 記事のタイトル
    /// * `body` - 記事の本文
    /// * `language` - 言語
    ///
    /// # Returns
    /// ClassificationResult（既存のGenreClassifierと同じ形式）
    pub fn predict(
        &self,
        title: &str,
        body: &str,
        language: ClassificationLanguage,
    ) -> Result<ClassificationResult> {
        // Centroid Classifierが利用可能な場合はそれを使用
        if let Some(ref centroid_classifier) = self.centroid_classifier {
            let normalized = self.token_pipeline.preprocess(title, body, language);
            let feature_vector = self.feature_extractor.extract(&normalized.tokens);

            // Centroid Classifierで分類を試みる
            let mut scores = HashMap::new();
            let mut ranking = Vec::new();

            if let Some((genre, score)) = centroid_classifier.predict(&feature_vector) {
                tracing::debug!(
                    "CentroidClassifier prediction: genre={}, score={:.4}",
                    genre,
                    score
                );
                scores.insert(genre.clone(), score);
                ranking.push((genre, score));
            } else {
                tracing::debug!(
                    "CentroidClassifier prediction: no genre above threshold (trained_genres: {:?})",
                    centroid_classifier.trained_genres()
                );
            }

            // スコアが空の場合はRescue Pass (Graph Propagator) を試行
            if scores.is_empty() {
                if let Some(ref propagator) = self.graph_propagator {
                    tracing::info!(
                        "CentroidClassifier: no matches, attempting Rescue Pass (GraphPropagator)"
                    );
                    // k=5, 動的閾値で近傍探索
                    let empty_thresholds = HashMap::new();
                    let thresholds = self
                        .centroid_classifier
                        .as_ref()
                        .map_or(&empty_thresholds, |c| c.get_thresholds());
                    if let Some((rescued_genre, score)) =
                        propagator.predict_by_neighbors(&feature_vector, 5, thresholds)
                    {
                        tracing::info!(
                            "Rescue Pass successful: genre='{}', score={:.4}",
                            rescued_genre,
                            score
                        );
                        scores.insert(rescued_genre.clone(), score);
                        ranking.push((rescued_genre, score));
                    } else {
                        tracing::debug!("Rescue Pass failed: no neighbors found");
                    }
                }
            }

            // それでもスコアが空の場合は"other"を追加
            if scores.is_empty() {
                tracing::debug!("CentroidClassifier: no matches, falling back to 'other'");
                scores.insert("other".to_string(), 0.0);
                ranking.push(("other".to_string(), 0.0));
            }

            // ランキングをスコア順にソート
            ranking.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

            // top_genresを取得（最大3つ）
            let top_genres: Vec<String> = ranking
                .iter()
                .take(3)
                .map(|(genre, _)| genre.clone())
                .collect();

            // keyword_hitsを設定（既存のGenreClassifierインターフェースとの互換性のため）
            // スコアを100倍して整数に変換（既存のロジックとの互換性のため）
            let mut keyword_hits = HashMap::new();
            for (genre, score) in &scores {
                let rounded = (score.max(0.0) * 100.0).round().max(0.0);
                let hit_count = u32::try_from(rounded as i64).unwrap_or(0).max(1);
                keyword_hits.insert(genre.clone(), hit_count as usize);
            }

            return Ok(ClassificationResult {
                top_genres,
                scores,
                ranking,
                feature_snapshot: feature_vector,
                keyword_hits,
                token_count: normalized.tokens.len(),
            });
        }

        // フォールバック: 既存のGenreClassifierを使用
        if let Some(ref fallback) = self.fallback_classifier {
            return fallback.predict(title, body, language);
        }

        // どちらも利用できない場合はエラー
        anyhow::bail!("Neither CentroidClassifier nor GenreClassifier is available")
    }
}
