import type { RenderFeed } from "$lib/schema/feed";

export const renderFeedFixture: RenderFeed = {
	id: "feed-1",
	title: "Daily AI Recap",
	description:
		"A concise summary of the most important AI research published today.",
	link: "https://alt.ai/research/daily-ai-recap",
	published: "2025-12-22T14:00:00Z",
	created_at: "2025-12-22T13:00:00Z",
	author: "Alt AI Team",
	publishedAtFormatted: "Dec 22, 2025",
	mergedTagsLabel: "AI / Research",
	normalizedUrl: "https://alt.ai/research/daily-ai-recap",
	excerpt:
		"A concise summary of the most important AI breakthroughs, ethics debates, and engineering tips from today's feeds.",
};
