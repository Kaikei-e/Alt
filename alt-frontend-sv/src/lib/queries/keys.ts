/**
 * TanStack Query Key definitions for Connect-RPC queries
 *
 * Using a consistent key factory pattern for:
 * - Query invalidation
 * - Cache management
 * - Optimistic updates
 */

// =============================================================================
// Feed Query Keys
// =============================================================================

export const feedKeys = {
	all: ["feeds"] as const,
	lists: () => [...feedKeys.all, "list"] as const,
	list: (filter: string) => [...feedKeys.lists(), { filter }] as const,
	unread: () => [...feedKeys.list("unread")] as const,
	read: () => [...feedKeys.list("read")] as const,
	favorites: () => [...feedKeys.list("favorites")] as const,
	search: (query: string) => [...feedKeys.all, "search", query] as const,
	stats: () => [...feedKeys.all, "stats"] as const,
	detailedStats: () => [...feedKeys.stats(), "detailed"] as const,
	unreadCount: () => [...feedKeys.all, "unreadCount"] as const,
};

// =============================================================================
// Article Query Keys
// =============================================================================

export const articleKeys = {
	all: ["articles"] as const,
	content: (url: string) => [...articleKeys.all, "content", url] as const,
	list: () => [...articleKeys.all, "list"] as const,
	cursor: (cursor?: string) => [...articleKeys.list(), { cursor }] as const,
};

// =============================================================================
// RSS Query Keys
// =============================================================================

export const rssKeys = {
	all: ["rss"] as const,
	links: () => [...rssKeys.all, "links"] as const,
};
