<script lang="ts">
import type { TableStorageInfo } from "$lib/types/sovereign-admin";
import AdminMetricCard from "./AdminMetricCard.svelte";

interface Props {
	stats: TableStorageInfo[];
}

let { stats }: Props = $props();

const sizeStatus = (bytes: number): "ok" | "warning" | "error" | "neutral" => {
	if (bytes > 1_000_000_000) return "error";
	if (bytes > 500_000_000) return "warning";
	return "ok";
};
</script>

<div class="panel" data-role="storage-stats">
	<h3 class="section-heading">Table Storage</h3>
	<div class="heading-rule"></div>

	{#if stats.length === 0}
		<p class="empty-text">No storage data available.</p>
	{:else}
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			{#each stats as stat (stat.table_name)}
				<AdminMetricCard
					label={stat.table_name}
					value={stat.total_size}
					status={sizeStatus(stat.total_bytes)}
				>
					{#snippet icon()}
						{#if stat.is_partitioned}
							<span class="partition-badge">partitioned</span>
						{/if}
					{/snippet}
				</AdminMetricCard>
			{/each}
		</div>

		<div class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th class="th-left">Table</th>
						<th class="th-right">Rows</th>
						<th class="th-right">Size</th>
						<th class="th-center">Partitioned</th>
					</tr>
				</thead>
				<tbody>
					{#each stats as stat (stat.table_name)}
						<tr>
							<td class="td-mono">{stat.table_name}</td>
							<td class="td-right">{stat.row_count.toLocaleString()}</td>
							<td class="td-right">{stat.total_size}</td>
							<td class="td-center">
								{#if stat.is_partitioned}
									<span class="yes-text">Yes</span>
								{:else}
									<span class="no-text">No</span>
								{/if}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.heading-rule {
		height: 1px;
		background: var(--surface-border);
		margin-bottom: 0.25rem;
	}

	.empty-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.partition-badge {
		font-family: var(--font-mono);
		font-size: 0.55rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-primary);
		border: 1px solid var(--alt-primary);
		padding: 0.1rem 0.3rem;
	}

	.table-container {
		overflow-x: auto;
		border: 1px solid var(--surface-border);
	}

	.data-table {
		width: 100%;
		border-collapse: collapse;
		font-size: 0.7rem;
	}

	.data-table thead tr {
		background: var(--surface-2);
	}

	.data-table th {
		padding: 0.5rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.th-left { text-align: left; }
	.th-right { text-align: right; }
	.th-center { text-align: center; }

	.data-table tbody tr {
		border-top: 1px solid var(--surface-border);
	}

	.data-table td {
		padding: 0.5rem 0.75rem;
		color: var(--alt-charcoal);
	}

	.td-mono {
		font-family: var(--font-mono);
		font-size: 0.65rem;
	}

	.td-right {
		text-align: right;
		font-variant-numeric: tabular-nums;
	}

	.td-center {
		text-align: center;
	}

	.yes-text {
		color: var(--alt-sage);
		font-weight: 600;
	}

	.no-text {
		color: var(--alt-ash);
	}
</style>
