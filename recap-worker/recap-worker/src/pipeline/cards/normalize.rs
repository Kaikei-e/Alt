//! Feed item normalization stage for CardsPipeline.

use anyhow::Result;
use chrono::{DateTime, Utc};
use regex::Regex;
use reqwest::Url;
use std::sync::LazyLock;
use uuid::Uuid;

use super::noise::NoiseRule;
use crate::clients::alt_backend::AltBackendFeed;
use crate::language_detection::detect_text_language;
use crate::pipeline::preprocess::clean_html;

/// Normalized feed item ready for noise filtering, deduplication, and embedding.
#[derive(Debug, Clone, PartialEq)]
pub struct NormalizedItem {
    pub feed_id: Uuid,
    pub title: String,
    pub lede: String,
    pub host: String,
    pub url: String,
    pub pub_date: DateTime<Utc>,
    pub language: String, // "ja" | "en" | "other"
    pub genre: Option<String>,
}

/// Derive normalized host from website URL (lowercase, strip `www.`).
pub fn extract_host(url_str: &str) -> String {
    let parsed = Url::parse(url_str).or_else(|_| Url::parse(&format!("https://{url_str}")));
    let raw_host = match parsed {
        Ok(url) => url.host_str().unwrap_or("").to_lowercase(),
        Err(_) => url_str.to_lowercase(),
    };
    raw_host
        .strip_prefix("www.")
        .unwrap_or(&raw_host)
        .to_string()
}

static LINK_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"\[([^\]]+)\](?:\([^\)]*\)|\[[^\]]*\])").expect("valid link regex")
});

static FOOTNOTE_MARKER_RE: LazyLock<Regex> = LazyLock::new(|| {
    Regex::new(r"\s*\[\s*(?:\d+|n)\s*\](?P<punct>[\.,;:!?。、！？])|\[\s*(?:\d+|n)\s*\]")
        .expect("valid footnote regex")
});

fn strip_line_prefixes(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    for line in text.lines() {
        let trimmed = line.trim_start();
        // Check for link reference definition: e.g. [1]: https://...
        if trimmed.starts_with('[') && trimmed.contains("]:") {
            continue;
        }
        let mut rest = trimmed;

        // Blockquotes: e.g. "> "
        while let Some(after) = rest.strip_prefix('>') {
            rest = after;
        }
        rest = rest.trim_start();

        // Headings: e.g. "# ", "## "
        if rest.starts_with('#') {
            let without_hashes = rest.trim_start_matches('#');
            if without_hashes.starts_with(' ') || without_hashes.is_empty() {
                rest = without_hashes.trim_start();
            }
        }

        // List bullets: e.g. "* ", "- ", "+ ", "• "
        if let Some(after) = rest
            .strip_prefix("- ")
            .or_else(|| rest.strip_prefix("* "))
            .or_else(|| rest.strip_prefix("+ "))
            .or_else(|| rest.strip_prefix("• "))
        {
            rest = after.trim_start();
        } else if let Some((num, after)) = rest.split_once(". ") {
            if num.chars().all(|c| c.is_ascii_digit()) {
                rest = after.trim_start();
            }
        } else if let Some((num, after)) = rest.split_once(") ") {
            if num.chars().all(|c| c.is_ascii_digit()) {
                rest = after.trim_start();
            }
        }
        out.push_str(rest);
        out.push('\n');
    }
    out
}

fn strip_emphasis(s: &str) -> String {
    let s = s.replace("**", "").replace("__", "");
    let chars: Vec<char> = s.chars().collect();
    let n = chars.len();
    let mut remove_indices = std::collections::HashSet::new();

    for delim in ['*', '_'] {
        let mut i = 0;
        while i < n {
            if chars[i] == delim {
                let is_open = i + 1 < n
                    && !chars[i + 1].is_whitespace()
                    && chars[i + 1] != delim
                    && (delim != '_' || i == 0 || !chars[i - 1].is_alphanumeric());

                if is_open {
                    let mut j = i + 1;
                    while j < n {
                        if chars[j] == delim {
                            let is_close = !chars[j - 1].is_whitespace()
                                && (delim != '_' || j + 1 == n || !chars[j + 1].is_alphanumeric());
                            if is_close {
                                remove_indices.insert(i);
                                remove_indices.insert(j);
                                i = j;
                                break;
                            }
                        }
                        j += 1;
                    }
                }
            }
            i += 1;
        }
    }

    chars
        .into_iter()
        .enumerate()
        .filter_map(|(idx, c)| {
            if remove_indices.contains(&idx) {
                None
            } else {
                Some(c)
            }
        })
        .collect()
}

