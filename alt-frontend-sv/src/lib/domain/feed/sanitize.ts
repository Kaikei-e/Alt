import type { BackendFeedItem, SanitizedFeed, RenderFeed } from "./types";
import {
	formatPublishedDate,
	generateExcerptFromDescription,
	mergeTagsLabel,
	normalizeUrl,
} from "$lib/utils/feed";
import { sanitizeHrefUrl } from "$lib/utils/urlSafety";

function sanitizeUrl(url: string): string {
	return sanitizeHrefUrl(url) ?? "";
}

/** Decode common HTML entities. SSR-safe (no DOMParser). */
export function decodeHtmlEntities(text: string): string {
	return text
		.replace(/&lt;/g, "<")
		.replace(/&gt;/g, ">")
		.replace(/&quot;/g, '"')
		.replace(/&#0?39;/g, "'")
		.replace(/&apos;/g, "'")
		.replace(/&#x27;/g, "'")
		.replace(/&amp;/g, "&");
}

/**
 * Reduce a feed's HTML blob to the text a reader can actually read.
 *
 * Tags are stripped BEFORE entities are decoded, never after: decoding first
 * would turn an escaped `&lt;script&gt;` into real markup that the strip pass
 * has already gone by. No length cap here — callers that need one apply it
 * themselves, so a surface with its own "read more" cut is not silently
 * shortened twice.
 */
export function stripHtmlToText(content: string | null | undefined): string {
	if (!content) return "";
	const textOnly = content
		.replace(/<[^>]*>/g, " ")
		.replace(/\s+/g, " ")
		.trim();
	return decodeHtmlEntities(textOnly);
}

function sanitizeContent(
	content: string | null | undefined,
	maxLength = 1000,
): string {
	const decoded = stripHtmlToText(content);
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
		id: rawFeed.article_id || rawFeed.link || "",
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
