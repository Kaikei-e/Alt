<script lang="ts">
import { Info } from "@lucide/svelte";
import type { RecallReasonData } from "$lib/connect/knowledge_home";
import { categorizeRecallReasons } from "./recall-why-categories";

interface Props {
	reasons: RecallReasonData[];
}

const { reasons }: Props = $props();
const groups = $derived(categorizeRecallReasons(reasons));
const hasReasons = $derived(groups.length > 0);
</script>

<div class="animate-fade-up rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-3">
	<h4 class="text-xs font-semibold text-[var(--text-secondary)] uppercase tracking-wider mb-2 flex items-center gap-1.5">
		<Info class="h-3.5 w-3.5" />
		Why recalled?
	</h4>

	{#if hasReasons}
		<div class="space-y-3">
			{#each groups as group}
				<div class="space-y-1.5">
					<p class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]">
						{group.label}
					</p>
					<div class="space-y-1.5">
						{#each group.items as { reason, displayLabel }}
							<div class="rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] px-3 py-2.5">
								<p class="text-xs font-medium text-[var(--text-primary)]">
									{displayLabel}
								</p>
								{#if reason.description}
									<p class="mt-0.5 text-xs leading-relaxed text-[var(--text-secondary)] italic">
										{reason.description}
									</p>
								{/if}
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{:else}
		<p class="text-xs text-[var(--text-tertiary)]">
			No specific reason recorded
		</p>
	{/if}
</div>
