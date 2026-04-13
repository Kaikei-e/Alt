<script lang="ts">
import type { GenreProgressInfo } from "$lib/schema/dashboard";
import { filterGenreProgress } from "$lib/utils/genreProgress";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	genreProgress: Record<string, GenreProgressInfo>;
}

let { genreProgress }: Props = $props();

const genres = $derived(filterGenreProgress(genreProgress));
</script>

{#if genres.length > 0}
	<section class="genre-section" data-role="genre-progress">
		<h4 class="kicker">By genre</h4>
		<ul class="grid">
			{#each genres as [genre, info]}
				<li class="row" data-status={info.status}>
					<span class="genre-name">{genre}</span>
					<span class="meta">
						{#if info.cluster_count !== null}
							<span class="count tabular-nums">{info.cluster_count}c</span>
						{/if}
						<StatusGlyph
							status={info.status}
							pulse={info.status === "running"}
						/>
					</span>
				</li>
			{/each}
		</ul>
	</section>
{:else}
	<p class="empty">No genre data.</p>
{/if}

<style>
	.genre-section {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.kicker {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
		gap: 0;
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: 1px solid var(--surface-border);
	}

	.row {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.75rem;
		padding: 0.55rem 0.75rem;
		border-bottom: 1px solid var(--surface-border);
		font-family: var(--font-body);
	}

	.genre-name {
		font-size: 0.85rem;
		color: var(--alt-charcoal);
		text-transform: capitalize;
	}

	.meta {
		display: inline-flex;
		align-items: baseline;
		gap: 0.6rem;
	}

	.count {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.empty {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0;
	}
</style>
