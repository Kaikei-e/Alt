import type { ThreeDayRecapCardsResponse } from "$lib/connect";

export type CardsPageState =
	| "loading"
	| "error"
	| "empty"
	| "degraded"
	| "populated";

export interface CardsPageStateInput {
	isLoading: boolean;
	error: Error | null;
	data: ThreeDayRecapCardsResponse | null;
}

/**
 * Determines the current display state of the 3-day topic cards page.
 */
export function determineCardsPageState({
	isLoading,
	error,
	data,
}: CardsPageStateInput): CardsPageState {
	if (isLoading) return "loading";
	if (error) return "error";
	if (!data || !data.job || data.cards.length === 0) return "empty";
	if (data.job.degraded) return "degraded";
	return "populated";
}

/**
 * Formats the recap job window (from → to) in local US date format.
 * Example: "Sep 19, 2026 – Sep 22, 2026"
 */
export function formatJobWindow(from: string, to: string): string {
	if (!from || !to) return "";

	const fromDate = new Date(from);
	const toDate = new Date(to);

	if (Number.isNaN(fromDate.getTime()) || Number.isNaN(toDate.getTime())) {
		return "";
	}

	const fromStr = fromDate.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
	});
	const toStr = toDate.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		year: "numeric",
	});

	return `${fromStr} – ${toStr}`;
}
