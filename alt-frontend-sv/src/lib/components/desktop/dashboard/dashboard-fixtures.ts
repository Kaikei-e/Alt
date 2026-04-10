import type { RenderFeed } from "$lib/schema/feed";
import type { RecapSummary } from "$lib/schema/recap";

export const MOCK_FEEDS: RenderFeed[] = [
	{
		id: "feed-001",
		title: "TSMC Expands 3nm Capacity to Meet AI Chip Demand",
		description:
			"Taiwan Semiconductor Manufacturing Co. announced a 40% expansion.",
		link: "https://example.com/tsmc-3nm",
		published: "2026-04-10T08:30:00Z",
		author: "Reuters",
		articleId: "art-001",
		publishedAtFormatted: "Apr 10",
		mergedTagsLabel: "semiconductors, AI",
		normalizedUrl: "example.com/tsmc-3nm",
		excerpt:
			"Taiwan Semiconductor Manufacturing Co. announced a 40% expansion of its 3nm production capacity.",
	},
	{
		id: "feed-002",
		title: "OpenAI Releases GPT-5 with Native Tool Use",
		description: "The latest model introduces built-in agentic capabilities.",
		link: "https://example.com/gpt5",
		published: "2026-04-10T07:15:00Z",
		author: "TechCrunch",
		articleId: "art-002",
		publishedAtFormatted: "Apr 10",
		mergedTagsLabel: "LLM, AI",
		normalizedUrl: "example.com/gpt5",
		excerpt:
			"The latest model introduces built-in agentic capabilities and multi-step reasoning.",
	},
	{
		id: "feed-003",
		title: "EU Passes Comprehensive AI Regulation Framework",
		description: "New rules set strict requirements for high-risk AI systems.",
		link: "https://example.com/eu-ai-act",
		published: "2026-04-09T18:00:00Z",
		author: "Ars Technica",
		articleId: "art-003",
		publishedAtFormatted: "Apr 9",
		mergedTagsLabel: "regulation, policy",
		normalizedUrl: "example.com/eu-ai-act",
		excerpt:
			"New rules set strict requirements for high-risk AI systems deployed in the European Union.",
	},
];

export const MOCK_RECAP: RecapSummary = {
	jobId: "recap-001",
	executedAt: "2026-04-10T06:00:00Z",
	windowStart: "2026-04-07T00:00:00Z",
	windowEnd: "2026-04-10T00:00:00Z",
	totalArticles: 142,
	genres: [
		{
			genre: "Technology",
			summary:
				"AI infrastructure spending continues to accelerate with major chip manufacturers expanding capacity.",
			topTerms: ["GPU", "inference", "edge computing"],
			articleCount: 45,
			clusterCount: 8,
			evidenceLinks: [],
			bullets: [],
		},
		{
			genre: "Policy & Regulation",
			summary:
				"Multiple jurisdictions advance AI governance frameworks with focus on high-risk applications.",
			topTerms: ["EU AI Act", "compliance", "risk assessment"],
			articleCount: 28,
			clusterCount: 5,
			evidenceLinks: [],
			bullets: [],
		},
		{
			genre: "Research",
			summary:
				"New architectures challenge transformer dominance with promising efficiency gains.",
			topTerms: ["state space models", "mixture of experts", "quantization"],
			articleCount: 19,
			clusterCount: 4,
			evidenceLinks: [],
			bullets: [],
		},
	],
};
