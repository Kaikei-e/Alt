<script lang="ts">
import DesktopTagArticlesScreen from "#lib/components/desktop/articles/DesktopTagArticlesScreen.svelte";
import MobileTagArticlesScreen from "#lib/components/mobile/articles/MobileTagArticlesScreen.svelte";
import {
	createClientTransport,
	fetchArticlesByTag,
	type TagTrailArticle,
} from "#lib/connect/index.js";
import { isDesktop } from "#lib/stores/viewport.svelte.js";
import { browser } from "$app/env";
import { page } from "$app/state";

const tagName = $derived(page.url.searchParams.get("tag") ?? "");

let articles = $state<TagTrailArticle[]>([]);
let isLoading = $state(true);
let error = $state<string | null>(null);
let nextCursor = $state<string | null>(null);
let hasMore = $state(false);
let isLoadingMore = $state(false);
let selectedArticle = $state<TagTrailArticle | null>(null);
let loadedTag = $state<string | null>(null);

async function loadArticles(tag: string, cursor?: string) {
	if (!browser) return;
	try {
		const transport = createClientTransport();
		const result = await fetchArticlesByTag(
			transport,
			tag,
			undefined,
			cursor ?? undefined,
			20,
		);
		if (cursor) {
			articles = [...articles, ...result.articles];
		} else {
			articles = result.articles;
		}
		nextCursor = result.nextCursor;
		hasMore = result.hasMore;
	} catch (e) {
		error = e instanceof Error ? e.message : "Failed to load articles";
	}
}

async function loadMore() {
	if (!nextCursor || isLoadingMore || !tagName) return;
	isLoadingMore = true;
	await loadArticles(tagName, nextCursor);
	isLoadingMore = false;
}

$effect(() => {
	const currentTag = tagName;
	if (!currentTag) {
		loadedTag = null;
		articles = [];
		isLoading = false;
		error = null;
		nextCursor = null;
		hasMore = false;
		selectedArticle = null;
		return;
	}
	if (currentTag === loadedTag) return;
	loadedTag = currentTag;
	isLoading = true;
	error = null;
	nextCursor = null;
	hasMore = false;
	selectedArticle = null;
	void loadArticles(currentTag).finally(() => {
		isLoading = false;
	});
});
</script>

<svelte:head>
	<title>{tagName ? `${tagName} - Articles` : "Tag Articles"} - Alt</title>
</svelte:head>

{#if !tagName}
	<div class="flex items-center justify-center h-full p-8 text-center">
		<p class="text-[var(--text-secondary)]">No tag specified. Click a tag from Knowledge Home to browse articles.</p>
	</div>
{:else if isDesktop()}
	<DesktopTagArticlesScreen
		{tagName}
		{articles}
		{isLoading}
		{error}
		{hasMore}
		{isLoadingMore}
		bind:selectedArticle
		onLoadMore={loadMore}
	/>
{:else}
	<MobileTagArticlesScreen
		{tagName}
		{articles}
		{isLoading}
		{error}
		{hasMore}
		{isLoadingMore}
		onLoadMore={loadMore}
	/>
{/if}
