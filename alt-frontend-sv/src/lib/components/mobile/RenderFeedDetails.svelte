<script lang="ts">
import type {
	FetchArticleSummaryResponse,
	FeedContentOnTheFlyResponse,
} from "$lib/api/client";

interface Props {
	feedDetails?: FetchArticleSummaryResponse | FeedContentOnTheFlyResponse | null;
	isLoading?: boolean;
	error?: string | null;
}

const { feedDetails, isLoading = false, error = null }: Props = $props();
</script>

{#if isLoading}
	<p
		class="text-center py-8 italic"
		style="color: var(--text-secondary);"
	>
		Loading summary...
	</p>
{:else if error}
	<p
		class="text-center py-8 italic"
		style="color: var(--text-secondary);"
	>
		{error}
	</p>
{:else if !feedDetails}
	<p
		class="text-center py-8 italic"
		style="color: var(--text-secondary);"
	>
		No summary available for this article
	</p>
{:else if "matched_articles" in feedDetails && feedDetails.matched_articles?.length > 0}
	<!-- FetchArticleSummaryResponse - Rich article display -->
	{@const article = feedDetails.matched_articles[0]}
	<div class="px-4 py-4">
		<!-- Article Metadata -->
		<div
			class="mb-4 p-4 rounded-xl border"
			style="
				background: rgba(255, 255, 255, 0.05);
				border-color: rgba(255, 255, 255, 0.1);
			"
		>
			<h2
				class="text-xl font-bold mb-2 leading-tight"
				style="color: var(--text-primary);"
			>
				{article.title}
			</h2>

			<div class="flex items-center gap-3 text-base" style="color: var(--text-secondary);">
				{#if article.author}
					<p>By {article.author}</p>
				{/if}

				<p>
					{new Date(article.published_at).toLocaleDateString("ja-JP", {
						year: "numeric",
						month: "short",
						day: "numeric",
					})}
				</p>

				<span
					class="px-2 py-1 rounded-full text-xs font-semibold"
					style="background: var(--accent-primary);"
				>
					{article.content_type}
				</span>
			</div>
		</div>

		<!-- Content -->
		<div
			class="rounded-lg p-3 border"
			style="
				background: rgba(255, 255, 255, 0.02);
				border-color: rgba(255, 255, 255, 0.05);
			"
		>
			<div
				class="article-content max-h-[50vh] overflow-auto"
				style="color: var(--text-primary); line-height: 1.7;"
			>
				{@html article.content}
			</div>
		</div>
	</div>
{:else if "content" in feedDetails && feedDetails.content}
	<!-- FeedContentOnTheFlyResponse - Simple content display -->
	<div class="px-4 py-4">
		<div
			class="text-base leading-relaxed break-words"
			style="color: var(--text-primary);"
		>
			{@html feedDetails.content}
		</div>
	</div>
{:else}
	<p
		class="text-center py-8 italic"
		style="color: var(--text-secondary);"
	>
		No summary available for this article
	</p>
{/if}

<style>
	:global(.article-content) {
		word-break: break-word;
		overflow-wrap: anywhere;
	}

	:global(.article-content p) {
		margin-bottom: 1em;
		line-height: 1.7;
	}

	:global(.article-content h1),
	:global(.article-content h2),
	:global(.article-content h3),
	:global(.article-content h4),
	:global(.article-content h5),
	:global(.article-content h6) {
		margin-top: 1.5em;
		margin-bottom: 0.5em;
		font-weight: bold;
		color: var(--text-primary);
	}

	:global(.article-content h1) {
		font-size: 1.5em;
	}

	:global(.article-content h2) {
		font-size: 1.3em;
	}

	:global(.article-content h3) {
		font-size: 1.1em;
	}

	:global(.article-content ul),
	:global(.article-content ol) {
		margin-left: 1.5em;
		margin-bottom: 1em;
	}

	:global(.article-content li) {
		margin-bottom: 0.3em;
	}

	:global(.article-content a) {
		color: var(--accent-primary);
		text-decoration: underline;
	}

	:global(.article-content a:hover) {
		color: var(--accent-secondary);
	}

	:global(.article-content img) {
		max-width: 100%;
		height: auto;
		border-radius: 8px;
		margin-top: 1em;
		margin-bottom: 1em;
	}

	:global(.article-content blockquote) {
		border-left: 3px solid var(--accent-primary);
		padding-left: 1em;
		margin-left: 0;
		font-style: italic;
		background: rgba(255, 255, 255, 0.05);
		padding: 1em;
		border-radius: 0 8px 8px 0;
	}

	:global(.article-content pre) {
		background: rgba(0, 0, 0, 0.3);
		padding: 1em;
		border-radius: 8px;
		overflow: auto;
		font-size: 0.9em;
	}

	:global(.article-content code) {
		background: rgba(0, 0, 0, 0.2);
		padding: 0.2em 0.4em;
		border-radius: 3px;
		font-size: 0.9em;
		font-family: monospace;
	}

	:global(.article-content table) {
		border-collapse: collapse;
		width: 100%;
		margin-top: 1em;
		margin-bottom: 1em;
	}

	:global(.article-content th),
	:global(.article-content td) {
		border: 1px solid rgba(255, 255, 255, 0.2);
		padding: 0.5em;
		text-align: left;
	}

	:global(.article-content th) {
		background: rgba(255, 255, 255, 0.1);
		font-weight: bold;
	}

	:global(.article-content::-webkit-scrollbar) {
		width: 4px;
	}

	:global(.article-content::-webkit-scrollbar-track) {
		background: transparent;
		border-radius: 2px;
	}

	:global(.article-content::-webkit-scrollbar-thumb) {
		background: rgba(255, 255, 255, 0.2);
		border-radius: 2px;
	}

	:global(.article-content::-webkit-scrollbar-thumb:hover) {
		background: rgba(255, 255, 255, 0.3);
	}
</style>

