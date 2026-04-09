<script lang="ts">
import { goto } from "$app/navigation";
import type {
	RecapSectionData,
	GlobalRecapHitData,
} from "$lib/connect/global_search";
import { BookOpen, ChevronRight } from "@lucide/svelte";
import {
	RecapPreviewModal,
	fromGlobalRecapHit,
	type RecapModalData,
} from "$lib/components/recap";

interface Props {
	section: RecapSectionData;
	query: string;
}

const { section, query }: Props = $props();

let selectedRecap = $state<RecapModalData | null>(null);
let modalOpen = $state(false);

function windowLabel(days: number): string {
	return `${days}-day`;
}

function openRecapModal(hit: GlobalRecapHitData) {
	selectedRecap = fromGlobalRecapHit(hit);
	modalOpen = true;
}

function seeAll() {
	goto(`/recap?q=${encodeURIComponent(query)}`);
}
</script>

<section class="space-y-3">
	<div class="flex items-center justify-between">
		<h2
			class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]"
		>
			Recaps
			{#if section.estimatedTotal > 0}
				<span class="ml-1 font-normal text-[var(--text-secondary)]"
					>({section.estimatedTotal})</span
				>
			{/if}
		</h2>
		{#if section.hasMore}
			<button
				type="button"
				onclick={seeAll}
				class="inline-flex items-center gap-1 text-xs text-[var(--interactive-text)] hover:text-[var(--interactive-text-hover)] transition-colors"
			>
				See all <ChevronRight class="h-3 w-3" />
			</button>
		{/if}
	</div>

	{#if section.hits.length === 0}
		<p class="text-sm text-[var(--text-secondary)] italic">
			No matching recaps found.
		</p>
	{:else}
		<div class="space-y-2">
			{#each section.hits as hit (hit.id)}
				<button
					type="button"
					onclick={() => openRecapModal(hit)}
					class="w-full text-left rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 space-y-2 cursor-pointer hover:border-[var(--interactive-text)] transition-colors"
				>
					<div class="flex items-start gap-3">
						<BookOpen
							class="mt-0.5 h-4 w-4 flex-shrink-0 text-[var(--text-secondary)]"
						/>
						<div class="min-w-0 flex-1 space-y-1.5">
							<div class="flex items-center gap-2">
								<h3
									class="text-sm font-medium text-[var(--text-primary)]"
								>
									{hit.genre}
								</h3>
								<span
									class="inline-block rounded border border-[var(--surface-border)] px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-[var(--text-secondary)]"
								>
									{windowLabel(hit.windowDays)}
								</span>
							</div>
							{#if hit.summary}
								<p
									class="text-xs text-[var(--text-secondary)] leading-relaxed line-clamp-3"
								>
									{hit.summary}
								</p>
							{/if}
							{#if hit.topTerms.length > 0}
								<div class="flex flex-wrap gap-1.5">
									{#each hit.topTerms.slice(0, 5) as term}
										<span
											class="inline-block rounded-full bg-[var(--surface-hover)] px-2 py-0.5 text-[10px] font-medium text-[var(--text-secondary)]"
										>
											{term}
										</span>
									{/each}
								</div>
							{/if}
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</section>

<RecapPreviewModal data={selectedRecap} open={modalOpen} onOpenChange={(v) => { modalOpen = v; }} />
