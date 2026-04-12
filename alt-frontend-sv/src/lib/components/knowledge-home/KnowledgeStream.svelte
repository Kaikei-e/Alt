<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";
import KnowledgeCard from "./KnowledgeCard.svelte";
import KnowledgeHomeEmpty, {
	type EmptyReason,
} from "./KnowledgeHomeEmpty.svelte";
import KnowledgeHomeSkeleton from "./KnowledgeHomeSkeleton.svelte";

export type StreamMode = "default" | "lens" | "search" | "recap_context";

interface Props {
	items: KnowledgeHomeItemData[];
	loading: boolean;
	hasMore: boolean;
	activeLensName?: string | null;
	emptyReason?: EmptyReason | null;
	streamMode?: StreamMode;
	searchQuery?: string;
	degradedNote?: string | null;
	onAction: (type: string, item: KnowledgeHomeItemData) => void;
	onTagClick?: (tag: string, item: KnowledgeHomeItemData) => void;
	onLoadMore: () => void;
	onItemsVisible: (itemKeys: string[]) => void;
	onClearLens?: () => void;
}

const {
	items,
	loading,
	hasMore,
	activeLensName = null,
	emptyReason = null,
	streamMode = "default",
	searchQuery = "",
	degradedNote = null,
	onAction,
	onTagClick,
	onLoadMore,
	onItemsVisible,
	onClearLens,
}: Props = $props();

let streamRef: HTMLDivElement | undefined = $state();
let sentinelRef: HTMLDivElement | undefined = $state();

// Seen tracking with IntersectionObserver
let seenKeys = new Set<string>();
let batchTimeout: ReturnType<typeof setTimeout> | null = null;

function flushSeen() {
	if (seenKeys.size > 0) {
		onItemsVisible([...seenKeys]);
		seenKeys = new Set();
	}
	batchTimeout = null;
}

function scheduleBatch() {
	if (batchTimeout) return;
	batchTimeout = setTimeout(flushSeen, 2000);
}

onMount(() => {
	if (!browser) return;

	// Seen tracking observer
	const seenObserver = new IntersectionObserver(
		(entries) => {
			for (const entry of entries) {
				if (entry.isIntersecting) {
					const key = (entry.target as HTMLElement).dataset.itemKey;
					if (key && !seenKeys.has(key)) {
						seenKeys.add(key);
						scheduleBatch();
					}
				}
			}
		},
		{ threshold: 0.5 },
	);

	// Infinite scroll sentinel observer
	let scrollObserver: IntersectionObserver | undefined;
	if (sentinelRef) {
		scrollObserver = new IntersectionObserver(
			(entries) => {
				if (entries[0]?.isIntersecting && hasMore && !loading) {
					onLoadMore();
				}
			},
			{ rootMargin: "200px" },
		);
		scrollObserver.observe(sentinelRef);
	}

	// Observe existing cards
	if (streamRef) {
		const cards = streamRef.querySelectorAll("[data-item-key]");
		for (const card of cards) {
			seenObserver.observe(card);
		}
	}

	// MutationObserver to watch for new cards
	let mutationObserver: MutationObserver | undefined;
	if (streamRef) {
		mutationObserver = new MutationObserver((mutations) => {
			for (const mutation of mutations) {
				for (const node of mutation.addedNodes) {
					if (node instanceof HTMLElement) {
						if (node.dataset.itemKey) {
							seenObserver.observe(node);
						}
						const inner = node.querySelectorAll("[data-item-key]");
						for (const el of inner) {
							seenObserver.observe(el);
						}
					}
				}
			}
		});
		mutationObserver.observe(streamRef, { childList: true, subtree: true });
	}

	return () => {
		seenObserver.disconnect();
		scrollObserver?.disconnect();
		mutationObserver?.disconnect();
		if (batchTimeout) clearTimeout(batchTimeout);
		flushSeen();
	};
});
</script>

<div class="stream-header">
	<div>
		<h2 class="stream-heading">
			{#if streamMode === "search"}
				Search results
			{:else if streamMode === "lens" && activeLensName}
				{activeLensName}
			{:else if streamMode === "recap_context"}
				Recap context
			{:else}
				Latest
			{/if}
		</h2>
		<p class="stream-subtitle">
			{#if streamMode === "search" && searchQuery}
				Query: "{searchQuery}"
			{:else if streamMode === "lens" && activeLensName}
				Server-side filtered view
			{:else}
				What to look at next, with explanation first.
			{/if}
		</p>
	</div>
</div>

{#if loading && items.length === 0}
	<KnowledgeHomeSkeleton />
{:else if !loading && items.length === 0}
	<KnowledgeHomeEmpty reason={emptyReason} {activeLensName} {onClearLens} />
{:else}
	<div class="stream-items" bind:this={streamRef}>
		{#if degradedNote}
			<div class="degraded-note">
				{degradedNote}
			</div>
		{/if}
		{#each items as item, i (item.itemKey)}
			<div class="stream-entry" style="--entry-delay: {i};">
				<KnowledgeCard {item} {onAction} {onTagClick} />
			</div>
		{/each}

		{#if loading}
			<div class="loading-more">
				<span class="loading-pulse"></span>
				<span class="loading-text">Loading more...</span>
			</div>
		{/if}

		<!-- Infinite scroll sentinel -->
		<div bind:this={sentinelRef} class="h-1"></div>
	</div>
{/if}

<style>
	.stream-header {
		padding-bottom: 0.75rem;
		margin-bottom: 1.25rem;
		border-bottom: 1px solid color-mix(in srgb, var(--surface-border) 30%, transparent);
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.75rem;
	}

	.stream-heading {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
	}

	.stream-subtitle {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-ash);
		margin-top: 0.15rem;
		font-style: italic;
	}

	.stream-items {
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.stream-entry {
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--entry-delay) * 40ms);
	}

	.degraded-note {
		border: 1px solid color-mix(in srgb, var(--alt-warning) 30%, transparent);
		background: color-mix(in srgb, var(--alt-warning) 5%, var(--surface-bg));
		padding: 0.5rem 0.75rem;
		font-size: 0.75rem;
		color: var(--alt-warning);
	}

	.loading-more {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
		padding: 1rem 0;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-ash);
		font-style: italic;
	}

	@keyframes entry-in {
		from {
			opacity: 0;
			transform: translateY(4px);
		}
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.stream-entry {
			animation: none;
			opacity: 1;
		}
	}
</style>
