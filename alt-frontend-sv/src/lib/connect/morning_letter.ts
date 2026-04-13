/**
 * MorningLetterService client for Connect-RPC
 *
 * Provides type-safe methods to call MorningLetterService endpoints for
 * time-bounded RAG-powered chat about recent news.
 */

import { createClient, ConnectError, Code } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	MorningLetterService,
	MorningLetterReadService,
	type StreamChatResponse,
	type Citation as ProtoCitation,
	type MorningLetterDocument,
	type MorningLetterSourceProto,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";

/** Type-safe MorningLetterService client */
type MorningLetterClient = Client<typeof MorningLetterService>;

// =============================================================================
// Types
// =============================================================================

/**
 * Citation/source reference from MorningLetter responses
 */
export interface MorningLetterCitation {
	url: string;
	title: string;
	publishedAt: string;
}

/**
 * Time window for filtering articles
 */
export interface MorningLetterTimeWindow {
	since: string;
	until: string;
}

/**
 * Chat message for MorningLetter conversation
 */
export interface MorningLetterChatMessage {
	role: "user" | "assistant";
	content: string;
}

/**
 * Options for streaming MorningLetter chat
 */
export interface MorningLetterStreamOptions {
	/** Chat message history (alternating user/assistant messages) */
	messages: MorningLetterChatMessage[];
	/** Time window in hours to filter articles (default: 24, max: 168) */
	withinHours?: number;
}

/**
 * Metadata about the response
 */
export interface MorningLetterMeta {
	citations: MorningLetterCitation[];
	timeWindow?: MorningLetterTimeWindow;
	articlesScanned: number;
}

/**
 * Result when streaming chat completes successfully
 */
export interface MorningLetterStreamResult {
	/** The full answer text */
	answer: string;
	/** Citations used in the answer */
	citations: MorningLetterCitation[];
}

// =============================================================================
// Client
// =============================================================================

/**
 * Creates a MorningLetterService client with the given transport.
 */
export function createMorningLetterClient(
	transport: Transport,
): MorningLetterClient {
	return createClient(MorningLetterService, transport);
}

/**
 * Stream MorningLetter chat with callback-based event handling.
 *
 * Similar to Augur but with time-bounded filtering for recent news.
 *
 * @param transport - The Connect transport to use
 * @param options - Chat options including message history and time filter
 * @param onDelta - Callback for text chunks (optional)
 * @param onMeta - Callback for metadata including citations and time window (optional)
 * @param onComplete - Callback when streaming completes (optional)
 * @param onFallback - Callback for fallback events (optional)
 * @param onError - Callback for errors (optional)
 * @returns AbortController to cancel the stream
 */
