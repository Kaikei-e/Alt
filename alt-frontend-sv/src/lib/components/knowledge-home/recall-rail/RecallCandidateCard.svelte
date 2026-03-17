<script lang="ts">
import { Clock, X, AlarmClockOff } from "@lucide/svelte";
import RecallReasonBadge from "./RecallReasonBadge.svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";

interface Props {
	candidate: RecallCandidateData;
	onSnooze: (itemKey: string) => void;
	onDismiss: (itemKey: string) => void;
	onOpen: (itemKey: string) => void;
}

const { candidate, onSnooze, onDismiss, onOpen }: Props = $props();

const title = $derived(candidate.item?.title ?? candidate.itemKey);

function formatAge(dateStr: string): string {
	const diff = Date.now() - new Date(dateStr).getTime();
	const days = Math.floor(diff / (1000 * 60 * 60 * 24));
	if (days === 0) return "today";
	if (days === 1) return "1d ago";
	return `${days}d ago`;
}

const age = $derived(
	candidate.firstEligibleAt ? formatAge(candidate.firstEligibleAt) : "",
);
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
