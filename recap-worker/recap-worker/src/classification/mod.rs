//! ジャンル分類のための高水準API。
use std::collections::HashMap;
use std::path::Path;

use anyhow::Result;

use crate::pipeline::genre_keywords::GenreKeywords;

mod features;
mod model;
mod tokenizer;

use features::{FeatureExtractor, FeatureVector};
use model::HybridModel;
use tokenizer::{NormalizedDocument, TokenPipeline};

/// 分類対象テキストの言語。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ClassificationLanguage {
    Japanese,
    English,
    Unknown,
}

impl ClassificationLanguage {
    #[must_use]
    pub fn from_code(code: &str) -> Self {
        match code.to_lowercase().as_str() {
            "ja" | "jp" => Self::Japanese,
            "en" | "us" | "uk" => Self::English,
            _ => Self::Unknown,
        }
    }
}

/// 分類結果。
#[derive(Debug, Clone)]
pub struct ClassificationResult {
    pub top_genres: Vec<String>,
    pub scores: HashMap<String, f32>,
}

/// ジャンル分類器の外部インターフェース。
#[derive(Debug)]
pub struct GenreClassifier {
    keywords: GenreKeywords,
    top_k: usize,
    pipeline: TokenPipeline,
    feature_extractor: FeatureExtractor,
    model: HybridModel,
    score_threshold: f32,
}

impl GenreClassifier {
    /// 本番環境向けのデフォルト構成。
    #[must_use]
    pub fn new_default() -> Self {
        let model = match std::env::var("RECAP_GENRE_MODEL_WEIGHTS") {
            Ok(path) => HybridModel::from_path(Some(Path::new(&path)))
                .unwrap_or_else(|err| panic!("failed to load classifier weights: {err}")),
            Err(_) => HybridModel::new().expect("hybrid model initialization"),
        };
        let score_threshold = std::env::var("RECAP_GENRE_MODEL_THRESHOLD")
            .ok()
            .and_then(|raw| raw.parse::<f32>().ok())
            .unwrap_or(0.75);
        Self {
            keywords: GenreKeywords::default_keywords(),
            top_k: 3,
            pipeline: TokenPipeline::new(),
            feature_extractor: FeatureExtractor::new(),
            model,
            score_threshold,
        }
    }

    /// テスト用ヘルパー。
    #[must_use]
    pub fn new_test() -> Self {
        Self::new_default()
    }

    /// テキストを分類し、上位ジャンルを返す。
    ///
    /// # Errors
    /// 現状の実装ではエラーを返さないが、将来的に外部モデルを利用する際に備えて `Result` を返す。
    pub fn predict(
        &self,
        title: &str,
        body: &str,
        _language: ClassificationLanguage,
    ) -> Result<ClassificationResult> {
        let NormalizedDocument {
            tokens, normalized, ..
        } = self
            .pipeline
            .preprocess(title.trim(), body.trim(), _language);
        let keyword_scores = self.keywords.score_text(&normalized);
        let mut combined_scores = keyword_scores
            .iter()
            .map(|(genre, score)| (genre.clone(), *score as f32))
            .into_iter()
            .collect::<HashMap<_, _>>();

        let feature_vector: FeatureVector = self.feature_extractor.extract(&tokens);
        let blend_weight = 0.35;
        for (genre, score) in self.model.predict(&feature_vector)? {
            let blended = score * blend_weight;
            combined_scores
                .entry(genre)
                .and_modify(|value| *value += blended)
                .or_insert(blended);
        }

        if combined_scores.is_empty() {
            combined_scores.insert("other".to_string(), 0.0);
        }

        let mut ranked = combined_scores
            .iter()
            .map(|(genre, score)| (genre.clone(), *score))
            .collect::<Vec<_>>();
        ranked.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));

        let mut filtered: Vec<String> = Vec::new();
        for (genre, score) in ranked.iter() {
            let keyword_support = keyword_scores.get(genre).copied().unwrap_or(0);
            let required = if keyword_support > 0 {
                self.score_threshold
            } else {
                self.score_threshold * 1.25
            };
            if *score >= required {
                if genre == "world" && keyword_support < 2 {
                    continue;
                }
                if genre == "business" && keyword_support == 0 {
                    continue;
                }
                if genre == "entertainment" && keyword_support == 0 {
                    continue;
                }
                filtered.push(genre.clone());
            }
            if filtered.len() == self.top_k {
                break;
            }
        }

        if filtered.is_empty() {
            if let Some((best_genre, _)) = ranked.first() {
                filtered.push(best_genre.clone());
            } else {
                filtered.push("other".to_string());
            }
        }

        Ok(ClassificationResult {
            top_genres: filtered,
            scores: combined_scores,
        })
    }
}
