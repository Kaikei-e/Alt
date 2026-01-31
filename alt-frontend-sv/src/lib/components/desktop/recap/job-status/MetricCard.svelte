<script lang="ts">
import type { Component } from "svelte";

interface Props {
	title: string;
	value: string | number;
	trend?: number | null;
	icon: Component;
	subtitle?: string;
}

let { title, value, trend = null, icon: Icon, subtitle }: Props = $props();

const trendClass = $derived(() => {
	if (!trend) return "";
	return trend > 0 ? "text-green-600" : "text-red-600";
});

const trendArrow = $derived(() => {
	if (!trend) return "";
	return trend > 0 ? "+" : "";
});
</script>

<div
	class="p-6 border rounded-lg"
	style="background: var(--surface-bg); border-color: var(--surface-border);"
>
	<div class="flex items-center gap-3 mb-4">
		<div
			class="w-10 h-10 flex items-center justify-center rounded-lg"
			style="background: var(--alt-primary-10, rgba(59, 130, 246, 0.1));"
		>
			<Icon class="h-5 w-5" style="color: var(--alt-primary, #3b82f6);" />
		</div>
		<h3
			class="text-sm font-semibold uppercase tracking-wider"
			style="color: var(--text-muted);"
		>
			{title}
		</h3>
	</div>
	<p class="text-3xl font-bold tabular-nums" style="color: var(--text-primary);">
		{value}
	</p>
	{#if subtitle}
		<p class="text-sm mt-1" style="color: var(--text-muted);">
			{subtitle}
		</p>
	{/if}
	{#if trend !== null}
		<p class="text-sm mt-1 {trendClass()}">
			{trendArrow()}{Math.abs(trend).toFixed(1)}%
		</p>
	{/if}
</div>
