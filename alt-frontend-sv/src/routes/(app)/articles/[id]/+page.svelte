<script lang="ts">
import { page } from "$app/state";
import { goto } from "$app/navigation";
import { ExternalLink, ArrowLeft, Loader2 } from "@lucide/svelte";
import { getFeedContentOnTheFlyClient } from "$lib/api/client/articles";
import RenderFeedDetails from "$lib/components/mobile/RenderFeedDetails.svelte";
import { Button } from "$lib/components/ui/button";

const articleId = $derived(page.params.id);
const articleUrl = $derived(page.url.searchParams.get("url"));

let isFetching = $state(false);
let articleContent = $state<string | null>(null);
let fetchedArticleId = $state<string | null>(null);
let contentError = $state<string | null>(null);

async function fetchContent() {
	if (!articleUrl || isFetching) return;

	isFetching = true;
	contentError = null;

	try {
		const response = await getFeedContentOnTheFlyClient(articleUrl);
		articleContent = response.content || null;
		fetchedArticleId = response.article_id || null;
	} catch (err) {
		contentError =
			err instanceof Error ? err.message : "Failed to fetch article";
	} finally {
		isFetching = false;
	}
}

$effect(() => {
	if (articleUrl) {
		fetchContent();
	}
});
</script>

<svelte:head>
	<title>Article - Alt</title>
</svelte:head>

<div class="max-w-4xl mx-auto px-4 py-6">
	<!-- Header -->
	<div class="flex items-center gap-4 mb-6">
		<Button variant="ghost" onclick={() => goto("/home")} class="flex items-center gap-2">
			<ArrowLeft class="h-4 w-4" />
			Back to Home
		</Button>
		{#if articleUrl}
			<a
				href={articleUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="ml-auto flex items-center gap-2 text-sm text-[var(--interactive-text)] hover:underline"
			>
				Open original
				<ExternalLink class="h-4 w-4" />
			</a>
		{/if}
	</div>

	<!-- Content -->
	{#if !articleUrl}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)]">
				No article URL provided. Unable to load content.
			</p>
			<Button variant="outline" onclick={() => goto("/home")} class="mt-4">
				Return to Home
			</Button>
		</div>
	{:else if isFetching}
		<div class="flex items-center justify-center py-12 gap-3">
			<Loader2 class="h-5 w-5 animate-spin text-[var(--text-secondary)]" />
			<span class="text-[var(--text-secondary)]">Loading article...</span>
		</div>
	{:else if contentError}
		<div class="text-center py-12">
			<p class="text-red-600 mb-4">{contentError}</p>
			<Button variant="outline" onclick={fetchContent}>
				Try again
			</Button>
		</div>
	{:else if articleContent}
		<div class="bg-white rounded-lg border border-[var(--surface-border)] p-6">
			<RenderFeedDetails
				feedDetails={{ content: articleContent, article_id: fetchedArticleId ?? "", og_image_url: "", og_image_proxy_url: "" }}
				error={contentError}
			/>
		</div>
	{:else}
		<div class="text-center py-12">
			<p class="text-[var(--text-secondary)]">No content available.</p>
		</div>
	{/if}
</div>
