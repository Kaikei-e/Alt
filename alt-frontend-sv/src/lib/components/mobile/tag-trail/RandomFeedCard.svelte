<script lang="ts">
import type { TagTrailTag } from "$lib/schema/tagTrail";
import { RefreshCw } from "@lucide/svelte";
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

<div class="featured-section" data-testid="random-feed-card" data-role="featured-section">
	<div class="flex flex-col gap-3 p-4">
		<!-- Header with label, title and refresh -->
		<div class="flex justify-between items-start gap-2">
			<div class="flex-1 min-w-0">
				<span class="section-label">Today's Featured Section</span>
				<a
					href={feed.url}
					target="_blank"
					rel="noopener noreferrer"
					class="feed-title"
					aria-label="Open {feed.title || feed.url} in new tab"
				>
					{feed.title || feed.url}
				</a>
			</div>
			<button
				type="button"
				class="refresh-btn"
				onclick={onRefresh}
				aria-label="Get another random feed"
			>
				<RefreshCw size={16} />
			</button>
		</div>

		<!-- Description -->
		{#if feed.description}
			<p class="feed-desc">{feed.description}</p>
		{/if}

		<!-- Tags section -->
		<div class="tags-section">
			<span class="section-label">Explore Topics</span>
			{#if isLoadingTags}
				<div class="flex flex-col gap-2 px-3">
					<div class="flex gap-2">
						{#each [1, 2, 3] as _i}
							<div class="skeleton-tag"></div>
						{/each}
					</div>
					<div class="flex items-center gap-2">
						<div class="loading-pulse"></div>
						<span class="loading-text">Generating tags&hellip;</span>
					</div>
				</div>
			{:else}
				<TagChipList {tags} onTagClick={onTagClick} />
			{/if}
		</div>
	</div>
</div>

<style>
	.featured-section {
		margin: 0 1rem;
		border: 1px solid var(--surface-border, #c8c8c8);
		background: var(--surface-bg, #faf9f7);
	}

	.section-label {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-ash, #999);
		margin-bottom: 0.35rem;
	}

	.feed-title {
		display: block;
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		text-decoration: none;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.feed-title:hover {
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	.feed-desc {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.8rem;
		font-style: italic;
		line-height: 1.5;
		color: var(--alt-slate, #666);
		margin: 0;
		display: -webkit-box;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		-webkit-box-orient: vertical;
		overflow: hidden;
	}

	.refresh-btn {
		display: flex;
		align-items: center;
		justify-content: center;
		min-height: 44px;
		min-width: 44px;
		flex-shrink: 0;

		color: var(--alt-slate, #666);
		background: transparent;
		border: 1px solid var(--surface-border, #c8c8c8);
		cursor: pointer;
		transition: background 0.15s, color 0.15s, border-color 0.15s;
	}
	.refresh-btn:hover {
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		border-color: var(--alt-charcoal, #1a1a1a);
	}

	.tags-section {
		border-top: 1px solid var(--surface-border, #c8c8c8);
		padding-top: 0.75rem;
	}

	.skeleton-tag {
		height: 44px;
		width: 80px;
		background: var(--muted);
		animation: shimmer 1.5s ease-in-out infinite;
	}
	@keyframes shimmer {
		0%, 100% { opacity: 0.5; }
		50% { opacity: 1; }
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}
	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
	.loading-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	@media (prefers-reduced-motion: reduce) {
		.skeleton-tag { animation: none; opacity: 0.5; }
		.loading-pulse { animation: none; opacity: 0.6; }
	}
</style>
