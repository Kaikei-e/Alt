<script lang="ts">
import { onMount } from "svelte";
import { browser } from "$app/environment";
import { goto } from "$app/navigation";
import { page } from "$app/state";
import AskSheet from "$lib/components/knowledge-home/AskSheet.svelte";
import DegradedModeBanner from "$lib/components/knowledge-home/DegradedModeBanner.svelte";
import KnowledgeStream from "$lib/components/knowledge-home/KnowledgeStream.svelte";
import ListenQueueBar from "$lib/components/knowledge-home/ListenQueueBar.svelte";
import Toast from "$lib/components/knowledge-home/Toast.svelte";
import LensModal from "$lib/components/knowledge-home/lens/LensModal.svelte";
import LensSelector from "$lib/components/knowledge-home/lens/LensSelector.svelte";
import RecallRail from "$lib/components/knowledge-home/recall-rail/RecallRail.svelte";
import StreamUpdateBar from "$lib/components/knowledge-home/StreamUpdateBar.svelte";
import TodayBar from "$lib/components/knowledge-home/TodayBar.svelte";
import UnifiedIntentBox from "$lib/components/knowledge-home/UnifiedIntentBox.svelte";
import MobileKnowledgeHomeHeader from "$lib/components/knowledge-home/MobileKnowledgeHomeHeader.svelte";
import GlobalSearchEntry from "$lib/components/knowledge-home/GlobalSearchEntry.svelte";
import MobileRecallSection from "$lib/components/knowledge-home/MobileRecallSection.svelte";
import {
	createClientTransport,
	listSubscriptions,
	type ConnectFeedSource,
} from "$lib/connect";
import type {
	KnowledgeHomeItemData,
	LensVersionData,
} from "$lib/connect/knowledge_home";
import type { TagSuggestion } from "$lib/components/knowledge-home/lens/TagCombobox.svelte";
import { useKnowledgeHome } from "$lib/hooks/useKnowledgeHome.svelte";
import { useLens } from "$lib/hooks/useLens.svelte";
import { useRecallRail } from "$lib/hooks/useRecallRail.svelte";
import { useStreamUpdates } from "$lib/hooks/useStreamUpdates.svelte";
import { useTtsPlayback } from "$lib/hooks/useTtsPlayback.svelte";
import { useToastStore } from "$lib/stores/toast.svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import { buildHomeActionMetadata } from "./home-actions";
import { refreshHomeWithRecallSync } from "./stream-refresh";

const { isDesktop } = useViewport();
const home = useKnowledgeHome();
const recall = useRecallRail();
const lens = useLens();
const tts = useTtsPlayback();
const toast = useToastStore();

let revealed = $state(false);
let exposureSessionId = $state("");
let lensModalOpen = $state(false);
let bannerDismissed = $state(false);
let askSheetOpen = $state(false);
let askScopeTitle = $state("");
let askScopeContext = $state("");
let askScopeArticleId = $state<string | undefined>(undefined);
let askScopeTags = $state<string[]>([]);
let searchQuery = $state("");
let listenQueue = $state<{ id: string; title: string; text: string }[]>([]);
let isQueueProcessing = $state(false);
let lensSources = $state<ConnectFeedSource[]>([]);
let lensSourcesLoading = $state(false);
let lensDraft = $state<Omit<LensVersionData, "versionId">>({
	queryText: "",
	tagIds: [],
	sourceIds: [],
	timeWindow: "7d",
	includeRecap: false,
	includePulse: false,
	sortMode: "relevance",
});

const activeLensName = $derived(
	lens.activeLensId
		? (lens.lenses.find((entry) => entry.lensId === lens.activeLensId)?.name ??
				null)
		: null,
);
const lensMatchCount = $derived(lens.activeLensId ? home.items.length : null);
const showBanner = $derived(
	!bannerDismissed &&
		(home.pageState === "degraded" || home.pageState === "fallback"),
);
const streamMode = $derived(lens.activeLensId ? "lens" : "default");
const visibleItems = $derived(home.items);
const emptyReason = $derived.by(() => {
	if (home.pageState === "degraded" && visibleItems.length === 0) {
		return "degraded";
	}
	return home.emptyReason;
});
const currentQueueTitle = $derived(listenQueue[0]?.title ?? null);
const lensTagSuggestions = $derived.by((): TagSuggestion[] => {
	const tagCounts = new Map<string, number>();
	for (const item of home.items) {
		for (const tag of item.tags ?? []) {
			tagCounts.set(tag, (tagCounts.get(tag) ?? 0) + 1);
		}
	}
	for (const tag of home.digest?.topTags ?? []) {
		if (!tagCounts.has(tag)) tagCounts.set(tag, 0);
	}
	return Array.from(tagCounts.entries())
		.map(([name, count]): TagSuggestion => ({ name, count }))
		.sort((a, b) => (b.count ?? 0) - (a.count ?? 0));
});

