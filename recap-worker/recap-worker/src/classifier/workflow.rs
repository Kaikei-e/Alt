//! 分類パイプラインのワークフロー統合
//! Centroid Classifier + Graph Label Propagation の統合処理

use std::collections::HashSet;
use std::fs;
use std::path::Path;

use anyhow::{Context, Result};
use serde::Deserialize;

use std::collections::HashMap;

use crate::classification::{
    ClassificationLanguage, ClassificationResult, FeatureExtractor, TokenPipeline,
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

/// 分類パイプライン
/// Centroid ClassifierとGraph Label Propagationを統合した分類器
#[derive(Debug)]
pub struct ClassificationPipeline {
    centroid_classifier: CentroidClassifier,
    feature_extractor: FeatureExtractor,
    token_pipeline: TokenPipeline,
}

impl ClassificationPipeline {
    /// デフォルトのGolden Datasetパスからパイプラインを初期化する。
    ///
    /// # Errors
    /// ファイルの読み込みやパースに失敗した場合にエラーを返す。
    pub fn new() -> Result<Self> {
        // 環境変数からパスを取得、なければデフォルトパスを使用
        let default_path = std::env::var("RECAP_GOLDEN_DATASET_PATH").unwrap_or_else(|_| {
            // 実行ファイルからの相対パスまたはCARGO_MANIFEST_DIRからの相対パス
            let manifest_dir =
                std::env::var("CARGO_MANIFEST_DIR").unwrap_or_else(|_| ".".to_string());
            format!("{}/../tests/data/golden_classification.json", manifest_dir)
        });
        Self::from_golden_dataset(Path::new(&default_path))
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

        // FeatureExtractorを初期化（fallbackを使用）
        let feature_extractor = FeatureExtractor::fallback();
        let token_pipeline = TokenPipeline::new();

        // 特徴ベクトルの次元数を計算
        // tfidf + bm25 + embedding
        // FALLBACK_VOCABの長さを使用（19）
        let vocab_len = 19; // FALLBACK_VOCAB.len()
        let embedding_dim = 6; // EMBEDDING_DIM
        let feature_dim = vocab_len + vocab_len + embedding_dim;

        // Golden Datasetから特徴ベクトルを抽出
        let mut labeled_articles = Vec::new();
        for item in &golden_items {
            let language = ClassificationLanguage::Unknown;
            let normalized = token_pipeline.preprocess(&item.content, "", language);
            let feature_vector = feature_extractor.extract(&normalized.tokens);

            labeled_articles.push(Article {
                id: item.id.clone(),
                content: item.content.clone(),
                genres: item.genres.clone(),
                feature_vector: Some(feature_vector),
            });
        }

        // CentroidClassifierを学習
        let mut centroid_classifier = CentroidClassifier::new(feature_dim);
        centroid_classifier.train(&labeled_articles)?;

        Ok(Self {
            centroid_classifier,
            feature_extractor,
            token_pipeline,
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
        // 特徴ベクトルを抽出
        let language = ClassificationLanguage::Unknown;
        let normalized = self.token_pipeline.preprocess(content, "", language);
        let feature_vector = self.feature_extractor.extract(&normalized.tokens);

        // Fast Pass: Centroid Classifierで分類
        if let Some((genre, _score)) = self.centroid_classifier.predict(&feature_vector) {
            return Ok(genre);
        }

        // Rescue Pass: Graph Label Propagation
        // Centroid Classifierで候補に絞る
        let mut centroid_candidates = HashSet::new();
        for article in all_articles {
            if let Some(ref fv) = article.feature_vector {
                if self.centroid_classifier.predict(fv).is_some() {
                    centroid_candidates.insert(article.id.clone());
                }
            }
        }

        // 現在の記事も追加
        let current_article = Article {
            id: article_id.to_string(),
            content: content.to_string(),
            genres: Vec::new(),
            feature_vector: Some(feature_vector),
        };

        let mut all_articles_with_current = all_articles.to_vec();
        all_articles_with_current.push(current_article);

        // Graphを構築してラベル伝播
        let mut propagator = GraphPropagator::default();
        propagator.build_graph(&all_articles_with_current, &centroid_candidates)?;
        let propagated_labels = propagator.propagate_labels();

        // 伝播されたラベルを確認
        if let Some(genre) = propagated_labels.get(article_id) {
            return Ok(genre.clone());
        }

        // Fallback: 分類できない場合は "other"
        Ok("other".to_string())
    }

    /// 学習済みのジャンル一覧を取得する。
    #[must_use]
    pub fn trained_genres(&self) -> Vec<String> {
        self.centroid_classifier.trained_genres()
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
        let normalized = self.token_pipeline.preprocess(title, body, language);
        let feature_vector = self.feature_extractor.extract(&normalized.tokens);

        // Centroid Classifierで分類を試みる
        let mut scores = HashMap::new();
        let mut ranking = Vec::new();

        if let Some((genre, score)) = self.centroid_classifier.predict(&feature_vector) {
            scores.insert(genre.clone(), score);
            ranking.push((genre, score));
        }

        // スコアが空の場合は"other"を追加
        if scores.is_empty() {
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

        // keyword_hitsは空（新分類器では使用しない）
        let keyword_hits = HashMap::new();

        Ok(ClassificationResult {
            top_genres,
            scores,
            ranking,
            feature_snapshot: feature_vector,
            keyword_hits,
            token_count: normalized.tokens.len(),
        })
    }
}
