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

let chatOpen = $state(false);

let messages = $state<Message[]>([
	{
		id: "welcome",
		message: `Ask follow-up questions about today's edition.`,
		role: "assistant",
		timestamp: new Date().toLocaleTimeString(),
	},
]);

let isLoading = $state(false);
let inputValue = $state("");
let chatContainer = $state<HTMLDivElement | undefined>(undefined);
let currentAbortController: AbortController | null = null;

async function scrollToBottom() {
	await tick();
	const el = chatContainer;
	if (el) {
		setTimeout(() => {
			el.scrollTop = el.scrollHeight;
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

<div style="background: var(--app-bg);">
	<!-- Disclosure toggle -->
	<button
		onclick={() => { chatOpen = !chatOpen; }}
		aria-expanded={chatOpen}
		aria-controls="follow-up-chat"
		class="disclosure-toggle"
	>
		<div class="flex items-center gap-2">
			<span class="toggle-label">Follow-Up</span>
			<span class="toggle-hint">Ask about the briefing</span>
		</div>
		<svg
			class="toggle-chevron"
			class:toggle-chevron--open={chatOpen}
			viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"
		>
			<path d="m6 9 6 6 6-6" />
		</svg>
	</button>

	{#if chatOpen}
	<div id="follow-up-chat" role="region" class="flex flex-col" style="height: 50dvh;">
		<!-- Thread -->
		<div
			bind:this={chatContainer}
			class="letter-thread"
			role="log"
			aria-live="polite"
			aria-busy={isLoading}
		>
			{#each messages as msg (msg.id)}
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
							<span class="entry-meta">{msg.meta.articlesScanned} articles</span>
						{/if}

						<!-- Citations -->
						{#if msg.citations && msg.citations.length > 0}
							<div class="entry-sources">
								<span class="sources-heading">Sources</span>
								<ul class="sources-list">
									{#each msg.citations.slice(0, 3) as cite, i}
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
									{#if msg.citations.length > 3}
										<li class="source-overflow">
											+{msg.citations.length - 3} more
										</li>
									{/if}
								</ul>
							</div>
						{/if}
					{/if}
				</article>
			{/each}

			{#if isLoading && messages[messages.length - 1]?.message === ""}
				<div class="letter-loading">
					<div class="loading-pulse"></div>
					<span class="loading-text">Searching&hellip;</span>
				</div>
			{/if}
		</div>

		<!-- Input -->
		<div class="letter-input-area">
			<div class="flex gap-2">
				<input
					type="text"
					bind:value={inputValue}
					onkeydown={handleKeydown}
					placeholder="Ask about the briefing..."
					disabled={isLoading}
					class="input-field"
					aria-label="Question input"
				/>
				<button
					onclick={handleSend}
					disabled={isLoading || !inputValue.trim()}
					class="input-submit"
					aria-label="Send"
				>
					Send
				</button>
			</div>
		</div>
	</div>
	{/if}
</div>

<style>
	/* ===== Disclosure Toggle ===== */
	.disclosure-toggle {
		width: 100%;
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.75rem 1rem;
		min-height: 44px;

		background: var(--surface-bg, #faf9f7);
		border: none;
		border-top: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer;
	}

	.toggle-label {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-charcoal, #1a1a1a);
	}

	.toggle-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	.toggle-chevron {
		width: 1rem;
		height: 1rem;
		color: var(--alt-ash, #999);
		transition: transform 0.2s;
	}

	.toggle-chevron--open {
		transform: rotate(180deg);
	}

	/* ===== Thread ===== */
	.letter-thread {
		flex: 1;
		overflow-y: auto;
		padding: 0.75rem 1rem;
	}

	/* ===== Thread Entry ===== */
	.thread-entry {
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.thread-entry:last-child {
		border-bottom: none;
	}

	/* ===== User Question ===== */
	.entry-question {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.95rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}

	/* ===== Byline ===== */
	.entry-byline {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin-bottom: 0.3rem;
	}

	.byline-avatar {
		width: 20px;
		height: 20px;
		object-fit: cover;
		border: 1px solid var(--surface-border, #c8c8c8);
		filter: saturate(0.85);
	}

	.byline-name {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	/* ===== Prose ===== */
	.entry-prose {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.9rem;
		line-height: 1.65;
		color: var(--alt-charcoal, #1a1a1a);
	}

	.entry-prose :global(p) { margin: 0 0 0.4rem; }
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
		font-size: 0.6rem;
		color: var(--alt-ash, #999);
		margin-top: 0.35rem;
	}

	/* ===== Sources ===== */
	.entry-sources {
		margin-top: 0.5rem;
		padding-top: 0.4rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	.sources-heading {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.55rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.2rem;
	}

	.sources-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}

	.source-link {
		display: flex;
		align-items: baseline;
		gap: 0.25rem;
		text-decoration: none;
	}

	.source-link:hover .source-title {
		text-decoration: underline;
	}

	.source-id {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
		flex-shrink: 0;
	}

	.source-title {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
		color: var(--alt-primary, #2f4f4f);
		text-underline-offset: 2px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.source-overflow {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem;
		color: var(--alt-ash, #999);
	}

	/* ===== Loading ===== */
	.letter-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.75rem 0;
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
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	/* ===== Input Area ===== */
	.letter-input-area {
		flex-shrink: 0;
		padding: 0.5rem 0.75rem;
		padding-bottom: calc(0.5rem + env(safe-area-inset-bottom, 0px));
		border-top: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
	}

	.input-field {
		flex: 1;
		min-height: 44px;
		padding: 0.5rem 0.75rem;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 1rem;
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
		min-width: 44px;
		padding: 0.5rem 0.75rem;
		flex-shrink: 0;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem;
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
		.toggle-chevron { transition: none; }
	}
</style>
