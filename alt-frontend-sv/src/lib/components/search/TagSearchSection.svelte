<script lang="ts">
import { goto } from "$app/navigation";
import type { TagSectionData } from "$lib/connect/global_search";

interface Props {
	section: TagSectionData;
	query: string;
}

const { section, query }: Props = $props();

function navigateToTag(tagName: string) {
	goto(`/articles/by-tag?tag=${encodeURIComponent(tagName)}`);
}
</script>

<section data-role="reference-tags-section">
	<div class="ref-section-header">
		<h2 class="ref-section-label">
			TAGS
			{#if section.total > 0}
				<span class="ref-section-count">({section.total})</span>
			{/if}
		</h2>
	</div>

	{#if section.hits.length === 0}
		<p class="ref-empty-text">No matching tags found.</p>
	{:else}
		<div class="ref-tag-grid">
			{#each section.hits as hit, i (hit.tagName)}
				<button
					type="button"
					onclick={() => navigateToTag(hit.tagName)}
					class="ref-tag-button stagger-entry"
					style="--stagger: {i}"
					data-role="tag-hit"
				>
					<span class="ref-tag-name">{hit.tagName}</span>
					<span class="ref-tag-count">({hit.articleCount})</span>
				</button>
			{/each}
		</div>
	{/if}
</section>

<style>
	.ref-section-header {
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

	.ref-tag-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.ref-tag-button {
		display: inline-flex;
		align-items: center;
		gap: 0.35rem;
		padding: 0.35rem 0.75rem;
		min-height: 36px;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		cursor: pointer;
		transition: background 0.15s, border-color 0.15s;
	}

	.ref-tag-button:hover {
		background: var(--surface-hover);
		border-color: var(--alt-charcoal);
	}

	.ref-tag-name {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-charcoal);
	}

	.ref-tag-count {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
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
