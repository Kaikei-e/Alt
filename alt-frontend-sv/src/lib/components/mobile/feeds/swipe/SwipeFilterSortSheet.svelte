<script lang="ts">
import { ArrowUpDown, Ban, Check, Search, SlidersHorizontal, X } from "@lucide/svelte";
import * as Sheet from "$lib/components/ui/sheet";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import { useKeyboardOffset } from "$lib/hooks/useKeyboardOffset.svelte";
import { extractDomain, filterSources } from "$lib/utils/feed-source-filter";

interface Props {
	sources: ConnectFeedSource[];
	excludedSourceId: string | null;
	sortOrder: "newest" | "oldest";
	onExclude: (id: string) => void;
	onClearExclusion: () => void;
	onSortChange: (order: "newest" | "oldest") => void;
}

let {
	sources,
	excludedSourceId,
	sortOrder,
	onExclude,
	onClearExclusion,
	onSortChange,
}: Props = $props();

let isOpen = $state(false);
let query = $state("");

const kb = useKeyboardOffset(() => isOpen);

const excludedSource = $derived(
	excludedSourceId ? sources.find((s) => s.id === excludedSourceId) : null,
);

const displayedSources = $derived(
	query.trim() === "" ? sources : filterSources(sources, query),
);

function handleSelect(source: ConnectFeedSource) {
	// Phase 1: Force keyboard dismiss before closing sheet.
	// Safari iOS freezes the viewport if isOpen becomes false (triggering
	// useKeyboardOffset → offset=0) while the keyboard is still animating away.
	const input = document.querySelector<HTMLInputElement>(
		'[data-testid="exclude-search-input"]',
	);
	if (input) input.readOnly = true;
	if (document.activeElement instanceof HTMLElement) {
		document.activeElement.blur();
	}

	onExclude(source.id);
	query = "";

	// Phase 2: Close sheet AFTER keyboard dismiss animation (~350ms)
	setTimeout(() => {
		isOpen = false;
	}, 350);
}

function handleClear() {
	onClearExclusion();
}
</script>

<!-- Trigger Button (fixed, left side) -->
<button
	type="button"
	class="fixed bottom-6 left-6 z-[1000] h-12 w-12 rounded-full border-2 border-[var(--text-primary)] bg-[var(--bg-surface)] text-[var(--text-primary)] shadow-[var(--shadow-glass)] backdrop-blur-md transition-all duration-300 hover:scale-105 hover:bg-[var(--bg-surface-hover)] hover:border-[var(--accent-primary)] active:scale-95 inline-flex shrink-0 items-center justify-center focus-visible:outline-none outline-none"
	onclick={() => { isOpen = true; }}
	aria-label="Filter and sort"
	data-testid="swipe-filter-trigger"
