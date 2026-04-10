<script lang="ts">
import { onMount, tick } from "svelte";
import ThreadEntry from "./ThreadEntry.svelte";
import QuestionInput from "./QuestionInput.svelte";
import {
	createClientTransport,
	streamAugurChat,
	type AugurCitation,
} from "$lib/connect";
import { formatAugurFallbackMessage } from "$lib/utils/augurFallback";
import augurAvatar from "$lib/assets/augur-chat.webp";

interface Props {
	initialContext?: string;
	initialQuestion?: string;
}

const { initialContext = "", initialQuestion = "" }: Props = $props();

type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Score?: number;
};

type Message = {
	id: string;
	message: string;
	role: "user" | "assistant";
	timestamp: string;
	citations?: Citation[];
};

let messages = $state<Message[]>([]);

let isLoading = $state(false);
let progressStage = $state<string>("");
let statusText = $state("");
let isProvisional = $state(false);
let threadContainer: HTMLDivElement;
let currentAbortController: AbortController | null = null;
let lastAutoSentQuestion = $state("");
let revealed = $state(false);

let hasMessages = $derived(messages.length > 0);

// Auto-scroll: throttled, suppressed when user scrolls up
let lastScrollTime = 0;
const SCROLL_THROTTLE_MS = 500;
let userScrolledUp = false;

function handleScroll() {
	if (!threadContainer) return;
	const { scrollTop, scrollHeight, clientHeight } = threadContainer;
	userScrolledUp = scrollHeight - scrollTop - clientHeight > 100;
}

async function scrollToBottom() {
	await tick();
	if (threadContainer) {
		setTimeout(() => {
			threadContainer.scrollTop = threadContainer.scrollHeight;
		}, 100);
	}
}

function throttledScrollToBottom() {
	if (userScrolledUp) return;
	const now = Date.now();
	if (now - lastScrollTime > SCROLL_THROTTLE_MS) {
		lastScrollTime = now;
		scrollToBottom();
	}
}

/**
 * Convert AugurCitation from Connect-RPC to component Citation format
 */
function convertCitations(citations: AugurCitation[]): Citation[] {
	return citations.map((c) => ({
		URL: c.url,
		Title: c.title,
		PublishedAt: c.publishedAt,
	}));
}

function stageStatus(stage: string): string {
	switch (stage) {
		case "planning":
			return "Planning search\u2026";
		case "searching":
			return "Searching evidence\u2026";
		case "reranking":
			return "Checking evidence quality\u2026";
		case "drafting":
			return "Drafting answer\u2026";
		case "validating":
			return "Validating answer\u2026";
		case "refining":
			return "Refining answer\u2026";
		default:
			return "";
	}
}

async function handleSend(messageText: string) {
	// Cancel any ongoing stream
	if (currentAbortController) {
		currentAbortController.abort();
		currentAbortController = null;
	}

	// Add user message
	const userMessage: Message = {
		id: `user-${Date.now()}`,
		message: messageText,
		role: "user",
		timestamp: new Date().toLocaleTimeString(),
	};

	messages = [...messages, userMessage];
	await scrollToBottom();

	isLoading = true;
	statusText = "";
	isProvisional = false;

	// Add placeholder for assistant message
	messages = [
		...messages,
		{
			id: `assistant-${Date.now()}`,
			message: "",
			role: "assistant",
			timestamp: new Date().toLocaleTimeString(),
		},
	];
	const currentAssistantMessageIndex = messages.length - 1;

	// Throttling state for delta updates
	let bufferedContent = "";
	let lastUpdateTime = 0;
	const THROTTLE_MS = 50;

	progressStage = "";
	userScrolledUp = false;

	try {
		const transport = createClientTransport();

		// Build message history (excluding the empty placeholder)
		const chatHistory = messages.slice(0, -1).map((m) => ({
			role: m.role as "user" | "assistant",
			content: m.message,
		}));

		currentAbortController = streamAugurChat(
			transport,
			{ messages: chatHistory },
			// onDelta: text chunk received
			(text) => {
				progressStage = "";
				bufferedContent += text;
				isProvisional = true;

				const now = Date.now();
				if (now - lastUpdateTime > THROTTLE_MS) {
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex],
						message: bufferedContent,
					};
					lastUpdateTime = now;
					throttledScrollToBottom();
				}
			},
			// onThinking: update live status text
			(text) => {
				statusText = text;
			},
			// onMeta: citations received
			(citations) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					citations: convertCitations(citations),
				};
			},
			// onComplete: streaming finished
			(result) => {
				// Ensure final content is rendered
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message: result.answer || bufferedContent,
					citations:
						result.citations.length > 0
							? convertCitations(result.citations)
							: messages[currentAssistantMessageIndex].citations,
				};
				isLoading = false;
				progressStage = "";
				statusText = "";
				isProvisional = false;
				currentAbortController = null;
				scrollToBottom();
			},
			// onFallback: insufficient context
			(code) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message: formatAugurFallbackMessage(code),
				};
				isLoading = false;
				progressStage = "";
				statusText = "";
				isProvisional = false;
				currentAbortController = null;
				scrollToBottom();
			},
			// onError: error occurred
			(error) => {
				console.error("Chat error:", error);
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message: `Error: ${error.message}. Please try again.`,
				};
				isLoading = false;
				progressStage = "";
				statusText = "";
				isProvisional = false;
				currentAbortController = null;
				scrollToBottom();
			},
			// onProgress: stage updates
			(stage) => {
				progressStage = stage;
				statusText = stageStatus(stage);
			},
		);
	} catch (error) {
		console.error("Chat error:", error);
		messages[currentAssistantMessageIndex] = {
			...messages[currentAssistantMessageIndex],
			message: `Error: ${error instanceof Error ? error.message : "Unknown error"}. Please try again.`,
		};
		isLoading = false;
		statusText = "";
		isProvisional = false;
		await scrollToBottom();
	}
}

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
	scrollToBottom();

	// Cleanup on unmount
	return () => {
		if (currentAbortController) {
			currentAbortController.abort();
		}
	};
});

