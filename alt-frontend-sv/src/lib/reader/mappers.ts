import type { TagTrailArticle } from "#lib/connect/index.js";
import type { RenderFeed } from "#lib/schema/feed.js";
import type { SearchFeedItem } from "#lib/schema/search.js";
import type { ReaderArticleItem } from "./types.js";

export function renderFeedToReaderItem(feed: RenderFeed): ReaderArticleItem {
	return {
		articleId: feed.articleId ?? null,
		url: feed.normalizedUrl || feed.link,
		title: feed.title ?? null,
		author: feed.author ?? null,
		publishedAt: feed.publishedAtFormatted || feed.published || null,
		tagsLabel: feed.mergedTagsLabel || null,
		fallbackDescription: feed.description || null,
		excerpt: feed.excerpt || null,
	};
}

export function searchFeedItemToReaderItem(
	item: SearchFeedItem,
): ReaderArticleItem {
	return {
		articleId: item.article_id ?? null,
		url: item.link,
		title: item.title ?? null,
		author: item.author?.name || item.authors?.[0]?.name || null,
		publishedAt: item.published ?? null,
		tagsLabel: item.tags?.join(" · ") || null,
		fallbackDescription: item.description || null,
		excerpt: item.description || null,
	};
}

export function tagTrailArticleToReaderItem(
	article: TagTrailArticle,
): ReaderArticleItem {
	return {
		articleId: article.id || null,
		url: article.link,
		title: article.title ?? null,
		author: article.feedTitle ?? null,
		publishedAt: article.publishedAt ?? null,
	};
}
