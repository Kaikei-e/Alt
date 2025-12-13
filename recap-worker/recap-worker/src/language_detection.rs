//! `lingua`ベースの言語判定ラッパ。
//!
//! English/Japaneseのみに限定して高速に言語を検出し、
//! 信頼度が低い場合は`Unknown`を返すことで誤ルーティングを防ぐ。
use lingua::{Language, LanguageDetector, LanguageDetectorBuilder};
use once_cell::sync::Lazy;

use crate::classification::ClassificationLanguage;

/// 言語検出器（English/Japaneseのみに限定）
static DETECTOR: Lazy<LanguageDetector> = Lazy::new(|| {
    LanguageDetectorBuilder::from_languages(&[Language::English, Language::Japanese])
        .with_minimum_relative_distance(0.01)
        .build()
});

/// 最小文字数（これより短い場合は`Unknown`を返す）
/// 環境変数`RECAP_LANG_DETECT_MIN_CHARS`で設定可能（デフォルト: 50）
fn min_chars() -> usize {
    std::env::var("RECAP_LANG_DETECT_MIN_CHARS")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(50)
}

/// 最小信頼度（これより低い場合は`Unknown`を返す）
/// 環境変数`RECAP_LANG_DETECT_MIN_CONFIDENCE`で設定可能（デフォルト: 0.65）
fn min_confidence() -> f64 {
    std::env::var("RECAP_LANG_DETECT_MIN_CONFIDENCE")
        .ok()
        .and_then(|s| s.parse().ok())
        .unwrap_or(0.65)
}

/// テキストから言語を検出する。
///
/// # Arguments
/// * `text` - 検出対象のテキスト
///
/// # Returns
/// `(ClassificationLanguage, confidence)` のタプル。
/// 文字数が少ない、または信頼度が低い場合は`(Unknown, 0.0)`を返す。
#[must_use]
pub fn detect_lang(text: &str) -> (ClassificationLanguage, f64) {
    // 文字数チェック
    if text.chars().count() < min_chars() {
        return (ClassificationLanguage::Unknown, 0.0);
    }

    // 言語検出
    // DETECTORはEnglish/Japaneseのみに限定されているため、この2つのケースのみを処理
    let detected = DETECTOR.detect_language_of(text);

    let (lang, confidence) = match detected {
        Some(Language::Japanese) => {
            let conf = DETECTOR
                .compute_language_confidence_values(text)
                .iter()
                .find(|(l, _)| *l == Language::Japanese)
                .map_or(0.0, |(_, conf)| *conf);
            (Language::Japanese, conf)
        }
        Some(Language::English) => {
            let conf = DETECTOR
                .compute_language_confidence_values(text)
                .iter()
                .find(|(l, _)| *l == Language::English)
                .map_or(0.0, |(_, conf)| *conf);
            (Language::English, conf)
        }
        _ => return (ClassificationLanguage::Unknown, 0.0),
    };

    // 信頼度チェック
    if confidence < min_confidence() {
        return (ClassificationLanguage::Unknown, confidence);
    }

    // 言語マッピング（上記のmatchでJapanese/Englishのみが来ることが保証されている）
    let classification_lang = match lang {
        Language::Japanese => ClassificationLanguage::Japanese,
        Language::English => ClassificationLanguage::English,
        // 上記のmatchで保証されているため到達不能
        #[allow(unreachable_patterns)]
        _ => unreachable!("Only Japanese or English can reach here"),
    };

    (classification_lang, confidence)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_detect_japanese() {
        let text = "これは日本語のテキストです。機械学習と自然言語処理について説明します。";
        let (lang, confidence) = detect_lang(text);
        assert_eq!(lang, ClassificationLanguage::Japanese);
        assert!(confidence >= min_confidence());
    }

    #[test]
    fn test_detect_english() {
        let text =
            "This is an English text about machine learning and natural language processing.";
        let (lang, confidence) = detect_lang(text);
        assert_eq!(lang, ClassificationLanguage::English);
        assert!(confidence >= min_confidence());
    }

    #[test]
    fn test_short_text_returns_unknown() {
        let text = "短い";
        let (lang, _) = detect_lang(text);
        assert_eq!(lang, ClassificationLanguage::Unknown);
    }

    #[test]
    fn test_mixed_text() {
        let text = "This is a mixed text with both English and 日本語 characters.";
        let (lang, confidence) = detect_lang(text);
        // 混在テキストの場合、どちらかに判定されるかUnknownになる
        assert!(matches!(
            lang,
            ClassificationLanguage::Japanese
                | ClassificationLanguage::English
                | ClassificationLanguage::Unknown
        ));
        if lang != ClassificationLanguage::Unknown {
            assert!(confidence >= min_confidence());
        }
    }
}
