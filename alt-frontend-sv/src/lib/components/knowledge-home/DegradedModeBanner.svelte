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
		class="flex items-center gap-2 rounded-lg border border-amber-400/30 bg-amber-400/10 px-4 py-2 text-sm text-amber-200"
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
		class="flex items-center gap-2 rounded-lg border border-red-400/30 bg-red-400/10 px-4 py-2 text-sm text-red-200"
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
