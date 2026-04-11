<script lang="ts">
import type {
	RetentionLogEntry,
	EligiblePartitionsResult,
} from "$lib/types/sovereign-admin";
import ConfirmActionDialog from "./ConfirmActionDialog.svelte";

interface Props {
	retentionLogs: RetentionLogEntry[];
	eligiblePartitions: EligiblePartitionsResult[];
	disabled?: boolean;
	onRunRetention?: (dryRun: boolean) => Promise<void> | void;
}

let {
	retentionLogs,
	eligiblePartitions,
	disabled = false,
	onRunRetention,
}: Props = $props();

let confirmLiveOpen = $state(false);

const statusColor = (status: string) => {
	switch (status) {
		case "exported":
		case "success":
			return "var(--alt-sage)";
		case "dry_run":
			return "var(--alt-primary)";
		case "failed":
			return "var(--alt-terracotta)";
		default:
			return "var(--alt-ash)";
	}
};

const formatBytes = (bytes: number) => {
	if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
	if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`;
	if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
	return `${bytes} B`;
};

const formatDate = (d: string) => (d ? new Date(d).toLocaleString() : "--");

const totalEligiblePartitions = $derived(
	eligiblePartitions.reduce((sum, ep) => sum + ep.eligible.length, 0),
);
</script>

<div class="panel" data-role="retention-status" data-testid="retention-status-panel">
	<div class="panel-header">
		<h3 class="section-heading">Retention &amp; Archival</h3>
		<div class="action-buttons">
			<button
				class="action-btn"
				{disabled}
				onclick={() => void onRunRetention?.(true)}
			>
				Run Retention (Dry Run)
			</button>
			<button
				class="action-btn action-btn-primary"
				{disabled}
				onclick={() => (confirmLiveOpen = true)}
			>
				Run Retention
			</button>
		</div>
	</div>
	<div class="heading-rule"></div>

	<!-- Eligible partitions -->
	<div class="sub-section">
		<h4 class="sub-label">Archive-Eligible Partitions ({totalEligiblePartitions})</h4>
		{#if eligiblePartitions.length === 0 || totalEligiblePartitions === 0}
			<p class="empty-text">No partitions eligible for archival.</p>
		{:else}
			<div class="table-container">
				<table class="data-table">
					<thead>
						<tr>
							<th class="th-left">Table</th>
							<th class="th-left">Partition</th>
							<th class="th-left">Range</th>
							<th class="th-right">Rows</th>
							<th class="th-right">Size</th>
						</tr>
					</thead>
					<tbody>
						{#each eligiblePartitions as ep}
							{#each ep.eligible as part (part.name)}
								<tr>
									<td class="td-mono">{ep.table}</td>
									<td class="td-mono">{part.name}</td>
									<td>
										{part.rangeStart ? new Date(part.rangeStart).toLocaleDateString() : "?"} -- {part.rangeEnd ? new Date(part.rangeEnd).toLocaleDateString() : "?"}
									</td>
									<td class="td-right">{part.rowCount.toLocaleString()}</td>
									<td class="td-right">{formatBytes(part.sizeBytes)}</td>
								</tr>
							{/each}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<!-- Retention log -->
	<div class="sub-section">
		<h4 class="sub-label">Recent Retention Operations</h4>
		{#if retentionLogs.length === 0}
			<p class="empty-text">No retention operations recorded.</p>
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
							<th class="th-center">Dry Run</th>
							<th class="th-left">Run At</th>
							<th class="th-left">Error</th>
						</tr>
					</thead>
					<tbody>
						{#each retentionLogs as log (log.logId)}
							<tr>
								<td>
									<span class="status-text" style="color: {statusColor(log.status)};">
										{log.status}
									</span>
								</td>
								<td>{log.action}</td>
								<td class="td-mono">{log.targetTable}</td>
								<td class="td-mono td-truncate" title={log.targetPartition}>
									{log.targetPartition || "--"}
								</td>
								<td class="td-right">{log.rowsAffected.toLocaleString()}</td>
								<td class="td-center">{log.dryRun ? "Yes" : "No"}</td>
								<td class="td-mono">{formatDate(log.runAt)}</td>
								<td class="td-truncate" title={log.errorMessage}>
									{log.errorMessage || "--"}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<ConfirmActionDialog
		open={confirmLiveOpen}
		title="Run Live Retention"
		description="This will export eligible partitions to JSONL.gz archives. Ensure a valid snapshot exists before running. This operation cannot be undone."
		confirmLabel="Run Live Retention"
		variant="danger"
		onConfirm={() => {
			confirmLiveOpen = false;
			void onRunRetention?.(false);
		}}
		onCancel={() => (confirmLiveOpen = false)}
	/>
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.panel-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
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

	.action-buttons {
		display: flex;
		gap: 0.5rem;
	}

	.action-btn {
		border: 1.5px solid var(--alt-charcoal);
		background: transparent;
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		padding: 0.35rem 0.6rem;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.action-btn:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.action-btn-primary {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-btn-primary:hover:not(:disabled) {
		background: transparent;
		color: var(--alt-charcoal);
	}

	.sub-section {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.sub-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
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

	.td-truncate {
		max-width: 10rem;
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
