<script lang="ts">
	import {
		PIPELINE_STAGES,
		getStageLabel,
		type PipelineStage,
		type SubStageProgress,
	} from "$lib/schema/dashboard";
	import { Check, Circle, Loader2 } from "@lucide/svelte";

	interface Props {
		currentStage: string | null;
		stageIndex: number;
		stagesCompleted: string[];
		subStageProgress?: SubStageProgress | null;
	}

	let { currentStage, stageIndex, stagesCompleted, subStageProgress = null }: Props = $props();

	function getStageStatus(
		stage: PipelineStage,
		index: number
	): "completed" | "running" | "pending" {
		if (stagesCompleted.includes(stage)) {
			return "completed";
		}
		if (currentStage === stage || index === stageIndex) {
			return "running";
		}
		return "pending";
	}
</script>

<div class="w-full overflow-x-auto">
	<div class="flex items-center gap-1 min-w-max py-2">
		{#each PIPELINE_STAGES as stage, index}
			{@const status = getStageStatus(stage, index)}
			<div class="flex items-center">
				<!-- Stage indicator -->
				<div class="flex flex-col items-center gap-1">
					<div
						class="w-8 h-8 rounded-full flex items-center justify-center text-sm font-medium transition-all
							{status === 'completed'
							? 'bg-green-100 text-green-700 border-2 border-green-500'
							: status === 'running'
								? 'bg-blue-100 text-blue-700 border-2 border-blue-500'
								: 'bg-gray-100 text-gray-500 border-2 border-gray-300'}"
					>
						{#if status === "completed"}
							<Check class="w-4 h-4" />
						{:else if status === "running"}
							<Loader2 class="w-4 h-4 animate-spin" />
						{:else}
							<Circle class="w-4 h-4" />
						{/if}
					</div>
					<span
						class="text-xs font-medium
							{status === 'completed'
							? 'text-green-700'
							: status === 'running'
								? 'text-blue-700'
								: 'text-gray-500'}"
					>
						{getStageLabel(stage)}
						{#if stage === "dispatch" && status === "running" && subStageProgress}
							<span class="text-blue-600 ml-0.5">
								({subStageProgress.completed_genres}/{subStageProgress.total_genres})
							</span>
						{/if}
					</span>
				</div>

				<!-- Connector line -->
				{#if index < PIPELINE_STAGES.length - 1}
					<div
						class="w-6 h-0.5 mx-1
							{status === 'completed' ? 'bg-green-500' : 'bg-gray-300'}"
					></div>
				{/if}
			</div>
		{/each}
	</div>
</div>
