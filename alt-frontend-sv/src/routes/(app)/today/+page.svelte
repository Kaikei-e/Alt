<script lang="ts">
import { onMount } from "svelte";
import type { ReadTodayQueueItem } from "#lib/connect/reading.js";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import type { ReaderArticleItem } from "#lib/reader/types.js";
import {
	formatTimeLeft,
	sourceLabelOf,
} from "#lib/reading/read-today-format.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";

let removeError = $state<string | null>(null);
let liveMessage = $state("");

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

function loadQueue() {
	removeError = null;
	void readTodayQueue.refresh().catch(() => {
		// Error state is exposed via readTodayQueue.error
	});
}

onMount(() => {
	loadQueue();
});

function toReaderItem(item: ReadTodayQueueItem): ReaderArticleItem {
	return {
		articleId: item.articleId,
		url: item.url,
		title: item.title,
		author: item.sourceName || null,
	};
}

function handleOpenItem(index: number) {
	if (readTodayQueue.items.length === 0) return;
	openReaderOverlay({
		surface: ReadingSurface.OTHER,
		items: readTodayQueue.items.map(toReaderItem),
		currentIndex: index,
		onArticlesRead: () => {
			void readTodayQueue.refresh().catch(() => {});
		},
	});
}

async function handleRemove(item: ReadTodayQueueItem) {
	removeError = null;
	try {
		await readTodayQueue.unqueue(item.articleId);
		liveMessage = `Removed ${item.title}`;
	} catch {
		removeError = "Could not remove article. Please try again.";
		liveMessage = removeError;
	}
}
</script>

<svelte:head>
	<title>Today - Alt</title>
</svelte:head>

<div class="sr-only" aria-live="polite" aria-atomic="true">
	{liveMessage}
</div>

