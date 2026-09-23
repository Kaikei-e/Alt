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
 * Connect-RPC format for v2 AugurService/StreamChat endpoint
 */
export const AUGUR_CONNECT_MESSAGES = [
	{
		result: {
			kind: "delta",
			payload: { case: "delta", value: "Based on your recent feeds, " },
		},
	},
	{
		result: {
			kind: "delta",
			payload: { case: "delta", value: "here are the key trends: " },
		},
	},
	{
		result: {
			kind: "delta",
			payload: { case: "delta", value: "AI development is accelerating." },
		},
	},
	{
		result: {
			kind: "done",
			payload: {
				case: "done",
				value: {
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
		},
	},
	{ result: {} },
];

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
