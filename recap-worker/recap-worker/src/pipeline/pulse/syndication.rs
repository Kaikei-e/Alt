//! Syndication removal for Evening Pulse.
//!
//! This module provides multi-stage syndication detection and removal:
//!
//! 1. **Stage 1: Canonical URL Matching** - Groups articles by canonical URL.
//! 2. **Stage 2: Wire Source Detection** - Identifies content from wire services
//!    (Reuters, AP, AFP, etc.).
//! 3. **Stage 3: Title Similarity** - Detects syndicated content via title matching
//!    (disabled by default).
//!
//! Each stage can be independently enabled/disabled via configuration.

use std::collections::{HashMap, HashSet};

use async_trait::async_trait;

use super::config::PulseSyndicationConfig;
use super::types::SyndicationStatus;

/// Known wire service domains.
const WIRE_SOURCES: &[&str] = &[
    "reuters.com",
    "apnews.com",
    "afp.com",
    "kyodonews.jp",
    "jiji.com",
    "prnewswire.com",
    "businesswire.com",
    "globenewswire.com",
];

/// Article with metadata for syndication detection.
#[derive(Debug, Clone)]
pub struct ArticleWithMetadata {
    /// Article identifier.
    pub id: String,
    /// Article title.
    pub title: String,
    /// Source URL.
    pub source_url: String,
    /// Canonical URL (if available).
    pub canonical_url: Option<String>,
    /// Open Graph URL (if available).
    pub og_url: Option<String>,
}

/// Result of syndication removal.
#[derive(Debug, Clone)]
pub struct SyndicationResult {
    /// Original articles after removing syndicated content.
    pub original_articles: Vec<ArticleWithMetadata>,
    /// Syndicated articles that were removed.
    pub removed_articles: Vec<RemovedArticle>,
    /// Number of articles removed by each stage.
    pub removal_counts: RemovalCounts,
}

impl SyndicationResult {
    /// Create a result with no changes (pass-through).
    #[must_use]
    pub fn no_change(articles: Vec<ArticleWithMetadata>) -> Self {
        Self {
            original_articles: articles,
            removed_articles: Vec::new(),
            removal_counts: RemovalCounts::default(),
        }
    }

    /// Check if any articles were removed.
    #[must_use]
    pub fn has_removals(&self) -> bool {
        !self.removed_articles.is_empty()
    }

    /// Get total number of removed articles.
    #[must_use]
    pub fn total_removed(&self) -> usize {
        self.removed_articles.len()
    }
}

/// Removed article with reason.
#[derive(Debug, Clone)]
pub struct RemovedArticle {
    /// Original article.
    pub article: ArticleWithMetadata,
    /// Syndication status (reason for removal).
    pub status: SyndicationStatus,
    /// ID of the original article (for canonical matches).
    pub original_id: Option<String>,
}

/// Counts of articles removed by each stage.
#[derive(Debug, Clone, Default)]
pub struct RemovalCounts {
    /// Articles removed by canonical URL matching.
    pub canonical: usize,
    /// Articles removed by wire source detection.
    pub wire: usize,
    /// Articles removed by title similarity.
    pub title: usize,
}

impl RemovalCounts {
    /// Total articles removed.
    #[must_use]
    pub fn total(&self) -> usize {
        self.canonical + self.wire + self.title
    }
}

/// Trait for syndication removal.
#[async_trait]
pub trait SyndicationRemover: Send + Sync {
    /// Remove syndicated content from articles.
    async fn remove_syndication(
        &self,
        articles: Vec<ArticleWithMetadata>,
    ) -> anyhow::Result<SyndicationResult>;
}

/// Default implementation of syndication removal.
pub struct DefaultSyndicationRemover {
    config: PulseSyndicationConfig,
}

impl DefaultSyndicationRemover {
    /// Create a new remover with the given configuration.
    #[must_use]
    pub fn new(config: PulseSyndicationConfig) -> Self {
        Self { config }
    }
}

impl Default for DefaultSyndicationRemover {
    fn default() -> Self {
        Self::new(PulseSyndicationConfig::default())
    }
}

