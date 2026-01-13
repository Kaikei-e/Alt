import type { TrendDataResponse, TimeWindow } from "$lib/schema/stats";
import { callClientAPI } from "./core";

/**
 * Fetches trend statistics for the given time window.
 *
 * @param window - Time window: "4h", "24h", "3d", or "7d"
 * @returns Trend data with data points, granularity, and window info
 */
export async function getTrendStats(
	window: TimeWindow = "24h",
): Promise<TrendDataResponse> {
	const endpoint = `/v1/feeds/stats/trends?window=${window}`;
	return callClientAPI<TrendDataResponse>(endpoint);
}
