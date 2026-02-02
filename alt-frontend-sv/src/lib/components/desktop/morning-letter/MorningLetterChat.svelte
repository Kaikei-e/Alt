<script lang="ts">
import { onMount, tick } from "svelte";
import { Loader2, User, Clock, Newspaper } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import { Textarea } from "$lib/components/ui/textarea";
import { cn } from "$lib/utils";
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import {
	createClientTransport,
	streamMorningLetterChat,
	type MorningLetterCitation,
	type MorningLetterMeta,
} from "$lib/connect";
import augurAvatar from "$lib/assets/augur-chat.webp";

type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
};

type Message = {
	id: string;
	message: string;
	role: "user" | "assistant";
	timestamp: string;
	citations?: Citation[];
	meta?: {
		timeWindow?: { since: string; until: string };
		articlesScanned?: number;
	};
};

type Props = {
	withinHours?: number;
};

let { withinHours = 24 }: Props = $props();

const welcomeMessage = $derived(
	`Hello! I'm your Morning Letter assistant. I can answer questions about news from the past ${withinHours} hours. What would you like to know?`
);

let messages = $state<Message[]>([]);

$effect(() => {
	if (messages.length === 0) {
		messages = [
			{
				id: "welcome",
				message: welcomeMessage,
				role: "assistant",
				timestamp: new Date().toLocaleTimeString(),
			},
		];
	}
});

let isLoading = $state(false);
let inputValue = $state("");
let chatContainer: HTMLDivElement;
let currentAbortController: AbortController | null = null;

async function scrollToBottom() {
	await tick();
	if (chatContainer) {
		setTimeout(() => {
			chatContainer.scrollTop = chatContainer.scrollHeight;
		}, 100);
	}
}

function convertCitations(citations: MorningLetterCitation[]): Citation[] {
	return citations.map((c) => ({
		URL: c.url,
		Title: c.title,
		PublishedAt: c.publishedAt,
	}));
}

async function handleSend() {
	const messageText = inputValue.trim();
	if (!messageText || isLoading) return;

	// Cancel any ongoing stream
	if (currentAbortController) {
		currentAbortController.abort();
		currentAbortController = null;
	}

	inputValue = "";

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

	try {
		const transport = createClientTransport();

		// Build message history (excluding the empty placeholder)
		const chatHistory = messages.slice(0, -1).map((m) => ({
			role: m.role as "user" | "assistant",
			content: m.message,
		}));

		currentAbortController = streamMorningLetterChat(
			transport,
			{ messages: chatHistory, withinHours },
			// onDelta: text chunk received
			(text) => {
				bufferedContent += text;

				const now = Date.now();
				if (now - lastUpdateTime > THROTTLE_MS) {
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex],
						message: bufferedContent,
					};
					lastUpdateTime = now;
				}
			},
			// onMeta: metadata received
			(meta: MorningLetterMeta) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					citations: convertCitations(meta.citations),
					meta: {
						timeWindow: meta.timeWindow,
						articlesScanned: meta.articlesScanned,
					},
				};
			},
			// onComplete: streaming finished
			(result) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message: result.answer || bufferedContent,
					citations:
						result.citations.length > 0
							? convertCitations(result.citations)
							: messages[currentAssistantMessageIndex].citations,
				};
				isLoading = false;
				currentAbortController = null;
				scrollToBottom();
			},
			// onFallback: insufficient context
			(code) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message:
						"I couldn't find enough recent news to answer that. Try asking about a different topic from today's news.",
				};
				isLoading = false;
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
				currentAbortController = null;
				scrollToBottom();
			},
		);
	} catch (error) {
		console.error("Chat error:", error);
		messages[currentAssistantMessageIndex] = {
			...messages[currentAssistantMessageIndex],
			message: `Error: ${error instanceof Error ? error.message : "Unknown error"}. Please try again.`,
		};
		isLoading = false;
		await scrollToBottom();
	}
}

function handleKeydown(event: KeyboardEvent) {
	if (event.key === "Enter" && !event.shiftKey) {
		event.preventDefault();
		handleSend();
	}
}

onMount(() => {
	scrollToBottom();

	return () => {
		if (currentAbortController) {
			currentAbortController.abort();
		}
	};
});
</script>

<div
	class="flex flex-col h-[calc(100vh-12rem)] max-w-4xl mx-auto border border-border bg-background rounded-lg overflow-hidden"
