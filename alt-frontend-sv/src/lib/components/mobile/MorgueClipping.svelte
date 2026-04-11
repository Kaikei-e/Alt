<script lang="ts">
import type { RenderFeed } from "$lib/schema/feed";
import FeedDetails from "./FeedDetails.svelte";

interface Props {
	feed: RenderFeed;
}

const { feed }: Props = $props();

let isDetailsOpen = $state(false);
</script>

<article
	class="morgue-card"
	data-role="morgue-clipping"
	role="article"
	aria-label="Filed: {feed.title}"
>
	<div class="morgue-content">
		<a
			href={feed.normalizedUrl}
			target="_blank"
			rel="noopener noreferrer"
			class="morgue-title"
		>
			{feed.title}
		</a>

		<p class="morgue-excerpt">{feed.excerpt}</p>

		<span class="morgue-dateline">
			{feed.publishedAtFormatted ?? ""}{feed.author ? ` \u00b7 ${feed.author}` : ""}
		</span>

		<div class="morgue-actions">
			<button
				onclick={() => { isDetailsOpen = true; }}
				class="action-btn"
				aria-label="Show details for {feed.title}"
			>
				Details
			</button>

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
	.morgue-card {
		display: flex;
		border-bottom: 1px solid var(--surface-border);
	}

	.morgue-content {
		flex: 1;
		padding: 0.75rem 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
	}

	.morgue-title {
		font-family: var(--font-display);
		font-size: 0.95rem;
		font-weight: 400;
		color: var(--alt-ash);
		line-height: 1.3;
		text-decoration: none;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.morgue-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.morgue-excerpt {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-ash);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
		margin: 0;
	}

	.morgue-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.morgue-actions {
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
