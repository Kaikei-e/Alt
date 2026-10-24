<script lang="ts">
import { onMount, untrack } from "svelte";
// Desktop components
import DesktopTagTrailScreen from "#lib/components/desktop/tag-trail/DesktopTagTrailScreen.svelte";
// Mobile components
import TagTrailScreen from "#lib/components/mobile/tag-trail/TagTrailScreen.svelte";
import type { TagTrailArticle, TagTrailTag } from "#lib/connect/index.js";
import type { TagTrailHop } from "#lib/schema/tagTrail.js";
import { isDesktop } from "#lib/stores/viewport.svelte.js";

interface FeedData {
	id: string;
	url: string;
	title?: string;
	description?: string;
	tags?: TagTrailTag[];
	latestArticleId?: string;
}

interface PageData {
	initialFeed?: FeedData | null;
	error?: string;
}

const { data }: { data: PageData } = $props();

let revealed = $state(false);

// Shared session state across mobile <-> desktop viewport rotation
let currentFeed = $state<FeedData | null>(
	untrack(() => data.initialFeed ?? null),
);
let feedTags = $state<TagTrailTag[]>(
	untrack(() => data.initialFeed?.tags ?? []),
);
let isLoadingFeedTags = $state(false);
let isLoadingFeed = $state(false);
let refreshError = $state<string | null>(null);
let feedTagsError = $state<string | null>(null);
let selectedTag = $state<TagTrailTag | null>(null);
let articles = $state<TagTrailArticle[]>([]);
let isLoadingArticles = $state(false);
let hasMoreArticles = $state(false);
let nextCursor = $state<string | undefined>(undefined);
let hops = $state<TagTrailHop[]>([]);
let articleTagsCache = $state<Map<string, TagTrailTag[]>>(new Map());
let loadingArticleTags = $state<Set<string>>(new Set());
let lastProcessedFeedId = $state<string | null>(null);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
});
</script>

<svelte:head>
	<title>Tag Trail - Alt</title>
</svelte:head>

<div class="tag-trail-page" class:revealed data-role="tag-trail-page">
	{#if isDesktop()}
		<DesktopTagTrailScreen
			initialFeed={data.initialFeed}
			bind:currentFeed
			bind:feedTags
			bind:isLoadingFeedTags
			bind:isLoadingFeed
			bind:refreshError
			bind:feedTagsError
			bind:selectedTag
			bind:articles
			bind:isLoadingArticles
			bind:hasMoreArticles
			bind:nextCursor
			bind:hops
			bind:articleTagsCache
			bind:loadingArticleTags
			bind:lastProcessedFeedId
		/>
	{:else}
		<div class="flex flex-col h-[100dvh] overflow-hidden" style="background: var(--app-bg);">
			<TagTrailScreen
				initialFeed={data.initialFeed}
				bind:currentFeed
				bind:feedTags
				bind:isLoadingFeedTags
				bind:isLoadingFeed
				bind:refreshError
				bind:feedTagsError
				bind:selectedTag
				bind:articles
				bind:isLoadingArticles
				bind:hasMoreArticles
				bind:nextCursor
				bind:hops
				bind:articleTagsCache
				bind:loadingArticleTags
				bind:lastProcessedFeedId
			/>
		</div>
	{/if}
</div>

<style>
	.tag-trail-page {
		opacity: 0;
		transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}
	.tag-trail-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	@media (prefers-reduced-motion: reduce) {
		.tag-trail-page {
			transition: none;
			opacity: 1;
			transform: none;
		}
	}
</style>
