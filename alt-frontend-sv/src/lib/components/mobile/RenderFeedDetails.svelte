<script lang="ts">
import type {
	FeedContentOnTheFlyResponse,
	FetchArticleSummaryResponse,
} from "$lib/api/client";
import { sanitizeHtml } from "$lib/utils/sanitizeHtml";

interface Props {
	feedDetails?:
		| FetchArticleSummaryResponse
		| FeedContentOnTheFlyResponse
		| null;
	isLoading?: boolean;
	error?: string | null;
}

const { feedDetails, isLoading = false, error = null }: Props = $props();

// Sanitize content for safe rendering with @html
const safeArticleContent = $derived(
	feedDetails &&
		"matched_articles" in feedDetails &&
		feedDetails.matched_articles?.[0]?.content
		? sanitizeHtml(feedDetails.matched_articles[0].content)
		: "",
);

const safeOnTheFlyContent = $derived(
	feedDetails && "content" in feedDetails && feedDetails.content
		? sanitizeHtml(feedDetails.content)
		: "",
);
</script>

{#if isLoading}
	<p class="text-center py-8 italic" style="color: var(--alt-text-secondary);">
		Loading summary...
	</p>
{:else if error}
	<p class="text-center py-8 italic" style="color: var(--alt-text-secondary);">
		{error}
	</p>
{:else if !feedDetails}
	<p class="text-center py-8 italic" style="color: var(--alt-text-secondary);">
		Unable to load article content
	</p>
{:else if "matched_articles" in feedDetails && feedDetails.matched_articles?.length > 0}
	<!-- FetchArticleSummaryResponse - Rich article display -->
	{@const article = feedDetails.matched_articles[0]}
	<div class="px-4 py-4">
		<!-- Article Metadata -->
		<div class="mb-4 p-4 rounded-lg border-2 border-surface-border bg-white shadow-sm">
			<h2 class="text-xl font-bold mb-3 leading-tight text-text-primary">
				{article.title}
			</h2>

			<div class="flex items-center gap-3 text-sm text-text-secondary">
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
				background: #f5f5f5;
				border-color: rgba(0, 0, 0, 0.1);
				color: #1a1a1a;
			"
		>
			<div
				class="article-content max-h-[50vh] overflow-auto"
				style="color: #1a1a1a;"
			>
				{@html safeArticleContent}
			</div>
		</div>
	</div>
{:else if "content" in feedDetails && feedDetails.content}
	<!-- FeedContentOnTheFlyResponse - Rich content display with sanitization -->
	<div class="px-4 py-4">
		<div
			class="article-content text-base leading-relaxed break-words"
			style="color: var(--alt-text-primary);"
		>
			{@html safeOnTheFlyContent}
		</div>
	</div>
{:else}
	<p class="text-center py-8 italic" style="color: var(--alt-text-secondary);">
		Article content is not available
	</p>
{/if}

<style>
	:global(.article-content) {
		word-break: break-word;
		overflow-wrap: anywhere;
		font-size: clamp(0.95rem, 2.5vw, 1.1rem);
		line-height: 1.75;
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
	:global(.article-content h6),
	:global(.article-content p),
	:global(.article-content li) {
		color: #1a1a1a;
	}

	:global(.article-content h1) {
		font-size: clamp(1.5rem, 4vw, 2rem);
		margin-top: 1.5em;
		margin-bottom: 0.75em;
		font-weight: bold;
	}

	:global(.article-content h2) {
		font-size: clamp(1.3rem, 3.5vw, 1.7rem);
		margin-top: 1.5em;
		margin-bottom: 0.5em;
		font-weight: bold;
	}

	:global(.article-content h3) {
		font-size: clamp(1.1rem, 3vw, 1.4rem);
		margin-top: 1.5em;
		margin-bottom: 0.5em;
		font-weight: bold;
	}

	:global(.article-content h4) {
		font-size: clamp(1rem, 2.5vw, 1.2rem);
		margin-top: 1.25em;
		margin-bottom: 0.5em;
		font-weight: bold;
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
		color: #2563eb; /* Blue for links on light bg */
		text-decoration: underline;
	}

	:global(.article-content a:hover) {
		color: #1d4ed8;
	}

	/* NOTE: img styles removed - images are stripped for security (XSS via onerror/onload) */

	:global(.article-content blockquote) {
		border-left: 3px solid #2563eb;
		padding-left: 1em;
		margin-left: 0;
		font-style: italic;
		background: rgba(0, 0, 0, 0.05);
		padding: 1em;
		border-radius: 0 8px 8px 0;
		color: #4b5563;
	}

	:global(.article-content pre) {
		background: #1e1e1e; /* Keep code blocks dark */
		color: #e5e5e5;
		padding: 1em;
		border-radius: 8px;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		font-size: clamp(0.8rem, 2vw, 0.9rem);
	}

	:global(.article-content code) {
		background: rgba(0, 0, 0, 0.1);
		color: #1a1a1a;
		padding: 0.2em 0.4em;
		border-radius: 3px;
		font-size: 0.9em;
		font-family: monospace;
	}

	/* Override code inside pre to be light on dark */
	:global(.article-content pre code) {
		background: transparent;
		color: inherit;
	}

	:global(.article-content table) {
		border-collapse: collapse;
		width: 100%;
		margin-top: 1em;
		margin-bottom: 1em;
	}

	:global(.article-content th),
	:global(.article-content td) {
		border: 1px solid rgba(0, 0, 0, 0.2);
		padding: 0.5em;
		text-align: left;
		color: #1a1a1a;
	}

	:global(.article-content th) {
		background: rgba(0, 0, 0, 0.05);
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
		background: rgba(0, 0, 0, 0.2);
		border-radius: 2px;
	}

	:global(.article-content::-webkit-scrollbar-thumb:hover) {
		background: rgba(0, 0, 0, 0.3);
	}

	/* Mobile optimization */
	@media (max-width: 640px) {
		:global(.article-content) {
			font-size: 1rem;
			line-height: 1.8;
		}

		:global(.article-content pre) {
			font-size: 0.85rem;
			padding: 0.75em;
		}
	}
</style>
