<script lang="ts">
import type { SnapshotMetadata } from "$lib/types/sovereign-admin";
import AdminMetricCard from "./AdminMetricCard.svelte";
import ConfirmActionDialog from "./ConfirmActionDialog.svelte";

interface Props {
	snapshots: SnapshotMetadata[];
	latestSnapshot: SnapshotMetadata | null;
	disabled?: boolean;
	onCreateSnapshot?: () => Promise<void> | void;
}

let {
	snapshots,
	latestSnapshot,
	disabled = false,
	onCreateSnapshot,
}: Props = $props();

let confirmOpen = $state(false);

const statusColor = (status: string) => {
	switch (status) {
		case "valid":
			return "var(--alt-sage)";
		case "pending":
			return "var(--alt-sand)";
		case "invalidated":
			return "var(--alt-terracotta)";
		case "archived":
			return "var(--alt-ash)";
		default:
			return "var(--alt-ash)";
	}
};

const formatDate = (d: string) => (d ? new Date(d).toLocaleString() : "--");
</script>

<div class="panel" data-role="snapshot-list" data-testid="snapshot-list-panel">
	<div class="panel-header">
		<h3 class="section-heading">Projection Snapshots</h3>
		<button
			class="action-btn"
			{disabled}
			onclick={() => (confirmOpen = true)}
		>
			Create Snapshot
		</button>
	</div>
	<div class="heading-rule"></div>

	{#if latestSnapshot}
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			<AdminMetricCard
				label="Latest Snapshot"
				value={latestSnapshot.status}
				status={latestSnapshot.status === "valid" ? "ok" : "warning"}
			/>
			<AdminMetricCard
				label="Event Boundary"
				value={latestSnapshot.eventSeqBoundary.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Items"
				value={latestSnapshot.itemsRowCount.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Schema"
				value={latestSnapshot.schemaVersion}
				status="neutral"
			/>
		</div>
	{/if}

	{#if snapshots.length === 0}
		<p class="empty-text">No snapshots.</p>
	{:else}
		<div class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th class="th-left">Status</th>
						<th class="th-left">Version</th>
						<th class="th-left">Build Ref</th>
						<th class="th-right">Event Seq</th>
						<th class="th-right">Items</th>
						<th class="th-right">Digest</th>
						<th class="th-right">Recall</th>
						<th class="th-left">Created</th>
					</tr>
				</thead>
				<tbody>
					{#each snapshots as snap (snap.snapshotId)}
						<tr>
							<td>
								<span class="status-text" style="color: {statusColor(snap.status)};">
									{snap.status}
								</span>
							</td>
							<td class="td-mono">v{snap.projectionVersion}</td>
							<td class="td-mono td-truncate" title={snap.projectorBuildRef}>
								{snap.projectorBuildRef}
							</td>
							<td class="td-right">{snap.eventSeqBoundary.toLocaleString()}</td>
							<td class="td-right">{snap.itemsRowCount.toLocaleString()}</td>
							<td class="td-right">{snap.digestRowCount.toLocaleString()}</td>
							<td class="td-right">{snap.recallRowCount.toLocaleString()}</td>
							<td class="td-mono">{formatDate(snap.createdAt)}</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}

	<ConfirmActionDialog
		open={confirmOpen}
		title="Create Projection Snapshot"
		description="This will export all projection tables (knowledge_home_items, today_digest_view, recall_candidate_view) to compressed JSONL files. This is a safe read-only operation."
		confirmLabel="Create Snapshot"
		variant="default"
		onConfirm={() => {
			confirmOpen = false;
			void onCreateSnapshot?.();
		}}
		onCancel={() => (confirmOpen = false)}
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
		max-width: 6rem;
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
