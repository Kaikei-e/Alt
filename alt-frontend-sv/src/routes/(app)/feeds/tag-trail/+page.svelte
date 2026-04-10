<script lang="ts">
import { onMount } from "svelte";
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

let revealed = $state(false);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
});
</script>

<svelte:head>
	<title>Tag Trail - Alt</title>
</svelte:head>

<div class="tag-trail-page" class:revealed data-role="tag-trail-page">
	{#if isDesktop}
		<DesktopTagTrailScreen initialFeed={data.initialFeed} />
	{:else}
		<div class="flex flex-col h-[100dvh] overflow-hidden" style="background: var(--app-bg);">
			<TagTrailScreen initialFeed={data.initialFeed} />
		</div>
	{/if}
</div>

<style>
	.tag-trail-page {
		opacity: 0;
		transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}
	.tag-trail-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	@media (prefers-reduced-motion: reduce) {
		.tag-trail-page {
			transition: none;
			opacity: 1;
			transform: none;
		}
	}
</style>
