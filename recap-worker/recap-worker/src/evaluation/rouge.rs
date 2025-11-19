//! ROUGEスコア計算ユーティリティ。

use std::collections::HashMap;

use unicode_segmentation::UnicodeSegmentation;

/// ROUGE計算結果。
#[derive(Debug, Clone, Copy, Default, PartialEq)]
pub struct RougeScores {
    pub rouge1_precision: f32,
    pub rouge1_recall: f32,
    pub rouge1_f: f32,
    pub rouge_l_precision: f32,
    pub rouge_l_recall: f32,
    pub rouge_l_f: f32,
}

/// 文字列同士のROUGEスコアを計算する。
#[must_use]
pub fn compute_rouge(candidate: &str, reference: &str) -> RougeScores {
    if candidate.trim().is_empty() || reference.trim().is_empty() {
        return RougeScores::default();
    }
    let cand_tokens = tokenize(candidate);
    let ref_tokens = tokenize(reference);

    let RougePair {
        precision: rouge1_precision,
        recall: rouge1_recall,
        f_score: rouge1_f,
    } = rouge_1(&cand_tokens, &ref_tokens);

    let RougePair {
        precision: rouge_l_precision,
        recall: rouge_l_recall,
        f_score: rouge_l_f,
    } = rouge_l(&cand_tokens, &ref_tokens);

    RougeScores {
        rouge1_precision,
        rouge1_recall,
        rouge1_f,
        rouge_l_precision,
        rouge_l_recall,
        rouge_l_f,
    }
}

#[derive(Debug, Clone, Copy)]
struct RougePair {
    precision: f32,
    recall: f32,
    f_score: f32,
}

fn rouge_1(candidate: &[String], reference: &[String]) -> RougePair {
    if candidate.is_empty() || reference.is_empty() {
        return RougePair {
            precision: 0.0,
            recall: 0.0,
            f_score: 0.0,
        };
    }

    let mut ref_counts: HashMap<&str, usize> = HashMap::new();
    for token in reference {
        *ref_counts.entry(token.as_str()).or_default() += 1;
    }

    let mut match_count = 0usize;
    let mut cand_counts: HashMap<&str, usize> = HashMap::new();
    for token in candidate {
        let cand_entry = cand_counts.entry(token.as_str()).or_default();
        if *cand_entry < *ref_counts.get(token.as_str()).unwrap_or(&0) {
            match_count += 1;
        }
        *cand_entry += 1;
    }

    let precision = match_count as f32 / candidate.len() as f32;
    let recall = match_count as f32 / reference.len() as f32;
    let f_score = harmonic_mean(precision, recall);

    RougePair {
        precision,
        recall,
        f_score,
    }
}

fn rouge_l(candidate: &[String], reference: &[String]) -> RougePair {
    if candidate.is_empty() || reference.is_empty() {
        return RougePair {
            precision: 0.0,
            recall: 0.0,
            f_score: 0.0,
        };
    }

    let lcs = longest_common_subsequence(candidate, reference) as f32;
    let precision = lcs / candidate.len() as f32;
    let recall = lcs / reference.len() as f32;
    let f_score = harmonic_mean(precision, recall);

    RougePair {
        precision,
        recall,
        f_score,
    }
}

fn tokenize(text: &str) -> Vec<String> {
    // 1. まずは単語ベースで分割（ASCII圏）
    let mut tokens: Vec<String> = Vec::new();
    let mut buffer = String::new();

    let push_buffer = |tokens: &mut Vec<String>, buffer: &mut String| {
        if !buffer.is_empty() {
            tokens.push(buffer.to_lowercase());
            buffer.clear();
        }
    };

    for ch in text.chars() {
        if ch.is_ascii_alphanumeric() {
            buffer.push(ch);
        } else if ch.is_whitespace() || ch.is_ascii_punctuation() {
            push_buffer(&mut tokens, &mut buffer);
        } else {
            push_buffer(&mut tokens, &mut buffer);
            if !ch.is_control() {
                tokens.push(ch.to_string());
            }
        }
    }
    push_buffer(&mut tokens, &mut buffer);

    // 2. CJKなど連続する非ASCII文字は1文字ずつ保持（既に追加済み）
    // 3. 空要素を排除
    tokens
        .into_iter()
        .filter(|token| !token.trim().is_empty())
        .flat_map(|token| {
            if token.is_ascii() {
                vec![token]
            } else {
                UnicodeSegmentation::graphemes(token.as_str(), true)
                    .map(std::string::ToString::to_string)
                    .collect::<Vec<String>>()
            }
        })
        .collect()
}

fn harmonic_mean(a: f32, b: f32) -> f32 {
    if a == 0.0 && b == 0.0 {
        0.0
    } else {
        (2.0 * a * b) / (a + b)
    }
}

fn longest_common_subsequence(a: &[String], b: &[String]) -> usize {
    let m = a.len();
    let n = b.len();
    if m == 0 || n == 0 {
        return 0;
    }

    let mut dp = vec![vec![0usize; n + 1]; m + 1];
    for i in 0..m {
        for j in 0..n {
            if a[i] == b[j] {
                dp[i + 1][j + 1] = dp[i][j] + 1;
            } else {
                dp[i + 1][j + 1] = dp[i + 1][j].max(dp[i][j + 1]);
            }
        }
    }
    dp[m][n]
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn rouge_handles_empty_input() {
        let scores = compute_rouge("", "reference");
        assert!((scores.rouge1_f - 0.0).abs() < f32::EPSILON);
        assert!((scores.rouge_l_f - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn rouge_scores_identical_strings() {
        let text = "AIが世界のビジネスを変える";
        let scores = compute_rouge(text, text);
        assert!((scores.rouge1_f - 1.0).abs() < f32::EPSILON);
        assert!((scores.rouge_l_f - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn rouge_scores_partial_overlap() {
        let candidate = "AIが世界を変える";
        let reference = "生成AIがビジネスを変える";
        let scores = compute_rouge(candidate, reference);
        assert!(scores.rouge1_f > 0.0);
        assert!(scores.rouge_l_f > 0.0);
        assert!(scores.rouge1_f <= 1.0);
        assert!(scores.rouge_l_f <= 1.0);
    }
}
