<script lang="ts">
let {
	consumedPct,
	label,
}: {
	consumedPct: number;
	label: string;
} = $props();

const clampedPct = $derived(Math.max(0, Math.min(100, consumedPct)));

const barColor = $derived(
	clampedPct > 80
		? "var(--accent-red, #ef4444)"
		: clampedPct > 50
			? "var(--accent-amber, #f59e0b)"
			: "var(--accent-green, #22c55e)",
);
</script>

<div class="flex flex-col gap-1.5">
	<div class="flex items-center justify-between text-xs">
		<span style="color: var(--text-secondary);">{label}</span>
		<span class="font-mono font-bold" style="color: {barColor};">
			{clampedPct.toFixed(1)}%
		</span>
	</div>
	<div
		class="h-2 w-full overflow-hidden rounded-full"
		style="background: var(--surface-border, #e5e7eb);"
	>
		<div
			class="h-full rounded-full transition-all"
			style="width: {clampedPct}%; background: {barColor};"
		></div>
	</div>
</div>
