import type { ThemeSourceItem } from "#lib/connect/reading.js";
import type { ReaderArticleItem } from "#lib/reader/types.js";

export interface ThemeSourceMarker {
	number: number;
	sourceIndex: number;
	source: ThemeSourceItem;
}

export function formatCompactDate(date: Date | null | undefined): string {
	if (!date || Number.isNaN(date.getTime())) return "";
	return date.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
	});
}

export function formatCalendarDate(raw: string | null | undefined): string {
	if (!raw) return "";
	const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw.trim());
	if (!match) return "";
	const year = Number(match[1]);
	const month = Number(match[2]);
	const day = Number(match[3]);
	// Anchor at UTC and format in UTC so negative-offset timezones never shift the day.
	const date = new Date(Date.UTC(year, month - 1, day));
	if (
		date.getUTCFullYear() !== year ||
		date.getUTCMonth() !== month - 1 ||
		date.getUTCDate() !== day
	) {
		return "";
	}
	return date.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
		timeZone: "UTC",
	});
}

export function formatArticleCount(count: number): string {
	return count === 1 ? "1 article" : `${count} articles`;
}

export function formatSourceCount(count: number): string {
	return count === 1 ? "1 source" : `${count} sources`;
}

export function formatThemeListMeta(
	articleCount: number,
	updatedAt: Date | null | undefined,
): string {
	const countPart = formatArticleCount(articleCount);
	const datePart = formatCompactDate(updatedAt);
	return datePart ? `${countPart} · updated ${datePart}` : countPart;
}

export function formatThemeStatusLabel(
	lastOpenedVersion: number,
	latestVersion: number,
): "new" | "updated since you read it" | null {
	if (lastOpenedVersion === 0) return "new";
	if (lastOpenedVersion > 0 && lastOpenedVersion < latestVersion) {
		return "updated since you read it";
	}
	return null;
}

export function formatThemePageMeta(
	version: number,
	latestVersion: number,
	createdAt: Date | null | undefined,
	sourceCount: number,
): string {
	const parts = [`Version ${version} of ${latestVersion}`];
	const datePart = formatCompactDate(createdAt);
	if (datePart) {
		parts.push(datePart);
	}
	parts.push(formatSourceCount(sourceCount));
	return parts.join(" · ");
}

export function formatThemeVersionRow(
	version: number,
	createdAt: Date | null | undefined,
	articleCount: number,
): string {
	const parts = [`Version ${version}`];
	const datePart = formatCompactDate(createdAt);
	if (datePart) {
		parts.push(datePart);
	}
	parts.push(formatArticleCount(articleCount));
	return parts.join(" · ");
}

export function sourceHostOf(url: string | null | undefined): string {
	if (!url) return "";
	try {
		return new URL(url).host;
	} catch {
		return "";
	}
}

export function sourceTitleOf(
	title: string | null | undefined,
	url: string | null | undefined,
): string {
	const trimmed = title?.trim();
	if (trimmed) return trimmed;
	const host = sourceHostOf(url);
	if (host) return host;
	const trimmedUrl = url?.trim();
	return trimmedUrl || "Untitled source";
}

export function resolveSourceMarkers(
	sourceArticleIds: readonly string[],
	sources: readonly ThemeSourceItem[],
): ThemeSourceMarker[] {
	const sourceById = new Map<
		string,
		{ sourceIndex: number; source: ThemeSourceItem }
	>();
	for (const [index, source] of sources.entries()) {
		if (!sourceById.has(source.articleId)) {
			sourceById.set(source.articleId, { sourceIndex: index, source });
		}
	}

	const markers: ThemeSourceMarker[] = [];
	const seenIds = new Set<string>();
	for (const articleId of sourceArticleIds) {
		if (seenIds.has(articleId)) continue;
		const match = sourceById.get(articleId);
		if (!match) continue;
		seenIds.add(articleId);
		markers.push({
			number: match.sourceIndex + 1,
			sourceIndex: match.sourceIndex,
			source: match.source,
		});
	}
	return markers;
}

export function toThemeReaderItems(
	sources: readonly ThemeSourceItem[],
): ReaderArticleItem[] {
	return sources.map((source) => ({
		articleId: source.articleId,
		url: source.url,
		title: sourceTitleOf(source.title, source.url),
		author: sourceHostOf(source.url) || null,
	}));
}

export function parseVersionParam(raw: string | null): number | undefined {
	if (!raw || !/^[1-9]\d*$/.test(raw)) return undefined;
	const parsed = Number(raw);
	return Number.isSafeInteger(parsed) && parsed > 0 ? parsed : undefined;
}
