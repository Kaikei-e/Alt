<script lang="ts">
import type { RenderFeed } from "$lib/schema/feed";
import FeedDetails from "./FeedDetails.svelte";

interface Props {
	feed: RenderFeed;
	isReadStatus: boolean;
	setIsReadStatus: (feedLink: string) => void;
}

const { feed, isReadStatus, setIsReadStatus }: Props = $props();

const handleReadStatus = () => {
	setIsReadStatus(feed.normalizedUrl);
};
</script>

{#if !isReadStatus}
	<article
		class="dispatch-card"
		data-testid="feed-card-container"
		role="article"
		aria-label="Feed: {feed.title}"
	>
		<div class="dispatch-stripe" aria-hidden="true"></div>
		<div class="dispatch-content">
			<a
				href={feed.normalizedUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="dispatch-title"
			>
				{feed.title}
			</a>

			<p class="dispatch-excerpt">{feed.excerpt}</p>

			{#if feed.author}
				<span class="dispatch-dateline">
					{feed.publishedAtFormatted ?? ""}{feed.author ? ` \u00b7 ${feed.author}` : ""}
				</span>
			{/if}

			<div class="dispatch-actions">
				<button
					onclick={handleReadStatus}
					class="action-btn"
					aria-label="Mark {feed.title} as read"
				>
					Mark as Read
				</button>

				<FeedDetails feedURL={feed.link} feedTitle={feed.title} />
			</div>
		</div>
	</article>
{/if}

<style>
	.dispatch-card {
		display: flex;
		border-bottom: 1px solid var(--surface-border);
	}

	.dispatch-stripe {
		width: 3px;
		flex-shrink: 0;
		background: var(--alt-primary);
	}

	.dispatch-content {
		flex: 1;
		padding: 0.75rem 0.75rem 0.75rem 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.dispatch-title {
		font-family: var(--font-display);
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--alt-primary);
		line-height: 1.3;
		text-decoration: none;
	}

	.dispatch-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.dispatch-excerpt {
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

	.dispatch-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.dispatch-actions {
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
		transition:
			background 0.15s,
			color 0.15s;
	}

	.action-btn:active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}
</style>
