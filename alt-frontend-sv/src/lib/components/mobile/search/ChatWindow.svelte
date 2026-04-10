<script lang="ts">
import { tick } from "svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import {
	createClientTransport,
	streamAugurChat,
	type AugurCitation,
} from "$lib/connect";
import { formatAugurFallbackMessage } from "$lib/utils/augurFallback";
import augurAvatar from "$lib/assets/augur-chat.webp";

type Citation = {
	url: string;
	title: string;
	publishedAt: string;
};

type Message = {
	role: "user" | "assistant";
	content: string;
	citations?: Citation[];
};

interface Props {
	initialContext?: string;
	initialQuestion?: string;
}

const { initialContext = "", initialQuestion = "" }: Props = $props();

// State
let messages: Message[] = $state([]);
let inputValue = $state("");
let isLoading = $state(false);
let progressStage = $state<string>("");
let statusText = $state("");
let isProvisional = $state(false);
let messagesEndRef: HTMLDivElement;
let messagesContainer: HTMLDivElement;
let lastAutoSentQuestion = $state("");

// Auto-scroll: throttled, suppressed when user scrolls up
let lastScrollTime = 0;
const SCROLL_THROTTLE_MS = 500;
let userScrolledUp = false;

function handleScroll() {
	if (!messagesContainer) return;
	const { scrollTop, scrollHeight, clientHeight } = messagesContainer;
	userScrolledUp = scrollHeight - scrollTop - clientHeight > 100;
}

// Auto-scroll to bottom
const scrollToBottom = async () => {
	await tick();
	if (messagesEndRef) {
		messagesEndRef.scrollIntoView({ behavior: "smooth" });
	}
};

function throttledScrollToBottom() {
	if (userScrolledUp) return;
	const now = Date.now();
	if (now - lastScrollTime > SCROLL_THROTTLE_MS) {
		lastScrollTime = now;
		scrollToBottom();
	}
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

function handleKeydown(event: KeyboardEvent) {
	if (event.isComposing) return;
	if (event.key === "Enter" && !event.shiftKey) {
		event.preventDefault();
		handleSubmit();
	}
}

const handleSubmit = async (messageOverride?: string) => {
	const userMessage = (messageOverride ?? inputValue).trim();
	if (!userMessage || isLoading) return;

	if (!messageOverride) {
		inputValue = "";
	}

	// Add user message
	messages = [...messages, { role: "user", content: userMessage }];
	await scrollToBottom();

	isLoading = true;
	statusText = "";
	isProvisional = false;

	// Add placeholder for assistant message
	messages = [...messages, { role: "assistant", content: "" }];
	const currentAssistantMessageIndex = messages.length - 1;

	// Throttling state
	let bufferedContent = "";
	let lastUpdateTime = 0;
	const THROTTLE_MS = 50;

	progressStage = "";
	userScrolledUp = false;

	try {
		const transport = createClientTransport();

		// Prepare messages for Connect-RPC (exclude empty placeholder)
		const chatMessages = messages.slice(0, -1).map((m) => ({
			role: m.role,
			content: m.content,
		}));

		streamAugurChat(
			transport,
			{ messages: chatMessages },
			// onDelta: text chunks
			(text) => {
				progressStage = "";
				bufferedContent += text;
				isProvisional = true;

				const now = Date.now();
				if (now - lastUpdateTime > THROTTLE_MS) {
					const currentMsg = messages[currentAssistantMessageIndex];
					messages[currentAssistantMessageIndex] = {
						...currentMsg,
						content: bufferedContent,
					};
					lastUpdateTime = now;
					throttledScrollToBottom();
				}
			},
			// onThinking: update live status text
			(text) => {
				statusText = text;
			},
			// onMeta: citations
			(citations: AugurCitation[]) => {
				const cleanCitations: Citation[] = citations.map((c) => ({
					url: c.url,
					title: c.title,
					publishedAt: c.publishedAt,
				}));

				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					citations: cleanCitations,
				};
			},
			// onComplete: final result
			(result) => {
				// Ensure final content is rendered
				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					content: result.answer,
					citations: result.citations.map((c) => ({
						url: c.url,
						title: c.title,
						publishedAt: c.publishedAt,
					})),
				};
				isLoading = false;
				progressStage = "";
				statusText = "";
				isProvisional = false;
				scrollToBottom();
			},
			// onFallback: insufficient evidence
			(code) => {
				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					content: formatAugurFallbackMessage(code),
				};
				isLoading = false;
				progressStage = "";
				statusText = "";
				isProvisional = false;
				scrollToBottom();
			},
			// onError: error handling
			(error) => {
				console.error("Chat error:", error);
				const currentMsg = messages[currentAssistantMessageIndex];
				messages[currentAssistantMessageIndex] = {
					...currentMsg,
					content: "Sorry, something went wrong. Please try again.",
				};
				isLoading = false;
				progressStage = "";
				statusText = "";
				isProvisional = false;
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
		const currentMsg = messages[currentAssistantMessageIndex];
		messages[currentAssistantMessageIndex] = {
			...currentMsg,
			content: "Sorry, something went wrong. Please try again.",
		};
		isLoading = false;
		statusText = "";
		isProvisional = false;
		await scrollToBottom();
	}
};

