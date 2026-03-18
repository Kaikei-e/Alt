<script lang="ts">
import { RefreshCw } from "@lucide/svelte";

interface Props {
	pendingCount: number;
	isConnected: boolean;
	isFallback: boolean;
	onApply: () => void;
}

const { pendingCount, isConnected, isFallback, onApply }: Props = $props();

const indicatorColor = $derived(
	isFallback ? "bg-orange-400" : isConnected ? "bg-green-400" : "bg-gray-400",
);
</script>

{#if pendingCount > 0}
	<div class="w-full flex items-center gap-2 px-4 py-2 text-sm rounded-lg border border-[var(--accent-primary)]/30 bg-[var(--accent-primary)]/5">
		<span class="h-2 w-2 rounded-full {indicatorColor} flex-shrink-0" aria-hidden="true"></span>
		<button
			class="flex-1 flex items-center justify-center gap-2 text-[var(--accent-primary)] hover:text-[var(--accent-primary)]/80 transition-colors"
			onclick={onApply}
		>
			<RefreshCw class="h-3.5 w-3.5" />
			{pendingCount} {pendingCount === 1 ? 'item' : 'items'} updated
		</button>
	</div>
{/if}
