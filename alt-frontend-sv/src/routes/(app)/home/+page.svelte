<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useKnowledgeHome } from "$lib/hooks/useKnowledgeHome.svelte";
import { useFeatureFlags } from "$lib/hooks/useFeatureFlags.svelte";

import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import TodayBar from "$lib/components/knowledge-home/TodayBar.svelte";
import KnowledgeStream from "$lib/components/knowledge-home/KnowledgeStream.svelte";
import MiniRecallPanel from "$lib/components/knowledge-home/MiniRecallPanel.svelte";
import DegradedModeBanner from "$lib/components/knowledge-home/DegradedModeBanner.svelte";

const { isDesktop } = useViewport();
const home = useKnowledgeHome();
const flags = useFeatureFlags();

let exposureSessionId = $state("");

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

function handleItemsVisible(itemKeys: string[]) {
	if (exposureSessionId && flags.trackingEnabled) {
		home.trackSeen(itemKeys, exposureSessionId);
	}
}

onMount(async () => {
	if (browser) {
		exposureSessionId = crypto.randomUUID();
		await home.fetchData(true);

		// Update feature flags from the response
		// The flags come from the KnowledgeHome response featureFlags field
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
		<div class="w-72 flex-shrink-0">
			<MiniRecallPanel digest={home.digest} />
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
