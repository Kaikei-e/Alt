import type { TagCloudItem } from "#lib/connect/index.js";

export function sortTagsForIndex(
	tags: readonly TagCloudItem[],
): TagCloudItem[] {
	return [...tags].sort((a, b) => {
		if (b.articleCount !== a.articleCount) {
			return b.articleCount - a.articleCount;
		}
		return a.tagName.localeCompare(b.tagName);
	});
}

export function filterTagsForIndex(
	tags: readonly TagCloudItem[],
	query: string,
): TagCloudItem[] {
	const sorted = sortTagsForIndex(tags);
	const normalized = query.trim().toLowerCase();
	if (!normalized) {
		return sorted;
	}
	return sorted.filter((tag) => tag.tagName.toLowerCase().includes(normalized));
}

export function sumTagArticleCounts(tags: readonly TagCloudItem[]): number {
	let sum = 0;
	for (const tag of tags) {
		sum += tag.articleCount;
	}
	return sum;
}

export function formatCountLabel(
	count: number,
	singular: string,
	plural = `${singular}s`,
): string {
	return `${count} ${count === 1 ? singular : plural}`;
}
