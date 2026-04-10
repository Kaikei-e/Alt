<script lang="ts">
import type { TagTrailHop } from "$lib/schema/tagTrail";
import { ChevronRight } from "@lucide/svelte";

interface Props {
	hops: TagTrailHop[];
	onHopClick: (index: number) => void;
}

const { hops, onHopClick }: Props = $props();
</script>

<nav
	class="flex items-center gap-1 overflow-x-auto py-2 px-4 scrollbar-hide"
	style="-webkit-overflow-scrolling: touch;"
	data-testid="tag-trail-breadcrumb"
	data-role="trail-breadcrumb"
	aria-label="Trail breadcrumb"
>
	<button
		type="button"
		class="trail-hop"
		onclick={() => onHopClick(-1)}
		aria-label="Go to start"
	>
		Start
	</button>

	{#each hops as hop, index (index)}
		<ChevronRight size={14} class="flex-shrink-0" style="color: var(--surface-border, #c8c8c8);" />
		<button
			type="button"
			class="trail-hop"
			class:trail-hop--current={index === hops.length - 1}
			onclick={() => onHopClick(index)}
			aria-label="Go to {hop.name}"
		>
			{hop.name}
		</button>
	{/each}
</nav>

<style>
	.scrollbar-hide {
		-ms-overflow-style: none;
		scrollbar-width: none;
	}
	.scrollbar-hide::-webkit-scrollbar {
		display: none;
	}

	.trail-hop {
		display: inline-flex;
		align-items: center;
		min-height: 44px;
		padding: 0 0.5rem;
		flex-shrink: 0;
		max-width: 120px;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;

		color: var(--alt-ash, #999);
		background: transparent;
		border: none;
		cursor: pointer;
		transition: color 0.15s;
	}

	.trail-hop:hover {
		color: var(--alt-charcoal, #1a1a1a);
	}

	.trail-hop--current {
		color: var(--alt-charcoal, #1a1a1a);
		font-weight: 700;
	}
</style>
