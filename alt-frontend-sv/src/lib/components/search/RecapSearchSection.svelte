<script lang="ts">
import { goto } from "$app/navigation";
import type {
	RecapSectionData,
	GlobalRecapHitData,
} from "$lib/connect/global_search";
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

<section data-role="reference-recaps-section">
	<div class="ref-section-header">
		<h2 class="ref-section-label">
			RECAPS
			{#if section.estimatedTotal > 0}
				<span class="ref-section-count">({section.estimatedTotal})</span>
			{/if}
		</h2>
		{#if section.hasMore}
			<button type="button" onclick={seeAll} class="ref-see-all" data-role="see-all-recaps">
				See all &gt;
			</button>
		{/if}
	</div>

	{#if section.hits.length === 0}
		<p class="ref-empty-text">No matching recaps found.</p>
	{:else}
		<div class="ref-hits">
			{#each section.hits as hit, i (hit.id)}
				<button
					type="button"
					onclick={() => openRecapModal(hit)}
					class="ref-hit stagger-entry"
					style="--stagger: {i}"
					data-role="recap-hit"
				>
					<div class="ref-hit-header">
						<h3 class="ref-hit-title">{hit.genre}</h3>
						<span class="ref-field-badge">{windowLabel(hit.windowDays)}</span>
					</div>
					{#if hit.summary}
						<p class="ref-hit-snippet">{hit.summary}</p>
					{/if}
					{#if hit.topTerms.length > 0}
						<div class="ref-hit-meta">
							{#each hit.topTerms.slice(0, 5) as term}
								<span class="ref-tag-token">{term}</span>
							{/each}
						</div>
					{/if}
				</button>
			{/each}
		</div>
	{/if}
</section>

<RecapPreviewModal data={selectedRecap} open={modalOpen} onOpenChange={(v) => { modalOpen = v; }} />

<style>
	.ref-section-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		margin-bottom: 0.5rem;
	}

	.ref-section-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.ref-section-count {
		font-weight: 400;
	}

	.ref-see-all {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		letter-spacing: 0.04em;
		color: var(--alt-primary);
		background: transparent;
		border: none;
		cursor: pointer;
		padding: 0;
	}

	.ref-see-all:hover {
		color: var(--alt-charcoal);
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.ref-hits {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.ref-hit {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		width: 100%;
		text-align: left;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		cursor: pointer;
		transition: background 0.15s;
	}

	.ref-hit:hover {
		background: var(--surface-hover);
	}

	.ref-hit-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.ref-hit-title {
		font-family: var(--font-display);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.ref-hit-snippet {
		font-family: var(--font-body);
		font-size: 0.78rem;
		color: var(--alt-slate);
		line-height: 1.5;
		margin: 0;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.ref-hit-meta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
		margin-top: 0.2rem;
	}

	.ref-field-badge {
		font-family: var(--font-mono);
		font-size: 0.55rem;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-ash);
		padding: 0.1rem 0.4rem;
		border: 1px solid var(--surface-border);
	}

	.ref-tag-token {
		font-family: var(--font-mono);
		font-size: 0.55rem;
		color: var(--alt-ash);
		padding: 0.1rem 0.4rem;
		background: var(--surface-hover);
	}

	.ref-empty-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		font-style: italic;
	}

	.stagger-entry {
		opacity: 0;
		animation: reveal 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}

	@keyframes reveal {
		to {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.stagger-entry {
			animation: none;
			opacity: 1;
		}
	}
</style>