$effect(() => {
	if (!initialQuestion.trim()) {
		return;
	}
	if (initialQuestion === lastAutoSentQuestion) {
		return;
	}
	lastAutoSentQuestion = initialQuestion;
	void handleSend(initialQuestion);
});
</script>

<div class="augur-column" class:revealed>
	{#if !hasMessages}
		<!-- Empty state: the invitation -->
		<div class="augur-empty">
			<div class="empty-presence">
				<img src={augurAvatar} alt="Augur" class="empty-avatar" />
				<p class="empty-title">Ask Augur</p>
				<div class="empty-rule"></div>
			</div>
			<div class="empty-input">
				<QuestionInput onSend={handleSend} disabled={isLoading} {initialContext} />
			</div>
		</div>
	{:else}
		<!-- Active state: the consultation -->
		<div bind:this={threadContainer} class="augur-thread" onscroll={handleScroll}>
			{#each messages as msg, idx (msg.id)}
				<ThreadEntry
					message={msg.message}
					role={msg.role}
					timestamp={msg.timestamp}
					citations={msg.citations}
					index={idx}
				/>
				{#if idx === messages.length - 1 && msg.role === "assistant" && isLoading && isProvisional && statusText}
					<div class="stage-hint">
						{statusText}
					</div>
				{/if}
			{/each}

			{#if isLoading && messages[messages.length - 1]?.message === ""}
				<div class="augur-loading">
					<div class="loading-pulse"></div>
					<span class="loading-text">{statusText || stageStatus(progressStage) || "Consulting the evidence\u2026"}</span>
				</div>
			{/if}
		</div>

		<QuestionInput onSend={handleSend} disabled={isLoading} {initialContext} />
	{/if}
</div>

<style>
	/* ===== Column layout ===== */
	.augur-column {
		max-width: 720px; margin: 0 auto;
		padding: 0 1rem;
		display: flex; flex-direction: column;
		height: calc(100vh - 5rem);
		opacity: 0; transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}
	.augur-column.revealed { opacity: 1; transform: translateY(0); }

	/* ===== Empty state: the invitation ===== */
	.augur-empty {
		display: flex; flex-direction: column;
		align-items: center; justify-content: center;
		flex: 1;
		gap: 0;
	}
	.empty-presence {
		display: flex; flex-direction: column;
		align-items: center;
		gap: 0.6rem;
		margin-bottom: 2rem;
	}
	.empty-avatar {
		width: 48px; height: 48px;
		object-fit: cover;
		border: 1px solid var(--alt-charcoal, #1a1a1a);
		filter: saturate(0.85);
	}
	.empty-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.3rem; font-weight: 600; font-style: italic;
		color: var(--alt-slate, #666);
		margin: 0;
	}
	.empty-rule {
		width: 120px; height: 1px;
		background: var(--surface-border, #c8c8c8);
	}
	.empty-input {
		width: 100%; max-width: 560px;
	}

	/* ===== Thread (active state) ===== */
	.augur-thread {
		flex: 1; overflow-y: auto;
		padding: 0.5rem 0;
	}

	/* ===== Loading state ===== */
	.augur-loading {
		display: flex; align-items: center; gap: 0.75rem;
		justify-content: center; padding: 2rem;
		color: var(--alt-ash, #999);
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
	}
	.loading-pulse {
		width: 8px; height: 8px; border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse { 0%, 100% { opacity: 0.3; } 50% { opacity: 1; } }
	.loading-text {
		font-style: italic;
	}

	/* ===== Stage hint ===== */
	.stage-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem; font-style: italic;
		color: var(--alt-ash, #999);
		padding: 0.25rem 0 0.75rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.augur-column { transition: none; opacity: 1; transform: none; }
	}
</style>
