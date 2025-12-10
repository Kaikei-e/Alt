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
			metrics = await getMetrics("classification", windowSeconds, 500);
		} catch (e) {
			error = e instanceof Error ? e.message : String(e);
			console.error("Failed to load classification metrics:", e);
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
		Classification Metrics
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
			No classification metrics found.
		</div>
	{:else}
		{#if metrics[0]}
			{@const latest = metrics[0]}
			{@const accuracy = getMetricValue(latest, "accuracy")}
			{@const macroF1 = getMetricValue(latest, "macro_f1")}
			{@const hammingLoss = getMetricValue(latest, "hamming_loss")}

			<div class="grid grid-cols-1 md:grid-cols-3 gap-4 mb-6">
				<div
					class="p-4 border"
					style="
						background: var(--surface-bg);
						border-color: var(--surface-border);
						box-shadow: var(--shadow-sm);
					"
				>
					<div class="text-sm mb-1" style="color: var(--text-muted);">
						Latest Accuracy
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{(accuracy * 100).toFixed(2)}%
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
						Latest Macro F1
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{macroF1.toFixed(2)}
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
						Hamming Loss
					</div>
					<div class="text-2xl font-bold" style="color: var(--text-primary);">
						{hammingLoss.toFixed(4)}
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
							Accuracy
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Macro F1
						</th>
						<th
							class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider"
							style="color: var(--text-muted);"
						>
							Hamming Loss
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
								{(getMetricValue(metric, "accuracy") * 100).toFixed(2)}%
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{getMetricValue(metric, "macro_f1").toFixed(2)}
							</td>
							<td
								class="px-6 py-4 whitespace-nowrap text-sm"
								style="color: var(--text-primary);"
							>
								{getMetricValue(metric, "hamming_loss").toFixed(4)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
		</div>
	{/if}
</div>

