import type { BackendFeedItem, SanitizedFeed, RenderFeed } from "./types";
import {
	formatPublishedDate,
	generateExcerptFromDescription,
	mergeTagsLabel,
	normalizeUrl,
} from "$lib/utils/feed";

function sanitizeUrl(url: string): string {
	if (!url) return "";
	const urlPattern = /^https?:\/\//i;
	if (!urlPattern.test(url)) return "";
	const dangerousProtocols = /^(javascript|vbscript|data|ftp|file):/i;
	if (dangerousProtocols.test(url)) return "";
	return url;
}

/** Decode common HTML entities. SSR-safe (no DOMParser). */
export function decodeHtmlEntities(text: string): string {
	return text
		.replace(/&amp;/g, "&")
		.replace(/&lt;/g, "<")
		.replace(/&gt;/g, ">")
		.replace(/&quot;/g, '"')
		.replace(/&#0?39;/g, "'")
		.replace(/&apos;/g, "'")
		.replace(/&#x27;/g, "'");
}

function sanitizeContent(
	content: string | null | undefined,
	maxLength = 1000,
): string {
	if (!content) return "";
	const textOnly = content
		.replace(/<[^>]*>/g, " ")
		.replace(/\s+/g, " ")
		.trim();
	const decoded = decodeHtmlEntities(textOnly);
	return decoded.length > maxLength ? decoded.slice(0, maxLength) : decoded;
}

export function sanitizeFeed(rawFeed: BackendFeedItem): SanitizedFeed {
	const sanitized = {
		title: sanitizeContent(rawFeed.title || "", 200),
		description: sanitizeContent(rawFeed.description || "", 500),
		author: sanitizeContent(
			rawFeed.author?.name || rawFeed.authors?.[0]?.name || "",
			100,
		),
		link: sanitizeUrl(rawFeed.link || ""),
	};

	return {
		id: rawFeed.link || "",
		title: sanitized.title,
		description: sanitized.description,
		link: sanitized.link,
		published: rawFeed.published || "",
		created_at: rawFeed.created_at,
		author: sanitized.author || undefined,
		articleId: rawFeed.article_id || undefined,
	};
}

export function toRenderFeed(feed: SanitizedFeed, tags?: string[]): RenderFeed {
	return {
		...feed,
		publishedAtFormatted: formatPublishedDate(
			feed.published || feed.created_at,
		),
		mergedTagsLabel: mergeTagsLabel(tags),
		normalizedUrl: normalizeUrl(feed.link),
		excerpt: generateExcerptFromDescription(feed.description),
	};
}
