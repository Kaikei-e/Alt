/// テキスト処理ユーティリティ。
///
/// 文分割、ハッシング、近似重複検出などを提供します。
use unicode_segmentation::UnicodeSegmentation;
use xxhash_rust::xxh3::xxh3_64;

/// テキストをXXH3でハッシュする。
///
/// XXH3は高速で衝突率が低いハッシュアルゴリズムです。
#[must_use]
pub fn hash_text(text: &str) -> u64 {
    xxh3_64(text.as_bytes())
}

/// テキストを文に分割する。
///
/// Unicode UAX#29に準拠した文境界検出を使用します。
#[must_use]
pub fn split_sentences(text: &str) -> Vec<String> {
    text.unicode_sentences()
        .map(|s| s.trim().to_string())
        .filter(|s| !s.is_empty())
        .collect()
}

/// ローリングウィンドウでテキストのN文字窓ハッシュを生成する。
///
/// 近似重複検出に使用します。
#[must_use]
pub(crate) fn rolling_hash_windows(text: &str, window_size: usize) -> Vec<u64> {
    if text.len() < window_size {
        return vec![hash_text(text)];
    }

    let chars: Vec<char> = text.chars().collect();
    let mut hashes = Vec::new();

    for window in chars.windows(window_size) {
        let window_text: String = window.iter().collect();
        hashes.push(hash_text(&window_text));
    }

    hashes
}

/// 2つのテキストが近似重複かどうかを判定する。
///
/// # Arguments
/// * `text1` - 最初のテキスト
/// * `text2` - 2番目のテキスト
/// * `window_size` - ローリングウィンドウサイズ
/// * `threshold` - 重複と判定する類似度の閾値（0.0〜1.0）
///
/// # Returns
/// 近似重複の場合はtrue
#[must_use]
pub fn is_near_duplicate(text1: &str, text2: &str, window_size: usize, threshold: f64) -> bool {
    if text1.is_empty() || text2.is_empty() {
        return false;
    }

    let hashes1 = rolling_hash_windows(text1, window_size);
    let hashes2 = rolling_hash_windows(text2, window_size);

    if hashes1.is_empty() || hashes2.is_empty() {
        return false;
    }

    use std::collections::HashMap;

    let mut counts1: HashMap<u64, usize> = HashMap::new();
    for hash in hashes1 {
        *counts1.entry(hash).or_insert(0) += 1;
    }

    let mut counts2: HashMap<u64, usize> = HashMap::new();
    for hash in hashes2 {
        *counts2.entry(hash).or_insert(0) += 1;
    }

    let mut intersection = 0usize;
    for (hash, count1) in &counts1 {
        if let Some(count2) = counts2.get(hash) {
            intersection += (*count1).min(*count2);
        }
    }

    let total1: usize = counts1.values().sum();
    let total2: usize = counts2.values().sum();
    let denominator = total1 + total2;

    if denominator == 0 {
        return false;
    }

    let similarity = (2 * intersection) as f64 / denominator as f64;
    similarity >= threshold
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn hash_text_is_deterministic() {
        let text = "Hello, world!";
        let hash1 = hash_text(text);
        let hash2 = hash_text(text);
        assert_eq!(hash1, hash2);
    }

    #[test]
    fn hash_text_produces_different_hashes() {
        let text1 = "Hello, world!";
        let text2 = "Goodbye, world!";
        let hash1 = hash_text(text1);
        let hash2 = hash_text(text2);
        assert_ne!(hash1, hash2);
    }

    #[test]
    fn split_sentences_handles_simple_text() {
        let text = "First sentence. Second sentence! Third sentence?";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 3);
        assert_eq!(sentences[0], "First sentence.");
        assert_eq!(sentences[1], "Second sentence!");
        assert_eq!(sentences[2], "Third sentence?");
    }

    #[test]
    fn split_sentences_handles_japanese() {
        let text = "最初の文。２番目の文！３番目の文？";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 3);
    }

    #[test]
    fn split_sentences_filters_empty() {
        let text = "Sentence one.  \n\n  Sentence two.";
        let sentences = split_sentences(text);
        assert_eq!(sentences.len(), 2);
    }

    #[test]
    fn rolling_hash_windows_creates_hashes() {
        let text = "This is a longer text for testing.";
        let hashes = rolling_hash_windows(text, 10);
        assert!(!hashes.is_empty());
        assert!(hashes.len() <= text.chars().count());
    }

    #[test]
    fn rolling_hash_windows_handles_short_text() {
        let text = "Short";
        let hashes = rolling_hash_windows(text, 100);
        assert_eq!(hashes.len(), 1);
    }

    #[test]
    fn is_near_duplicate_detects_identical() {
        let text1 = "This is identical text.";
        let text2 = "This is identical text.";
        assert!(is_near_duplicate(text1, text2, 10, 0.8));
    }

    #[test]
    fn is_near_duplicate_detects_similar() {
        let text1 = "This is some text with minor differences.";
        let text2 = "This is some text with small differences.";
        assert!(is_near_duplicate(text1, text2, 10, 0.5));
    }

    #[test]
    fn is_near_duplicate_rejects_different() {
        let text1 = "Completely different text here.";
        let text2 = "Another unrelated piece of content.";
        assert!(!is_near_duplicate(text1, text2, 10, 0.8));
    }

    #[test]
    fn is_near_duplicate_handles_empty() {
        assert!(!is_near_duplicate("", "text", 10, 0.5));
        assert!(!is_near_duplicate("text", "", 10, 0.5));
        assert!(!is_near_duplicate("", "", 10, 0.5));
    }
}
