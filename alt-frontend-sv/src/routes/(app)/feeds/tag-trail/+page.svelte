<script lang="ts">
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components
import DesktopTagTrailScreen from "$lib/components/desktop/tag-trail/DesktopTagTrailScreen.svelte";

// Mobile components
import TagTrailScreen from "$lib/components/mobile/tag-trail/TagTrailScreen.svelte";

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
const { isDesktop } = useViewport();
</script>

<svelte:head>
	<title>Tag Trail - Alt</title>
</svelte:head>

{#if isDesktop}
	<DesktopTagTrailScreen initialFeed={data.initialFeed} />
{:else}
	<div
		class="h-screen overflow-hidden flex flex-col"
		style="background: var(--app-bg);"
	>
		<TagTrailScreen initialFeed={data.initialFeed} />
	</div>
{/if}
