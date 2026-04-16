/**
 * Chat state hook for the Ask Augur inline pane.
 *
 * Encapsulates message state, streaming lifecycle, and abort logic.
 * Reuses streamAugurChat() from $lib/connect.
 */

import {
	createClientTransport,
	streamAugurChat,
	type AugurCitation,
} from "$lib/connect";
import { formatAugurFallbackMessage } from "$lib/utils/augurFallback";

type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Score?: number;
};

export type AugurPaneMessage = {
	id: string;
	message: string;
	role: "user" | "assistant";
	timestamp: string;
	citations?: Citation[];
};

const STREAM_TIMEOUT_MS = 180_000;

function convertCitations(citations: AugurCitation[]): Citation[] {
	return citations.map((c) => ({
		URL: c.url,
		Title: c.title,
		PublishedAt: c.publishedAt,
	}));
}

export interface UseAugurPaneOptions {
	/** Pre-populate the pane (e.g. when resuming a persisted conversation). */
	initialMessages?: AugurPaneMessage[];
	/** Existing persisted conversation id to append to. */
	initialConversationId?: string;
	/**
	 * Called once the server confirms the persisted conversation id for a new
	 * chat. Consumers typically use it to update the URL (e.g. /augur/<id>).
	 */
	onConversationIdChange?: (conversationId: string) => void;
}

export function useAugurPane(options: UseAugurPaneOptions = {}) {
	let messages = $state<AugurPaneMessage[]>(options.initialMessages ?? []);
	let conversationId = $state<string>(options.initialConversationId ?? "");
	let isLoading = $state(false);
	let progressStage = $state("");
	let statusText = $state("");
	let isProvisional = $state(false);
	let currentAbortController: AbortController | null = null;
	let streamTimeout: ReturnType<typeof setTimeout> | null = null;
	// When reset() is invoked mid-stream we defer the cleanup so the Connect
	// stream can complete and rag-orchestrator can persist the partial turn.
	// AskSheet's close-on-dismiss previously aborted the stream, orphaning the
	// conversation row with zero messages.
	let pendingReset = false;

	function clearStreamTimeout() {
		if (streamTimeout !== null) {
			clearTimeout(streamTimeout);
			streamTimeout = null;
		}
	}

	function clearTransientState() {
		isLoading = false;
		progressStage = "";
		statusText = "";
		isProvisional = false;
		clearStreamTimeout();
	}

	function runPendingReset() {
		if (!pendingReset) return;
		pendingReset = false;
		messages = [];
		conversationId = "";
	}

	function finalize() {
		currentAbortController = null;
		clearTransientState();
		runPendingReset();
	}

	function abort() {
		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}
		clearTransientState();
		runPendingReset();
	}

	function reset() {
		if (isLoading) {
			// Defer the clear until the stream finalizes so the backend can
			// commit the partial assistant turn. finalize()/abort() will run
			// runPendingReset() once the stream actually ends.
			pendingReset = true;
			return;
		}
		clearTransientState();
		currentAbortController = null;
		messages = [];
		conversationId = "";
	}

	function sendMessage(text: string) {
		// Abort any ongoing stream
		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}
		clearStreamTimeout();

		// Add user message
		const userMessage: AugurPaneMessage = {
			id: `user-${Date.now()}`,
			message: text,
			role: "user",
			timestamp: new Date().toLocaleTimeString(),
		};

		// Add assistant placeholder
		const assistantMessage: AugurPaneMessage = {
			id: `assistant-${Date.now()}`,
			message: "",
			role: "assistant",
			timestamp: new Date().toLocaleTimeString(),
		};

		messages = [...messages, userMessage, assistantMessage];
		const assistantIndex = messages.length - 1;

		isLoading = true;
		progressStage = "";

		// Build history excluding the empty assistant placeholder
		const chatHistory = messages.slice(0, -1).map((m) => ({
			role: m.role,
			content: m.message,
		}));

		let bufferedContent = "";

		// Timeout: auto-recover if onComplete never fires (e.g., protobuf failure)
		streamTimeout = setTimeout(() => {
			if (isLoading) {
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: bufferedContent || "Response timed out. Please try again.",
				};
				finalize();
			}
		}, STREAM_TIMEOUT_MS);

		const transport = createClientTransport();
		currentAbortController = streamAugurChat(
			transport,
			{ messages: chatHistory, conversationId },
			// onDelta — provisional preview text
			(delta) => {
				bufferedContent += delta;
				isProvisional = true;
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: bufferedContent,
				};
			},
			// onThinking — update status text for UI
			(text) => {
				statusText = text;
			},
			// onMeta
			(citations) => {
				messages[assistantIndex] = {
					...messages[assistantIndex],
					citations: convertCitations(citations),
				};
			},
			// onComplete — authoritative final answer replaces all provisional text
			(result) => {
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: result.answer || bufferedContent,
					citations:
						result.citations.length > 0
							? convertCitations(result.citations)
							: messages[assistantIndex].citations,
				};
				finalize();
			},
			// onFallback — clear provisional, show fallback
			(code) => {
				isProvisional = false;
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: formatAugurFallbackMessage(code),
				};
				finalize();
			},
			// onError
			(error) => {
				isProvisional = false;
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: `Error: ${error.message}. Please try again.`,
				};
				finalize();
			},
			// onProgress — update stage + statusText for refining
			(stage) => {
				progressStage = stage;
				if (stage === "refining") {
					statusText = "Refining answer...";
				}
			},
			// onConversationId — server confirmed the persisted id
			(id) => {
				if (!id || id === conversationId) return;
				const isNewChat = conversationId === "";
				conversationId = id;
				if (isNewChat) {
					options.onConversationIdChange?.(id);
				}
			},
		);
	}

	return {
		get messages() {
			return messages;
		},
		get conversationId() {
			return conversationId;
		},
		get isLoading() {
			return isLoading;
		},
		get progressStage() {
			return progressStage;
		},
		get statusText() {
			return statusText;
		},
		get isProvisional() {
			return isProvisional;
		},
		sendMessage,
		abort,
		reset,
	};
}
