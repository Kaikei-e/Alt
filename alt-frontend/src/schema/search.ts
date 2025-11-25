import type { BackendFeedItem } from "@/schema/feed";

/**
 * Search result feed item with full description text (not truncated)
 * This is separate from BackendFeedItem to ensure search results always have full descriptions
 * for the "Read more" functionality in Search Feeds page
 */
export interface SearchFeedItem extends BackendFeedItem {
  description: string; // Full description text, not truncated
}

export type FeedSearchResult = {
  results: SearchFeedItem[];
  error: string | null;
};
