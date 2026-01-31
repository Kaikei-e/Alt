<script lang="ts">
import { getStageLabel, type PipelineStage } from "$lib/schema/dashboard";
import {
	type StageDuration,
	formatDurationWithUnits,
	calculateBarWidth,
} from "$lib/utils/stageMetrics";
import { Check, Loader2, Clock, Circle } from "@lucide/svelte";

interface Props {
	stageDurations: StageDuration[];
	/** Average duration per stage for comparison (optional) */
	avgStageDurations?: Map<PipelineStage, number>;
	/** Show compact view (single line per stage) */
	compact?: boolean;
}

let { stageDurations, avgStageDurations, compact = false }: Props = $props();

// Calculate max duration for relative bar sizing
const maxDuration = $derived(
	Math.max(...stageDurations.map((s) => s.durationSecs), 1),
);

// Calculate total duration
const totalDuration = $derived(
	stageDurations.reduce((sum, s) => sum + s.durationSecs, 0),
);

function getStatusIcon(status: StageDuration["status"]) {
	switch (status) {
		case "completed":
			return Check;
		case "running":
			return Loader2;
		case "skipped":
			return Circle;
		default:
			return Clock;
	}
}

function getBarColor(
	status: StageDuration["status"],
	durationSecs: number,
	avgDuration?: number,
): string {
	if (status === "pending") return "bg-gray-200";
	if (status === "running") return "bg-blue-400";
	if (status === "skipped") return "bg-gray-300";

	// Performance-based coloring for completed stages
	if (avgDuration && avgDuration > 0) {
		const ratio = durationSecs / avgDuration;
		if (ratio <= 0.8) return "bg-green-500";
		if (ratio <= 1.2) return "bg-blue-500";
		return "bg-amber-500";
	}

	return "bg-green-500";
}

function getTextColor(status: StageDuration["status"]): string {
	switch (status) {
		case "completed":
			return "text-green-700";
		case "running":
			return "text-blue-700";
		case "skipped":
			return "text-gray-400";
		default:
			return "text-gray-500";
	}
}
</script>

<div class="space-y-2" role="list" aria-label="Stage duration breakdown">
	{#each stageDurations as stage}
		{@const barWidth = calculateBarWidth(stage.durationSecs, maxDuration)}
		{@const Icon = getStatusIcon(stage.status)}
		{@const avgDuration = avgStageDurations?.get(stage.stage)}
		{@const barColor = getBarColor(stage.status, stage.durationSecs, avgDuration)}
		{@const textColor = getTextColor(stage.status)}

		<div
			class="flex items-center gap-2 {compact ? 'py-0.5' : 'py-1'}"
			role="listitem"
		>
			<!-- Stage name and icon -->
			<div class="flex items-center gap-1.5 w-24 flex-shrink-0">
				<Icon
					class="w-3.5 h-3.5 {textColor} {stage.status === 'running' ? 'animate-spin' : ''}"
				/>
				<span
					class="text-xs font-medium truncate {textColor}"
					title={getStageLabel(stage.stage)}
				>
					{getStageLabel(stage.stage)}
				</span>
			</div>

			<!-- Progress bar container -->
			<div class="flex-1 h-4 rounded-sm overflow-hidden bg-gray-100" role="progressbar" aria-valuenow={stage.durationSecs} aria-valuemax={maxDuration}>
				{#if barWidth > 0}
					<div
						class="h-full rounded-sm transition-all duration-300 {barColor}"
						style="width: {barWidth}%"
					></div>
				{/if}
			</div>

			<!-- Duration value -->
			<div class="w-14 text-right flex-shrink-0">
				<span
					class="text-xs tabular-nums {stage.durationSecs > 0 ? 'font-medium' : ''}"
					style="color: var(--text-primary);"
				>
					{formatDurationWithUnits(stage.durationSecs)}
				</span>
			</div>
		</div>
	{/each}

	<!-- Total row -->
	<div
		class="flex items-center gap-2 pt-2 mt-2 border-t"
		style="border-color: var(--surface-border);"
	>
		<div class="w-24 flex-shrink-0">
			<span class="text-xs font-semibold" style="color: var(--text-primary);">
				Total
			</span>
		</div>
		<div class="flex-1"></div>
		<div class="w-14 text-right flex-shrink-0">
			<span
				class="text-xs font-bold tabular-nums"
				style="color: var(--text-primary);"
			>
				{formatDurationWithUnits(totalDuration)}
			</span>
		</div>
	</div>
</div>
