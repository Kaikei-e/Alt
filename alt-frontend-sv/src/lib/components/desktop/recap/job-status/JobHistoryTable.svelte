<script lang="ts">
import type { RecentJobSummary, JobStats } from "$lib/schema/dashboard";
import { formatDuration } from "$lib/schema/dashboard";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";
import JobDetailMetrics from "./JobDetailMetrics.svelte";
import { calculateStageDurations } from "$lib/utils/stageMetrics";

interface Props {
	jobs: RecentJobSummary[];
	stats?: JobStats;
}

let { jobs, stats }: Props = $props();
let expandedJobId = $state<string | null>(null);

function getStageCompletionCount(job: RecentJobSummary): {
	completed: number;
	total: number;
} {
	const durations = calculateStageDurations(
		job.status_history,
		job.kicked_at,
		job.status,
	);
	const completed = durations.filter((s) => s.status === "completed").length;
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

<div class="recent-jobs" data-role="recent-jobs">
	{#if jobs.length === 0}
		<p class="empty">No jobs in this window.</p>
	{:else}
		<ul class="job-list">
			{#each jobs as job}
				{@const stageCount = getStageCompletionCount(job)}
				{@const isOpen = expandedJobId === job.job_id}
				<li class="job-row" data-role="job-row" data-status={job.status}>
					<button
						type="button"
						class="row-button"
						onclick={() => toggleExpand(job.job_id)}
						aria-expanded={isOpen}
						aria-label="Toggle job {job.job_id}"
					>
						<span class="stripe" aria-hidden="true"></span>
						<span class="caret" aria-hidden="true">{isOpen ? "−" : "+"}</span>
						<span class="job-id" title={job.job_id}>
							{job.job_id.slice(0, 12)}…
						</span>
						<span class="status-cell">
							<StatusGlyph
								status={job.status}
								pulse={job.status === "running"}
								includeLabel={true}
							/>
						</span>
						<span class="stages tabular-nums">
							{stageCount.completed}/{stageCount.total}
						</span>
						<span class="source">
							{job.trigger_source === "user" ? "User" : "System"}
						</span>
						<span class="duration tabular-nums">
							{formatDuration(job.duration_secs)}
						</span>
						<span class="started" title={formatTime(job.kicked_at)}>
							{formatRelativeTime(job.kicked_at)}
						</span>
					</button>

					{#if isOpen}
						<div class="detail-pane" data-role="job-detail-pane">
							<JobDetailMetrics {job} {stats} />
						</div>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>

<style>
	.recent-jobs {
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.empty {
		padding: 1.5rem 0.75rem;
		font-family: var(--font-body);
		font-size: 0.95rem;
		font-style: italic;
		color: var(--alt-slate);
		text-align: center;
		margin: 0;
		border-top: 1px solid var(--surface-border);
		border-bottom: 1px solid var(--surface-border);
	}

	.job-list {
		list-style: none;
		margin: 0;
		padding: 0;
		border-top: 1px solid var(--surface-border);
	}

	.job-row {
		border-bottom: 1px solid var(--surface-border);
		position: relative;
	}

	.row-button {
		all: unset;
		display: grid;
		grid-template-columns: 3px 1.25rem minmax(8rem, 1.5fr) minmax(7rem, 1fr) 4rem 4.5rem 4.5rem 1fr;
		align-items: baseline;
		gap: 0.6rem;
		width: 100%;
		padding: 0.75rem 0.75rem 0.75rem 0;
		cursor: pointer;
		font-family: var(--font-body);
		min-height: 44px;
	}

	.row-button:hover {
		background: var(--surface-hover);
	}

	.row-button:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: -2px;
	}

	.stripe {
		display: block;
		width: 3px;
		height: 100%;
		min-height: 1.75rem;
		background: var(--alt-ash);
		align-self: stretch;
	}

	.job-row[data-status="completed"] .stripe,
	.job-row[data-status="succeeded"] .stripe {
		background: var(--alt-success);
	}

	.job-row[data-status="failed"] .stripe {
		background: var(--alt-error);
	}

	.job-row[data-status="running"] .stripe {
		background: var(--alt-charcoal);
		animation: stripe-pulse 1.2s ease-in-out infinite;
	}

	.job-row[data-status="pending"] .stripe {
		background: var(--alt-ash);
	}

	@keyframes stripe-pulse {
		0%,
		100% {
			opacity: 0.5;
		}
		50% {
			opacity: 1;
		}
	}

	.caret {
		font-family: var(--font-mono);
		font-size: 0.85rem;
		color: var(--alt-slate);
		text-align: center;
	}

	.job-id {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
	}

	.status-cell {
		display: inline-flex;
	}

	.stages,
	.duration {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
	}

	.source {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
	}

	.started {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
	}

	.detail-pane {
		padding: 0.75rem 1rem 1rem 1rem;
		background: var(--surface-2);
		border-top: 1px solid var(--surface-border);
	}

	@media (prefers-reduced-motion: reduce) {
		.job-row[data-status="running"] .stripe {
			animation: none;
		}
	}

	@media (max-width: 900px) {
		.row-button {
			grid-template-columns: 3px 1.25rem 1fr auto;
			grid-auto-rows: auto;
			row-gap: 0.25rem;
		}
		.stages,
		.source,
		.started {
			grid-column: 3 / span 2;
			color: var(--alt-slate);
			font-size: 0.7rem;
		}
	}
</style>
