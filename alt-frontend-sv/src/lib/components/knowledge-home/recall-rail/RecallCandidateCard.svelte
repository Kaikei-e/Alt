<script lang="ts">
import { AlarmClockOff, Clock, X } from "@lucide/svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";
import RecallReasonBadge from "./RecallReasonBadge.svelte";

interface Props {
	candidate: RecallCandidateData;
	onSnooze: (itemKey: string) => void;
	onDismiss: (itemKey: string) => void;
	onOpen: (itemKey: string) => void;
}

const { candidate, onSnooze, onDismiss, onOpen }: Props = $props();

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
	class="border rounded-lg p-3 bg-[var(--surface-bg)] border-[var(--surface-border)] hover:border-[var(--accent-primary)] transition-colors cursor-pointer"
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

	<div class="flex flex-wrap gap-1 mb-2">
		{#each candidate.reasons.slice(0, 2) as reason}
			<RecallReasonBadge reasonType={reason.type} />
		{/each}
	</div>

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
			class="p-1 rounded text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] transition-colors"
			title="Snooze for 24 hours"
			onclick={(e) => { e.stopPropagation(); onSnooze(candidate.itemKey); }}
		>
			<AlarmClockOff class="h-3.5 w-3.5" />
		</button>
		<button
			class="p-1 rounded text-[var(--text-tertiary)] hover:text-[var(--text-secondary)] hover:bg-[var(--surface-hover)] transition-colors"
			title="Dismiss"
			onclick={(e) => { e.stopPropagation(); onDismiss(candidate.itemKey); }}
		>
			<X class="h-3.5 w-3.5" />
		</button>
	</div>
</div>
