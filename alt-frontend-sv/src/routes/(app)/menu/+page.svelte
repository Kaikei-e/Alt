<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import MobileMenuPage from "$lib/components/mobile/MobileMenuPage.svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

const { isDesktop } = useViewport();

onMount(() => {
	if (browser && isDesktop) {
		goto("/feeds", { replaceState: true });
	}
});
</script>

<svelte:head>
	<title>Menu - Alt</title>
</svelte:head>

{#if isDesktop}
	<!-- Desktop: redirect to feeds (handled in onMount) -->
	<div></div>
{:else}
	<MobileMenuPage />
{/if}
