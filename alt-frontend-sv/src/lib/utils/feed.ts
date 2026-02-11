/**
 * Server-side feed formatting utilities for LCP optimization
 * These functions generate render-ready values on the server to reduce client-side processing
 */

import { decodeHtmlEntities } from "$lib/domain/feed/sanitize";

/**
 * Formats a date string to a human-readable format with time
 * @param dateString - ISO date string or RFC3339 format
 * @returns Formatted date string (e.g., "Nov 23, 2025, 2:30 PM")
 */
export function formatPublishedDate(
	dateString: string | undefined | null,
): string {
	if (!dateString) {
		return "";
	}

	try {
		const date = new Date(dateString);
		if (Number.isNaN(date.getTime())) {
			return "";
		}

		// Format as "Nov 23, 2025, 2:30 PM"
		return date.toLocaleString("en-US", {
			year: "numeric",
			month: "short",
			day: "numeric",
			hour: "numeric",
			minute: "2-digit",
		});
	} catch {
		return "";
	}
}

/**
 * Merges tags array into a display label
 * @param tags - Array of tag strings
 * @returns Merged label (e.g., "Next.js / Performance")
 */
export function mergeTagsLabel(tags: string[] | undefined | null): string {
	if (!tags || tags.length === 0) {
		return "";
	}

	return tags.join(" / ");
}

/**
 * Normalizes URL by removing tracking parameters and cleaning up the URL
 * @param url - Original URL
 * @returns Normalized URL
 */
export function normalizeUrl(url: string | undefined | null): string {
	if (!url) {
		return "";
	}

	try {
		const parsed = new URL(url);
		// Remove fragment (hash)
		parsed.hash = "";

		// Remove tracking parameters
		const trackingParams = [
			"utm_source",
			"utm_medium",
			"utm_campaign",
			"utm_term",
			"utm_content",
			"utm_id",
			"fbclid",
			"gclid",
			"mc_eid",
			"msclkid",
		];
		for (const param of trackingParams) {
			parsed.searchParams.delete(param);
		}

		// Remove trailing slash (except for root path)
		if (parsed.pathname !== "/" && parsed.pathname.endsWith("/")) {
			parsed.pathname = parsed.pathname.slice(0, -1);
		}

		return parsed.toString();
	} catch {
		// If URL parsing fails, return original URL
		return url;
	}
}

/**
 * Generates an excerpt from description text (fallback when HTML content is not available)
 * @param description - Plain text description
 * @param maxLength - Maximum length of excerpt (default: 160)
 * @returns Truncated description
 */
export function generateExcerptFromDescription(
	description: string | undefined | null,
	maxLength: number = 160,
): string {
	if (!description) {
		return "";
	}

	// Remove HTML tags and decode entities
	const textOnly = decodeHtmlEntities(
		description
			.replace(/<[^>]*>/g, " ")
			.replace(/\s+/g, " ")
			.trim(),
	);

	if (textOnly.length <= maxLength) {
		return textOnly;
	}

	// Truncate at word boundary
	const truncated = textOnly.slice(0, maxLength);
	const lastSpace = truncated.lastIndexOf(" ");

	if (lastSpace > maxLength * 0.8) {
		return `${truncated.slice(0, lastSpace)}...`;
	}

	return `${truncated}...`;
}

/**
 * Canonicalize URL (same as normalizeUrl but with different name for compatibility)
 */
export function canonicalize(url: string): string {
	return normalizeUrl(url);
}
