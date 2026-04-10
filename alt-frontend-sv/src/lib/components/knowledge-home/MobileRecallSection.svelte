<script lang="ts">
import { Brain } from "@lucide/svelte";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";
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

const displayCandidates = $derived(candidates.slice(0, 2));
const hasMore = $derived(candidates.length > 2);
</script>

{#if unavailable}
	<div class="mx-4 mt-3 border border-[var(--recall-border)] bg-[var(--recall-bg)] px-4 py-3">
		<p class="text-sm text-[var(--text-tertiary)]">Recall is temporarily unavailable.</p>
	</div>
{:else if candidates.length > 0}
	<div class="mx-4 mt-3 border border-[var(--recall-border)] border-t-2 border-t-[var(--text-primary)] bg-[var(--recall-bg)]">
		<!-- Section header -->
		<div class="flex items-center gap-1.5 px-4 pt-3 pb-2">
			<Brain class="h-3.5 w-3.5 text-[var(--text-secondary)]" />
			<span class="font-[var(--font-display)] text-sm italic text-[var(--text-secondary)]">
				Resume where you left off
			</span>
		</div>

		<!-- Recall items (max 2) -->
		<div class="px-3 pb-3 flex flex-col gap-2">
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
			<div class="border-t border-[var(--divider-rule)] px-4 py-2.5">
				<button
					type="button"
					class="text-xs font-medium text-[var(--interactive-text)] hover:underline"
					onclick={() => {/* TODO: open full recall sheet */}}
				>
					See all recalls ({candidates.length})
				</button>
			</div>
		{/if}
	</div>
{/if}
