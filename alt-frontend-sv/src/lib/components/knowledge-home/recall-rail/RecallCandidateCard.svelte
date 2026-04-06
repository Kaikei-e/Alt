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
const borderColor = $derived.by(() => {
	if (!primaryReason) return "border-l-[var(--surface-border)]";
	const display = resolveRecallReason(primaryReason.type);
	if (display.colorClass.includes("amber"))
		return "border-l-[var(--badge-amber-border)]";
	if (display.colorClass.includes("blue"))
		return "border-l-[var(--badge-blue-border)]";
	if (display.colorClass.includes("purple"))
		return "border-l-[var(--badge-purple-border)]";
	if (display.colorClass.includes("teal"))
		return "border-l-[var(--badge-teal-border)]";
	if (display.colorClass.includes("orange"))
		return "border-l-[var(--badge-orange-border)]";
	if (display.colorClass.includes("green"))
		return "border-l-[var(--badge-green-border)]";
	return "border-l-[var(--surface-border)]";
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
	class="border border-l-[3px] rounded-md p-3 bg-[var(--surface-bg)] border-[var(--surface-border)] {borderColor} hover:border-[var(--accent-primary)] hover:-translate-y-0.5 transition-all duration-200 cursor-pointer shadow-sm"
	role="button"
	tabindex="0"
	onclick={() => onOpen(candidate.itemKey)}
	onkeydown={(e) => { if (e.key === "Enter") onOpen(candidate.itemKey); }}
>
	<div class="flex items-start justify-between gap-2 mb-2">
		<h4 class="text-sm font-medium text-[var(--text-primary)] line-clamp-2 flex-1">
			{title}
		</h4>
		{#if age}
			<span class="text-xs text-[var(--text-tertiary)] whitespace-nowrap flex items-center gap-1">
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
				class="rounded-full border border-[var(--surface-border)] px-2.5 py-0.5 text-xs font-medium text-[var(--text-muted)] hover:border-[var(--interactive-text)] hover:text-[var(--interactive-text)] transition-colors ml-auto"
				onclick={(e) => { e.stopPropagation(); whyExpanded = !whyExpanded; }}
			>
				{whyExpanded ? "Hide why" : "Why?"}
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
		<p class="mb-2 text-xs leading-5 text-[var(--text-secondary)] line-clamp-2">
			{summaryExcerpt}
		</p>
	{/if}

	{#if displayTags.length > 0}
		<div class="flex flex-wrap gap-1 mb-2">
			{#each displayTags as tag}
				<span
					class="inline-flex items-center rounded border px-2 py-0.5 text-xs font-medium bg-[var(--chip-bg)] border-[var(--chip-border)] text-[var(--chip-text)]"
				>
					{tag}
				</span>
			{/each}
		</div>
	{/if}

	<div class="flex items-center gap-1 mt-2">
		<button
			class="p-1.5 rounded text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] transition-colors"
			title="Snooze for 24 hours"
			onclick={(e) => { e.stopPropagation(); onSnooze(candidate.itemKey); }}
		>
			<AlarmClockOff class="h-3.5 w-3.5" />
		</button>
		<button
			class="p-1.5 rounded text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] transition-colors"
			title="Dismiss"
			onclick={(e) => { e.stopPropagation(); onDismiss(candidate.itemKey); }}
		>
			<X class="h-3.5 w-3.5" />
		</button>
	</div>
</div>
