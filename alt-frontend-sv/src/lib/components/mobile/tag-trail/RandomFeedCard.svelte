<script lang="ts">
import type { TagTrailTag } from "$lib/schema/tagTrail";
import { SquareArrowOutUpRight, RefreshCw } from "@lucide/svelte";
import TagChipList from "./TagChipList.svelte";

interface FeedData {
	id: string;
	url: string;
	title?: string;
	description?: string;
}

interface Props {
	feed: FeedData;
	tags: TagTrailTag[];
	isLoadingTags: boolean;
	onTagClick: (tag: TagTrailTag) => void;
	onRefresh: () => void;
}

const { feed, tags, isLoadingTags, onTagClick, onRefresh }: Props = $props();
</script>

<div
	class="mx-4 p-[2px] rounded-[18px] border-2 transition-transform duration-300 ease-in-out"
	style="border-color: var(--surface-border);"
	data-testid="random-feed-card"
>
	<div
		class="glass w-full p-4 rounded-2xl"
		style="background: var(--surface-bg);"
	>
		<div class="flex flex-col gap-3">
			<!-- Header with title and refresh button -->
			<div class="flex justify-between items-start gap-2">
				<div class="flex items-center gap-2 flex-1 min-w-0">
					<div class="flex items-center justify-center w-6 h-6 flex-shrink-0">
						<SquareArrowOutUpRight size={16} style="color: var(--alt-primary);" />
					</div>
					<a
						href={feed.url}
						target="_blank"
						rel="noopener noreferrer"
						class="text-sm font-semibold hover:underline leading-tight break-words flex-1 min-w-0 truncate"
						style="color: var(--accent-primary);"
						aria-label="Open {feed.title || feed.url} in new tab"
					>
						{feed.title || feed.url}
					</a>
				</div>
				<button
					type="button"
					class="min-h-[44px] min-w-[44px] flex items-center justify-center rounded-full hover:bg-muted active:scale-95 transition-all"
					onclick={onRefresh}
					aria-label="Get another random feed"
				>
					<RefreshCw size={18} style="color: var(--text-secondary);" />
				</button>
			</div>

			<!-- Description -->
			{#if feed.description}
				<p
					class="text-xs leading-normal line-clamp-2"
					style="color: var(--text-secondary);"
				>
					{feed.description}
				</p>
			{/if}

			<!-- Tags section -->
			<div class="border-t pt-3" style="border-color: var(--surface-border);">
				<p class="text-xs font-medium mb-2 px-4" style="color: var(--text-secondary);">
					Tap a tag to explore:
				</p>
				{#if isLoadingTags}
					<div class="flex flex-col gap-2 px-4">
						<div class="flex gap-2">
							{#each [1, 2, 3] as i}
								<div
									class="h-[36px] w-[80px] rounded-full animate-pulse"
									style="background: var(--muted);"
								></div>
							{/each}
						</div>
						<p class="text-xs" style="color: var(--text-tertiary);">Generating tags...</p>
					</div>
				{:else}
					<TagChipList {tags} onTagClick={onTagClick} />
				{/if}
			</div>
		</div>
	</div>
</div>
