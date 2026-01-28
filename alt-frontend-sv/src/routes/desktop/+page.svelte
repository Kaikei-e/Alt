<script lang="ts">
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import UnreadFeedsWidget from "$lib/components/desktop/dashboard/UnreadFeedsWidget.svelte";
	import RecapSummaryWidget from "$lib/components/desktop/dashboard/RecapSummaryWidget.svelte";
	import StatsBarWidget from "$lib/components/desktop/dashboard/StatsBarWidget.svelte";
</script>

<svelte:head>
	<title>Dashboard - Alt</title>
</svelte:head>

<PageHeader title="Dashboard" description="Overview of Alt RSS Reader" />

<div class="grid grid-cols-2 gap-6 mb-6">
	<UnreadFeedsWidget />
	<RecapSummaryWidget />
</div>

<StatsBarWidget />
