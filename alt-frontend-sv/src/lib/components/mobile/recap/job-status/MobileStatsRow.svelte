<script lang="ts">
import { Activity, CheckCircle, XCircle, Clock } from "@lucide/svelte";

interface Props {
	successRate: string;
	avgDuration: string;
	totalJobs: number;
	runningJobs: number;
	failedJobs: number;
}

let { successRate, avgDuration, totalJobs, runningJobs, failedJobs }: Props =
	$props();

const stats = $derived([
	{
		title: "Success Rate",
		value: successRate,
		subtitle: "24h",
		icon: CheckCircle,
		color: "text-green-600",
	},
	{
		title: "Avg Duration",
		value: avgDuration,
		subtitle: "Per job",
		icon: Clock,
		color: "text-blue-600",
	},
	{
		title: "Jobs Today",
		value: totalJobs,
		subtitle: `${runningJobs} running`,
		icon: Activity,
		color: "text-indigo-600",
	},
	{
		title: "Failed",
		value: failedJobs,
		subtitle: "24h",
		icon: XCircle,
		color: "text-red-600",
	},
]);
</script>

<div
	class="flex gap-3 overflow-x-auto px-4 pb-4 scrollbar-hide"
	data-testid="mobile-stats-row"
	style="overflow-x: auto;"
>
	{#each stats as stat}
		<div
			class="flex-shrink-0 w-[120px] p-3 rounded-xl border"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<div class="flex items-center gap-2 mb-2">
				<stat.icon class="w-4 h-4 {stat.color}" />
			</div>
			<p
				class="text-lg font-bold tabular-nums"
				style="color: var(--text-primary);"
			>
				{stat.value}
			</p>
			<p class="text-xs" style="color: var(--text-muted);">
				{stat.title}
			</p>
			{#if stat.subtitle}
				<p class="text-xs" style="color: var(--text-muted);">
					{stat.subtitle}
				</p>
			{/if}
		</div>
	{/each}
</div>

<style>
	.scrollbar-hide {
		-ms-overflow-style: none;
		scrollbar-width: none;
	}
	.scrollbar-hide::-webkit-scrollbar {
		display: none;
	}
</style>
