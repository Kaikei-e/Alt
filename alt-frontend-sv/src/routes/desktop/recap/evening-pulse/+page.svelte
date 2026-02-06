<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopPulseView from "$lib/components/desktop/pulse/DesktopPulseView.svelte";
import DesktopPulseSkeleton from "$lib/components/desktop/pulse/DesktopPulseSkeleton.svelte";
import DesktopPulseQuietDay from "$lib/components/desktop/pulse/DesktopPulseQuietDay.svelte";
import DesktopPulseError from "$lib/components/desktop/pulse/DesktopPulseError.svelte";
import DesktopPulseDetailPanel from "$lib/components/desktop/pulse/DesktopPulseDetailPanel.svelte";
import { usePulse } from "$lib/hooks/usePulse.svelte";

const pulse = usePulse();
let isPanelOpen = $state(false);

const navigateToRecap = () => {
	goto("/sv/desktop/recap");
};

const handleTopicClick = (clusterId: number) => {
	if (pulse.selectTopic(clusterId)) {
		isPanelOpen = true;
	}
};

const handleClosePanel = () => {
	isPanelOpen = false;
};

const handleNavigateToRecap = (clusterId: number, genre?: string) => {
	const params = new URLSearchParams();
	params.set("cluster", String(clusterId));
	if (genre) {
		params.set("genre", genre);
	}
	goto(`/sv/desktop/recap?${params.toString()}`);
};

const handleHighlightClick = (id: string) => {
	goto(`/sv/desktop/recap?highlight=${id}`);
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