<div class="today-page">
	<header class="today-header">
		<span class="today-date">{dateStr}</span>
		<h1 class="today-title">Today</h1>
		<div class="today-rule" aria-hidden="true"></div>
	</header>

	<section class="today-section" aria-labelledby="read-today-heading">
		<div class="section-head">
			<h2 id="read-today-heading" class="section-heading">Read today</h2>
			<span class="section-count" data-testid="today-queue-count">
				{readTodayQueue.items.length} / {readTodayQueue.cap}
			</span>
		</div>

		{#if removeError}
			<p class="inline-error" role="alert" data-testid="today-remove-error">
				{removeError}
			</p>
		{/if}

		{#if readTodayQueue.isLoading && !readTodayQueue.hasLoaded}
			<div class="state-block state-loading" data-testid="today-loading">
				<span class="loading-dot" aria-hidden="true"></span>
				<span class="state-text">Loading queue...</span>
			</div>
		{:else if readTodayQueue.error && readTodayQueue.items.length === 0}
			<div class="state-block state-error" role="alert" data-testid="today-error">
				<p class="error-text">Could not load Read today.</p>
				<button
					type="button"
					class="action-btn"
					data-testid="today-retry"
					onclick={loadQueue}
				>
					Retry
				</button>
			</div>
		{:else if readTodayQueue.items.length === 0}
			<div class="empty-block" data-testid="today-empty">
				<p class="empty-sentence">Nothing queued.</p>
				<div class="entry-links">
					<a href="/feeds/swipe/visual-preview" class="entry-link">Swipe</a>
					<a href="/feeds/visual-preview" class="entry-link">Gallery</a>
				</div>
			</div>
		{:else}
			<ul class="queue-list" data-testid="today-queue-list">
				{#each readTodayQueue.items as item, index (item.articleId)}
					{@const kicker = sourceLabelOf(item.sourceName, item.url)}
					{@const expiry = formatTimeLeft(item.expiresAt)}
					<li class="queue-row" data-testid="today-queue-row">
						<button
							type="button"
							class="row-open"
							onclick={() => handleOpenItem(index)}
						>
							{#if kicker}
								<span class="row-kicker">{kicker}</span>
							{/if}
							<span class="row-headline">{item.title}</span>
							{#if expiry}
								<span class="row-meta">{expiry}</span>
							{/if}
						</button>
						<button
							type="button"
							class="row-remove"
							aria-label="Remove {item.title}"
							data-testid="today-remove-button"
							onclick={() => void handleRemove(item)}
						>
							Remove
						</button>
					</li>
				{/each}
			</ul>
		{/if}

		{#if readTodayQueue.readCountSince > 0}
			<p class="read-count" data-testid="today-read-count">
				Read today: {readTodayQueue.readCountSince}
			</p>
		{/if}
	</section>
</div>

<style>
	.sr-only {
		position: absolute;
		left: -10000px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}

	.today-page {
		max-width: 42rem;
		margin: 0 auto;
		padding: 1rem 1.25rem 2rem;
		background: var(--surface-bg);
		color: var(--alt-charcoal);
		font-family: var(--font-body);
	}

	.today-header {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}

	.today-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.today-title {
		font-family: var(--font-display);
		font-size: 1.5rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
		line-height: 1.2;
	}

	.today-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.65rem;
	}

	.today-section {
		margin-top: 1.15rem;
	}

	.section-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		padding-bottom: 0.5rem;
		border-bottom: 1px solid var(--alt-charcoal);
	}

	.section-heading {
		font-family: var(--font-body);
		font-size: 0.78rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.section-count {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-slate);
	}

	.inline-error {
		margin: 0.75rem 0 0;
		padding: 0.5rem 0.75rem;
		font-size: 0.85rem;
		color: var(--alt-error);
		border-left: 3px solid var(--alt-error);
		background: var(--surface-2);
	}

	.queue-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.queue-row {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.row-open {
		flex: 1;
		min-width: 0;
		min-height: 44px;
		padding: 0;
		border: none;
		background: transparent;
		text-align: left;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		cursor: pointer;
		touch-action: manipulation;
	}

	.row-open:active .row-headline {
		text-decoration: underline;
	}

	.row-kicker {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		letter-spacing: 0.04em;
		color: var(--alt-slate);
	}

	.row-headline {
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 600;
		line-height: 1.3;
		color: var(--alt-primary);
		word-break: break-word;
	}

	.row-meta {
		font-family: var(--font-mono);
		font-size: 0.68rem;
		color: var(--alt-ash);
	}

	.row-remove {
		flex-shrink: 0;
		min-height: 44px;
		min-width: 44px;
		padding: 0.4rem 0.75rem;
		border: 1px solid var(--surface-border);
		background: transparent;
		font-family: var(--font-body);
		font-size: 0.78rem;
		font-weight: 600;
		color: var(--alt-slate);
		cursor: pointer;
		touch-action: manipulation;
	}

	.row-remove:active {
		background: var(--surface-hover);
		color: var(--alt-charcoal);
	}

	.empty-block {
		padding: 1.75rem 0;
		border-bottom: 1px solid var(--surface-border);
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.empty-sentence {
		margin: 0;
		font-size: 0.95rem;
		color: var(--alt-slate);
	}

	.entry-links {
		display: flex;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.entry-link {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		padding: 0.5rem 1.25rem;
		border: 1.5px solid var(--alt-charcoal);
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		text-decoration: none;
		touch-action: manipulation;
	}

	.entry-link:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.state-block {
		padding: 1.5rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.state-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.loading-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.state-text {
		font-size: 0.85rem;
		color: var(--alt-slate);
	}

	.state-error {
		border-left: 3px solid var(--alt-error);
		padding-left: 0.75rem;
	}

	.error-text {
		margin: 0 0 0.75rem;
		font-size: 0.85rem;
		color: var(--alt-error);
	}

	.action-btn {
		min-height: 44px;
		padding: 0.45rem 1.1rem;
		border: 1.5px solid var(--alt-charcoal);
		background: transparent;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		cursor: pointer;
	}

	.action-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.read-count {
		margin: 1rem 0 0;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-slate);
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-dot {
			animation: none;
			opacity: 0.6;
		}
	}
</style>
