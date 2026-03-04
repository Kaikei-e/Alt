/**
 * Factory for tag trail mock data.
 */

let trailCounter = 0;

export interface TagTrailFeed {
	id: string;
	url: string;
	title: string;
	description: string;
}

export interface TagTrailArticle {
	articleId: string;
	title: string;
	description: string;
	link: string;
	published: string;
	author: string;
}

export function buildTagTrailFeed(
	overrides: Partial<TagTrailFeed> = {},
): TagTrailFeed {
	trailCounter++;
	return {
		id: `trail-feed-${trailCounter}`,
		url: `https://example.com/trail-feed-${trailCounter}`,
		title: `Trail Feed ${trailCounter}`,
		description: `A random feed for tag trail ${trailCounter}`,
		...overrides,
	};
}

export function buildTagTrailArticle(
	overrides: Partial<TagTrailArticle> = {},
): TagTrailArticle {
	trailCounter++;
	return {
		articleId: `trail-article-${trailCounter}`,
		title: `Trail Article ${trailCounter}`,
		description: `Description for trail article ${trailCounter}`,
		link: `https://example.com/trail-article-${trailCounter}`,
		published: "2 hours ago",
		author: "Trail Author",
		...overrides,
	};
}

export function buildArticlesByTagResponse(
	articles?: TagTrailArticle[],
	hasMore = false,
) {
	return {
		articles: articles ?? [
			buildTagTrailArticle({ title: "AI Trends in 2026" }),
			buildTagTrailArticle({ title: "Machine Learning Basics" }),
		],
		hasMore,
		nextCursor: hasMore ? "cursor-123" : "",
	};
}

export function buildTagStreamMessages(
	tags: string[] = ["AI", "Machine Learning", "Technology"],
) {
	return tags.map((tag) => ({
		kind: "tag" as const,
		tag,
	}));
}

export function resetTrailCounter(): void {
	trailCounter = 0;
}
