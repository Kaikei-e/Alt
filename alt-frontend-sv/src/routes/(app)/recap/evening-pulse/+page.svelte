<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { useViewport } from "$lib/stores/viewport.svelte";

// Desktop components
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopPulseView from "$lib/components/desktop/pulse/DesktopPulseView.svelte";
import DesktopPulseSkeleton from "$lib/components/desktop/pulse/DesktopPulseSkeleton.svelte";
import DesktopPulseQuietDay from "$lib/components/desktop/pulse/DesktopPulseQuietDay.svelte";
import DesktopPulseError from "$lib/components/desktop/pulse/DesktopPulseError.svelte";
import DesktopPulseDetailPanel from "$lib/components/desktop/pulse/DesktopPulseDetailPanel.svelte";

// Mobile components
import MobilePulseView from "$lib/components/mobile/pulse/MobilePulseView.svelte";
import MobilePulseSkeleton from "$lib/components/mobile/pulse/MobilePulseSkeleton.svelte";
import MobilePulseQuietDay from "$lib/components/mobile/pulse/MobilePulseQuietDay.svelte";
import MobilePulseError from "$lib/components/mobile/pulse/MobilePulseError.svelte";
import MobilePulseTopicSheet from "$lib/components/mobile/pulse/MobilePulseTopicSheet.svelte";

import { usePulse } from "$lib/hooks/usePulse.svelte";

const { isDesktop } = useViewport();
const pulse = usePulse();

// Desktop state
let isPanelOpen = $state(false);

// Mobile state
let isSheetOpen = $state(false);

const navigateToRecap = () => {
	goto("/sv/recap");
};

const handleTopicClick = (clusterId: number) => {
	if (pulse.selectTopic(clusterId)) {
		if (isDesktop) {
			isPanelOpen = true;
		} else {
			isSheetOpen = true;
		}
	}
};

// Desktop handlers
const handleClosePanel = () => {
	isPanelOpen = false;
};

const handleNavigateToRecap = (clusterId: number, genre?: string) => {
	const params = new URLSearchParams();
	params.set("cluster", String(clusterId));
	if (genre) {
		params.set("genre", genre);
	}
	goto(`/sv/recap?${params.toString()}`);
};

const handleHighlightClick = (id: string) => {
	goto(`/sv/recap?highlight=${id}`);
};

// Mobile handlers
const handleCloseSheet = () => {
	isSheetOpen = false;
	pulse.clearSelectedTopic();
};

const handleMobileNavigateToRecap = (clusterId: number) => {
	isSheetOpen = false;
	goto(`/sv/recap?cluster=${clusterId}`);
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

{#if isDesktop}
	<PageHeader
		title="Evening Pulse"
		description="Today's key topics curated for you"
	/>

	<div class="min-h-[calc(100vh-12rem)]">
		{#if pulse.isLoading}
			<DesktopPulseSkeleton />
		{:else if pulse.error}
			<DesktopPulseError error={pulse.error} onRetry={pulse.retry} isRetrying={pulse.isRetrying} />
		{:else if pulse.data?.status === "quiet_day"}
			<DesktopPulseQuietDay
				date={pulse.data.generatedAt}
				quietDay={pulse.data.quietDay}
				onNavigateToRecap={navigateToRecap}
				onHighlightClick={handleHighlightClick}
			/>
		{:else if pulse.data}
			<DesktopPulseView
				pulse={pulse.data}
				onTopicClick={handleTopicClick}
				onNavigateToRecap={navigateToRecap}
			/>
		{:else}
			<DesktopPulseQuietDay
				date={new Date().toISOString()}
				quietDay={{ message: "Evening Pulse is not yet available", weeklyHighlights: [] }}
				onNavigateToRecap={navigateToRecap}
			/>
		{/if}
	</div>

	<DesktopPulseDetailPanel
		topic={pulse.selectedTopic}
		open={isPanelOpen}
		onClose={handleClosePanel}
		onNavigateToRecap={handleNavigateToRecap}
	/>
{:else}
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

		<MobilePulseTopicSheet
			topic={pulse.selectedTopic}
			open={isSheetOpen}
			onClose={handleCloseSheet}
			onNavigateToRecap={handleMobileNavigateToRecap}
		/>
	</div>
{/if}
