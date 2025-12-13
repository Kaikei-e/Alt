//! 言語別のトークナイズと正規化処理。
use lindera::tokenizer::{Tokenizer as LinderaTokenizer, TokenizerConfig as LinderaConfig};
use regex::Regex;
use unicode_normalization::UnicodeNormalization;
use unicode_segmentation::UnicodeSegmentation;

use super::ClassificationLanguage;
use crate::language_detection::detect_lang;

fn normalize_text(input: &str) -> String {
    input.nfc().collect::<String>()
}

#[derive(Debug)]
#[allow(dead_code)] // english field is kept for potential future state
pub struct TokenPipeline {
    japanese: JapaneseTokenizer,
    english: EnglishTokenizer,
    fallback: FallbackTokenizer,
}

#[derive(Debug)]
pub struct NormalizedDocument {
    pub tokens: Vec<String>,
    pub normalized: String,
}

impl TokenPipeline {
    #[must_use]
    pub fn new() -> Self {
        Self {
            japanese: JapaneseTokenizer::new(),
            english: EnglishTokenizer::new(),
            fallback: FallbackTokenizer::new(),
        }
    }

    #[must_use]
    pub fn resolve_language(
        provided: ClassificationLanguage,
        text: &str,
    ) -> ClassificationLanguage {
        match provided {
            ClassificationLanguage::Unknown => {
                let (detected_lang, _confidence) = detect_lang(text);
                detected_lang
            }
            other => other,
        }
    }

    #[must_use]
    pub fn tokenize(&self, text: &str, lang: ClassificationLanguage) -> Vec<String> {
        match lang {
            ClassificationLanguage::Japanese => self.japanese.tokenize(text),
            ClassificationLanguage::English => EnglishTokenizer::tokenize(text),
            ClassificationLanguage::Unknown => self.fallback.tokenize(text),
        }
    }

    #[must_use]
    pub fn preprocess(
        &self,
        title: &str,
        body: &str,
        lang: ClassificationLanguage,
    ) -> NormalizedDocument {
        let combined = format!("{title} {body}");
        let resolved = Self::resolve_language(lang, &combined);
        let mut tokens = self.tokenize(&combined, resolved);
        Self::augment_tokens(&mut tokens, resolved);
        let normalized = tokens.join(" ");
        NormalizedDocument { tokens, normalized }
    }

    fn augment_tokens(tokens: &mut Vec<String>, lang: ClassificationLanguage) {
        match lang {
            ClassificationLanguage::Japanese => apply_augmented_tokens(
                tokens,
                &[
                    ("資本提携", &["資金調達", "投資"]),
                    ("政調会長", &["政策", "政府"]),
                    ("ゲノム", &["遺伝子", "医療"]),
                    ("干渉計", &["量子", "研究"]),
                    ("劇伴", &["音楽", "エンタメ"]),
                    ("自律走行", &["自動運転", "人工知能"]),
                ],
            ),
            ClassificationLanguage::English => apply_augmented_tokens(
                tokens,
                &[
                    (
                        "confidential",
                        &["confidential computing", "cloud", "cybersecurity"],
                    ),
                    ("attestation", &["cybersecurity", "cloud"]),
                    ("ceasefire", &["diplomacy", "treaty"]),
                    ("reconstruction", &["economy", "business"]),
                    ("multimodal", &["transformer", "machine learning"]),
                ],
            ),
            ClassificationLanguage::Unknown => {}
        }
    }
}

impl Default for TokenPipeline {
    fn default() -> Self {
        Self::new()
    }
}

fn apply_augmented_tokens(tokens: &mut Vec<String>, mapping: &[(&str, &[&str])]) {
    let mut extras = Vec::new();
    for (needle, synonyms) in mapping {
        if tokens
            .iter()
            .any(|token| token == needle || token.contains(needle))
        {
            extras.extend(synonyms.iter().map(|syn| syn.to_lowercase()));
        }
    }
    tokens.extend(extras);
}

#[derive(Debug)]
struct JapaneseTokenizer {
    lindera: Option<LinderaAdapter>,
    // #[cfg(feature = "with-sudachi")]
    // sudachi: Option<SudachiAdapter>,  // Disabled: sudachi crate not available on crates.io
    fallback_word_re: Regex,
}

impl JapaneseTokenizer {
    fn new() -> Self {
        Self {
            lindera: LinderaAdapter::new(),
            // #[cfg(feature = "with-sudachi")]
            // sudachi: SudachiAdapter::new(),  // Disabled: sudachi crate not available on crates.io
            fallback_word_re: Regex::new(r"[^\p{L}\p{N}]+").expect("compile fallback regex"),
        }
    }

