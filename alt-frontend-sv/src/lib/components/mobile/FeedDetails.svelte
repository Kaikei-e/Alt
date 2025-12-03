<script lang="ts">
import { Archive, Star, X } from "@lucide/svelte";
import { onMount } from "svelte";
import { fade, fly } from "svelte/transition";
import { browser } from "$app/environment";
import {
	archiveContentClient,
	type FeedContentOnTheFlyResponse,
	type FetchArticleSummaryResponse,
	getArticleSummaryClient,
	getFeedContentOnTheFlyClient,
	registerFavoriteFeedClient,
	summarizeArticleClient,
} from "$lib/api/client";
import { Button, buttonVariants } from "$lib/components/ui/button";
import * as Dialog from "$lib/components/ui/dialog";
import RenderFeedDetails from "./RenderFeedDetails.svelte";

interface Props {
	feedURL?: string;
	feedTitle?: string;
	initialData?: FetchArticleSummaryResponse | FeedContentOnTheFlyResponse;
}

const { feedURL, feedTitle, initialData }: Props = $props();

let isOpen = $state(false);
let isLoading = $state(false);
let isFavoriting = $state(false);
let error = $state<string | null>(null);
let isBookmarked = $state(false);
let isArchiving = $state(false);
let isArchived = $state(false);
let summary = $state<string | null>(null);
let summaryError = $state<string | null>(null);
let isSummarizing = $state(false);
// Initialize state from props (props are immutable, so this is safe)
let articleSummary = $state<FetchArticleSummaryResponse | null>(
	(() => {
		if (initialData && "matched_articles" in initialData) {
			return initialData as FetchArticleSummaryResponse;
		}
		return null;
	})(),
);
let feedDetails = $state<FeedContentOnTheFlyResponse | null>(
	(() => {
		if (initialData && "content" in initialData) {
			return initialData as FeedContentOnTheFlyResponse;
		}
		return null;
	})(),
);

// Create unique test ID based on feedURL (capture initial value)
const uniqueId = $derived(feedURL ? btoa(feedURL).slice(0, 8) : "default");

// Handle escape key to close modal
$effect(() => {
	if (!browser || !isOpen) return;

	const handleEscape = (event: KeyboardEvent) => {
		if (event.key === "Escape" && isOpen) {
			handleHideDetails();
		}
	};

	document.addEventListener("keydown", handleEscape);

	return () => {
		document.removeEventListener("keydown", handleEscape);
	};
});

const handleHideDetails = () => {
	isOpen = false;
	isArchived = false;
};

const handleShowDetails = async () => {
	isArchived = false;

	// If we already have initial data, just open the modal
	if (initialData) {
		isOpen = true;
		return;
	}

	if (!feedURL) {
		error = "No feed URL available";
		isOpen = true;
		return;
	}

	isLoading = true;
	error = null;

	// Fetch both summary and content independently
	const summaryPromise = getArticleSummaryClient(feedURL).catch((err) => {
		console.error("Error fetching article summary:", err);
		return null;
	});

	const detailsPromise = getFeedContentOnTheFlyClient(feedURL).catch((err) => {
		console.error("Error fetching article content:", err);
		return null;
	});

	try {
		const [summaryResult, detailsResult] = await Promise.all([
			summaryPromise,
			detailsPromise,
		]);

		// Check if summary has valid content
		const hasValidSummary =
			summaryResult?.matched_articles &&
			summaryResult.matched_articles.length > 0;
		// Check if details has valid content
		const hasValidDetails =
			detailsResult?.content && detailsResult.content.trim() !== "";

		if (hasValidSummary) {
			articleSummary = summaryResult;
		}

		if (hasValidDetails) {
			feedDetails = detailsResult;

			// Auto-archive article when displaying content
			// This ensures the article exists in DB before summarization
			archiveContentClient(feedURL, feedTitle).catch((err) => {
				console.warn("Failed to auto-archive article:", err);
				// Don't block UI on archive failure
			});
		}

		// If neither API call succeeded with valid content, show error
		if (!hasValidSummary && !hasValidDetails) {
			error = "Unable to fetch article content";
		}
	} catch (err) {
		console.error("Unexpected error:", err);
		error = "Unexpected error occurred";
	} finally {
		isLoading = false;
		isOpen = true;
	}
};
</script>

