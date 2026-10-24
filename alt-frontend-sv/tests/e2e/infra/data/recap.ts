/**
 * Recap Mock Data
 */

import type { RecapGenre, RecapResponse } from "../types";

// =============================================================================
// Recap Data
// =============================================================================

export const MOCK_RECAP_GENRES: RecapGenre[] = [
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
	},
];

export const RECAP_RESPONSE: RecapResponse = {
	genres: MOCK_RECAP_GENRES,
};

// =============================================================================
// Connect-RPC v2 Recap Data (camelCase format)
// =============================================================================

/**
 * Connect-RPC format for GetSevenDayRecap response.
 * Used by tests mocking the alt.recap.v2.RecapService/GetSevenDayRecap endpoint.
 */
export const CONNECT_RECAP_RESPONSE = {
	jobId: "test-job-123",
	executedAt: "2025-12-20T12:00:00Z",
	windowStart: "2025-12-13T00:00:00Z",
	windowEnd: "2025-12-20T00:00:00Z",
	totalArticles: 3,
	genres: MOCK_RECAP_GENRES.map((g) => ({
		...g,
		references: [], // Connect-RPC requires this field
	})),
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
// Augur Streaming Data
// =============================================================================

/**
 * SSE format for REST v1 augur/chat endpoint
 */
export const AUGUR_SSE_CHUNKS = [
	"event: delta\ndata: Based on your recent feeds, \n\n",
	"event: delta\ndata: here are the key trends: \n\n",
	"event: delta\ndata: AI development is accelerating.\n\n",
	"event: done\ndata: {}\n\n",
];

/**
 * Connect-RPC format for v2 AugurService/StreamChat endpoint.
 * Uses protobuf JSON wire format where oneof fields use their field name directly.
 */
export const AUGUR_CONNECT_MESSAGES = [
	{ kind: "delta", delta: "Based on your recent feeds, " },
	{ kind: "delta", delta: "here are the key trends: " },
	{ kind: "delta", delta: "AI development is accelerating." },
	{
		kind: "done",
		done: {
			answer:
				"Based on your recent feeds, here are the key trends: AI development is accelerating.",
			citations: [
				{
					url: "https://example.com/ai",
					title: "AI News",
					publishedAt: "2025-12-20T10:00:00Z",
				},
			],
		},
	},
];

// =============================================================================
// Morning Letter Mock Data
// =============================================================================

export const CONNECT_MORNING_LETTER_DOCUMENT = {
	id: "letter-001",
	targetDate: "2026-04-07",
	editionTimezone: "Asia/Tokyo",
	isDegraded: false,
	schemaVersion: 1,
	generationRevision: 1,
	model: "gemma4-e4b",
	createdAt: "2026-04-07T06:00:00Z",
	etag: '"rev-1"',
	body: {
		lead: "Today's key developments across AI and web engineering.",
		generatedAt: "2026-04-07T06:00:00Z",
		sourceRecapWindowDays: 3,
		throughLine: "Inference efficiency and browser standards converge.",
		sections: [
			{
				key: "top3",
				title: "Top Stories",
				bullets: [
					"New reasoning models cut latency by 40%.",
					"WebGPU ships across major desktop browsers.",
				],
				genre: "technology",
				narrative: "Key breakthroughs in model serving and client runtimes.",
				whyReasons: [
					{ code: "subscription_update", tag: "AI" },
					{ code: "subscription_update", tag: "Web" },
				],
			},
		],
	},
};

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
			answer:
				"Based on the past 24 hours of news, here are the key developments: AI research has made significant progress.",
			citations: [
				{
					url: "https://example.com/ai-news",
					title: "AI Research Update",
					publishedAt: "2025-12-31T08:00:00Z",
				},
				{
					url: "https://example.com/tech-weekly",
					title: "Tech Weekly",
					publishedAt: "2025-12-31T06:00:00Z",
				},
			],
		},
	},
];

// =============================================================================
// Job Progress Dashboard Data (inlined for Dockerfile.mock-auth isolation)
// =============================================================================

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
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-12-20T10:00:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 2,
					status: "completed",
					stage: "fetch",
					transitioned_at: "2025-12-20T10:00:45Z",
					reason: null,
					actor: "system",
				},
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
				{
					id: 1,
					status: "running",
					stage: "fetch",
					transitioned_at: "2025-12-20T08:00:00Z",
					reason: null,
					actor: "system",
				},
				{
					id: 2,
					status: "failed",
					stage: "genre",
					transitioned_at: "2025-12-20T08:02:15Z",
					reason: "Genre classification timeout",
					actor: "system",
				},
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

export const CONNECT_RECAP_CARDS_RESPONSE = {
	job: {
		jobId: "11111111-2222-3333-4444-555555555555",
		kickedAt: "2026-09-22T17:00:00Z",
		from: "2026-09-19T17:00:00Z",
		to: "2026-09-22T17:00:00Z",
		paramsVersion: "cards-v0.2",
		cardsSelected: 2,
		degraded: false,
	},
	cards: [
		{
			id: "22222222-3333-4444-5555-666666666666",
			rank: 1,
			storyId: "33333333-4444-5555-6666-777777777777",
			headlineJa: "大規模推論モデルの新たな展開",
			whatJa:
				"最新の推論モデルが公開された。[1]ベンチマークで高い性能を示した。[2]",
			whyJa: "日本語処理の効率化が期待される。[1]",
			genre: "technology",
			sources: [
				{
					n: 1,
					feedId: "44444444-5555-6666-7777-888888888888",
					url: "https://example.com/ai-news",
					host: "example.com",
					title: "新モデル発表のニュース",
					pubDate: "2026-09-22T10:00:00Z",
				},
				{
					n: 2,
					feedId: "55555555-6666-7777-8888-999999999999",
					url: "https://example.jp/benchmark",
					host: "example.jp",
					title: "ベンチマーク結果の詳細",
					pubDate: "2026-09-22T11:00:00Z",
				},
			],
			createdAt: "2026-09-22T17:05:00Z",
		},
		{
			id: "66666666-7777-8888-9999-000000000000",
			rank: 2,
			storyId: "77777777-8888-9999-0000-111111111111",
			continuesCardId: "22222222-3333-4444-5555-666666666666",
			headlineJa: "Web標準仕様の更新動向",
			whatJa: "新たなWeb API策定に向けた合意が発表された。[1]",
			genre: "web",
			sources: [
				{
					n: 1,
					feedId: "88888888-9999-0000-1111-222222222222",
					url: "https://example.com/web-standards",
					host: "example.com",
					title: "新仕様ドラフト公開",
					pubDate: "2026-09-22T12:00:00Z",
				},
			],
			createdAt: "2026-09-22T17:06:00Z",
		},
	],
};
