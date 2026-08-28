<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import MobileMenuPage from "$lib/components/mobile/MobileMenuPage.svelte";
import { isDesktop } from "$lib/stores/viewport.svelte";

// Deliberately `onMount` and deliberately not an `$effect`: this is an
// arrival check, not a viewport binding. Landing on /menu with a sidebar
// already on screen means the reader took a bookmark or the back button to a
// duplicate of the sidebar, so we hand them to /feeds. Turning a phone is not
// an arrival — a `goto` in an `$effect` would fire on every rotation, and on
// every drag of a desktop window across 768px, throwing away wherever the
// reader was.
//
// The page therefore has to be readable at every width, because `onMount` will
// not run again to rescue it. It used to render an empty `<div>` on desktop
// and rely entirely on this redirect; once the viewport check became reactive,
// rotating into landscape flipped to that empty branch with no redirect behind
// it and left a white screen. The menu is a grid of links — it reads fine wide,
// so there is no second branch to get stuck in.
onMount(() => {
	if (browser && isDesktop()) {
		goto("/feeds", { replaceState: true });
	}
});
</script>

<svelte:head>
	<title>Menu - Alt</title>
</svelte:head>

<MobileMenuPage />
