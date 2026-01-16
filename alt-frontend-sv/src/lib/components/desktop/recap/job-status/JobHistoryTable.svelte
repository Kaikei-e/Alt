<script lang="ts">
	import type { RecentJobSummary } from "$lib/schema/dashboard";
	import { formatDuration } from "$lib/schema/dashboard";
	import StatusBadge from "./StatusBadge.svelte";
	import StatusTransitionTimeline from "./StatusTransitionTimeline.svelte";
	import { Clock, User, Server, ChevronDown, ChevronRight } from "@lucide/svelte";

	interface Props {
		jobs: RecentJobSummary[];
	}

	let { jobs }: Props = $props();
	let expandedJobId = $state<string | null>(null);

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
						Last Stage
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
							{#if job.last_stage}
								<span
									class="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-gray-100"
									style="color: var(--text-muted);"
								>
									{job.last_stage}
								</span>
							{:else}
								<span style="color: var(--text-muted);">-</span>
							{/if}
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
							<td colspan="6" class="px-4 py-2">
								<div class="pl-6 border-l-2 border-gray-200 ml-2">
									<p class="text-xs font-semibold mb-2" style="color: var(--text-muted);">
										Status History
									</p>
									<StatusTransitionTimeline transitions={job.status_history} />
								</div>
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
