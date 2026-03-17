<script lang="ts">
import { RefreshCw } from "@lucide/svelte";

interface Props {
	pendingCount: number;
	onApply: () => void;
}

const { pendingCount, onApply }: Props = $props();
</script>

{#if pendingCount > 0}
	<button
		class="w-full flex items-center justify-center gap-2 px-4 py-2 text-sm rounded-lg border border-[var(--accent-primary)]/30 bg-[var(--accent-primary)]/5 text-[var(--accent-primary)] hover:bg-[var(--accent-primary)]/10 transition-colors"
		onclick={onApply}
	>
		<RefreshCw class="h-3.5 w-3.5" />
		{pendingCount} {pendingCount === 1 ? 'item' : 'items'} updated
	</button>
{/if}
