/**
 * Recap Mock Data
 */

import type { RecapResponse, RecapGenre } from "../types";

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
