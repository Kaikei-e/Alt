<script lang="ts">
import { goto } from "$app/navigation";
import type { ArticleSectionData } from "$lib/connect/global_search";

interface Props {
	section: ArticleSectionData;
	query: string;
}

const { section, query }: Props = $props();

function navigateToArticle(id: string, link: string, title: string) {
	const params = new URLSearchParams({ url: link });
	if (title) params.set("title", title);
	goto(`/articles/${id}?${params.toString()}`);
}

function seeAll() {
	goto(`/feeds/search?q=${encodeURIComponent(query)}`);
}
</script>

<section data-role="reference-articles-section">
	<div class="ref-section-header">
		<h2 class="ref-section-label">
			ARTICLES
			{#if section.estimatedTotal > 0}
				<span class="ref-section-count">({section.estimatedTotal})</span>
			{/if}
		</h2>
		{#if section.hasMore}
			<button type="button" onclick={seeAll} class="ref-see-all" data-role="see-all-articles">
				See all &gt;
			</button>
		{/if}
	</div>

	{#if section.hits.length === 0}
		<p class="ref-empty-text">No matching articles found.</p>
	{:else}
		<div class="ref-hits">
			{#each section.hits as hit, i (hit.id)}
				<button
					type="button"
					onclick={() => navigateToArticle(hit.id, hit.link, hit.title)}
					class="ref-hit stagger-entry"
					style="--stagger: {i}"
					data-role="article-hit"
				>
					<h3 class="ref-hit-title">{hit.title}</h3>
					{#if hit.snippet}
						<p class="ref-hit-snippet">{@html hit.snippet}</p>
					{/if}
					<div class="ref-hit-meta">
						{#each hit.matchedFields as field}
							<span class="ref-field-badge">{field}</span>
						{/each}
						{#each hit.tags.slice(0, 3) as tag}
							<span class="ref-tag-token">{tag}</span>
						{/each}
					</div>
				</button>
			{/each}
		</div>
	{/if}
</section>

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

	.ref-hit-title {
		font-family: var(--font-display);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		line-height: 1.3;
		margin: 0;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.ref-hit-snippet {
		font-family: var(--font-body);
		font-size: 0.78rem;
		color: var(--alt-slate);
		line-height: 1.5;
		margin: 0;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
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
