<script lang="ts">
import type { RenderFeed } from "$lib/schema/feed";

interface Props {
	feeds: RenderFeed[];
	isLoading: boolean;
	error: Error | null;
}

const { feeds, isLoading, error }: Props = $props();
</script>

<section class="dispatches">
	<h2 class="section-heading">LATEST DISPATCHES</h2>

	{#if isLoading}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Retrieving dispatches&hellip;</span>
		</div>
	{:else if error}
		<div class="error-state">
			{error.message}
		</div>
	{:else if feeds.length === 0}
		<p class="empty-state">No dispatches</p>
	{:else}
		<ul class="dispatch-list">
			{#each feeds as feed, i}
				<li
					class="dispatch-item"
					style="--stagger: {i};"
				>
					<a
						href={feed.link}
						target="_blank"
						rel="noopener noreferrer"
						class="dispatch-link"
					>
						<span class="dispatch-dateline">
							{feed.publishedAtFormatted}{feed.author ? ` \u00b7 ${feed.author}` : ""}
						</span>
						<span class="dispatch-title">{feed.title}</span>
						{#if feed.excerpt}
							<span class="dispatch-excerpt">{feed.excerpt}</span>
						{/if}
					</a>
				</li>
			{/each}
		</ul>

		<a href="/feeds" class="view-all">View All &rarr;</a>
	{/if}
</section>

<style>
	.dispatches {
		min-height: 0;
	}

	.section-heading {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0 0 0.75rem;
	}

	.dispatch-list {
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.dispatch-item {
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--surface-border);
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}

	.dispatch-item:last-child {
		border-bottom: none;
	}

	.dispatch-link {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		text-decoration: none;
		padding: 0.2rem 0;
		transition: background 0.15s;
	}

	.dispatch-link:hover {
		background: var(--surface-hover);
		margin: 0 -0.5rem;
		padding: 0.2rem 0.5rem;
	}

	.dispatch-dateline {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.dispatch-title {
		font-family: var(--font-display);
		font-size: 0.95rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		line-height: 1.3;
	}

	.dispatch-excerpt {
		font-family: var(--font-body);
		font-size: 0.82rem;
		color: var(--alt-slate);
		line-height: 1.5;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.view-all {
		display: inline-block;
		margin-top: 0.75rem;
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		color: var(--alt-primary);
		text-decoration: none;
	}

	.view-all:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 2rem 0;
		color: var(--alt-ash);
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.error-state {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-terracotta);
		padding: 1rem 0;
		border-left: 3px solid var(--alt-terracotta);
		padding-left: 0.75rem;
	}

	.empty-state {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-ash);
		padding: 2rem 0;
		margin: 0;
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

	@keyframes entry-in {
		to {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dispatch-item {
			animation: none;
			opacity: 1;
		}
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
