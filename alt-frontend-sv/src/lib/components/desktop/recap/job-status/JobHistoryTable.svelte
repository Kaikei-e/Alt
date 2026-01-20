<script lang="ts">
import { ChevronDown, ChevronRight, Clock, Server, User, BarChart3 } from "@lucide/svelte";
import type { RecentJobSummary, JobStats } from "$lib/schema/dashboard";
import { formatDuration } from "$lib/schema/dashboard";
import StatusBadge from "./StatusBadge.svelte";
import JobDetailMetrics from "./JobDetailMetrics.svelte";
import { calculateStageDurations, formatDurationWithUnits } from "$lib/utils/stageMetrics";

interface Props {
	jobs: RecentJobSummary[];
	/** Job statistics for performance comparison */
	stats?: JobStats;
}

let { jobs, stats }: Props = $props();
let expandedJobId = $state<string | null>(null);

// Pre-calculate stage completion count for mini indicator
function getStageCompletionCount(job: RecentJobSummary): { completed: number; total: number } {
	const durations = calculateStageDurations(job.status_history, job.kicked_at, job.status);
	const completed = durations.filter(s => s.status === "completed").length;
	return { completed, total: durations.length };
}

function toggleExpand(jobId: string) {
	expandedJobId = expandedJobId === jobId ? null : jobId;
}

function formatTime(isoString: string): string {
	return new Date(isoString).toLocaleString();
}

function formatRelativeTime(isoString: string): string {
	const date = new Date(isoString);
	const now = new Date();
	const diffMs = now.getTime() - date.getTime();
	const diffMins = Math.floor(diffMs / 60000);

	if (diffMins < 1) return "just now";
	if (diffMins < 60) return `${diffMins}m ago`;
	const diffHours = Math.floor(diffMins / 60);
	if (diffHours < 24) return `${diffHours}h ago`;
	const diffDays = Math.floor(diffHours / 24);
	return `${diffDays}d ago`;
}
</script>

<div
	class="border rounded-lg overflow-hidden"
	style="border-color: var(--surface-border);"
>
	<div class="overflow-x-auto">
		<table class="w-full">
			<thead>
				<tr style="background: var(--surface-bg);">
					<th
						class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider"
						style="color: var(--text-muted);"
					>
						Job ID
					</th>
					<th
						class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider"
						style="color: var(--text-muted);"
					>
						Status
					</th>
					<th
						class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider"
						style="color: var(--text-muted);"
					>
						Stages
					</th>
					<th
						class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider"
						style="color: var(--text-muted);"
					>
						Source
					</th>
					<th
						class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider"
						style="color: var(--text-muted);"
					>
						Duration
					</th>
					<th
						class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider"
						style="color: var(--text-muted);"
					>
						Started
					</th>
				</tr>
			</thead>
			<tbody class="divide-y" style="border-color: var(--surface-border);">
				{#each jobs as job}
					{@const stageCount = getStageCompletionCount(job)}
					<tr
						class="hover:bg-gray-50 transition-colors cursor-pointer"
						style="background: var(--bg);"
						onclick={() => toggleExpand(job.job_id)}
						onkeydown={(e) => e.key === 'Enter' && toggleExpand(job.job_id)}
						tabindex="0"
						role="button"
						aria-expanded={expandedJobId === job.job_id}
					>
						<td class="px-4 py-3">
							<span class="inline-flex items-center gap-2">
								{#if expandedJobId === job.job_id}
									<ChevronDown class="w-4 h-4" style="color: var(--text-muted);" />
								{:else}
									<ChevronRight class="w-4 h-4" style="color: var(--text-muted);" />
								{/if}
								<span
									class="font-mono text-sm"
									style="color: var(--text-primary);"
									title={job.job_id}
								>
									{job.job_id.slice(0, 8)}...
								</span>
							</span>
						</td>
						<td class="px-4 py-3">
							<StatusBadge status={job.status} size="sm" />
						</td>
						<td class="px-4 py-3">
							<div class="flex items-center gap-2">
								<BarChart3 class="w-3.5 h-3.5" style="color: var(--text-muted);" />
								<div class="flex items-center gap-1">
									<!-- Mini progress bar -->
									<div class="w-16 h-1.5 bg-gray-200 rounded-full overflow-hidden">
										<div
											class="h-full rounded-full transition-all {job.status === 'completed' ? 'bg-green-500' : job.status === 'failed' ? 'bg-red-400' : 'bg-blue-500'}"
											style="width: {(stageCount.completed / stageCount.total) * 100}%"
										></div>
									</div>
									<span
										class="text-xs font-medium tabular-nums"
										style="color: var(--text-muted);"
									>
										{stageCount.completed}/{stageCount.total}
									</span>
								</div>
							</div>
						</td>
						<td class="px-4 py-3">
							{#if job.trigger_source === "user"}
								<span
									class="inline-flex items-center gap-1 text-xs"
									style="color: var(--text-muted);"
								>
									<User class="w-3 h-3" />
									User
								</span>
							{:else}
								<span
									class="inline-flex items-center gap-1 text-xs"
									style="color: var(--text-muted);"
								>
									<Server class="w-3 h-3" />
									System
								</span>
							{/if}
						</td>
						<td class="px-4 py-3">
							<span
								class="text-sm tabular-nums"
								style="color: var(--text-primary);"
							>
								{formatDuration(job.duration_secs)}
							</span>
						</td>
						<td class="px-4 py-3">
							<span
								class="text-sm flex items-center gap-1"
								style="color: var(--text-muted);"
								title={formatTime(job.kicked_at)}
							>
								<Clock class="w-3 h-3" />
								{formatRelativeTime(job.kicked_at)}
							</span>
						</td>
					</tr>
					{#if expandedJobId === job.job_id}
						<tr style="background: var(--surface-bg);">
							<td colspan="6" class="px-4 py-4">
								<JobDetailMetrics job={job} {stats} />
							</td>
						</tr>
					{/if}
				{/each}
			</tbody>
		</table>
	</div>

	{#if jobs.length === 0}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			No jobs found in the selected time window.
		</div>
	{/if}
</div>
