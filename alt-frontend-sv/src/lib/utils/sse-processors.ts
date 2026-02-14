/**
 * Domain-specific SSE stream processors
 * Each processor handles a different SSE event format
 */

import { parseSSEStream } from "./sse-parser";
import {
	createStreamingRenderer,
	type StreamingRendererOptions,
} from "./streamingRenderer";

/**
 * Augur-specific streaming processor
 * Handles 'delta', 'meta', 'fallback' events
 */
export async function processAugurStreamingText(
	reader: ReadableStreamDefaultReader<Uint8Array>,
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
): Promise<{
	chunkCount: number;
	totalLength: number;
	hasReceivedData: boolean;
}> {
	const renderer = createStreamingRenderer(updateState, options);
	let hasReceivedData = false;

	try {
		for await (const event of parseSSEStream(reader)) {
			if (event.event === "delta") {
				if (event.data) {
					await renderer.processChunk(event.data);
					hasReceivedData = true;
				}
			} else if (event.event === "meta" || event.event === "done") {
				if (options.onMetadata && event.data) {
					try {
						const parsed = JSON.parse(event.data);
						options.onMetadata(parsed);
					} catch (e) {
						console.warn("[AugurStream] Failed to parse metadata", e);
					}
				}
			} else if (event.event === "fallback") {
				if (options.onMetadata) {
					options.onMetadata({ fallback: true, code: event.data });
				}
			} else if (event.event === "error") {
				console.error("[AugurStream] Error Event:", event.data);
			}
		}
		renderer.flush();
	} catch (error) {
		renderer.cancel();
		throw error;
	}

	return {
		chunkCount: renderer.getChunkCount(),
		totalLength: renderer.getTotalLength(),
		hasReceivedData,
	};
}

/**
 * Generic Streaming Text Processor
 * Treats everything as text content unless specific event handling is added.
 * Useful for standard text-only streams.
 */
export async function processGenericStreamingText(
	reader: ReadableStreamDefaultReader<Uint8Array>,
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
): Promise<void> {
	const renderer = createStreamingRenderer(updateState, options);
	try {
		for await (const event of parseSSEStream(reader)) {
			// Treat all events data as content for now, or just 'message'/'delta'
			if (event.data) {
				await renderer.processChunk(event.data);
			}
		}
		renderer.flush();
	} catch (e) {
		renderer.cancel();
		throw e;
	}
}

/**
 * Summarize-specific streaming processor (News Creator)
 * Handles standard SSE 'message' events where data is a JSON string of the text chunk.
 */
export async function processSummarizeStreamingText(
	reader: ReadableStreamDefaultReader<Uint8Array>,
	updateState: (text: string) => void,
	options: StreamingRendererOptions = {},
): Promise<{
	chunkCount: number;
	totalLength: number;
	hasReceivedData: boolean;
}> {
	const renderer = createStreamingRenderer(updateState, options);

	try {
		for await (const event of parseSSEStream(reader)) {
			if (event.data) {
				// Handling "message" (default) or "delta".
				if (event.event === "message" || event.event === "delta") {
					try {
						// Parse JSON string (e.g. '"Hello"') -> 'Hello'
						const text = JSON.parse(event.data);
						if (typeof text === "string") {
							await renderer.processChunk(text);
						} else if (text && typeof text === "object") {
							// If it's a complex object (rare for this usecase), stringify it
							await renderer.processChunk(JSON.stringify(text));
						} else {
							// Numbers, booleans, etc
							await renderer.processChunk(String(text));
						}
					} catch (e) {
						// Not valid JSON, treat as raw text (fallback)
						await renderer.processChunk(event.data);
					}
				}
			}
		}
		renderer.flush();
	} catch (error) {
		renderer.cancel();
		throw error;
	}

	return {
		chunkCount: renderer.getChunkCount(),
		totalLength: renderer.getTotalLength(),
		hasReceivedData: renderer.getChunkCount() > 0,
	};
}

// @deprecated Use processAugurStreamingText or processSummarizeStreamingText
export const processStreamingText = processAugurStreamingText;
