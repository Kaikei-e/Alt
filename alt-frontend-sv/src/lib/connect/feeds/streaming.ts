/**
 * Streaming feed functions via Connect-RPC Server Streaming
 * Includes streamFeedStats and streamSummarize with retry logic
 */

import { ConnectError, Code } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import type {
	StreamFeedStatsResponse,
	StreamSummarizeResponse,
} from "$lib/gen/alt/feeds/v2/feeds_pb";
import { createFeedClient } from "./client";

// =============================================================================
// Streaming Feed Stats
// =============================================================================

/**
 * Streaming feed stats via Server Streaming RPC.
 */
export interface StreamingFeedStats {
	feedAmount: number;
	unsummarizedFeedAmount: number;
	totalArticles: number;
	isHeartbeat: boolean;
	timestamp: number;
}

/**
 * Stream feed statistics in real-time via Connect-RPC Server Streaming.
 *
 * @param transport - The Connect transport to use
 * @param onData - Callback when new stats are received
 * @param onError - Callback on error (optional)
 * @returns AbortController to cancel the stream
 */
export async function streamFeedStats(
	transport: Transport,
	onData: (stats: StreamingFeedStats) => void,
	onError?: (error: Error) => void,
): Promise<AbortController> {
	console.log("[streamFeedStats] Starting stream...");
	const client = createFeedClient(transport);
	const abortController = new AbortController();

	// Start streaming in background
	(async () => {
		try {
			console.log("[streamFeedStats] Calling client.streamFeedStats()...");
			const stream = client.streamFeedStats(
				{},
				{ signal: abortController.signal },
			);

			console.log("[streamFeedStats] Stream created, waiting for data...");
			for await (const rawResponse of stream) {
				const response = rawResponse as StreamFeedStatsResponse;
				const isHeartbeat = response.metadata?.isHeartbeat ?? false;
				console.log("[streamFeedStats] Received response:", {
					isHeartbeat,
					feedAmount: response.feedAmount,
				});

				// Always call onData, even for heartbeats
				// Components can decide whether to ignore heartbeats
				onData({
					feedAmount: Number(response.feedAmount),
					unsummarizedFeedAmount: Number(response.unsummarizedFeedAmount),
					totalArticles: Number(response.totalArticles),
					isHeartbeat,
					timestamp: Number(response.metadata?.timestamp ?? Date.now() / 1000),
				});
			}
			console.log("[streamFeedStats] Stream ended normally");
		} catch (error) {
			// Check abort BEFORE logging to suppress navigation-related errors
			if (abortController.signal.aborted) {
				console.log("[streamFeedStats] Stream closed due to navigation");
				return;
			}
			console.error("[streamFeedStats] Stream error:", error);
			if (onError && error instanceof Error) {
				onError(error);
			}
		}
	})();

	return abortController;
}

// =============================================================================
// Streaming Summarize
// =============================================================================

/** Default delay before retrying on 409 Conflict error (in milliseconds). */
const CONFLICT_RETRY_DELAY_MS = 3000;

/** Maximum number of retries for 409 Conflict errors. */
const CONFLICT_MAX_RETRIES = 3;

/** Checks if an error is a 409 Conflict (article already processing) error. */
function isConflictError(error: unknown): boolean {
	if (error instanceof ConnectError) {
		return error.code === Code.AlreadyExists;
	}
	return false;
}

/** Delays execution for the specified number of milliseconds. */
function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Request options for streaming summarization.
 */
export interface StreamSummarizeOptions {
	/** Feed/article URL (required if articleId not provided) */
	feedUrl?: string;
	/** Existing article ID (required if feedUrl not provided) */
	articleId?: string;
	/** Pre-fetched content (optional, skips fetch if provided) */
	content?: string;
	/** Article title (optional) */
	title?: string;
	/**
	 * Enable automatic retry on 409 Conflict errors.
	 * When another request is processing the same article, this will wait and retry.
	 * Default: true
	 */
	retryOnConflict?: boolean;
	/**
	 * Callback when waiting for retry due to 409 Conflict.
	 * Can be used to show "processing in progress" message to user.
	 */
	onConflictRetry?: (retryCount: number, maxRetries: number) => void;
}

/**
 * Streaming summarize chunk response.
 */
export interface StreamSummarizeChunk {
	/** Text chunk from summarization */
	chunk: string;
	/** Whether this is the final chunk */
	isFinal: boolean;
	/** Article ID (populated after first chunk or from cache) */
	articleId: string;
	/** Whether this response is from cache */
	isCached: boolean;
	/** Full summary (only populated if isCached=true or isFinal=true) */
	fullSummary: string | null;
}

/**
 * Result returned when streaming completes successfully.
 */
export interface StreamSummarizeResult {
	/** The article ID */
	articleId: string;
	/** The full summary text */
	summary: string;
	/** Whether the result was from cache */
	wasCached: boolean;
}

// =============================================================================
// Shared Response Processing (Step 5 DRY)
// =============================================================================

interface SummarizeAccumulator {
	articleId: string;
	fullSummary: string;
	wasCached: boolean;
}

/**
 * Process a single stream response and accumulate state.
 * Shared between streamSummarize and streamSummarizeWithAbort.
 */
function processStreamResponse(
	response: StreamSummarizeResponse,
	acc: SummarizeAccumulator,
): void {
	if (response.articleId) {
		acc.articleId = response.articleId;
	}
	if (response.isCached && response.fullSummary) {
		acc.wasCached = true;
		acc.fullSummary = response.fullSummary;
	}
	if (!response.isCached && response.chunk) {
		acc.fullSummary += response.chunk;
	}
	if (response.isFinal && response.fullSummary) {
		acc.fullSummary = response.fullSummary;
	}
}