$effect(() => {
	if (!initialQuestion.trim()) {
		if (messages.length === 0) {
			inputValue = initialContext;
		}
		return;
	}
	if (initialQuestion === lastAutoSentQuestion) {
		return;
	}
	lastAutoSentQuestion = initialQuestion;
	void handleSubmit(initialQuestion);
});
</script>

<div class="augur-mobile">
	<!-- Thread area -->
	<div bind:this={messagesContainer} class="augur-thread" onscroll={handleScroll}>
		{#if messages.length === 0}
			<div class="augur-empty">
				<img src={augurAvatar} alt="Augur" class="empty-avatar" />
				<p class="empty-title">Ask Augur</p>
				<div class="empty-rule"></div>
			</div>
		{/if}

		{#each messages as message, idx}
			<article class="thread-entry" data-role={message.role} style="--stagger: {idx}">
				{#if message.role === "user"}
					<h3 class="entry-question">{message.content}</h3>
				{:else}
					<div class="entry-byline">
						<img src={augurAvatar} alt="Augur" class="byline-avatar" />
						<span class="byline-name">Augur</span>
					</div>
					{#if !message.content && isLoading && !message.citations}
						<div class="augur-loading">
							<div class="loading-pulse"></div>
							<span class="loading-text">{statusText || stageStatus(progressStage) || "Consulting the evidence\u2026"}</span>
						</div>
					{:else}
						<div class="entry-prose">
							{@html parseMarkdown(message.content)}
						</div>
					{/if}

					{#if idx === messages.length - 1 && message.role === "assistant" && isLoading && isProvisional && statusText}
						<div class="stage-hint">{statusText}</div>
					{/if}

					{#if message.citations && message.citations.length > 0}
						<footer class="entry-sources">
							<h4 class="sources-heading">Sources</h4>
							<ol class="sources-list">
								{#each message.citations as cite, i}
									<li class="source-item">
										<span class="source-id">[{i + 1}]</span>
										<a href={cite.url} target="_blank" rel="noopener noreferrer" class="source-title">
											{cite.title || "Untitled Source"}
										</a>
									</li>
								{/each}
							</ol>
						</footer>
					{/if}
				{/if}

				<div class="entry-rule"></div>
			</article>
		{/each}
		<div bind:this={messagesEndRef}></div>
	</div>

	<FloatingMenu />

	<!-- Input Area -->
	<div class="augur-input-fixed">
		<div class="input-rule"></div>
		<form
			class="input-row"
			onsubmit={(e) => { e.preventDefault(); handleSubmit(); }}
		>
			<textarea
				class="input-field"
				bind:value={inputValue}
				onkeydown={handleKeydown}
				placeholder="What would you like to know?"
				disabled={isLoading}
				rows={1}
			></textarea>
			<button
				type="submit"
				class="input-submit"
				disabled={!inputValue.trim() || isLoading}
				aria-label="Submit question"
			>
				{#if isLoading}
					<span class="submit-loading"></span>
				{:else}
					&#8594;
				{/if}
			</button>
		</form>
	</div>
</div>

<style>
	.augur-mobile {
		display: flex; flex-direction: column;
		height: 100%;
		background: var(--surface-bg, #faf9f7);
		position: relative;
		overflow: hidden;
		overscroll-behavior: none;
	}

	/* ===== Thread ===== */
	.augur-thread {
		flex: 1; overflow-y: auto; overflow-x: hidden;
		padding: calc(0.5rem + env(safe-area-inset-top, 0px)) 1rem 1rem;
		overscroll-behavior-y: contain;
		-webkit-overflow-scrolling: touch;
	}

	/* ===== Empty state: invitation ===== */
	.augur-empty {
		display: flex; flex-direction: column;
		align-items: center; justify-content: center;
		width: 100%; height: 100%; gap: 0.5rem;
		text-align: center;
		box-sizing: border-box;
	}
	.empty-avatar {
		display: block;
		width: 40px; height: 40px;
		object-fit: cover;
		border: 1px solid var(--alt-charcoal, #1a1a1a);
		filter: saturate(0.85);
		margin: 0 auto;
	}
	.empty-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.1rem; font-weight: 600; font-style: italic;
		color: var(--alt-slate, #666);
		margin: 0;
	}
	.empty-rule {
		width: 80px; height: 1px;
		background: var(--surface-border, #c8c8c8);
	}

	/* ===== Thread entry ===== */
	.thread-entry {
		padding: 0.75rem 0;
		opacity: 0; animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}
	@keyframes entry-in { to { opacity: 1; } }

	/* Augur byline */
	.entry-byline {
		display: flex; align-items: center; gap: 0.35rem;
		margin-bottom: 0.3rem;
	}
	.byline-avatar {
		width: 20px; height: 20px;
		object-fit: cover;
		border: 1px solid var(--surface-border, #c8c8c8);
	}
	.byline-name {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem; font-weight: 600;
		letter-spacing: 0.08em; text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	/* User question */
	.entry-question {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: clamp(1rem, 3.5vw, 1.15rem);
		font-weight: 700; line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
		overflow-wrap: anywhere; word-break: break-word;
	}

	/* Assistant prose */
	.entry-prose {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.95rem; line-height: 1.72;
		color: var(--alt-charcoal, #1a1a1a);
		overflow-wrap: anywhere;
		word-break: break-word;
	}
	.entry-prose :global(h1) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.2rem; font-weight: 700; margin: 1.25rem 0 0.4rem; line-height: 1.25;
	}
	.entry-prose :global(h2) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.05rem; font-weight: 700; margin: 1rem 0 0.35rem; line-height: 1.3;
	}
	.entry-prose :global(h3) {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 0.9rem; font-weight: 700; margin: 0.75rem 0 0.25rem; line-height: 1.35;
	}
	.entry-prose :global(p) { margin: 0 0 0.65rem; line-height: 1.72; }
	.entry-prose :global(ul),
	.entry-prose :global(ol) { margin: 0.4rem 0 0.65rem; padding-left: 1.25rem; }
	.entry-prose :global(ul) { list-style-type: disc; }
	.entry-prose :global(ol) { list-style-type: decimal; }
	.entry-prose :global(li) { margin-bottom: 0.2rem; line-height: 1.6; }
	.entry-prose :global(blockquote) {
		border-left: 2px solid var(--alt-charcoal, #1a1a1a); padding-left: 0.6rem;
		margin: 0.5rem 0; font-style: italic; color: var(--alt-slate, #666);
	}
	.entry-prose :global(a) {
		color: var(--alt-primary, #2f4f4f); text-decoration: underline;
		text-decoration-thickness: 1px; text-underline-offset: 2px;
	}
	.entry-prose :global(a:hover) { color: var(--alt-charcoal, #1a1a1a); }
	.entry-prose :global(hr) { border: none; border-top: 1px solid var(--surface-border, #c8c8c8); margin: 1rem 0; }
	.entry-prose :global(pre) {
		background: var(--surface-2, #f5f4f1); padding: 0.6rem; overflow-x: auto;
		margin: 0.5rem 0; font-size: 0.8rem; line-height: 1.5;
		max-width: calc(100vw - 2rem);
	}
	.entry-prose :global(code) { font-family: var(--font-mono, "IBM Plex Mono", monospace); font-size: 0.85em; }
	.entry-prose :global(strong) { font-weight: 700; }

	/* Sources / citations */
	.entry-sources {
		margin-top: 0.75rem; padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}
	.sources-heading {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.55rem; font-weight: 700; letter-spacing: 0.12em;
		text-transform: uppercase; color: var(--alt-ash, #999);
		margin: 0 0 0.35rem;
	}
	.sources-list {
		list-style: none; padding: 0; margin: 0;
		display: flex; flex-direction: column; gap: 0.25rem;
	}
	.source-item {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; line-height: 1.5; color: var(--alt-slate, #666);
		display: flex; gap: 0.25rem; align-items: baseline;
	}
	.source-id {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem; font-weight: 600; color: var(--alt-charcoal, #1a1a1a);
		flex-shrink: 0;
	}
	.source-title {
		color: var(--alt-primary, #2f4f4f); text-decoration: underline;
		text-decoration-thickness: 1px; text-underline-offset: 2px;
		overflow-wrap: anywhere; word-break: break-word;
		min-width: 0;
	}
	.source-title:hover { color: var(--alt-charcoal, #1a1a1a); }

	/* Bottom rule separator */
	.entry-rule {
		height: 1px; background: var(--surface-border, #c8c8c8);
		margin-top: 0.75rem;
	}

	/* ===== Loading ===== */
	.augur-loading {
		display: flex; align-items: center; gap: 0.6rem;
		padding: 0.5rem 0;
		color: var(--alt-ash, #999);
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
	}
	.loading-pulse {
		width: 6px; height: 6px; border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse { 0%, 100% { opacity: 0.3; } 50% { opacity: 1; } }
	.loading-text { font-style: italic; }

	.stage-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.7rem; font-style: italic;
		color: var(--alt-ash, #999);
		padding: 0.15rem 0 0.5rem;
	}

	/* ===== Input area (flexbox, no positioning) ===== */
	.augur-input-fixed {
		flex-shrink: 0;
		background: var(--surface-bg, #faf9f7);
		padding: 0 1rem calc(0.75rem + env(safe-area-inset-bottom, 0px));
	}
	.input-rule {
		height: 1px; background: var(--surface-border, #c8c8c8);
		margin-bottom: 0.5rem;
	}
	.input-row {
		display: flex; gap: 0.5rem; align-items: flex-end;
	}
	.input-field {
		flex: 1;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 1rem; line-height: 1.4;
		padding: 0.5rem 0.6rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		border-radius: 0;
		background: transparent;
		color: var(--alt-charcoal, #1a1a1a);
		resize: none;
		min-height: 44px; max-height: 100px;
	}
	.input-field::placeholder {
		color: var(--alt-ash, #999); font-style: italic;
	}
	.input-field:focus {
		outline: none; border-color: var(--alt-charcoal, #1a1a1a);
	}
	.input-field:disabled {
		opacity: 0.5; cursor: not-allowed;
	}
	.input-submit {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 1.2rem; font-weight: 600;
		width: 44px; height: 44px;
		display: flex; align-items: center; justify-content: center;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		background: transparent;
		color: var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background-color 0.2s, color 0.2s;
		flex-shrink: 0;
	}
	.input-submit:active:not(:disabled) {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}
	.input-submit:disabled {
		opacity: 0.4; cursor: not-allowed;
	}
	.submit-loading {
		width: 6px; height: 6px; border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}

	@media (prefers-reduced-motion: reduce) {
		.thread-entry { animation: none; opacity: 1; }
	}
</style>
