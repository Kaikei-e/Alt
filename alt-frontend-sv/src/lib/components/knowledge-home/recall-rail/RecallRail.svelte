<script lang="ts">
import { Brain } from "@lucide/svelte";
import type { RecallCandidateData } from "#lib/connect/knowledge_home.js";
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
		<Brain class="h-3.5 w-3.5" />
		RECALL
	</h3>

	{#if unavailable}
		<p class="rail-empty">Recall is temporarily unavailable.</p>
	{:else if candidates.length === 0}
		<p class="rail-empty">Nothing to recall right now.</p>
	{:else}
		<div class="flex flex-col">
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
		border-top: 2px solid var(--alt-charcoal);
		padding-top: 0.6rem;
	}

	.rail-label {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		font-family: var(--font-mono);
		font-size: 0.66rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		color: var(--alt-slate);
		padding-bottom: 0.6rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.rail-empty {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
		padding-top: 0.75rem;
	}
</style>
