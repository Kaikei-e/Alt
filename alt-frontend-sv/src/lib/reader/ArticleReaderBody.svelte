<script lang="ts">
import { Loader2 } from "@lucide/svelte";
import type { Snippet } from "svelte";
import ArticleEndBranches from "#lib/components/knowledge-trail/ArticleEndBranches.svelte";
import RenderFeedDetails from "#lib/components/mobile/RenderFeedDetails.svelte";
import PageKicker from "#lib/components/recap/job-status/PageKicker.svelte";
import { Button } from "#lib/components/ui/button/index.js";
import {
	READ_ORIGINAL_LABEL,
	TRY_AGAIN_LABEL,
} from "#lib/utils/articleContentState.js";
import ArticleSummaryBlock from "./ArticleSummaryBlock.svelte";
import type { ArticleReader } from "./createArticleReader.svelte.js";

interface Props {
	reader: ArticleReader;
	showInlineSummary?: boolean;
	fallbackDescription?: string | null;
	emptyAction?: Snippet;
	connections?: Snippet<[{ articleId: string | null }]>;
}

const {
	reader,
	showInlineSummary = true,
	fallbackDescription = null,
	emptyAction,
	connections,
}: Props = $props();
</script>

<PageKicker
	testId="article-masthead"
	kicker={reader.mastheadKicker}
	title={reader.mastheadTitle}
/>

{#if reader.queueError}
	<p class="queue-error" role="alert" data-testid="reader-queue-error">
		{reader.queueError}
	</p>
{/if}

{#if showInlineSummary}
	<ArticleSummaryBlock {reader} />
{/if}

{#if !reader.articleUrl}
	<div class="placeholder">
		<p>No article URL provided. Unable to load content.</p>
		{#if emptyAction}
			{@render emptyAction()}
		{/if}
	</div>
{:else if reader.isFetching}
	<div
		class="placeholder placeholder--center"
		data-testid="article-content-pending"
	>
		<Loader2 class="h-5 w-5 animate-spin" />
		<span>{reader.pendingLabel}</span>
	</div>
	{#if fallbackDescription}
		<p class="fallback-description" data-testid="article-fallback-summary">
			{fallbackDescription}
		</p>
	{/if}
{:else if reader.contentPhase === "failed"}
	<div class="placeholder" data-testid="article-content-failed" role="alert">
		<p class="placeholder__error">{reader.contentError}</p>
		<div class="placeholder-remedies">
			<Button
				variant="outline"
				onclick={reader.handleFetch}
				data-testid="retry-content"
				class="placeholder-cta mt-1"
			>
				{TRY_AGAIN_LABEL}
			</Button>
			<a
				href={reader.articleUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="placeholder-link"
				data-testid="read-original-link"
			>
				{READ_ORIGINAL_LABEL}
			</a>
		</div>
	</div>
	{#if fallbackDescription}
		<p class="fallback-description" data-testid="article-fallback-summary">
			{fallbackDescription}
		</p>
	{/if}
{:else if reader.articleContent}
	<div
		class="article-surface"
		data-testid="article-content-surface"
		bind:this={reader.articleBodyEl}
	>
		<RenderFeedDetails
			feedDetails={{
				content: reader.articleContent,
				article_id: reader.fetchedArticleId ?? "",
				og_image_url: "",
				og_image_proxy_url: "",
			}}
			error={null}
		/>
		<div
			class="reading-end-sentinel"
			bind:this={reader.endSentinelEl}
			aria-hidden="true"
		></div>
	</div>
	<ArticleEndBranches
		branches={reader.endBranches.branches}
		onResolve={reader.endBranches.resolve}
	/>
	{#if connections}
		{@render connections({ articleId: reader.effectiveArticleId })}
	{/if}
{:else}
	<div class="placeholder">
		<p>No content available.</p>
	</div>
{/if}

<style>
	.article-surface {
		border-top: 1px solid var(--surface-border, #c8c8c8);
		padding-top: 0.5rem;
		min-width: 0;
	}

	.reading-end-sentinel {
		height: 1px;
		margin-top: -1px;
		pointer-events: none;
	}

	.placeholder {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.75rem;
		padding: 3rem 1rem;
		color: var(--alt-slate, #666666);
		text-align: center;
	}

	.placeholder--center {
		flex-direction: row;
	}

	.placeholder__error {
		color: var(--alt-error, #8c1d1d);
		margin: 0;
	}

	.placeholder-remedies {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: 1rem;
		flex-wrap: wrap;
	}

	.placeholder-link {
		font-size: 0.8125rem;
		font-weight: 600;
		color: var(--alt-primary, #2f4f4f);
		text-decoration: underline;
		text-underline-offset: 0.2em;
		min-height: 44px;
		display: inline-flex;
		align-items: center;
	}

	.fallback-description {
		margin: 0;
		font-family: var(--font-body);
		font-size: 0.95rem;
		line-height: 1.7;
		color: var(--alt-charcoal, #1a1a1a);
	}

	.queue-error {
		margin: 0;
		padding: 0.5rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-error, #8c1d1d);
		background: var(--surface-2, #f5f4f1);
		border-left: 3px solid var(--alt-error, #8c1d1d);
	}
</style>
