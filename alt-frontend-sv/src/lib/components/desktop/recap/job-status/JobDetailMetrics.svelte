<script lang="ts">
import type {
	RecentJobSummary,
	JobStats,
} from "$lib/schema/dashboard";
import {
	calculateJobMetrics,
	getPerformanceLabel,
	formatDurationWithUnits,
} from "$lib/utils/stageMetrics";
import StageDurationBar from "./StageDurationBar.svelte";
import StatusTransitionTimeline from "./StatusTransitionTimeline.svelte";

interface Props {
	job: RecentJobSummary;
	stats?: JobStats;
}

let { job, stats }: Props = $props();

const metrics = $derived(
	calculateJobMetrics(
		job.status_history,
		job.kicked_at,
		job.status,
		job.duration_secs,
		stats?.avg_duration_secs ?? null,
	),
);

const performance = $derived(getPerformanceLabel(metrics.performanceRatio));

const performanceTone = $derived(
	performance.color === "green"
		? "success"
		: performance.color === "amber"
			? "warning"
			: performance.color === "red"
				? "error"
				: "muted",
);

const performanceGlyph = $derived(() => {
	if (!metrics.performanceRatio) return "●";
	if (metrics.performanceRatio <= 0.8) return "▲";
	if (metrics.performanceRatio > 1.2) return "▼";
	return "●";
});

const timeDelta = $derived.by(() => {
	if (!stats?.avg_duration_secs || !metrics.totalDurationSecs) return null;
	return metrics.totalDurationSecs - stats.avg_duration_secs;
});

const stagesCompleted = $derived(
	metrics.stageDurations.filter((s) => s.status === "completed").length,
);
</script>

<div class="detail-metrics" data-role="job-detail-metrics">
	<dl class="summary-row">
		<div class="summary-cell">
			<dt>Duration</dt>
			<dd class="figure tabular-nums">
				{formatDurationWithUnits(metrics.totalDurationSecs)}
			</dd>
		</div>
		<div class="summary-cell">
			<dt>Performance</dt>
			<dd class="performance" data-tone={performanceTone}>
				<span class="glyph" aria-hidden="true">{performanceGlyph()}</span>
				<span>{performance.label}</span>
			</dd>
		</div>
		<div class="summary-cell">
			<dt>vs Average</dt>
			<dd class="figure tabular-nums" data-tone={timeDelta === null ? "muted" : timeDelta > 0 ? "warning" : timeDelta < 0 ? "success" : "neutral"}>
				{#if timeDelta !== null}
					{timeDelta > 0 ? "+" : timeDelta < 0 ? "−" : ""}{formatDurationWithUnits(Math.abs(timeDelta))}
				{:else}
					—
				{/if}
			</dd>
		</div>
		<div class="summary-cell">
			<dt>Stages</dt>
			<dd class="figure tabular-nums">
				{stagesCompleted}/{metrics.stageDurations.length}
			</dd>
		</div>
	</dl>

	<section class="detail-section">
		<h4 class="kicker">Stage duration</h4>
		<StageDurationBar stageDurations={metrics.stageDurations} />
	</section>

	<section class="detail-section">
		<h4 class="kicker">Status history</h4>
		<StatusTransitionTimeline transitions={job.status_history} />
	</section>
</div>

<style>
	.detail-metrics {
		display: flex;
		flex-direction: column;
		gap: 1.25rem;
		padding: 1rem 0;
	}

	.summary-row {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
		gap: 0;
		margin: 0;
		border-top: 1px solid var(--surface-border);
		border-bottom: 1px solid var(--surface-border);
	}

	.summary-cell {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
		padding: 0.65rem 0.75rem;
		border-right: 1px solid var(--surface-border);
	}

	.summary-cell:last-child {
		border-right: none;
	}

	.summary-cell dt {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.summary-cell dd {
		margin: 0;
		font-family: var(--font-body);
		font-size: 0.95rem;
		color: var(--alt-charcoal);
	}

	.figure {
		font-family: var(--font-display);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.1;
	}

	[data-tone="success"] {
		color: var(--alt-success);
	}

	[data-tone="warning"] {
		color: var(--alt-warning);
	}

	[data-tone="error"] {
		color: var(--alt-error);
	}

	[data-tone="muted"] {
		color: var(--alt-ash);
	}

	.performance {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
	}

	.performance .glyph {
		font-family: var(--font-mono);
	}

	.detail-section {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.kicker {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}
</style>