>
	<SlidersHorizontal class="h-5 w-5 relative z-[1]" />
	{#if excludedSourceId}
		<span
			class="absolute -top-1 -right-1 h-3 w-3 rounded-full bg-[var(--alt-primary)] border-2 border-[var(--bg-surface)]"
			data-testid="filter-active-badge"
		></span>
	{/if}
</button>

<!-- Bottom Sheet -->
<Sheet.Root bind:open={isOpen}>
	<Sheet.Content
		side="bottom"
		class="max-h-[70vh] rounded-t-[24px] border-t border-[var(--border-glass)] text-[var(--text-primary)] shadow-[0_-10px_40px_rgba(0,0,0,0.2)] backdrop-blur-[20px] w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: white !important; {kb.style}"
		onOpenAutoFocus={(e) => e.preventDefault()}
		onCloseAutoFocus={(e) => e.preventDefault()}
	>
		<Sheet.Header class="border-b border-[var(--border-glass)] px-6 pb-4 pt-6">
			<div class="flex items-center justify-between">
				<div>
					<Sheet.Title class="text-xl font-bold text-[var(--text-primary)]">
						Filter & Sort
					</Sheet.Title>
					<Sheet.Description class="text-sm text-[var(--text-secondary)]">
						Customize your swipe feed
					</Sheet.Description>
				</div>
			</div>
		</Sheet.Header>

		<div class="overflow-y-auto flex-1 px-6 py-4 pb-[calc(1.5rem+env(safe-area-inset-bottom,0px))]">
			<!-- Sort Section -->
			<section>
				<h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-3 flex items-center gap-2">
					<ArrowUpDown class="h-3.5 w-3.5" />
					Sort by
				</h3>
				<div class="flex flex-col gap-1">
					<!-- Newest first -->
					<button
						type="button"
						class="flex items-center justify-between w-full px-3 py-3 min-h-[44px] rounded-lg text-sm text-left transition-colors {sortOrder === 'newest' ? 'bg-[var(--surface-hover)] font-medium' : 'hover:bg-[var(--surface-hover)]'}"
						onclick={() => onSortChange("newest")}
						data-testid="sort-newest"
					>
						<span class="text-[var(--text-primary)]">Newest first</span>
						{#if sortOrder === "newest"}
							<Check class="h-4 w-4 text-[var(--alt-primary)]" />
						{/if}
					</button>
					<!-- Oldest first (disabled) -->
					<button
						type="button"
						class="flex items-center justify-between w-full px-3 py-3 min-h-[44px] rounded-lg text-sm text-left opacity-50 cursor-not-allowed"
						disabled
						data-testid="sort-oldest"
					>
						<span class="text-[var(--text-muted)]">
							Oldest first
							<span class="text-xs ml-1">(Coming soon)</span>
						</span>
					</button>
				</div>
			</section>

			<hr class="my-4 border-[var(--surface-border)]" />

			<!-- Exclude Source Section -->
			<section>
				<h3 class="text-xs font-semibold uppercase tracking-wider text-[var(--text-muted)] mb-3 flex items-center gap-2">
					<Ban class="h-3.5 w-3.5" />
					Exclude source
				</h3>

				{#if excludedSource}
					<!-- Active exclusion chip -->
					<button
						type="button"
						class="inline-flex items-center gap-1.5 px-3 py-2 rounded-full border border-[var(--surface-border)] bg-[var(--surface-bg)] text-sm text-[var(--text-primary)] active:bg-[var(--surface-hover)] transition-colors min-h-[44px]"
						onclick={handleClear}
						aria-label="Remove exclusion for {extractDomain(excludedSource.url)}"
						data-testid="exclude-chip-active"
					>
						<Ban class="h-4 w-4 text-[var(--text-secondary)] shrink-0" />
						<span class="max-w-[200px] truncate">{extractDomain(excludedSource.url)}</span>
						<X class="h-4 w-4 text-[var(--text-secondary)] shrink-0" />
					</button>
				{:else}
					<!-- Search input -->
					<div class="relative mb-2">
						<Search class="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-[var(--text-muted)]" />
						<input
							type="text"
							bind:value={query}
							placeholder="Search sources..."
							inputmode="search"
							enterkeyhint="search"
							class="w-full h-11 pl-9 pr-3 rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] text-base placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--alt-primary)]"
							data-testid="exclude-search-input"
						/>
					</div>

					<!-- Source list -->
					<div
						class="overflow-y-auto max-h-[30vh]"
						role="listbox"
						aria-label="Feed sources"
					>
						{#each displayedSources as source (source.id)}
							<button
								type="button"
								role="option"
								aria-selected={false}
								class="w-full text-left px-3 py-3 min-h-[44px] text-sm text-[var(--text-primary)] rounded-lg active:bg-[var(--surface-hover)] hover:bg-[var(--surface-hover)] transition-colors"
								onclick={() => handleSelect(source)}
								data-testid="exclude-source-item"
							>
								<span class="block truncate">{source.url}</span>
							</button>
						{:else}
							<p
								class="px-3 py-4 text-sm text-center"
								style="color: var(--text-muted);"
							>
								No matching sources
							</p>
						{/each}
					</div>
				{/if}
			</section>
		</div>

		<Sheet.Close
			class="absolute right-6 top-6 h-10 w-10 rounded-full border border-[var(--border-glass)] bg-[var(--bg-glass)] backdrop-blur-md text-[var(--text-primary)] hover:bg-[var(--bg-surface-hover)] hover:border-[var(--accent-primary)] transition-all duration-200 inline-flex shrink-0 items-center justify-center focus-visible:outline-none outline-none"
			aria-label="Close dialog"
		>
			<X class="h-4 w-4" />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>
