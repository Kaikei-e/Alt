<script lang="ts">
import type {
	RetentionLogEntry,
	EligiblePartitionsResult,
} from "$lib/types/sovereign-admin";
import { Button } from "$lib/components/ui/button";
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
			return "var(--accent-green, #22c55e)";
		case "dry_run":
			return "var(--accent-blue, #3b82f6)";
		case "failed":
			return "var(--accent-red, #ef4444)";
		default:
			return "var(--text-secondary)";
	}
};

const formatBytes = (bytes: number) => {
	if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
	if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(1)} MB`;
	if (bytes >= 1024) return `${(bytes / 1024).toFixed(1)} KB`;
	return `${bytes} B`;
};

const formatDate = (d: string) => (d ? new Date(d).toLocaleString() : "—");

const totalEligiblePartitions = $derived(
	eligiblePartitions.reduce((sum, ep) => sum + ep.eligible.length, 0),
);
</script>

<div class="flex flex-col gap-4" data-testid="retention-status-panel">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
			Retention & Archival
		</h3>
		<div class="flex gap-2">
			<Button
				variant="outline"
				size="sm"
				{disabled}
				onclick={() => void onRunRetention?.(true)}
			>
				Run Retention (Dry Run)
			</Button>
			<Button
				variant="outline"
				size="sm"
				{disabled}
				onclick={() => (confirmLiveOpen = true)}
			>
				Run Retention (Live)
			</Button>
		</div>
	</div>

	<!-- Eligible partitions -->
	<div class="flex flex-col gap-2">
		<h4 class="text-xs font-medium" style="color: var(--text-secondary);">
			Archive-Eligible Partitions ({totalEligiblePartitions})
		</h4>
		{#if eligiblePartitions.length === 0 || totalEligiblePartitions === 0}
			<p class="text-xs" style="color: var(--text-secondary);">No partitions eligible for archival.</p>
		{:else}
			<div
				class="overflow-x-auto rounded-lg border-2"
				style="border-color: var(--border-primary);"
			>
				<table class="w-full text-xs">
					<thead>
						<tr style="background: var(--surface-bg);">
							<th class="px-3 py-2 text-left">Table</th>
							<th class="px-3 py-2 text-left">Partition</th>
							<th class="px-3 py-2 text-left">Range</th>
							<th class="px-3 py-2 text-right">Rows</th>
							<th class="px-3 py-2 text-right">Size</th>
						</tr>
					</thead>
					<tbody>
						{#each eligiblePartitions as ep}
							{#each ep.eligible as part (part.name)}
								<tr class="border-t" style="border-color: var(--border-primary);">
									<td class="px-3 py-2 font-mono">{ep.table}</td>
									<td class="px-3 py-2 font-mono">{part.name}</td>
									<td class="px-3 py-2">
										{part.rangeStart ? new Date(part.rangeStart).toLocaleDateString() : "?"} — {part.rangeEnd ? new Date(part.rangeEnd).toLocaleDateString() : "?"}
									</td>
									<td class="px-3 py-2 text-right tabular-nums">{part.rowCount.toLocaleString()}</td>
									<td class="px-3 py-2 text-right">{formatBytes(part.sizeBytes)}</td>
								</tr>
							{/each}
						{/each}
					</tbody>
				</table>
			</div>
		{/if}
	</div>

	<!-- Retention log -->
	<div class="flex flex-col gap-2">
		<h4 class="text-xs font-medium" style="color: var(--text-secondary);">
			Recent Retention Operations
		</h4>
		{#if retentionLogs.length === 0}
			<p class="text-xs" style="color: var(--text-secondary);">No retention operations recorded.</p>
		{:else}
			<div
				class="overflow-x-auto rounded-lg border-2"
				style="border-color: var(--border-primary);"
			>
				<table class="w-full text-xs">
					<thead>
						<tr style="background: var(--surface-bg);">
							<th class="px-3 py-2 text-left">Status</th>
							<th class="px-3 py-2 text-left">Action</th>
							<th class="px-3 py-2 text-left">Table</th>
							<th class="px-3 py-2 text-left">Partition</th>
							<th class="px-3 py-2 text-right">Rows</th>
							<th class="px-3 py-2 text-center">Dry Run</th>
							<th class="px-3 py-2 text-left">Run At</th>
							<th class="px-3 py-2 text-left">Error</th>
						</tr>
					</thead>
					<tbody>
						{#each retentionLogs as log (log.logId)}
							<tr class="border-t" style="border-color: var(--border-primary);">
								<td class="px-3 py-2">
									<span
										class="inline-block rounded px-2 py-0.5 text-white text-xs font-medium"
										style="background: {statusColor(log.status)};"
									>
										{log.status}
									</span>
								</td>
								<td class="px-3 py-2">{log.action}</td>
								<td class="px-3 py-2 font-mono">{log.targetTable}</td>
								<td class="px-3 py-2 font-mono max-w-40 truncate" title={log.targetPartition}>
									{log.targetPartition || "—"}
								</td>
								<td class="px-3 py-2 text-right tabular-nums">
									{log.rowsAffected.toLocaleString()}
								</td>
								<td class="px-3 py-2 text-center">
									{log.dryRun ? "Yes" : "No"}
								</td>
								<td class="px-3 py-2">{formatDate(log.runAt)}</td>
								<td class="px-3 py-2 max-w-40 truncate" title={log.errorMessage}>
									{log.errorMessage || "—"}
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
