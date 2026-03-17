<script lang="ts">
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";
import WhySurfacedBadge from "./WhySurfacedBadge.svelte";
import QuickActionRow from "./QuickActionRow.svelte";

interface Props {
	item: KnowledgeHomeItemData;
	onAction: (type: string, itemKey: string) => void;
}

const { item, onAction }: Props = $props();

const displayTags = $derived(item.tags.slice(0, 3));
const remainingTagCount = $derived(
	item.tags.length > 3 ? item.tags.length - 3 : 0,
);
const displayReasons = $derived(item.why.slice(0, 2));

function formatRelativeTime(isoString: string): string {
	const date = new Date(isoString);
	const now = new Date();
	const diffMs = now.getTime() - date.getTime();
	const diffMins = Math.floor(diffMs / 60000);
	if (diffMins < 1) return "just now";
	if (diffMins < 60) return `${diffMins}m ago`;
	const diffHours = Math.floor(diffMins / 60);
	if (diffHours < 24) return `${diffHours}h ago`;
	const diffDays = Math.floor(diffHours / 24);
	return `${diffDays}d ago`;
}

function handleAction(type: string) {
	onAction(type, item.itemKey);
}
</script>

<article
	class="border-2 rounded-lg p-4 bg-[var(--surface-bg)] border-[var(--surface-border)] hover:border-[var(--accent-primary)] transition-colors duration-200"
	data-item-key={item.itemKey}
>
	<!-- Header: Title + Relative Time -->
	<div class="flex items-start justify-between gap-2 mb-2">
		<h3 class="text-sm font-semibold text-[var(--text-primary)] line-clamp-2 flex-1">
			{item.title}
		</h3>
		<time
			class="text-xs text-[var(--text-secondary)] whitespace-nowrap flex-shrink-0"
			datetime={item.publishedAt}
		>
			{formatRelativeTime(item.publishedAt)}
		</time>
	</div>

	<!-- Why Badges -->
	{#if displayReasons.length > 0}
		<div class="flex flex-wrap gap-1 mb-2">
			{#each displayReasons as reason}
				<WhySurfacedBadge {reason} />
			{/each}
		</div>
	{/if}

	<!-- Summary Excerpt -->
	<p class="text-xs text-[var(--text-secondary)] line-clamp-2 mb-2">
		{item.summaryExcerpt ?? "Summarizing..."}
	</p>

	<!-- Tags -->
	{#if displayTags.length > 0}
		<div class="flex flex-wrap gap-1 mb-3">
			{#each displayTags as tag}
				<span
					class="px-1.5 py-0.5 text-xs rounded bg-[var(--surface-hover)] text-[var(--text-secondary)]"
				>
					{tag}
				</span>
			{/each}
			{#if remainingTagCount > 0}
				<span class="px-1.5 py-0.5 text-xs text-[var(--text-secondary)]">
					+{remainingTagCount}
				</span>
			{/if}
		</div>
	{/if}

	<!-- Actions -->
	<QuickActionRow
		itemKey={item.itemKey}
		itemType={item.itemType}
		articleId={item.articleId}
		onAction={handleAction}
	/>
</article>
