<script lang="ts">
import FeedsClient from "$lib/components/mobile/FeedsClient.svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import MobileFeedsHero from "$lib/components/mobile/MobileFeedsHero.svelte";
import type { RenderFeed } from "$lib/schema/feed";

interface PageData {
	initialFeeds?: RenderFeed[];
	error?: string;
}

const { data }: { data: PageData } = $props();
</script>

<div
	class="h-screen overflow-hidden flex flex-col"
	style="background: var(--app-bg);"
>
	<MobileFeedsHero />
	<div class="flex-1 relative">
		<FeedsClient initialFeeds={data.initialFeeds || []} />
		<FloatingMenu />
	</div>
</div>
