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

const CHART_COLORS: Record<string, string> = {
	articles: "#1a1a1a",
	summarized: "#666666",
	feed_activity: "#999999",
};

interface Props {
	title: string;
	dataPoints: TrendDataPoint[];
	dataKey: "articles" | "summarized" | "feed_activity";
	loading?: boolean;
}

let { title, dataPoints, dataKey, loading = false }: Props = $props();

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

	if (chart) {
		chart.destroy();
	}

	const lineColor = CHART_COLORS[dataKey] ?? "#1a1a1a";
	const fillColor = `${lineColor}14`;

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
					borderColor: lineColor,
					backgroundColor: fillColor,
					fill: true,
					tension: 0.1,
					pointRadius: 2,
					pointHoverRadius: 4,
					borderWidth: 1.5,
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
					titleFont: {
						family: "'IBM Plex Mono', monospace",
						size: 11,
					},
					bodyFont: {
						family: "'IBM Plex Mono', monospace",
						size: 11,
					},
					backgroundColor: "#1a1a1a",
					cornerRadius: 0,
					padding: 8,
				},
			},
			scales: {
				x: {
					grid: {
						display: false,
					},
					border: {
						color: "#c8c8c8",
					},
					ticks: {
						font: {
							family: "'IBM Plex Mono', monospace",
							size: 10,
						},
						color: "#999999",
					},
				},
				y: {
					beginAtZero: true,
					border: {
						display: false,
					},
					grid: {
						color: "rgba(200, 200, 200, 0.4)",
					},
					ticks: {
						font: {
							family: "'IBM Plex Mono', monospace",
							size: 10,
						},
						color: "#999999",
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

<div class="chart-container">
	<span class="chart-label">{title.toUpperCase()}</span>

	{#if loading}
		<div class="chart-placeholder">
			<span class="loading-pulse"></span>
			<span class="loading-text">Loading&hellip;</span>
		</div>
	{:else if dataPoints.length === 0}
		<div class="chart-placeholder">
			<span class="empty-text">No data available</span>
		</div>
	{:else}
		<div class="chart-area">
			<canvas bind:this={canvas}></canvas>
		</div>
	{/if}
</div>

<style>
	.chart-container {
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
		padding: 1rem;
	}

	.chart-label {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--alt-ash);
		display: block;
		margin-bottom: 0.75rem;
	}

	.chart-area {
		height: 12rem;
	}

	.chart-placeholder {
		height: 12rem;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 0.5rem;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.empty-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	@keyframes pulse {
		0%,
		100% {
			opacity: 0.3;
		}
		50% {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse {
			animation: none;
			opacity: 1;
		}
	}
</style>
