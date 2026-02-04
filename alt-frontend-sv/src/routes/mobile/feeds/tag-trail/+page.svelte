<script lang="ts">
import TagTrailScreen from "$lib/components/mobile/tag-trail/TagTrailScreen.svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";

interface PageData {
	initialFeed?: {
		id: string;
		url: string;
		title?: string;
		description?: string;
	} | null;
	error?: string;
}

const { data }: { data: PageData } = $props();
</script>

<svelte:head>
	<title>Tag Trail - Alt</title>
</svelte:head>

<div
	class="h-screen overflow-hidden flex flex-col"
	style="background: var(--app-bg);"
>
	<TagTrailScreen initialFeed={data.initialFeed} />
	<FloatingMenu />
</div>
