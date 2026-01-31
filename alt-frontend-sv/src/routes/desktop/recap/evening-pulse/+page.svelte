<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import { createClientTransport, getEveningPulse } from "$lib/connect";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopPulseView from "$lib/components/desktop/pulse/DesktopPulseView.svelte";
import DesktopPulseSkeleton from "$lib/components/desktop/pulse/DesktopPulseSkeleton.svelte";
import DesktopPulseQuietDay from "$lib/components/desktop/pulse/DesktopPulseQuietDay.svelte";
import DesktopPulseError from "$lib/components/desktop/pulse/DesktopPulseError.svelte";
import DesktopPulseDetailPanel from "$lib/components/desktop/pulse/DesktopPulseDetailPanel.svelte";
import type { EveningPulse, PulseTopic } from "$lib/schema/evening_pulse";

let data = $state<EveningPulse | null>(null);
let isLoading = $state(true);
let error = $state<Error | null>(null);
let isRetrying = $state(false);
let selectedTopic = $state<PulseTopic | null>(null);
let isPanelOpen = $state(false);

const fetchData = async () => {
	try {
		isLoading = true;
		error = null;
		const transport = createClientTransport();
		data = await getEveningPulse(transport);
	} catch (err) {
		if (err instanceof ConnectError) {
			if (err.code === Code.Unauthenticated) {
				goto("/login");
				return;
			}
			// NOT_FOUND is treated as no data (not an error)
			if (err.code === Code.NotFound) {
				data = null;
				error = null;
				return;
			}
		}
		error = err instanceof Error ? err : new Error("Unknown error");
		data = null;
	} finally {
		isLoading = false;
	}
};

const retry = async () => {
	isRetrying = true;
	try {
		await fetchData();
	} catch (err) {
		console.error("Retry failed:", err);
	} finally {
		isRetrying = false;
	}
};

const navigateToRecap = () => {
	goto("/sv/desktop/recap");
};

const handleTopicClick = (clusterId: number) => {
	const topic = data?.topics.find((t) => t.clusterId === clusterId);
	if (topic) {
		selectedTopic = topic;
		isPanelOpen = true;
	}
};

const handleClosePanel = () => {
	isPanelOpen = false;
};

const handleNavigateToRecap = (clusterId: number, genre?: string) => {
	const params = new URLSearchParams();
	params.set('cluster', String(clusterId));
	if (genre) {
		params.set('genre', genre);
	}
	goto(`/sv/desktop/recap?${params.toString()}`);
};

const handleHighlightClick = (id: string) => {
	goto(`/sv/desktop/recap?highlight=${id}`);
};

onMount(() => {
	if (browser) {
		void fetchData();
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
	{#if isLoading}
		<DesktopPulseSkeleton />
	{:else if error}
		<DesktopPulseError {error} onRetry={retry} {isRetrying} />
	{:else if data?.status === "quiet_day"}
		<DesktopPulseQuietDay
			date={data.generatedAt}
			quietDay={data.quietDay}
			onNavigateToRecap={navigateToRecap}
			onHighlightClick={handleHighlightClick}
		/>
	{:else if data}
		<DesktopPulseView
			pulse={data}
			onTopicClick={handleTopicClick}
			onNavigateToRecap={navigateToRecap}
		/>
	{:else}
		<!-- No data available (NOT_FOUND) -->
		<DesktopPulseQuietDay
			date={new Date().toISOString()}
			quietDay={{ message: "Evening Pulse is not yet available", weeklyHighlights: [] }}
			onNavigateToRecap={navigateToRecap}
		/>
	{/if}
</div>

<DesktopPulseDetailPanel
	topic={selectedTopic}
	open={isPanelOpen}
	onClose={handleClosePanel}
	onNavigateToRecap={handleNavigateToRecap}
/>
