<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useKnowledgeHome } from "$lib/hooks/useKnowledgeHome.svelte";
import { useFeatureFlags } from "$lib/hooks/useFeatureFlags.svelte";
import { useRecallRail } from "$lib/hooks/useRecallRail.svelte";
import { useLens } from "$lib/hooks/useLens.svelte";

import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import TodayBar from "$lib/components/knowledge-home/TodayBar.svelte";
import UnifiedIntentBox from "$lib/components/knowledge-home/UnifiedIntentBox.svelte";
import KnowledgeStream from "$lib/components/knowledge-home/KnowledgeStream.svelte";
import MiniRecallPanel from "$lib/components/knowledge-home/MiniRecallPanel.svelte";
import RecallRail from "$lib/components/knowledge-home/recall-rail/RecallRail.svelte";
import RecallRailCollapsible from "$lib/components/knowledge-home/recall-rail/RecallRailCollapsible.svelte";
import LensSelector from "$lib/components/knowledge-home/lens/LensSelector.svelte";
import StreamUpdateBar from "$lib/components/knowledge-home/StreamUpdateBar.svelte";
import DegradedModeBanner from "$lib/components/knowledge-home/DegradedModeBanner.svelte";

const { isDesktop } = useViewport();
const home = useKnowledgeHome();
const flags = useFeatureFlags();
const recall = useRecallRail();
const lens = useLens();

let exposureSessionId = $state("");

// Feature flag checks
const recallEnabled = $derived(flags.isEnabled("enable_recall_rail"));
const lensEnabled = $derived(flags.isEnabled("enable_lens"));

function handleAction(type: string, itemKey: string) {
	if (flags.trackingEnabled) {
		home.trackAction(type, itemKey);
	}

	if (type === "dismiss") {
		home.dismissItem(itemKey);
		return;
	}

	// Extract articleId from itemKey (format: "article:{id}")
	const articleId = itemKey.startsWith("article:") ? itemKey.slice(8) : null;

	if (type === "open" && articleId) {
		goto(`/feeds?article=${articleId}`);
	} else if (type === "ask") {
		goto("/augur");
	}
}

function handleRecallOpen(itemKey: string) {
	const articleId = itemKey.startsWith("article:") ? itemKey.slice(8) : null;
	if (articleId) {
		goto(`/feeds?article=${articleId}`);
	}
}

function handleItemsVisible(itemKeys: string[]) {
	if (exposureSessionId && flags.trackingEnabled) {
		home.trackSeen(itemKeys, exposureSessionId);
	}
}

function handleLensSelect(lensId: string | null) {
	lens.select(lensId);
	// Re-fetch with the new lens
	home.fetchData(true);
}

onMount(async () => {
	if (browser) {
		exposureSessionId = crypto.randomUUID();
		await home.fetchData(true);

		// Fetch recall candidates if enabled
		if (recallEnabled) {
			recall.fetchCandidates();
		}

		// Fetch lenses if enabled
		if (lensEnabled) {
			lens.fetchLenses();
		}
	}
});
</script>

<svelte:head>
	<title>Knowledge Home - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader
		title="Knowledge Home"
		description="Today's knowledge starting point"
	/>

	{#if home.degraded}
		<div class="mb-3">
			<DegradedModeBanner />
		</div>
	{/if}

	<TodayBar digest={home.digest} />
	<UnifiedIntentBox />

	{#if lensEnabled}
		<div class="mt-3">
			<LensSelector
				lenses={lens.lenses}
				activeLensId={lens.activeLensId}
				onSelect={handleLensSelect}
				onCreateClick={() => {/* TODO: open LensModal */}}
			/>
		</div>
	{/if}

	<div class="flex gap-6 mt-4">
		<div class="flex-1 min-w-0">
			<KnowledgeStream
				items={home.items}
				loading={home.loading}
				hasMore={home.hasMore}
				onAction={handleAction}
				onLoadMore={() => home.loadMore()}
				onItemsVisible={handleItemsVisible}
			/>
		</div>
		<div class="w-80 flex-shrink-0">
			{#if recallEnabled && recall.candidates.length > 0}
				<RecallRail
					candidates={recall.candidates}
					onSnooze={(key: string) => recall.snooze(key)}
					onDismiss={(key: string) => recall.dismiss(key)}
					onOpen={handleRecallOpen}
				/>
			{:else}
				<MiniRecallPanel digest={home.digest} />
			{/if}
		</div>
	</div>
{:else}
	<!-- Mobile: Compact layout -->
	<div class="min-h-[100dvh]" style="background: var(--app-bg);">
		{#if home.degraded}
			<div class="px-3 pt-2">
				<DegradedModeBanner />
			</div>
		{/if}

		<TodayBar digest={home.digest} />
		<UnifiedIntentBox />

		{#if recallEnabled}
			<div class="px-3 pt-2">
				<RecallRailCollapsible
					candidates={recall.candidates}
					onSnooze={(key: string) => recall.snooze(key)}
					onDismiss={(key: string) => recall.dismiss(key)}
					onOpen={handleRecallOpen}
				/>
			</div>
		{/if}

		{#if lensEnabled}
			<div class="px-3 pt-2">
				<LensSelector
					lenses={lens.lenses}
					activeLensId={lens.activeLensId}
					onSelect={handleLensSelect}
					onCreateClick={() => {/* TODO: open LensModal */}}
				/>
			</div>
		{/if}

		<div class="p-3">
			<KnowledgeStream
				items={home.items}
				loading={home.loading}
				hasMore={home.hasMore}
				onAction={handleAction}
				onLoadMore={() => home.loadMore()}
				onItemsVisible={handleItemsVisible}
			/>
		</div>
	</div>
{/if}
