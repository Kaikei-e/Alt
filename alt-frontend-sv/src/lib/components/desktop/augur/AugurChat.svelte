<script lang="ts">
import { onMount, tick } from "svelte";
import { Loader2 } from "@lucide/svelte";
import ChatMessage from "./ChatMessage.svelte";
import ChatInput from "./ChatInput.svelte";
import {
	createClientTransport,
	streamAugurChat,
	type AugurCitation,
} from "$lib/connect";
import augurAvatar from "$lib/assets/augur-chat.webp";
import { formatAugurFallbackMessage } from "$lib/utils/augurFallback";

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

let messages = $state<Message[]>([
	{
		id: "welcome",
		message: "Hello! I'm Augur. Ask me anything about your RSS feeds.",
		role: "assistant",
		timestamp: new Date().toLocaleTimeString(),
	},
]);

let isLoading = $state(false);
let progressStage = $state<string>("");
let statusText = $state("");
let isProvisional = $state(false);
let chatContainer: HTMLDivElement;
let currentAbortController: AbortController | null = null;
let lastAutoSentQuestion = $state("");

// Auto-scroll: throttled, suppressed when user scrolls up
let lastScrollTime = 0;
const SCROLL_THROTTLE_MS = 500;
let userScrolledUp = false;

function handleScroll() {
	if (!chatContainer) return;
	const { scrollTop, scrollHeight, clientHeight } = chatContainer;
	userScrolledUp = scrollHeight - scrollTop - clientHeight > 100;
}

async function scrollToBottom() {
	await tick();
	if (chatContainer) {
		setTimeout(() => {
			chatContainer.scrollTop = chatContainer.scrollHeight;
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
			return "Planning search...";
		case "searching":
			return "Searching evidence...";
		case "reranking":
			return "Checking evidence quality...";
		case "drafting":
			return "Drafting answer...";
		case "validating":
			return "Validating answer...";
		case "refining":
			return "Refining answer...";
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

		// Build message history (excluding the empty placeholder and welcome message)
		const chatHistory = messages
			.slice(0, -1)
			.filter((m) => m.id !== "welcome")
			.map((m) => ({
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

<div class="flex flex-col h-[calc(100vh-12rem)] max-w-4xl mx-auto border border-border bg-background rounded-lg overflow-hidden">
	<!-- Chat messages -->
	<div bind:this={chatContainer} class="flex-1 overflow-y-auto p-6" onscroll={handleScroll}>
		{#each messages as msg, idx (msg.id)}
			<ChatMessage
				message={msg.message}
				role={msg.role}
				timestamp={msg.timestamp}
				citations={msg.citations}
			/>
			{#if idx === messages.length - 1 && msg.role === "assistant" && isLoading && isProvisional && statusText}
				<div class="mb-4 ml-11 text-xs text-muted-foreground">
					{statusText}
				</div>
			{/if}
		{/each}

		{#if isLoading && messages[messages.length - 1]?.message === ""}
			<div class="flex gap-3 mb-4">
				<div class="flex-shrink-0 w-8 h-8 rounded-full overflow-hidden bg-muted mt-1 shadow-sm border border-border/50 relative">
					<img src={augurAvatar} alt="Augur" class="w-full h-full object-cover" />
					<div class="absolute inset-0 bg-background/40 flex items-center justify-center">
						<Loader2 class="h-4 w-4 text-primary animate-spin" />
					</div>
				</div>
				<div class="bg-muted/50 p-3 text-sm rounded-2xl rounded-bl-none shadow-sm border border-border/50">
					<p class="text-muted-foreground">
						{statusText || stageStatus(progressStage) || "Augur is thinking..."}
					</p>
				</div>
			</div>
		{/if}
	</div>

	<!-- Input -->
	<ChatInput onSend={handleSend} disabled={isLoading} {initialContext} />
</div>
