<script lang="ts">
import FeedsClient from "$lib/components/mobile/FeedsClient.svelte";
import MobileFeedsHero from "$lib/components/mobile/MobileFeedsHero.svelte";
import type { RenderFeed } from "$lib/schema/feed";

interface PageData {
	initialFeeds?: RenderFeed[];
	error?: string;
}

const { data }: { data: PageData } = $props();
</script>

<div class="min-h-screen flex flex-col" style="background: var(--app-bg);">
	<MobileFeedsHero />
	<div class="flex-1 relative mt-2">
		<FeedsClient initialFeeds={data.initialFeeds || []} />
	</div>
</div>

