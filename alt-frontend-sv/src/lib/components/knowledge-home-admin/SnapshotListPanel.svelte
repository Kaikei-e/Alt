<script lang="ts">
import type { SnapshotMetadata } from "$lib/types/sovereign-admin";
import { Button } from "$lib/components/ui/button";
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
			return "var(--accent-green, #22c55e)";
		case "pending":
			return "var(--accent-amber, #f59e0b)";
		case "invalidated":
			return "var(--accent-red, #ef4444)";
		case "archived":
			return "var(--text-secondary)";
		default:
			return "var(--text-secondary)";
	}
};

const formatDate = (d: string) => (d ? new Date(d).toLocaleString() : "—");
</script>

<div class="flex flex-col gap-3" data-testid="snapshot-list-panel">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
			Projection Snapshots
		</h3>
		<Button
			variant="outline"
			size="sm"
			{disabled}
			onclick={() => (confirmOpen = true)}
		>
			Create Snapshot
		</Button>
	</div>

	<!-- Latest snapshot summary -->
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

	<!-- Snapshots table -->
	{#if snapshots.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No snapshots.</p>
	{:else}
		<div
			class="overflow-x-auto rounded-lg border-2"
			style="border-color: var(--border-primary);"
		>
			<table class="w-full text-xs">
				<thead>
					<tr style="background: var(--surface-bg);">
						<th class="px-3 py-2 text-left">Status</th>
						<th class="px-3 py-2 text-left">Version</th>
						<th class="px-3 py-2 text-left">Build Ref</th>
						<th class="px-3 py-2 text-right">Event Seq</th>
						<th class="px-3 py-2 text-right">Items</th>
						<th class="px-3 py-2 text-right">Digest</th>
						<th class="px-3 py-2 text-right">Recall</th>
						<th class="px-3 py-2 text-left">Created</th>
					</tr>
				</thead>
				<tbody>
					{#each snapshots as snap (snap.snapshotId)}
						<tr class="border-t" style="border-color: var(--border-primary);">
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-white text-xs font-medium"
									style="background: {statusColor(snap.status)};"
								>
									{snap.status}
								</span>
							</td>
							<td class="px-3 py-2">v{snap.projectionVersion}</td>
							<td class="px-3 py-2 font-mono max-w-24 truncate" title={snap.projectorBuildRef}>
								{snap.projectorBuildRef}
							</td>
							<td class="px-3 py-2 text-right tabular-nums">
								{snap.eventSeqBoundary.toLocaleString()}
							</td>
							<td class="px-3 py-2 text-right tabular-nums">
								{snap.itemsRowCount.toLocaleString()}
							</td>
							<td class="px-3 py-2 text-right tabular-nums">
								{snap.digestRowCount.toLocaleString()}
							</td>
							<td class="px-3 py-2 text-right tabular-nums">
								{snap.recallRowCount.toLocaleString()}
							</td>
							<td class="px-3 py-2">{formatDate(snap.createdAt)}</td>
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
