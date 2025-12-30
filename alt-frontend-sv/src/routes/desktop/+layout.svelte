<script lang="ts">
	import type { Snippet } from "svelte";
	import { navigating } from "$app/state";
	import DesktopLayout from "$lib/components/desktop/layout/DesktopLayout.svelte";
	import { SystemLoader } from "$lib/components/ui/system-loader";
	import { loadingStore } from "$lib/stores/loading.svelte";

	let { children }: { children: Snippet } = $props();

	// Show loader during navigation OR when page is fetching data
	// navigating from $app/state is used directly (no $ prefix needed)
	let showLoader = $derived(!!navigating || loadingStore.isDesktopLoading);
</script>

{#if showLoader}
	<SystemLoader />
{/if}

<DesktopLayout>
	{@render children()}
</DesktopLayout>
