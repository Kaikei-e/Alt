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

<aside class="rail">
	<h3 class="rail-label">
		<Brain class="h-3.5 w-3.5" style="color: var(--alt-primary);" />
		RECALL
	</h3>

	{#if unavailable}
		<p class="rail-empty">Recall is temporarily unavailable.</p>
	{:else if candidates.length === 0}
		<p class="rail-empty">Nothing to recall right now.</p>
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

<style>
	.rail {
		position: sticky;
		top: 1rem;
		border: 1px solid var(--surface-border);
		padding: 1rem;
		background: color-mix(in srgb, var(--surface-2) 100%, var(--surface-bg));
	}

	.rail-label {
		display: flex;
		align-items: center;
		gap: 0.375rem;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--alt-ash);
		margin-bottom: 0.75rem;
	}

	.rail-empty {
		font-family: var(--font-body);
		font-size: 0.875rem;
		color: var(--alt-ash);
	}
</style>
