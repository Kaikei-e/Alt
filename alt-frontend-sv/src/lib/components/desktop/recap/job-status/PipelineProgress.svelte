<script lang="ts">
import {
	PIPELINE_STAGES,
	getStageLabel,
	type PipelineStage,
	type SubStageProgress,
} from "$lib/schema/dashboard";
import {
	shouldShowSubStageProgress,
	formatSubStageProgress,
	inferStageCompletion,
} from "$lib/utils/pipelineProgress";
import StatusGlyph from "$lib/components/recap/job-status/StatusGlyph.svelte";

interface Props {
	currentStage: string | null;
	stageIndex: number;
	stagesCompleted: string[];
	subStageProgress?: SubStageProgress | null;
}

let {
	currentStage,
	stageIndex,
	stagesCompleted,
	subStageProgress = null,
}: Props = $props();

function getStageStatus(
	stage: PipelineStage,
	index: number,
): "completed" | "running" | "pending" {
	return inferStageCompletion(
		stage,
		index,
		stagesCompleted,
		currentStage,
		stageIndex,
	);
}

const NUMBERS = ["①", "②", "③", "④", "⑤", "⑥", "⑦", "⑧"] as const;
</script>

<ol class="pipeline" data-role="pipeline-progress">
	{#each PIPELINE_STAGES as stage, index}
		{@const status = getStageStatus(stage, index)}
		<li class="step" data-stage-status={status}>
			<span class="step-number" aria-hidden="true">{NUMBERS[index]}</span>
			<span class="step-label">{getStageLabel(stage)}</span>
			<StatusGlyph {status} pulse={status === "running"} />
			{#if shouldShowSubStageProgress(stage, status, subStageProgress)}
				<span class="substage">{formatSubStageProgress(subStageProgress!)}</span>
			{/if}
		</li>
	{/each}
</ol>

<style>
	.pipeline {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.4rem 1rem;
		list-style: none;
		margin: 0;
		padding: 0;
	}

	.step {
		display: inline-flex;
		align-items: baseline;
		gap: 0.35rem;
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-ash);
	}

	.step[data-stage-status="completed"] {
		color: var(--alt-charcoal);
	}

	.step[data-stage-status="running"] {
		color: var(--alt-charcoal);
		font-weight: 600;
	}

	.step[data-stage-status="pending"] {
		color: var(--alt-ash);
	}

	.step-number {
		font-family: var(--font-display);
		font-size: 0.85rem;
		color: inherit;
	}

	.step-label {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		letter-spacing: 0.02em;
	}

	.substage {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-slate);
		font-style: italic;
	}
</style>
