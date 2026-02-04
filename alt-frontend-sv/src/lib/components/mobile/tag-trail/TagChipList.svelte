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
	{#each tags as tag (tag.id)}
		<TagChip
			{tag}
			isSelected={tag.id === selectedTagId}
			onclick={onTagClick}
		/>
	{/each}
	{#if tags.length === 0}
		<p class="text-sm" style="color: var(--text-secondary);">No tags available</p>
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
</style>
