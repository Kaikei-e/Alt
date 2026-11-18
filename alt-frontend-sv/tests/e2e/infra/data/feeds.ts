/**
 * Feed Mock Data
 */

import type {
	ConnectArticleContent,
	ConnectDetailedStats,
	ConnectFeed,
	ConnectFeedsResponse,
	DetailedFeedStats,
	Feed,
	FeedStats,
	FeedsResponse,
	RssFeedLink,
} from "../types";

// =============================================================================
// REST v1 Feed Data
// =============================================================================

export const MOCK_FEEDS: Feed[] = [
	{
		id: "feed-1",
		url: "https://example.com/ai-trends",
		title: "AI Trends",
		description: "Deep dive into the ecosystem.",
		link: "https://example.com/ai-trends",
		published_at: "2025-12-20T10:00:00Z",
		tags: ["AI", "Tech"],
		author: { name: "Alice" },
		thumbnail: "https://example.com/thumb.jpg",
		feed_domain: "example.com",
		read_at: null,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
	},
	{
		id: "feed-2",
		url: "https://example.com/svelte-5",
		title: "Svelte 5 Tips",
		description: "Runes-first patterns for fast interfaces.",
		link: "https://example.com/svelte-5",
		published_at: "2025-12-19T09:00:00Z",
		tags: ["Svelte", "Web"],
		author: { name: "Bob" },
		thumbnail: null,
		feed_domain: "svelte.dev",
		read_at: null,
		created_at: new Date().toISOString(),
		updated_at: new Date().toISOString(),
	},
];

export const FEEDS_RESPONSE: FeedsResponse = {
	data: MOCK_FEEDS,
	next_cursor: "next-cursor-123",
	has_more: true,
};

export const VIEWED_FEEDS_RESPONSE: FeedsResponse = {
	data: [],
	next_cursor: null,
	has_more: false,
};

export const FEED_STATS: FeedStats = {
	total_feeds: 12,
	total_reads: 345,
	unread_count: 7,
};

export const DETAILED_FEED_STATS: DetailedFeedStats = {
	feed_amount: { amount: 10 },
	total_articles: { amount: 50 },
	unsummarized_articles: { amount: 5 },
};

export const UNREAD_COUNT = { count: 5 };

// =============================================================================
// Connect-RPC v2 Feed Data
// =============================================================================

export const MOCK_CONNECT_FEEDS: ConnectFeed[] = [
	{
		// `id` and `feedId` are deliberately different strings. `id` is the
		// render key (articles.id, or a UUID minted per response); `feedId` is
		// feeds.id, which is what ResolveOgImages matches. A fixture where the
		// two agreed would keep passing if a card regressed to sending `id` —
		// the defect that shipped once already.
		id: "feed-1",
		feedId: "feeds-row-1",
		articleId: "01938e82-7c00-7a7b-9b10-000000000001",
		title: "AI Trends",
		description: "Deep dive into the ecosystem.",
		link: "https://example.com/ai-trends",
		published: "2 hours ago",
		createdAt: new Date().toISOString(),
		author: "Alice",
	},
	{
		id: "feed-2",
		feedId: "feeds-row-2",
		articleId: "01938e82-7c00-7a7b-9b10-000000000002",
		title: "Svelte 5 Tips",
		description: "Runes-first patterns for fast interfaces.",
		link: "https://example.com/svelte-5",
		published: "1 day ago",
		createdAt: new Date().toISOString(),
		author: "Bob",
	},
];

export const CONNECT_FEEDS_RESPONSE: ConnectFeedsResponse = {
	data: MOCK_CONNECT_FEEDS,
	nextCursor: "next-cursor-123",
	hasMore: true,
};

/**
 * Terminal page for the cursor-paginated feed endpoints: what the mock backend
 * answers once a cursor has been supplied. `CONNECT_FEEDS_RESPONSE` advertises
 * `hasMore: true`, so without a real second page the client's infinite scroll
 * would request forever.
 */
export const CONNECT_FEEDS_LAST_PAGE_RESPONSE: ConnectFeedsResponse = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

export const CONNECT_READ_FEEDS_RESPONSE: ConnectFeedsResponse = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

export const CONNECT_DETAILED_STATS: ConnectDetailedStats = {
	feedAmount: 12,
	articleAmount: 345,
	unsummarizedFeedAmount: 7,
};

export const CONNECT_UNREAD_COUNT = { count: 42 };

export const CONNECT_ARTICLE_CONTENT: ConnectArticleContent = {
	url: "https://example.com/ai-trends",
	content: "<p>This is a mocked article content for E2E testing.</p>",
	articleId: "01938e82-7c00-7a7b-9b10-000000000123",
};

// =============================================================================
// Connect-RPC Tag Trail Feed Data
// =============================================================================

export const CONNECT_TAG_TRAIL_FEED = {
	id: "trail-feed-1",
	url: "https://example.com/trail-feed",
	title: "Random Trail Feed",
	description: "A randomly selected feed for tag exploration.",
	tags: [
		{ id: "tag-1", name: "AI", createdAt: "2026-01-01T00:00:00Z" },
		{
			id: "tag-2",
			name: "Machine Learning",
			createdAt: "2026-01-01T00:00:00Z",
		},
		{ id: "tag-3", name: "Technology", createdAt: "2026-01-01T00:00:00Z" },
	],
};

// =============================================================================
// Connect-RPC Tag Trail Articles Data
// =============================================================================

export const CONNECT_TAG_TRAIL_ARTICLES = {
	articles: [
		{
			id: "trail-art-1",
			title: "AI Trends in 2026",
			link: "https://example.com/ai-trends-2026",
			publishedAt: "2026-01-15T10:00:00Z",
			feedTitle: "TechBlog",
		},
		{
			id: "trail-art-2",
			title: "Machine Learning Basics",
			link: "https://example.com/ml-basics",
			publishedAt: "2026-01-15T07:00:00Z",
			feedTitle: "AI Weekly",
		},
	],
	hasMore: false,
};

// =============================================================================
// RSS Feed Link Data
// =============================================================================

export const RSS_FEED_LINKS: RssFeedLink[] = [];

// =============================================================================
// Feed Inflow Stats & Daily Cap (Connect-RPC v2)
// =============================================================================

export interface MockFeedInflowStat {
	feedLinkId: string;
	url: string;
	dailyCap?: number;
	inflowCount: string;
	readCount: string;
	openedCountLegacy: string;
	readRate: number;
}

export const INITIAL_FEED_INFLOW_STATS: MockFeedInflowStat[] = [
	{
		feedLinkId: "11111111-1111-4111-8111-111111111111",
		url: "https://engaged.example/atom.xml",
		inflowCount: "420",
		readCount: "210",
		openedCountLegacy: "8",
		readRate: 0.5,
	},
	{
		feedLinkId: "22222222-2222-4222-8222-222222222222",
		url: "https://noisy.example/feed.xml",
		inflowCount: "180",
		readCount: "9",
		openedCountLegacy: "0",
		readRate: 0.05,
	},
	{
		feedLinkId: "33333333-3333-4333-8333-333333333333",
		url: "https://alpha-quiet.example/rss.xml",
		inflowCount: "0",
		readCount: "0",
		openedCountLegacy: "0",
		readRate: 0,
	},
];
