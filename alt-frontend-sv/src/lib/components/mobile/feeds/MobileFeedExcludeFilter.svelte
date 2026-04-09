<script lang="ts">
import { Ban, X } from "@lucide/svelte";
import * as Sheet from "$lib/components/ui/sheet";
import type { ConnectFeedSource } from "$lib/connect/feeds";
import { useKeyboardOffset } from "$lib/hooks/useKeyboardOffset.svelte";
import {
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

let isSheetOpen = $state(false);
let query = $state("");

const kb = useKeyboardOffset(() => isSheetOpen);

const excludedDomain = $derived.by(() => {
	if (excludedFeedLinkIds.length === 0) return null;
	const source = sources.find((s) => excludedFeedLinkIds.includes(s.id));
	return source ? getEffectiveDomain(source.url) : null;
});

const domainEntries = $derived.by(() => {
	const grouped = groupSourcesByDomain(sources);
	return [...grouped.entries()]
		.map(([domain, items]) => ({
			domain,
			count: items.length,
		}))
		.sort((a, b) => a.domain.localeCompare(b.domain));
});

const displayedDomains = $derived(
	query.trim() === ""
		? domainEntries
		: domainEntries.filter((e) =>
				e.domain.toLowerCase().includes(query.toLowerCase()),
			),
);

function handleSelect(domain: string) {
	// Phase 1: Force keyboard dismiss before closing sheet.
	// Safari iOS freezes the viewport if isSheetOpen becomes false (triggering
	// useKeyboardOffset → offset=0) while the keyboard is still animating away.
	const input = document.querySelector<HTMLInputElement>(
		'[data-testid="exclude-search-input"]',
	);
	if (input) input.readOnly = true;
	if (document.activeElement instanceof HTMLElement) {
		document.activeElement.blur();
	}

	const ids = collectFeedLinkIdsByDomain(sources, domain);
	onExclude(ids);
	query = "";

	// Phase 2: Close sheet AFTER keyboard dismiss animation (~350ms)
	setTimeout(() => {
		isSheetOpen = false;
	}, 350);
}

function handleClear() {
	onClearExclusion();
}

function openSheet() {
	query = "";
	isSheetOpen = true;
}
</script>

<div class="px-4 py-2" data-testid="mobile-feed-exclude-filter">
	{#if excludedDomain}
		<!-- Active state: chip showing excluded domain -->
		<button
			type="button"
			class="inline-flex items-center gap-1.5 px-3 py-2 rounded-full border border-[var(--surface-border)] bg-[var(--surface-bg)] text-sm text-[var(--text-primary)] active:bg-[var(--surface-hover)] transition-colors min-h-[44px]"
			onclick={handleClear}
			aria-label="Remove exclusion for {excludedDomain}"
			data-testid="exclude-chip-active"
		>
			<Ban class="h-4 w-4 text-[var(--text-secondary)] shrink-0" />
			<span class="max-w-[200px] truncate"
				>{excludedDomain}</span
			>
			<X class="h-4 w-4 text-[var(--text-secondary)] shrink-0" />
		</button>
	{:else}
		<!-- Inactive state: tap to open bottom sheet -->
		<button
			type="button"
			class="inline-flex items-center gap-1.5 px-3 py-2 rounded-full border border-dashed border-[var(--surface-border)] text-sm text-[var(--text-secondary)] active:bg-[var(--surface-hover)] transition-colors min-h-[44px]"
			onclick={openSheet}
			aria-label="Exclude a feed source"
			data-testid="exclude-chip-inactive"
		>
			<Ban class="h-4 w-4 shrink-0" />
			<span>Exclude source</span>
		</button>
	{/if}
</div>

<!-- Bottom sheet for source selection -->
<Sheet.Root bind:open={isSheetOpen}>
	<Sheet.Content
		side="bottom"
		class="max-h-[70vh] rounded-t-2xl p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: white !important; {kb.style}"
		onOpenAutoFocus={(e) => e.preventDefault()}
		onCloseAutoFocus={(e) => e.preventDefault()}
	>
		<Sheet.Header>
			<Sheet.Title>Exclude a Source</Sheet.Title>
		</Sheet.Header>

		<div class="px-4 pb-2">
			<input
				type="text"
				bind:value={query}
				placeholder="Search sources..."
				inputmode="search"
				enterkeyhint="search"
				class="w-full h-11 px-3 rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] text-base placeholder:text-[var(--text-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--alt-primary)]"
				data-testid="exclude-search-input"
			/>
		</div>

		<div
			class="overflow-y-auto flex-1 px-2 pb-4"
			role="listbox"
			aria-label="Feed sources"
		>
			{#each displayedDomains as entry (entry.domain)}
				<button
					type="button"
					role="option"
					aria-selected={false}
					class="w-full text-left px-3 py-3 min-h-[44px] text-sm text-[var(--text-primary)] rounded-lg active:bg-[var(--surface-hover)] hover:bg-[var(--surface-hover)] transition-colors"
					onclick={() => handleSelect(entry.domain)}
					data-testid="exclude-source-item"
				>
					<span class="block truncate">{entry.domain}{entry.count > 1 ? ` (${entry.count} feeds)` : ""}</span>
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
	</Sheet.Content>
</Sheet.Root>
