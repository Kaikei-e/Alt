<script lang="ts">
import { goto } from "$app/navigation";
import type { TagSectionData } from "$lib/connect/global_search";
import { Tag } from "@lucide/svelte";

interface Props {
	section: TagSectionData;
	query: string;
}

const { section, query }: Props = $props();

function navigateToTag(tagName: string) {
	goto(`/articles/by-tag?tag=${encodeURIComponent(tagName)}`);
}
</script>

<section class="space-y-3">
	<div class="flex items-center justify-between">
		<h2
			class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]"
		>
			Tags
			{#if section.total > 0}
				<span class="ml-1 font-normal text-[var(--text-secondary)]"
					>({section.total})</span
				>
			{/if}
		</h2>
	</div>

	{#if section.hits.length === 0}
		<p class="text-sm text-[var(--text-secondary)] italic">
			No matching tags found.
		</p>
	{:else}
		<div class="flex flex-wrap gap-2">
			{#each section.hits as hit (hit.tagName)}
				<button
					type="button"
					onclick={() => navigateToTag(hit.tagName)}
					class="inline-flex items-center gap-1.5 rounded-full border border-[var(--surface-border)] bg-[var(--surface-bg)] px-3 py-1.5 text-sm text-[var(--text-primary)] hover:bg-[var(--surface-hover)] hover:border-[var(--interactive-text)] transition-colors cursor-pointer"
				>
					<Tag class="h-3 w-3 text-[var(--text-secondary)]" />
					<span>{hit.tagName}</span>
					<span
						class="ml-0.5 text-xs text-[var(--text-secondary)]"
					>
						({hit.articleCount})
					</span>
				</button>
			{/each}
		</div>
	{/if}
</section>
