<script lang="ts">
import { onMount, tick } from "svelte";
import { Loader2, User, Clock, Newspaper, Send } from "@lucide/svelte";
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

let messages = $state<Message[]>([
	{
		id: "welcome",
		message: `Hello! Ask me about today's news.`,
		role: "assistant",
		timestamp: new Date().toLocaleTimeString(),
	},
]);

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

	if (currentAbortController) {
		currentAbortController.abort();
		currentAbortController = null;
	}

	inputValue = "";

	const userMessage: Message = {
		id: `user-${Date.now()}`,
		message: messageText,
		role: "user",
		timestamp: new Date().toLocaleTimeString(),
	};

	messages = [...messages, userMessage];
	await scrollToBottom();

	isLoading = true;

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

	let bufferedContent = "";
	let lastUpdateTime = 0;
	const THROTTLE_MS = 50;

	try {
		const transport = createClientTransport();

		const chatHistory = messages.slice(0, -1).map((m) => ({
			role: m.role as "user" | "assistant",
			content: m.message,
		}));

		currentAbortController = streamMorningLetterChat(
			transport,
			{ messages: chatHistory, withinHours },
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
			(code) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message: "I couldn't find enough recent news to answer that.",
				};
				isLoading = false;
				currentAbortController = null;
				scrollToBottom();
			},
			(error) => {
				console.error("Chat error:", error);
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message: `Error: ${error.message}`,
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
			message: `Error: ${error instanceof Error ? error.message : "Unknown error"}`,
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

<div class="flex flex-col h-full" style="background: var(--app-bg);">
	<!-- Header -->
	<div
		class="sticky top-0 z-10 p-4 border-b"
		style="background: var(--surface-bg); border-color: var(--border-color);"
	>
		<h1 class="text-lg font-bold" style="color: var(--text-primary);">
			Morning Letter
		</h1>
		<div class="flex items-center gap-2 text-xs mt-1" style="color: var(--text-secondary);">
			<Clock class="h-3 w-3" />
			<span>News from the past {withinHours} hours</span>
		</div>
	</div>

	<!-- Messages -->
	<div
		bind:this={chatContainer}
		class="flex-1 overflow-y-auto p-4 space-y-4"
	>
		{#each messages as msg (msg.id)}
			<div
				class={cn(
					"flex gap-2",
					msg.role === "user" ? "justify-end" : "justify-start",
				)}
			>
				{#if msg.role === "assistant"}
					<div
						class="flex-shrink-0 w-7 h-7 rounded-full overflow-hidden mt-1"
						style="background: var(--surface-bg);"
					>
						<img
							src={augurAvatar}
							alt="AI"
							class="w-full h-full object-cover"
						/>
					</div>
				{/if}

				<div class="max-w-[80%] flex flex-col gap-1">
					<div
						class={cn(
							"px-3 py-2 text-sm rounded-2xl",
							msg.role === "user"
								? "rounded-br-sm"
								: "rounded-bl-sm",
						)}
						style={msg.role === "user"
							? "background: var(--alt-primary); color: var(--text-primary);"
							: "background: var(--surface-bg); color: var(--text-primary); border: 1px solid var(--border-color);"}
					>
						{#if msg.role === "user"}
							<p class="whitespace-pre-wrap break-words">{msg.message}</p>
						{:else}
							<div class="whitespace-pre-wrap break-words prose prose-sm max-w-none">
								{@html parseMarkdown(msg.message)}
							</div>
						{/if}
					</div>

					{#if msg.role === "assistant" && msg.meta?.articlesScanned}
						<div
							class="flex items-center gap-1 text-xs px-1"
							style="color: var(--text-secondary);"
						>
							<Newspaper class="h-3 w-3" />
							<span>{msg.meta.articlesScanned} articles</span>
						</div>
					{/if}

					{#if msg.role === "assistant" && msg.citations && msg.citations.length > 0}
						<div
							class="rounded-lg p-2 text-xs"
							style="background: var(--surface-bg); border: 1px solid var(--border-color);"
						>
							<p class="font-semibold mb-1" style="color: var(--text-secondary);">Sources:</p>
							<ul class="space-y-1">
								{#each msg.citations.slice(0, 3) as cite, i}
									<li>
										<a
											href={cite.URL}
											target="_blank"
											rel="noopener noreferrer"
											class="hover:underline flex gap-1 items-start"
											style="color: var(--text-secondary);"
										>
											<span class="font-mono shrink-0">[{i + 1}]</span>
											<span class="line-clamp-1">{cite.Title || "Untitled"}</span>
										</a>
									</li>
								{/each}
								{#if msg.citations.length > 3}
									<li style="color: var(--text-secondary);">
										+{msg.citations.length - 3} more sources
									</li>
								{/if}
							</ul>
						</div>
					{/if}
				</div>

				{#if msg.role === "user"}
					<div
						class="flex-shrink-0 w-7 h-7 rounded-full flex items-center justify-center mt-1"
						style="background: var(--alt-primary);"
					>
						<User class="h-4 w-4" style="color: var(--text-primary);" />
					</div>
				{/if}
			</div>
		{/each}

		{#if isLoading}
			<div class="flex gap-2">
				<div
					class="flex-shrink-0 w-7 h-7 rounded-full overflow-hidden relative"
					style="background: var(--surface-bg);"
				>
					<img src={augurAvatar} alt="AI" class="w-full h-full object-cover opacity-50" />
					<div class="absolute inset-0 flex items-center justify-center">
						<Loader2 class="h-4 w-4 animate-spin" style="color: var(--alt-primary);" />
					</div>
				</div>
				<div
					class="px-3 py-2 text-sm rounded-2xl rounded-bl-sm"
					style="background: var(--surface-bg); border: 1px solid var(--border-color);"
				>
					<span style="color: var(--text-secondary);">Searching...</span>
				</div>
			</div>
		{/if}
	</div>

	<!-- Input -->
	<div
		class="sticky bottom-0 p-3 border-t"
		style="background: var(--surface-bg); border-color: var(--border-color);"
	>
		<div class="flex gap-2">
			<input
				type="text"
				bind:value={inputValue}
				onkeydown={handleKeydown}
				placeholder="Ask about today's news..."
				disabled={isLoading}
				class="flex-1 px-4 py-2 rounded-full border focus:outline-none focus:ring-2 disabled:opacity-50"
				style="background: var(--app-bg); border-color: var(--border-color); color: var(--text-primary); font-size: 16px;"
			/>
			<button
				onclick={handleSend}
				disabled={isLoading || !inputValue.trim()}
				class="flex-shrink-0 w-10 h-10 rounded-full flex items-center justify-center disabled:opacity-50"
				style="background: var(--alt-primary);"
				aria-label="Send"
			>
				<Send class="h-4 w-4" style="color: var(--text-primary);" />
			</button>
		</div>
	</div>
</div>
