/**
 * Consolidated mock data for E2E tests.
 * Centralizes all mock responses for consistency across test files.
 */

// Feed data
export const FEEDS_RESPONSE = {
	data: [
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
	],
	next_cursor: "next-cursor-123",
	has_more: true,
};

export const FEEDS_EMPTY_RESPONSE = {
	data: [],
	next_cursor: null,
	has_more: false,
};

export const VIEWED_FEEDS_EMPTY = {
	data: [],
	next_cursor: null,
	has_more: false,
};

// Stats data
export const STATS_RESPONSE = {
	feed_amount: { amount: 12 },
	total_articles: { amount: 345 },
	unsummarized_articles: { amount: 7 },
};

export const UNREAD_COUNT_RESPONSE = {
	count: 42,
};

// Search data
export const SEARCH_RESPONSE = {
	data: [
		{
			title: "AI Weekly",
			description:
				"A deep dive into AI research, tooling, and production learnings.",
			link: "https://example.com/ai-weekly",
			published: "2025-12-18T08:30:00Z",
			author: { name: "Casey" },
		},
	],
	next_cursor: null,
	has_more: false,
};

// Article content
export const ARTICLE_CONTENT_RESPONSE = {
	content: "<p>This is a mocked article content.</p>",
};

// Recap data - matches backend API response (snake_case)
// The client adapts these to camelCase internally
export const RECAP_RESPONSE = {
	job_id: "test-job-123",
	executed_at: "2025-12-20T12:00:00Z",
	window_start: "2025-12-13T00:00:00Z",
	window_end: "2025-12-20T00:00:00Z",
	total_articles: 4,
	genres: [
		{
			genre: "Technology",
			summary: "Major developments in technology this week include AI advances and new web frameworks.",
			top_terms: ["AI", "Web", "Frameworks"],
			article_count: 3,
			cluster_count: 2,
			evidence_links: [
				{ article_id: "art-1", title: "GPT-5 Announced", source_url: "https://example.com/gpt5", published_at: "2025-12-20T10:00:00Z", lang: "en" },
				{ article_id: "art-2", title: "Claude Gets Smarter", source_url: "https://example.com/claude", published_at: "2025-12-20T09:00:00Z", lang: "en" },
			],
			bullets: ["AI advances continue", "New web frameworks emerging"],
		},
		{
			genre: "AI/ML",
			summary: "Latest papers and breakthroughs in machine learning research.",
			top_terms: ["ML", "Research", "Transformers"],
			article_count: 1,
			cluster_count: 1,
			evidence_links: [
				{ article_id: "art-4", title: "New Transformer Architecture", source_url: "https://example.com/transformer", published_at: "2025-12-19T10:00:00Z", lang: "en" },
			],
			bullets: ["New transformer architecture proposed"],
		},
	],
};

export const RECAP_EMPTY_RESPONSE = {
	job_id: "test-job-empty",
	executed_at: "2025-12-20T12:00:00Z",
	window_start: "2025-12-13T00:00:00Z",
	window_end: "2025-12-20T00:00:00Z",
	total_articles: 0,
	genres: [],
};

// Augur chat data
export const AUGUR_WELCOME_MESSAGE = {
	text: "Hello! I'm Augur, your AI assistant for exploring feeds.",
};

// Augur streaming chunks - each is plain text (not JSON)
// The SSE format uses `event: delta` with data as plain text
export const AUGUR_RESPONSE_CHUNKS = [
	"Based on your recent feeds, ",
	"here are the key trends: ",
	"AI development is accelerating.",
];

// RSS Feed Link data
export const RSS_FEED_LINKS_RESPONSE: unknown[] = [];

// Mark as read response
export const MARK_AS_READ_RESPONSE = {
	ok: true,
};

// Factory functions for creating custom mock data
export const createFeed = (overrides: Partial<typeof FEEDS_RESPONSE.data[0]> = {}) => ({
	id: `feed-${Date.now()}`,
	url: "https://example.com/feed",
	title: "Mock Feed",
	description: "Mock description",
	link: "https://example.com/feed",
	published_at: new Date().toISOString(),
	tags: [],
	author: { name: "Mock Author" },
	thumbnail: null,
	feed_domain: "example.com",
	read_at: null,
	created_at: new Date().toISOString(),
	updated_at: new Date().toISOString(),
	...overrides,
});

export const createFeedsResponse = (
	feeds: typeof FEEDS_RESPONSE.data,
	hasMore = false,
	cursor: string | null = null,
) => ({
	data: feeds,
	next_cursor: cursor,
	has_more: hasMore,
});

