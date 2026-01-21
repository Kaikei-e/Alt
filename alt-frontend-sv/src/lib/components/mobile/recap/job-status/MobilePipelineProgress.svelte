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
	import { Check, Circle, Loader2 } from "@lucide/svelte";

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
</script>

<div class="py-2" data-testid="mobile-pipeline-progress">
	<div class="relative">
		{#each PIPELINE_STAGES as stage, index}
			{@const status = getStageStatus(stage, index)}
			{@const isLast = index === PIPELINE_STAGES.length - 1}
			<div
				class="flex items-start gap-3 {isLast ? '' : 'pb-4'}"
				data-stage-status={status}
			>
				<!-- Stage indicator with connector line -->
				<div class="flex flex-col items-center">
					<div
						class="w-6 h-6 rounded-full flex items-center justify-center text-xs font-medium flex-shrink-0
							{status === 'completed'
							? 'bg-green-100 text-green-700 border-2 border-green-500'
							: status === 'running'
								? 'bg-blue-100 text-blue-700 border-2 border-blue-500'
								: 'bg-gray-100 text-gray-500 border-2 border-gray-300'}"
					>
						{#if status === "completed"}
							<Check class="w-3 h-3" />
						{:else if status === "running"}
							<Loader2 class="w-3 h-3 animate-spin" />
						{:else}
							<Circle class="w-3 h-3" />
						{/if}
					</div>
					<!-- Connector line -->
					{#if !isLast}
						<div
							class="w-0.5 flex-1 min-h-[12px] mt-1
								{status === 'completed' ? 'bg-green-300' : 'bg-gray-200'}"
						></div>
					{/if}
				</div>

				<!-- Stage label -->
				<div class="flex-1 pt-0.5">
					<span
						class="text-sm font-medium
							{status === 'completed'
							? 'text-green-700'
							: status === 'running'
								? 'text-blue-700'
								: 'text-gray-500'}"
					>
						{getStageLabel(stage)}
					</span>
					{#if shouldShowSubStageProgress(stage, status, subStageProgress)}
						<span class="text-xs text-blue-600 ml-1">
							({formatSubStageProgress(subStageProgress!)})
						</span>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>
