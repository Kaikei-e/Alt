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

<div class="pb-3 mb-5 border-b border-[var(--surface-border)]/30 flex items-center justify-between gap-3">
	<div>
		<h2 class="text-lg font-bold text-[var(--text-primary)] tracking-tight">
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
		<p class="mt-1 text-sm text-[var(--text-muted)]">
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
	<div class="flex flex-col gap-4" bind:this={streamRef}>
		{#if degradedNote}
			<div class="rounded-lg border border-amber-400/30 bg-amber-400/5 px-3 py-2 text-xs text-amber-400">
				{degradedNote}
			</div>
		{/if}
		{#each items as item (item.itemKey)}
			<KnowledgeCard {item} {onAction} {onTagClick} />
		{/each}

		{#if loading}
			<div class="flex justify-center py-4">
				<div
					class="h-5 w-5 border-2 border-[var(--text-secondary)] border-t-transparent rounded-full animate-spin"
				></div>
			</div>
		{/if}

		<!-- Infinite scroll sentinel -->
		<div bind:this={sentinelRef} class="h-1"></div>
	</div>
{/if}
