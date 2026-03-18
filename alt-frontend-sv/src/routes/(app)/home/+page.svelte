<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useKnowledgeHome } from "$lib/hooks/useKnowledgeHome.svelte";
import { useFeatureFlags } from "$lib/hooks/useFeatureFlags.svelte";
import { useRecallRail } from "$lib/hooks/useRecallRail.svelte";
import { useLens } from "$lib/hooks/useLens.svelte";
import { useTtsPlayback } from "$lib/hooks/useTtsPlayback.svelte";
import type { KnowledgeHomeItemData, LensVersionData } from "$lib/connect/knowledge_home";

import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import TodayBar from "$lib/components/knowledge-home/TodayBar.svelte";
import UnifiedIntentBox from "$lib/components/knowledge-home/UnifiedIntentBox.svelte";
import KnowledgeStream from "$lib/components/knowledge-home/KnowledgeStream.svelte";
import MiniRecallPanel from "$lib/components/knowledge-home/MiniRecallPanel.svelte";
import RecallRail from "$lib/components/knowledge-home/recall-rail/RecallRail.svelte";
import RecallRailCollapsible from "$lib/components/knowledge-home/recall-rail/RecallRailCollapsible.svelte";
import LensSelector from "$lib/components/knowledge-home/lens/LensSelector.svelte";
import LensModal from "$lib/components/knowledge-home/lens/LensModal.svelte";
import DegradedModeBanner from "$lib/components/knowledge-home/DegradedModeBanner.svelte";

const { isDesktop } = useViewport();
const home = useKnowledgeHome();
const flags = useFeatureFlags();
const recall = useRecallRail();
const lens = useLens();
const tts = useTtsPlayback();

let exposureSessionId = $state("");
let lensModalOpen = $state(false);

// Feature flag checks
const recallEnabled = $derived(flags.isEnabled("enable_recall_rail"));
const lensEnabled = $derived(flags.isEnabled("enable_lens"));
const activeLensName = $derived(
	lens.activeLensId
		? lens.lenses.find((entry) => entry.lensId === lens.activeLensId)?.name ?? null
		: null,
);

async function syncLensQuery(lensId: string | null) {
	const url = new URL(page.url);
	if (lensId) {
		url.searchParams.set("lens", lensId);
	} else {
		url.searchParams.delete("lens");
	}
	await goto(`${url.pathname}${url.search}`, {
		replaceState: true,
		noScroll: true,
		keepFocus: true,
	});
}

function handleAction(type: string, item: KnowledgeHomeItemData) {
	const itemKey = item.itemKey;

	if (flags.trackingEnabled) {
		const metadata = JSON.stringify({
			articleId: item.articleId,
			title: item.title,
			summaryExcerpt: item.summaryExcerpt,
		});
		home.trackAction(type, itemKey, metadata);
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
		const context = item.summaryExcerpt || item.title;
		if (context) {
			goto(`/augur?context=${encodeURIComponent(context)}`);
		} else {
			goto("/augur");
		}
	} else if (type === "listen") {
		const text = item.summaryExcerpt || item.title;
		if (text) {
			tts.play(text);
		}
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

async function handleLensSelect(lensId: string | null) {
	await lens.select(lensId);
	await syncLensQuery(lensId);
	// Re-fetch with the new lens
	await home.fetchData(true, lensId);
}

async function handleCreateLens(payload: {
	name: string;
	description: string;
	version: Omit<LensVersionData, "versionId">;
}) {
	const created = await lens.create(payload.name, payload.description, payload.version);
	if (!created) {
		return;
	}
	await handleLensSelect(created.lensId);
}

onMount(async () => {
	if (browser) {
		exposureSessionId = crypto.randomUUID();

		// Fetch recall candidates if enabled
		if (recallEnabled) {
			recall.fetchCandidates();
		}

		// Fetch lenses if enabled
		if (lensEnabled) {
			await lens.fetchLenses();
			const urlLensId = page.url.searchParams.get("lens");
			const initialLensId = urlLensId ?? lens.activeLensId;
			if (urlLensId && urlLensId !== lens.activeLensId) {
				await lens.select(urlLensId);
			}
			await syncLensQuery(initialLensId);
			await home.fetchData(true, initialLensId);
			return;
		}

		await home.fetchData(true);
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
				onCreateClick={() => {
					lensModalOpen = true;
				}}
			/>
		</div>
	{/if}

	<div class="flex gap-6 mt-4">
		<div class="flex-1 min-w-0">
			<KnowledgeStream
				items={home.items}
				loading={home.loading}
				hasMore={home.hasMore}
				{activeLensName}
				onAction={handleAction}
				onLoadMore={() => home.loadMore(lens.activeLensId)}
				onItemsVisible={handleItemsVisible}
				onClearLens={() => handleLensSelect(null)}
			/>
		</div>
		<div class="w-80 flex-shrink-0">
			{#if recallEnabled}
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
					onCreateClick={() => {
						lensModalOpen = true;
					}}
				/>
			</div>
		{/if}

		<div class="p-3">
			<KnowledgeStream
				items={home.items}
				loading={home.loading}
				hasMore={home.hasMore}
				{activeLensName}
				onAction={handleAction}
				onLoadMore={() => home.loadMore(lens.activeLensId)}
				onItemsVisible={handleItemsVisible}
				onClearLens={() => handleLensSelect(null)}
			/>
		</div>
	</div>
{/if}

{#if lensEnabled}
	<LensModal
		open={lensModalOpen}
		onOpenChange={(open: boolean) => {
			lensModalOpen = open;
		}}
		onSave={handleCreateLens}
	/>
{/if}
