<script lang="ts">
import { browser } from "$app/environment";
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import SwipeFeedScreen from "$lib/components/mobile/feeds/swipe/SwipeFeedScreen.svelte";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import type { RenderFeed } from "$lib/schema/feed";

const { isDesktop } = useViewport();

interface ArticleData {
	firstArticleImageUrl: string | null;
	firstArticleContent: string | null;
	firstArticleId: string | null;
}

interface PageData {
	initialFeeds: RenderFeed[];
	nextCursor: string | null;
	articleData: Promise<ArticleData>;
}

const { data }: { data: PageData } = $props();

// Resolved article data from streaming - populated when promise resolves
let resolvedArticleData = $state<ArticleData | null>(null);
let cacheSeeded = false;

// Resolve streamed article data and seed prefetcher cache on mount
onMount(() => {
	data.articleData.then((articleData) => {
		resolvedArticleData = articleData;

		// Seed cache once
		if (!cacheSeeded && data.initialFeeds.length > 0 && articleData.firstArticleId) {
			cacheSeeded = true;
			const feedUrl = data.initialFeeds[0].normalizedUrl;
			articlePrefetcher.seedCache(
				feedUrl,
				articleData.firstArticleContent || "",
				articleData.firstArticleId,
				articleData.firstArticleImageUrl,
				null,
			);
		}
	});
});
</script>

<svelte:head>
	<title>Visual Preview - Alt</title>
	{#if resolvedArticleData?.firstArticleImageUrl}
		<link rel="preload" as="image" href={resolvedArticleData.firstArticleImageUrl} fetchpriority="high" />
	{/if}
</svelte:head>

{#if isDesktop}
	<div class="flex flex-col items-center justify-center py-24 text-center">
		<p class="text-lg font-medium text-[var(--text-primary)] mb-2">
			Visual Preview mode is optimized for mobile
		</p>
		<p class="text-sm text-[var(--text-secondary)] mb-6">
			Use a mobile device or resize your browser window to use the swipe interface.
		</p>
		<a
			href="/sv/feeds"
			class="px-4 py-2 rounded-lg text-sm font-medium transition-colors hover:opacity-90"
			style="background: var(--accent-primary); color: var(--accent-primary-foreground);"
		>
			Go to Feeds
		</a>
	</div>
{:else}
	<SwipeFeedScreen
		initialFeeds={data.initialFeeds}
		initialNextCursor={data.nextCursor}
		initialArticleContent={resolvedArticleData?.firstArticleContent ?? null}
		initialOgImageUrl={resolvedArticleData?.firstArticleImageUrl ?? null}
		mode="visual-preview"
	/>
{/if}
