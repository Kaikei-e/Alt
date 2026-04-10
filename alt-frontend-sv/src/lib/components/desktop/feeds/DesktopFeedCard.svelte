<script lang="ts">
import type { RenderFeed } from "$lib/schema/feed";

interface Props {
	feed: RenderFeed;
	onSelect: (feed: RenderFeed) => void;
	isRead?: boolean;
}

let { feed, onSelect, isRead = false }: Props = $props();

const tags = $derived(
	feed.mergedTagsLabel
		? feed.mergedTagsLabel.split(" / ").slice(0, 3).join(" \u00b7 ")
		: "",
);

function handleClick() {
	onSelect(feed);
}
</script>

<button
	type="button"
	onclick={handleClick}
	class="dispatch-card"
	class:dispatch-card--unread={!isRead}
	class:dispatch-card--read={isRead}
	aria-label="Open {feed.title}"
>
	<span class="dispatch-dateline">
		{feed.publishedAtFormatted}{feed.author ? ` \u00b7 ${feed.author}` : ""}
	</span>
	<span class="dispatch-title">{feed.title}</span>
	{#if feed.excerpt}
		<span class="dispatch-excerpt">{feed.excerpt}</span>
	{/if}
	{#if tags}
		<span class="dispatch-tags">{tags}</span>
	{/if}
</button>

<style>
	.dispatch-card {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		width: 100%;
		text-align: left;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		cursor: pointer;
		transition: background 0.15s;
		background: var(--surface-bg);
	}

	.dispatch-card:hover {
		background: var(--surface-hover);
	}

	.dispatch-card--unread {
		border-left: 3px solid var(--alt-primary);
	}

	.dispatch-card--read {
		border-left: 3px solid transparent;
	}

	.dispatch-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.dispatch-title {
		font-family: var(--font-display);
		font-size: 0.9rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		line-height: 1.3;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.dispatch-card--read .dispatch-title {
		color: var(--alt-ash);
		font-weight: 400;
	}

	.dispatch-excerpt {
		font-family: var(--font-body);
		font-size: 0.78rem;
		color: var(--alt-slate);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.dispatch-card--read .dispatch-excerpt {
		color: var(--alt-ash);
	}

	.dispatch-tags {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
		margin-top: auto;
		padding-top: 0.3rem;
		border-top: 1px solid var(--surface-border);
	}
</style>
