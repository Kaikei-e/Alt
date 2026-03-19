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
		class="flex items-center gap-2 rounded-xl border border-[var(--badge-amber-border)] bg-[var(--badge-amber-bg)] px-4 py-2 text-sm text-[var(--badge-amber-text)]"
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
				class="flex-shrink-0 rounded p-0.5 transition-colors hover:bg-white/10"
				title="Dismiss"
				onclick={onDismiss}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
{:else if serviceQuality === "fallback"}
	<div
		class="flex items-center gap-2 rounded-xl border border-[var(--badge-orange-border)] bg-[var(--badge-orange-bg)] px-4 py-2 text-sm text-[var(--badge-orange-text)]"
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
				class="flex-shrink-0 rounded p-0.5 transition-colors hover:bg-white/10"
				title="Dismiss"
				onclick={onDismiss}
			>
				<X size={14} />
			</button>
		{/if}
	</div>
{/if}
