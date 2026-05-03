import type { RenderFeed } from "$lib/schema/feed";

export type RemoveFeedResult = {
	nextFeedUrl: string | null;
	totalCount: number;
};

export type FeedGridApi = {
	/** Synchronously removes a feed and returns navigation info */
	removeFeedByUrl: (url: string) => RemoveFeedResult;
	/** Get all currently visible feeds */
	getVisibleFeeds: () => RenderFeed[];
	/** Get a specific feed by URL */
	getFeedByUrl: (url: string) => RenderFeed | null;
	/** Fetch a replacement feed in the background (fire-and-forget) */
	fetchReplacementFeed: () => void;
	/** Refresh the feed list (for connection recovery after Safari idle) */
	refresh: () => void;
};
