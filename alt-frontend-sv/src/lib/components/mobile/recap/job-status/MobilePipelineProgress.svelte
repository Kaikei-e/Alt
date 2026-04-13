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

<section
	class="mobile-pipeline"
	data-testid="mobile-pipeline-progress"
	data-role="pipeline-progress"
>
	<h4 class="kicker">Pipeline</h4>
	<ol class="steps">
		{#each PIPELINE_STAGES as stage, index}
			{@const status = getStageStatus(stage, index)}
			<li class="step" data-stage-status={status}>
				<span class="number" aria-hidden="true">{NUMBERS[index]}</span>
				<span class="label">{getStageLabel(stage)}</span>
				<StatusGlyph {status} pulse={status === "running"} />
				{#if shouldShowSubStageProgress(stage, status, subStageProgress)}
					<span class="substage">{formatSubStageProgress(subStageProgress!)}</span>
				{/if}
			</li>
		{/each}
	</ol>
</section>

<style>
	.mobile-pipeline {
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

	.steps {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0;
	}

	.step {
		display: grid;
		grid-template-columns: 1.5rem 1fr auto auto;
		align-items: baseline;
		gap: 0.6rem;
		padding: 0.45rem 0;
		border-bottom: 1px solid var(--surface-border);
		font-family: var(--font-mono);
		font-size: 0.75rem;
		color: var(--alt-ash);
	}

	.step:last-child {
		border-bottom: none;
	}

	.step[data-stage-status="completed"] {
		color: var(--alt-charcoal);
	}

	.step[data-stage-status="running"] {
		color: var(--alt-charcoal);
		font-weight: 600;
	}

	.number {
		font-family: var(--font-display);
		font-size: 0.95rem;
	}

	.label {
		font-family: var(--font-mono);
		font-size: 0.75rem;
	}

	.substage {
		font-size: 0.65rem;
		font-style: italic;
		color: var(--alt-slate);
	}
</style>