/// Check if a character is a sentence boundary.
fn is_sentence_boundary(c: char) -> bool {
    matches!(c, '。' | '．' | '.' | '!' | '?' | '！' | '？')
}

/// Extract lede from HTML description with a custom scalar length limit.
pub fn extract_lede_with_limit(raw_description: &str, limit: usize) -> Option<String> {
    let (plain, _) = clean_html(raw_description).ok()?;

    // Post-process markdown from html2text:
    // 1. Drop leading #/> block markers, list bullets at line starts, and reference links
    let unblocked = strip_line_prefixes(&plain);
    // 2. Collapse [text](url) and [text][n] to text
    let unlinked = LINK_RE.replace_all(&unblocked, "$1");
    // 3. Turn bare [n]-style footnote markers into nothing
    let unfootnoted = FOOTNOTE_MARKER_RE.replace_all(&unlinked, "${punct}");
    // 4. Remove **, __ and single */_ emphasis markers that wrap words
    let unemphasized = strip_emphasis(&unfootnoted);
    // 5. Remove inline code backticks
    let unbackticked = unemphasized.replace('`', "");
    // 6. Collapse whitespace
    let collapsed = unbackticked
        .split_whitespace()
        .collect::<Vec<_>>()
        .join(" ");

    let chars: Vec<char> = collapsed.chars().collect();
    if chars.len() <= limit {
        return Some(collapsed.trim().to_string());
    }

    // Look for sentence boundary within first `limit` Unicode scalars
    let prefix = &chars[..limit];
    if let Some(last_boundary_idx) = prefix.iter().rposition(|&c| is_sentence_boundary(c)) {
        Some(
            chars[..=last_boundary_idx]
                .iter()
                .collect::<String>()
                .trim()
                .to_string(),
        )
    } else {
        Some(prefix.iter().collect::<String>().trim().to_string())
    }
}

/// Extract lede from HTML description (up to 600 Unicode scalars):
/// - Strips HTML using ammonia + html2text helpers.
/// - If clean_html fails, returns None (caller drops with HtmlStripFailed, never raw markup).
/// - Post-processes markdown: strips emphasis, links, block markers, bullets.
/// - Collapses whitespace.
/// - First ≤600 Unicode scalars cut at a sentence boundary (。．.!?！？).
///   If no boundary before 600, hard-cut at 600.
pub fn extract_lede(raw_description: &str) -> Option<String> {
    extract_lede_with_limit(raw_description, 600)
}

