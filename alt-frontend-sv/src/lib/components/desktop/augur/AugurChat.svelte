<script lang="ts">
/**
 * The Ask Augur consultation — one implementation, every viewport.
 *
 * This used to be two components: this one and `mobile/search/ChatWindow`,
 * written separately against the same `streamAugurChat`, and swapped by
 * `{#if isDesktop()}` in the route. Once that branch became genuinely reactive
 * (1f83bb5), turning a phone destroyed one chat and mounted the other, which
 * threw the thread away and re-sent the question the URL still carried: one
 * question, three answers billed. The layout difference — a 720px column with
 * a citation rail beside it, versus a full-bleed phone shell — is a difference
 * in width, so it is settled in CSS below and never by mounting a second chat.
 */
import { onDestroy, onMount, tick, untrack } from "svelte";
import augurAvatar from "$lib/assets/augur-chat.webp";
import {
	type AugurCitation,
	createClientTransport,
	streamAugurChat,
} from "$lib/connect";
import { formatAugurFallbackMessage } from "$lib/utils/augurFallback";
import CitationRail from "./CitationRail.svelte";
import ThreadEntry from "./ThreadEntry.svelte";

type CitationKindName = "UNSPECIFIED" | "WEB" | "ARTICLE" | "SUMMARY";

type Citation = {
	URL: string;
	Title: string;
	PublishedAt?: string;
	Score?: number;
	Kind?: CitationKindName;
	RefID?: string;
};

type Message = {
	id: string;
	message: string;
	role: "user" | "assistant";
	timestamp: string;
	citations?: Citation[];
	relatedCitations?: Citation[];
	// failed marks a bubble whose text we wrote ourselves (a fallback notice or
	// an error), so it can be shown to the reader without being replayed to the
	// model as something it said.
	failed?: boolean;
};

interface Props {
	initialContext?: string;
	initialQuestion?: string;
	initialMessages?: Message[];
	initialConversationId?: string;
	onConversationIdChange?: (id: string) => void;
}

const {
	initialContext = "",
	initialQuestion = "",
	initialMessages = [],
	initialConversationId = "",
	onConversationIdChange,
}: Props = $props();

let messages = $state<Message[]>(untrack(() => [...initialMessages]));
let conversationId = $state<string>(untrack(() => initialConversationId));

let inputValue = $state("");
let isLoading = $state(false);
let progressStage = $state<string>("");
let statusText = $state("");
let isProvisional = $state(false);
let threadContainer = $state<HTMLDivElement | undefined>(undefined);
let currentAbortController: AbortController | null = null;
let revealed = $state(false);

// A plain `let`, deliberately: the auto-send effect both reads and writes this,
// and `$state` would make it a dependency of the effect that maintains it.
let lastAutoSentQuestion = "";

let hasMessages = $derived(messages.length > 0);

// The latest assistant turn drives the right-column citation rail. Past
// answers' citations stay reachable through the inline footer in ThreadEntry.
let railCitations = $derived.by<Citation[]>(() => {
	for (let i = messages.length - 1; i >= 0; i--) {
		const m = messages[i]!;
		if (m.role === "assistant") {
			return m.citations ?? [];
		}
	}
	return [];
});

// Sibling "Related" rail driven by the same latest assistant turn. Empty
// arrays collapse the section so legacy conversations (no inline related
// projection) render exactly as before.
let railRelatedCitations = $derived.by<Citation[]>(() => {
	for (let i = messages.length - 1; i >= 0; i--) {
		const m = messages[i]!;
		if (m.role === "assistant") {
			return m.relatedCitations ?? [];
		}
	}
	return [];
});

let railActiveIndex = $state(-1);

function focusCitation(index: number) {
	railActiveIndex = index;
}

