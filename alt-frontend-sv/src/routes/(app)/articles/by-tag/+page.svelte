<script lang="ts">
import { page } from "$app/state";
import DesktopTagArticlesScreen from "$lib/components/desktop/articles/DesktopTagArticlesScreen.svelte";
import MobileTagArticlesScreen from "$lib/components/mobile/articles/MobileTagArticlesScreen.svelte";
import { isDesktop } from "$lib/stores/viewport.svelte";

const tagName = $derived(page.url.searchParams.get("tag") ?? "");
</script>

<svelte:head>
	<title>{tagName ? `${tagName} - Articles` : "Tag Articles"} - Alt</title>
</svelte:head>

{#if !tagName}
	<div class="flex items-center justify-center h-full p-8 text-center">
		<p class="text-[var(--text-secondary)]">No tag specified. Click a tag from Knowledge Home to browse articles.</p>
	</div>
{:else if isDesktop()}
	<DesktopTagArticlesScreen {tagName} />
{:else}
	<MobileTagArticlesScreen {tagName} />
{/if}
