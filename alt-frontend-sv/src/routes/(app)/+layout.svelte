<script lang="ts">
import type { Snippet } from "svelte";
import { navigating } from "$app/state";
import ResponsiveLayout from "$lib/components/layout/ResponsiveLayout.svelte";
import { SystemLoader } from "$lib/components/ui/system-loader";
import { getLoadingStore } from "$lib/stores/loading.svelte";

let { children }: { children: Snippet } = $props();

const loadingStore = getLoadingStore();

// Show loader during navigation OR when page is fetching data
let showLoader = $derived(
	navigating.type !== null || loadingStore.isDesktopLoading,
);
</script>

{#if showLoader}
	<SystemLoader />
{/if}

<ResponsiveLayout>
	{@render children()}
</ResponsiveLayout>
