/**
 * AugurService client for Connect-RPC
 *
 * Provides type-safe methods to call AugurService endpoints for RAG-powered chat.
 */

import { createClient } from "@connectrpc/connect";
import type { Transport } from "@connectrpc/connect";
import { AugurService } from "$lib/gen/alt/augur/v2/augur_pb";

// =============================================================================
// Types
// =============================================================================

/**
 * Citation/source reference from Augur responses
 */
export interface AugurCitation {
	url: string;
	title: string;
	publishedAt: string;
}

/**
 * Chat message for Augur conversation
 */
export interface AugurChatMessage {
	role: "user" | "assistant";
	content: string;
}

/**
 * Options for streaming Augur chat
 */
export interface AugurStreamOptions {
	/** Chat message history (alternating user/assistant messages) */
	messages: AugurChatMessage[];
}

/**
 * Result when streaming chat completes successfully
 */
export interface AugurStreamResult {
	/** The full answer text */
	answer: string;
	/** Citations used in the answer */
	citations: AugurCitation[];
}

// =============================================================================
// Client
// =============================================================================

/**
 * Creates an AugurService client with the given transport.
 */
export function createAugurClient(transport: Transport) {
	return createClient(AugurService, transport);
}

/**
 * Stream Augur chat with callback-based event handling.
 *
 * Provides fine-grained control over different event types:
 * - delta: Text chunks as they arrive
 * - meta: Citations/sources for the response
 * - done: Final complete response
 * - fallback: Fallback reason when RAG context is insufficient
 * - error: Error messages
 *
 * @param transport - The Connect transport to use
 * @param options - Chat options including message history
 * @param onDelta - Callback for text chunks (optional)
 * @param onMeta - Callback for citations (optional)
 * @param onComplete - Callback when streaming completes (optional)
 * @param onFallback - Callback for fallback events (optional)
 * @param onError - Callback for errors (optional)
 * @returns AbortController to cancel the stream
 */
export function streamAugurChat(
	transport: Transport,
	options: AugurStreamOptions,
	onDelta?: (text: string) => void,
	onMeta?: (citations: AugurCitation[]) => void,
	onComplete?: (result: AugurStreamResult) => void,
	onFallback?: (code: string) => void,
	onError?: (error: Error) => void,
): AbortController {
	const abortController = new AbortController();
	const client = createAugurClient(transport);

	// Track accumulated text for complete result
	let accumulatedText = "";
	let latestCitations: AugurCitation[] = [];

	// Start streaming in background
	(async () => {
		try {
			const stream = client.streamChat(
				{
					messages: options.messages.map((m) => ({
						role: m.role,
						content: m.content,
					})),
				},
				{ signal: abortController.signal },
			);

			for await (const event of stream) {
				const { kind, payload } = event;

				switch (payload.case) {
					case "delta":
						if (payload.value) {
							accumulatedText += payload.value;
							if (onDelta) {
								onDelta(payload.value);
							}
						}
						break;

					case "meta":
						if (payload.value?.citations) {
							const citations = payload.value.citations.map((c) => ({
								url: c.url,
								title: c.title,
								publishedAt: c.publishedAt,
							}));
							latestCitations = citations;
							if (onMeta) {
								onMeta(citations);
							}
						}
						break;

					case "done":
						if (payload.value) {
							const citations = payload.value.citations.map((c) => ({
								url: c.url,
								title: c.title,
								publishedAt: c.publishedAt,
							}));
							// Use final answer from done payload if available
							const finalAnswer = payload.value.answer || accumulatedText;
							if (onComplete) {
								onComplete({
									answer: finalAnswer,
									citations: citations.length > 0 ? citations : latestCitations,
								});
							}
						}
						break;

					case "fallbackCode":
						if (onFallback && payload.value) {
							onFallback(payload.value);
						}
						break;

					case "errorMessage":
						if (onError && payload.value) {
							onError(new Error(payload.value));
						}
						break;
				}
			}

			// If stream completes without done event, call onComplete with accumulated data
			if (!abortController.signal.aborted && onComplete && accumulatedText) {
				onComplete({
					answer: accumulatedText,
					citations: latestCitations,
				});
			}
		} catch (error) {
			// Only report error if not aborted
			if (!abortController.signal.aborted && onError && error instanceof Error) {
				onError(error);
			}
		}
	})();

	return abortController;
}

/**
 * Stream Augur chat with Promise-based interface.
 *
 * Simpler interface when you just need the final result.
 *
 * @param transport - The Connect transport to use
 * @param options - Chat options including message history
 * @param onDelta - Optional callback for real-time text updates
 * @returns Promise that resolves with the complete result
 */
export async function streamAugurChatAsync(
	transport: Transport,
	options: AugurStreamOptions,
	onDelta?: (text: string) => void,
): Promise<AugurStreamResult> {
	return new Promise((resolve, reject) => {
		streamAugurChat(
			transport,
			options,
			onDelta,
			undefined, // onMeta
			(result) => resolve(result), // onComplete
			undefined, // onFallback
			(error) => reject(error), // onError
		);
	});
}

// =============================================================================
// Context Retrieval (Unary RPC)
// =============================================================================

/**
 * Context item from RetrieveContext response
 */
export interface AugurContextItem {
	url: string;
	title: string;
	publishedAt: string;
	score: number;
}

/**
 * Options for context retrieval
 */
export interface RetrieveContextOptions {
	/** Query to search for relevant context */
	query: string;
	/** Maximum number of context items to return (default: 5) */
	limit?: number;
}

/**
 * Retrieve relevant context for a query without generating an answer.
 *
 * Useful for debugging or showing sources before starting a chat.
 *
 * @param transport - The Connect transport to use
 * @param options - Query and optional limit
 * @returns Array of context items with relevance scores
 */
export async function retrieveAugurContext(
	transport: Transport,
	options: RetrieveContextOptions,
): Promise<AugurContextItem[]> {
	const client = createAugurClient(transport);
	const response = await client.retrieveContext({
		query: options.query,
		limit: options.limit ?? 5,
	});

	return response.contexts.map((c) => ({
		url: c.url,
		title: c.title,
		publishedAt: c.publishedAt,
		score: c.score,
	}));
}
