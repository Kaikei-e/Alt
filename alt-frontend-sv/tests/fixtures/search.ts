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
