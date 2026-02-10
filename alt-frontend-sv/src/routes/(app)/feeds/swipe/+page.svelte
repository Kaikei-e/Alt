<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";
import SwipeFeedScreen from "$lib/components/mobile/feeds/swipe/SwipeFeedScreen.svelte";
import type { RenderFeed } from "$lib/schema/feed";

const { isDesktop } = useViewport();

interface PageData {
	initialFeeds: RenderFeed[];
	nextCursor: string | null;
	articleContentPromise: Promise<string | null>;
}

const { data }: { data: PageData } = $props();

// Handle the streamed articleContent Promise
let articleContent = $state<string | null>(null);

$effect(() => {
	data.articleContentPromise?.then((content) => {
		articleContent = content;
	});
});
</script>

<svelte:head>
	<title>Swipe - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="flex flex-col items-center justify-center py-24 text-center">
		<p class="text-lg font-medium text-[var(--text-primary)] mb-2">
			Swipe mode is optimized for mobile
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
	/>
{/if}
