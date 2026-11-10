import type { TagCloudItem } from "#lib/connect/index.js";

export function sortTagsForIndex(
	_tags: readonly TagCloudItem[],
): TagCloudItem[] {
	return [];
}

export function filterTagsForIndex(
	_tags: readonly TagCloudItem[],
	_query: string,
): TagCloudItem[] {
	return [];
}

export function sumTagArticleCounts(_tags: readonly TagCloudItem[]): number {
	return -1;
}
