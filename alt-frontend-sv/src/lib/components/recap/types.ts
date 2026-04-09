import type { GlobalRecapHitData } from "$lib/connect/global_search";
import type { RecapSearchResultItem } from "$lib/connect";

export interface RecapModalData {
	genre: string;
	summary: string;
	topTerms: string[];
	windowDays: number;
	executedAt: string;
	bullets?: string[];
	tags?: string[];
	jobId?: string;
}

export function fromGlobalRecapHit(hit: GlobalRecapHitData): RecapModalData {
	return {
		genre: hit.genre,
		summary: hit.summary,
		topTerms: hit.topTerms,
		windowDays: hit.windowDays,
		executedAt: hit.executedAt,
		jobId: hit.jobId,
		tags: hit.tags,
	};
}

export function fromRecapSearchResult(
	item: RecapSearchResultItem,
): RecapModalData {
	return {
		genre: item.genre,
		summary: item.summary,
		topTerms: item.topTerms,
		windowDays: item.windowDays,
		executedAt: item.executedAt,
		jobId: item.jobId,
		bullets: item.bullets,
	};
}
