<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";
import SwipeFeedScreen from "$lib/components/mobile/feeds/swipe/SwipeFeedScreen.svelte";
import { articlePrefetcher } from "$lib/utils/articlePrefetcher";
import type { RenderFeed } from "$lib/schema/feed";

const { isDesktop } = useViewport();

interface ArticleResult {
	content: string | null;
	og_image_url: string | null;
	article_id: string | null;
	feedUrl: string | null;
}

interface PageData {
	initialFeeds: RenderFeed[];
	nextCursor: string | null;
	articleContentPromise: Promise<ArticleResult>;
}

const { data }: { data: PageData } = $props();

// Handle the streamed articleContent Promise
let articleContent = $state<string | null>(null);

$effect(() => {
	data.articleContentPromise?.then((result) => {
		articleContent = result.content;

		// Seed articlePrefetcher cache with initial article data
		if (result.feedUrl) {
			const ogImageCache = (articlePrefetcher as any)
				.ogImageCache as Map<string, string | null>;
			ogImageCache.set(result.feedUrl, result.og_image_url);

			if (result.article_id) {
				const articleIdCache = (articlePrefetcher as any)
					.articleIdCache as Map<string, string>;
				articleIdCache.set(result.feedUrl, result.article_id);
			}
		}
	});
});
</script>

<svelte:head>
	<title>Visual Preview - Alt</title>
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
		initialArticleContent={articleContent}
		mode="visual-preview"
	/>
{/if}
