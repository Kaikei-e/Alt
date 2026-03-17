<script lang="ts">
let {
	funnel,
}: {
	funnel: { label: string; value: number }[];
} = $props();

const maxValue = $derived(
	funnel.length > 0 ? Math.max(...funnel.map((f) => f.value)) : 1,
);

const funnelColors = [
	"var(--accent-blue, #3b82f6)",
	"var(--accent-teal, #14b8a6)",
	"var(--accent-green, #22c55e)",
	"var(--accent-amber, #f59e0b)",
];
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Interaction Funnel
	</h3>

	{#if funnel.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No data available.</p>
	{:else}
		<div class="flex flex-col gap-2">
			{#each funnel as step, i}
				<div class="flex items-center gap-2 text-xs">
					<span class="w-20 text-right" style="color: var(--text-secondary);">
						{step.label}
					</span>
					<div
						class="h-5 rounded flex items-center px-2 text-white text-xs font-medium"
						style="width: {maxValue > 0 ? (step.value / maxValue) * 100 : 0}%; min-width: 24px; background: {funnelColors[i % funnelColors.length]};"
					>
						{step.value}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
