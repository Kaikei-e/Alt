//! 計測・評価用のユーティリティ群。
use rand::{Rng, SeedableRng, rngs::StdRng};
use std::collections::HashMap;

use crate::pipeline::preprocess::preprocess_article;

/// 合成記事を生成する。
///
/// # Arguments
/// * `count` - 生成する記事数
/// * `avg_sentences` - 1記事あたりの平均文数（±2の揺らぎで決定）
#[must_use]
#[allow(clippy::too_many_lines)]
pub fn synthetic_bodies(count: usize, avg_sentences: usize) -> Vec<String> {
    let mut rng = StdRng::seed_from_u64(42);
    let templates = [
        (
            "ai",
            "AI system launched for {sector} with {impact} impact on {region}. \
             Researchers reported {metric} improvements during {event}.",
        ),
        (
            "business",
            "{company} announced a {action} worth {amount} focusing on {market}. \
             Analysts expect {trend} throughout the fiscal year.",
        ),
        (
            "sports",
            "{team} secured a {result} against {opponent} with {score}. \
             Coach highlighted {keyword} ahead of the tournament.",
        ),
        (
            "science",
            "Study on {subject} reveals {observation} after {duration} of experimentation. \
             Findings could transform {application} workflows.",
        ),
    ];

    let replacements = [
        (
            "sector",
            ["finance", "healthcare", "logistics", "education"].as_ref(),
        ),
        (
            "impact",
            ["major", "moderate", "transformative", "incremental"].as_ref(),
        ),
        (
            "region",
            ["Asia", "Europe", "North America", "Japan"].as_ref(),
        ),
        ("metric", ["30%", "2x", "45%", "1.5x"].as_ref()),
        (
            "event",
            [
                "pilot rollout",
                "beta testing",
                "initial deployment",
                "trial phase",
            ]
            .as_ref(),
        ),
        (
            "company",
            [
                "Alt Systems",
                "Nova Labs",
                "Parallel Fusion",
                "Cortex Industries",
            ]
            .as_ref(),
        ),
        (
            "action",
            [
                "merger",
                "acquisition",
                "investment",
                "strategic partnership",
            ]
            .as_ref(),
        ),
        ("amount", ["$1.2B", "$450M", "$800M", "$3.1B"].as_ref()),
        (
            "market",
            ["APAC", "global enterprise", "consumer tech", "energy"].as_ref(),
        ),
        (
            "trend",
            [
                "steady growth",
                "short-term volatility",
                "margin expansion",
                "strong demand",
            ]
            .as_ref(),
        ),
        (
            "team",
            [
                "Tokyo Sparks",
                "Osaka Dynamos",
                "Nagoya Blitz",
                "Kyoto Falcons",
            ]
            .as_ref(),
        ),
        (
            "result",
            ["victory", "draw", "loss", "comeback win"].as_ref(),
        ),
        (
            "opponent",
            [
                "Seoul Titans",
                "Taipei Hawks",
                "Shanghai Storm",
                "Fukuoka Waves",
            ]
            .as_ref(),
        ),
        ("score", ["3-1", "2-2", "1-0", "4-3"].as_ref()),
        (
            "keyword",
            ["fitness", "possession control", "defense", "set plays"].as_ref(),
        ),
        (
            "subject",
            [
                "quantum sensors",
                "fusion reactors",
                "genomic sequencing",
                "climate modeling",
            ]
            .as_ref(),
        ),
        (
            "observation",
            [
                "significant variance",
                "unexpected stability",
                "record efficiency",
                "reduced latency",
            ]
            .as_ref(),
        ),
        (
            "duration",
            ["18 months", "six quarters", "nine weeks", "three years"].as_ref(),
        ),
        (
            "application",
            [
                "manufacturing",
                "financial trading",
                "satellite imaging",
                "drug discovery",
            ]
            .as_ref(),
        ),
    ];

    let mut bodies = Vec::with_capacity(count);
    for idx in 0..count {
        let (genre, template) = templates[rng.gen_range(0..templates.len())];
        let sentences = rng.gen_range((avg_sentences.saturating_sub(2))..=(avg_sentences + 2));
        let mut body_parts = Vec::with_capacity(sentences);

        for _ in 0..sentences {
            let mut sentence = template.to_string();
            for (key, options) in &replacements {
                let choice = options[rng.gen_range(0..options.len())];
                sentence = sentence.replace(&format!("{{{key}}}"), choice);
            }
            body_parts.push(sentence.clone());
        }

        let body = body_parts.join(" ");
        let lang_marker = if genre == "ai" || genre == "science" {
            "[lang=en]"
        } else {
            "[lang=ja]"
        };
        bodies.push(format!("{lang_marker} id-{idx} {body}"));
    }

    bodies
}

/// 前処理後の本文と言語情報。
#[derive(Debug, Clone)]
pub struct ProcessedDocument {
    pub body: String,
    pub language: String,
}

/// 合成記事群を前処理し、ドロップせずに残った件数を返す。
#[must_use]
pub fn preprocess_documents(bodies: &[String]) -> (usize, Vec<ProcessedDocument>) {
    let mut processed = Vec::with_capacity(bodies.len());
    for (idx, body) in bodies.iter().enumerate() {
        let (language_hint, text) = body
            .split_once(' ')
            .map(|(tag, rest)| (Some(tag.replace(['[', ']'], "")), rest))
            .unwrap_or((None, body.as_str()));

        let article = crate::pipeline::fetch::FetchedArticle {
            id: format!("synthetic-{idx}"),
            title: Some(format!("synthetic headline {idx}")),
            body: text.to_string(),
            language: language_hint.map(|tag| {
                if let Some(code) = tag.split('=').nth(1) {
                    code.to_string()
                } else {
                    "und".to_string()
                }
            }),
            published_at: None,
            source_url: None,
            tags: Vec::new(),
        };

        if let Ok(Some(result)) = preprocess_article(article) {
            processed.push(ProcessedDocument {
                body: result.body,
                language: result.language,
            });
        }
    }
    (processed.len(), processed)
}

/// 前処理済みテキストをジャンルごとにスコアリングする。
///
/// 戻り値は `genre -> score` のマップのリスト。
#[must_use]
pub fn keyword_scores(documents: &[ProcessedDocument]) -> Vec<HashMap<String, usize>> {
    use crate::pipeline::genre_keywords::GenreKeywords;

    let keywords = GenreKeywords::default_keywords();
    documents
        .iter()
        .map(|doc| keywords.score_text(&doc.body))
        .collect()
}
