<script lang="ts">
import { onMount } from "svelte";
import { goto } from "$app/navigation";
import TagVerseScreen from "$lib/components/desktop/tag-verse/TagVerseScreen.svelte";
import { isDesktop, isMobile } from "$lib/stores/viewport.svelte";

// Deliberately `onMount` and deliberately not an `$effect`: this is an arrival
// check, not a viewport binding. Tag Verse is a WebGL tag cloud built for a
// pointer and a wide canvas, so a phone-sized *arrival* is handed to Knowledge
// Home rather than shown a screen it cannot use. Turning a phone is not an
// arrival, and neither is dragging a desktop window across 768px — a `goto` in
// an `$effect` would fire on both.
//
// So the narrow branch has to stand on its own, because `onMount` will not run
// again to rescue it. It says what happened and offers the same exit the
// redirect would have taken.
onMount(() => {
	if (isMobile()) {
		goto("/home", { replaceState: true });
	}
});
</script>

<svelte:head>
	<title>Tag Verse - Alt</title>
</svelte:head>

{#if isDesktop()}
	<TagVerseScreen />
{:else}
	<div
		class="flex h-[100dvh] flex-col items-center justify-center gap-3 p-8 text-center"
		style="background: var(--app-bg);"
	>
		<p class="text-muted-foreground">
			Tag Verse is available on desktop only.
		</p>
		<a
			href="/home"
			class="text-sm underline underline-offset-4"
			style="color: var(--text-primary);"
		>
			Go to Knowledge Home
		</a>
	</div>
{/if}
