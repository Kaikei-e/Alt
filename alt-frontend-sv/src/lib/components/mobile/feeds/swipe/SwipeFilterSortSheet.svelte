<script lang="ts">
import {
	ArrowUpDown,
	Ban,
	Check,
	Search,
	SlidersHorizontal,
	X,
} from "@lucide/svelte";
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
	sortOrder: "newest" | "oldest";
	onExclude: (ids: string[]) => void;
	onClearExclusion: () => void;
	onSortChange: (order: "newest" | "oldest") => void;
}

let {
	sources,
	excludedFeedLinkIds,
	sortOrder,
	onExclude,
	onClearExclusion,
	onSortChange,
}: Props = $props();

let isOpen = $state(false);
let query = $state("");

const kb = useKeyboardOffset(() => isOpen);

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

	setTimeout(() => {
		isOpen = false;
	}, 350);
}

function handleClear() {
	onClearExclusion();
}
</script>

<!-- Trigger Button -->
<button
	type="button"
	class="filter-trigger"
	onclick={() => { isOpen = true; }}
	aria-label="Filter and sort"
	data-testid="swipe-filter-trigger"
>
	<SlidersHorizontal size={18} />
	{#if excludedFeedLinkIds.length > 0}
		<span
			class="filter-badge"
			data-testid="filter-active-badge"
		></span>
	{/if}
</button>

<!-- Bottom Sheet -->
<Sheet.Root bind:open={isOpen}>
	<Sheet.Content
		side="bottom"
		class="max-h-[70vh] w-full max-w-full sm:max-w-full p-0 gap-0 flex flex-col overflow-hidden [&>button.ring-offset-background]:hidden"
		style="background: var(--surface-bg) !important; border-top: 1px solid var(--surface-border); border-radius: 0; {kb.style}"
		onOpenAutoFocus={(e) => e.preventDefault()}
		onCloseAutoFocus={(e) => e.preventDefault()}
	>
		<Sheet.Header class="sheet-header">
			<div class="flex items-center justify-between">
				<div>
					<Sheet.Title class="sheet-title">
						Filter & Sort
					</Sheet.Title>
					<Sheet.Description class="sheet-description">
						Customize your swipe feed
					</Sheet.Description>
				</div>
			</div>
		</Sheet.Header>

		<div class="sheet-body">
			<!-- Sort Section -->
			<section>
				<h3 class="section-label">
					<ArrowUpDown size={12} />
					Sort by
				</h3>
				<div class="flex flex-col gap-1">
					<button
						type="button"
						class="sort-option {sortOrder === 'newest' ? 'sort-option--active' : ''}"
						onclick={() => onSortChange("newest")}
						data-testid="sort-newest"
					>
						<span>Newest first</span>
						{#if sortOrder === "newest"}
							<Check size={14} class="check-icon" />
						{/if}
					</button>
					<button
						type="button"
						class="sort-option sort-option--disabled"
						disabled
						data-testid="sort-oldest"
					>
						<span>
							Oldest first
							<span class="coming-soon">(Coming soon)</span>
						</span>
					</button>
				</div>
			</section>

			<hr class="section-rule" />

			<!-- Exclude Source Section -->
			<section>
				<h3 class="section-label">
					<Ban size={12} />
					Exclude source
				</h3>

				{#if excludedDomain}
					<button
						type="button"
						class="exclude-chip"
						onclick={handleClear}
						aria-label="Remove exclusion for {excludedDomain}"
						data-testid="exclude-chip-active"
					>
						<Ban size={14} />
						<span class="exclude-chip-text">{excludedDomain}</span>
						<X size={14} />
					</button>
				{:else}
					<div class="search-wrapper">
						<Search size={14} class="search-icon" />
						<input
							type="text"
							bind:value={query}
							placeholder="Search sources..."
							inputmode="search"
							enterkeyhint="search"
							class="search-input"
							data-testid="exclude-search-input"
						/>
					</div>

					<div
						class="source-list"
						role="listbox"
						aria-label="Feed sources"
					>
						{#each displayedDomains as entry (entry.domain)}
							<button
								type="button"
								role="option"
								aria-selected={false}
								class="source-item"
								onclick={() => handleSelect(entry.domain)}
								data-testid="exclude-source-item"
							>
								<span class="source-item-text">{entry.domain}{entry.count > 1 ? ` (${entry.count} feeds)` : ""}</span>
							</button>
						{:else}
							<p class="source-empty">No matching sources</p>
						{/each}
					</div>
				{/if}
			</section>
		</div>

		<Sheet.Close
			class="sheet-close"
			aria-label="Close dialog"
		>
			<X size={14} />
		</Sheet.Close>
	</Sheet.Content>
</Sheet.Root>

<style>
	/* ── Trigger ── */
	.filter-trigger {
		position: fixed;
		bottom: 1.5rem;
		left: 1.5rem;
		z-index: 1000;
		width: 44px;
		height: 44px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: var(--surface-bg);
		border: 1.5px solid var(--alt-charcoal);
		color: var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.filter-trigger:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.filter-badge {
		position: absolute;
		top: -2px;
		right: -2px;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-primary);
		border: 2px solid var(--surface-bg);
	}

	/* ── Sheet ── */
	:global(.sheet-header) {
		border-bottom: 1px solid var(--surface-border);
		padding: 1.5rem 1.5rem 1rem;
	}

	:global(.sheet-title) {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--alt-charcoal);
	}

	:global(.sheet-description) {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-slate);
	}

	.sheet-body {
		overflow-y: auto;
		flex: 1;
		padding: 1rem 1.5rem;
		padding-bottom: calc(1.5rem + env(safe-area-inset-bottom, 0px));
	}

	:global(.sheet-close) {
		position: absolute;
		right: 1.5rem;
		top: 1.5rem;
		width: 36px;
		height: 36px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		background: transparent;
		border: 1px solid var(--surface-border);
		color: var(--alt-charcoal);
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s;
		border-radius: 0;
	}

	:global(.sheet-close:hover) {
		background: var(--surface-hover);
		border-color: var(--alt-charcoal);
	}

	/* ── Section label ── */
	.section-label {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0 0 0.75rem;
	}

	.section-rule {
		border: none;
		border-top: 1px solid var(--surface-border);
		margin: 1rem 0;
	}

	/* ── Sort options ── */
	.sort-option {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		padding: 0.75rem;
		min-height: 44px;
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		background: transparent;
		border: none;
		text-align: left;
		cursor: pointer;
		transition: background 0.15s;
	}

	.sort-option:hover {
		background: var(--surface-hover);
	}

	.sort-option--active {
		background: var(--surface-hover);
		font-weight: 500;
	}

	.sort-option--disabled {
		opacity: 0.4;
		cursor: not-allowed;
		color: var(--alt-ash);
	}

	.sort-option :global(.check-icon) {
		color: var(--alt-primary);
	}

	.coming-soon {
		font-size: 0.7rem;
		margin-left: 0.25rem;
	}

	/* ── Exclude ── */
	.exclude-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.5rem 0.75rem;
		min-height: 44px;
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		background: var(--surface-bg);
		border: 1px solid var(--surface-border);
		cursor: pointer;
		transition: background 0.15s;
	}

	.exclude-chip:active {
		background: var(--surface-hover);
	}

	.exclude-chip-text {
		max-width: 200px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	/* ── Search ── */
	.search-wrapper {
		position: relative;
		margin-bottom: 0.5rem;
	}

	.search-wrapper :global(.search-icon) {
		position: absolute;
		left: 0.75rem;
		top: 50%;
		transform: translateY(-50%);
		color: var(--alt-ash);
	}

	.search-input {
		width: 100%;
		height: 44px;
		padding-left: 2.25rem;
		padding-right: 0.75rem;
		font-family: var(--font-body);
		font-size: 1rem;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1px solid var(--surface-border);
		outline: none;
		transition: border-color 0.15s;
	}

	.search-input::placeholder {
		color: var(--alt-ash);
	}

	.search-input:focus {
		border-color: var(--alt-charcoal);
	}

	/* ── Source list ── */
	.source-list {
		overflow-y: auto;
		max-height: 30vh;
	}

	.source-item {
		width: 100%;
		text-align: left;
		padding: 0.75rem;
		min-height: 44px;
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: background 0.15s;
	}

	.source-item:hover,
	.source-item:active {
		background: var(--surface-hover);
	}

	.source-item-text {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.source-empty {
		padding: 1rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		text-align: center;
		margin: 0;
	}
</style>
