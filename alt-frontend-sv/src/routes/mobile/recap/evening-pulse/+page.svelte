<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import MobilePulseView from "$lib/components/mobile/pulse/MobilePulseView.svelte";
import MobilePulseSkeleton from "$lib/components/mobile/pulse/MobilePulseSkeleton.svelte";
import MobilePulseQuietDay from "$lib/components/mobile/pulse/MobilePulseQuietDay.svelte";
import MobilePulseError from "$lib/components/mobile/pulse/MobilePulseError.svelte";
import MobilePulseTopicSheet from "$lib/components/mobile/pulse/MobilePulseTopicSheet.svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import { usePulse } from "$lib/hooks/usePulse.svelte";

const pulse = usePulse();
let isSheetOpen = $state(false);

const navigateToRecap = () => {
	goto("/sv/mobile/recap/3days");
};

const handleTopicClick = (clusterId: number) => {
	if (pulse.selectTopic(clusterId)) {
		isSheetOpen = true;
	}
};

const handleCloseSheet = () => {
	isSheetOpen = false;
	pulse.clearSelectedTopic();
};

const handleNavigateToRecap = (clusterId: number) => {
	isSheetOpen = false;
	goto(`/sv/mobile/recap/3days?cluster=${clusterId}`);
};

onMount(() => {
	if (browser) {
		void pulse.fetchData();
	}
});
</script>

<svelte:head>
	<title>Evening Pulse - Alt</title>
</svelte:head>

<div class="min-h-[100dvh] relative" style="background: var(--app-bg);">
	{#if pulse.isLoading}
		<MobilePulseSkeleton />
	{:else if pulse.error}
		<MobilePulseError error={pulse.error} onRetry={pulse.retry} isRetrying={pulse.isRetrying} />
	{:else if pulse.data?.status === "quiet_day"}
		<MobilePulseQuietDay
			date={pulse.data.generatedAt}
			quietDay={pulse.data.quietDay}
			onNavigateToRecap={navigateToRecap}
		/>
	{:else if pulse.data}
		<MobilePulseView pulse={pulse.data} onTopicClick={handleTopicClick} />
	{:else}
		<MobilePulseQuietDay
			date={new Date().toISOString()}
			quietDay={{ message: "Evening Pulse is not yet available", weeklyHighlights: [] }}
			onNavigateToRecap={navigateToRecap}
		/>
	{/if}

	<FloatingMenu />

	<MobilePulseTopicSheet
		topic={pulse.selectedTopic}
		open={isSheetOpen}
		onClose={handleCloseSheet}
		onNavigateToRecap={handleNavigateToRecap}
	/>
</div>
