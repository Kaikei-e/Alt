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

// RSS Feed Link mock data for settings/feeds tests
export const RSS_FEED_LINKS_LIST_RESPONSE = {
	links: [
		{
			id: "feed-link-1",
			url: "https://example.com/feed.xml",
		},
		{
			id: "feed-link-2",
			url: "https://blog.example.org/rss",
		},
		{
			id: "feed-link-3",
			url: "https://news.site.com/atom.xml",
		},
	],
};

export const RSS_FEED_LINKS_EMPTY_RESPONSE = {
	links: [],
};

export const RSS_FEED_REGISTER_RESPONSE = {
	message: "Feed registered successfully",
};

export const RSS_FEED_DELETE_RESPONSE = {
	message: "Feed deleted successfully",
};

// Connect-RPC RSS service paths
export const CONNECT_RSS_PATHS = {
	listRSSFeedLinks: "**/api/v2/alt.rss.v2.RSSService/ListRSSFeedLinks",
	registerRSSFeed: "**/api/v2/alt.rss.v2.RSSService/RegisterRSSFeed",
	deleteRSSFeedLink: "**/api/v2/alt.rss.v2.RSSService/DeleteRSSFeedLink",
};

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
	],
	nextCursor: "next-cursor-123",
	hasMore: true,
};

/**
 * Connect-RPC FeedItem with 3 feeds for navigation testing
 */
