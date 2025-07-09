import { Feed, SanitizedFeed } from "@/schema/feed";
import { escapeHtml } from "@/utils/htmlEscape";

export interface SearchOptions {
  multiKeyword?: boolean;
  searchFields?: ("title" | "description" | "tags")[];
  fuzzyMatch?: boolean;
  minimumScore?: number;
}

export interface SearchResult {
  feed: Feed | SanitizedFeed;
  score: number;
  matchedFields: string[];
  highlightedTitle?: string;
  highlightedDescription?: string;
}

// Cache for compiled regex patterns to improve performance
const regexCache = new Map<string, RegExp>();

/**
 * Get a cached regex pattern or create and cache a new one
 */
function getCachedRegex(keyword: string): RegExp {
  if (!regexCache.has(keyword)) {
    regexCache.set(keyword, new RegExp(`\\b${escapeRegExp(keyword)}\\b`, "i"));
  }
  return regexCache.get(keyword)!;
}

/**
 * Advanced search function that supports multiple keywords and scoring
 * Optimized for performance with caching and early exits
 */
export function searchFeeds(
  feeds: (Feed | SanitizedFeed)[],
  query: string,
  options: SearchOptions = {},
): SearchResult[] {
  if (!query.trim()) {
    return feeds.map((feed) => ({
      feed,
      score: 1,
      matchedFields: [],
    }));
  }

  const {
    multiKeyword = true,
    searchFields = ["title", "description"],
    fuzzyMatch = false,
    minimumScore = 0.1,
  } = options;

  // Split query into keywords and normalize
  const keywords = multiKeyword
    ? query
        .toLowerCase()
        .split(/\s+/)
        .filter((k) => k.length > 0)
    : [query.toLowerCase().trim()];

  const results: SearchResult[] = [];

  for (const feed of feeds) {
    let totalScore = 0;
    const matchedFields: string[] = [];
    let highlightedTitle = feed.title;
    let highlightedDescription = feed.description;

    // Score calculation for each field
    for (const field of searchFields) {
      let fieldContent = "";
      let fieldWeight = 1;

      switch (field) {
        case "title":
          fieldContent = feed.title.toLowerCase();
          fieldWeight = 2; // Title matches are more important
          break;
        case "description":
          fieldContent = feed.description.toLowerCase();
          fieldWeight = 1;
          break;
        case "tags":
          // Handle tags if available in metadata
          const metadata = (feed as Feed & { metadata?: { tags?: string[] } })
            .metadata;
          if (metadata?.tags) {
            fieldContent = metadata.tags.join(" ").toLowerCase();
            fieldWeight = 1.5;
          }
          break;
      }

      if (!fieldContent) continue;

      let fieldScore = 0;
      let keywordMatches = 0;

      for (const keyword of keywords) {
        if (fuzzyMatch) {
          // Simple fuzzy matching - check if keyword is a substring
          if (fieldContent.includes(keyword)) {
            fieldScore += 1;
            keywordMatches++;
          }
        } else {
          // Exact word matching with cached regex
          const wordBoundaryRegex = getCachedRegex(keyword);
          if (wordBoundaryRegex.test(fieldContent)) {
            fieldScore += 1;
            keywordMatches++;
          } else if (fieldContent.includes(keyword)) {
            // Partial match gets lower score
            fieldScore += 0.5;
            keywordMatches++;
          }
        }
      }

      if (keywordMatches > 0) {
        matchedFields.push(field);
        // Normalize score by number of keywords and apply field weight
        const normalizedScore = (fieldScore / keywords.length) * fieldWeight;
        totalScore += normalizedScore;

        // Generate highlights for title and description
        if (field === "title") {
          highlightedTitle = highlightText(feed.title, keywords);
        } else if (field === "description") {
          highlightedDescription = highlightText(feed.description, keywords);
        }
      }
    }

    // Only include results that meet minimum score
    if (totalScore >= minimumScore) {
      results.push({
        feed,
        score: totalScore,
        matchedFields,
        highlightedTitle,
        highlightedDescription,
      });
    }
  }

  // Sort by score (descending)
  return results.sort((a, b) => b.score - a.score);
}

/**
 * Highlight matching keywords in text
 * XSS攻撃防止のため、テキストを事前にエスケープしてからハイライトを適用
 */
function highlightText(text: string, keywords: string[]): string {
  // テキストを事前にエスケープしてXSS攻撃を防止
  const escapedText = escapeHtml(text);
  let highlighted = escapedText;

  for (const keyword of keywords) {
    // キーワードもエスケープして安全なパターンを作成
    const escapedKeyword = escapeRegExp(escapeHtml(keyword));
    const regex = new RegExp(`(${escapedKeyword})`, "gi");
    highlighted = highlighted.replace(regex, "<mark>$1</mark>");
  }

  return highlighted;
}

/**
 * Escape special regex characters
 */
function escapeRegExp(string: string): string {
  return string.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

/**
 * Simple search that maintains backward compatibility
 */
export function simpleSearch(feeds: (Feed | SanitizedFeed)[], query: string): (Feed | SanitizedFeed)[] {
  if (!query.trim()) return feeds;

  const lowercaseQuery = query.toLowerCase();
  return feeds.filter(
    (feed) =>
      feed.title.toLowerCase().includes(lowercaseQuery) ||
      feed.description.toLowerCase().includes(lowercaseQuery),
  );
}
