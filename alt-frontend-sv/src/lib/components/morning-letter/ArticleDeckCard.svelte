<script lang="ts">
import type { MorningLetterBulletEnrichment } from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";

type Props = {
	enrichment: MorningLetterBulletEnrichment;
};

let { enrichment }: Props = $props();

const title = $derived(enrichment.articleTitle || "Untitled article");
const altHref = $derived(enrichment.articleAltHref || "");
const hasExcerpt = $derived(Boolean(enrichment.summaryExcerpt));
const hasTags = $derived(enrichment.tags.length > 0);
const hasRelated = $derived(enrichment.relatedArticles.length > 0);
const hasAcolyte = $derived(Boolean(enrichment.acolyteHref));
</script>

<article class="deck-card" data-role="article-deck-card">
	<header class="deck-head">
		{#if enrichment.feedTitle}
			<span class="deck-feed">{enrichment.feedTitle}</span>
		{/if}
		{#if altHref}
			<a class="deck-title" href={altHref} data-role="deck-title-link">
				{title}
			</a>
		{:else}
			<span class="deck-title deck-title--disabled">{title}</span>
		{/if}
	</header>

	{#if hasExcerpt}
		<p class="deck-excerpt">{enrichment.summaryExcerpt}</p>
	{/if}

	{#if hasTags}
		<div class="deck-tags" data-role="deck-tags">
			{#each enrichment.tags.slice(0, 4) as tag (tag)}
				<span class="deck-tag">#{tag}</span>
			{/each}
			{#if enrichment.tags.length > 4}
				<span class="deck-tag deck-tag--more">+{enrichment.tags.length - 4}</span>
			{/if}
		</div>
	{/if}

	{#if hasRelated}
		<div class="deck-related" data-role="deck-related">
			<span class="deck-related-label">Threads nearby</span>
			<ul>
				{#each enrichment.relatedArticles as rel (rel.articleId)}
					<li>
						<a href={rel.articleAltHref}>{rel.title}</a>
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	<footer class="deck-actions">
		{#if hasAcolyte}
			<a
				class="deck-cta deck-cta--primary"
				href={enrichment.acolyteHref}
				data-role="deck-acolyte-cta"
			>
				Chat with Acolyte →
			</a>
		{/if}
		{#if enrichment.articleUrl}
			<a
				class="deck-cta deck-cta--ghost"
				href={enrichment.articleUrl}
				target="_blank"
				rel="noopener"
			>
				Original ↗
			</a>
		{/if}
	</footer>
</article>

<style>
	.deck-card {
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		padding: 1rem 1.1rem 0.9rem;
		background: var(--alt-paper, #f4f1ea);
		border-top: 1px solid var(--alt-ink, #1a1a1a);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.deck-card + .deck-card {
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	.deck-head {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.deck-feed {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	.deck-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.35;
		color: var(--alt-ink, #1a1a1a);
		text-decoration: none;
	}

	.deck-title:hover {
		color: var(--alt-vermillion, #a83232);
		text-decoration: underline;
		text-underline-offset: 3px;
	}

	.deck-title--disabled {
		color: var(--alt-slate, #666);
	}

	.deck-excerpt {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		line-height: 1.55;
		color: var(--alt-slate, #555);
		margin: 0;
	}

	.deck-tags {
		display: flex;
		flex-wrap: wrap;
		gap: 0.35rem;
	}

	.deck-tag {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.68rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		color: var(--alt-ink, #1a1a1a);
		background: transparent;
		border: 1px solid var(--alt-ink, #1a1a1a);
		padding: 0.1rem 0.45rem;
	}

	.deck-tag--more {
		border-style: dashed;
		color: var(--alt-ash, #999);
		border-color: var(--alt-ash, #c8c8c8);
	}

	.deck-related {
		border-left: 2px solid var(--alt-ink, #1a1a1a);
		padding: 0.15rem 0 0.15rem 0.6rem;
	}

	.deck-related-label {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.62rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.2rem;
	}

	.deck-related ul {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.deck-related li a {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		line-height: 1.4;
		color: var(--alt-ink, #1a1a1a);
		text-decoration: none;
	}

	.deck-related li a:hover {
		color: var(--alt-vermillion, #a83232);
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.deck-actions {
		display: flex;
		flex-wrap: wrap;
		gap: 0.6rem;
		margin-top: 0.2rem;
	}

	.deck-cta {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.72rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		text-decoration: none;
		padding: 0.35rem 0.75rem;
		border: 1px solid var(--alt-ink, #1a1a1a);
	}

	.deck-cta--primary {
		background: var(--alt-ink, #1a1a1a);
		color: var(--alt-paper, #f4f1ea);
	}

	.deck-cta--primary:hover {
		background: var(--alt-vermillion, #a83232);
		border-color: var(--alt-vermillion, #a83232);
	}

	.deck-cta--ghost {
		background: transparent;
		color: var(--alt-ink, #1a1a1a);
	}

	.deck-cta--ghost:hover {
		background: var(--alt-ink, #1a1a1a);
		color: var(--alt-paper, #f4f1ea);
	}
</style>
