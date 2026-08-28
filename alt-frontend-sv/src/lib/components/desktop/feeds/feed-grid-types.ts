import type { RenderFeed } from "$lib/schema/feed";

export type RemoveFeedResult = {
	nextFeedUrl: string | null;
	totalCount: number;
};

export type FeedGridApi = {
	/** Synchronously removes a feed and returns navigation info */
	removeFeedByUrl: (url: string) => RemoveFeedResult;
	/**
	 * Undoes a `removeFeedByUrl`, putting the feed back where it was.
	 *
	 * Removal is optimistic — the card leaves the screen before the server has
	 * agreed — so there has to be a way back when the server refuses. Without
	 * one, the only rollback available was a full refresh, which throws away
	 * every page the reader had scrolled to in order to undo one tap.
	 */
	restoreFeedByUrl: (url: string) => void;
	/** Get all currently visible feeds */
	getVisibleFeeds: () => RenderFeed[];
	/** Get a specific feed by URL */
	getFeedByUrl: (url: string) => RenderFeed | null;
	/** Fetch a replacement feed in the background (fire-and-forget) */
	fetchReplacementFeed: () => void;
	/** Refresh the feed list (for connection recovery after Safari idle) */
	refresh: () => void;
};
