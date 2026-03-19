<script lang="ts">
import type { KnowledgeHomeItemData } from "$lib/connect/knowledge_home";
import QuickActionRow from "./QuickActionRow.svelte";
import SummaryStateChip from "./SummaryStateChip.svelte";
import SupersedeBadge from "./SupersedeBadge.svelte";
import SupersedeDetail from "./SupersedeDetail.svelte";
import WhyPanel from "./WhyPanel.svelte";
import WhySurfacedBadge from "./WhySurfacedBadge.svelte";

interface Props {
	item: KnowledgeHomeItemData;
	onAction: (type: string, item: KnowledgeHomeItemData) => void;
}

const { item, onAction }: Props = $props();

const nonEmptyTags = $derived(item.tags.filter((t) => t.trim() !== ""));
const displayTags = $derived(nonEmptyTags.slice(0, 3));
const remainingTagCount = $derived(
	nonEmptyTags.length > 3 ? nonEmptyTags.length - 3 : 0,
);
const displayReasons = $derived(
	item.why.length > 0 ? item.why.slice(0, 2) : [{ code: "new_unread" }],
);
let supersedeExpanded = $state(false);
let whyExpanded = $state(false);

function formatRelativeTime(isoString: string): string {
	if (!isoString) return "recent";
	const date = new Date(isoString);
	if (Number.isNaN(date.getTime())) return "recent";
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
	onAction(type, item);
}
</script>

<article
	class="rounded-lg border-l-4 p-4 bg-[var(--surface-bg)] border-[var(--surface-border)] border-l-[var(--interactive-text)] hover:border-[var(--interactive-text)] transition-colors duration-200 shadow-[var(--shadow-sm)]"
	data-item-key={item.itemKey}
>
	<!-- Header: Title + Supersede Badge + Relative Time -->
	<div class="flex items-start justify-between gap-2 mb-2">
		<div class="flex-1 min-w-0">
			<h3 class="text-sm font-semibold text-[var(--text-primary)] line-clamp-2">
				{item.title}
			</h3>
			{#if item.supersedeInfo}
				<div class="mt-1">
					<SupersedeBadge
						info={item.supersedeInfo}
						expanded={supersedeExpanded}
						onToggle={() => { supersedeExpanded = !supersedeExpanded; }}
					/>
				</div>
				{#if supersedeExpanded}
					<SupersedeDetail info={item.supersedeInfo} />
				{/if}
		{/if}
	</div>

	{#if whyExpanded}
		<div class="mb-3">
			<WhyPanel reasons={item.why.length > 0 ? item.why : displayReasons} />
		</div>
	{/if}
		<time
			class="text-xs text-[var(--text-secondary)] whitespace-nowrap flex-shrink-0"
			datetime={item.publishedAt}
		>
			{formatRelativeTime(item.publishedAt)}
		</time>
	</div>

	<!-- Why Badges + SummaryStateChip -->
	{#if displayReasons.length > 0 || item.summaryState === "pending"}
		<div class="flex flex-wrap items-center gap-1 mb-2">
			{#each displayReasons as reason}
				<WhySurfacedBadge {reason} />
			{/each}
			<SummaryStateChip state={item.summaryState} />
		</div>
	{/if}

	<!-- Summary Excerpt or Skeleton -->
	{#if item.summaryState === "ready" && item.summaryExcerpt}
		<p class="mb-2 text-xs leading-5 text-[var(--text-secondary)] line-clamp-2">
			{item.summaryExcerpt}
		</p>
	{:else if item.summaryState === "pending" || item.summaryState === "missing"}
		<div class="space-y-1 mb-2">
			<div class="h-3 w-full rounded bg-[var(--surface-hover)] animate-pulse"></div>
			<div class="h-3 w-2/3 rounded bg-[var(--surface-hover)] animate-pulse"></div>
		</div>
	{:else}
		<p class="mb-2 text-xs leading-5 text-[var(--text-secondary)] line-clamp-2">
			{item.summaryExcerpt}
		</p>
	{/if}

	<!-- Bottom Row: Tags (left) + Actions (right) -->
	<div class="flex items-center justify-between gap-2">
		{#if displayTags.length > 0}
			<div class="flex flex-wrap gap-1 min-w-0">
				{#each displayTags as tag}
					<span
						class="inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium bg-[var(--chip-bg)] border-[var(--chip-border)] text-[var(--chip-text)]"
					>
						{tag}
					</span>
				{/each}
				{#if remainingTagCount > 0}
					<span
						class="inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium bg-[var(--chip-bg)] border-[var(--chip-border)] text-[var(--chip-text)]"
					>
						+{remainingTagCount} tags
					</span>
				{/if}
			</div>
		{:else}
			<div></div>
		{/if}
		<QuickActionRow
			itemKey={item.itemKey}
			itemType={item.itemType}
			articleId={item.articleId}
			onAction={handleAction}
		/>
	</div>

	<div class="mt-2 flex justify-end">
		<button
			type="button"
			class="text-xs text-[var(--text-secondary)] hover:text-[var(--interactive-text)]"
			onclick={() => {
				whyExpanded = !whyExpanded;
			}}
		>
			{whyExpanded ? "Hide why" : "Explain why"}
		</button>
	</div>
</article>
