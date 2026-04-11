<script lang="ts">
import type { RetentionRunResponse } from "$lib/types/sovereign-admin";

interface Props {
	result: RetentionRunResponse | null;
}

let { result }: Props = $props();

const statusColor = (status: string) => {
	switch (status) {
		case "exported":
		case "completed":
			return "var(--alt-sage)";
		case "dry_run":
		case "planned":
			return "var(--alt-primary)";
		case "failed":
			return "var(--alt-terracotta)";
		default:
			return "var(--alt-ash)";
	}
};
</script>

{#if result}
	<div class="panel" data-role="retention-result">
		<div class="panel-header">
			<h3 class="section-heading">Retention Run Result</h3>
			<span class="mode-badge" class:dry={result.dry_run}>
				{result.dry_run ? "Dry Run" : "Live"}
			</span>
		</div>
		<div class="heading-rule"></div>

		{#if result.error}
			<div class="error-banner">
				{result.error}
			</div>
		{/if}

		{#if result.actions.length === 0}
			<p class="empty-text">No actions taken.</p>
		{:else}
			<div class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th class="th-left">Status</th>
							<th class="th-left">Action</th>
							<th class="th-left">Table</th>
							<th class="th-left">Partition</th>
							<th class="th-right">Rows</th>
							<th class="th-left">Path</th>
						</tr>
					</thead>
					<tbody>
						{#each result.actions as action}
							<tr>
								<td>
									<span class="status-text" style="color: {statusColor(action.status)};">
										{action.status}
									</span>
								</td>
								<td>{action.action}</td>
								<td class="td-mono">{action.table}</td>
								<td class="td-mono">{action.partition}</td>
								<td class="td-right">{action.rows.toLocaleString()}</td>
								<td class="td-mono td-truncate" title={action.path ?? ""}>
									{action.path ?? "--"}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>
{/if}

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.panel-header {
		display: flex;
		align-items: center;
		gap: 0.5rem;
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

	.mode-badge {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-sage);
		border: 1px solid var(--alt-sage);
		padding: 0.15rem 0.4rem;
	}

	.mode-badge.dry {
		color: var(--alt-primary);
		border-color: var(--alt-primary);
	}

	.error-banner {
		border-left: 3px solid var(--alt-terracotta);
		background: var(--surface-bg);
		padding: 0.5rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-terracotta);
	}

	.empty-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
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

	.td-truncate {
		max-width: 12rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.status-text {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}
</style>
