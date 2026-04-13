/**
 * AugurService client for Connect-RPC
 *
 * Provides type-safe methods to call AugurService endpoints for RAG-powered chat.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";
import {
	AugurService,
	type StreamChatResponse,
	type RetrieveContextResponse,
	type Citation as ProtoCitation,
	type ContextItem as ProtoContextItem,
	type ConversationSummary as ProtoConversationSummary,
	type ChatMessage as ProtoChatMessage,
} from "$lib/gen/alt/augur/v2/augur_pb";

/** Type-safe AugurService client */
type AugurClient = Client<typeof AugurService>;

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
	/**
	 * Conversation id to append to. Empty/undefined means a brand-new chat;
	 * the server will mint an id and echo it back via the first meta event.
	 */
	conversationId?: string;
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
export function createAugurClient(transport: Transport): AugurClient {
	return createClient(AugurService, transport);
}

/**
 * Stream Augur chat with callback-based event handling.
 *
 * Provides fine-grained control over different event types:
 * - delta: Text chunks as they arrive
 * - thinking: Reasoning/thinking chunks as they arrive
 * - meta: Citations/sources for the response
 * - done: Final complete response
 * - fallback: Fallback reason when RAG context is insufficient
 * - error: Error messages
 *
 * @param transport - The Connect transport to use
 * @param options - Chat options including message history
 * @param onDelta - Callback for text chunks (optional)
 * @param onThinking - Callback for thinking/reasoning chunks (optional)
 * @param onMeta - Callback for citations (optional)
 * @param onComplete - Callback when streaming completes (optional)
 * @param onFallback - Callback for fallback events (optional)
 * @param onError - Callback for errors (optional)
 * @param onProgress - Callback for progress stage updates e.g. "searching", "generating" (optional)
 * @param onConversationId - Callback when the server confirms the persisted conversation id (optional)
 * @returns AbortController to cancel the stream
 */
