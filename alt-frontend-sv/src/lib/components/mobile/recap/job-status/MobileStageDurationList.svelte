<script lang="ts">
import type { StatusTransition, JobStatus } from "$lib/schema/dashboard";
import { getStageLabel } from "$lib/schema/dashboard";
import { calculateStageDurations } from "$lib/utils/stageMetrics";

interface Props {
	statusHistory: StatusTransition[];
	jobStatus?: JobStatus;
	jobKickedAt?: string;
}

let {
	statusHistory,
	jobStatus = "completed",
	jobKickedAt = "",
}: Props = $props();

const stageDurations = $derived(
	calculateStageDurations(statusHistory, jobKickedAt, jobStatus).filter(
		(s) => s.durationSecs > 0,
	),
);

const maxDuration = $derived.by(() => {
	let max = 0;
	for (const s of stageDurations) {
		if (s.durationSecs > max) max = s.durationSecs;
	}
	return max || 1;
});

const totalDuration = $derived.by(() => {
	let total = 0;
	for (const s of stageDurations) {
		total += s.durationSecs;
	}
	return total;
});

function formatSeconds(secs: number): string {
	if (secs < 60) return `${secs.toFixed(1)}s`;
	const mins = Math.floor(secs / 60);
	const remainingSecs = Math.round(secs % 60);
	return `${mins}m ${remainingSecs}s`;
}
</script>

<section class="duration-list" data-role="stage-duration">
	<h4 class="kicker">Stage duration</h4>

	{#if stageDurations.length === 0}
		<p class="empty">No duration data.</p>
	{:else}
		<div class="rows">
			{#each stageDurations as stageDuration}
				<div class="row">
					<span class="stage-label">
						{getStageLabel(stageDuration.stage)}
					</span>
					<div class="bar">
						<div
							class="bar-fill"
							style="width: {(stageDuration.durationSecs / maxDuration) * 100}%"
						></div>
					</div>
					<span class="duration tabular-nums">
						{formatSeconds(stageDuration.durationSecs)}
					</span>
				</div>
			{/each}

			<div class="total-row">
				<span class="total-label">Total</span>
				<span class="duration total-duration tabular-nums">
					{formatSeconds(totalDuration)}
				</span>
			</div>
		</div>
	{/if}
</section>

<style>
	.duration-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
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

	.empty {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0;
	}

	.rows {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.row {
		display: grid;
		grid-template-columns: 5rem 1fr 4rem;
		align-items: center;
		gap: 0.5rem;
	}

	.stage-label {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-charcoal);
		text-transform: lowercase;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
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
		background: var(--alt-charcoal);
		transition: width 0.3s ease;
	}

	.duration {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		text-align: right;
		color: var(--alt-charcoal);
	}

	.total-row {
		display: grid;
		grid-template-columns: 5rem 1fr 4rem;
		align-items: baseline;
		gap: 0.5rem;
		padding-top: 0.5rem;
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
		.bar-fill {
			transition: none;
		}
	}
</style>