export const CONNECT_FEEDS_NAVIGATION_RESPONSE = {
	data: [
		{
			id: "feed-1",
			articleId: "article-1",
			title: "First Feed",
			description: "First feed description.",
			link: "https://example.com/first",
			published: "1 hour ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
		{
			id: "feed-2",
			articleId: "article-2",
			title: "Second Feed",
			description: "Second feed description.",
			link: "https://example.com/second",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Bob",
		},
		{
			id: "feed-3",
			articleId: "article-3",
			title: "Third Feed",
			description: "Third feed description.",
			link: "https://example.com/third",
			published: "3 hours ago",
			createdAt: new Date().toISOString(),
			author: "Charlie",
		},
	],
	nextCursor: "",
	hasMore: false,
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

// Connect-RPC FeedItem without articleId (not saved in database)
// Used to test the "Not Saved" -> "Mark as Read" transition after fetching full article
export const CONNECT_FEEDS_WITHOUT_ARTICLE_ID = {
	data: [
		{
			id: "feed-1",
			articleId: "", // Empty = not saved in articles table
			title: "AI Trends",
			description: "Deep dive into the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
	],
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
	fetchArticleSummary: "**/api/v2/alt.articles.v2.ArticleService/FetchArticleSummary",
	// Augur service
	augurStreamChat: "**/api/v2/alt.augur.v2.AugurService/StreamChat",
	// MorningLetter service
	morningLetterStreamChat: "**/api/v2/alt.morning_letter.v2.MorningLetterService/StreamChat",
	// Recap service
	getSevenDayRecap: "**/alt.recap.v2.RecapService/GetSevenDayRecap",
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

// =============================================================================
// MorningLetter Mock Data
// =============================================================================

/**
 * Connect-RPC MorningLetter streaming response messages.
 * MorningLetter is similar to Augur but includes time window metadata.
 */
export const CONNECT_MORNING_LETTER_STREAM_MESSAGES = [
	{
		kind: "meta",
		meta: {
			citations: [],
			timeWindow: {
				since: "2025-12-30T00:00:00Z",
				until: "2025-12-31T00:00:00Z",
			},
			articlesScanned: 42,
		},
	},
	{ kind: "delta", delta: "Based on the past 24 hours of news, " },
	{ kind: "delta", delta: "here are the key developments: " },
	{ kind: "delta", delta: "AI research has made significant progress." },
	{
		kind: "done",
		done: {
			answer: "Based on the past 24 hours of news, here are the key developments: AI research has made significant progress.",
			citations: [
				{ url: "https://example.com/ai-news", title: "AI Research Update", publishedAt: "2025-12-31T08:00:00Z" },
				{ url: "https://example.com/tech-weekly", title: "Tech Weekly", publishedAt: "2025-12-31T06:00:00Z" },
			],
		},
	},
];

/**
 * Simple Connect-RPC MorningLetter response for conversation tests
 */
export const CONNECT_MORNING_LETTER_SIMPLE_RESPONSE = [
	{
		kind: "meta",
		meta: {
			citations: [],
			timeWindow: {
				since: "2025-12-30T00:00:00Z",
				until: "2025-12-31T00:00:00Z",
			},
			articlesScanned: 10,
		},
	},
	{ kind: "delta", delta: "Here is your morning briefing." },
	{
		kind: "done",
		done: {
			answer: "Here is your morning briefing.",
			citations: [],
		},
	},
];

// =============================================================================
// Connect-RPC Recap Mock Data
// =============================================================================

/**
 * Connect-RPC format for GetSevenDayRecap response.
 */
export const CONNECT_RECAP_RESPONSE = {
	jobId: "test-job-123",
	executedAt: "2025-12-20T12:00:00Z",
	windowStart: "2025-12-13T00:00:00Z",
	windowEnd: "2025-12-20T00:00:00Z",
	totalArticles: 3,
	genres: [
		{
			genre: "Technology",
			summary: "Major developments in technology this week.",
			topTerms: ["AI", "Web", "Frameworks"],
			articleCount: 2,
			clusterCount: 1,
			evidenceLinks: [
				{
					articleId: "art-1",
					title: "GPT-5 Announced",
					sourceUrl: "https://example.com/gpt5",
					publishedAt: "2025-12-20T10:00:00Z",
					lang: "en",
				},
				{
					articleId: "art-2",
					title: "Claude Updates",
					sourceUrl: "https://example.com/claude",
					publishedAt: "2025-12-20T09:00:00Z",
					lang: "en",
				},
			],
			bullets: ["AI advances continue"],
			references: [],
		},
		{
			genre: "AI/ML",
			summary: "Latest papers and breakthroughs in ML.",
			topTerms: ["ML", "Research"],
			articleCount: 1,
			clusterCount: 1,
			evidenceLinks: [
				{
					articleId: "art-3",
					title: "New Architecture",
					sourceUrl: "https://example.com/arch",
					publishedAt: "2025-12-19T10:00:00Z",
					lang: "en",
				},
			],
			bullets: ["New architecture proposed"],
			references: [],
		},
	],
};

/**
 * Empty Connect-RPC recap response for testing empty state.
 */
export const CONNECT_RECAP_EMPTY_RESPONSE = {
	jobId: "test-job-empty",
	executedAt: "2025-12-20T12:00:00Z",
	windowStart: "2025-12-13T00:00:00Z",
	windowEnd: "2025-12-20T00:00:00Z",
	totalArticles: 0,
	genres: [],
};

// =============================================================================
// Job Dashboard Mock Data
// =============================================================================

/**
 * Job progress response with completed and failed jobs
 */
export const JOB_PROGRESS_RESPONSE = {
	active_job: null,
	recent_jobs: [
		{
			job_id: "job-001-test-abc123",
			status: "completed",
			last_stage: "persist",
			kicked_at: "2025-12-20T10:00:00Z",
			updated_at: "2025-12-20T10:05:30Z",
			duration_secs: 330,
			trigger_source: "system",
			user_id: null,
			status_history: [
				{ id: 1, status: "running", stage: "fetch", transitioned_at: "2025-12-20T10:00:00Z", reason: null, actor: "system" },
				{ id: 2, status: "completed", stage: "fetch", transitioned_at: "2025-12-20T10:00:45Z", reason: null, actor: "system" },
				{ id: 3, status: "running", stage: "preprocess", transitioned_at: "2025-12-20T10:00:45Z", reason: null, actor: "system" },
				{ id: 4, status: "completed", stage: "preprocess", transitioned_at: "2025-12-20T10:01:30Z", reason: null, actor: "system" },
				{ id: 5, status: "running", stage: "dedup", transitioned_at: "2025-12-20T10:01:30Z", reason: null, actor: "system" },
				{ id: 6, status: "completed", stage: "dedup", transitioned_at: "2025-12-20T10:02:00Z", reason: null, actor: "system" },
				{ id: 7, status: "running", stage: "genre", transitioned_at: "2025-12-20T10:02:00Z", reason: null, actor: "system" },
				{ id: 8, status: "completed", stage: "genre", transitioned_at: "2025-12-20T10:02:30Z", reason: null, actor: "system" },
				{ id: 9, status: "running", stage: "select", transitioned_at: "2025-12-20T10:02:30Z", reason: null, actor: "system" },
				{ id: 10, status: "completed", stage: "select", transitioned_at: "2025-12-20T10:03:00Z", reason: null, actor: "system" },
				{ id: 11, status: "running", stage: "evidence", transitioned_at: "2025-12-20T10:03:00Z", reason: null, actor: "system" },
				{ id: 12, status: "completed", stage: "evidence", transitioned_at: "2025-12-20T10:04:00Z", reason: null, actor: "system" },
				{ id: 13, status: "running", stage: "dispatch", transitioned_at: "2025-12-20T10:04:00Z", reason: null, actor: "system" },
				{ id: 14, status: "completed", stage: "dispatch", transitioned_at: "2025-12-20T10:05:00Z", reason: null, actor: "system" },
				{ id: 15, status: "running", stage: "persist", transitioned_at: "2025-12-20T10:05:00Z", reason: null, actor: "system" },
				{ id: 16, status: "completed", stage: "persist", transitioned_at: "2025-12-20T10:05:30Z", reason: null, actor: "system" },
			],
		},
		{
			job_id: "job-002-test-def456",
			status: "failed",
			last_stage: "genre",
			kicked_at: "2025-12-20T08:00:00Z",
			updated_at: "2025-12-20T08:02:15Z",
			duration_secs: 135,
			trigger_source: "user",
			user_id: "user-123",
			status_history: [
				{ id: 1, status: "running", stage: "fetch", transitioned_at: "2025-12-20T08:00:00Z", reason: null, actor: "system" },
				{ id: 2, status: "completed", stage: "fetch", transitioned_at: "2025-12-20T08:00:30Z", reason: null, actor: "system" },
				{ id: 3, status: "running", stage: "preprocess", transitioned_at: "2025-12-20T08:00:30Z", reason: null, actor: "system" },
				{ id: 4, status: "completed", stage: "preprocess", transitioned_at: "2025-12-20T08:01:15Z", reason: null, actor: "system" },
				{ id: 5, status: "running", stage: "genre", transitioned_at: "2025-12-20T08:01:15Z", reason: null, actor: "system" },
				{ id: 6, status: "failed", stage: "genre", transitioned_at: "2025-12-20T08:02:15Z", reason: "Genre classification timeout", actor: "system" },
			],
		},
	],
	stats: {
		success_rate_24h: 0.85,
		avg_duration_secs: 280,
		total_jobs_24h: 12,
		running_jobs: 0,
		failed_jobs_24h: 2,
	},
	user_context: {
		user_article_count: 12,
		user_jobs_count: 5,
		user_feed_ids: ["feed-1", "feed-2", "feed-3"],
	},
};

/**
 * Job progress response with an active running job
 */
export const JOB_PROGRESS_WITH_ACTIVE_JOB = {
	active_job: {
		job_id: "job-active-xyz789",
		status: "running",
		current_stage: "evidence",
		stage_index: 5, // evidence is the 6th stage (0-indexed: 5)
		stages_completed: ["fetch", "preprocess", "dedup", "genre", "select"],
		genre_progress: {
			tech: { status: "running", cluster_count: 5, article_count: 25 },
			business: { status: "pending", cluster_count: null, article_count: null },
		},
		total_articles: 150,
		user_article_count: 12,
		kicked_at: "2025-12-20T12:00:00Z",
		trigger_source: "system",
		sub_stage_progress: null,
	},
	recent_jobs: JOB_PROGRESS_RESPONSE.recent_jobs,
	stats: {
		success_rate_24h: 0.85,
		avg_duration_secs: 280,
		total_jobs_24h: 13,
		running_jobs: 1,
		failed_jobs_24h: 2,
	},
	user_context: {
		user_article_count: 12,
		user_jobs_count: 5,
		user_feed_ids: ["feed-1", "feed-2", "feed-3"],
	},
};

/**
 * Empty job progress response
 */
export const JOB_PROGRESS_EMPTY = {
	active_job: null,
	recent_jobs: [],
	stats: {
		success_rate_24h: 0,
		avg_duration_secs: null,
		total_jobs_24h: 0,
		running_jobs: 0,
		failed_jobs_24h: 0,
	},
	user_context: null,
};

/**
 * API paths for job dashboard
 */
export const JOB_DASHBOARD_PATHS = {
	jobProgress: "**/api/v1/dashboard/job-progress*",
	jobStats: "**/api/v1/dashboard/job-stats",
	triggerJob: "**/api/v1/generate/recaps/7days",
};
