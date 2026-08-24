import type { SearchFeedItem } from "$lib/schema/search";

export const searchResultFixture: SearchFeedItem = {
	title: "Svelte 5 Runes Deep Dive",
	description:
		"A comprehensive look at the new Runes reactivity system in Svelte 5, covering $state, $derived, and $effect patterns for modern component development.",
	link: "https://example.com/svelte-5-runes",
	published: "2025-12-20T10:00:00Z",
	author: { name: "Svelte Team" },
	tags: ["svelte", "javascript"],
};

export const searchResultNoDescFixture: SearchFeedItem = {
	title: "Quick Update",
	description: "",
	link: "https://example.com/quick-update",
	published: "2025-12-19T10:00:00Z",
};

export function createSearchFeedItem(
	id: string,
	title: string,
): SearchFeedItem {
	return {
		title,
		description: `Description for ${title}`,
		link: `https://example.com/${id}`,
		published: "2025-12-22T14:00:00Z",
		author: { name: "Test Author" },
		tags: ["test"],
	};
}

export const searchResultsFixture: SearchFeedItem[] = [
	createSearchFeedItem("1", "First Article"),
	createSearchFeedItem("2", "Second Article"),
	createSearchFeedItem("3", "Third Article"),
];

/**
 * A hit as `SearchResultItem` actually receives it — markup already stripped by
 * `transformFeedSearchResult` — whose remaining text still carries a bare CDN
 * URL. Stripping tags does not create a break opportunity inside the URL, so
 * this is the shape that runs out of the card under `overflow-wrap: normal`.
 */
export const searchResultLongUrlFixture: SearchFeedItem = {
	title: "Best of MWC 2026: Live updates on phones, concepts, and innovations",
	description:
		"MWC 2026 gallery: https://www.zdnet.com/a/img/resize/a380d377116335d01087bcb191f4613da7010344/2026/02/28/77bc4efe-edc8-4a95-8fc0-d630a7f6f1c9/dsc09454.jpg?auto=webp&fit=crop&height=675&width=1200 plus everything else from the show floor.",
	link: "https://www.zdnet.com/article/best-of-mwc-2026-live-updates",
	published: "2026-02-28T10:00:00Z",
	author: { name: "ZDNET" },
	tags: ["mwc", "phones"],
};