/// Normalize an AltBackendFeed into NormalizedItem.
///
/// Returns:
/// - `Err(...)`: unparseable feed id (contract violation, fail loud).
/// - `Ok(Err(NoiseRule::HtmlStripFailed))`: clean_html failed on description (warn logged).
/// - `Ok(Err(NoiseRule::NoDate))`: feed has neither pub_date nor created_at (no Utc::now() fabrication).
/// - `Ok(Ok(NormalizedItem))`: successfully normalized item.
pub fn normalize_feed(feed: &AltBackendFeed) -> Result<Result<NormalizedItem, NoiseRule>> {
    let feed_id = Uuid::parse_str(&feed.id).map_err(|e| {
        anyhow::anyhow!("contract violation: unparseable feed id '{}': {e}", feed.id)
    })?;

    let raw_desc = feed.description.as_deref().unwrap_or("");
    let Some(lede) = extract_lede(raw_desc) else {
        tracing::warn!(
            feed_id = %feed.id,
            "clean_html failed on feed description; dropping with HtmlStripFailed"
        );
        return Ok(Err(NoiseRule::HtmlStripFailed));
    };

    let Some(pub_date) = feed.pub_date.or(feed.created_at) else {
        return Ok(Err(NoiseRule::NoDate));
    };

    let title = feed.title.trim().to_string();
    let host = extract_host(&feed.website_url);
    let url = feed.website_url.trim().to_string();

    let text_for_lang = format!("{title} {lede}");
    let language = detect_text_language(&text_for_lang).to_string();

    Ok(Ok(NormalizedItem {
        feed_id,
        title,
        lede,
        host,
        url,
        pub_date,
        language,
        genre: None,
    }))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_extract_host_normalizes_properly() {
        assert_eq!(
            extract_host("https://www.example.com/post/1"),
            "example.com"
        );
        assert_eq!(extract_host("http://EXAMPLE.ORG:8080/path"), "example.org");
        assert_eq!(
            extract_host("https://sub.domain.co.jp/feed"),
            "sub.domain.co.jp"
        );
        assert_eq!(
            extract_host("www.example-news.com/article"),
            "example-news.com"
        );
    }

    #[test]
    fn test_extract_lede_strips_html_and_collapses_whitespace() {
        let html = "<p>Hello <b>World</b>!</p>\n\n  <div>This is a <i>test</i>.</div>";
        let lede = extract_lede(html).expect("valid html");
        assert_eq!(lede, "Hello World! This is a test.");
    }

    #[test]
    fn test_extract_lede_boundary_under_600() {
        let text = "First sentence. Second sentence. Third sentence.";
        let lede = extract_lede(text).expect("valid text");
        assert_eq!(lede, text);
    }

    #[test]
    fn test_extract_lede_cuts_at_sentence_boundary_before_600() {
        let sentence1 = "A".repeat(300) + ". ";
        let sentence2 = "B".repeat(250) + "。";
        let sentence3 = "C".repeat(200) + "!";
        let long_text = format!("{sentence1}{sentence2}{sentence3}");

        // sentence1 is 302 chars, sentence2 is 251 chars -> total 553 chars.
        // Within 600 chars, the last sentence boundary is at the end of sentence2 (char index 552).
        let lede = extract_lede(&long_text).expect("valid text");
        assert_eq!(lede.chars().count(), 553);
        assert!(lede.ends_with('。'));
    }

    #[test]
    fn test_extract_lede_hard_cuts_at_600_if_no_boundary() {
        let long_no_boundary = "A".repeat(800);
        let lede = extract_lede(&long_no_boundary).expect("valid text");
        assert_eq!(lede.chars().count(), 600);
    }

    #[test]
    fn test_normalize_feed_unparseable_uuid_fails_loud() {
        let feed = AltBackendFeed {
            id: "not-a-uuid".to_string(),
            title: "Example Title".to_string(),
            description: Some("<p>Example description</p>".to_string()),
            website_url: "https://example.com/item".to_string(),
            pub_date: Some(Utc::now()),
            created_at: None,
            updated_at: None,
            article_id: None,
            is_read: false,
            feed_link_id: None,
            og_image_url: None,
        };
        let res = normalize_feed(&feed);
        assert!(res.is_err());
        assert!(res.unwrap_err().to_string().contains("contract violation"));
    }

    #[test]
    fn test_normalize_feed_missing_both_dates_returns_no_date_rule() {
        let feed = AltBackendFeed {
            id: Uuid::new_v4().to_string(),
            title: "Example Title".to_string(),
            description: Some("<p>Example description</p>".to_string()),
            website_url: "https://example.com/item".to_string(),
            pub_date: None,
            created_at: None,
            updated_at: None,
            article_id: None,
            is_read: false,
            feed_link_id: None,
            og_image_url: None,
        };
        let res = normalize_feed(&feed).expect("valid uuid");
        assert_eq!(res, Err(NoiseRule::NoDate));
    }

    #[test]
    fn test_extract_lede_strips_links_bullets_and_block_markers() {
        let raw = "# Main Heading\n> Blockquote text\n* List item with [example link](https://example.com)\n- Second [numbered link][1]\n\n[1]: https://example.com/1";
        let lede = extract_lede(raw).expect("valid text");
        assert_eq!(
            lede,
            "Main Heading Blockquote text List item with example link Second numbered link"
        );
    }

    #[test]
    fn test_extract_lede_strips_markdown_link() {
        let input = "Check out [our website](https://example.com/info) for more details.";
        let lede = extract_lede(input).expect("valid text");
        assert_eq!(lede, "Check out our website for more details.");
    }

    #[test]
    fn test_extract_lede_strips_heading_hashes() {
        let input = "## Important Announcement\nThis is the content of the announcement.";
        let lede = extract_lede(input).expect("valid text");
        assert_eq!(
            lede,
            "Important Announcement This is the content of the announcement."
        );
    }

    #[test]
    fn test_extract_lede_strips_inline_code_backticks_and_footnote_markers() {
        let input = "Run `cargo test` to verify according to sources [1]. Another update [2].";
        let lede = extract_lede(input).expect("valid text");
        assert_eq!(
            lede,
            "Run cargo test to verify according to sources. Another update."
        );
    }
}
