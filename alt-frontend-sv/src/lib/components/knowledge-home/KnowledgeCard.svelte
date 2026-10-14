<script lang="ts">
import type { KnowledgeHomeItemData } from "#lib/connect/knowledge_home.js";
import { safeArticleHref } from "#lib/utils/safeHref.js";
import QuickActionRow from "./QuickActionRow.svelte";
import SummaryStateChip from "./SummaryStateChip.svelte";
import SupersedeBadge from "./SupersedeBadge.svelte";
import SupersedeDetail from "./SupersedeDetail.svelte";
import WhySurfacedBadge from "./WhySurfacedBadge.svelte";

interface Props {
	item: KnowledgeHomeItemData;
	onAction: (type: string, item: KnowledgeHomeItemData) => void;
	onTagClick?: (tag: string, item: KnowledgeHomeItemData) => void;
}

const { item, onAction, onTagClick }: Props = $props();

// safeHref: null = no usable URL. Three reasons in increasing severity:
//   - missing      : item.url is empty (legitimate gap, e.g. corrective
//                    event hasn't run yet for a historical row)
//   - scheme-blocked: item.url is non-empty but scheme is not http(s)
//                    (defense-in-depth; backend should never let this
//                    through but FE renders the kicker either way)
// Both surface the same Archived kicker per docs/glossary —
// frontend-design ruled visual disambiguation as noise. Operator can
// distinguish via the data-link-unavailable-reason attribute / DevTools.
const safeHref = $derived(safeArticleHref(item.url));
const linkAvailable = $derived(
	item.itemType === "article" ? safeHref != null : true,
);
const linkUnavailableReason = $derived(
	item.itemType !== "article" || linkAvailable
		? undefined
		: !item.url
			? "missing"
			: "scheme-blocked",
);

const sourceHost = $derived.by(() => {
	if (!safeHref) return "";
	try {
		return new URL(safeHref).hostname.replace(/^www\./, "");
	} catch {
		return "";
	}
});

const warnedItemKeys = new Set<string>();
$effect(() => {
	if (linkAvailable) return;
	if (warnedItemKeys.has(item.itemKey)) return;
	warnedItemKeys.add(item.itemKey);
	console.warn(
		"kh-card-link-unavailable",
		item.itemKey,
		linkUnavailableReason,
		"see docs/glossary/ubiquitous-language.md (Article URL)",
	);
});

const nonEmptyTags = $derived(item.tags.filter((t) => t.trim() !== ""));
const displayTags = $derived(nonEmptyTags.slice(0, 3));
const remainingTagCount = $derived(
	nonEmptyTags.length > 3 ? nonEmptyTags.length - 3 : 0,
);
let tagsExpanded = $state(false);
const visibleTags = $derived(tagsExpanded ? nonEmptyTags : displayTags);
const displayReasons = $derived(
	item.why
		.filter((r) => r.code !== "new_unread" && r.code !== "summary_completed")
		.slice(0, 2),
);
const isNeedToKnow = $derived(
	item.why.some((r) => r.code === "pulse_need_to_know"),
);
let supersedeExpanded = $state(false);

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
	class="card {isNeedToKnow ? 'card--urgent' : ''} {linkAvailable
		? ''
		: 'card--no-source'}"
	data-item-key={item.itemKey}
	data-link-unavailable={linkAvailable ? undefined : "true"}
	data-link-unavailable-reason={linkUnavailableReason}
>
	{#if !linkAvailable}
		<div
			class="card-no-source"
			data-testid="kh-card-link-unavailable"
			aria-disabled="true"
			aria-label="Archived — no source URL"
		>
			Archived · No source URL
		</div>
	{/if}

	<!-- Header: Title + Supersede Badge + Source & Relative Time -->
	<div class="card-head flex items-start justify-between gap-4 mb-2">
		<div class="flex-1 min-w-0">
			<h3 class="card-title">
				{#if linkAvailable}
					<button
						type="button"
						class="card-title-btn"
						onclick={() => handleAction("open")}
					>
						{item.title}
					</button>
				{:else}
					{item.title}
				{/if}
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

		<div class="card-meta">
			{#if sourceHost}
				<span class="card-source" data-testid="card-source">{sourceHost}</span>
				<span class="card-meta-sep" aria-hidden="true">·</span>
			{/if}
			<time
				class="card-time"
				datetime={item.publishedAt}
			>
				{formatRelativeTime(item.publishedAt)}
			</time>
		</div>
	</div>

	<!-- High-Signal Why Badges + SummaryStateChip -->
	{#if displayReasons.length > 0 || item.summaryState === "pending"}
		<div class="flex flex-wrap items-center gap-1.5 mb-2.5">
			{#each displayReasons as reason}
				<WhySurfacedBadge {reason} />
			{/each}
			<SummaryStateChip state={item.summaryState} />
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
			<div class="flex flex-wrap gap-1.5 min-w-0">
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
		padding: 1.1rem 1.25rem 0.85rem;
		background: var(--surface-bg);
		transition: border-color 0.15s;
	}

	.card:hover {
		border-color: var(--alt-charcoal);
	}

	.card--urgent {
		border-left: 3px solid var(--accent-emphasis-text);
	}

	.card--no-source {
		border-style: dashed;
		background: transparent;
	}

	.card-no-source {
		display: inline-block;
		font-family: var(--font-mono);
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-slate);
		padding: 0.14rem 0.5rem;
		border: 1px solid var(--surface-border);
		margin-bottom: 0.6rem;
	}

	.card-title {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.card-title-btn {
		all: unset;
		cursor: pointer;
		color: inherit;
		font: inherit;
		text-align: left;
		display: inline;
	}

	.card-title-btn:hover {
		color: var(--interactive-text-hover);
		text-decoration: underline;
		text-decoration-thickness: 1px;
		text-underline-offset: 3px;
	}

	.card-title-btn:focus-visible {
		outline: 2px solid var(--alt-primary);
		outline-offset: 2px;
	}

	.card-meta {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		flex-shrink: 0;
		padding-top: 0.3rem;
	}

	.card-source {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-slate);
		max-width: 11rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.card-meta-sep {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-ash);
	}

	.card-time {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-ash);
		white-space: nowrap;
		flex-shrink: 0;
	}

	/* Narrow screens: the source/time line moves above the headline like a
	   dateline so the title keeps the full width. */
	@media (max-width: 640px) {
		.card-head {
			flex-direction: column-reverse;
			gap: 0.25rem;
		}
		.card-meta {
			padding-top: 0;
		}
	}

	.card-summary {
		font-family: var(--font-body);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--text-secondary);
		margin-bottom: 0.25rem;
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
		gap: 0.75rem;
		border-top: 1px dashed var(--surface-border);
		padding-top: 0.6rem;
		margin-top: 0.75rem;
	}

	.card-tag {
		display: inline-flex;
		align-items: center;
		border: 1px solid var(--chip-border);
		padding: 0.08rem 0.5rem;
		font-family: var(--font-mono);
		font-size: 0.68rem;
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
		background: transparent;
		border-style: dashed;
		color: var(--alt-slate);
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
