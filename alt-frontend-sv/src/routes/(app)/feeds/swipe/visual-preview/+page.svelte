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

// Resolved article data from streaming
let resolvedArticleData = $state<ArticleData | null>(null);
let cacheSeeded = false;

// Resolve streamed article data and seed prefetcher cache on mount
onMount(() => {
	data.articleData.then((articleData) => {
		resolvedArticleData = articleData;

		if (
			!cacheSeeded &&
			data.initialFeeds.length > 0 &&
			articleData.firstArticleId
		) {
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
	<div class="desktop-fallback">
		<p class="fallback-heading">
			Visual Preview mode is optimized for mobile
		</p>
		<p class="fallback-text">
			Use a mobile device or resize your browser window to use the swipe interface.
		</p>
		<a href="/feeds" class="fallback-link">
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

<style>
	.desktop-fallback {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		padding: 6rem 1.5rem;
		text-align: center;
	}

	.fallback-heading {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 600;
		color: var(--alt-charcoal);
		margin: 0 0 0.5rem;
	}

	.fallback-text {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-slate);
		margin: 0 0 1.5rem;
	}

	.fallback-link {
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
		background: transparent;
		border: 1.5px solid var(--alt-charcoal);
		padding: 0.5rem 1.5rem;
		min-height: 44px;
		display: inline-flex;
		align-items: center;
		text-decoration: none;
		transition: background 0.15s, color 0.15s;
	}

	.fallback-link:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}
</style>
