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
	onTagClick?: (tag: string, item: KnowledgeHomeItemData) => void;
}

const { item, onAction, onTagClick }: Props = $props();

const nonEmptyTags = $derived(item.tags.filter((t) => t.trim() !== ""));
const displayTags = $derived(nonEmptyTags.slice(0, 3));
const remainingTagCount = $derived(
	nonEmptyTags.length > 3 ? nonEmptyTags.length - 3 : 0,
);
let tagsExpanded = $state(false);
const visibleTags = $derived(tagsExpanded ? nonEmptyTags : displayTags);
const displayReasons = $derived(
	item.why.length > 0 ? item.why.slice(0, 2) : [{ code: "new_unread" }],
);
const isNeedToKnow = $derived(
	item.why.some((r) => r.code === "pulse_need_to_know"),
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
	class="card {isNeedToKnow ? 'card--urgent' : ''}"
	data-item-key={item.itemKey}
>
	<!-- Header: Title + Supersede Badge + Relative Time -->
	<div class="flex items-start justify-between gap-2 mb-2">
		<div class="flex-1 min-w-0">
			<h3 class="card-title">
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
			class="card-time"
			datetime={item.publishedAt}
		>
			{formatRelativeTime(item.publishedAt)}
		</time>
	</div>

	<!-- Why Badges + SummaryStateChip + Explain why -->
	{#if displayReasons.length > 0 || item.summaryState === "pending"}
		<div class="flex flex-wrap items-center gap-1 mb-2">
			{#each displayReasons as reason}
				<WhySurfacedBadge {reason} />
			{/each}
			<SummaryStateChip state={item.summaryState} />
			<button
				type="button"
				class="why-toggle"
				onclick={() => {
					whyExpanded = !whyExpanded;
				}}
			>
				{whyExpanded ? "HIDE WHY" : "EXPLAIN WHY"}
			</button>
		</div>
	{/if}

	<!-- Summary Excerpt or Skeleton -->
	{#if item.summaryState === "ready" && item.summaryExcerpt}
		<p class="card-summary">
			{item.summaryExcerpt}
		</p>
	{:else if item.summaryState === "pending" || item.summaryState === "missing"}
		<div class="space-y-1 mb-2">
			<div class="skeleton-line"></div>
			<div class="skeleton-line skeleton-line--short"></div>
		</div>
	{:else}
		<p class="card-summary">
			{item.summaryExcerpt}
		</p>
	{/if}

	<!-- Bottom Row: Tags (left) + Actions (right) -->
	<div class="card-footer">
		{#if nonEmptyTags.length > 0}
			<div class="flex flex-wrap gap-1 min-w-0">
				{#each visibleTags as tag}
					<a
						href="/articles/by-tag?tag={encodeURIComponent(tag)}"
						class="card-tag"
						onclick={() => {
							onTagClick?.(tag, item);
						}}
					>
						{tag}
					</a>
				{/each}
				{#if remainingTagCount > 0 && !tagsExpanded}
					<button
						type="button"
						class="card-tag card-tag--expand"
						onclick={() => { tagsExpanded = true; }}
					>
						+{remainingTagCount} tags
					</button>
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
</article>

<style>
	.card {
		border: 1px solid var(--surface-border);
		padding: 1.25rem 1.25rem;
		background: var(--surface-bg);
		transition: border-color 0.15s;
	}

	.card:hover {
		border-color: var(--alt-charcoal);
	}

	.card--urgent {
		border-left: 3px solid var(--accent-emphasis-text);
	}

	.card-title {
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.card-time {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.card-summary {
		font-family: var(--font-body);
		font-size: 0.875rem;
		line-height: 1.6;
		color: var(--alt-slate);
		margin-bottom: 0.5rem;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.card-footer {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		border-top: 1px solid color-mix(in srgb, var(--surface-border) 40%, transparent);
		padding-top: 0.75rem;
		margin-top: 0.75rem;
	}

	.card-tag {
		display: inline-flex;
		align-items: center;
		border: 1px solid var(--chip-border);
		padding: 0.125rem 0.5rem;
		font-size: 0.75rem;
		font-weight: 500;
		background: var(--chip-bg);
		color: var(--chip-text);
		transition: border-color 0.15s, color 0.15s;
		text-decoration: none;
	}

	.card-tag:hover {
		border-color: var(--interactive-text);
		color: var(--interactive-text);
	}

	.card-tag--expand {
		cursor: pointer;
	}

	.why-toggle {
		margin-left: auto;
		border: 1px solid var(--surface-border);
		padding: 0.125rem 0.625rem;
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--alt-ash);
		background: transparent;
		cursor: pointer;
		transition: border-color 0.15s, color 0.15s;
	}

	.why-toggle:hover {
		border-color: var(--interactive-text);
		color: var(--interactive-text);
	}

	.skeleton-line {
		height: 0.75rem;
		width: 100%;
		background: var(--surface-hover);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.skeleton-line--short {
		width: 66%;
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
</style>
