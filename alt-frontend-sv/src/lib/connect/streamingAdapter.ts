/**
 * Connect-RPC Streaming Adapter
 *
 * Bridges Connect-RPC streaming callbacks to the existing createStreamingRenderer interface.
 * Supports typewriter effect and other rendering options.
 */

import type { Transport } from "@connectrpc/connect";
import {
	streamSummarizeWithAbort,
	type StreamSummarizeOptions,
	type StreamSummarizeChunk,
	type StreamSummarizeResult,
} from "./feeds";
import {
	createStreamingRenderer,
	type StreamingRendererOptions,
} from "$lib/utils/streamingRenderer";

// =============================================================================
// Types
// =============================================================================

/**
 * Adapter options for streaming summarization.
 * Matches the existing REST SSE client signature for easy migration.
 */
export interface StreamSummarizeAdapterOptions {
	/** Feed/article URL (required if articleId not provided) */
	feedUrl: string;
	/** Existing article ID (optional) */
	articleId?: string;
	/** Pre-fetched content (optional, skips fetch if provided) */
	content?: string;
	/** Article title (optional) */
	title?: string;
}

/**
 * Result returned when streaming completes successfully.
 */
export interface StreamSummarizeAdapterResult {
	/** Number of chunks received */
	chunkCount: number;
	/** Total length of text rendered */
	totalLength: number;
	/** Whether any data was received */
	hasReceivedData: boolean;
	/** The article ID */
	articleId: string;
	/** Whether the result was from cache */
	wasCached: boolean;
}

// =============================================================================
// Adapter Functions
// =============================================================================

/**
 * Streams summarization using Connect-RPC with renderer integration.
 * Promise-based version that awaits completion.
 *
 * @param transport - Connect-RPC transport (from createClientTransport)
 * @param options - Summarization options
 * @param updateState - Callback to update state with each chunk (accumulating)
 * @param rendererOptions - Typewriter effect and other rendering options
 * @returns Promise that resolves when streaming completes
 *
 * @example
 * ```typescript
 * const transport = createClientTransport();
 * const result = await streamSummarizeWithRenderer(
 *   transport,
 *   { feedUrl: "https://example.com/article" },
 *   (chunk) => { summary = (summary || "") + chunk; },
 *   { typewriter: true, typewriterDelay: 10 },
 * );
 * ```
 */
export async function streamSummarizeWithRenderer(
	transport: Transport,
	options: StreamSummarizeAdapterOptions,
	updateState: (text: string) => void,
	rendererOptions: StreamingRendererOptions = {},
): Promise<StreamSummarizeAdapterResult> {
	const renderer = createStreamingRenderer(updateState, rendererOptions);

	return new Promise((resolve, reject) => {
		let articleId = "";
		let wasCached = false;
		let hasReceivedData = false;
		let isFirstChunk = true;

		const abortController = streamSummarizeWithAbort(
			transport,
			{
				feedUrl: options.feedUrl,
				articleId: options.articleId,
				content: options.content,
				title: options.title,
			},
			// onChunk
			async (chunk: StreamSummarizeChunk) => {
				hasReceivedData = true;

				if (chunk.articleId) {
					articleId = chunk.articleId;
				}

				if (chunk.isCached) {
					wasCached = true;
					// For cached responses, render the full summary
					if (chunk.fullSummary) {
						await renderer.processChunk(chunk.fullSummary);
					}
				} else if (chunk.chunk) {
					// Stream each chunk through the renderer (supports typewriter)
					await renderer.processChunk(chunk.chunk);
				}

				// Trigger onChunk callback for first chunk detection etc.
				if (isFirstChunk && rendererOptions.onChunk) {
					rendererOptions.onChunk(
						renderer.getChunkCount(),
						chunk.chunk?.length ?? 0,
						chunk.chunk?.length ?? 0,
						renderer.getTotalLength(),
						chunk.chunk?.substring(0, 50) ?? "",
					);
					isFirstChunk = false;
				}
			},
			// onComplete
			(result: StreamSummarizeResult) => {
				renderer.flush();
				resolve({
					chunkCount: renderer.getChunkCount(),
					totalLength: renderer.getTotalLength(),
					hasReceivedData,
					articleId: result.articleId,
					wasCached: result.wasCached,
				});
			},
			// onError
			(error: Error) => {
				renderer.cancel();
				reject(error);
			},
		);

		// The abortController is managed internally; external cancellation
		// would need to be handled via component cleanup
	});
}

/**
 * Streams summarization with AbortController for external cancellation.
 * Returns immediately with an AbortController; use callbacks for data.
 *
 * @param transport - Connect-RPC transport (from createClientTransport)
 * @param options - Summarization options
 * @param updateState - Callback to update state with each chunk (accumulating)
 * @param rendererOptions - Typewriter effect and other rendering options
 * @param onComplete - Callback when streaming completes successfully
 * @param onError - Callback on error
 * @returns AbortController to cancel the stream
 *
 * @example
 * ```typescript
 * const transport = createClientTransport();
 * abortController = streamSummarizeWithAbortAdapter(
 *   transport,
 *   { feedUrl: feed.link, title: feed.title },
 *   (chunk) => { summary = (summary || "") + chunk; },
 *   { typewriter: true, typewriterDelay: 10 },
 *   (result) => { console.log("Complete:", result); },
 *   (error) => { console.error("Error:", error); },
 * );
 *
 * // To cancel:
 * abortController.abort();
 * ```
 */
export function streamSummarizeWithAbortAdapter(
	transport: Transport,
	options: StreamSummarizeAdapterOptions,
	updateState: (text: string) => void,
	rendererOptions: StreamingRendererOptions = {},
	onComplete?: (result: StreamSummarizeAdapterResult) => void,
	onError?: (error: Error) => void,
): AbortController {
	const renderer = createStreamingRenderer(updateState, rendererOptions);
	let articleId = "";
	let wasCached = false;
	let hasReceivedData = false;
	let isFirstChunk = true;

	return streamSummarizeWithAbort(
		transport,
		{
			feedUrl: options.feedUrl,
			articleId: options.articleId,
			content: options.content,
			title: options.title,
		},
		// onChunk
		async (chunk: StreamSummarizeChunk) => {
			hasReceivedData = true;

			if (chunk.articleId) {
				articleId = chunk.articleId;
			}

			if (chunk.isCached) {
				wasCached = true;
				// For cached responses, render the full summary
				if (chunk.fullSummary) {
					await renderer.processChunk(chunk.fullSummary);
				}
			} else if (chunk.chunk) {
				// Stream each chunk through the renderer (supports typewriter)
				await renderer.processChunk(chunk.chunk);
			}

			// Trigger onChunk callback for first chunk detection etc.
			if (isFirstChunk && rendererOptions.onChunk) {
				rendererOptions.onChunk(
					renderer.getChunkCount(),
					chunk.chunk?.length ?? 0,
					chunk.chunk?.length ?? 0,
					renderer.getTotalLength(),
					chunk.chunk?.substring(0, 50) ?? "",
				);
				isFirstChunk = false;
			}
		},
		// onComplete
		(result: StreamSummarizeResult) => {
			renderer.flush();
			if (onComplete) {
				onComplete({
					chunkCount: renderer.getChunkCount(),
					totalLength: renderer.getTotalLength(),
					hasReceivedData,
					articleId: result.articleId,
					wasCached: result.wasCached,
				});
			}
		},
		// onError
		(error: Error) => {
			renderer.cancel();
			if (onError) {
				onError(error);
			}
		},
	);
}
