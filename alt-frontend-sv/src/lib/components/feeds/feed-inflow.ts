import type {
	ConnectFeedSource,
	FeedInflowStatItem,
} from "#lib/connect/feeds.js";
import type { FeedLink } from "#lib/schema/feedLink.js";

export type FeedInflowSortKey = "most-inflow" | "lowest-read-rate" | "name";

export const DAILY_CAP_MIN = 1;
export const DAILY_CAP_MAX = 500;

/**
 * Resolves the feed display label by joining against the page's feed links on feedLinkId.
 */
export function resolveFeedDisplayName(
	stat: Pick<FeedInflowStatItem, "feedLinkId" | "url">,
	feedLinksById: ReadonlyMap<string, FeedLink>,
	subscriptionsById?: ReadonlyMap<string, ConnectFeedSource>,
): string {
	return (
		feedLinksById.get(stat.feedLinkId)?.url ||
		stat.url ||
		subscriptionsById?.get(stat.feedLinkId)?.url ||
		stat.feedLinkId
	);
}

export interface FeedUrlParts {
	host: string;
	pathAndQuery: string;
}

/**
 * Splits a feed URL into its hostname (without leading `www.`) and path+query.
 * Falls back to returning the unparsed string as `host` when invalid.
 */
export function parseFeedUrlParts(rawUrl: string): FeedUrlParts {
	try {
		const parsed = new URL(rawUrl);
		if (!parsed.hostname) {
			return { host: rawUrl, pathAndQuery: "" };
		}
		const stripped = parsed.hostname.replace(/^www\./i, "");
		return {
			host: stripped || parsed.hostname,
			pathAndQuery: `${parsed.pathname}${parsed.search}`,
		};
	} catch {
		return { host: rawUrl, pathAndQuery: "" };
	}
}

export function formatInflowCount(count: number): string {
	return Math.max(0, Math.trunc(count)).toLocaleString("en-US");
}

export function formatReadRate(inflowCount: number, readRate: number): string {
	if (inflowCount <= 0) return "–";
	return `${Math.round(readRate * 100)}%`;
}

export function formatDailyCap(dailyCap?: number): string {
	if (dailyCap === undefined || dailyCap === null) return "No cap";
	return `${dailyCap} per day`;
}

export function parseDailyCapInput(
	raw: string,
): { ok: true; value: number } | { ok: false; error: string } {
	const trimmed = raw.trim();
	if (!/^\d+$/.test(trimmed)) {
		return {
			ok: false,
			error: `Enter a whole number between ${DAILY_CAP_MIN} and ${DAILY_CAP_MAX}.`,
		};
	}
	const value = Number.parseInt(trimmed, 10);
	if (value < DAILY_CAP_MIN || value > DAILY_CAP_MAX) {
		return {
			ok: false,
			error: `Enter a whole number between ${DAILY_CAP_MIN} and ${DAILY_CAP_MAX}.`,
		};
	}
	return { ok: true, value };
}

/**
 * Sorts inflow stats client-side without mutating the input array.
 * For lowest-read-rate, feeds with zero inflow are placed last.
 */
export function sortFeedInflowStats(
	stats: readonly FeedInflowStatItem[],
	sortBy: FeedInflowSortKey,
	getDisplayName: (stat: FeedInflowStatItem) => string,
): FeedInflowStatItem[] {
	return [...stats].sort((a, b) => {
		if (sortBy === "name") {
			const nameA = getDisplayName(a);
			const nameB = getDisplayName(b);
			const hostCmp = parseFeedUrlParts(nameA).host.localeCompare(
				parseFeedUrlParts(nameB).host,
			);
			return hostCmp !== 0 ? hostCmp : nameA.localeCompare(nameB);
		}

		if (sortBy === "lowest-read-rate") {
			const aZero = a.inflowCount <= 0;
			const bZero = b.inflowCount <= 0;
			if (aZero !== bZero) return aZero ? 1 : -1;
			if (!aZero && a.readRate !== b.readRate) {
				return a.readRate - b.readRate;
			}
			if (a.inflowCount !== b.inflowCount) {
				return b.inflowCount - a.inflowCount;
			}
			return getDisplayName(a).localeCompare(getDisplayName(b));
		}

		// Default: most-inflow
		if (b.inflowCount !== a.inflowCount) {
			return b.inflowCount - a.inflowCount;
		}
		return getDisplayName(a).localeCompare(getDisplayName(b));
	});
}
