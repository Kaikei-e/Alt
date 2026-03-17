<script lang="ts">
import { ChevronDown, ChevronUp, Brain } from "@lucide/svelte";
import RecallCandidateCard from "./RecallCandidateCard.svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";

interface Props {
	candidates: RecallCandidateData[];
	onSnooze: (itemKey: string) => void;
	onDismiss: (itemKey: string) => void;
	onOpen: (itemKey: string) => void;
}

const { candidates, onSnooze, onDismiss, onOpen }: Props = $props();
let expanded = $state(false);
</script>

{#if candidates.length > 0}
	<div class="border rounded-lg bg-[var(--surface-bg)] border-[var(--surface-border)]">
		<button
			class="w-full flex items-center justify-between px-4 py-3 text-sm"
			onclick={() => { expanded = !expanded; }}
		>
			<span class="flex items-center gap-1.5 text-[var(--text-secondary)] font-semibold uppercase text-xs tracking-wider">
				<Brain class="h-3.5 w-3.5" />
				Recall ({candidates.length})
			</span>
			{#if expanded}
				<ChevronUp class="h-4 w-4 text-[var(--text-tertiary)]" />
			{:else}
				<ChevronDown class="h-4 w-4 text-[var(--text-tertiary)]" />
			{/if}
		</button>

		{#if expanded}
			<div class="px-4 pb-3 flex flex-col gap-2">
				{#each candidates as candidate (candidate.itemKey)}
					<RecallCandidateCard
						{candidate}
						{onSnooze}
						{onDismiss}
						{onOpen}
					/>
				{/each}
			</div>
		{/if}
	</div>
{/if}
