import { sanitizeFeedContent } from "@/utils/contentSanitizer";
import {
  formatPublishedDate,
  mergeTagsLabel,
  normalizeUrl,
  generateExcerptFromDescription,
} from "@/lib/server/feed-formatters";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

export type Feed = {
  id: string;
  title: string;
  description: string;
  link: string;
  published: string;
  created_at?: string;
  author?: string;
};

export type SanitizedFeed = {
  id: string;
  title: string; // サニタイゼーション済み
  description: string; // サニタイゼーション済み
  link: string; // 検証済みURL
  published: string;
  created_at?: string;
  author?: string; // サニタイゼーション済み
};

/**
 * Render-ready feed type with server-generated display values for LCP optimization
 * This type extends Feed with SSR-generated formatting fields
 */
export type RenderFeed = Feed & {
  publishedAtFormatted: string; // e.g., "Nov 23, 2025"
  mergedTagsLabel: string; // e.g., "Next.js / Performance" (empty if no tags)
  normalizedUrl: string; // URL with tracking params removed
  excerpt: string; // 100-160 char excerpt from content
};

export interface BackendFeedItem {
  title: string;
  description: string;
  link: string;
  links?: string[];
  published?: string;
  created_at?: string;
  author?: {
    name: string;
  };
  authors?: Array<{
    name: string;
  }>;
  tags?: string[]; // Tags array from backend (if available)
}

export interface FeedURLPayload {
  feed_url: string;
}

// 汎用的な記事サマリーレスポンス型（内部実装を隠蔽）
export interface ArticleSummaryItem {
  article_url: string;
  title: string;
  author?: string;
  content: SafeHtmlString; // Server-sanitized HTML
  content_type: string;
  published_at: string;
  fetched_at: string;
  source_id: string; // 内部IDを汎用化
}

// 汎用記事サマリーAPI リクエスト/レスポンス型
export interface FetchArticleSummaryRequest {
  feed_urls: string[];
}

export interface FetchArticleSummaryResponse {
  matched_articles: ArticleSummaryItem[];
  total_matched: number;
  requested_count: number;
}

export interface FeedDetails {
  feed_url: string;
  summary: string;
}

/**
 * Transform raw feed data to sanitized feed
 * @param rawFeed - Raw feed data from API
 * @returns Sanitized feed object
 */
export function sanitizeFeed(rawFeed: BackendFeedItem): SanitizedFeed {
  const sanitized = sanitizeFeedContent({
    title: rawFeed.title,
    description: rawFeed.description,
    author: rawFeed.author?.name || rawFeed.authors?.[0]?.name || "",
    link: rawFeed.link,
  });

  return {
    id: rawFeed.link || "",
    title: sanitized.title,
    description: sanitized.description,
    link: sanitized.link,
    published: rawFeed.published || "",
    created_at: rawFeed.created_at,
    author: sanitized.author || undefined,
  };
}

/**
 * Transform sanitized feed to render-ready feed with SSR-generated display values
 * This function should be called in Server Components to generate display values on the server
 * @param feed - Sanitized feed object
 * @param tags - Optional tags array (if available from backend)
 * @returns Render-ready feed with formatted display values
 */
export function toRenderFeed(feed: SanitizedFeed, tags?: string[]): RenderFeed {
  return {
    ...feed,
    publishedAtFormatted: formatPublishedDate(
      feed.published || feed.created_at,
    ),
    mergedTagsLabel: mergeTagsLabel(tags),
    normalizedUrl: normalizeUrl(feed.link),
    excerpt: generateExcerptFromDescription(feed.description),
  };
}

export interface FeedContentOnTheFlyResponse {
  content: SafeHtmlString; // Server-sanitized HTML
}
