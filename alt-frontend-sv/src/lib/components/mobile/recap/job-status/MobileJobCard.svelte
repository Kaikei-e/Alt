<script lang="ts">
import type { RecentJobSummary } from "$lib/schema/dashboard";
import { formatDuration, PIPELINE_STAGES } from "$lib/schema/dashboard";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	job: RecentJobSummary;
	onSelect: (job: RecentJobSummary) => void;
}

let { job, onSelect }: Props = $props();

const startedAt = $derived(
	new Date(job.kicked_at).toLocaleString("ja-JP", {
		month: "numeric",
		day: "numeric",
		hour: "2-digit",
		minute: "2-digit",
	}),
);

const duration = $derived(formatDuration(job.duration_secs));

const completedStages = $derived.by(() => {
	const history = job.status_history || [];
	const completedSet = new Set<string>();
	for (const t of history) {
		if (t.status === "completed" && t.stage) {
			completedSet.add(t.stage);
		}
	}
	return completedSet.size;
});

const totalStages = PIPELINE_STAGES.length;

function handleClick() {
	onSelect(job);
}

function handleKeyDown(e: KeyboardEvent) {
	if (e.key === "Enter" || e.key === " ") {
		e.preventDefault();
		onSelect(job);
	}
}
</script>

<div
	class="mobile-job-card"
	data-testid="mobile-job-card"
	data-role="job-row"
	data-status={job.status}
	role="button"
	tabindex="0"
	onclick={handleClick}
	onkeydown={handleKeyDown}
	aria-label="View job {job.job_id}"
>
	<span class="stripe" aria-hidden="true"></span>
	<div class="content">
		<div class="row-top">
			<span class="job-id">{job.job_id.slice(0, 16)}…</span>
			<StatusGlyph
				status={job.status}
				pulse={job.status === "running"}
				includeLabel={true}
			/>
		</div>
		<div class="row-meta">
			<span class="started">{startedAt}</span>
			<span class="dot" aria-hidden="true">·</span>
			<span class="duration tabular-nums">{duration}</span>
			<span class="dot" aria-hidden="true">·</span>
			<span class="stages tabular-nums">{completedStages}/{totalStages}</span>
			<span class="dot" aria-hidden="true">·</span>
			<span class="source">
				{job.trigger_source === "user" ? "User" : "System"}
			</span>
		</div>
	</div>
	<span class="caret" aria-hidden="true">›</span>
</div>

<style>
	.mobile-job-card {
		display: grid;
		grid-template-columns: 3px 1fr auto;
		gap: 0.6rem;
		align-items: center;
		padding: 0.65rem 0.5rem 0.65rem 0;
		min-height: 44px;
		cursor: pointer;
		background: transparent;
	}

	.mobile-job-card:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: -2px;
	}

	.mobile-job-card:hover {
		background: var(--surface-hover);
	}

	.stripe {
		display: block;
		align-self: stretch;
		width: 3px;
		background: var(--alt-ash);
	}

	[data-status="completed"] .stripe,
	[data-status="succeeded"] .stripe {
		background: var(--alt-success);
	}

	[data-status="failed"] .stripe {
		background: var(--alt-error);
	}

	[data-status="running"] .stripe {
		background: var(--alt-charcoal);
		animation: stripe-pulse 1.2s ease-in-out infinite;
	}

	[data-status="pending"] .stripe {
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

	.content {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0;
	}

	.row-top {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 0.6rem;
	}

	.job-id {
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-charcoal);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.row-meta {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.3rem;
		font-family: var(--font-body);
		font-size: 0.7rem;
		color: var(--alt-slate);
	}

	.dot {
		color: var(--alt-ash);
	}

	.duration,
	.stages {
		font-family: var(--font-mono);
	}

	.caret {
		font-family: var(--font-mono);
		font-size: 1rem;
		color: var(--alt-ash);
	}

	@media (prefers-reduced-motion: reduce) {
		[data-status="running"] .stripe {
			animation: none;
		}
	}
</style>
