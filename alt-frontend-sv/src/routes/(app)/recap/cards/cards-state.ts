import type { RecapCard, RecapCardsJob, RecapCardsRun } from "$lib/connect";

export type CardsPageState =
	| "loading"
	| "error"
	| "empty"
	| "degraded"
	| "populated";

export interface CardsPageStateInput {
	isLoading: boolean;
	error: Error | null;
	data: {
		job?: RecapCardsJob | null;
		cards: RecapCard[];
		latestRun?: RecapCardsRun | null;
	} | null;
}

/**
 * Determines the current display state of the topic cards page.
 */
export function determineCardsPageState({
	isLoading,
	error,
	data,
}: CardsPageStateInput): CardsPageState {
	if (isLoading) return "loading";
	if (error) return "error";
	if (!data?.job || data.cards.length === 0) return "empty";
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

/**
 * Computes window label from job.from and job.to.
 * Example: "72-hour window"
 */
export function computeWindowLabel(from: string, to: string): string {
	if (!from || !to) return "";

	const fromDate = new Date(from);
	const toDate = new Date(to);

	const fromTime = fromDate.getTime();
	const toTime = toDate.getTime();

	if (Number.isNaN(fromTime) || Number.isNaN(toTime) || toTime <= fromTime) {
		return "";
	}

	const diffHours = Math.round((toTime - fromTime) / (1000 * 60 * 60));
	return diffHours === 1 ? "1-hour window" : `${diffHours}-hour window`;
}

/**
 * Formats ISO timestamp into readable date and time.
 * Example: "Sep 22, 5:00 PM"
 */
export function formatCardTimestamp(iso: string): string {
	if (!iso) return "";

	const date = new Date(iso);
	if (Number.isNaN(date.getTime())) {
		return "";
	}

	return date.toLocaleString("en-US", {
		month: "short",
		day: "numeric",
		hour: "numeric",
		minute: "2-digit",
	});
}

/**
 * Returns the empty state description text depending on whether a job has run.
 */
export function getCardsEmptyMessage(hasJob: boolean): string {
	return hasJob
		? "No topic cards were selected in the latest run."
		: "Topic cards will appear here after the first daily run.";
}

export interface TopicCardsNotice {
	tone: "info" | "error";
	text: string;
}

export interface TopicCardsNoticeInput {
	job?: { kickedAt: string } | null;
	cards?: readonly unknown[] | null;
	latestRun?: {
		jobId?: string;
		/** Known values: "pending" | "running" | "completed" | "failed" */
		status: string;
		kickedAt?: string;
		updatedAt?: string;
	} | null;
}

/**
 * Determines whether to display a status notice for the topic cards update.
 */
export function getTopicCardsNotice({
	job,
	cards,
	latestRun,
}: TopicCardsNoticeInput): TopicCardsNotice | null {
	if (!latestRun) return null;

	if (latestRun.status === "running" || latestRun.status === "pending") {
		const time = latestRun.kickedAt
			? formatCardTimestamp(latestRun.kickedAt)
			: "";
		return {
			tone: "info",
			text: time
				? `Update in progress (started ${time}).`
				: "Update in progress.",
		};
	}

	if (latestRun.status === "failed") {
		const failTime = formatCardTimestamp(
			latestRun.updatedAt || latestRun.kickedAt || "",
		);
		let text = failTime
			? `The latest update failed at ${failTime}.`
			: "The latest update failed.";

		if (job && cards && cards.length > 0) {
			const jobTime = formatCardTimestamp(job.kickedAt);
			if (jobTime) {
				text += ` Showing cards from ${jobTime}.`;
			}
		}

		return {
			tone: "error",
			text,
		};
	}

	return null;
}
