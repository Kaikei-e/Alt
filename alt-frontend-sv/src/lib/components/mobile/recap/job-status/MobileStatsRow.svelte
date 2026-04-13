<script lang="ts">
import LedgerFigure from "$lib/components/recap/job-status/LedgerFigure.svelte";

interface Props {
	successRate: string;
	avgDuration: string;
	totalJobs: number;
	runningJobs: number;
	failedJobs: number;
}

let { successRate, avgDuration, totalJobs, runningJobs, failedJobs }: Props =
	$props();

const stats = $derived([
	{ label: "Success rate", value: successRate, subtitle: "24h" },
	{ label: "Avg duration", value: avgDuration, subtitle: "Per job" },
	{
		label: "Jobs today",
		value: String(totalJobs),
		subtitle: `${runningJobs} running`,
	},
	{ label: "Failed", value: String(failedJobs), subtitle: "24h" },
]);
</script>

<div class="stats-row" data-testid="mobile-stats-row" data-role="ledger">
	{#each stats as stat}
		<div class="cell">
			<LedgerFigure
				label={stat.label}
				value={stat.value}
				subtitle={stat.subtitle}
			/>
		</div>
	{/each}
</div>

<style>
	.stats-row {
		display: flex;
		gap: 0;
		overflow-x: auto;
		scrollbar-width: none;
		-ms-overflow-style: none;
		padding: 0.5rem 1rem 1rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.stats-row::-webkit-scrollbar {
		display: none;
	}

	.cell {
		flex-shrink: 0;
		min-width: 140px;
		padding: 0 0.85rem;
		border-right: 1px solid var(--surface-border);
	}

	.cell:first-child {
		padding-left: 0;
	}

	.cell:last-child {
		border-right: none;
		padding-right: 0;
	}
</style>
