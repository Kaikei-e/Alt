/**
 * Reusable summarize hook for streaming AI summarization.
 *
 * Extracts the summarize logic from FeedDetailModal into a reusable hook
 * following the useTtsPlayback pattern.
 */

import {
	createClientTransport,
	streamSummarizeWithAbortAdapter,
} from "$lib/connect";
import { isTransientError } from "$lib/utils/errorClassification";

export type SummarizeButtonState = "idle" | "loading" | "error" | "success";

export interface UseSummarize {
	readonly summary: string | null;
	readonly isSummarizing: boolean;
	readonly summaryError: string | null;
	readonly buttonState: SummarizeButtonState;
	summarize(
		feedUrl: string,
		articleId?: string,
		title?: string,
		forceRefresh?: boolean,
	): void;
	abort(): void;
	reset(): void;
}

export function useSummarize(): UseSummarize {
	let isSummarizing = $state(false);
	let summary = $state<string | null>(null);
	let summaryError = $state<string | null>(null);
	let abortController = $state<AbortController | null>(null);
	let retryCount = $state(0);

	const buttonState = $derived.by(() => {
		if (isSummarizing) return "loading" as const;
		if (summaryError) return "error" as const;
		if (summary) return "success" as const;
		return "idle" as const;
	});

	function summarize(
		feedUrl: string,
		articleId?: string,
		title?: string,
		forceRefresh = false,
	) {
		// Cancel previous request
		if (abortController) {
			abortController.abort();
		}

		isSummarizing = true;
		summaryError = null;
		summary = "";

		try {
			const transport = createClientTransport();
			abortController = streamSummarizeWithAbortAdapter(
				transport,
				{
					feedUrl,
					articleId,
					title,
					forceRefresh,
				},
				(chunk: string) => {
					summary = (summary || "") + chunk;
				},
				{},
				(_result) => {
					isSummarizing = false;
					abortController = null;
				},
				(error) => {
					// Ignore abort/cancel errors (user navigation)
					if (error.name === "AbortError") {
						isSummarizing = false;
						abortController = null;
						return;
					}
					if (
						error.message?.includes("abort") ||
						error.message?.includes("cancel")
					) {
						isSummarizing = false;
						abortController = null;
						return;
					}

					// Auto-retry for transient errors (1 attempt only)
					if (isTransientError(error) && retryCount < 1) {
						retryCount++;
						abortController = null;
						setTimeout(() => {
							isSummarizing = false;
							summarize(feedUrl, articleId, title, forceRefresh);
						}, 500);
						return;
					}

					summaryError = error.message || "Failed to generate summary";
					isSummarizing = false;
					abortController = null;
				},
			);
		} catch (err) {
			if (err instanceof Error) {
				if (err.name === "AbortError") return;
				if (err.message.includes("abort") || err.message.includes("cancel"))
					return;
			}
			summaryError =
				err instanceof Error ? err.message : "Failed to generate summary";
			isSummarizing = false;
			abortController = null;
		}
	}

	function abort() {
		if (abortController) {
			abortController.abort();
			abortController = null;
		}
	}

	function reset() {
		abort();
		isSummarizing = false;
		summary = null;
		summaryError = null;
		retryCount = 0;
	}

	return {
		get summary() {
			return summary;
		},
		get isSummarizing() {
			return isSummarizing;
		},
		get summaryError() {
			return summaryError;
		},
		get buttonState() {
			return buttonState;
		},
		summarize,
		abort,
		reset,
	};
}