export function streamAugurChat(
	transport: Transport,
	options: AugurStreamOptions,
	onDelta?: (text: string) => void,
	onThinking?: (text: string) => void,
	onMeta?: (citations: AugurCitation[]) => void,
	onComplete?: (result: AugurStreamResult) => void,
	onFallback?: (code: string) => void,
	onError?: (error: Error) => void,
	onProgress?: (stage: string) => void,
	onConversationId?: (conversationId: string) => void,
): AbortController {
	const abortController = new AbortController();
	const client = createAugurClient(transport);

	// Track accumulated text for complete result
	let accumulatedText = "";
	let latestCitations: AugurCitation[] = [];
	let completeCalled = false;

	// Start streaming in background
	(async () => {
		try {
			const stream = client.streamChat(
				{
					messages: options.messages.map((m) => ({
						role: m.role,
						content: m.content,
					})),
					conversationId: options.conversationId ?? "",
				},
				{ signal: abortController.signal },
			);

			let conversationIdNotified = false;

			for await (const rawEvent of stream) {
				const event = rawEvent as StreamChatResponse;
				const { payload } = event;

				// Heartbeat events keep the connection alive through Cloudflare Tunnel
				if (event.kind === "heartbeat") {
					continue;
				}

				// Progress events reuse delta payload as carrier — always skip, never treat as content delta
				if (event.kind === "progress") {
					if (onProgress && payload.case === "delta" && payload.value) {
						onProgress(payload.value);
					}
					continue;
				}

				switch (payload.case) {
					case "delta":
						if (payload.value) {
							accumulatedText += payload.value;
							if (onDelta) {
								onDelta(payload.value);
							}
						}
						break;

					case "thinkingDelta":
						if (payload.value && onThinking) {
							onThinking(payload.value);
						}
						break;

					case "meta":
						if (payload.value) {
							if (
								payload.value.conversationId &&
								!conversationIdNotified &&
								onConversationId
							) {
								conversationIdNotified = true;
								onConversationId(payload.value.conversationId);
							}
							if (payload.value.citations?.length) {
								const citations = payload.value.citations.map(
									(c: ProtoCitation) => ({
										url: c.url,
										title: c.title,
										publishedAt: c.publishedAt,
									}),
								);
								latestCitations = citations;
								if (onMeta) {
									onMeta(citations);
								}
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
							// Use final answer from done payload if available
							const finalAnswer = payload.value.answer || accumulatedText;
							if (onComplete && !completeCalled) {
								completeCalled = true;
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
			if (
				!abortController.signal.aborted &&
				onComplete &&
				accumulatedText &&
				!completeCalled
			) {
				completeCalled = true;
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
 * Stream Augur chat with Promise-based interface.
 *
 * Simpler interface when you just need the final result.
 *
 * @param transport - The Connect transport to use
 * @param options - Chat options including message history
 * @param onDelta - Optional callback for real-time text updates
 * @param onThinking - Optional callback for reasoning/thinking updates
 * @returns Promise that resolves with the complete result
 */
export async function streamAugurChatAsync(
	transport: Transport,
	options: AugurStreamOptions,
	onDelta?: (text: string) => void,
	onThinking?: (text: string) => void,
): Promise<AugurStreamResult> {
	return new Promise((resolve, reject) => {
		streamAugurChat(
			transport,
			options,
			onDelta,
			onThinking,
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
	const response = (await client.retrieveContext({
		query: options.query,
		limit: options.limit ?? 5,
	})) as RetrieveContextResponse;

	return response.contexts.map((c: ProtoContextItem) => ({
		url: c.url,
		title: c.title,
		publishedAt: c.publishedAt,
		score: c.score,
	}));
}

// =============================================================================
// Conversation History
// =============================================================================

/** Row shape returned by ListConversations. Times are JS Date. */
export interface AugurConversationSummary {
	id: string;
	title: string;
	createdAt: Date | null;
	lastActivityAt: Date | null;
	lastMessagePreview: string;
	messageCount: number;
}

/** Full message as surfaced by GetConversation. */
export interface AugurStoredMessage {
	role: "user" | "assistant";
	content: string;
	createdAt: Date | null;
	citations: AugurCitation[];
}

/** Full conversation payload from GetConversation. */
export interface AugurStoredConversation {
	id: string;
	title: string;
	createdAt: Date | null;
	messages: AugurStoredMessage[];
}

function protoTimestampToDate(
	ts: { seconds: bigint; nanos: number } | undefined,
): Date | null {
	if (!ts) return null;
	const ms = Number(ts.seconds) * 1000 + Math.floor(ts.nanos / 1_000_000);
	return new Date(ms);
}

/**
 * List the caller's Ask Augur chat history (most recent first).
 * pageSize defaults to 20; pass pageToken from the previous response to page.
 */
export async function listAugurConversations(
	transport: Transport,
	options: { pageSize?: number; pageToken?: string } = {},
): Promise<{ conversations: AugurConversationSummary[]; nextPageToken: string }> {
	const client = createAugurClient(transport);
	const response = await client.listConversations({
		pageSize: options.pageSize ?? 20,
		pageToken: options.pageToken ?? "",
	});
	return {
		conversations: response.conversations.map(
			(c: ProtoConversationSummary) => ({
				id: c.id,
				title: c.title,
				createdAt: protoTimestampToDate(c.createdAt),
				lastActivityAt: protoTimestampToDate(c.lastActivityAt),
				lastMessagePreview: c.lastMessagePreview,
				messageCount: c.messageCount,
			}),
		),
		nextPageToken: response.nextPageToken,
	};
}

/** Load a single conversation including every stored turn. */
export async function getAugurConversation(
	transport: Transport,
	id: string,
): Promise<AugurStoredConversation> {
	const client = createAugurClient(transport);
	const response = await client.getConversation({ id });
	return {
		id: response.id,
		title: response.title,
		createdAt: protoTimestampToDate(response.createdAt),
		messages: response.messages.map((m: ProtoChatMessage) => ({
			role: (m.role === "assistant" ? "assistant" : "user") as
				| "user"
				| "assistant",
			content: m.content,
			createdAt: protoTimestampToDate(m.createdAt),
			citations: (m.citations ?? []).map((c: ProtoCitation) => ({
				url: c.url,
				title: c.title,
				publishedAt: c.publishedAt,
			})),
		})),
	};
}

/** Destructive delete — row + cascading messages are removed. */
export async function deleteAugurConversation(
	transport: Transport,
	id: string,
): Promise<void> {
	const client = createAugurClient(transport);
	await client.deleteConversation({ id });
}
