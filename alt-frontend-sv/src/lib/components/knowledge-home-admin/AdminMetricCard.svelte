<script lang="ts">
import type { Snippet } from "svelte";

let {
	label,
	value,
	status = "neutral",
	icon,
}: {
	label: string;
	value: string | number;
	status?: "ok" | "warning" | "error" | "neutral";
	icon?: Snippet;
} = $props();

const statusColors: Record<string, string> = {
	ok: "var(--accent-green, #22c55e)",
	warning: "var(--accent-amber, #f59e0b)",
	error: "var(--accent-red, #ef4444)",
	neutral: "var(--text-secondary, #6b7280)",
};
</script>

<div
	class="rounded-lg border-2 p-4 flex flex-col gap-1"
	style="background: var(--surface-bg); border-color: var(--border-primary);"
>
	<div class="flex items-center gap-2 text-xs" style="color: var(--text-secondary);">
		{#if icon}
			{@render icon()}
		{/if}
		<span>{label}</span>
	</div>
	<div class="text-2xl font-bold" style="color: {statusColors[status]};">
		{value}
	</div>
</div>