// Auto-scroll: throttled, suppressed when the reader scrolls up
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
	const container = threadContainer;
	if (container) {
		setTimeout(() => {
			container.scrollTop = container.scrollHeight;
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

function stageStatus(stage: string): string {
	switch (stage) {
		case "planning":
			return "Planning search…";
		case "searching":
			return "Searching evidence…";
		case "reranking":
			return "Checking evidence quality…";
		case "drafting":
			return "Drafting answer…";
		case "validating":
			return "Validating answer…";
		case "refining":
			return "Refining answer…";
		default:
			return "";
	}
}

function handleKeydown(event: KeyboardEvent) {
	// Ignore Enter during IME composition (e.g. Japanese input)
	if (event.isComposing) return;
	if (event.key === "Enter" && !event.shiftKey) {
		event.preventDefault();
		void handleSubmit();
	}
}

async function handleSubmit() {
	const typed = inputValue.trim();
	if (!typed || isLoading) return;
	inputValue = "";
	await handleSend(typed);
}

async function handleSend(messageText: string) {
	const text = messageText.trim();
	if (!text || isLoading) return;

	// Add user message
	const userMessage: Message = {
		id: `user-${Date.now()}`,
		message: text,
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

		// Build message history (excluding the empty placeholder), and excluding
		// any turn that failed. A fallback or error bubble is our own apology
		// text, not something the model produced.
		const chatHistory = messages
			.slice(0, -1)
			.filter((m) => !m.failed)
			.map((m) => ({
				role: m.role as "user" | "assistant",
				content: m.message,
			}));

		currentAbortController = streamAugurChat(
			transport,
			{ messages: chatHistory, conversationId },
			// onDelta: text chunk received
			(text) => {
				progressStage = "";
				bufferedContent += text;
				isProvisional = true;

				const now = Date.now();
				if (now - lastUpdateTime > THROTTLE_MS) {
					messages[currentAssistantMessageIndex] = {
						...messages[currentAssistantMessageIndex]!,
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
					...messages[currentAssistantMessageIndex]!,
					citations: convertCitations(citations),
				};
			},
			// onComplete: streaming finished
			(result) => {
				// Ensure final content is rendered
				messages[currentAssistantMessageIndex] = {
					...messages[currentAssistantMessageIndex]!,
					message: result.answer || bufferedContent,
					citations:
						result.citations.length > 0
							? convertCitations(result.citations)
							: messages[currentAssistantMessageIndex]!.citations,
					relatedCitations: convertCitations(result.relatedCitations),
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
					...messages[currentAssistantMessageIndex]!,
					message: formatAugurFallbackMessage(code),
					failed: true,
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
					...messages[currentAssistantMessageIndex]!,
					message: `Error: ${error.message}. Please try again.`,
					failed: true,
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
			// onConversationId: server confirms the persisted id
			(id) => {
				if (!id || id === conversationId) return;
				const wasNewChat = conversationId === "";
				conversationId = id;
				if (wasNewChat) {
					onConversationIdChange?.(id);
				}
			},
		);
	} catch (error) {
		console.error("Chat error:", error);
		messages[currentAssistantMessageIndex] = {
			...messages[currentAssistantMessageIndex]!,
			message: `Error: ${error instanceof Error ? error.message : "Unknown error"}. Please try again.`,
			failed: true,
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
});

// The stream is billed for as long as it runs, so it does not outlive the
// component that asked for it.
onDestroy(() => {
	currentAbortController?.abort();
	currentAbortController = null;
});

// A draft handed over in the URL (`?context=`) belongs in the box, but only
// while the box is still the start of the conversation — never on top of what
// the reader is typing in turn three.
$effect(() => {
	const draft = initialContext;
	untrack(() => {
		if (messages.length === 0) {
			inputValue = draft;
		}
	});
});

// A question handed over in the URL (`?q=`) is asked once. The guard survives a
// rotation because this component now does.
$effect(() => {
	const question = initialQuestion.trim();
	if (!question) return;
	if (question === lastAutoSentQuestion) return;
	lastAutoSentQuestion = question;
	untrack(() => {
		void handleSend(initialQuestion);
	});
});
</script>

<div class="augur-shell" class:revealed class:is-empty={!hasMessages}>
	<div class="augur-column">
		<div bind:this={threadContainer} class="augur-thread" onscroll={handleScroll}>
			{#if !hasMessages}
				<!-- Empty state: the invitation -->
				<div class="augur-empty">
					<img src={augurAvatar} alt="Augur" class="empty-avatar" />
					<p class="empty-title">Ask Augur</p>
					<div class="empty-rule"></div>
				</div>
			{/if}

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
					<span class="loading-text">{statusText || stageStatus(progressStage) || "Consulting the evidence…"}</span>
				</div>
			{/if}
		</div>

		<div class="augur-input-area">
			<div class="input-rule" aria-hidden="true"></div>
			<form
				class="input-row"
				onsubmit={(e) => {
					e.preventDefault();
					void handleSubmit();
				}}
			>
				<textarea
					id="augur-input"
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
					disabled={isLoading || !inputValue.trim()}
					aria-label="Submit question"
				>
					{#if isLoading}
						<span class="submit-loading"></span>
					{:else}
						<span class="submit-word">Submit</span>
						<span class="submit-arrow" aria-hidden="true">&#8594;</span>
					{/if}
				</button>
			</form>
			<p class="input-hint">Press Enter to submit, Shift+Enter for new line</p>
		</div>
	</div>

	<aside class="augur-rail-slot">
		<CitationRail
			citations={railCitations as Citation[]}
			relatedCitations={railRelatedCitations as Citation[]}
			activeIndex={railActiveIndex}
			onSelect={focusCitation}
			loading={isLoading && railCitations.length === 0}
		/>
	</aside>
</div>

<style>
	/* ===== Shell + column =====
	   Written phone-first. The 768px query is the same `md` breakpoint
	   `isDesktop()` answers for, so the layout and the code agree on where the
	   desktop reading begins; the 1280px query is where the citation rail has
	   room to sit beside the thread. Below it ThreadEntry's inline citation
	   footer is the canonical citation surface. */
	.augur-shell {
		display: grid;
		grid-template-columns: 1fr;
		height: 100%;
		background: var(--surface-bg, #faf9f7);
		overflow: hidden;
		overscroll-behavior: none;
		opacity: 0; transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}
	.augur-shell.revealed { opacity: 1; transform: translateY(0); }
	.augur-column {
		display: flex; flex-direction: column;
		width: 100%; height: 100%;
		min-height: 0;
	}
	.augur-rail-slot { display: none; }

	/* ===== Thread ===== */
	.augur-thread {
		flex: 1; min-height: 0;
		overflow-y: auto; overflow-x: hidden;
		padding: calc(0.5rem + env(safe-area-inset-top, 0px)) 1rem 1rem;
		overscroll-behavior-y: contain;
		-webkit-overflow-scrolling: touch;
	}

	/* ===== Empty state: the invitation ===== */
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
		font-size: 1.1rem; font-weight: 700;
		letter-spacing: -0.01em;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}
	.empty-rule {
		width: 80px; height: 1px;
		background: var(--surface-border, #c8c8c8);
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

	/* ===== Input ===== */
	.augur-input-area {
		flex-shrink: 0;
		background: var(--surface-bg, #faf9f7);
		padding: 0 1rem 0.75rem;
	}
	.input-rule {
		height: 1px; background: var(--surface-border, #c8c8c8);
		margin-bottom: 0.5rem;
	}
	.input-row {
		display: flex; gap: 0.5rem; align-items: flex-end;
	}
	.input-field {
		flex: 1 1 auto;
		min-width: 0;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		/* >= 16px, or iOS Safari zooms the whole page on focus. */
		font-size: 1rem; line-height: 1.4;
		padding: 0.5rem 0.6rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		border-radius: 0;
		background: transparent;
		color: var(--alt-charcoal, #1a1a1a);
		resize: none;
		min-height: 44px; max-height: 100px;
		transition: border-color 0.15s;
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
	/* The word is the desktop affordance, the arrow the phone one. Both are
	   inside one button whose accessible name is its aria-label, so swapping
	   them in CSS changes nothing a screen reader hears. */
	.submit-word { display: none; }
	.input-hint { display: none; }

	/* ===== Desktop: a centred column, roomier type, the word "Submit" ===== */
	@media (min-width: 48rem) {
		.augur-shell {
			min-height: calc(100dvh - 5rem);
			height: auto;
			overflow: visible;
		}
		.augur-column {
			max-width: 720px; margin: 0 auto;
			padding: 0 1rem;
			height: calc(100dvh - 5rem);
		}
		.augur-thread { padding: 0.5rem 0; }
		.empty-avatar { width: 48px; height: 48px; }
		.empty-title { font-size: 1.3rem; }
		.empty-rule { width: 120px; }

		/* An empty desktop consultation is an invitation, not a waiting room:
		   the avatar and the box sit together in the middle of the column
		   rather than at opposite ends of it. */
		.is-empty .augur-column { justify-content: center; }
		.is-empty .augur-thread { flex: 0 0 auto; overflow: visible; }
		.is-empty .augur-empty { height: auto; gap: 0.6rem; margin-bottom: 2rem; }
		.is-empty .augur-input-area { width: 100%; max-width: 560px; margin: 0 auto; }

		.augur-loading {
			justify-content: center; padding: 2rem;
			gap: 0.75rem; font-size: 0.85rem;
		}
		.loading-pulse { width: 8px; height: 8px; }
		.stage-hint { font-size: 0.75rem; padding: 0.25rem 0 0.75rem; }

		.augur-input-area { padding: 0 0 1rem; }
		.input-rule { display: none; }
		.input-field {
			font-size: 0.95rem; line-height: 1.5;
			padding: 0.6rem 0.75rem;
			min-height: 60px; max-height: 120px;
		}
		.input-submit {
			font-size: 0.75rem; letter-spacing: 0.06em; text-transform: uppercase;
			width: auto; height: auto; min-height: 44px;
			padding: 0.55rem 1.1rem;
			white-space: nowrap;
		}
		.input-submit:hover:not(:disabled) {
			background: var(--alt-charcoal, #1a1a1a);
			color: var(--surface-bg, #faf9f7);
		}
		.submit-word { display: inline; }
		.submit-arrow { display: none; }
		.input-hint {
			display: block;
			font-family: var(--font-body, "Source Sans 3", sans-serif);
			font-size: 0.7rem; font-style: italic;
			color: var(--alt-ash, #999);
			margin: 0.4rem 0 0;
		}
	}

	/* ===== Wide desktop: the citation rail earns its column ===== */
	@media (min-width: 80rem) {
		.augur-shell {
			grid-template-columns: minmax(0, 1fr) 320px;
		}
		.augur-column {
			margin: 0;
			margin-left: auto;
		}
		.augur-rail-slot {
			display: block;
			height: calc(100dvh - 5rem);
			position: sticky;
			top: 0;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.augur-shell { transition: none; opacity: 1; transform: none; }
	}
</style>
