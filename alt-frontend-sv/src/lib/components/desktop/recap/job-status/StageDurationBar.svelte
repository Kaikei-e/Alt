<script lang="ts">
import { getStageLabel, type PipelineStage } from "$lib/schema/dashboard";
import {
	type StageDuration,
	formatDurationWithUnits,
	calculateBarWidth,
} from "$lib/utils/stageMetrics";

interface Props {
	stageDurations: StageDuration[];
	avgStageDurations?: Map<PipelineStage, number>;
	compact?: boolean;
}

let { stageDurations, avgStageDurations, compact = false }: Props = $props();

const maxDuration = $derived(
	Math.max(...stageDurations.map((s) => s.durationSecs), 1),
);

const totalDuration = $derived(
	stageDurations.reduce((sum, s) => sum + s.durationSecs, 0),
);

type Tone = "success" | "neutral" | "warning" | "muted" | "running";

function toneFor(
	status: StageDuration["status"],
	durationSecs: number,
	avgDuration?: number,
): Tone {
	if (status === "pending" || status === "skipped") return "muted";
	if (status === "running") return "running";
	if (avgDuration && avgDuration > 0) {
		const ratio = durationSecs / avgDuration;
		if (ratio <= 0.8) return "success";
		if (ratio <= 1.2) return "neutral";
		return "warning";
	}
	return "success";
}
</script>

<div
	class="duration-list"
	class:compact
	role="list"
	aria-label="Stage duration breakdown"
	data-role="stage-duration"
>
	{#each stageDurations as stage}
		{@const barWidth = calculateBarWidth(stage.durationSecs, maxDuration)}
		{@const avgDuration = avgStageDurations?.get(stage.stage)}
		{@const tone = toneFor(stage.status, stage.durationSecs, avgDuration)}

		<div class="row" role="listitem" data-tone={tone}>
			<span class="stage-label">{getStageLabel(stage.stage)}</span>
			<div
				class="bar"
				role="progressbar"
				aria-valuenow={stage.durationSecs}
				aria-valuemax={maxDuration}
			>
				{#if barWidth > 0}
					<div
						class="bar-fill"
						class:bar-fill--pulse={tone === "running"}
						style="width: {barWidth}%"
					></div>
				{/if}
			</div>
			<span class="duration tabular-nums">
				{formatDurationWithUnits(stage.durationSecs)}
			</span>
		</div>
	{/each}

	<div class="total-row">
		<span class="stage-label total-label">Total</span>
		<span class="duration total-duration tabular-nums">
			{formatDurationWithUnits(totalDuration)}
		</span>
	</div>
</div>

<style>
	.duration-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.duration-list.compact {
		gap: 0.3rem;
	}

	.row {
		display: grid;
		grid-template-columns: 6rem 1fr 4.5rem;
		align-items: center;
		gap: 0.6rem;
	}

	.stage-label {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-charcoal);
		text-transform: lowercase;
	}

	.row[data-tone="muted"] .stage-label {
		color: var(--alt-ash);
	}

	.bar {
		height: 1px;
		background: var(--surface-border);
		position: relative;
	}

	.bar-fill {
		position: absolute;
		top: -1px;
		left: 0;
		height: 3px;
		transition: width 0.3s ease;
	}

	.row[data-tone="success"] .bar-fill {
		background: var(--alt-success);
	}

	.row[data-tone="neutral"] .bar-fill {
		background: var(--alt-charcoal);
	}

	.row[data-tone="warning"] .bar-fill {
		background: var(--alt-warning);
	}

	.row[data-tone="running"] .bar-fill {
		background: var(--alt-charcoal);
	}

	.row[data-tone="muted"] .bar-fill {
		background: var(--surface-border);
	}

	.bar-fill--pulse {
		animation: bar-pulse 1.2s ease-in-out infinite;
	}

	@keyframes bar-pulse {
		0%,
		100% {
			opacity: 0.45;
		}
		50% {
			opacity: 1;
		}
	}

	.duration {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		text-align: right;
		color: var(--alt-charcoal);
	}

	.total-row {
		display: grid;
		grid-template-columns: 6rem 1fr 4.5rem;
		align-items: baseline;
		gap: 0.6rem;
		padding-top: 0.5rem;
		margin-top: 0.25rem;
		border-top: 1px solid var(--surface-border);
	}

	.total-label {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.total-duration {
		font-weight: 600;
	}

	@media (prefers-reduced-motion: reduce) {
		.bar-fill--pulse {
			animation: none;
		}
		.bar-fill {
			transition: none;
		}
	}
</style>
