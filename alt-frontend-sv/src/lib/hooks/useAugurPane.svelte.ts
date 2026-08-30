/**
 * Chat state hook for the Ask Augur inline pane.
 *
 * Encapsulates message state, streaming lifecycle, and abort logic.
 * Reuses streamAugurChat() from $lib/connect.
 */

import {
	type AugurCitation,
	createClientTransport,
	streamAugurChat,
} from "$lib/connect";
import { prefersReducedMotion } from "$lib/stores/motion.svelte";
import { formatAugurFallbackMessage } from "$lib/utils/augurFallback";
import {
	createTypewriterReveal,
	type TypewriterReveal,
} from "$lib/utils/typewriterReveal";

type CitationKindName = "UNSPECIFIED" | "WEB" | "ARTICLE" | "SUMMARY";

type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Score?: number;
	Kind?: CitationKindName;
	RefID?: string;
};

export type AugurPaneMessage = {
	id: string;
	message: string;
	role: "user" | "assistant";
	timestamp: string;
	citations?: Citation[];
	relatedCitations?: Citation[];
	/** When true, AskSheet (and peers) may offer a retry action. */
	retryable?: boolean;
};

const STREAM_TIMEOUT_MS = 180_000;

function convertCitations(citations: AugurCitation[] | undefined): Citation[] {
	if (!citations) return [];
	return citations.map((c) => ({
		URL: c.url,
		Title: c.title,
		PublishedAt: c.publishedAt,
		Kind: c.kind,
		RefID: c.refId,
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
	// The paced reveal for the assistant turn in flight — the same shared
	// engine AugurChat uses (ADR-000985: one place for everyone). It is the
	// only writer of the streaming bubble's text; stopping it is what makes a
	// straggler delta after abort/timeout harmless.
	let reveal: TypewriterReveal | null = null;
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
		reveal?.stop();
		clearTransientState();
		runPendingReset();
	}

	function abort() {
		if (currentAbortController) {
			currentAbortController.abort();
			currentAbortController = null;
		}
		reveal?.stop();
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

		// Build history excluding the empty assistant placeholder, and excluding
		// any turn that failed. A fallback or error bubble is our own apology
		// text, not something the model produced; replaying it as assistant
		// history teaches the model that declining to answer is the expected
		// shape of a reply, so one infrastructure failure degrades every later
		// turn in the conversation.
		const chatHistory = messages
			.slice(0, -1)
			.filter((m) => !m.retryable)
			.map((m) => ({
				role: m.role,
				content: m.message,
			}));

		let bufferedContent = "";

		// One reveal per turn; it alone writes the streaming bubble's text, so
		// the loading pulse (gated on message === "") clears with the first
		// revealed character.
		reveal?.stop();
		reveal = createTypewriterReveal({
			immediate: prefersReducedMotion(),
			onReveal: (text) => {
				messages[assistantIndex] = {
					...messages[assistantIndex]!,
					message: text,
				};
			},
		});

		// Timeout: auto-recover if onComplete never fires (e.g., protobuf failure)
		streamTimeout = setTimeout(() => {
			if (isLoading) {
				// Stopped before the notice is written: a reveal frame still in
				// flight must not overwrite the recovery text.
				reveal?.stop();
				messages[assistantIndex] = {
					...messages[assistantIndex]!,
					message: bufferedContent || "Response timed out. Please try again.",
					retryable: true,
				};
				finalize();
			}
		}, STREAM_TIMEOUT_MS);

		const transport = createClientTransport();
		currentAbortController = streamAugurChat(
			transport,
			{ messages: chatHistory, conversationId },
			// onDelta — provisional preview text, paced by the reveal
			(delta) => {
				bufferedContent += delta;
				isProvisional = true;
				reveal?.push(delta);
			},
			// onThinking — update status text for UI
			(text) => {
				statusText = text;
			},
			// onMeta
			(citations) => {
				messages[assistantIndex] = {
					...messages[assistantIndex]!,
					citations: convertCitations(citations),
				};
			},
			// onComplete — authoritative final answer replaces all provisional text
			(result) => {
				// Hard flush with the same value the write below uses, so the
				// revealed text and the message state never disagree.
				reveal?.finish(result.answer || bufferedContent);
				messages[assistantIndex] = {
					...messages[assistantIndex]!,
					message: result.answer || bufferedContent,
					citations:
						result.citations.length > 0
							? convertCitations(result.citations)
							: messages[assistantIndex]!.citations,
					relatedCitations: convertCitations(result.relatedCitations),
				};
				finalize();
			},
			// onFallback — clear provisional, show fallback
			(code) => {
				reveal?.stop();
				isProvisional = false;
				messages[assistantIndex] = {
					...messages[assistantIndex]!,
					message: formatAugurFallbackMessage(code),
					retryable: true,
				};
				finalize();
			},
			// onError
			(error) => {
				reveal?.stop();
				isProvisional = false;
				messages[assistantIndex] = {
					...messages[assistantIndex]!,
					message: `Error: ${error.message}. Please try again.`,
					retryable: true,
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
