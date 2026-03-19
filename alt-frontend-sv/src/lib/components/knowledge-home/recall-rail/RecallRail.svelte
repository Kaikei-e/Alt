<script lang="ts">
import { Brain } from "@lucide/svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";
import RecallCandidateCard from "./RecallCandidateCard.svelte";

interface Props {
	candidates: RecallCandidateData[];
	unavailable?: boolean;
	onSnooze: (itemKey: string) => void;
	onDismiss: (itemKey: string) => void;
	onOpen: (itemKey: string) => void;
}

const {
	candidates,
	unavailable = false,
	onSnooze,
	onDismiss,
	onOpen,
}: Props = $props();
</script>

<aside class="sticky top-4 border rounded-xl p-4 bg-[var(--surface-2,var(--surface-bg))] border-[var(--surface-border)]">
	<h3 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-3 flex items-center gap-1.5">
		<Brain class="h-3.5 w-3.5 text-[var(--accent-primary,var(--interactive-text))]" />
		Recall
	</h3>

	{#if unavailable}
		<p class="text-sm text-[var(--text-tertiary)]">Recall is temporarily unavailable.</p>
	{:else if candidates.length === 0}
		<p class="text-sm text-[var(--text-tertiary)]">Nothing to recall right now.</p>
	{:else}
		<div class="flex flex-col gap-3">
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
