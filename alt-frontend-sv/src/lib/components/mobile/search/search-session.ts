import type { SearchFeedItem } from "$lib/schema/search";

/**
 * Everything a phone search is made of, held by the route rather than by the
 * component that draws it.
 *
 * `SearchFeedsClient` used to own all of this as its own `$state`, and it lives
 * inside `{#if isDesktop()}`. Rotating the phone flipped that branch, destroyed
 * the component, and took the reader's query, their results and their offset
 * with it — they came back to an empty search box with no way to tell what they
 * had been looking for.
 *
 * Search is deliberately NOT folded into `FeedGrid` the way the other feed
 * routes were: it reads a different RPC (offset cursors, `SearchFeedItem`
 * rather than cursor-paginated `RenderFeed`), validates its input, and reports
 * a search time. There is no shared grid to hoist — so the same idea is applied
 * to the state instead of to the markup, and the `{#if}` keeps only the
 * drawing.
 */
export interface MobileSearchSession {
	results: SearchFeedItem[];
	isLoading: boolean;
	searchTime: number | undefined;
	cursor: string | null;
	hasMore: boolean;
	/**
	 * Set once the phone search has been on screen. It gates the two things
	 * that must happen on arrival and never again: running the `?q=` search
	 * automatically, and scrolling to the top. Re-running either on a remount
	 * would spend a request the reader did not ask for and throw away their
	 * scroll position on every rotation.
	 */
	visited: boolean;
}

export function createMobileSearchSession(): MobileSearchSession {
	return {
		results: [],
		isLoading: false,
		searchTime: undefined,
		cursor: null,
		hasMore: false,
		visited: false,
	};
}
