<script lang="ts">
import SwipeFeedScreen from "$lib/components/mobile/feeds/swipe/SwipeFeedScreen.svelte";
import type { PageData } from "./$types";

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

<SwipeFeedScreen
	initialFeeds={data.initialFeeds}
	initialNextCursor={data.nextCursor}
	initialArticleContent={articleContent}
/>
