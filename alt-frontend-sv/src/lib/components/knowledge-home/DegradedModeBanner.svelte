<script lang="ts">
import { AlertTriangle, ShieldAlert } from "@lucide/svelte";

let {
	serviceQuality = "full",
}: {
	serviceQuality?: "full" | "degraded" | "fallback" | string;
} = $props();
</script>

{#if serviceQuality === "degraded"}
	<div
		class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm"
		style="background: var(--warning-bg, #fef3cd); border-color: var(--warning-border, #ffc107); color: var(--warning-text, #856404);"
	>
		<AlertTriangle size={16} />
		<span>Some data sources are temporarily unavailable. Showing partial results.</span>
	</div>
{:else if serviceQuality === "fallback"}
	<div
		class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm"
		style="background: var(--error-bg, #fee2e2); border-color: var(--error-border, #ef4444); color: var(--error-text, #991b1b);"
	>
		<ShieldAlert size={16} />
		<span>Service is running in fallback mode. Showing cached snapshot. Some features may be unavailable.</span>
	</div>
{/if}