const dateStr = new Date().toLocaleDateString("en-US", {
	weekday: "long",
	year: "numeric",
	month: "long",
	day: "numeric",
});

const stream = useStreamUpdates({
	get enabled() {
		return true;
	},
	get lensId() {
		return lens.activeLensId ?? undefined;
	},
	onRefresh: () => refreshHomeWithRecallSync(home, recall, lens.activeLensId),
});

async function processQueue() {
	if (isQueueProcessing || listenQueue.length === 0) return;
	isQueueProcessing = true;

	while (listenQueue.length > 0) {
		const current = listenQueue[0];
		try {
			await tts.play(current.text);
		} catch {
			toast.push("Listen playback failed.", "error", 3000);
			break;
		}
		listenQueue = listenQueue.slice(1);
	}

	isQueueProcessing = false;
}

function enqueueListen(title: string, text: string) {
	if (!text.trim()) {
		toast.push("No audio-ready summary is available yet.", "error", 3000);
		return;
	}
	listenQueue = [...listenQueue, { id: crypto.randomUUID(), title, text }];
	toast.push("Added to listen queue.", "success");
	void processQueue();
}

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
	const metadata = buildHomeActionMetadata(type, item);

	if (type === "dismiss") {
		home.trackAction(type, itemKey, metadata);
		home.dismissItem(itemKey);
		toast.push("Dismissed from the current stream.", "success");
		return;
	}

	home.trackAction(type, itemKey, metadata);

	const articleId = itemKey.startsWith("article:") ? itemKey.slice(8) : null;

	if (type === "open" && articleId) {
		if (item.link) {
			const params = new URLSearchParams({ url: item.link });
			if (item.title) params.set("title", item.title);
			goto(`/articles/${articleId}?${params.toString()}`);
		} else {
			toast.push("Article link is not available yet.", "error", 3000);
		}
		return;
	}

	if (type === "ask") {
		askScopeTitle = item.title;
		askScopeContext = item.title;
		askScopeArticleId = item.articleId;
		askScopeTags = item.tags?.slice(0, 3) ?? [];
		askSheetOpen = true;
		return;
	}

	if (type === "listen") {
		enqueueListen(item.title, item.summaryExcerpt || item.title);
	}
}

function handleTagClick(tag: string, item: KnowledgeHomeItemData) {
	home.trackAction("tag_click", item.itemKey, JSON.stringify({ tag }));
}

function handleRecallOpen(itemKey: string) {
	home.trackAction("open", itemKey);
	const articleId = itemKey.startsWith("article:") ? itemKey.slice(8) : null;
	if (!articleId) return;
	const candidate = recall.candidates.find((c) => c.itemKey === itemKey);
	const link = candidate?.item?.link;
	if (link) {
		const params = new URLSearchParams({ url: link });
		if (candidate?.item?.title) params.set("title", candidate.item.title);
		goto(`/articles/${articleId}?${params.toString()}`);
	}
}

function handleItemsVisible(itemKeys: string[]) {
	if (exposureSessionId) {
		home.trackSeen(itemKeys, exposureSessionId);
	}
}

function handleSearchSubmit(query: string) {
	goto(`/search?q=${encodeURIComponent(query)}`);
}

function handleSearchClear() {
	searchQuery = "";
	lensDraft.queryText = "";
}

function handleAskFromHome(query: string) {
	askScopeTitle = query.trim() ? query : "Knowledge Home";
	askScopeContext = query.trim();
	askScopeArticleId = undefined;
	askScopeTags = [];
	askSheetOpen = true;
}

function syncRecallState() {
	recall.setCandidates(home.recallCandidates);
}

async function handleLensSelect(lensId: string | null) {
	await lens.select(lensId);
	const selectedLens = lensId
		? lens.lenses.find((entry) => entry.lensId === lensId)
		: null;
	if (selectedLens?.currentVersion) {
		searchQuery = selectedLens.currentVersion.queryText;
		lensDraft = {
			queryText: selectedLens.currentVersion.queryText,
			tagIds: [...selectedLens.currentVersion.tagIds],
			sourceIds: [...selectedLens.currentVersion.sourceIds],
			timeWindow: selectedLens.currentVersion.timeWindow || "7d",
			includeRecap: selectedLens.currentVersion.includeRecap,
			includePulse: selectedLens.currentVersion.includePulse,
			sortMode: selectedLens.currentVersion.sortMode || "relevance",
		};
	} else if (lensId === null) {
		searchQuery = "";
		lensDraft = {
			queryText: "",
			tagIds: [],
			sourceIds: [],
			timeWindow: "7d",
			includeRecap: false,
			includePulse: false,
			sortMode: "relevance",
		};
	}
	await syncLensQuery(lensId);
	await home.fetchData(true, lensId);
	syncRecallState();
}

