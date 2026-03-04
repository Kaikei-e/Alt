/**
 * Feed Mock Data
 */

import type {
	Feed,
	FeedsResponse,
	FeedStats,
	DetailedFeedStats,
	ConnectFeed,
	ConnectFeedsResponse,
	ConnectDetailedStats,
	ConnectArticleContent,
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
		id: "feed-1",
		articleId: "article-1",
		title: "AI Trends",
		description: "Deep dive into the ecosystem.",
		link: "https://example.com/ai-trends",
		published: "2 hours ago",
		createdAt: new Date().toISOString(),
		author: "Alice",
	},
	{
		id: "feed-2",
		articleId: "article-2",
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
	articleId: "article-123",
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
// Connect-RPC Evening Pulse Data (proto JSON wire format)
// =============================================================================

export const CONNECT_EVENING_PULSE = {
	jobId: "pulse-job-123",
	date: "2026-01-15",
	generatedAt: new Date().toISOString(),
	status: 1, // PULSE_STATUS_NORMAL
	topics: [
		{
			clusterId: "1",
			role: 1,
			title: "AI Breakthrough",
			rationale: { text: "Major advances in AI this week.", confidence: 1 },
			articleCount: 5,
			sourceCount: 3,
			timeAgo: "3 hours ago",
			genre: "Technology",
			articleIds: ["art-1", "art-2"],
			representativeArticles: [
				{
					articleId: "art-1",
					title: "GPT-5 Announced",
					sourceUrl: "https://example.com/gpt5",
					sourceName: "TechNews",
					publishedAt: "2026-01-15T10:00:00Z",
				},
			],
			topEntities: ["AI", "GPT", "LLM"],
			sourceNames: ["TechNews", "AIDaily"],
		},
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