#[async_trait]
impl SyndicationRemover for DefaultSyndicationRemover {
    async fn remove_syndication(
        &self,
        articles: Vec<ArticleWithMetadata>,
    ) -> anyhow::Result<SyndicationResult> {
        let mut remaining = articles;
        let mut removed = Vec::new();
        let mut counts = RemovalCounts::default();

        // Stage 1: Canonical URL matching
        if self.config.is_canonical_enabled() {
            let (originals, canonical_removed) = remove_by_canonical(&remaining);
            counts.canonical = canonical_removed.len();
            removed.extend(canonical_removed);
            remaining = originals;
        }

        // Stage 2: Wire source detection
        if self.config.is_wire_enabled() {
            let (originals, wire_removed) = remove_wire_sources(&remaining);
            counts.wire = wire_removed.len();
            removed.extend(wire_removed);
            remaining = originals;
        }

        // Stage 3: Title similarity (disabled by default)
        if self.config.is_title_enabled() {
            let (originals, title_removed) =
                remove_by_title_similarity(&remaining, self.config.title_threshold);
            counts.title = title_removed.len();
            removed.extend(title_removed);
            remaining = originals;
        }

        Ok(SyndicationResult {
            original_articles: remaining,
            removed_articles: removed,
            removal_counts: counts,
        })
    }
}

/// Stage 1: Group articles by canonical URL and keep only the first in each group.
fn remove_by_canonical(
    articles: &[ArticleWithMetadata],
) -> (Vec<ArticleWithMetadata>, Vec<RemovedArticle>) {
    let mut groups: HashMap<String, Vec<&ArticleWithMetadata>> = HashMap::new();

    for article in articles {
        let key = extract_canonical_url(article).unwrap_or_else(|| article.source_url.clone());
        groups.entry(key).or_default().push(article);
    }

    let mut originals = Vec::new();
    let mut removed = Vec::new();

    for (_, group) in groups {
        if group.is_empty() {
            continue;
        }

        // Keep the first article as the original
        let original = group[0].clone();
        originals.push(original.clone());

        // Mark the rest as syndicated
        for &article in group.iter().skip(1) {
            removed.push(RemovedArticle {
                article: article.clone(),
                status: SyndicationStatus::CanonicalMatch,
                original_id: Some(original.id.clone()),
            });
        }
    }

    (originals, removed)
}

/// Stage 2: Remove articles from known wire sources.
fn remove_wire_sources(
    articles: &[ArticleWithMetadata],
) -> (Vec<ArticleWithMetadata>, Vec<RemovedArticle>) {
    let mut originals = Vec::new();
    let mut removed = Vec::new();

    for article in articles {
        if is_wire_source(&article.source_url) {
            removed.push(RemovedArticle {
                article: article.clone(),
                status: SyndicationStatus::WireSource,
                original_id: None,
            });
        } else {
            originals.push(article.clone());
        }
    }

    (originals, removed)
}

/// Stage 3: Remove articles with similar titles.
fn remove_by_title_similarity(
    articles: &[ArticleWithMetadata],
    threshold: f32,
) -> (Vec<ArticleWithMetadata>, Vec<RemovedArticle>) {
    let mut originals = Vec::new();
    let mut removed = Vec::new();
    let mut processed: HashSet<usize> = HashSet::new();

    for (i, article_a) in articles.iter().enumerate() {
        if processed.contains(&i) {
            continue;
        }

        // This article becomes the "original" for its group
        originals.push(article_a.clone());
        processed.insert(i);

        // Find similar articles
        for (j, article_b) in articles.iter().enumerate().skip(i + 1) {
            if processed.contains(&j) {
                continue;
            }

            let sim = title_similarity(&article_a.title, &article_b.title);
            if sim >= threshold {
                removed.push(RemovedArticle {
                    article: article_b.clone(),
                    status: SyndicationStatus::TitleSimilar,
                    original_id: Some(article_a.id.clone()),
                });
                processed.insert(j);
            }
        }
    }

    (originals, removed)
}

/// Extract canonical URL from article metadata.
fn extract_canonical_url(article: &ArticleWithMetadata) -> Option<String> {
    article
        .canonical_url
        .clone()
        .or_else(|| article.og_url.clone())
        .map(|url| normalize_url(&url))
}

/// Check if a URL is from a known wire source.
#[must_use]
pub fn is_wire_source(url: &str) -> bool {
    let host = extract_host(url);
    if let Some(host) = host {
        let host_lower = host.to_lowercase();
        return WIRE_SOURCES
            .iter()
            .any(|source| host_lower.ends_with(source) || host_lower == *source);
    }
    false
}

