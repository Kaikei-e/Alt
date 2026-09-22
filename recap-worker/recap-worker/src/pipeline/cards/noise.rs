//! Noise filtering rules for topic card candidates.

use regex::Regex;
use std::sync::LazyLock;

use super::normalize::NormalizedItem;

/// Title noise pattern (case-insensitive):
/// crossword|sudoku|quiz|sponsored|buy .*accounts
static NOISE_TITLE_REGEX: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"(?i)(crossword|sudoku|quiz|sponsored|buy .*accounts)")
        .expect("valid noise title regex")
});

/// Named noise filtering rules.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum NoiseRule {
    TitleRegex,
    Language,
    ShortLede,
    NoDate,
    HtmlStripFailed,
}

impl NoiseRule {
    pub const fn name(&self) -> &'static str {
        match self {
            Self::TitleRegex => "title_regex",
            Self::Language => "language",
            Self::ShortLede => "short_lede",
            Self::NoDate => "no_date",
            Self::HtmlStripFailed => "html_strip_failed",
        }
    }
}

#[derive(Debug, Default, Clone, PartialEq, Eq)]
pub struct NoiseStats {
    pub dropped_title_regex: usize,
    pub dropped_language: usize,
    pub dropped_short_lede: usize,
    pub dropped_no_date: usize,
    pub dropped_html_strip_failed: usize,
    pub total_dropped: usize,
}

impl NoiseStats {
    pub fn record_dropped(&mut self, rule: NoiseRule) {
        self.total_dropped += 1;
        match rule {
            NoiseRule::TitleRegex => self.dropped_title_regex += 1,
            NoiseRule::Language => self.dropped_language += 1,
            NoiseRule::ShortLede => self.dropped_short_lede += 1,
            NoiseRule::NoDate => self.dropped_no_date += 1,
            NoiseRule::HtmlStripFailed => self.dropped_html_strip_failed += 1,
        }
    }
}

/// Check if an item violates any noise rule (table-driven evaluation).
pub fn check_noise(item: &NormalizedItem) -> Option<NoiseRule> {
    // Rule 1: Title regex pattern
    if NOISE_TITLE_REGEX.is_match(&item.title) {
        return Some(NoiseRule::TitleRegex);
    }

    // Rule 2: Language must be ja or en
    if item.language != "ja" && item.language != "en" {
        return Some(NoiseRule::Language);
    }

    // Rule 3: Stripped lede must be at least 20 Unicode scalar values
    if item.lede.chars().count() < 20 {
        return Some(NoiseRule::ShortLede);
    }

    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::{DateTime, Utc};
    use uuid::Uuid;

    fn make_item(title: &str, lede: &str, language: &str) -> NormalizedItem {
        NormalizedItem {
            feed_id: Uuid::new_v4(),
            title: title.to_string(),
            lede: lede.to_string(),
            host: "example.com".to_string(),
            url: "https://example.com/item".to_string(),
            pub_date: DateTime::parse_from_rfc3339("2026-03-20T10:00:00Z")
                .unwrap()
                .with_timezone(&Utc),
            language: language.to_string(),
            genre: None,
        }
    }

    #[test]
    fn test_noise_rule_title_regex() {
        let titles_to_drop = [
            "Daily Crossword #42",
            "Sudoku of the day",
            "Weekly tech Quiz",
            "Sponsored post: Our favorite monitors",
            "Buy old Reddit accounts cheap",
            "Buy verified accounts now",
        ];

        let valid_lede =
            "This is a sufficiently long lede that contains more than twenty characters.";

        for title in titles_to_drop {
            let item = make_item(title, valid_lede, "en");
            assert_eq!(
                check_noise(&item),
                Some(NoiseRule::TitleRegex),
                "Title '{title}' should be dropped by TitleRegex rule"
            );
        }

        let titles_to_keep = [
            "Today's Wordle hints",
            "Advertising notice in modern media",
            "Weekly Roundup of news",
            "The Monday Newsletter",
            "Weekly Digest",
            "Tech Digest #12",
            "Best Telegram accounts for crypto",
        ];

        for title in titles_to_keep {
            let item = make_item(title, valid_lede, "en");
            assert_eq!(
                check_noise(&item),
                None,
                "Title '{title}' should NOT be dropped by TitleRegex rule"
            );
        }
    }

    #[test]
    fn test_noise_rule_language() {
        let valid_lede =
            "This is a sufficiently long lede that contains more than twenty characters.";
        let item_fr = make_item("Example French Headline", valid_lede, "fr");
        assert_eq!(check_noise(&item_fr), Some(NoiseRule::Language));

        let item_other = make_item("Example Unknown Headline", valid_lede, "other");
        assert_eq!(check_noise(&item_other), Some(NoiseRule::Language));

        let item_ja = make_item(
            "例の見出し 1",
            "これは十分に長いリード文で、20文字を超えています。",
            "ja",
        );
        assert_eq!(check_noise(&item_ja), None);

        let item_en = make_item("Example English Headline 1", valid_lede, "en");
        assert_eq!(check_noise(&item_en), None);
    }

    #[test]
    fn test_noise_rule_short_lede() {
        let item_short = make_item(
            "Example headline 1",
            "Too short lede.", // 15 characters
            "en",
        );
        assert_eq!(check_noise(&item_short), Some(NoiseRule::ShortLede));

        let item_just_enough = make_item(
            "Example headline 1",
            "12345678901234567890", // exactly 20 characters
            "en",
        );
        assert_eq!(check_noise(&item_just_enough), None);
    }

    #[test]
    fn test_noise_rule_valid_passes() {
        let item = make_item(
            "Example headline 1",
            "This is a valid long lede text that contains more than twenty characters.",
            "en",
        );
        assert_eq!(check_noise(&item), None);
    }
}
