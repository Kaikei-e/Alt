//! 分類パイプラインのワークフロー統合
//! Centroid Classifier + Graph Label Propagation の統合処理

use std::collections::HashSet;
use std::fs;
use std::path::Path;

use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};

use std::collections::HashMap;

use crate::classification::{
    Article, ClassificationLanguage, ClassificationResult, FeatureExtractor, GenreClassifier,
    TokenPipeline,
};
use crate::classifier::GraphPropagator;

// Rescue Passの統計カウンター（ログサンプリング用）
static RESCUE_ATTEMPT_COUNT: std::sync::atomic::AtomicUsize =
    std::sync::atomic::AtomicUsize::new(0);
static RESCUE_SUCCESS_COUNT: std::sync::atomic::AtomicUsize =
    std::sync::atomic::AtomicUsize::new(0);
static RESCUE_FAIL_COUNT: std::sync::atomic::AtomicUsize = std::sync::atomic::AtomicUsize::new(0);

/// Golden Datasetのトップレベル構造
#[derive(Debug, Deserialize)]
#[allow(dead_code)]
struct GoldenDatasetRoot {
    #[serde(default)]
    schema_version: Option<String>,
    #[serde(default)]
    taxonomy_version: Option<String>,
    #[serde(default)]
    genres: Vec<String>,
    #[serde(default, rename = "facets_suggestion")]
    facets_suggestion: Vec<String>,
    items: Vec<GoldenItem>,
}

/// Golden Datasetのアイテム（JSON形式）
#[derive(Debug, Deserialize, Serialize, Clone)]
pub struct GoldenItem {
    pub id: String,
    #[serde(default)]
    pub content_ja: Option<String>,
    #[serde(default)]
    pub content_en: Option<String>,
    #[serde(default)]
    pub content: Option<String>, // レガシー対応
    #[serde(rename = "expected_genres")]
    pub genres: Vec<String>,
}

impl GoldenItem {
    /// コンテンツを取得（content_en優先、次にcontent_ja、最後にcontent）
    pub fn content(&self) -> String {
        if let Some(ref en) = self.content_en {
            if !en.trim().is_empty() {
                return en.clone();
            }
        }
        if let Some(ref ja) = self.content_ja {
            if !ja.trim().is_empty() {
                return ja.clone();
            }
        }
        if let Some(ref content) = self.content {
            return content.clone();
        }
        String::new()
    }
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
                        "ClassificationPipeline initialized from {}",
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
                            "ClassificationPipeline initialized from {}",
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

        // 新しいスキーマ（items配列あり）とレガシースキーマ（直接配列）の両方に対応
        let golden_items: Vec<GoldenItem> =
            if let Ok(root) = serde_json::from_str::<GoldenDatasetRoot>(&content) {
                root.items
            } else {
                // レガシー形式：直接配列としてパース
                serde_json::from_str(&content)
                    .context("failed to parse golden dataset JSON (legacy format)")?
            };

        let token_pipeline = TokenPipeline::new();

