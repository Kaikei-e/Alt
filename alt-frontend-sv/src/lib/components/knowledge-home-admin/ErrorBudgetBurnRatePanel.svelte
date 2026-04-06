<script lang="ts">
let {
	slis,
}: {
	slis: {
		name: string;
		currentValue: number;
		targetValue: number;
		errorBudgetConsumedPct: number;
		status: string;
	}[];
} = $props();

const formatSliName = (name: string) =>
	name
		.split("_")
		.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
		.join(" ");

const barColor = (pct: number) => {
	if (pct >= 80) return "var(--accent-red, #ef4444)";
	if (pct >= 50) return "var(--accent-amber, #f59e0b)";
	return "var(--accent-green, #22c55e)";
};
</script>

<div class="flex flex-col gap-4">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Error Budget Burn Rate
	</h3>

	{#if slis.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No SLI data available.</p>
	{:else}
		<div class="flex flex-col gap-3">
			{#each slis as sli (sli.name)}
				{@const clamped = Math.max(0, Math.min(100, sli.errorBudgetConsumedPct))}
				{@const color = barColor(sli.errorBudgetConsumedPct)}
				<div
					class="flex flex-col gap-1.5 rounded-lg border-2 px-4 py-3"
					style="background: var(--surface-bg); border-color: var(--border-primary);"
				>
					<div class="flex items-center justify-between text-xs">
						<span class="font-medium" style="color: var(--text-primary);">
							{formatSliName(sli.name)}
						</span>
						<span class="font-mono font-bold" style="color: {color};">
							{clamped.toFixed(1)}%
						</span>
					</div>
					<div
						class="h-2 w-full overflow-hidden rounded-full"
						style="background: var(--surface-border, #e5e7eb);"
					>
						<div
							class="h-full rounded-full transition-all"
							style="width: {clamped}%; background: {color};"
						></div>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
