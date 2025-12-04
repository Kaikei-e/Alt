<script lang="ts">
  import SwipeFeedScreen from "$lib/components/mobile/feeds/swipe/SwipeFeedScreen.svelte";
  import type { PageData } from "./$types";

  const { data }: { data: PageData } = $props();
</script>

<SwipeFeedScreen
  initialFeeds={data.initialFeeds}
  initialNextCursor={data.nextCursor}
  initialArticleContent={data.articleContent}
/>
