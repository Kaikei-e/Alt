<script lang="ts">
import { onMount, tick } from "svelte";
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
	targetDate?: string;
};

let { withinHours = 24, targetDate }: Props = $props();

const welcomeMessage = $derived(
	`Ask follow-up questions about today's edition.`,
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
let threadContainer: HTMLDivElement;
let currentAbortController: AbortController | null = null;

async function scrollToBottom() {
	await tick();
	if (threadContainer) {
		setTimeout(() => {
			threadContainer.scrollTop = threadContainer.scrollHeight;
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
			(_code) => {
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex],
					message:
						"I couldn't find enough recent news to answer that. Try asking about a different topic from today's news.",
				};
				isLoading = false;
				currentAbortController = null;
				scrollToBottom();
			},
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

<div class="letter-chat">
	<!-- Thread -->
	<div bind:this={threadContainer} class="letter-thread" role="log" aria-live="polite" aria-busy={isLoading}>
		{#each messages as msg, idx (msg.id)}
			<article class="thread-entry" data-role={msg.role}>
				{#if msg.role === "user"}
					<h3 class="entry-question">{msg.message}</h3>
				{:else}
					<!-- Byline -->
					<div class="entry-byline">
						<img src={augurAvatar} alt="Morning Letter" class="byline-avatar" />
						<span class="byline-name">Morning Letter</span>
					</div>

					<!-- Prose -->
					{#if msg.message}
						<div class="entry-prose">
							{@html parseMarkdown(msg.message)}
						</div>
					{/if}

					<!-- Meta -->
					{#if msg.meta?.articlesScanned}
						<span class="entry-meta">{msg.meta.articlesScanned} articles scanned</span>
					{/if}

					<!-- Citations -->
					{#if msg.citations && msg.citations.length > 0}
						<div class="entry-sources">
							<span class="sources-heading">Sources</span>
							<ul class="sources-list">
								{#each msg.citations as cite, i}
									<li>
										<a
											href={cite.URL}
											target="_blank"
											rel="noopener noreferrer"
											class="source-link"
										>
											<span class="source-id">[{i + 1}]</span>
											<span class="source-title">{cite.Title || "Untitled"}</span>
										</a>
									</li>
								{/each}
							</ul>
						</div>
					{/if}
				{/if}
			</article>
		{/each}

		{#if isLoading && messages[messages.length - 1]?.message === ""}
			<div class="letter-loading">
				<div class="loading-pulse"></div>
				<span class="loading-text">Searching recent news&hellip;</span>
			</div>
		{/if}
	</div>

	<!-- Input -->
	<div class="letter-input-area">
		<div class="input-meta">
			Searching news from the past {withinHours} hours
		</div>
		<div class="flex gap-2">
			<textarea
				bind:value={inputValue}
				onkeydown={handleKeydown}
				placeholder="Ask about today's edition..."
				disabled={isLoading}
				rows={1}
				class="input-field"
				aria-label="Question input"
			></textarea>
			<button
				onclick={handleSend}
				disabled={isLoading || !inputValue.trim()}
				class="input-submit"
				aria-label="Send"
			>
				Submit
			</button>
		</div>
	</div>
</div>

<style>
	/* ===== Container ===== */
	.letter-chat {
		display: flex;
		flex-direction: column;
		height: calc(100vh - 12rem);
		max-width: 720px;
		margin: 0 auto;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
		overflow: hidden;
	}

	/* ===== Thread ===== */
	.letter-thread {
		flex: 1;
		overflow-y: auto;
		padding: 1rem;
	}

	/* ===== Thread Entry ===== */
	.thread-entry {
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.thread-entry:last-child {
		border-bottom: none;
	}

	/* ===== User Question ===== */
	.entry-question {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}

	/* ===== Assistant Byline ===== */
	.entry-byline {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		margin-bottom: 0.4rem;
	}

	.byline-avatar {
		width: 24px;
		height: 24px;
		object-fit: cover;
		border: 1px solid var(--surface-border, #c8c8c8);
		filter: saturate(0.85);
	}

	.byline-name {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	/* ===== Prose ===== */
	.entry-prose {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem;
		line-height: 1.72;
		color: var(--alt-charcoal, #1a1a1a);
		max-width: 65ch;
	}

	.entry-prose :global(p) { margin: 0 0 0.5rem; }
	.entry-prose :global(strong) { font-weight: 600; }
	.entry-prose :global(a) {
		color: var(--alt-primary, #2f4f4f);
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	/* ===== Meta ===== */
	.entry-meta {
		display: block;
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
		margin-top: 0.5rem;
	}

	/* ===== Sources ===== */
	.entry-sources {
		margin-top: 0.75rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	.sources-heading {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.3rem;
	}

	.sources-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.source-link {
		display: flex;
		align-items: baseline;
		gap: 0.35rem;
		text-decoration: none;
	}

	.source-link:hover .source-title {
		text-decoration: underline;
	}

	.source-id {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
		flex-shrink: 0;
	}

	.source-title {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem;
		color: var(--alt-primary, #2f4f4f);
		text-underline-offset: 2px;
	}

	/* ===== Loading ===== */
	.letter-loading {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 1rem 0;
		color: var(--alt-ash, #999);
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		font-style: italic;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	/* ===== Input Area ===== */
	.letter-input-area {
		flex-shrink: 0;
		padding: 0.75rem 1rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
	}

	.input-meta {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
		margin-bottom: 0.5rem;
	}

	.input-field {
		flex: 1;
		min-height: 44px;
		max-height: 120px;
		padding: 0.5rem 0.75rem;
		resize: none;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 1rem;
		line-height: 1.5;
		color: var(--alt-charcoal, #1a1a1a);

		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		outline: none;
		transition: border-color 0.15s;
	}

	.input-field:focus {
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.input-field:disabled {
		opacity: 0.5;
	}

	.input-submit {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		padding: 0.5rem 1rem;
		flex-shrink: 0;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;

		color: var(--alt-charcoal, #1a1a1a);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.input-submit:hover:not(:disabled) {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}

	.input-submit:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse { animation: none; opacity: 0.6; }
	}
</style>
