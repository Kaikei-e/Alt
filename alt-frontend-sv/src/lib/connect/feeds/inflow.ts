/**
 * Feed inflow statistics and daily cap management via Connect-RPC
 */

import type { Transport } from "@connectrpc/connect";
import type {
	GetFeedInflowStatsResponse,
	FeedInflowStat as ProtoFeedInflowStat,
	SetSubscriptionDailyCapResponse,
} from "#lib/gen/alt/feeds/v2/feeds_pb.js";
import { createFeedClient } from "./client";

// Match actions.ts so unary calls fail fast against a stalled BFF/backend.
const UNARY_FEED_TIMEOUT_MS = 5000;

/**
 * 30-day inflow and reading statistics for one subscribed feed.
 */
export interface FeedInflowStatItem {
	feedLinkId: string;
	url: string;
	dailyCap?: number;
	inflowCount: number;
	readCount: number;
	openedCountLegacy: number;
	readRate: number;
}

/**
 * Per-subscribed-feed inflow stats response.
 */
export interface FeedInflowStatsResult {
	stats: FeedInflowStatItem[];
	windowDays: number;
}

/**
 * Result of setting or clearing a subscription daily cap.
 */
export interface SetSubscriptionDailyCapResult {
	message: string;
	dailyCap?: number;
}

/**
 * Fetches 30-day inflow and reading statistics per subscribed feed.
 */
export async function getFeedInflowStats(
	transport: Transport,
): Promise<FeedInflowStatsResult> {
	const client = createFeedClient(transport);
	const response = (await client.getFeedInflowStats(
		{},
		{ timeoutMs: UNARY_FEED_TIMEOUT_MS },
	)) as GetFeedInflowStatsResponse;

	return {
		stats: (response.stats ?? []).map((stat: ProtoFeedInflowStat) => ({
			feedLinkId: stat.feedLinkId,
			url: stat.url,
			dailyCap: stat.dailyCap ?? undefined,
			inflowCount: Number(stat.inflowCount ?? 0),
			readCount: Number(stat.readCount ?? 0),
			openedCountLegacy: Number(stat.openedCountLegacy ?? 0),
			readRate: Number(stat.readRate ?? 0),
		})),
		windowDays: response.windowDays ?? 30,
	};
}

/**
 * Sets a positive daily unread cap on one subscription, or clears it when dailyCap is omitted.
 */
export async function setSubscriptionDailyCap(
	transport: Transport,
	feedLinkId: string,
	dailyCap?: number,
): Promise<SetSubscriptionDailyCapResult> {
	const client = createFeedClient(transport);
	const response = (await client.setSubscriptionDailyCap(
		dailyCap !== undefined ? { feedLinkId, dailyCap } : { feedLinkId },
		{ timeoutMs: UNARY_FEED_TIMEOUT_MS },
	)) as SetSubscriptionDailyCapResponse;

	return {
		message: response.message,
		dailyCap: response.dailyCap ?? undefined,
	};
}