{#if !isOpen}
	<Button
		class="text-sm font-bold px-4 min-h-[44px] min-w-[120px] rounded-full border border-white/20 disabled:opacity-50 transition-all duration-200 hover:brightness-110 hover:-translate-y-[1px] active:scale-[0.98]"
		style="background: var(--alt-secondary); color: var(--text-primary);"
		onclick={handleShowDetails}
		data-testid="show-details-button-{uniqueId}"
		disabled={isLoading}
	>
		{isLoading ? "Loading" : "Show Details"}
	</Button>
{/if}

<Dialog.Root
	bind:open={isOpen}
	onOpenChange={(open: boolean) => {
		if (!open) handleHideDetails();
	}}
>
	<Dialog.Content
		class="max-w-[500px] h-[85vh] bg-[#dbdbdb] border border-white/10 p-0 gap-0 flex flex-col overflow-hidden rounded-2xl shadow-2xl p-4"
	>
		<!-- Header -->
		<div
			class="flex items-center justify-between p-4 border-b border-white/10 shrink-0"
		>
		<Dialog.Title class="text-lg font-bold text-black break-words line-clamp-3 pr-4">
			{feedTitle || "Article Details"}
		</Dialog.Title>
		</div>

		<!-- Content -->
		<div
			class="flex-1 overflow-y-auto p-4 scrollable-content"
			id="summary-content"
		>
			{#if feedDetails || articleSummary}
				<RenderFeedDetails
					feedDetails={feedDetails ?? articleSummary}
					isLoading={false}
					error={null}
				/>
			{:else}
				<RenderFeedDetails
					feedDetails={articleSummary || feedDetails}
					{isLoading}
					{error}
				/>
			{/if}

			<!-- Display Japanese Summary -->
			{#if summary}
				<div
					class="mt-6 p-4 rounded-xl border"
					style="
							background: var(--alt-secondary);
							border-color: var(--alt-secondary);
						"
				>
					<h3
						class="text-lg font-bold mb-3 flex items-center gap-2"
						style="color: var(--text-primary);"
					>
						Article Summary
					</h3>
					<p class="leading-relaxed" style="color: var(--text-secondary);">
						{summary}
					</p>
				</div>
			{/if}

			{#if summaryError}
				<div
					class="mt-4 p-3 rounded-lg bg-red-500/10 border border-red-500/20 text-red-400 text-sm text-center"
				>
					{summaryError}
				</div>
			{/if}
		</div>

		<!-- Footer Actions -->
		<Dialog.Footer
			class="p-4 border-t border-black/10 bg-[#dbdbdb] shrink-0 flex-row justify-end gap-3 sm:justify-end"
		>
			<Button
				variant="outline"
				size="sm"
				class="rounded-full border-white/20 text-black hover:bg-black/10 hover:text-black"
				onclick={async () => {
					if (!feedURL) return;
					isFavoriting = true;
					try {
						await registerFavoriteFeedClient(feedURL);
						// Optional: Show success toast
					} catch (e) {
						console.error("Failed to favorite feed", e);
					} finally {
						isFavoriting = false;
					}
				}}
				disabled={isFavoriting}
			>
				<Star size={14} class="mr-1.5" />
				Favorite
			</Button>

			<Button
				variant="outline"
				size="sm"
				class="rounded-full border-white/20 text-black hover:bg-black/10 hover:text-black"
				onclick={async () => {
					if (!feedURL) return;
					isArchiving = true;
					try {
						await archiveContentClient(feedURL, feedTitle);
						isArchived = true;
					} catch (e) {
						console.error("Failed to archive content", e);
					} finally {
						isArchiving = false;
					}
				}}
				disabled={isArchiving || isArchived}
			>
				<Archive size={14} class="mr-1.5" />
				{isArchiving ? "..." : isArchived ? "Saved" : "Archive"}
			</Button>

			<Button
				size="sm"
				class="rounded-full font-bold min-w-[80px] text-black hover:bg-black/10 hover:text-black"
				onclick={async () => {
					if (!feedURL) return;
					isSummarizing = true;
					summaryError = null;
					try {
						const result = await summarizeArticleClient(feedURL);
						const trimmedSummary = result.summary?.trim();

						if (trimmedSummary) {
							summary = trimmedSummary;
							summaryError = null;
						} else {
							summaryError = "Failed to get summary. Please try again.";
						}
					} catch (e) {
						console.error("Failed to summarize article", e);
						summaryError = "Failed to summarize article. Please try again.";
					} finally {
						isSummarizing = false;
					}
				}}
				disabled={isSummarizing}
			>
				{isSummarizing ? "Summarizing..." : "Summarize"}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<style>
	:global(.scrollable-content::-webkit-scrollbar) {
		width: 4px;
	}

	:global(.scrollable-content::-webkit-scrollbar-track) {
		background: transparent;
		border-radius: 2px;
	}

	:global(.scrollable-content::-webkit-scrollbar-thumb) {
		background: rgba(255, 255, 255, 0.2);
		border-radius: 2px;
	}

	:global(.scrollable-content::-webkit-scrollbar-thumb:hover) {
		background: rgba(255, 255, 255, 0.3);
	}
</style>