/// Extract the host from a URL.
fn extract_host(url: &str) -> Option<String> {
    // Simple URL parsing without external crate
    let url = url.trim();
    let without_scheme = url
        .strip_prefix("https://")
        .or_else(|| url.strip_prefix("http://"))
        .unwrap_or(url);

    // Split at first "/" or "?" or "#" to get host part
    let host_part = without_scheme.split(['/', '?', '#']).next()?;

    // Remove port if present
    let host = host_part.split(':').next()?;

    if host.is_empty() {
        None
    } else {
        Some(host.to_string())
    }
}

/// Normalize a URL for comparison.
fn normalize_url(url: &str) -> String {
    let url = url.trim();

    // Remove fragment (everything after #)
    let without_fragment = url.split('#').next().unwrap_or(url);

    without_fragment.to_string()
}

/// Compute title similarity using Jaccard coefficient on word n-grams.
#[must_use]
pub fn title_similarity(a: &str, b: &str) -> f32 {
    let ngrams_a: HashSet<_> = word_ngrams(a, 2).collect();
    let ngrams_b: HashSet<_> = word_ngrams(b, 2).collect();

    if ngrams_a.is_empty() && ngrams_b.is_empty() {
        return 1.0;
    }

    if ngrams_a.is_empty() || ngrams_b.is_empty() {
        return 0.0;
    }

    let intersection = ngrams_a.intersection(&ngrams_b).count();
    let union = ngrams_a.union(&ngrams_b).count();

    if union == 0 {
        0.0
    } else {
        intersection as f32 / union as f32
    }
}