export function streamMorningLetterChat(
	transport: Transport,
	options: MorningLetterStreamOptions,
	onDelta?: (text: string) => void,
	onMeta?: (meta: MorningLetterMeta) => void,
	onComplete?: (result: MorningLetterStreamResult) => void,
	onFallback?: (code: string) => void,
	onError?: (error: Error) => void,
): AbortController {
	const abortController = new AbortController();
	const client = createMorningLetterClient(transport);

	// Track accumulated text for complete result
	let accumulatedText = "";
	let latestCitations: MorningLetterCitation[] = [];

	// Start streaming in background
	(async () => {
		try {
			const stream = client.streamChat(
				{
					messages: options.messages.map((m) => ({
						role: m.role,
						content: m.content,
					})),
					withinHours: options.withinHours ?? 24,
				},
				{ signal: abortController.signal },
			);

			for await (const rawEvent of stream) {
				const event = rawEvent as StreamChatResponse;
				const { payload } = event;

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
						if (payload.value) {
							const citations = payload.value.citations.map(
								(c: ProtoCitation) => ({
									url: c.url,
									title: c.title,
									publishedAt: c.publishedAt,
								}),
							);
							latestCitations = citations;
							if (onMeta) {
								onMeta({
									citations,
									timeWindow: payload.value.timeWindow
										? {
												since: payload.value.timeWindow.since,
												until: payload.value.timeWindow.until,
											}
										: undefined,
									articlesScanned: payload.value.articlesScanned,
								});
							}
						}
						break;

					case "done":
						if (payload.value) {
							const citations = payload.value.citations.map(
								(c: ProtoCitation) => ({
									url: c.url,
									title: c.title,
									publishedAt: c.publishedAt,
								}),
							);
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
			if (
				!abortController.signal.aborted &&
				onError &&
				error instanceof Error
			) {
				onError(error);
			}
		}
	})();

	return abortController;
}

/**
 * Stream MorningLetter chat with Promise-based interface.
 *
 * Simpler interface when you just need the final result.
 *
 * @param transport - The Connect transport to use
 * @param options - Chat options including message history and time filter
 * @param onDelta - Optional callback for real-time text updates
 * @returns Promise that resolves with the complete result
 */
export async function streamMorningLetterChatAsync(
	transport: Transport,
	options: MorningLetterStreamOptions,
	onDelta?: (text: string) => void,
): Promise<MorningLetterStreamResult> {
	return new Promise((resolve, reject) => {
		streamMorningLetterChat(
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
// MorningLetterReadService Client
// =============================================================================

/**
 * Get the latest morning letter document.
 * Returns null if no letter exists (NotFound).
 * Rethrows Unauthenticated and other errors.
 */
export async function getLatestLetter(
	transport: Transport,
): Promise<MorningLetterDocument | null> {
	const client = createClient(MorningLetterReadService, transport);
	try {
		const res = await client.getLatestLetter({});
		return res.letter ?? null;
	} catch (err) {
		if (err instanceof ConnectError && err.code === Code.NotFound) {
			return null;
		}
		throw err;
	}
}

/**
 * Get a morning letter by civil date (YYYY-MM-DD).
 * Returns null if no letter exists for that date (NotFound).
 * Rethrows Unauthenticated and other errors.
 */
export async function getLetterByDate(
	transport: Transport,
	targetDate: string,
): Promise<MorningLetterDocument | null> {
	const client = createClient(MorningLetterReadService, transport);
	try {
		const res = await client.getLetterByDate({ targetDate });
		return res.letter ?? null;
	} catch (err) {
		if (err instanceof ConnectError && err.code === Code.NotFound) {
			return null;
		}
		throw err;
	}
}

/**
 * Trigger on-demand regeneration of the caller's Morning Letter.
 * Rate-limited server-side to one request per hour per user; the returned
 * `regenerated` flag is false when the call served a cached letter instead
 * of invoking the projector.
 */
export async function regenerateLatestLetter(
	transport: Transport,
	editionTimezone?: string,
): Promise<{
	letter: MorningLetterDocument | null;
	regenerated: boolean;
	retryAfterSeconds: number;
}> {
	const client = createClient(MorningLetterReadService, transport);
	const res = await client.regenerateLatest({
		editionTimezone: editionTimezone ?? undefined,
	});
	return {
		letter: res.letter ?? null,
		regenerated: res.regenerated,
		retryAfterSeconds: res.retryAfterSeconds,
	};
}

/**
 * Fetch per-bullet enrichment for a letter: article alt-href, original URL,
 * feed title, tags, related articles, Augur chat seed link, summary excerpt.
 * Capped server-side; absence means "no richer info available yet", not an
 * error. Returns [] on NotFound so the UI can degrade gracefully.
 */
export async function getLetterEnrichment(
	transport: Transport,
	letterId: string,
) {
	const client = createClient(MorningLetterReadService, transport);
	try {
		const res = await client.getLetterEnrichment({ letterId });
		return res.enrichments;
	} catch (err) {
		if (err instanceof ConnectError && err.code === Code.NotFound) {
			return [];
		}
		throw err;
	}
}

/**
 * Get article provenance sources for a letter.
 * Returns null if letter not found (NotFound).
 * Rethrows Unauthenticated and other errors.
 */
export async function getLetterSources(
	transport: Transport,
	letterId: string,
): Promise<MorningLetterSourceProto[] | null> {
	const client = createClient(MorningLetterReadService, transport);
	try {
		const res = await client.getLetterSources({ letterId });
		return res.sources;
	} catch (err) {
		if (err instanceof ConnectError && err.code === Code.NotFound) {
			return null;
		}
		throw err;
	}
}