>
	<!-- Chat messages -->
	<div bind:this={chatContainer} class="flex-1 overflow-y-auto p-6">
		{#each messages as msg (msg.id)}
			<div
				class={cn(
					"flex gap-3 mb-4",
					msg.role === "user" ? "justify-end" : "justify-start",
				)}
			>
				{#if msg.role === "assistant"}
					<div
						class="flex-shrink-0 w-8 h-8 rounded-full overflow-hidden bg-muted mt-1 shadow-sm border border-border/50"
					>
						<img
							src={augurAvatar}
							alt="Morning Letter"
							class="w-full h-full object-cover"
						/>
					</div>
				{/if}

				<div class="max-w-[70%] flex flex-col gap-2">
					<div
						class={cn(
							"p-3 text-sm rounded-2xl shadow-sm",
							msg.role === "user"
								? "bg-primary text-primary-foreground rounded-br-none"
								: "bg-muted/50 text-foreground rounded-bl-none border border-border/50",
						)}
					>
						{#if msg.role === "user"}
							<p class="whitespace-pre-wrap break-words leading-relaxed">
								{msg.message}
							</p>
						{:else}
							<div class="whitespace-pre-wrap break-words leading-relaxed">
								{@html parseMarkdown(msg.message)}
							</div>
						{/if}
						{#if msg.timestamp}
							<p class="text-xs mt-1 opacity-70">{msg.timestamp}</p>
						{/if}
					</div>

					{#if msg.role === "assistant" && msg.meta?.articlesScanned}
						<div
							class="flex items-center gap-2 text-xs text-muted-foreground px-2"
						>
							<Newspaper class="h-3 w-3" />
							<span>{msg.meta.articlesScanned} articles scanned</span>
						</div>
					{/if}

					{#if msg.role === "assistant" && msg.citations && msg.citations.length > 0}
						<div
							class="bg-muted/30 border border-border/50 rounded-lg p-3 text-xs text-muted-foreground"
						>
							<p class="font-semibold mb-2">Sources:</p>
							<ul class="space-y-2">
								{#each msg.citations as cite, i}
									<li>
										<a
											href={cite.URL}
											target="_blank"
											rel="noopener noreferrer"
											class="hover:text-foreground flex gap-2 group items-start"
										>
											<span class="font-mono opacity-70 shrink-0 mt-0.5"
												>[{i + 1}]</span
											>
											<span
												class="underline decoration-muted-foreground/50 group-hover:decoration-foreground underline-offset-4 break-words leading-relaxed"
											>
												{cite.Title || "Untitled Source"}
											</span>
										</a>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				</div>

				{#if msg.role === "user"}
					<div
						class="flex-shrink-0 w-8 h-8 rounded-full bg-muted mt-1 shadow-sm border border-border/50 flex items-center justify-center"
					>
						<div
							class="w-full h-full bg-primary/20 flex items-center justify-center rounded-full"
						>
							<User class="h-4 w-4 text-primary" />
						</div>
					</div>
				{/if}
			</div>
		{/each}

		{#if isLoading}
			<div class="flex gap-3 mb-4">
				<div
					class="flex-shrink-0 w-8 h-8 rounded-full overflow-hidden bg-muted mt-1 shadow-sm border border-border/50 relative"
				>
					<img
						src={augurAvatar}
						alt="Morning Letter"
						class="w-full h-full object-cover"
					/>
					<div
						class="absolute inset-0 bg-background/40 flex items-center justify-center"
					>
						<Loader2 class="h-4 w-4 text-primary animate-spin" />
					</div>
				</div>
				<div
					class="bg-muted/50 p-3 text-sm rounded-2xl rounded-bl-none shadow-sm border border-border/50"
				>
					<p class="text-muted-foreground">Searching recent news...</p>
				</div>
			</div>
		{/if}
	</div>

	<!-- Input -->
	<div class="border-t border-border bg-background p-4">
		<div class="flex items-center gap-2 text-xs text-muted-foreground mb-3 px-1">
			<Clock class="h-3 w-3" />
			<span>Searching news from the past {withinHours} hours</span>
		</div>
		<div class="flex gap-2">
			<Textarea
				bind:value={inputValue}
				onkeydown={handleKeydown}
				placeholder="Ask about today's news..."
				class="flex-1 resize-none min-h-[44px] max-h-[120px] rounded-full border-border/50 bg-muted/30"
				disabled={isLoading}
				rows={1}
			/>
			<Button
				onclick={handleSend}
				disabled={isLoading || !inputValue.trim()}
				class="flex-shrink-0 px-4"
				aria-label="Send message"
			>
				<svg
					xmlns="http://www.w3.org/2000/svg"
					width="16"
					height="16"
					viewBox="0 0 24 24"
					fill="none"
					stroke="currentColor"
					stroke-width="2"
					stroke-linecap="round"
					stroke-linejoin="round"
				>
					<path d="M14.536 21.686a.5.5 0 0 0 .937-.024l6.5-19a.496.496 0 0 0-.635-.635l-19 6.5a.5.5 0 0 0-.024.937l7.93 3.18a2 2 0 0 1 1.112 1.11z" />
					<path d="m21.854 2.147-10.94 10.939" />
				</svg>
			</Button>
		</div>
		<p class="text-xs text-muted-foreground mt-2">
			Press Enter to send, Shift+Enter for new line
		</p>
	</div>
</div>
