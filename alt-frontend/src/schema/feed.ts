export type Feed = {
  id: string;
  title: string;
  description: string;
  link: string;
  published: string;
  author?: string;
};

export type SanitizedFeed = {
  id: string;
  title: string; // サニタイゼーション済み
  description: string; // サニタイゼーション済み
  link: string; // 検証済みURL
  published: string;
  author?: string; // サニタイゼーション済み
};

import { sanitizeFeedContent } from "@/utils/contentSanitizer";

export interface BackendFeedItem {
  title: string;
  description: string;
  link: string;
  links?: string[];
  published?: string;
  author?: {
    name: string;
  };
  authors?: Array<{
    name: string;
  }>;
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
    author: sanitized.author || undefined,
  };
}

import type { SafeHtmlString } from "@/lib/server/sanitize-html";

export interface FeedContentOnTheFlyResponse {
  content: SafeHtmlString; // Server-sanitized HTML
}
