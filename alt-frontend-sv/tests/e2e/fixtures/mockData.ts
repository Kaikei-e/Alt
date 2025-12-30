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

// Recap data - matches RecapGenre schema
export const RECAP_RESPONSE = {
	genres: [
		{
			genre: "Technology",
			summary: "Major developments in technology this week include AI advances and new web frameworks.",
			topTerms: ["AI", "Web", "Frameworks"],
			articleCount: 3,
			clusterCount: 2,
			evidenceLinks: [
				{ articleId: "art-1", title: "GPT-5 Announced", sourceUrl: "https://example.com/gpt5", publishedAt: "2025-12-20T10:00:00Z", lang: "en" },
				{ articleId: "art-2", title: "Claude Gets Smarter", sourceUrl: "https://example.com/claude", publishedAt: "2025-12-20T09:00:00Z", lang: "en" },
			],
			bullets: ["AI advances continue", "New web frameworks emerging"],
		},
		{
			genre: "AI/ML",
			summary: "Latest papers and breakthroughs in machine learning research.",
			topTerms: ["ML", "Research", "Transformers"],
			articleCount: 1,
			clusterCount: 1,
			evidenceLinks: [
				{ articleId: "art-4", title: "New Transformer Architecture", sourceUrl: "https://example.com/transformer", publishedAt: "2025-12-19T10:00:00Z", lang: "en" },
			],
			bullets: ["New transformer architecture proposed"],
		},
	],
};

export const RECAP_EMPTY_RESPONSE = {
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
	topTerms: [genreName],
	articleCount,
	clusterCount,
	evidenceLinks: Array.from({ length: articleCount }, (_, i) => ({
		articleId: `${genreName.toLowerCase()}-art-${i}`,
		title: `${genreName} Article ${i + 1}`,
		sourceUrl: `https://example.com/${genreName.toLowerCase()}-${i}`,
		publishedAt: new Date().toISOString(),
		lang: "en" as const,
	})),
	bullets: [`Key point about ${genreName}`],
});
