/**
 * Factory for feed mock data.
 * Produces both REST v1 (snake_case) and Connect-RPC v2 (camelCase) formats.
 */

let feedCounter = 0;

export interface FeedV1 {
	id: string;
	url: string;
	title: string;
	description: string;
	link: string;
	published_at: string;
	tags: string[];
	author: { name: string };
	thumbnail: string | null;
	feed_domain: string;
	read_at: string | null;
	created_at: string;
	updated_at: string;
}

export interface ConnectFeedItem {
	id: string;
	articleId: string;
	title: string;
	description: string;
	link: string;
	published: string;
	createdAt: string;
	author: string;
}

export function buildFeedV1(overrides: Partial<FeedV1> = {}): FeedV1 {
	feedCounter++;
	return {
		id: `feed-${feedCounter}`,
		url: `https://example.com/feed-${feedCounter}`,
		title: `Feed ${feedCounter}`,
		description: `Description for feed ${feedCounter}`,
		link: `https://example.com/feed-${feedCounter}`,
		published_at: new Date().toISOString(),
		tags: [],
		author: { name: "Test Author" },
		thumbnail: null,
		feed_domain: "example.com",
		read_at: null,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
		...overrides,
	};
}

export function buildConnectFeedItem(
	overrides: Partial<ConnectFeedItem> = {},
): ConnectFeedItem {
	feedCounter++;
	return {
		id: `feed-${feedCounter}`,
		articleId: `article-${feedCounter}`,
		title: `Feed ${feedCounter}`,
		description: `Description for feed ${feedCounter}`,
		link: `https://example.com/feed-${feedCounter}`,
		published: "1 hour ago",
		createdAt: new Date().toISOString(),
		author: "Test Author",
		...overrides,
	};
}

export function buildFeedsV1Response(
	feeds?: FeedV1[],
	hasMore = false,
	cursor: string | null = null,
) {
	return {
		data: feeds ?? [buildFeedV1({ title: "AI Trends" }), buildFeedV1({ title: "Svelte 5 Tips" })],
		next_cursor: cursor,
		has_more: hasMore,
	};
}

export function buildConnectFeedsResponse(
	feeds?: ConnectFeedItem[],
	hasMore = false,
	nextCursor = "",
) {
	return {
		data: feeds ?? [
			buildConnectFeedItem({ title: "AI Trends" }),
			buildConnectFeedItem({ title: "Svelte 5 Tips" }),
		],
		nextCursor,
		hasMore,
	};
}

export function buildConnectArticleContent(overrides: Record<string, unknown> = {}) {
	return {
		url: "https://example.com/article",
		content: "<p>Mocked article content for E2E testing.</p>",
		articleId: "article-123",
		...overrides,
	};
}

export function resetFeedCounter(): void {
	feedCounter = 0;
}