        // 1. すべての記事をトークン化してコーパスを構築
        let mut tokenized_corpus = Vec::new();
        for item in &golden_items {
            let language = ClassificationLanguage::Unknown;
            let content = item.content();
            let normalized = token_pipeline.preprocess(&content, "", language);
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

        // ログ出力: Golden Dataset特徴抽出統計
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

        // 5. CentroidClassifier Removed

        // Initialize GraphPropagator with lower threshold
        let mut graph_propagator = GraphPropagator::new(0.5); // エッジ構築用閾値
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
            graph_propagator: Some(graph_propagator),
            // fallback_classifier: Some(GenreClassifier::new_default()), // REMOVED: Specified in line above?
            // Oh wait, duplicate field error was in struct usage?
            // "field fallback_classifier specified more than once"
            // Let's see lines 249-253 in original view:
            // 249:             centroid_classifier: Some(centroid_classifier),
            // 250:             graph_propagator: Some(graph_propagator),
            // 251:             fallback_classifier: None,
            // 252:             feature_extractor,
            // 253:             token_pipeline: TokenPipeline::new(),
            // I replaced lines 226-255.
            // My replacement was:
            // ...
            // Ok(Self {
            //    graph_propagator: Some(graph_propagator),
            //    fallback_classifier: None,
            //    fallback_classifier: Some(GenreClassifier::new_default()),
            //    feature_extractor,
            //    token_pipeline: TokenPipeline::new(),
            // })
            // Yes, I duplicated it blindly. I should remove one.
            fallback_classifier: Some(GenreClassifier::new_default()),
            feature_extractor,
            token_pipeline: TokenPipeline::new(),
        })
    }

    pub fn feature_extractor(&self) -> &FeatureExtractor {
        &self.feature_extractor
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

        // ログ出力: 特徴ベクトルのノルム（デバッグ用）
        let tfidf_norm = ndarray::Array1::from_vec(feature_vector.tfidf.clone())
            .dot(&ndarray::Array1::from_vec(feature_vector.tfidf.clone()))
            .sqrt();
        let bm25_norm = ndarray::Array1::from_vec(feature_vector.bm25.clone())
            .dot(&ndarray::Array1::from_vec(feature_vector.bm25.clone()))
            .sqrt();
        let embedding_norm = ndarray::Array1::from_vec(feature_vector.embedding.clone())
            .dot(&ndarray::Array1::from_vec(feature_vector.embedding.clone()))
            .sqrt();

        if sample_count < 10 {
            tracing::info!(
                "ClassificationPipeline classify: article_id={}, tfidf_norm={:.4}, bm25_norm={:.4}, embedding_norm={:.4}",
                article_id,
                tfidf_norm,
                bm25_norm,
                embedding_norm
            );
        }

        // Fast Pass: Fallback (GenreClassifier) で分類
        let fallback_result = if let Some(ref fallback) = self.fallback_classifier {
            fallback.predict("", content, language).ok()
        } else {
            None
        };

        if let Some(res) = fallback_result {
            if let Some(genre) = res.top_genres.first() {
                // "other" 以外なら採用、あるいは信頼度チェック？
                // GenreClassifierは閾値判定済みなので、返ってきたものは採用してよい。
                if genre != "other" {
                    return Ok(genre.clone());
                }
            }
        }

        // Rescue Pass: Graph Label Propagation
        if let Some(genre) = Self::try_rescue_pass(
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
                content: item.content(),
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
        _article_id: &str,
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
                "Zero-norm feature vector: tokens={}, tfidf_sum={:.4}, bm25_sum={:.4}, embedding_sum={:.4}",
                normalized.tokens.len(),
                feature_vector.tfidf.iter().sum::<f32>(),
                feature_vector.bm25.iter().sum::<f32>(),
                feature_vector.embedding.iter().sum::<f32>()
            );
        }
    }

    /// Rescue Pass（Graph Label Propagation）で分類を試みる。
    fn try_rescue_pass(
        article_id: &str,
        content: &str,
        all_articles: &[Article],
        feature_vector: &crate::classification::FeatureVector,
        sample_count: usize,
    ) -> Result<Option<String>> {
        // Centroid Classifierで候補に絞るロジックを廃止し、全記事を候補とする
        let mut centroid_candidates = HashSet::new();
        for article in all_articles {
            centroid_candidates.insert(article.id.clone());
        }

        // 現在の記事も候補に追加（これがないとグラフに含まれない）
        centroid_candidates.insert(article_id.to_string());

        // ログ出力: Rescue Pass候補数
        tracing::info!(
            "Rescue Pass: article_id={}, total_articles={}, centroid_candidates={}",
            article_id,
            all_articles.len(),
            centroid_candidates.len()
        );

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
        self.fallback_classifier
            .as_ref()
            .map(crate::classification::GenreClassifier::known_genres)
            .unwrap_or_default()
    }

    /// predictメソッド内でRescue Passを試行する（ログのサンプリング付き）。
    fn try_rescue_pass_in_predict(
        &self,
        feature_vector: &crate::classification::FeatureVector,
        scores: &mut HashMap<String, f32>,
        ranking: &mut Vec<(String, f32)>,
        sample_count: usize,
    ) {
        if let Some(ref propagator) = self.graph_propagator {
            let attempt_count =
                RESCUE_ATTEMPT_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed);

            // Sample attempts: first 10, then every 100
            #[allow(clippy::manual_is_multiple_of)]
            if sample_count < 10 || (attempt_count + 1) % 100 == 0 {
                tracing::info!(
                    "CentroidClassifier: no matches, attempting Rescue Pass (GraphPropagator) [attempts={}]",
                    attempt_count + 1
                );
            }

            // k=5, static thresholds since Centroid is gone
            let mut thresholds = HashMap::new();
            if let Some(_fallback) = &self.fallback_classifier {
                // Use default threshold from GenreClassifier as base?
                // Just use 0.5 for now as a safe bet for Rescue
                thresholds.insert("default".to_string(), 0.5);
            }

            if let Some((rescued_genre, score)) =
                propagator.predict_by_neighbors(feature_vector, 5, &thresholds)
            {
                let success_count =
                    RESCUE_SUCCESS_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed);

                tracing::info!(
                    "Rescue Pass successful: genre='{}', score={:.4} [successes={}]",
                    rescued_genre,
                    score,
                    success_count + 1
                );
                scores.insert(rescued_genre.clone(), score);
                ranking.push((rescued_genre, score));
            } else {
                let fail_count =
                    RESCUE_FAIL_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed);

                // Sample failures: first 10, then every 10
                // We want to see failures to debug "no evidence" issues, but not flood logs if 100% fail.
                #[allow(clippy::manual_is_multiple_of)]
                if sample_count < 10 || (fail_count + 1) % 10 == 0 {
                    tracing::info!(
                        "Rescue Pass failed: no neighbors found [failures={}]",
                        fail_count + 1
                    );
                }
            }
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
        // サンプリング用のカウンター（静的変数）
        static PREDICT_SAMPLE_COUNT: std::sync::atomic::AtomicUsize =
            std::sync::atomic::AtomicUsize::new(0);

        let sample_count = PREDICT_SAMPLE_COUNT.fetch_add(1, std::sync::atomic::Ordering::Relaxed);

        // GenreClassifier (Fallback) を使用
        if let Some(ref fallback) = self.fallback_classifier {
            let mut result = fallback.predict(title, body, language)?;

            // Rescue Pass (Graph Label Propagation)
            if result.top_genres.first().is_none_or(|g| g == "other") {
                // 特徴ベクトル抽出 (FeatureExtractorはPipelineが持っているものを使う)
                let normalized = self.token_pipeline.preprocess(title, body, language);
                let feature_vector = self.feature_extractor.extract(&normalized.tokens);

                // Resultのランキングを更新するために、mutable scores/rankingが必要だが
                // ClassificationResultはfeature_snapshotなどを持っている。
                // FeatureVectorを再計算してしまっているが、ここではPipelineのfeature_extractorを使う必要がある
                // (Fallback内部のTFIDFとは違う可能性があるため)

                // ここでRescue Passを呼び出し、もし結果があればresultを上書き/マージする
                // しかし try_rescue_pass は all_articles が必要だが、predictの引数にはない。
                // したがって、この signature の predict では Rescue Pass は実行できない。
                // try_rescue_pass_in_predict も Ranking を更新するヘルパーだが、GraphPropagatorが必要。
                // GraphPropagator は self にあるので呼べる。

                // 既存の try_rescue_pass_in_predict を流用する
                self.try_rescue_pass_in_predict(
                    &feature_vector,
                    &mut result.scores,
                    &mut result.ranking,
                    sample_count,
                );

                // ランキング再ソート
                result
                    .ranking
                    .sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

                // top_genres更新
                let top_genres: Vec<String> = result
                    .ranking
                    .iter()
                    .take(3)
                    .map(|(genre, _)| genre.clone())
                    .collect();
                result.top_genres = top_genres;
            }

            return Ok(result);
        }

        anyhow::bail!("GenreClassifier is not available")
    }
}
