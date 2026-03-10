import type { FeedLink, FeedHealthStatus } from "$lib/schema/feedLink";

export function classifyFeedHealth(feed: FeedLink): FeedHealthStatus {
	return feed.healthStatus;
}

export function getHealthColor(status: FeedHealthStatus): string {
	switch (status) {
		case "healthy":
			return "#22c55e";
		case "warning":
			return "#f59e0b";
		case "error":
			return "#ef4444";
		case "inactive":
			return "#9ca3af";
		case "unknown":
			return "#6b7280";
	}
}

export function getHealthLabel(status: FeedHealthStatus): string {
	switch (status) {
		case "healthy":
			return "Healthy";
		case "warning":
			return "Warning";
		case "error":
			return "Error";
		case "inactive":
			return "Inactive";
		case "unknown":
			return "Unknown";
	}
}

export function summarizeHealth(
	feeds: FeedLink[],
): Record<FeedHealthStatus, number> {
	const counts: Record<FeedHealthStatus, number> = {
		healthy: 0,
		warning: 0,
		error: 0,
		inactive: 0,
		unknown: 0,
	};
	for (const feed of feeds) {
		counts[feed.healthStatus]++;
	}
	return counts;
}
