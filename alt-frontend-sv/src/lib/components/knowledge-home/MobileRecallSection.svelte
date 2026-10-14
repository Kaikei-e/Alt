<script lang="ts">
import { Brain } from "@lucide/svelte";
import type { RecallCandidateData } from "#lib/connect/knowledge_home.js";
import RecallCandidateCard from "./recall-rail/RecallCandidateCard.svelte";

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

let showAll = $state(false);
const displayCandidates = $derived(
	showAll ? candidates : candidates.slice(0, 2),
);
const hasMore = $derived(candidates.length > 2);
</script>

{#if unavailable}
	<div class="mx-4 mt-4 border-t-2 border-t-[var(--alt-charcoal)] pt-3">
		<p class="text-sm italic text-[var(--alt-ash)]">Recall is temporarily unavailable.</p>
	</div>
{:else if candidates.length > 0}
	<div class="mx-4 mt-4 border-t-2 border-t-[var(--alt-charcoal)]">
		<!-- Section header -->
		<div class="recall-label">
			<Brain class="h-3.5 w-3.5" />
			<span>
				Resume where you left off
			</span>
		</div>

		<!-- Recall items -->
		<div class="flex flex-col">
			{#each displayCandidates as candidate (candidate.itemKey)}
				<RecallCandidateCard
					{candidate}
					{onSnooze}
					{onDismiss}
					{onOpen}
				/>
			{/each}
		</div>

		<!-- See all link -->
		{#if hasMore}
			<div class="py-2.5">
				<button
					type="button"
					class="recall-more"
					onclick={() => { showAll = !showAll; }}
				>
					{showAll ? "Show fewer" : `See all recalls (${candidates.length})`}
				</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.recall-label {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--surface-border);
		font-family: var(--font-mono);
		font-size: 0.66rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate);
	}

	.recall-more {
		font-family: var(--font-body);
		font-size: 0.82rem;
		font-weight: 500;
		color: var(--interactive-text);
	}

	.recall-more:hover {
		text-decoration: underline;
		text-underline-offset: 3px;
	}
</style>
