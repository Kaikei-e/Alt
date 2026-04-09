<script lang="ts">
import { Ban, X } from "@lucide/svelte";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import {
	extractDomain,
	getEffectiveDomain,
	groupSourcesByDomain,
	collectFeedLinkIdsByDomain,
} from "$lib/utils/feed-source-filter";

interface Props {
	sources: ConnectFeedSource[];
	excludedFeedLinkIds: string[];
	onExclude: (feedLinkIds: string[]) => void;
	onClearExclusion: () => void;
}

let { sources, excludedFeedLinkIds, onExclude, onClearExclusion }: Props =
	$props();

let query = $state("");
let isOpen = $state(false);
let highlightedIndex = $state(-1);
let inputEl = $state<HTMLInputElement | null>(null);

// Find the domain being excluded (use first excluded source to determine domain)
const excludedDomain = $derived.by(() => {
	if (excludedFeedLinkIds.length === 0) return null;
	const source = sources.find((s) => excludedFeedLinkIds.includes(s.id));
	return source ? getEffectiveDomain(source.url) : null;
});

// Group sources by domain for the dropdown
const domainEntries = $derived.by(() => {
	const grouped = groupSourcesByDomain(sources);
	return [...grouped.entries()]
		.map(([domain, items]) => ({
			domain,
			displayUrl: items[0].url,
			count: items.length,
		}))
		.sort((a, b) => a.domain.localeCompare(b.domain));
});

const filteredDomains = $derived(
	query.trim() === ""
		? []
		: domainEntries
				.filter((e) => e.domain.toLowerCase().includes(query.toLowerCase()))
				.slice(0, 10),
);

function handleSelect(domain: string) {
	const ids = collectFeedLinkIdsByDomain(sources, domain);
	onExclude(ids);
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
			filteredDomains.length - 1,
		);
	} else if (e.key === "ArrowUp") {
		e.preventDefault();
		highlightedIndex = Math.max(highlightedIndex - 1, 0);
	} else if (e.key === "Enter" && highlightedIndex >= 0) {
		e.preventDefault();
		handleSelect(filteredDomains[highlightedIndex].domain);
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

const listboxId = "exclude-source-listbox";
</script>

{#if excludedDomain}
	<!-- Chip display when a domain is excluded -->
	<button
		type="button"
		class="inline-flex items-center gap-1.5 px-2 py-1 border border-[var(--surface-border)] bg-[var(--surface-bg)] text-xs text-[var(--text-primary)] hover:bg-[var(--surface-hover)] transition-colors"
		onclick={handleClear}
		aria-label="Remove exclusion for {excludedDomain}"
	>
		<Ban class="h-3 w-3 text-[var(--text-secondary)]" />
		<span class="max-w-[180px] truncate">{excludedDomain}</span>
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
				aria-expanded={isOpen && filteredDomains.length > 0}
				aria-controls={listboxId}
				aria-activedescendant={highlightedIndex >= 0 ? `exclude-option-${highlightedIndex}` : undefined}
				autocomplete="off"
			/>
		</div>

		{#if isOpen && filteredDomains.length > 0}
			<ul
				id={listboxId}
				role="listbox"
				class="absolute top-full left-0 mt-1 w-[280px] max-h-[200px] overflow-y-auto border border-[var(--surface-border)] bg-white shadow-sm z-50"
			>
				{#each filteredDomains as entry, index (entry.domain)}
					<li
						id="exclude-option-{index}"
						role="option"
						aria-selected={highlightedIndex === index}
						class="px-3 py-1.5 text-xs cursor-pointer {highlightedIndex === index
							? 'bg-[var(--surface-hover)]'
							: 'hover:bg-[var(--surface-hover)]'}"
						onmousedown={() => handleSelect(entry.domain)}
						onmouseenter={() => (highlightedIndex = index)}
					>
						<span class="truncate block">{entry.domain}{entry.count > 1 ? ` (${entry.count} feeds)` : ""}</span>
					</li>
				{/each}
			</ul>
		{:else if isOpen && query.trim().length > 0 && filteredDomains.length === 0}
			<div
				class="absolute top-full left-0 mt-1 w-[280px] border border-[var(--surface-border)] bg-white shadow-sm z-50 px-3 py-2 text-xs text-[var(--text-muted)]"
			>
				No matching sources
			</div>
		{/if}
	</div>
{/if}
