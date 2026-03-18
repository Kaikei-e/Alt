<script lang="ts">
import type { SLOStatusData } from "$lib/connect/knowledge_home_admin";
import AdminMetricCard from "./AdminMetricCard.svelte";
import ErrorBudgetGauge from "./ErrorBudgetGauge.svelte";

let { sloStatus }: { sloStatus: SLOStatusData | null } = $props();

const statusBadgeColor = (status: string) => {
	switch (status) {
		case "meeting":
			return "var(--accent-green, #22c55e)";
		case "burning":
			return "var(--accent-amber, #f59e0b)";
		case "breached":
			return "var(--accent-red, #ef4444)";
		default:
			return "var(--text-secondary)";
	}
};

const overallHealthColor = (health: string) => {
	switch (health) {
		case "healthy":
			return "ok" as const;
		case "at_risk":
			return "warning" as const;
		case "breaching":
			return "error" as const;
		default:
			return "neutral" as const;
	}
};

const formatSliName = (name: string) =>
	name
		.split("_")
		.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
		.join(" ");
</script>

<div class="flex flex-col gap-4">
	<div class="flex items-center justify-between">
		<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
			SLO Status
		</h3>
		{#if sloStatus}
			<span class="text-xs" style="color: var(--text-secondary);">
				Window: {sloStatus.errorBudgetWindowDays}d | Computed: {sloStatus.computedAt
					? new Date(sloStatus.computedAt).toLocaleTimeString("ja-JP")
					: "--"}
			</span>
		{/if}
	</div>

	{#if !sloStatus}
		<p class="text-xs" style="color: var(--text-secondary);">Loading SLO data...</p>
	{:else}
		<AdminMetricCard
			label="Overall Health"
			value={sloStatus.overallHealth}
			status={overallHealthColor(sloStatus.overallHealth)}
		/>

		<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
			{#each sloStatus.slis as sli (sli.name)}
				<div
					class="flex flex-col gap-2 rounded-lg border-2 p-3"
					style="background: var(--surface-bg); border-color: var(--border-primary);"
				>
					<div class="flex items-center justify-between">
						<span class="text-xs font-medium" style="color: var(--text-primary);">
							{formatSliName(sli.name)}
						</span>
						<span
							class="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
							style="background: {statusBadgeColor(sli.status)};"
						>
							{sli.status}
						</span>
					</div>
					<div class="flex items-baseline gap-1">
						<span class="text-lg font-bold font-mono" style="color: var(--text-primary);">
							{sli.currentValue.toFixed(2)}
						</span>
						<span class="text-xs" style="color: var(--text-secondary);">
							/ {sli.targetValue.toFixed(2)} {sli.unit}
						</span>
					</div>
					<ErrorBudgetGauge
						consumedPct={sli.errorBudgetConsumedPct}
						label="Error Budget"
					/>
				</div>
			{/each}
		</div>
	{/if}
</div>
