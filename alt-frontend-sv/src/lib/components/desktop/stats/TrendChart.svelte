<script lang="ts">
import { onMount, onDestroy } from "svelte";
import {
	Chart,
	LineController,
	LineElement,
	PointElement,
	LinearScale,
	CategoryScale,
	Title,
	Tooltip,
	Legend,
	Filler,
} from "chart.js";
import type { TrendDataPoint } from "$lib/schema/stats";

// Register Chart.js components
Chart.register(
	LineController,
	LineElement,
	PointElement,
	LinearScale,
	CategoryScale,
	Title,
	Tooltip,
	Legend,
	Filler,
);

interface Props {
	title: string;
	dataPoints: TrendDataPoint[];
	dataKey: "articles" | "summarized" | "feed_activity";
	color: string;
	loading?: boolean;
}

let { title, dataPoints, dataKey, color, loading = false }: Props = $props();

let canvas = $state<HTMLCanvasElement | undefined>(undefined);
let chart: Chart | null = null;

function formatTimestamp(timestamp: string, granularity: string): string {
	const date = new Date(timestamp);
	if (granularity === "hourly") {
		return date.toLocaleTimeString("ja-JP", {
			hour: "2-digit",
			minute: "2-digit",
		});
	}
	return date.toLocaleDateString("ja-JP", {
		month: "short",
		day: "numeric",
	});
}

function getDataValue(point: TrendDataPoint): number {
	return point[dataKey];
}

function createChart() {
	if (!canvas || dataPoints.length === 0) return;

	// Destroy existing chart if any
	if (chart) {
		chart.destroy();
	}

	const labels = dataPoints.map((p) =>
		formatTimestamp(p.timestamp, dataPoints.length > 7 ? "hourly" : "daily"),
	);
	const data = dataPoints.map(getDataValue);

	chart = new Chart(canvas, {
		type: "line",
		data: {
			labels,
			datasets: [
				{
					label: title,
					data,
					borderColor: color,
					backgroundColor: `${color}20`,
					fill: true,
					tension: 0.3,
					pointRadius: 3,
					pointHoverRadius: 5,
				},
			],
		},
		options: {
			responsive: true,
			maintainAspectRatio: false,
			plugins: {
				legend: {
					display: false,
				},
				tooltip: {
					mode: "index",
					intersect: false,
				},
			},
			scales: {
				x: {
					grid: {
						display: false,
					},
				},
				y: {
					beginAtZero: true,
					grid: {
						color: "rgba(0, 0, 0, 0.1)",
					},
				},
			},
		},
	});
}

onMount(() => {
	createChart();
});

onDestroy(() => {
	if (chart) {
		chart.destroy();
	}
});

// Reactively update chart when dataPoints change
$effect(() => {
	if (dataPoints && canvas) {
		createChart();
	}
});
</script>

<div class="border border-[var(--surface-border)] bg-white p-4">
	<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-3">
		{title}
	</h3>

	{#if loading}
		<div class="h-48 flex items-center justify-center">
			<div class="animate-pulse text-[var(--text-muted)]">Loading...</div>
		</div>
	{:else if dataPoints.length === 0}
		<div class="h-48 flex items-center justify-center">
			<div class="text-[var(--text-muted)]">No data available</div>
		</div>
	{:else}
		<div class="h-48">
			<canvas bind:this={canvas}></canvas>
		</div>
	{/if}
</div>
