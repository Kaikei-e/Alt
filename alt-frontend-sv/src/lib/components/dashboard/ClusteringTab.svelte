<script lang="ts">
	import { getMetrics } from "$lib/api/client/dashboard";
	import type { SystemMetric } from "$lib/schema/dashboard";

	interface Props {
		windowSeconds: number;
	}

	let { windowSeconds }: Props = $props();

	let metrics = $state<SystemMetric[]>([]);
	let loading = $state(true);
	let error = $state<string | null>(null);

	$effect(() => {
		loadData();
	});

	async function loadData() {
		loading = true;
		error = null;
		try {
			metrics = await getMetrics("clustering", windowSeconds, 500);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			console.error("Failed to load clustering metrics:", e);
		} finally {
			loading = false;
		}
	}

	function getMetricValue(metric: SystemMetric, key: string): number {
		const value = metric.metrics[key];
		if (typeof value === "number") return value;
		return 0;
	}
</script>

<div>
	<h2 class="text-2xl font-bold mb-4" style="color: var(--text-primary);">
		Clustering Metrics
	</h2>

	{#if loading}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			Loading...
		</div>
	{:else if error}
		<div class="p-8 text-center" style="color: var(--alt-error);">
			Error: {error}
		</div>
	{:else if metrics.length === 0}
		<div class="p-8 text-center" style="color: var(--text-muted);">
			No clustering metrics found.
		</div>
	{:else}
		{#if metrics[0]}
			{@const latest = metrics[0]}
			{@const dbcvScore = getMetricValue(latest, "dbcv_score")}
			{@const silhouetteScore = getMetricValue(latest, "silhouette_score")}
			{@const numClusters = getMetricValue(latest, "num_clusters")}
			{@const noiseRatio = getMetricValue(latest, "noise_ratio")}

			<div class="grid grid-cols-1 md:grid-cols-4 gap-4 mb-6">
				<div
					class="p-4 border"
					style="
						background: var(--surface-bg);
						border-color: var(--surface-border);
						box-shadow: var(--shadow-sm);
					"
				>
					<div class="text-sm mb-1" style="color: var(--text-muted);">
						DBCV Score
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{dbcvScore.toFixed(3)}
					</div>
				</div>
				<div
					class="p-4 border"
					style="
						background: var(--surface-bg);
						border-color: var(--surface-border);
						box-shadow: var(--shadow-sm);
					"
				>
					<div class="text-sm mb-1" style="color: var(--text-muted);">
						Silhouette Score
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{silhouetteScore.toFixed(3)}
					</div>
				</div>
				<div
					class="p-4 border"
					style="
						background: var(--surface-bg);
						border-color: var(--surface-border);
						box-shadow: var(--shadow-sm);
					"
				>
					<div class="text-sm mb-1" style="color: var(--text-muted);">
						Num Clusters
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{Math.round(numClusters)}
					</div>
				</div>
				<div
					class="p-4 border"
					style="
						background: var(--surface-bg);
						border-color: var(--surface-border);
						box-shadow: var(--shadow-sm);
					"
				>
					<div class="text-sm mb-1" style="color: var(--text-muted);">
						Noise Ratio
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{(noiseRatio * 100).toFixed(2)}%
					</div>
				</div>
			</div>
		{/if}

		<div
			class="border overflow-hidden"
			style="
				background: var(--surface-bg);
				border-color: var(--surface-border);
				box-shadow: var(--shadow-sm);
			"
		>
			<table class="w-full">
				<thead
					style="
						background: var(--surface-hover);
						border-bottom: 1px solid var(--surface-border);
					"
				>
					<tr>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Timestamp
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							DBCV
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Silhouette
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Clusters
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Noise %
						</th>
					</tr>
				</thead>
				<tbody style="border-top: 1px solid var(--surface-border);">
					{#each metrics.slice(0, 50) as metric}
						<tr
							style="
								border-bottom: 1px solid var(--surface-border);
								transition: background 0.2s;
							"
							onmouseenter={(e) => {
								e.currentTarget.style.background = "var(--surface-hover)";
							}}
							onmouseleave={(e) => {
								e.currentTarget.style.background = "transparent";
							}}
						>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{new Date(metric.timestamp).toLocaleString()}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{getMetricValue(metric, "dbcv_score").toFixed(3)}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{getMetricValue(metric, "silhouette_score").toFixed(3)}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{Math.round(getMetricValue(metric, "num_clusters"))}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{(getMetricValue(metric, "noise_ratio") * 100).toFixed(2)}%
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

