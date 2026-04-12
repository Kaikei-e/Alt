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
		class="quality-banner quality-banner--degraded flex items-center gap-2 border px-4 py-2 text-sm"
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
				class="flex-shrink-0 p-0.5 transition-colors hover:bg-white/10"
				title="Dismiss"
				onclick={onDismiss}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
{:else if serviceQuality === "fallback"}
	<div
		class="quality-banner quality-banner--fallback flex items-center gap-2 border px-4 py-2 text-sm"
		role="alert"
	>
		<ShieldAlert size={16} class="flex-shrink-0" />
		<span class="flex-1"
			>Service is running in fallback mode. Some sections may be unavailable
			or stale.</span
		>
		{#if onDismiss}
			<button
				type="button"
				class="flex-shrink-0 p-0.5 transition-colors hover:bg-white/10"
				title="Dismiss"
				onclick={onDismiss}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
{/if}

<style>
	.quality-banner--degraded {
		color: var(--alt-warning);
		background: color-mix(in srgb, var(--alt-warning) 6%, var(--surface-bg));
		border-color: color-mix(in srgb, var(--alt-warning) 30%, transparent);
	}
	.quality-banner--fallback {
		color: var(--alt-error);
		background: color-mix(in srgb, var(--alt-error) 6%, var(--surface-bg));
		border-color: color-mix(in srgb, var(--alt-error) 30%, transparent);
	}
</style>
