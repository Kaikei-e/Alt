<script lang="ts">
import { Brain } from "@lucide/svelte";
import RecallCandidateCard from "./RecallCandidateCard.svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";

interface Props {
	candidates: RecallCandidateData[];
	onSnooze: (itemKey: string) => void;
	onDismiss: (itemKey: string) => void;
	onOpen: (itemKey: string) => void;
}

const { candidates, onSnooze, onDismiss, onOpen }: Props = $props();
</script>

<aside class="border rounded-lg p-4 bg-[var(--surface-bg)] border-[var(--surface-border)]">
	<h3 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-3 flex items-center gap-1.5">
		<Brain class="h-3.5 w-3.5" />
		Recall
	</h3>

	{#if candidates.length === 0}
		<p class="text-sm text-[var(--text-tertiary)]">Nothing to recall right now.</p>
	{:else}
		<div class="flex flex-col gap-2">
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
</aside>