/// Generate word n-grams from text.
fn word_ngrams(text: &str, n: usize) -> impl Iterator<Item = String> + '_ {
    let words: Vec<&str> = text
        .split_whitespace()
        .map(|w| w.trim_matches(|c: char| c.is_ascii_punctuation()))
        .filter(|w| !w.is_empty())
        .collect();

    (0..words.len().saturating_sub(n - 1)).map(move |i| words[i..i + n].join(" ").to_lowercase())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn article(id: &str, title: &str, url: &str) -> ArticleWithMetadata {
        ArticleWithMetadata {
            id: id.to_string(),
            title: title.to_string(),
            source_url: url.to_string(),
            canonical_url: None,
            og_url: None,
        }
    }

    #[test]
    fn test_is_wire_source_positive() {
        assert!(is_wire_source("https://www.reuters.com/article/123"));
        assert!(is_wire_source("https://apnews.com/story/456"));
        assert!(is_wire_source("https://www.afp.com/news/789"));
        assert!(is_wire_source("https://www.kyodonews.jp/news/abc"));
        assert!(is_wire_source("https://www.jiji.com/jc/article"));
    }

    #[test]
    fn test_is_wire_source_negative() {
        assert!(!is_wire_source("https://www.nytimes.com/article/123"));
        assert!(!is_wire_source("https://techcrunch.com/story/456"));
        assert!(!is_wire_source("https://www.bbc.com/news/789"));
    }

    #[test]
    fn test_title_similarity_identical() {
        let sim = title_similarity("Apple releases new iPhone", "Apple releases new iPhone");
        assert!((sim - 1.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_title_similarity_different() {
        let sim = title_similarity("Apple releases iPhone", "Microsoft launches Windows");
        // No common bigrams
        assert!(sim < 0.1);
    }

    #[test]
    fn test_title_similarity_similar() {
        let sim = title_similarity(
            "Apple releases new iPhone 15 Pro",
            "Apple releases new iPhone 15 Pro Max",
        );
        // Should have moderate similarity due to common bigrams
        // Bigrams for first: ["apple releases", "releases new", "new iphone", "iphone 15", "15 pro"]
        // Bigrams for second: ["apple releases", "releases new", "new iphone", "iphone 15", "15 pro", "pro max"]
        // Intersection: 5, Union: 6, Jaccard: 5/6 ≈ 0.833
        assert!(sim > 0.5);
    }

    #[test]
    fn test_title_similarity_empty() {
        assert!((title_similarity("", "") - 1.0).abs() < f32::EPSILON);
        assert!((title_similarity("hello world", "") - 0.0).abs() < f32::EPSILON);
    }

    #[test]
    fn test_remove_by_canonical_no_duplicates() {
        let articles = vec![
            article("1", "Title 1", "https://a.com/1"),
            article("2", "Title 2", "https://b.com/2"),
        ];

        let (originals, removed) = remove_by_canonical(&articles);

        assert_eq!(originals.len(), 2);
        assert!(removed.is_empty());
    }

    #[test]
    fn test_remove_by_canonical_with_duplicates() {
        let mut a1 = article("1", "Title 1", "https://a.com/1");
        a1.canonical_url = Some("https://original.com/article".to_string());

        let mut a2 = article("2", "Title 1 (syndicated)", "https://b.com/2");
        a2.canonical_url = Some("https://original.com/article".to_string());

        let articles = vec![a1, a2];
        let (originals, removed) = remove_by_canonical(&articles);

        assert_eq!(originals.len(), 1);
        assert_eq!(removed.len(), 1);
        assert_eq!(removed[0].status, SyndicationStatus::CanonicalMatch);
    }

    #[test]
    fn test_remove_wire_sources() {
        let articles = vec![
            article("1", "Breaking news", "https://reuters.com/article"),
            article("2", "Local news", "https://localnews.com/story"),
            article("3", "Wire story", "https://apnews.com/news"),
        ];

        let (originals, removed) = remove_wire_sources(&articles);

        assert_eq!(originals.len(), 1);
        assert_eq!(originals[0].id, "2");
        assert_eq!(removed.len(), 2);
        assert!(
            removed
                .iter()
                .all(|r| r.status == SyndicationStatus::WireSource)
        );
    }

    #[test]
    fn test_remove_by_title_similarity() {
        let articles = vec![
            article("1", "Apple releases new iPhone 15 Pro", "https://a.com/1"),
            article(
                "2",
                "Apple releases new iPhone 15 Pro Max",
                "https://b.com/2",
            ),
            article("3", "Microsoft announces Windows 12", "https://c.com/3"),
        ];

        // Use threshold of 0.5 - articles 1 and 2 share 5/6 bigrams (0.833)
        let (originals, removed) = remove_by_title_similarity(&articles, 0.5);

        // Article 1 and 2 are similar, so one should be removed
        assert_eq!(originals.len(), 2);
        assert_eq!(removed.len(), 1);
        assert_eq!(removed[0].status, SyndicationStatus::TitleSimilar);
    }

    #[tokio::test]
    async fn test_default_remover_all_stages() {
        let config = PulseSyndicationConfig {
            canonical_enabled: crate::config::FeatureToggle::Enabled,
            wire_enabled: crate::config::FeatureToggle::Enabled,
            title_enabled: crate::config::FeatureToggle::Enabled,
            title_threshold: 0.7,
        };

        let remover = DefaultSyndicationRemover::new(config);

        let articles = vec![
            article("1", "Original story", "https://example.com/story"),
            article("2", "Wire copy", "https://reuters.com/wire"),
        ];

        let result = remover.remove_syndication(articles).await.unwrap();

        assert_eq!(result.original_articles.len(), 1);
        assert_eq!(result.removal_counts.wire, 1);
    }

    #[tokio::test]
    async fn test_default_remover_disabled_stages() {
        let config = PulseSyndicationConfig {
            canonical_enabled: crate::config::FeatureToggle::Disabled,
            wire_enabled: crate::config::FeatureToggle::Disabled,
            title_enabled: crate::config::FeatureToggle::Disabled,
            title_threshold: 0.85,
        };

        let remover = DefaultSyndicationRemover::new(config);

        let articles = vec![
            article("1", "Original story", "https://example.com/story"),
            article("2", "Wire copy", "https://reuters.com/wire"),
        ];

        let result = remover.remove_syndication(articles).await.unwrap();

        // All stages disabled, so nothing should be removed
        assert_eq!(result.original_articles.len(), 2);
        assert!(!result.has_removals());
    }

    #[test]
    fn test_word_ngrams() {
        let text = "Hello world from Rust";
        let ngrams: Vec<String> = word_ngrams(text, 2).collect();
        assert_eq!(ngrams.len(), 3);
        assert!(ngrams.contains(&"hello world".to_string()));
        assert!(ngrams.contains(&"world from".to_string()));
        assert!(ngrams.contains(&"from rust".to_string()));
    }

    #[test]
    fn test_normalize_url_removes_fragment() {
        let url = "https://example.com/article#section1";
        let normalized = normalize_url(url);
        assert_eq!(normalized, "https://example.com/article");
    }

    #[test]
    fn test_syndication_result_no_change() {
        let articles = vec![article("1", "Title", "https://example.com")];
        let result = SyndicationResult::no_change(articles);

        assert_eq!(result.original_articles.len(), 1);
        assert!(!result.has_removals());
        assert_eq!(result.total_removed(), 0);
    }
}
