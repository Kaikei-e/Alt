<script lang="ts">
import {
	Bookmark,
	BookmarkCheck,
	Download,
	ExternalLink,
	Loader2,
	RefreshCw,
	Sparkles,
} from "@lucide/svelte";
import { Button } from "#lib/components/ui/button/index.js";
import type { ArticleReader } from "./createArticleReader.svelte.js";

interface Props {
	reader: ArticleReader;
}

const { reader }: Props = $props();
</script>

<div class="actions-cluster">
	<Button
		data-testid="read-today-toggle-button"
		aria-label={reader.isQueued ? "In Read today" : "Read today"}
		aria-pressed={reader.isQueued}
		onclick={() => void reader.toggleReadToday()}
		disabled={!reader.canToggleQueue}
		variant={reader.isQueued ? "default" : "outline"}
		size="icon"
		class={reader.isQueued
			? "action-btn action-btn--primary"
			: "action-btn"}
	>
		{#if reader.isQueued}
			<BookmarkCheck class="h-4 w-4" />
			<span class="action-label">In Read today</span>
		{:else}
			<Bookmark class="h-4 w-4" />
			<span class="action-label">Read today</span>
		{/if}
	</Button>

	<Button
		data-testid="fetch-button"
		aria-label={reader.fetchAriaLabel}
		onclick={reader.handleFetch}
		disabled={reader.fetchButtonState === "loading"}
		variant={reader.fetchButtonState === "error" ? "destructive" : "outline"}
		size="icon"
		class="action-btn"
	>
		{#if reader.fetchButtonState === "loading"}
			<Loader2 class="h-4 w-4 animate-spin" />
			<span class="action-label">Fetching...</span>
		{:else if reader.fetchButtonState === "error"}
			<RefreshCw class="h-4 w-4" />
			<span class="action-label">Try again</span>
		{:else if reader.fetchButtonState === "success"}
			<RefreshCw class="h-4 w-4" />
			<span class="action-label">Re-fetch</span>
		{:else}
			<Download class="h-4 w-4" />
			<span class="action-label">Fetch</span>
		{/if}
	</Button>

	<Button
		data-testid="summarize-button"
		aria-label={reader.summarizeAriaLabel}
		onclick={reader.handleSummarize}
		disabled={reader.summarizer.buttonState === "loading" ||
			(!reader.articleContent &&
				reader.summarizer.buttonState !== "error" &&
				reader.summarizer.buttonState !== "success")}
		variant={reader.summarizer.buttonState === "error" ? "destructive" : "default"}
		size="icon"
		class={reader.summarizer.buttonState === "error"
			? "action-btn"
			: "action-btn action-btn--primary"}
	>
		{#if reader.summarizer.buttonState === "loading"}
			<Loader2 class="h-4 w-4 animate-spin" />
			<span class="action-label">Summarizing...</span>
		{:else if reader.summarizer.buttonState === "error"}
			<RefreshCw class="h-4 w-4" />
			<span class="action-label">Try again</span>
		{:else if reader.summarizer.buttonState === "success"}
			<RefreshCw class="h-4 w-4" />
			<span class="action-label">Re-summarize</span>
		{:else}
			<Sparkles class="h-4 w-4" />
			<span class="action-label">Summarize</span>
		{/if}
	</Button>

	{#if reader.articleUrl}
		<a
			href={reader.articleUrl}
			target="_blank"
			rel="noopener noreferrer"
			aria-label="Open original"
			class="action-link"
		>
			<ExternalLink class="h-4 w-4" />
			<span class="action-label">Open original</span>
		</a>
	{/if}
</div>

<style>
	:global(.actions-cluster) {
		display: flex;
		align-items: center;
		gap: 0.25rem;
		margin-left: auto;
		min-width: 0;
	}

	:global(.action-label) {
		display: none;
	}

	:global(.action-btn) {
		display: inline-flex;
		align-items: center;
		justify-content: center;
	}

	.action-link {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 2.25rem;
		height: 2.25rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		color: var(--interactive-text, #2f4f4f);
		background: transparent;
		text-decoration: none;
		transition: background 0.15s ease;
	}

	.action-link:hover,
	.action-link:focus-visible {
		background: var(--surface-hover, #f3f1ed);
		outline: none;
	}

	@media (min-width: 640px) {
		:global(.action-label) {
			display: inline;
		}

		:global(.action-btn) {
			width: auto;
			height: auto;
			gap: 0.4rem;
			padding: 0.4rem 0.75rem;
			font-family: var(--font-body);
			font-size: 0.85rem;
		}

		.action-link {
			width: auto;
			height: auto;
			gap: 0.4rem;
			padding: 0.4rem 0.75rem;
			font-family: var(--font-body);
			font-size: 0.85rem;
		}
	}
</style>
