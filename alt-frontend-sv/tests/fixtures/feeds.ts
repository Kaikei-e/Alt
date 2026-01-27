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

export function createRenderFeed(id: string, url: string): RenderFeed {
	return {
		id,
		title: `Feed ${id}`,
		description: `Description for feed ${id}`,
		link: url,
		published: "2025-12-22T14:00:00Z",
		created_at: "2025-12-22T13:00:00Z",
		author: "Test Author",
		publishedAtFormatted: "Dec 22, 2025",
		mergedTagsLabel: "Test / Tag",
		normalizedUrl: url,
		excerpt: `Excerpt for feed ${id}`,
	};
}

export const renderFeedsFixture: RenderFeed[] = [
	createRenderFeed("feed-1", "https://example.com/feed-1"),
	createRenderFeed("feed-2", "https://example.com/feed-2"),
	createRenderFeed("feed-3", "https://example.com/feed-3"),
	createRenderFeed("feed-4", "https://example.com/feed-4"),
	createRenderFeed("feed-5", "https://example.com/feed-5"),
];
