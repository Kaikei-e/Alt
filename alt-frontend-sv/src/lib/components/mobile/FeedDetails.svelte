<script lang="ts">
import { Archive, Star, X } from "@lucide/svelte";
import { onMount } from "svelte";
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
import { Button } from "$lib/components/ui/button";
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

{#if isOpen}
	<!-- Modal Backdrop -->
	<div
		class="fixed inset-0 z-[9999] flex items-center justify-center p-4"
		style="
				background: rgba(0, 0, 0, 0.6);
				backdrop-filter: blur(12px);
				touch-action: manipulation;
			"
		data-testid="modal-backdrop"
		role="dialog"
		aria-modal="true"
		aria-labelledby="summary-header"
		aria-describedby="summary-content"
		tabindex="-1"
		onclick={(e) => {
			if (e.target === e.currentTarget) {
				handleHideDetails();
			}
		}}
		onkeydown={(e) => {
			if (e.key === "Escape") {
				handleHideDetails();
			}
		}}
		ontouchend={(e) => {
			if (e.target === e.currentTarget) {
				e.preventDefault();
				handleHideDetails();
			}
		}}
	>
		<!-- Modal Content -->
		<div
			class="w-[95vw] max-w-[450px] h-[85vh] max-h-[700px] min-h-[400px] rounded-2xl border flex flex-col overflow-hidden"
			style="
					background: var(--app-bg);
					box-shadow: 0 20px 40px rgba(0, 0, 0, 0.3);
					border-color: rgba(255, 255, 255, 0.1);
					padding-bottom: env(safe-area-inset-bottom, 0px);
				"
			data-testid="modal-content"
			role="region"
			aria-label="Article summary content"
		>
			<!-- Header -->
			<div
				class="sticky top-0 z-[2] h-[60px] min-h-[60px] backdrop-blur-[20px] border-b px-4 py-3 flex items-center justify-center rounded-t-2xl"
				style="
						background: rgba(255, 255, 255, 0.05);
						border-color: rgba(255, 255, 255, 0.1);
					"
				data-testid="summary-header"
				id="summary-header"
			>
				<p
					class="font-bold text-base"
					style="
							color: var(--text-primary);
							text-shadow: 0 2px 4px var(--alt-glass-shadow);
						"
				>
					Article Summary
				</p>
			</div>

			<!-- Content -->
			<div
				class="flex-1 overflow-auto px-0 py-0 scroll-smooth overscroll-contain will-change-[scroll-position]"
				data-testid="scrollable-content"
				id="summary-content"
				style="background: transparent;"
			>
				{#if feedDetails || articleSummary}
					<div class="h-full overflow-y-auto scrollable-content">
						<RenderFeedDetails
							feedDetails={feedDetails ?? articleSummary}
							isLoading={false}
							error={null}
						/>
					</div>
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
						class="mt-4 px-4 py-4 rounded-xl border mx-4 mb-4"
						style="
								background: rgba(255, 255, 255, 0.03);
								border-color: rgba(255, 255, 255, 0.1);
							"
					>
						<p
							class="text-xs font-bold mb-2 uppercase tracking-wider"
							style="color: var(--text-secondary);"
						>
							日本語要約 / Japanese Summary
						</p>
						<p
							class="text-sm leading-relaxed whitespace-pre-wrap"
							style="color: var(--text-primary);"
						>
							{summary}
						</p>
					</div>
				{/if}

				{#if summaryError}
					<div
						class="mt-4 px-4 py-4 rounded-xl border mx-4 mb-4"
						style="
								background: rgba(255, 99, 71, 0.12);
								border-color: rgba(255, 255, 255, 0.1);
							"
					>
						<p
							class="text-xs font-bold mb-2 uppercase tracking-wider"
							style="color: var(--text-secondary);"
						>
							要約エラー / Summary Error
						</p>
						<p
							class="text-sm leading-relaxed"
							style="color: var(--text-primary);"
						>
							{summaryError}
						</p>
					</div>
				{/if}
			</div>

			<!-- Footer -->
			<div
				class="sticky bottom-0 z-[2] backdrop-blur-[20px] border-t px-3 py-3 rounded-b-2xl flex items-center justify-between min-h-[60px] gap-2"
				style="
						background: rgba(255, 255, 255, 0.05);
						border-color: rgba(255, 255, 255, 0.1);
					"
			>
				<Button
					class="rounded-full p-2 min-h-[36px] min-w-[36px] text-sm font-bold border border-white/20 disabled:opacity-50"
					style="background: var(--alt-primary); color: var(--text-primary);"
					onclick={async () => {
						if (!feedURL) return;
						try {
							isFavoriting = true;
							await registerFavoriteFeedClient(feedURL);
							isBookmarked = true;
						} catch (e) {
							console.error("Failed to favorite feed", e);
						} finally {
							isFavoriting = false;
						}
					}}
					disabled={isFavoriting || isBookmarked}
					title="Favorite"
				>
					<Star size={16} />
				</Button>
				<Button
					class="rounded-full px-3 min-h-[36px] text-xs font-bold border border-white/20 disabled:opacity-50"
					style="background: var(--alt-primary); color: var(--text-primary);"
					onclick={async () => {
						if (!feedURL) return;
						try {
							isArchiving = true;
							await archiveContentClient(feedURL, feedTitle);
							isArchived = true;
						} catch (e) {
							console.error("Error archiving feed:", e);
						} finally {
							isArchiving = false;
						}
					}}
					disabled={isArchiving || isArchived}
					title="Archive"
				>
					<Archive size={14} style="margin-right: 4px;" />
					{isArchiving ? "..." : isArchived ? "✓" : "Archive"}
				</Button>
				<Button
					class="rounded-full px-3 min-h-[36px] text-xs font-bold border border-white/20 disabled:opacity-50"
					style="background: var(--alt-secondary); color: var(--text-primary);"
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
								summaryError = "要約を取得できませんでした。";
							}
						} catch (e) {
							console.error("Failed to summarize article", e);
							summaryError =
								"要約の生成に失敗しました。もう一度お試しください。";
						} finally {
							isSummarizing = false;
						}
					}}
					disabled={isSummarizing}
					title="Summarize to Japanese"
				>
					{isSummarizing ? "要約中..." : "要約"}
				</Button>
				<Button
					class="rounded-full p-2.5 min-h-[36px] min-w-[36px] text-base font-bold border border-white/20 transition-all duration-200"
					style="
							background: var(--accent-gradient);
							color: var(--text-primary);
							box-shadow: var(--btn-shadow);
							border-color: var(--alt-glass-border);
						"
					onclick={handleHideDetails}
					data-testid="hide-details-button-{uniqueId}"
				>
					<X size={16} />
				</Button>
			</div>
		</div>
	</div>
{/if}

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
