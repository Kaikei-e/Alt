<script lang="ts">
import ArticleReaderActions from "./ArticleReaderActions.svelte";
import ArticleSummaryBlock from "./ArticleSummaryBlock.svelte";
import type { ArticleReader } from "./createArticleReader.svelte.js";

interface Props {
	reader: ArticleReader;
	excerpt?: string | null;
}

const { reader, excerpt = null }: Props = $props();

function normalizeComparableText(text: string | null | undefined): string {
	return (text ?? "").trim().replace(/\s+/g, " ");
}

const visibleExcerpt = $derived.by(() => {
	const trimmed = excerpt?.trim() ?? "";
	if (!trimmed) return null;
	const summary = normalizeComparableText(reader.summarizer.summary);
	if (summary && normalizeComparableText(trimmed) === summary) {
		return null;
	}
	return trimmed;
});
</script>

<aside
	class="article-rail"
	data-testid="article-rail"
	aria-label="Article details"
>
	<section class="rail-section">
		<h2 class="rail-kicker">Source</h2>
		<p class="rail-value">{reader.sourceHost || "—"}</p>
	</section>

	<section class="rail-section">
		<h2 class="rail-kicker">Reading time</h2>
		<p class="rail-value">
			{reader.readingTimeMinutes ? `${reader.readingTimeMinutes} min` : "—"}
		</p>
	</section>

	{#if visibleExcerpt}
		<section class="rail-section">
			<h2 class="rail-kicker">Excerpt</h2>
			<p class="rail-value">{visibleExcerpt}</p>
		</section>
	{/if}

	<section class="rail-section rail-section--actions">
		<h2 class="rail-kicker">Actions</h2>
		<ArticleReaderActions {reader} />
	</section>

	<ArticleSummaryBlock {reader} />
</aside>

<style>
	.article-rail {
		position: sticky;
		top: var(--rail-sticky-top, calc(env(safe-area-inset-top, 0px) + 1rem));
		align-self: start;
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		padding: 1.1rem 1.1rem 1.25rem;
		background: var(--surface-2, #f5f4f1);
		border: 1px solid var(--surface-border, #c8c8c8);
		border-top: 3px solid var(--alt-primary, #2f4f4f);
		max-height: var(--rail-max-height, calc(100vh - 2rem));
		overflow-y: auto;
	}

	.rail-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.rail-kicker {
		margin: 0;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.14em;
		text-transform: uppercase;
		color: var(--alt-ash, #999999);
	}

	.rail-value {
		margin: 0;
		font-family: var(--font-body);
		font-size: 0.95rem;
		color: var(--alt-charcoal, #1a1a1a);
		word-break: break-word;
	}

	.rail-section--actions :global(.actions-cluster) {
		flex-direction: column;
		align-items: stretch;
		gap: 0.5rem;
		margin-left: 0;
	}

	.rail-section--actions :global(.action-btn) {
		width: 100%;
		justify-content: flex-start;
	}

	.rail-section--actions .rail-kicker + :global(.actions-cluster) {
		margin-top: 0.1rem;
	}

	.rail-section--actions :global(.actions-cluster) > :global(*) {
		width: 100%;
	}

	.rail-section--actions :global(.action-link) {
		width: 100%;
		justify-content: flex-start;
	}
</style>
