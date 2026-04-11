<script lang="ts">
import type { RenderFeed } from "$lib/schema/feed";
import FeedDetails from "./FeedDetails.svelte";

interface Props {
	feed: RenderFeed;
	onRemove?: (feedUrl: string) => void;
}

const { feed, onRemove }: Props = $props();

let isRemoving = $state(false);
let isDetailsOpen = $state(false);

const isRead = $derived(feed.isRead ?? false);

const handleRemove = async () => {
	if (!onRemove || isRemoving) return;
	isRemoving = true;
	try {
		onRemove(feed.normalizedUrl);
	} finally {
		isRemoving = false;
	}
};
</script>

<article
	class="clipping-entry"
	class:clipping-entry--unread={!isRead}
	class:clipping-entry--read={isRead}
	data-role="clippings-entry"
	role="article"
	aria-label="Clipping: {feed.title}"
>
	<div class="clipping-content">
		<div class="clipping-header">
			<a
				href={feed.normalizedUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="clipping-title"
				class:clipping-title--read={isRead}
			>
				{feed.title}
			</a>

			{#if isRead}
				<span class="read-label">Read</span>
			{/if}
		</div>

		<p class="clipping-excerpt" class:clipping-excerpt--read={isRead}>
			{feed.excerpt}
		</p>

		<span class="clipping-dateline">
			{feed.publishedAtFormatted ?? ""}{feed.author ? ` \u00b7 ${feed.author}` : ""}
		</span>

		<div class="clipping-actions">
			<button
				onclick={() => { isDetailsOpen = true; }}
				class="action-btn"
				aria-label="Show details for {feed.title}"
			>
				Details
			</button>

			{#if onRemove}
				<button
					onclick={handleRemove}
					disabled={isRemoving}
					class="action-btn action-btn--remove"
					aria-label="Remove from clippings"
				>
					{isRemoving ? "Removing\u2026" : "Remove"}
				</button>
			{/if}

			<a
				href={feed.normalizedUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="action-link"
				aria-label="Open {feed.title}"
			>
				Open
			</a>
		</div>
	</div>
</article>

<FeedDetails
	feedURL={feed.link}
	feedTitle={feed.title}
	open={isDetailsOpen}
	onOpenChange={(open) => { isDetailsOpen = open; }}
	showButton={false}
/>

<style>
	.clipping-entry {
		display: flex;
		border-bottom: 1px solid var(--surface-border);
	}

	.clipping-entry--unread {
		border-left: 1px solid var(--alt-primary);
	}

	.clipping-entry--read {
		border-left: 1px solid transparent;
	}

	.clipping-content {
		flex: 1;
		padding: 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.clipping-header {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 0.5rem;
	}

	.clipping-title {
		font-family: var(--font-display);
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		line-height: 1.3;
		text-decoration: none;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
		flex: 1;
	}

	.clipping-title--read {
		font-weight: 400;
		color: var(--alt-ash);
	}

	.clipping-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.read-label {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 500;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-ash);
		flex-shrink: 0;
		padding-top: 0.15rem;
	}

	.clipping-excerpt {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-charcoal);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
		margin: 0;
	}

	.clipping-excerpt--read {
		color: var(--alt-ash);
	}

	.clipping-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.clipping-actions {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.75rem;
		margin-top: 0.5rem;
	}

	.action-btn {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		padding: 0.4rem 0.75rem;
		min-height: 44px;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.action-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.action-btn--remove {
		border-color: var(--alt-ash);
		color: var(--alt-ash);
	}

	.action-link {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-primary);
		text-decoration: none;
		padding: 0.4rem 0.75rem;
		min-height: 44px;
		display: inline-flex;
		align-items: center;
	}

	.action-link:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}
</style>
