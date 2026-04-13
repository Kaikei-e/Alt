<script lang="ts">
import type { MorningLetterBulletEnrichment } from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";
import ArticleDeckCard from "./ArticleDeckCard.svelte";

type Props = {
	enrichments: MorningLetterBulletEnrichment[];
	loading?: boolean;
};

let { enrichments, loading = false }: Props = $props();

const hasAny = $derived(enrichments.length > 0);
</script>

{#if hasAny || loading}
	<section class="deck" data-role="article-deck">
		<header class="deck-section-head">
			<span class="deck-section-kicker">Threads to pull</span>
			<h3 class="deck-section-title">Develop from here</h3>
			<p class="deck-section-hint">
				Each card links into the article, the tags it touches, and nearby
				threads from your Alt. Open a chat with Augur to dig further.
			</p>
		</header>

		{#if loading && !hasAny}
			<p class="deck-loading">Gathering enrichment…</p>
		{:else}
			<div class="deck-grid">
				{#each enrichments as e (`${e.sectionKey}:${e.articleId}`)}
					<ArticleDeckCard enrichment={e} />
				{/each}
			</div>
		{/if}
	</section>
{/if}

<style>
	.deck {
		margin-top: 2.25rem;
		padding-top: 1.5rem;
		border-top: 3px double var(--alt-ink, #1a1a1a);
	}

	.deck-section-head {
		margin-bottom: 1.25rem;
	}

	.deck-section-kicker {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
	}

	.deck-section-title {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.3rem;
		font-weight: 700;
		margin: 0.2rem 0 0.35rem;
		color: var(--alt-ink, #1a1a1a);
	}

	.deck-section-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		line-height: 1.55;
		color: var(--alt-slate, #555);
		margin: 0;
		max-width: 42rem;
	}

	.deck-grid {
		display: flex;
		flex-direction: column;
	}

	.deck-loading {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}
</style>
