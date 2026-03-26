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

const STREAM_TIMEOUT_MS = 60_000;

function convertCitations(citations: AugurCitation[]): Citation[] {
	return citations.map((c) => ({
		URL: c.url,
		Title: c.title,
		PublishedAt: c.publishedAt,
	}));
}

function fallbackMessage(code: string): string {
	if (code.includes("insufficient")) {
		return "Not enough indexed content for this article yet. Please try again later.";
	}
	return "I couldn't find enough information to answer that properly.";
}

export function useAugurPane() {
	let messages = $state<AugurPaneMessage[]>([]);
	let isLoading = $state(false);
	let progressStage = $state("");
	let currentAbortController: AbortController | null = null;
	let streamTimeout: ReturnType<typeof setTimeout> | null = null;

	function clearStreamTimeout() {
		if (streamTimeout !== null) {
			clearTimeout(streamTimeout);
			streamTimeout = null;
		}
	}

	function finalize() {
		isLoading = false;
		progressStage = "";
		currentAbortController = null;
		clearStreamTimeout();
	}

	function abort() {
		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}
		isLoading = false;
		progressStage = "";
		clearStreamTimeout();
	}

	function reset() {
		abort();
		messages = [];
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
					message:
						bufferedContent || "Response timed out. Please try again.",
				};
				finalize();
			}
		}, STREAM_TIMEOUT_MS);

		const transport = createClientTransport();
		currentAbortController = streamAugurChat(
			transport,
			{ messages: chatHistory },
			// onDelta
			(delta) => {
				bufferedContent += delta;
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: bufferedContent,
				};
			},
			// onThinking (unused)
			undefined,
			// onMeta
			(citations) => {
				messages[assistantIndex] = {
					...messages[assistantIndex],
					citations: convertCitations(citations),
				};
			},
			// onComplete
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
			// onFallback
			(code) => {
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: fallbackMessage(code),
				};
				finalize();
			},
			// onError
			(error) => {
				messages[assistantIndex] = {
					...messages[assistantIndex],
					message: `Error: ${error.message}. Please try again.`,
				};
				finalize();
			},
			// onProgress
			(stage) => {
				progressStage = stage;
			},
		);
	}

	return {
		get messages() {
			return messages;
		},
		get isLoading() {
			return isLoading;
		},
		get progressStage() {
			return progressStage;
		},
		sendMessage,
		abort,
		reset,
	};
}
