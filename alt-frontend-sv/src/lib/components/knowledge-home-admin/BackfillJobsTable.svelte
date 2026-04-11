<script lang="ts">
import type { BackfillJobData } from "$lib/connect/knowledge_home_admin";

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
			return "var(--alt-sage)";
		case "running":
			return "var(--alt-sand)";
		case "paused":
			return "var(--alt-ash)";
		case "failed":
			return "var(--alt-terracotta)";
		default:
			return "var(--alt-ash)";
	}
};

const progressPercent = (job: BackfillJobData) => {
	if (job.totalEvents === 0) return 0;
	return Math.round((job.processedEvents / job.totalEvents) * 100);
};

const canPause = (job: BackfillJobData) => job.status === "running";
const canResume = (job: BackfillJobData) => job.status === "paused";
</script>

<div class="panel" data-role="backfill-jobs">
	<h3 class="section-heading">Backfill Jobs</h3>
	<div class="heading-rule"></div>

	{#if jobs.length === 0}
		<p class="empty-text">No backfill jobs.</p>
	{:else}
		<div class="table-container">
			<table class="data-table">
				<thead>
					<tr>
						<th class="th-left">Status</th>
						<th class="th-left">Version</th>
						<th class="th-left">Progress</th>
						<th class="th-left">Created</th>
						<th class="th-left">Error</th>
						<th class="th-left">Actions</th>
					</tr>
				</thead>
				<tbody>
					{#each jobs as job (job.jobId)}
						<tr>
							<td>
								<span class="status-text" style="color: {statusColor(job.status)};">
									{job.status}
								</span>
							</td>
							<td class="td-mono">v{job.projectionVersion}</td>
							<td>
								<span class="td-mono">{job.processedEvents}/{job.totalEvents}</span>
								<span class="progress-pct">({progressPercent(job)}%)</span>
							</td>
							<td class="td-mono">
								{job.createdAt ? new Date(job.createdAt).toLocaleString() : "--"}
							</td>
							<td class="td-truncate" title={job.errorMessage}>
								{job.errorMessage || "--"}
							</td>
							<td>
								<div class="action-group">
									<button
										class="action-btn"
										disabled={disableActions || activeJobId === job.jobId || !canPause(job)}
										onclick={() => void onPause?.(job)}
									>
										{activeJobId === job.jobId && canPause(job) ? "Pausing..." : "Pause"}
									</button>
									<button
										class="action-btn"
										disabled={disableActions || activeJobId === job.jobId || !canResume(job)}
										onclick={() => void onResume?.(job)}
									>
										{activeJobId === job.jobId && canResume(job) ? "Resuming..." : "Resume"}
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

	.td-truncate {
		max-width: 12rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.progress-pct {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
		margin-left: 0.25rem;
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
