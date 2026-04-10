<script lang="ts">
import type { RecapSummary } from "$lib/schema/recap";

interface Props {
	recapData: RecapSummary | null;
	isLoading: boolean;
	error: Error | null;
}

const { recapData, isLoading, error }: Props = $props();

let topGenres = $derived(recapData?.genres?.slice(0, 3) ?? []);

let formattedTimestamp = $derived.by(() => {
	if (!recapData?.executedAt) return "";
	const d = new Date(recapData.executedAt);
	return `Updated: ${d.toLocaleDateString("en-US", { month: "2-digit", day: "2-digit" })} ${d.toLocaleTimeString("en-US", { hour: "2-digit", minute: "2-digit", hour12: false })}`;
});
</script>

<section class="brief">
	<h2 class="section-heading">THREE-DAY BRIEF</h2>

	{#if isLoading}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Retrieving brief&hellip;</span>
		</div>
	{:else if error}
		<div class="error-state">
			{error.message}
		</div>
	{:else if topGenres.length === 0}
		<p class="empty-state">No briefing available</p>
	{:else}
		<div class="genre-list">
			{#each topGenres as genre, i}
				{#if i > 0}
					<div class="genre-separator" aria-hidden="true"></div>
				{/if}
				<article
					class="genre-entry"
					style="--stagger: {i};"
				>
					<div class="genre-header">
						<h3 class="genre-heading">{genre.genre}</h3>
						<span class="genre-count">{genre.articleCount} articles</span>
					</div>
					<p class="genre-summary">{genre.summary}</p>
					{#if genre.topTerms && genre.topTerms.length > 0}
						<span class="genre-terms">
							{genre.topTerms.join(" \u00b7 ")}
						</span>
					{/if}
				</article>
			{/each}
		</div>

		{#if formattedTimestamp}
			<p class="timestamp">{formattedTimestamp}</p>
		{/if}
	{/if}
</section>

<style>
	.brief {
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

	.genre-list {
		display: flex;
		flex-direction: column;
	}

	.genre-entry {
		opacity: 0;
		animation: entry-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}

	.genre-separator {
		height: 1px;
		background: var(--surface-border);
		margin: 0.75rem 0;
	}

	.genre-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
		margin-bottom: 0.25rem;
	}

	.genre-heading {
		font-family: var(--font-display);
		font-size: 1rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
		line-height: 1.3;
	}

	.genre-count {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		white-space: nowrap;
		flex-shrink: 0;
	}

	.genre-summary {
		font-family: var(--font-body);
		font-size: 0.85rem;
		line-height: 1.6;
		color: var(--alt-slate);
		margin: 0 0 0.3rem;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.genre-terms {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		letter-spacing: 0.04em;
	}

	.timestamp {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
		margin: 1rem 0 0;
		letter-spacing: 0.04em;
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
		.genre-entry {
			animation: none;
			opacity: 1;
		}
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
