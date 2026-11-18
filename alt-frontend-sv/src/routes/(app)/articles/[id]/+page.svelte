<script lang="ts">
import { ArrowLeft } from "@lucide/svelte";
import { onMount } from "svelte";
import { MediaQuery } from "svelte/reactivity";
import { Button } from "#lib/components/ui/button/index.js";
import ArticleEditorialRail from "#lib/reader/ArticleEditorialRail.svelte";
import ArticleReaderActions from "#lib/reader/ArticleReaderActions.svelte";
import ArticleReaderBody from "#lib/reader/ArticleReaderBody.svelte";
import { createArticleReader } from "#lib/reader/createArticleReader.svelte.js";
import {
	ReadingSurface,
	recordSurfaceViewed,
} from "#lib/reading/instrumentation.js";
import { beforeNavigate, goto } from "$app/navigation";
import { page } from "$app/state";
import ArticleVisit from "./ArticleVisit.svelte";

const articleId = $derived(page.params.id);
const paramUrl = $derived(page.url.searchParams.get("url"));
const paramTitle = $derived(page.url.searchParams.get("title"));

const reader = createArticleReader({
	getArticleId: () => articleId,
	getUrl: () => paramUrl,
	getTitle: () => paramTitle,
	getSurface: () => ReadingSurface.ARTICLE_PAGE,
	getAutoSummarize: () => page.url.searchParams.get("summarize") === "true",
	getUsePrefetchCache: () => false,
});

const railQuery = new MediaQuery("(min-width: 1024px)", false);
const hasRail = $derived(railQuery.current);

onMount(() => {
	recordSurfaceViewed(ReadingSurface.ARTICLE_PAGE);
});

beforeNavigate(({ shallow }) => {
	if (!shallow) {
		reader.endActiveSession();
	}
});
</script>

<svelte:head>
	<title>Article — Alt</title>
</svelte:head>

<div class="article-page">
	<header class="action-bar" aria-label="Article actions">
		<Button
			variant="ghost"
			size="icon"
			aria-label="Back to Home"
			onclick={() => goto("/home")}
			class="action-btn action-btn--back"
		>
			<ArrowLeft class="h-4 w-4" />
			<span class="action-label">Back</span>
		</Button>

		{#if !hasRail}
			<ArticleReaderActions {reader} />
		{/if}
	</header>

	{#key articleId}
		{#if articleId}
			<ArticleVisit {articleId} />
		{/if}
		<div class="article-layout">
			<article class="article-body">
				<ArticleReaderBody {reader} showInlineSummary={!hasRail}>
					{#snippet emptyAction()}
						<Button
							variant="outline"
							onclick={() => goto("/home")}
							class="placeholder-cta mt-1"
						>
							Return to Home
						</Button>
					{/snippet}
				</ArticleReaderBody>
			</article>

			{#if hasRail}
				<ArticleEditorialRail {reader} />
			{/if}
		</div>
	{/key}
</div>

<style>
	.article-page {
		max-width: 720px;
		margin: 0 auto;
		padding: 0 1rem 2rem;
		color: var(--alt-charcoal, #1a1a1a);
		font-family: var(--font-body);
	}

	.action-bar {
		position: sticky;
		top: 0;
		z-index: 20;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		min-height: 56px;
		padding: calc(env(safe-area-inset-top, 0px) + 0.5rem) 0 0.5rem;
		background: var(--surface-bg, #faf9f7);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.article-layout {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.article-body {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		padding-top: 1.25rem;
		min-width: 0;
	}

	@media (min-width: 640px) {
		.article-page {
			padding: 0 1.5rem 3rem;
		}
	}

	@media (min-width: 768px) {
		.article-page {
			max-width: 820px;
			padding: 0 2rem 3rem;
		}
	}

	/* Editorial rail — desktop progressive enhancement over ADR-000715 */
	@media (min-width: 1024px) {
		.article-page {
			max-width: min(100%, 96rem);
			padding: 0 2rem 3rem;
		}

		.article-layout {
			display: grid;
			grid-template-columns: minmax(0, 1fr) 22rem;
			column-gap: 2rem;
			align-items: start;
		}

		.article-body {
			margin-inline: auto;
			width: 100%;
			max-width: 72ch;
		}
	}
</style>
