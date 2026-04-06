<script lang="ts">
import { onMount } from "svelte";
import { goto } from "$app/navigation";
import { useViewport } from "$lib/stores/viewport.svelte";
import TagVerseScreen from "$lib/components/desktop/tag-verse/TagVerseScreen.svelte";

const { isDesktop } = useViewport();

onMount(() => {
	if (!isDesktop) {
		goto("/home", { replaceState: true });
	}
});
</script>

<svelte:head>
	<title>Tag Verse - Alt</title>
</svelte:head>

{#if isDesktop}
	<TagVerseScreen />
{:else}
	<div
		class="flex h-screen items-center justify-center p-8 text-center"
		style="background: var(--app-bg);"
	>
		<p class="text-muted-foreground">
			Tag Verse is available on desktop only.
		</p>
	</div>
{/if}
