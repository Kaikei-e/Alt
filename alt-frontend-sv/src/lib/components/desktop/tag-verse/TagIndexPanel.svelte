<script lang="ts">
import type { TagCloudItem } from "#lib/connect/index.js";
import { filterTagsForIndex } from "./tagIndex.js";

interface Props {
	tags: TagCloudItem[];
	selectedTag: string | null;
	onSelectTag: (tagName: string) => void;
}

let { tags, selectedTag, onSelectTag }: Props = $props();

let isOpen = $state(true);
let searchQuery = $state("");

const filteredTags = $derived(filterTagsForIndex(tags, searchQuery));
const panelId = "tag-verse-index-panel";
</script>

<nav
	aria-label="Tag index"
	class="flex h-full shrink-0 flex-col border-r border-[var(--surface-border)] bg-[var(--surface-bg)] {isOpen
		? 'w-[280px]'
		: 'w-auto'}"
>
	<div
		class="flex items-center justify-between border-b border-[var(--surface-border)] px-3 py-2"
	>
		<button
			type="button"
			aria-expanded={isOpen}
			aria-controls={panelId}
			onclick={() => {
				isOpen = !isOpen;
			}}
			class="rounded-none border border-[var(--surface-border)] bg-[var(--action-surface)] px-2.5 py-1 text-xs font-medium text-[var(--text-primary)] transition-colors hover:bg-[var(--action-surface-hover)]"
		>
			{isOpen ? "Hide index" : "Show index"}
		</button>
	</div>

	<div
		id={panelId}
		hidden={!isOpen}
		class="flex min-h-0 flex-1 flex-col"
	>
		<div class="border-b border-[var(--surface-border)] p-3">
			<label
				for="tag-verse-search-input"
				class="mb-1.5 block text-xs font-medium text-[var(--text-secondary)]"
			>
				Find a tag
			</label>
			<input
				id="tag-verse-search-input"
				type="search"
				bind:value={searchQuery}
				placeholder="Filter by tag name"
				class="w-full rounded-none border border-[var(--surface-border)] bg-[var(--surface-2)] px-2.5 py-1.5 text-sm text-[var(--text-primary)] placeholder:text-[var(--text-muted)] focus:border-[var(--interactive-text)] focus:outline-none"
			/>
		</div>

		<div class="flex-1 divide-y divide-[var(--surface-border)] overflow-y-auto">
			{#if filteredTags.length === 0}
				<p class="px-3 py-6 text-center text-xs text-[var(--text-muted)]">
					No matching tags.
				</p>
			{:else}
				{#each filteredTags as item (item.tagName)}
					{@const isSelected = selectedTag === item.tagName}
					<button
						type="button"
						aria-current={isSelected ? "true" : undefined}
						onclick={() => onSelectTag(item.tagName)}
						class="flex w-full items-center justify-between gap-2 border-l-[3px] px-3 py-2 text-left text-sm transition-colors {isSelected
							? 'border-l-[var(--interactive-text)] bg-[var(--surface-hover)] font-semibold text-[var(--text-primary)]'
							: 'border-l-transparent text-[var(--text-primary)] hover:bg-[var(--surface-hover)]'}"
					>
						<span class="truncate">{item.tagName}</span>
						<span
							class="shrink-0 text-right text-xs text-[var(--text-muted)]"
							style="font-family: var(--font-mono);"
						>
							{item.articleCount}
						</span>
					</button>
				{/each}
			{/if}
		</div>
	</div>
</nav>
