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
				class="article-content max-h-[50dvh] overflow-auto"
				style="color: #1a1a1a;"
			>
				{@html safeArticleContent}
			</div>
		</div>
	</div>
{:else if "content" in feedDetails && feedDetails.content}
	<!-- FeedContentOnTheFlyResponse - Rich content display with sanitization -->
	<div class="article-content break-words">
		{@html safeOnTheFlyContent}
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
		font-family: var(--font-body);
		font-size: clamp(0.95rem, 2.5vw, 1.05rem);
		line-height: 1.75;
		color: var(--alt-charcoal, #1a1a1a);
		max-width: 65ch;
	}

	:global(.article-content p) {
		margin: 0 0 1em;
		line-height: 1.7;
	}

	:global(.article-content h1),
	:global(.article-content h2),
	:global(.article-content h3),
	:global(.article-content h4),
	:global(.article-content h5),
	:global(.article-content h6) {
		font-family: var(--font-display);
		color: var(--alt-charcoal, #1a1a1a);
		letter-spacing: -0.01em;
		line-height: 1.2;
	}

	:global(.article-content h1) {
		font-size: clamp(1.5rem, 4vw, 1.9rem);
		margin-top: 1.5em;
		margin-bottom: 0.6em;
		font-weight: 700;
	}

	:global(.article-content h2) {
		font-size: clamp(1.3rem, 3.5vw, 1.55rem);
		margin-top: 1.4em;
		margin-bottom: 0.5em;
		font-weight: 700;
	}

	:global(.article-content h3) {
		font-size: clamp(1.1rem, 3vw, 1.3rem);
		margin-top: 1.3em;
		margin-bottom: 0.4em;
		font-weight: 700;
	}

	:global(.article-content h4) {
		font-size: clamp(1rem, 2.5vw, 1.15rem);
		margin-top: 1.2em;
		margin-bottom: 0.4em;
		font-weight: 700;
	}

	:global(.article-content ul),
	:global(.article-content ol) {
		margin: 0 0 1em 1.4em;
	}

	:global(.article-content li) {
		margin-bottom: 0.3em;
		color: var(--alt-charcoal, #1a1a1a);
	}

	:global(.article-content a) {
		color: var(--alt-primary, #2f4f4f);
		text-decoration: underline;
		text-underline-offset: 0.15em;
	}

	:global(.article-content a:hover),
	:global(.article-content a:focus-visible) {
		color: var(--interactive-text-hover, #223b3b);
	}

	/* NOTE: img styles removed - images are stripped for security (XSS via onerror/onload) */

	:global(.article-content blockquote) {
		border-left: 3px solid var(--alt-primary, #2f4f4f);
		padding: 0.75em 1em;
		margin: 1em 0;
		font-style: italic;
		background: var(--surface-2, #f5f4f1);
		color: var(--alt-slate, #666666);
	}

	:global(.article-content pre) {
		background: var(--surface-2, #f5f4f1);
		color: var(--alt-charcoal, #1a1a1a);
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0.9em 1em;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		font-family: var(--font-mono);
		font-size: clamp(0.8rem, 2vw, 0.9rem);
		line-height: 1.55;
	}

	:global(.article-content code) {
		background: var(--surface-2, #f5f4f1);
		color: var(--alt-charcoal, #1a1a1a);
		padding: 0.15em 0.35em;
		font-family: var(--font-mono);
		font-size: 0.9em;
	}

	/* Code inside pre inherits pre styling */
	:global(.article-content pre code) {
		background: transparent;
		padding: 0;
		color: inherit;
	}

	:global(.article-content table) {
		border-collapse: collapse;
		width: 100%;
		margin: 1em 0;
		font-size: 0.9em;
	}

	:global(.article-content th),
	:global(.article-content td) {
		border: 1px solid var(--surface-border, #c8c8c8);
		padding: 0.5em 0.75em;
		text-align: left;
		color: var(--alt-charcoal, #1a1a1a);
	}

	:global(.article-content th) {
		background: var(--surface-2, #f5f4f1);
		font-weight: 600;
	}

	:global(.article-content::-webkit-scrollbar) {
		width: 4px;
	}

	:global(.article-content::-webkit-scrollbar-track) {
		background: transparent;
	}

	:global(.article-content::-webkit-scrollbar-thumb) {
		background: var(--surface-border, #c8c8c8);
	}

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
