<script lang="ts">
import { X, Ban } from "@lucide/svelte";
import type { ConnectFeedSource } from "$lib/connect/feeds";

interface Props {
	sources: ConnectFeedSource[];
	excludedSourceId: string | null;
	onExclude: (feedLinkId: string) => void;
	onClearExclusion: () => void;
}

let { sources, excludedSourceId, onExclude, onClearExclusion }: Props = $props();

let query = $state("");
let isOpen = $state(false);
let highlightedIndex = $state(-1);
let inputEl = $state<HTMLInputElement | null>(null);

const excludedSource = $derived(
	excludedSourceId ? sources.find((s) => s.id === excludedSourceId) : null,
);

const filteredSources = $derived(
	query.trim() === ""
		? []
		: sources
				.filter((s) => s.url.toLowerCase().includes(query.toLowerCase()))
				.slice(0, 10),
);

function handleSelect(source: ConnectFeedSource) {
	onExclude(source.id);
	query = "";
	isOpen = false;
	highlightedIndex = -1;
}

function handleClear() {
	onClearExclusion();
}

function handleKeydown(e: KeyboardEvent) {
	if (e.key === "ArrowDown") {
		e.preventDefault();
		if (!isOpen && query.trim().length > 0) {
			isOpen = true;
		}
		highlightedIndex = Math.min(
			highlightedIndex + 1,
			filteredSources.length - 1,
		);
	} else if (e.key === "ArrowUp") {
		e.preventDefault();
		highlightedIndex = Math.max(highlightedIndex - 1, 0);
	} else if (e.key === "Enter" && highlightedIndex >= 0) {
		e.preventDefault();
		handleSelect(filteredSources[highlightedIndex]);
	} else if (e.key === "Escape") {
		isOpen = false;
		highlightedIndex = -1;
	}
}

function handleInput() {
	isOpen = query.trim().length > 0;
	highlightedIndex = -1;
}

function handleBlur() {
	// Delay to allow click on dropdown item to register
	setTimeout(() => {
		isOpen = false;
		highlightedIndex = -1;
	}, 150);
}

function extractDomain(url: string): string {
	try {
		return new URL(url).hostname;
	} catch {
		return url;
	}
}

const listboxId = "exclude-source-listbox";
</script>

{#if excludedSource}
	<!-- Chip display when a source is excluded -->
	<button
		type="button"
		class="inline-flex items-center gap-1.5 px-2 py-1 border border-[var(--surface-border)] bg-[var(--surface-bg)] text-xs text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
		onclick={handleClear}
		aria-label="Remove exclusion for {extractDomain(excludedSource.url)}"
	>
		<Ban class="h-3 w-3 text-[var(--text-secondary)]" />
		<span class="max-w-[180px] truncate">{extractDomain(excludedSource.url)}</span>
		<X class="h-3 w-3 text-[var(--text-secondary)]" />
	</button>
{:else}
	<!-- Autocomplete input -->
	<div class="relative">
		<div class="flex items-center gap-1.5">
			<Ban class="h-3.5 w-3.5 text-[var(--text-secondary)] shrink-0" />
			<input
				bind:this={inputEl}
				type="text"
				bind:value={query}
				oninput={handleInput}
				onkeydown={handleKeydown}
				onblur={handleBlur}
				onfocus={handleInput}
				placeholder="Exclude source..."
				class="w-[140px] h-7 px-2 border border-[var(--surface-border)] bg-white text-xs placeholder:text-[var(--text-muted)]"
				role="combobox"
				aria-expanded={isOpen && filteredSources.length > 0}
				aria-controls={listboxId}
				aria-activedescendant={highlightedIndex >= 0 ? `exclude-option-${highlightedIndex}` : undefined}
				autocomplete="off"
			/>
		</div>

		{#if isOpen && filteredSources.length > 0}
			<ul
				id={listboxId}
				role="listbox"
				class="absolute top-full left-0 mt-1 w-[280px] max-h-[200px] overflow-y-auto border border-[var(--surface-border)] bg-white shadow-sm z-50"
			>
				{#each filteredSources as source, index (source.id)}
					<li
						id="exclude-option-{index}"
						role="option"
						aria-selected={highlightedIndex === index}
						class="px-3 py-1.5 text-xs cursor-pointer {highlightedIndex === index
							? 'bg-[var(--surface-hover)]'
							: 'hover:bg-[var(--surface-hover)]'}"
						onmousedown={() => handleSelect(source)}
						onmouseenter={() => (highlightedIndex = index)}
					>
						<span class="truncate block">{source.url}</span>
					</li>
				{/each}
			</ul>
		{:else if isOpen && query.trim().length > 0 && filteredSources.length === 0}
			<div
				class="absolute top-full left-0 mt-1 w-[280px] border border-[var(--surface-border)] bg-white shadow-sm z-50 px-3 py-2 text-xs text-[var(--text-muted)]"
			>
				No matching sources
			</div>
		{/if}
	</div>
{/if}