async function handleCreateLens(payload: {
	name: string;
	description: string;
	version: Omit<LensVersionData, "versionId">;
}) {
	const created = await lens.create(
		payload.name,
		payload.description,
		payload.version,
	);
	if (!created) {
		return;
	}
	await handleLensSelect(created.lensId);
}

async function loadLensSources() {
	try {
		lensSourcesLoading = true;
		const transport = createClientTransport();
		lensSources = await listSubscriptions(transport);
	} catch {
		lensSources = [];
	} finally {
		lensSourcesLoading = false;
	}
}

onMount(async () => {
	if (browser) {
		requestAnimationFrame(() => {
			revealed = true;
		});
		exposureSessionId = crypto.randomUUID();

		await home.fetchData(true);

		await loadLensSources();
		await lens.fetchLenses();
		const urlLensId = page.url.searchParams.get("lens");
		const initialLensId = urlLensId ?? lens.activeLensId;
		if (urlLensId && urlLensId !== lens.activeLensId) {
			await lens.select(urlLensId);
		}
		if (initialLensId) {
			const initialLens = lens.lenses.find(
				(entry) => entry.lensId === initialLensId,
			);
			if (initialLens?.currentVersion) {
				searchQuery = initialLens.currentVersion.queryText;
				lensDraft = {
					queryText: initialLens.currentVersion.queryText,
					tagIds: [...initialLens.currentVersion.tagIds],
					sourceIds: [...initialLens.currentVersion.sourceIds],
					timeWindow: initialLens.currentVersion.timeWindow || "7d",
					includeRecap: initialLens.currentVersion.includeRecap,
					includePulse: initialLens.currentVersion.includePulse,
					sortMode: initialLens.currentVersion.sortMode || "relevance",
				};
			}
		}
		await syncLensQuery(initialLensId);
		if (initialLensId) {
			await home.fetchData(true, initialLensId);
		}

		syncRecallState();
	}
});
</script>

