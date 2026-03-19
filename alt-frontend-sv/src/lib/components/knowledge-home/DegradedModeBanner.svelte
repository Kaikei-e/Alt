<script lang="ts">
import { AlertTriangle, ShieldAlert, X } from "@lucide/svelte";

interface Props {
	serviceQuality?: "full" | "degraded" | "fallback" | string;
	onDismiss?: () => void;
}

const { serviceQuality = "full", onDismiss }: Props = $props();
</script>

{#if serviceQuality === "degraded"}
	<div
		class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm"
		style="background: var(--warning-bg, #fef3cd); border-color: var(--warning-border, #ffc107); color: var(--warning-text, #856404);"
		role="alert"
	>
		<AlertTriangle size={16} class="flex-shrink-0" />
		<span class="flex-1"
			>Some data sources are temporarily unavailable. Showing partial
			results.</span
		>
		{#if onDismiss}
			<button
				type="button"
				class="flex-shrink-0 rounded p-0.5 transition-colors hover:bg-black/10"
				title="Dismiss"
				onclick={onDismiss}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
{:else if serviceQuality === "fallback"}
	<div
		class="flex items-center gap-2 rounded-lg border px-4 py-2 text-sm"
		style="background: var(--error-bg, #fee2e2); border-color: var(--error-border, #ef4444); color: var(--error-text, #991b1b);"
		role="alert"
	>
		<ShieldAlert size={16} class="flex-shrink-0" />
		<span class="flex-1"
			>Service is running in fallback mode. Showing cached snapshot. Some
			features may be unavailable.</span
		>
		{#if onDismiss}
			<button
				type="button"
				class="flex-shrink-0 rounded p-0.5 transition-colors hover:bg-black/10"
				title="Dismiss"
				onclick={onDismiss}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
{/if}
