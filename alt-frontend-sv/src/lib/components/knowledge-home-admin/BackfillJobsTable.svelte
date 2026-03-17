<script lang="ts">
import type { BackfillJobData } from "$lib/connect/knowledge_home_admin";
import { Button } from "$lib/components/ui/button";

let {
	jobs,
	disableActions = false,
	activeJobId = null,
	onPause,
	onResume,
}: {
	jobs: BackfillJobData[];
	disableActions?: boolean;
	activeJobId?: string | null;
	onPause?: (job: BackfillJobData) => Promise<void> | void;
	onResume?: (job: BackfillJobData) => Promise<void> | void;
} = $props();

const statusColor = (status: string) => {
	switch (status) {
		case "completed":
			return "var(--accent-green, #22c55e)";
		case "running":
			return "var(--accent-blue, #3b82f6)";
		case "paused":
			return "var(--accent-amber, #f59e0b)";
		case "failed":
			return "var(--accent-red, #ef4444)";
		default:
			return "var(--text-secondary)";
	}
};

const progressPercent = (job: BackfillJobData) => {
	if (job.totalEvents === 0) return 0;
	return Math.round((job.processedEvents / job.totalEvents) * 100);
};

const canPause = (job: BackfillJobData) => job.status === "running";
const canResume = (job: BackfillJobData) => job.status === "paused";
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Backfill Jobs
	</h3>

	{#if jobs.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No backfill jobs.</p>
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
						<th class="px-3 py-2 text-left">Progress</th>
						<th class="px-3 py-2 text-left">Created</th>
						<th class="px-3 py-2 text-left">Error</th>
						<th class="px-3 py-2 text-left">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each jobs as job (job.jobId)}
						<tr class="border-t" style="border-color: var(--border-primary);">
							<td class="px-3 py-2">
								<span
									class="inline-block rounded px-2 py-0.5 text-white text-xs font-medium"
									style="background: {statusColor(job.status)};"
								>
									{job.status}
								</span>
							</td>
							<td class="px-3 py-2">v{job.projectionVersion}</td>
							<td class="px-3 py-2">
								{job.processedEvents}/{job.totalEvents}
								({progressPercent(job)}%)
							</td>
							<td class="px-3 py-2">
								{job.createdAt ? new Date(job.createdAt).toLocaleString() : "—"}
							</td>
							<td class="px-3 py-2 max-w-48 truncate" title={job.errorMessage}>
								{job.errorMessage || "—"}
							</td>
							<td class="px-3 py-2">
								<div class="flex gap-2">
									<Button
										variant="outline"
										size="sm"
										disabled={disableActions || activeJobId === job.jobId || !canPause(job)}
										onclick={() => void onPause?.(job)}
									>
										{activeJobId === job.jobId && canPause(job) ? "Pausing..." : "Pause"}
									</Button>
									<Button
										variant="outline"
										size="sm"
										disabled={disableActions || activeJobId === job.jobId || !canResume(job)}
										onclick={() => void onResume?.(job)}
									>
										{activeJobId === job.jobId && canResume(job) ? "Resuming..." : "Resume"}
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