export const createRecapGenre = (
	genreName: string,
	articleCount = 1,
	clusterCount = 1,
) => ({
	genre: genreName,
	summary: `Summary for ${genreName}`,
	top_terms: [genreName],
	article_count: articleCount,
	cluster_count: clusterCount,
	evidence_links: Array.from({ length: articleCount }, (_, i) => ({
		article_id: `${genreName.toLowerCase()}-art-${i}`,
		title: `${genreName} Article ${i + 1}`,
		source_url: `https://example.com/${genreName.toLowerCase()}-${i}`,
		published_at: new Date().toISOString(),
		lang: "en" as const,
	})),
	bullets: [`Key point about ${genreName}`],
});

// =============================================================================
// Connect-RPC v2 Mock Data (camelCase format)
// =============================================================================

/**
 * Connect-RPC FeedItem format (camelCase)
 */
export const CONNECT_FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			title: "AI Trends",
			description: "Deep dive into the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
		{
			id: "feed-2",
			title: "Svelte 5 Tips",
			description: "Runes-first patterns for fast interfaces.",
			link: "https://example.com/svelte-5",
			published: "1 day ago",
			createdAt: new Date().toISOString(),
			author: "Bob",
		},
	],
	nextCursor: "next-cursor-123",
	hasMore: true,
};

export const CONNECT_FEEDS_EMPTY_RESPONSE = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

export const CONNECT_READ_FEEDS_EMPTY_RESPONSE = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

export const CONNECT_MARK_AS_READ_RESPONSE = {
	message: "Feed marked as read",
};

export const CONNECT_SEARCH_RESPONSE = {
	data: [
		{
			id: "search-1",
			title: "AI Weekly",
			description: "A deep dive into AI research, tooling, and production learnings.",
			link: "https://example.com/ai-weekly",
			published: "3 days ago",
			createdAt: new Date().toISOString(),
			author: "Casey",
		},
	],
	nextCursor: null,
	hasMore: false,
};

// Connect-RPC service paths
export const CONNECT_RPC_PATHS = {
	getUnreadFeeds: "**/api/v2/alt.feeds.v2.FeedService/GetUnreadFeeds",
	getReadFeeds: "**/api/v2/alt.feeds.v2.FeedService/GetReadFeeds",
	markAsRead: "**/api/v2/alt.feeds.v2.FeedService/MarkAsRead",
	searchFeeds: "**/api/v2/alt.feeds.v2.FeedService/SearchFeeds",
	getFeedStats: "**/api/v2/alt.feeds.v2.FeedService/GetFeedStats",
	getDetailedFeedStats: "**/api/v2/alt.feeds.v2.FeedService/GetDetailedFeedStats",
	getUnreadCount: "**/api/v2/alt.feeds.v2.FeedService/GetUnreadCount",
	streamSummarize: "**/api/v2/alt.feeds.v2.FeedService/StreamSummarize",
	// Article service
	fetchArticleContent: "**/api/v2/alt.articles.v2.ArticleService/FetchArticleContent",
	// Augur service
	augurStreamChat: "**/api/v2/alt.augur.v2.AugurService/StreamChat",
};

// Connect-RPC Article Content response
export const CONNECT_ARTICLE_CONTENT_RESPONSE = {
	url: "https://example.com/ai-trends",
	content: "<p>This is a mocked article content for E2E testing.</p>",
	articleId: "article-123",
};

/**
 * Connect-RPC Augur streaming response messages.
 * Uses protobuf JSON wire format where oneof fields use their field name directly.
 */
export const CONNECT_AUGUR_STREAM_MESSAGES = [
	{ kind: "delta", delta: "Based on your recent feeds, " },
	{ kind: "delta", delta: "here are the key trends: " },
	{ kind: "delta", delta: "AI development is accelerating." },
	{
		kind: "done",
		done: {
			answer: "Based on your recent feeds, here are the key trends: AI development is accelerating.",
			citations: [
				{ url: "https://example.com/ai", title: "AI News", publishedAt: "2025-12-20T10:00:00Z" },
			],
		},
	},
];

/**
 * Simple Connect-RPC Augur response for conversation tests
 */
export const CONNECT_AUGUR_SIMPLE_RESPONSE = [
	{ kind: "delta", delta: "Response to your question." },
	{
		kind: "done",
		done: {
			answer: "Response to your question.",
			citations: [],
		},
	},
];
