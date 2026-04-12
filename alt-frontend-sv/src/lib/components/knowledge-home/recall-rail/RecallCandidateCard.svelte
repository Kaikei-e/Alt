<script lang="ts">
import { AlarmClockOff, Clock, X } from "@lucide/svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";
import RecallReasonBadge from "./RecallReasonBadge.svelte";
import RecallWhyPanel from "./RecallWhyPanel.svelte";
import { resolveRecallReason } from "./recall-reason-map";

interface Props {
	candidate: RecallCandidateData;
	onSnooze: (itemKey: string) => void;
	onDismiss: (itemKey: string) => void;
	onOpen: (itemKey: string) => void;
}

const { candidate, onSnooze, onDismiss, onOpen }: Props = $props();

let whyExpanded = $state(false);

const title = $derived(candidate.item?.title ?? candidate.itemKey);
const summaryExcerpt = $derived(
	candidate.item?.summaryState === "ready"
		? (candidate.item.summaryExcerpt ?? "")
		: "",
);
const displayTags = $derived(
	(candidate.item?.tags ?? []).filter((tag) => tag.trim() !== "").slice(0, 2),
);
const dateSource = $derived(
	candidate.item?.publishedAt || candidate.firstEligibleAt || "",
);
const primaryReason = $derived(candidate.reasons[0]);
const isUrgent = $derived.by(() => {
	if (!primaryReason) return false;
	const display = resolveRecallReason(primaryReason.type);
	return display.colorClass.includes("accent-emphasis");
});

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

const age = $derived(formatRelativeTime(dateSource));
</script>

<div
	class="recall-card"
	class:recall-card--urgent={isUrgent}
	role="button"
	tabindex="0"
	onclick={() => onOpen(candidate.itemKey)}
	onkeydown={(e) => { if (e.key === "Enter") onOpen(candidate.itemKey); }}
>
	<div class="flex items-start justify-between gap-2 mb-2">
		<h4 class="recall-title">{title}</h4>
		{#if age}
			<span class="recall-time">
				<Clock class="h-3 w-3" />
				{age}
			</span>
		{/if}
	</div>

	<!-- Reason badges + Why recalled? toggle -->
	{#if candidate.reasons.length > 0}
		<div class="flex flex-wrap items-center gap-1 mb-2">
			{#each candidate.reasons.slice(0, 2) as reason}
				<RecallReasonBadge reasonType={reason.type} description={reason.description} />
			{/each}
			<button
				type="button"
				class="recall-why-toggle"
				onclick={(e) => { e.stopPropagation(); whyExpanded = !whyExpanded; }}
			>
				{whyExpanded ? "HIDE WHY" : "WHY?"}
			</button>
		</div>
	{/if}

	<!-- Expanded Why Panel -->
	{#if whyExpanded}
		<!-- svelte-ignore a11y_no_static_element_interactions -->
		<div class="mb-2" onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()}>
			<RecallWhyPanel reasons={candidate.reasons} />
		</div>
	{/if}

	{#if summaryExcerpt}
		<p class="recall-summary">{summaryExcerpt}</p>
	{/if}

	{#if displayTags.length > 0}
		<div class="flex flex-wrap gap-1 mb-2">
			{#each displayTags as tag}
				<span class="recall-tag">{tag}</span>
			{/each}
		</div>
	{/if}

	<div class="flex items-center gap-1 mt-2">
		<button
			class="recall-action"
			title="Snooze for 24 hours"
			onclick={(e) => { e.stopPropagation(); onSnooze(candidate.itemKey); }}
		>
			<AlarmClockOff class="h-3.5 w-3.5" />
		</button>
		<button
			class="recall-action"
			title="Dismiss"
			onclick={(e) => { e.stopPropagation(); onDismiss(candidate.itemKey); }}
		>
			<X class="h-3.5 w-3.5" />
		</button>
	</div>
</div>

<style>
	.recall-card {
		border: 1px solid var(--surface-border);
		padding: 0.75rem;
		background: var(--surface-bg);
		cursor: pointer;
		transition: border-color 0.15s;
	}

	.recall-card:hover {
		border-color: var(--alt-charcoal);
	}

	.recall-card--urgent {
		border-left: 3px solid var(--accent-emphasis-text);
	}

	.recall-title {
		font-family: var(--font-display);
		font-size: 0.875rem;
		font-weight: 600;
		line-height: 1.3;
		color: var(--alt-charcoal);
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		flex: 1;
	}

	.recall-time {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
		white-space: nowrap;
	}

	.recall-summary {
		font-family: var(--font-body);
		font-size: 0.75rem;
		line-height: 1.4;
		color: var(--alt-slate);
		margin-bottom: 0.5rem;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.recall-tag {
		display: inline-flex;
		align-items: center;
		border: 1px solid var(--chip-border);
		padding: 0.125rem 0.5rem;
		font-size: 0.75rem;
		font-weight: 500;
		background: var(--chip-bg);
		color: var(--chip-text);
	}

	.recall-why-toggle {
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

	.recall-why-toggle:hover {
		border-color: var(--interactive-text);
		color: var(--interactive-text);
	}

	.recall-action {
		padding: 0.375rem;
		color: var(--alt-ash);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: color 0.15s, background 0.15s;
	}

	.recall-action:hover {
		color: var(--alt-slate);
		background: var(--surface-hover);
	}
</style>
