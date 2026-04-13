<script lang="ts">
import type { RecentJobSummary } from "$lib/schema/dashboard";
import MobileJobCard from "./MobileJobCard.svelte";

interface Props {
	jobs: RecentJobSummary[];
	onJobSelect: (job: RecentJobSummary) => void;
}

let { jobs, onJobSelect }: Props = $props();
</script>

<section class="recent" data-role="recent-jobs">
	<header class="head">
		<h2 class="title">Recent jobs</h2>
		<span class="meta">{jobs.length}</span>
	</header>

	{#if jobs.length === 0}
		<p class="empty">No jobs in this window.</p>
	{:else}
		<ul class="list">
			{#each jobs as job (job.job_id)}
				<li>
					<MobileJobCard {job} onSelect={onJobSelect} />
				</li>
			{/each}
		</ul>
	{/if}
</section>

<style>
	.recent {
		padding: 0 1rem 1rem;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}

	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.5rem;
		padding-bottom: 0.4rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.title {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.meta {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
	}

	.empty {
		font-family: var(--font-body);
		font-size: 0.95rem;
		font-style: italic;
		color: var(--alt-slate);
		text-align: center;
		padding: 2rem 1rem;
		margin: 0;
	}

	.list {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: 1px solid var(--surface-border);
	}

	.list > li {
		border-bottom: 1px solid var(--surface-border);
	}
</style>
