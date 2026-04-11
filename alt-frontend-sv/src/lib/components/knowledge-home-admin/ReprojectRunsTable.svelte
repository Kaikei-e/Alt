<script lang="ts">
import type { ReprojectRunData } from "$lib/connect/knowledge_home_admin";

let {
	runs,
	disableActions = false,
	onCompare,
	onSwap,
	onRollback,
}: {
	runs: ReprojectRunData[];
	disableActions?: boolean;
	onCompare?: (run: ReprojectRunData) => Promise<void> | void;
	onSwap?: (run: ReprojectRunData) => Promise<void> | void;
	onRollback?: (run: ReprojectRunData) => Promise<void> | void;
} = $props();

const statusColor = (status: string) => {
	switch (status) {
		case "swappable":
		case "swapped":
			return "var(--alt-sage)";
		case "running":
		case "validating":
			return "var(--alt-sand)";
		case "pending":
			return "var(--alt-ash)";
		case "failed":
		case "cancelled":
			return "var(--alt-terracotta)";
		default:
			return "var(--alt-ash)";
	}
};

const modeLabel = (mode: string) => {
	switch (mode) {
		case "dry_run":
			return "Dry Run";
		case "user_subset":
			return "User Subset";
		case "time_range":
			return "Time Range";
		case "full":
			return "Full";
		default:
			return mode;
	}
};

const canCompare = (run: ReprojectRunData) => run.status === "swappable";
const canSwap = (run: ReprojectRunData) => run.status === "swappable";
const canRollback = (run: ReprojectRunData) => run.status === "swapped";
</script>

<div class="panel" data-role="reproject-runs">
	<h3 class="section-heading">Reproject Runs</h3>
	<div class="heading-rule"></div>

	{#if runs.length === 0}
		<p class="empty-text">No reproject runs.</p>
	{:else}
		<div class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th class="th-left">Status</th>
						<th class="th-left">Mode</th>
						<th class="th-left">From</th>
						<th class="th-left">To</th>
						<th class="th-left">Created</th>
						<th class="th-left">Finished</th>
						<th class="th-left">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each runs as run (run.reprojectRunId)}
						<tr>
							<td>
								<span class="status-text" style="color: {statusColor(run.status)};">
									{run.status}
								</span>
							</td>
							<td>{modeLabel(run.mode)}</td>
							<td class="td-mono">{run.fromVersion}</td>
							<td class="td-mono">{run.toVersion}</td>
							<td class="td-mono">
								{run.createdAt
									? new Date(run.createdAt).toLocaleString()
									: "--"}
							</td>
							<td class="td-mono">
								{run.finishedAt
									? new Date(run.finishedAt).toLocaleString()
									: "--"}
							</td>
							<td>
								<div class="action-group">
									<button
										class="action-btn"
										disabled={disableActions || !canCompare(run)}
										onclick={() => void onCompare?.(run)}
									>
										Compare
									</button>
									<button
										class="action-btn"
										disabled={disableActions || !canSwap(run)}
										onclick={() => void onSwap?.(run)}
									>
										Swap
									</button>
									<button
										class="action-btn"
										disabled={disableActions || !canRollback(run)}
										onclick={() => void onRollback?.(run)}
									>
										Rollback
									</button>
								</div>
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

	.status-text {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
	}

	.action-group {
		display: flex;
		gap: 0.35rem;
	}

	.action-btn {
		border: 1px solid var(--alt-charcoal);
		background: transparent;
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		padding: 0.2rem 0.5rem;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.action-btn:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-btn:disabled {
		opacity: 0.3;
		cursor: not-allowed;
	}
</style>
