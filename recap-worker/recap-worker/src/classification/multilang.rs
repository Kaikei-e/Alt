//! 日英別モデルを同時ロードしてルーティングする分類器。
use std::path::Path;

use anyhow::Result;

use super::{ClassificationLanguage, ClassificationResult, GenreClassifier, TokenPipeline};

/// 日英別のジャンル分類器。
///
/// 言語判定に基づいて適切な分類器（JA/EN）を選択し、
/// 判定できない場合は日本語分類器にフォールバックする。
#[derive(Debug)]
pub struct MultiLangGenreClassifier {
    ja: GenreClassifier,
    en: GenreClassifier,
}

impl MultiLangGenreClassifier {
    /// 日英別の分類器を初期化する。
    ///
    /// # Arguments
    /// * `weights_ja` - 日本語モデルの重みファイルパス（`None`の場合はデフォルト）
    /// * `weights_en` - 英語モデルの重みファイルパス（`None`の場合は`weights_ja`を使用、それも`None`ならデフォルト）
    /// * `threshold` - スコア閾値
    ///
    /// # Errors
    /// 重みファイルの読み込みに失敗した場合にエラーを返す。
    pub fn new(
        weights_ja: Option<&Path>,
        weights_en: Option<&Path>,
        threshold: f32,
    ) -> Result<Self> {
        let ja = GenreClassifier::new_from_path(weights_ja, threshold)?;

        // ENが未指定の場合はJAを流用（フォールバック）
        let en = if let Some(en_path) = weights_en {
            GenreClassifier::new_from_path(Some(en_path), threshold)?
        } else {
            // ENが未提供ならJAをコピー（同じインスタンスは作れないので、同じパスから再読み込み）
            GenreClassifier::new_from_path(weights_ja, threshold)?
        };

        Ok(Self { ja, en })
    }

    /// テキストを分類し、上位ジャンルを返す。
    ///
    /// # Arguments
    /// * `title` - 記事のタイトル
    /// * `body` - 記事の本文
    /// * `provided_language` - 事前に指定された言語（`Unknown`の場合は自動検出）
    ///
    /// # Returns
    /// 分類結果。言語判定に失敗した場合は日本語分類器を使用する。
    ///
    /// # Errors
    /// 分類処理に失敗した場合にエラーを返す。
    pub fn predict(
        &self,
        title: &str,
        body: &str,
        provided_language: ClassificationLanguage,
    ) -> Result<ClassificationResult> {
        let combined = format!("{title} {body}");

        // 言語を解決（providedがUnknownの場合は自動検出）
        let resolved = TokenPipeline::resolve_language(provided_language, &combined);

        // 言語に応じて適切な分類器を選択
        match resolved {
            ClassificationLanguage::Japanese => {
                // 明示的にJapaneseを渡すことで、内部の再判定を回避
                self.ja
                    .predict(title, body, ClassificationLanguage::Japanese)
            }
            ClassificationLanguage::English => {
                // 明示的にEnglishを渡すことで、内部の再判定を回避
                self.en
                    .predict(title, body, ClassificationLanguage::English)
            }
            ClassificationLanguage::Unknown => {
                // 判定できない場合は日本語分類器にフォールバック
                self.ja
                    .predict(title, body, ClassificationLanguage::Japanese)
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_multilang_classifier_initialization() {
        let classifier = MultiLangGenreClassifier::new(None, None, 0.75);
        assert!(classifier.is_ok());
    }

    #[test]
    fn test_multilang_classifier_japanese_text() {
        let classifier = MultiLangGenreClassifier::new(None, None, 0.75).unwrap();
        let title = "機械学習の最新動向";
        let body = "深層学習と自然言語処理の進展について説明します。";
        let result = classifier.predict(title, body, ClassificationLanguage::Unknown);
        assert!(result.is_ok());
    }

    #[test]
    fn test_multilang_classifier_english_text() {
        let classifier = MultiLangGenreClassifier::new(None, None, 0.75).unwrap();
        let title = "Latest Trends in Machine Learning";
        let body =
            "This article discusses advances in deep learning and natural language processing.";
        let result = classifier.predict(title, body, ClassificationLanguage::Unknown);
        assert!(result.is_ok());
    }

    #[test]
    fn test_multilang_classifier_explicit_language() {
        let classifier = MultiLangGenreClassifier::new(None, None, 0.75).unwrap();
        let title = "Test";
        let body = "This is a test.";

        // 明示的にJapaneseを指定しても、分類器は動作する
        let result_ja = classifier.predict(title, body, ClassificationLanguage::Japanese);
        assert!(result_ja.is_ok());

        // 明示的にEnglishを指定
        let result_en = classifier.predict(title, body, ClassificationLanguage::English);
        assert!(result_en.is_ok());
    }
}
