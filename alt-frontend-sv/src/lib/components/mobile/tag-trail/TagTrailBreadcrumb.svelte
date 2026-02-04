<script lang="ts">
import type { TagTrailHop } from "$lib/schema/tagTrail";
import { ChevronRight, Home } from "@lucide/svelte";

interface Props {
	hops: TagTrailHop[];
	onHopClick: (index: number) => void;
}

const { hops, onHopClick }: Props = $props();
</script>

<div
	class="flex items-center gap-1 overflow-x-auto py-2 px-4 text-sm scrollbar-hide"
	style="-webkit-overflow-scrolling: touch;"
	data-testid="tag-trail-breadcrumb"
>
	<button
		type="button"
		class="flex items-center gap-1 min-h-[44px] px-2 rounded-lg hover:bg-muted active:scale-95 transition-all flex-shrink-0"
		onclick={() => onHopClick(-1)}
		aria-label="Go to start"
	>
		<Home size={16} style="color: var(--alt-primary);" />
		<span style="color: var(--text-secondary);">Start</span>
	</button>

	{#each hops as hop, index (index)}
		<ChevronRight size={16} class="flex-shrink-0" style="color: var(--text-secondary);" />
		<button
			type="button"
			class="min-h-[44px] px-2 rounded-lg hover:bg-muted active:scale-95 transition-all flex-shrink-0 truncate max-w-[120px]"
			style="color: {index === hops.length - 1 ? 'var(--alt-primary)' : 'var(--text-secondary)'};"
			onclick={() => onHopClick(index)}
			aria-label="Go to {hop.name}"
		>
			{hop.name}
		</button>
	{/each}
</div>

<style>
	.scrollbar-hide {
		-ms-overflow-style: none;
		scrollbar-width: none;
	}
	.scrollbar-hide::-webkit-scrollbar {
		display: none;
	}
</style>
