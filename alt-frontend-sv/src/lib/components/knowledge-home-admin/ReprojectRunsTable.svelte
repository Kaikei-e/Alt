<script lang="ts">
import type { ReprojectRunData } from "$lib/connect/knowledge_home_admin";
import { Button } from "$lib/components/ui/button";

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
		case "completed":
			return "var(--accent-green, #22c55e)";
		case "running":
			return "var(--accent-blue, #3b82f6)";
		case "pending":
			return "var(--accent-amber, #f59e0b)";
		case "failed":
			return "var(--accent-red, #ef4444)";
		case "swapped":
			return "var(--accent-green, #22c55e)";
		case "rolled_back":
			return "var(--text-secondary)";
		default:
			return "var(--text-secondary)";
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

const canCompare = (run: ReprojectRunData) =>
	run.status === "completed";
const canSwap = (run: ReprojectRunData) =>
	run.status === "completed";
const canRollback = (run: ReprojectRunData) =>
	run.status === "swapped";
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Reproject Runs
	</h3>

	{#if runs.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No reproject runs.</p>
	{:else}
		<div
			class="overflow-x-auto rounded-lg border-2"
			style="border-color: var(--border-primary);"
		>
			<table class="w-full text-xs">
				<thead>
					<tr style="background: var(--surface-bg);">
						<th class="px-3 py-2 text-left">Status</th>
						<th class="px-3 py-2 text-left">Mode</th>
						<th class="px-3 py-2 text-left">From</th>
						<th class="px-3 py-2 text-left">To</th>
						<th class="px-3 py-2 text-left">Created</th>
						<th class="px-3 py-2 text-left">Finished</th>
						<th class="px-3 py-2 text-left">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each runs as run (run.reprojectRunId)}
						<tr class="border-t" style="border-color: var(--border-primary);">
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-white text-xs font-medium"
									style="background: {statusColor(run.status)};"
								>
									{run.status}
								</span>
							</td>
							<td class="px-3 py-2">{modeLabel(run.mode)}</td>
							<td class="px-3 py-2 font-mono">{run.fromVersion}</td>
							<td class="px-3 py-2 font-mono">{run.toVersion}</td>
							<td class="px-3 py-2">
								{run.createdAt
									? new Date(run.createdAt).toLocaleString("ja-JP")
									: "--"}
							</td>
							<td class="px-3 py-2">
								{run.finishedAt
									? new Date(run.finishedAt).toLocaleString("ja-JP")
									: "--"}
							</td>
							<td class="px-3 py-2">
								<div class="flex gap-1">
									<Button
										variant="outline"
										size="sm"
										disabled={disableActions || !canCompare(run)}
										onclick={() => void onCompare?.(run)}
									>
										Compare
									</Button>
									<Button
										variant="outline"
										size="sm"
										disabled={disableActions || !canSwap(run)}
										onclick={() => void onSwap?.(run)}
									>
										Swap
									</Button>
									<Button
										variant="outline"
										size="sm"
										disabled={disableActions || !canRollback(run)}
										onclick={() => void onRollback?.(run)}
									>
										Rollback
									</Button>
								</div>
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>