    fn tokenize(&self, text: &str) -> Vec<String> {
        if let Some(adapter) = &self.lindera
            && let Some(tokens) = adapter.tokenize(text)
            && !tokens.is_empty()
        {
            return tokens;
        }
        // #[cfg(feature = "with-sudachi")]
        // if let Some(adapter) = &self.sudachi {
        //     if let Some(tokens) = adapter.tokenize(text) {
        //         if !tokens.is_empty() {
        //             return tokens;
        //         }
        //     }
        // }
        self.fallback_tokenize(text)
    }

    fn fallback_tokenize(&self, text: &str) -> Vec<String> {
        normalize_text(text)
            .split(|c: char| c.is_whitespace())
            .flat_map(|piece| self.fallback_word_re.split(piece))
            .filter(|token| !token.is_empty())
            .map(ToString::to_string)
            .collect()
    }
}

// Sudachi support is disabled because the sudachi crate is not available on crates.io
// When the sudachi crate becomes available, uncomment the following code:
//
// #[derive(Debug)]
// struct SudachiAdapter {
//     tokenizer: sudachi::analysis::stateless_tokenizer::Tokenizer,
// }
//
// impl SudachiAdapter {
//     fn new() -> Option<Self> {
//         use sudachi::config::{Config, ConfigBuilder};
//         let config = if let Ok(path) = std::env::var("SUDACHI_CONFIG_PATH") {
//             Config::from_file(&path).ok()?
//         } else {
//             ConfigBuilder::new().build().ok()?
//         };
//         sudachi::analysis::stateless_tokenizer::Tokenizer::new(config)
//             .ok()
//             .map(|tokenizer| Self { tokenizer })
//     }
//
//     fn tokenize(&self, text: &str) -> Option<Vec<String>> {
//         use sudachi::prelude::Mode;
//         let morphemes = self
//             .tokenizer
//             .tokenize(Mode::C, text)
//             .ok()?
//             .into_iter()
//             .map(|morpheme| {
//                 morpheme
//                     .lemma()
//                     .unwrap_or_else(|| morpheme.surface().to_string())
//             })
//             .filter(|token| !token.trim().is_empty())
//             .collect::<Vec<_>>();
//         Some(morphemes)
//     }
// }

struct LinderaAdapter {
    tokenizer: LinderaTokenizer,
}

impl LinderaAdapter {
    fn new() -> Option<Self> {
        let config = LinderaConfig::default();
        LinderaTokenizer::from_config(config)
            .ok()
            .map(|tokenizer| Self { tokenizer })
    }

    fn tokenize(&self, text: &str) -> Option<Vec<String>> {
        let tokens = self.tokenizer.tokenize(text).ok()?;
        let mut results: Vec<String> = Vec::with_capacity(tokens.len());
        for token in tokens {
            let surface = token.get_text().trim();
            if !surface.is_empty() {
                results.push(surface.to_string());
            }
        }
        if results.is_empty() {
            None
        } else {
            Some(results)
        }
    }
}

impl std::fmt::Debug for LinderaAdapter {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("LinderaAdapter").finish()
    }
}

#[derive(Debug)]
struct EnglishTokenizer {}

impl EnglishTokenizer {
    fn new() -> Self {
        Self {}
    }

    fn tokenize(text: &str) -> Vec<String> {
        normalize_text(text)
            .split_word_bounds()
            .map(|token| token.trim_matches(|c: char| !c.is_ascii_alphanumeric()))
            .filter(|token| !token.is_empty())
            .map(normalize_english_token)
            .collect()
    }
}

#[derive(Debug)]
struct FallbackTokenizer {
    split_re: Regex,
}

impl FallbackTokenizer {
    fn new() -> Self {
        Self {
            split_re: Regex::new(r"[^\p{L}\p{N}]+").expect("compile fallback pattern"),
        }
    }

    fn tokenize(&self, text: &str) -> Vec<String> {
        normalize_text(text)
            .split(|c: char| c.is_whitespace())
            .flat_map(|piece| self.split_re.split(piece))
            .filter(|token| !token.is_empty())
            .map(str::to_lowercase)
            .collect()
    }
}

fn normalize_english_token(token: &str) -> String {
    let lower = token.to_lowercase();
    if lower.ends_with("ies") && lower.len() > 3 {
        let stem = lower.trim_end_matches("ies");
        return format!("{stem}y");
    }
    if lower.ends_with("ing") && lower.len() > 4 {
        return lower.trim_end_matches("ing").to_string();
    }
    if lower.ends_with('s') && lower.len() > 3 {
        return lower.trim_end_matches('s').to_string();
    }
    lower
}
