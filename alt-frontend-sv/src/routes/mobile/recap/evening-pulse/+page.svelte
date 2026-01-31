<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import { createClientTransport, getEveningPulse } from "$lib/connect";
import MobilePulseView from "$lib/components/mobile/pulse/MobilePulseView.svelte";
import MobilePulseSkeleton from "$lib/components/mobile/pulse/MobilePulseSkeleton.svelte";
import MobilePulseQuietDay from "$lib/components/mobile/pulse/MobilePulseQuietDay.svelte";
import MobilePulseError from "$lib/components/mobile/pulse/MobilePulseError.svelte";
import MobilePulseTopicSheet from "$lib/components/mobile/pulse/MobilePulseTopicSheet.svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import type { EveningPulse, PulseTopic } from "$lib/schema/evening_pulse";

let data = $state<EveningPulse | null>(null);
let isLoading = $state(true);
let error = $state<Error | null>(null);
let isRetrying = $state(false);
let selectedTopic = $state<PulseTopic | null>(null);
let isSheetOpen = $state(false);

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
			// NOT_FOUND はデータなしとして扱う（エラーではない）
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
	goto("/sv/mobile/recap/7days");
};

const handleTopicClick = (clusterId: number) => {
	const topic = data?.topics.find((t) => t.clusterId === clusterId);
	if (topic) {
		selectedTopic = topic;
		isSheetOpen = true;
	}
};

const handleCloseSheet = () => {
	isSheetOpen = false;
	selectedTopic = null;
};

const handleNavigateToRecap = (clusterId: number) => {
	isSheetOpen = false;
	goto(`/sv/mobile/recap/7days?cluster=${clusterId}`);
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

<div class="min-h-[100dvh] relative" style="background: var(--app-bg);">
	{#if isLoading}
		<MobilePulseSkeleton />
	{:else if error}
		<MobilePulseError {error} onRetry={retry} {isRetrying} />
	{:else if data?.status === "quiet_day"}
		<MobilePulseQuietDay
			date={data.generatedAt}
			quietDay={data.quietDay}
			onNavigateToRecap={navigateToRecap}
		/>
	{:else if data}
		<MobilePulseView pulse={data} onTopicClick={handleTopicClick} />
	{:else}
		<!-- データが存在しない場合（NOT_FOUND） -->
		<MobilePulseQuietDay
			date={new Date().toISOString()}
			quietDay={{ message: "Evening Pulse is not yet available", weeklyHighlights: [] }}
			onNavigateToRecap={navigateToRecap}
		/>
	{/if}

	<FloatingMenu />

	<MobilePulseTopicSheet
		topic={selectedTopic}
		open={isSheetOpen}
		onClose={handleCloseSheet}
		onNavigateToRecap={handleNavigateToRecap}
	/>
</div>
