<script lang="ts">
import type { Snippet } from "svelte";
import { navigating } from "$app/state";
import ResponsiveLayout from "$lib/components/layout/ResponsiveLayout.svelte";
import { SystemLoader } from "$lib/components/ui/system-loader";
import { getLoadingStore } from "$lib/stores/loading.svelte";

let { children }: { children: Snippet } = $props();

const loadingStore = getLoadingStore();

// Safety net: if navigation stays stuck for too long, hide the loader
const NAVIGATION_TIMEOUT_MS = 10_000;
let navigationTimedOut = $state(false);
let timeoutId: ReturnType<typeof setTimeout> | undefined;

$effect(() => {
	const isNavigating = navigating.type !== null;
	if (isNavigating) {
		navigationTimedOut = false;
		timeoutId = setTimeout(() => {
			navigationTimedOut = true;
		}, NAVIGATION_TIMEOUT_MS);
	} else {
		navigationTimedOut = false;
		if (timeoutId) {
			clearTimeout(timeoutId);
			timeoutId = undefined;
		}
	}
	return () => {
		if (timeoutId) {
			clearTimeout(timeoutId);
			timeoutId = undefined;
		}
	};
});

// Show loader during navigation OR when page is fetching data
let showLoader = $derived(
	(navigating.type !== null && !navigationTimedOut) ||
		loadingStore.isDesktopLoading,
);
</script>

{#if showLoader}
	<SystemLoader />
{/if}

<ResponsiveLayout>
	{@render children()}
</ResponsiveLayout>
