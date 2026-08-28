/**
 * Connect-RPC Streaming Adapter
 *
 * Bridges Connect-RPC streaming callbacks to the existing createStreamingRenderer interface.
 * Supports typewriter effect and other rendering options.
 */

import type { Transport } from "@connectrpc/connect";
import {
	createStreamingRenderer,
	type StreamingRendererOptions,
} from "$lib/utils/streamingRenderer";
import {
	type StreamSummarizeChunk,
	type StreamSummarizeResult,
	streamSummarizeWithAbort,
} from "./feeds";

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
	/** Force re-summarize, bypassing summary cache */
	forceRefresh?: boolean;
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
// Error Handling
// =============================================================================

/**
 * Whether the stream ended because the reader cancelled it locally.
 *
 * Only then is the untyped backlog theirs to lose. A stream cut by the network
 * ("missing EndStreamResponse" and friends) leaves received text queued behind
 * the typewriter, and dropping it hides the words the server already sent.
 */
function isLocalAbort(error: Error, controller?: AbortController): boolean {
	return error.name === "AbortError" || controller?.signal.aborted === true;
}

// =============================================================================
// Shared Chunk Processor
// =============================================================================

/**
 * Creates a chunk processor that bridges Connect-RPC streaming chunks
 * to the StreamingRenderer interface. Shared by both adapter functions.
 */
function createChunkProcessor(
	renderer: ReturnType<typeof createStreamingRenderer>,
	rendererOptions: StreamingRendererOptions,
) {
	let articleId = "";
	let wasCached = false;
	let hasReceivedData = false;
	let isFirstChunk = true;

	const processChunk = async (chunk: StreamSummarizeChunk) => {
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
	};

	const getState = () => ({ articleId, wasCached, hasReceivedData });

	return { processChunk, getState };
}

// =============================================================================
// Adapter Functions
// =============================================================================

/**
 * Streams summarization using Connect-RPC with renderer integration.
 * Promise-based version that awaits completion.
 */
export async function streamSummarizeWithRenderer(
	transport: Transport,
	options: StreamSummarizeAdapterOptions,
	updateState: (text: string) => void,
	rendererOptions: StreamingRendererOptions = {},
): Promise<StreamSummarizeAdapterResult> {
	const renderer = createStreamingRenderer(updateState, rendererOptions);
	const processor = createChunkProcessor(renderer, rendererOptions);
	let controller: AbortController | undefined;

	return new Promise((resolve, reject) => {
		controller = streamSummarizeWithAbort(
			transport,
			{
				feedUrl: options.feedUrl,
				articleId: options.articleId,
				content: options.content,
				title: options.title,
				forceRefresh: options.forceRefresh,
			},
			processor.processChunk,
			(result: StreamSummarizeResult) => {
				renderer.flush();
				const state = processor.getState();
				resolve({
					chunkCount: renderer.getChunkCount(),
					totalLength: renderer.getTotalLength(),
					hasReceivedData: state.hasReceivedData,
					articleId: result.articleId,
					wasCached: result.wasCached,
				});
			},
			(error: Error) => {
				if (isLocalAbort(error, controller)) {
					renderer.cancel();
				} else {
					renderer.flushPending();
				}
				reject(error);
			},
		);
	});
}

/**
 * Streams summarization with AbortController for external cancellation.
 * Returns immediately with an AbortController; use callbacks for data.
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
	const processor = createChunkProcessor(renderer, rendererOptions);
	// Assigned below; the error callback may run before the assignment lands
	// (the validation failure surfaces synchronously), so it reads it lazily.
	let controller: AbortController | undefined;

	controller = streamSummarizeWithAbort(
		transport,
		{
			feedUrl: options.feedUrl,
			articleId: options.articleId,
			content: options.content,
			title: options.title,
			forceRefresh: options.forceRefresh,
		},
		processor.processChunk,
		(result: StreamSummarizeResult) => {
			renderer.flush();
			if (onComplete) {
				const state = processor.getState();
				onComplete({
					chunkCount: renderer.getChunkCount(),
					totalLength: renderer.getTotalLength(),
					hasReceivedData: state.hasReceivedData,
					articleId: result.articleId,
					wasCached: result.wasCached,
				});
			}
		},
		(error: Error) => {
			if (isLocalAbort(error, controller)) {
				renderer.cancel();
			} else {
				// Everything received reaches the UI before the error does, so a
				// cut stream shows the words the server did send.
				renderer.flushPending();
			}
			if (onError) {
				onError(error);
			}
		},
	);

	return controller;
}
