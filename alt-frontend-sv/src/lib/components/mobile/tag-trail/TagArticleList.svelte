<script lang="ts">
import type { TagTrailArticle, TagTrailTag } from "$lib/schema/tagTrail";
import TagChipList from "./TagChipList.svelte";

interface Props {
	articles: TagTrailArticle[];
	isLoading: boolean;
	hasMore: boolean;
	selectedTagName: string;
	onTagClick: (tag: TagTrailTag) => void;
	onLoadMore: () => void;
	getArticleTags: (articleId: string) => TagTrailTag[];
	loadingArticleTags: Set<string>;
}

const {
	articles,
	isLoading,
	hasMore,
	selectedTagName,
	onTagClick,
	onLoadMore,
	getArticleTags,
	loadingArticleTags,
}: Props = $props();

const formatDate = (dateStr: string) => {
	const date = new Date(dateStr);
	return date.toLocaleDateString(undefined, {
		month: "short",
		day: "numeric",
	});
};
</script>

<div class="flex-1 overflow-y-auto px-4 pb-4" data-testid="tag-article-list">
	<div class="article-header">
		<span class="section-label">Cross-Referenced Stories</span>
		<h2 class="tag-heading">{selectedTagName}</h2>
	</div>

	<div class="flex flex-col">
		{#each articles as article, i (article.id)}
			<div
				class="article-row"
				style="--stagger: {i};"
				data-testid="tag-article-{article.id}"
			>
				<a
					href={article.link}
					target="_blank"
					rel="noopener noreferrer"
					class="article-title"
				>
					{article.title}
				</a>
				<div class="article-meta">
					{#if article.feedTitle}
						<span>{article.feedTitle}</span>
						<span class="meta-dot">&middot;</span>
					{/if}
					<span>{formatDate(article.publishedAt)}</span>
				</div>
				<!-- Article tags for hopping -->
				{#if loadingArticleTags.has(article.id)}
					<div class="flex gap-2 mt-1.5">
						{#each [1, 2] as _i}
							<div class="skeleton-tag"></div>
						{/each}
					</div>
				{:else if getArticleTags(article.id).length > 0}
					<div class="mt-1.5 -mx-3">
						<TagChipList
							tags={getArticleTags(article.id)}
							onTagClick={onTagClick}
						/>
					</div>
				{/if}
			</div>
		{/each}

		{#if isLoading}
			<div class="flex items-center justify-center gap-3 py-8">
				<div class="loading-pulse"></div>
				<span class="loading-text">Loading articles&hellip;</span>
			</div>
		{/if}

		{#if !isLoading && hasMore}
			<button
				type="button"
				class="load-more-btn"
				onclick={onLoadMore}
			>
				Load More
			</button>
		{/if}

		{#if !isLoading && articles.length === 0}
			<div class="empty-state">
				<p class="empty-text">No articles found with this tag</p>
			</div>
		{/if}
	</div>
</div>

<style>
	.article-header {
		margin-bottom: 0.75rem;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.section-label {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-ash, #999);
		margin-bottom: 0.2rem;
	}

	.tag-heading {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0;
	}

	.article-row {
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--surface-border, #c8c8c8);

		opacity: 0;
		animation: row-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}
	@keyframes row-in {
		to { opacity: 1; }
	}

	.article-title {
		display: block;
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1rem;
		font-weight: 700;
		line-height: 1.35;
		color: var(--alt-charcoal, #1a1a1a);
		text-decoration: none;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}
	.article-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.article-meta {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		margin-top: 0.25rem;
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}

	.meta-dot {
		color: var(--surface-border, #c8c8c8);
	}

	.skeleton-tag {
		height: 28px;
		width: 60px;
		background: var(--muted);
		animation: shimmer 1.5s ease-in-out infinite;
	}
	@keyframes shimmer {
		0%, 100% { opacity: 0.5; }
		50% { opacity: 1; }
	}

	.load-more-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 100%;
		min-height: 44px;
		margin-top: 0.75rem;
		padding: 0.5rem 1rem;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;

		color: var(--alt-charcoal, #1a1a1a);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal, #1a1a1a);
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}
	.load-more-btn:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
	.loading-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	.empty-state {
		text-align: center;
		padding: 2rem 0;
	}
	.empty-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash, #999);
		margin: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.article-row { animation: none; opacity: 1; }
		.skeleton-tag { animation: none; opacity: 0.5; }
		.loading-pulse { animation: none; opacity: 0.6; }
	}
</style>
