<script lang="ts">
import { AlertCircle, RefreshCw, WifiOff } from "@lucide/svelte";

interface Props {
	pendingCount: number;
	isConnected: boolean;
	isFallback: boolean;
	onApply: () => void;
}

const { pendingCount, isConnected, isFallback, onApply }: Props = $props();

const indicatorColor = $derived(
	isFallback ? "bg-[var(--badge-orange-text)]" : isConnected ? "bg-[var(--badge-green-text)]" : "bg-[var(--badge-gray-text)]",
);
const indicatorPulse = $derived(isConnected && !isFallback ? "animate-pulse" : "");
</script>

{#if pendingCount > 0}
	<div class="w-full flex items-center gap-2 px-4 py-2 text-sm rounded-lg border border-[var(--accent-primary)]/30 bg-[var(--accent-primary)]/5">
		<span class="h-2.5 w-2.5 rounded-full {indicatorColor} {indicatorPulse} flex-shrink-0" aria-hidden="true"></span>
		<button
			class="flex-1 flex items-center justify-center gap-2 text-[var(--accent-primary)] hover:text-[var(--accent-primary)]/80 transition-colors"
			onclick={onApply}
		>
			<RefreshCw class="h-3.5 w-3.5" />
			{pendingCount} {pendingCount === 1 ? 'item' : 'items'} updated
		</button>
	</div>
{:else if isFallback}
	<div class="w-full flex items-center gap-2 px-4 py-2 text-sm rounded-lg border border-[var(--badge-orange-border)] bg-[var(--badge-orange-bg)]">
		<AlertCircle class="h-3.5 w-3.5 text-[var(--badge-orange-text)] flex-shrink-0" />
		<span class="text-[var(--text-secondary)]">Live updates unavailable</span>
	</div>
{:else if !isConnected}
	<div class="w-full flex items-center gap-2 px-4 py-2 text-sm rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)]">
		<WifiOff class="h-3.5 w-3.5 text-[var(--text-tertiary)] flex-shrink-0" />
		<span class="text-[var(--text-tertiary)]">Reconnecting...</span>
	</div>
{/if}
