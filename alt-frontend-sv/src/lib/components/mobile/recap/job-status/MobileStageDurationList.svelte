<script lang="ts">
	import type { StatusTransition, PipelineStage, JobStatus } from "$lib/schema/dashboard";
	import { PIPELINE_STAGES, getStageLabel } from "$lib/schema/dashboard";
	import { calculateStageDurations, type StageDuration } from "$lib/utils/stageMetrics";

	interface Props {
		statusHistory: StatusTransition[];
		jobStatus?: JobStatus;
		jobKickedAt?: string;
	}

	let { statusHistory, jobStatus = "completed", jobKickedAt = "" }: Props = $props();

	const stageDurations = $derived(
		calculateStageDurations(statusHistory, jobKickedAt, jobStatus)
			.filter((s) => s.durationSecs > 0)
	);

	// Find max duration for bar width calculation
	const maxDuration = $derived.by(() => {
		let max = 0;
		for (const s of stageDurations) {
			if (s.durationSecs > max) max = s.durationSecs;
		}
		return max || 1;
	});

	// Calculate total duration
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

<div class="space-y-2">
	<h4 class="text-sm font-semibold" style="color: var(--text-muted);">
		Stage Duration Breakdown
	</h4>

	{#if stageDurations.length === 0}
		<p class="text-xs" style="color: var(--text-muted);">No duration data available.</p>
	{:else}
		<div class="space-y-2">
			{#each stageDurations as stageDuration}
				<div class="flex items-center gap-2">
					<span
						class="w-20 text-xs font-medium truncate"
						style="color: var(--text-primary);"
					>
						{getStageLabel(stageDuration.stage)}
					</span>
					<div class="flex-1 h-4 bg-gray-100 rounded-full overflow-hidden">
						<div
							class="h-full bg-blue-500 rounded-full transition-all"
							style="width: {(stageDuration.durationSecs / maxDuration) * 100}%"
						></div>
					</div>
					<span
						class="w-14 text-xs text-right tabular-nums"
						style="color: var(--text-muted);"
					>
						{formatSeconds(stageDuration.durationSecs)}
					</span>
				</div>
			{/each}

			<!-- Total -->
			<div class="flex items-center gap-2 pt-2 border-t" style="border-color: var(--surface-border);">
				<span
					class="w-20 text-xs font-semibold"
					style="color: var(--text-primary);"
				>
					Total
				</span>
				<div class="flex-1"></div>
				<span
					class="w-14 text-xs font-semibold text-right tabular-nums"
					style="color: var(--text-primary);"
				>
					{formatSeconds(totalDuration)}
				</span>
			</div>
		</div>
	{/if}
</div>
