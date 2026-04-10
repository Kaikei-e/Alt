<script lang="ts">
import type { TagTrailTag } from "$lib/schema/tagTrail";
import TagChip from "./TagChip.svelte";

interface Props {
	tags: TagTrailTag[];
	selectedTagId?: string;
	onTagClick: (tag: TagTrailTag) => void;
}

const { tags, selectedTagId, onTagClick }: Props = $props();
</script>

<div
	class="flex gap-1.5 overflow-x-auto py-1 px-3 scrollbar-hide"
	style="-webkit-overflow-scrolling: touch;"
	data-testid="tag-chip-list"
>
	{#each tags as tag, index (tag.id)}
		<div class="chip-enter" style="--stagger: {index};">
			<TagChip
				{tag}
				isSelected={tag.id === selectedTagId}
				onclick={onTagClick}
			/>
		</div>
	{/each}
	{#if tags.length === 0}
		<p class="empty-hint">No tags available</p>
	{/if}
</div>

<style>
	.scrollbar-hide {
		-ms-overflow-style: none;
		scrollbar-width: none;
	}
	.scrollbar-hide::-webkit-scrollbar {
		display: none;
	}

	.chip-enter {
		opacity: 0;
		animation: chip-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 40ms);
	}
	@keyframes chip-in {
		to { opacity: 1; }
	}

	.empty-hint {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash, #999);
		margin: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.chip-enter {
			animation: none;
			opacity: 1;
		}
	}
</style>
