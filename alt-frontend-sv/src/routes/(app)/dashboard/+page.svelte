<script lang="ts">
import { goto } from "$app/navigation";
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import UnreadFeedsWidget from "$lib/components/desktop/dashboard/UnreadFeedsWidget.svelte";
import RecapSummaryWidget from "$lib/components/desktop/dashboard/RecapSummaryWidget.svelte";
import StatsBarWidget from "$lib/components/desktop/dashboard/StatsBarWidget.svelte";

const { isDesktop } = useViewport();

onMount(() => {
	if (!isDesktop) {
		goto("/sv/feeds", { replaceState: true });
	}
});
</script>

<svelte:head>
	<title>Dashboard - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader title="Dashboard" description="Overview of Alt RSS Reader" />

	<div class="grid grid-cols-2 gap-6 mb-6">
		<UnreadFeedsWidget />
		<RecapSummaryWidget />
	</div>

	<StatsBarWidget />
{:else}
	<div class="flex items-center justify-center min-h-screen" style="background: var(--app-bg);">
		<p class="text-sm text-[var(--text-secondary)]">Redirecting...</p>
	</div>
{/if}
