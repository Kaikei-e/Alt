import type { BackendFeedItem } from "./feed";

/**
 * Search result feed item with full description text (not truncated)
 * This is separate from BackendFeedItem to ensure search results always have full descriptions
 * for the "Read more" functionality in Search Feeds page
 */
export interface SearchFeedItem extends BackendFeedItem {
	description: string; // Full description text, not truncated
}

/**
 * Search query type
 */
export type SearchQuery = {
	query: string;
};

/**
 * Feed search result with cursor-based pagination support
 */
export type FeedSearchResult = {
	results: SearchFeedItem[];
	error: string | null;
	next_cursor?: number | null; // Offset for next page (integer)
	has_more?: boolean;
};

/**
 * Cursor-based search response (when backend supports pagination)
 */
export interface CursorSearchResponse {
	data: SearchFeedItem[];
	next_cursor: number | null; // Offset for next page (integer)
	has_more: boolean;
}
