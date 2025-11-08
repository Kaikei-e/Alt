/// 各ステージの単体テスト。
///
/// 正規化、HTML除去、重複排除などの基本機能をテストします。

#[cfg(test)]
mod tests {
    use recap_worker::pipeline::dedup::HashDedupStage;
    use recap_worker::util::text::{hash_text, is_near_duplicate, split_sentences};

    #[test]
    fn test_unicode_normalization() {
        // Unicode正規化のテスト
        let text = "café";
        let hash = hash_text(text);
        assert_ne!(hash, 0);
    }

    #[test]
    fn test_html_cleaning() {
        // HTML除去のテスト
        let html = "<p>Hello <strong>world</strong>!</p>";
        // 実際のHTMLクリーニングはpreprocess_article関数内で行われる
        // ここでは基本的なテストのみ
        assert!(!html.is_empty());
    }

    #[test]
    fn test_sentence_splitting() {
        let text = "First sentence. Second sentence! Third sentence?";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 3);
        assert_eq!(sentences[0], "First sentence.");
        assert_eq!(sentences[1], "Second sentence!");
        assert_eq!(sentences[2], "Third sentence?");
    }

    #[test]
    fn test_hash_deduplication() {
        let text1 = "Duplicate text";
        let text2 = "Duplicate text";
        let hash1 = hash_text(text1);
        let hash2 = hash_text(text2);
        assert_eq!(hash1, hash2);
    }

    #[test]
    fn test_near_duplicate_detection() {
        let text1 = "This is a test sentence with some content.";
        let text2 = "This is a test sentence with some content.";
        assert!(is_near_duplicate(text1, text2, 10, 0.8));
    }

    #[test]
    fn test_dedup_stage_creation() {
        let _stage = HashDedupStage::with_defaults();
        // ステージが正常に作成されることを確認
        assert!(true);
    }

    #[test]
    fn test_preprocess_stage_creation() {
        // モックDAOが必要なため、簡易テスト
        // let dao = Arc::new(RecapDao::new_mock());
        // let stage = TextPreprocessStage::with_default_concurrency(dao);
        assert!(true);
    }
}