<svelte:head>
	<title>Knowledge Home - Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="reading-desk" class:revealed>
		<!-- Editorial Header -->
		<header class="desk-header">
			<span class="desk-date">{dateStr}</span>
			<h1 class="desk-title">Knowledge Home</h1>
			<p class="desk-subtitle">Today's knowledge starting point</p>
			<div class="desk-rule" aria-hidden="true"></div>
		</header>

		{#if showBanner}
			<div class="section-reveal" style="--delay: 1;">
				<DegradedModeBanner
					serviceQuality={home.serviceQuality}
					onDismiss={() => {
						bannerDismissed = true;
					}}
				/>
			</div>
		{/if}

		<div class="section-reveal" style="--delay: 1;">
			<TodayBar digest={home.digest} serviceQuality={home.serviceQuality} />
		</div>

		<div class="section-reveal" style="--delay: 2;">
			<UnifiedIntentBox
				query={searchQuery}
				onSearchSubmit={handleSearchSubmit}
				onSearchClear={handleSearchClear}
				onAsk={handleAskFromHome}
			/>
		</div>

		<div class="section-reveal" style="--delay: 3;">
			<LensSelector
				lenses={lens.lenses}
				activeLensId={lens.activeLensId}
				matchCount={lensMatchCount}
				onSelect={handleLensSelect}
				onCreateClick={() => {
					lensModalOpen = true;
				}}
			/>
		</div>

		<div class="section-reveal" style="--delay: 3;">
			<StreamUpdateBar
				pendingCount={stream.pendingCount}
				isConnected={stream.isConnected}
				isFallback={stream.isFallback}
				onApply={() => stream.applyUpdates()}
			/>
		</div>

		<div class="desk-columns section-reveal" style="--delay: 4;">
			<div class="min-w-0 flex-1">
				<KnowledgeStream
					items={visibleItems}
					loading={home.loading}
					hasMore={home.hasMore}
					{activeLensName}
					emptyReason={emptyReason}
					streamMode={streamMode}
					searchQuery={searchQuery}
					onAction={handleAction}
					onTagClick={handleTagClick}
					onLoadMore={() => home.loadMore(lens.activeLensId)}
					onItemsVisible={handleItemsVisible}
					onClearLens={() => handleLensSelect(null)}
				/>
			</div>
			<div class="desk-rail">
				<RecallRail
					candidates={recall.candidates}
					unavailable={Boolean(recall.error)}
					onSnooze={(key: string) => recall.snooze(key)}
					onDismiss={(key: string) => recall.dismiss(key)}
					onOpen={handleRecallOpen}
				/>
			</div>
		</div>
	</div>
{:else}
	<div style="background: var(--app-bg);">
		<MobileKnowledgeHomeHeader
			serviceQuality={home.serviceQuality}
		/>

		{#if showBanner}
			<div class="px-3 pt-2">
				<DegradedModeBanner
					serviceQuality={home.serviceQuality}
					onDismiss={() => {
						bannerDismissed = true;
					}}
				/>
			</div>
		{/if}

		<TodayBar digest={home.digest} serviceQuality={home.serviceQuality} />

		<GlobalSearchEntry onAsk={handleAskFromHome} />

		<div class="px-3 pt-2">
			<LensSelector
				lenses={lens.lenses}
				activeLensId={lens.activeLensId}
				matchCount={lensMatchCount}
				onSelect={handleLensSelect}
				onCreateClick={() => {
					lensModalOpen = true;
				}}
			/>
		</div>

		<MobileRecallSection
			candidates={recall.candidates}
			unavailable={Boolean(recall.error)}
			onSnooze={(key: string) => recall.snooze(key)}
			onDismiss={(key: string) => recall.dismiss(key)}
			onOpen={handleRecallOpen}
		/>

		<div class="px-3 pt-2">
			<StreamUpdateBar
				pendingCount={stream.pendingCount}
				isConnected={stream.isConnected}
				isFallback={stream.isFallback}
				onApply={() => stream.applyUpdates()}
			/>
		</div>

		<div class="p-3">
			<KnowledgeStream
				items={visibleItems}
				loading={home.loading}
				hasMore={home.hasMore}
				{activeLensName}
				emptyReason={emptyReason}
				streamMode={streamMode}
				searchQuery={searchQuery}
				onAction={handleAction}
				onTagClick={handleTagClick}
				onLoadMore={() => home.loadMore(lens.activeLensId)}
				onItemsVisible={handleItemsVisible}
				onClearLens={() => handleLensSelect(null)}
			/>
		</div>
	</div>

{/if}

<AskSheet
	open={askSheetOpen}
	scopeTitle={askScopeTitle}
	scopeContext={askScopeContext}
	scopeArticleId={askScopeArticleId}
	scopeTags={askScopeTags}
	onClose={() => {
		askSheetOpen = false;
	}}
	onConversationId={(id) => {
		// Expose the persisted Augur conversation as a bookmarkable URL so a
		// premature sheet close no longer hides the saved conversation.
		if (typeof history !== "undefined") {
			history.replaceState(history.state, "", `/augur/${id}`);
		}
	}}
/>

<ListenQueueBar
	queue={listenQueue}
	currentTitle={currentQueueTitle}
	isPlaying={tts.isPlaying || isQueueProcessing}
	onToggle={() => {
		if (tts.isPlaying) {
			tts.stop();
			isQueueProcessing = false;
		} else {
			void processQueue();
		}
	}}
	onClear={() => {
		tts.stop();
		listenQueue = [];
		isQueueProcessing = false;
	}}
/>

<Toast items={toast.items} onDismiss={toast.remove} />

<LensModal
	open={lensModalOpen}
	version={lensDraft}
	availableSources={lensSources}
	availableTags={lensTagSuggestions}
	loadingSources={lensSourcesLoading}
	onOpenChange={(open: boolean) => {
		lensModalOpen = open;
	}}
	onSave={handleCreateLens}
/>

<style>
	/* ── Page reveal ── */
	.reading-desk {
		max-width: 1400px;
		opacity: 0;
		transform: translateY(6px);
		transition:
			opacity 0.4s ease,
			transform 0.4s ease;
	}

	.reading-desk.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	/* ── Editorial header ── */
	.desk-header {
		padding: 1.5rem 0 0;
	}

	.desk-date {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		color: var(--alt-ash);
		letter-spacing: 0.06em;
	}

	.desk-title {
		font-family: var(--font-display);
		font-size: 1.6rem;
		font-weight: 800;
		color: var(--alt-charcoal);
		letter-spacing: -0.01em;
		margin: 0.15rem 0 0;
		line-height: 1.2;
	}

	.desk-subtitle {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-ash);
		margin-top: 0.2rem;
	}

	.desk-rule {
		height: 1px;
		background: var(--surface-border);
		margin-top: 0.75rem;
	}

	/* ── Two-column layout ── */
	.desk-columns {
		display: flex;
		gap: 2rem;
		margin-top: 1.5rem;
	}

	.desk-rail {
		width: 20rem;
		flex-shrink: 0;
	}

	/* ── Section stagger ── */
	.section-reveal {
		opacity: 0;
		transform: translateY(6px);
		animation: reveal 0.4s ease forwards;
		animation-delay: calc(var(--delay) * 100ms);
		margin-top: 0.75rem;
	}

	@keyframes reveal {
		to {
			opacity: 1;
			transform: translateY(0);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.reading-desk {
			opacity: 1;
			transform: none;
			transition: none;
		}

		.section-reveal {
			animation: none;
			opacity: 1;
			transform: none;
		}
	}
</style>
