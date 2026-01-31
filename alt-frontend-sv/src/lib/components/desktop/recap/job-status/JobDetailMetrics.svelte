<script lang="ts">
import type {
	RecentJobSummary,
	JobStats,
	StatusTransition,
} from "$lib/schema/dashboard";
import {
	calculateJobMetrics,
	getPerformanceLabel,
	formatDurationWithUnits,
} from "$lib/utils/stageMetrics";
import StageDurationBar from "./StageDurationBar.svelte";
import StatusTransitionTimeline from "./StatusTransitionTimeline.svelte";
import {
	Clock,
	TrendingUp,
	TrendingDown,
	Minus,
	BarChart3,
	History,
	Zap,
} from "@lucide/svelte";

interface Props {
	job: RecentJobSummary;
	stats?: JobStats;
}

let { job, stats }: Props = $props();

// Calculate metrics
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

function getPerformanceIcon() {
	if (!metrics.performanceRatio) return Minus;
	if (metrics.performanceRatio <= 0.8) return TrendingUp;
	if (metrics.performanceRatio > 1.2) return TrendingDown;
	return Minus;
}

function getPerformanceColorClass(
	color: "green" | "amber" | "red" | "gray",
): string {
	const colorMap = {
		green: "text-green-600 bg-green-50",
		amber: "text-amber-600 bg-amber-50",
		red: "text-red-600 bg-red-50",
		gray: "text-gray-500 bg-gray-50",
	};
	return colorMap[color];
}

// Calculate time delta from average
const timeDelta = $derived.by(() => {
	if (!stats?.avg_duration_secs || !metrics.totalDurationSecs) return null;
	const delta = metrics.totalDurationSecs - stats.avg_duration_secs;
	return delta;
});

const PerformanceIcon = $derived(getPerformanceIcon());
</script>

<div class="space-y-6">
	<!-- Performance Summary Cards -->
	<div class="grid grid-cols-2 sm:grid-cols-4 gap-3">
		<!-- Total Duration -->
		<div
			class="p-3 rounded-lg border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<div class="flex items-center gap-2 mb-1">
				<Clock class="w-4 h-4" style="color: var(--text-muted);" />
				<span class="text-xs font-medium" style="color: var(--text-muted);">
					Duration
				</span>
			</div>
			<p class="text-lg font-bold tabular-nums" style="color: var(--text-primary);">
				{formatDurationWithUnits(metrics.totalDurationSecs)}
			</p>
		</div>

		<!-- Performance Indicator -->
		<div
			class="p-3 rounded-lg border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<div class="flex items-center gap-2 mb-1">
				<Zap class="w-4 h-4" style="color: var(--text-muted);" />
				<span class="text-xs font-medium" style="color: var(--text-muted);">
					Performance
				</span>
			</div>
			<div class="flex items-center gap-2">
				<span
					class="inline-flex items-center gap-1 px-2 py-0.5 rounded-full text-sm font-medium {getPerformanceColorClass(performance.color)}"
				>
					<PerformanceIcon class="w-3.5 h-3.5" />
					{performance.label}
				</span>
			</div>
		</div>

		<!-- Comparison to Average -->
		<div
			class="p-3 rounded-lg border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<div class="flex items-center gap-2 mb-1">
				<BarChart3 class="w-4 h-4" style="color: var(--text-muted);" />
				<span class="text-xs font-medium" style="color: var(--text-muted);">
					vs Average
				</span>
			</div>
			{#if timeDelta !== null}
				<p
					class="text-lg font-bold tabular-nums {timeDelta > 0 ? 'text-amber-600' : timeDelta < 0 ? 'text-green-600' : ''}"
					style={timeDelta === 0 ? 'color: var(--text-primary);' : ''}
				>
					{timeDelta > 0 ? '+' : ''}{formatDurationWithUnits(Math.abs(timeDelta))}
				</p>
			{:else}
				<p class="text-lg font-bold" style="color: var(--text-muted);">-</p>
			{/if}
		</div>

		<!-- Stages Completed -->
		<div
			class="p-3 rounded-lg border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<div class="flex items-center gap-2 mb-1">
				<History class="w-4 h-4" style="color: var(--text-muted);" />
				<span class="text-xs font-medium" style="color: var(--text-muted);">
					Stages
				</span>
			</div>
			<p class="text-lg font-bold tabular-nums" style="color: var(--text-primary);">
				{metrics.stageDurations.filter((s) => s.status === "completed").length}/{metrics.stageDurations.length}
			</p>
		</div>
	</div>

	<!-- Stage Duration Breakdown -->
	<div>
		<h4
			class="text-sm font-semibold mb-3 flex items-center gap-2"
			style="color: var(--text-primary);"
		>
			<BarChart3 class="w-4 h-4" />
			Stage Duration Breakdown
		</h4>
		<div
			class="p-4 rounded-lg border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<StageDurationBar stageDurations={metrics.stageDurations} />
		</div>
	</div>

	<!-- Status History Timeline -->
	<div>
		<h4
			class="text-sm font-semibold mb-3 flex items-center gap-2"
			style="color: var(--text-primary);"
		>
			<History class="w-4 h-4" />
			Status History
		</h4>
		<div
			class="p-4 rounded-lg border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<StatusTransitionTimeline transitions={job.status_history} />
		</div>
	</div>
</div>