/** Convert a proto response to a StreamSummarizeChunk. */
function toSummarizeChunk(response: StreamSummarizeResponse): StreamSummarizeChunk {
	return {
		chunk: response.chunk,
		isFinal: response.isFinal,
		articleId: response.articleId,
		isCached: response.isCached,
		fullSummary: response.fullSummary ?? null,
	};
}

// =============================================================================
// Stream Summarize Functions
// =============================================================================

/**
 * Stream article summarization in real-time via Connect-RPC Server Streaming.
 * Automatically retries on 409 Conflict errors (article already processing).
 *
 * @param transport - The Connect transport to use
 * @param options - Request options (feedUrl or articleId required)
 * @param onChunk - Callback when a chunk is received (optional)
 * @param onError - Callback on error (optional)
 * @returns Promise that resolves with the full summary when complete
 */
export async function streamSummarize(
	transport: Transport,
	options: StreamSummarizeOptions,
	onChunk?: (chunk: StreamSummarizeChunk) => void,
	onError?: (error: Error) => void,
): Promise<StreamSummarizeResult> {
	const client = createFeedClient(transport);
	const retryOnConflict = options.retryOnConflict ?? true;

	// Validate options
	if (!options.feedUrl && !options.articleId) {
		throw new Error("Either feedUrl or articleId is required");
	}

	// Retry loop for 409 Conflict errors
	let retryCount = 0;
	while (retryCount <= CONFLICT_MAX_RETRIES) {
		const acc: SummarizeAccumulator = {
			articleId: "",
			fullSummary: "",
			wasCached: false,
		};

		try {
			const stream = client.streamSummarize({
				feedUrl: options.feedUrl,
				articleId: options.articleId,
				content: options.content,
				title: options.title,
			});

			for await (const rawResponse of stream) {
				const response = rawResponse as StreamSummarizeResponse;
				processStreamResponse(response, acc);

				if (onChunk) {
					onChunk(toSummarizeChunk(response));
				}
			}

			return {
				articleId: acc.articleId,
				summary: acc.fullSummary,
				wasCached: acc.wasCached,
			};
		} catch (error) {
			// Handle 409 Conflict with retry
			if (
				retryOnConflict &&
				isConflictError(error) &&
				retryCount < CONFLICT_MAX_RETRIES
			) {
				retryCount++;
				if (options.onConflictRetry) {
					options.onConflictRetry(retryCount, CONFLICT_MAX_RETRIES);
				}
				await delay(CONFLICT_RETRY_DELAY_MS);
				continue;
			}

			if (onError && error instanceof Error) {
				onError(error);
			}
			throw error;
		}
	}

	// Should never reach here, but TypeScript needs a return
	throw new Error("Max retries exceeded for summarization");
}

/**
 * Stream article summarization with AbortController support.
 * Automatically retries on 409 Conflict errors (article already processing).
 *
 * @param transport - The Connect transport to use
 * @param options - Request options (feedUrl or articleId required)
 * @param onChunk - Callback when a chunk is received
 * @param onComplete - Callback when streaming completes successfully
 * @param onError - Callback on error (optional)
 * @returns AbortController to cancel the stream
 */
export function streamSummarizeWithAbort(
	transport: Transport,
	options: StreamSummarizeOptions,
	onChunk: (chunk: StreamSummarizeChunk) => void,
	onComplete: (result: StreamSummarizeResult) => void,
	onError?: (error: Error) => void,
): AbortController {
	const abortController = new AbortController();
	const retryOnConflict = options.retryOnConflict ?? true;

	// Validate options
	if (!options.feedUrl && !options.articleId) {
		const error = new Error("Either feedUrl or articleId is required");
		if (onError) {
			onError(error);
		}
		return abortController;
	}

	const client = createFeedClient(transport);

	// Internal function to perform streaming with retry support
	const performStream = async (retryCount: number) => {
		if (abortController.signal.aborted) {
			return;
		}

		const acc: SummarizeAccumulator = {
			articleId: "",
			fullSummary: "",
			wasCached: false,
		};

		try {
			const stream = client.streamSummarize(
				{
					feedUrl: options.feedUrl,
					articleId: options.articleId,
					content: options.content,
					title: options.title,
				},
				{ signal: abortController.signal },
			);

			for await (const rawResponse of stream) {
				const response = rawResponse as StreamSummarizeResponse;
				processStreamResponse(response, acc);
				onChunk(toSummarizeChunk(response));
			}

			onComplete({
				articleId: acc.articleId,
				summary: acc.fullSummary,
				wasCached: acc.wasCached,
			});
		} catch (error) {
			if (abortController.signal.aborted) {
				return;
			}

			// Handle 409 Conflict with retry
			if (
				retryOnConflict &&
				isConflictError(error) &&
				retryCount < CONFLICT_MAX_RETRIES
			) {
				const nextRetry = retryCount + 1;
				if (options.onConflictRetry) {
					options.onConflictRetry(nextRetry, CONFLICT_MAX_RETRIES);
				}
				await delay(CONFLICT_RETRY_DELAY_MS);
				if (!abortController.signal.aborted) {
					await performStream(nextRetry);
				}
				return;
			}

			if (onError && error instanceof Error) {
				onError(error);
			}
		}
	};

	// Start streaming in background
	performStream(0);

	return abortController;
}
