<script lang="ts">
import { SquareArrowOutUpRight, Loader2 } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import FeedDetails from "$lib/components/mobile/FeedDetails.svelte";
import type { SearchFeedItem } from "$lib/schema/search";
import type { FetchArticleSummaryResponse } from "$lib/api/client";
import { getArticleSummaryClient } from "$lib/api/client";

interface Props {
	result: SearchFeedItem;
}

const { result }: Props = $props();

let isExpanded = $state(false);
let summary = $state<FetchArticleSummaryResponse | null>(null);
let isLoadingSummary = $state(false);
let summaryError = $state<string | null>(null);
let isSummarizing = $state(false);
let isDescriptionExpanded = $state(false);

// Check if description is long enough to need truncation
const descriptionText = $derived((result.description || "").trim());
const hasDescription = $derived(descriptionText.length > 0);
const shouldTruncateDescription = $derived(descriptionText.length > 200);
const displayDescription = $derived(
	isDescriptionExpanded
		? descriptionText
		: shouldTruncateDescription
			? descriptionText.slice(0, 200) + "..."
			: descriptionText,
);

const handleToggleSummary = async () => {
	if (!isExpanded && !summary && result.link) {
		isLoadingSummary = true;
		summaryError = null;

		try {
			const summaryResponse = await getArticleSummaryClient(result.link);
			summary = summaryResponse;
		} catch (error) {
			console.error("Error fetching summary:", error);
			summaryError = "Failed to fetch summary";
		} finally {
			isLoadingSummary = false;
		}
	}
	isExpanded = !isExpanded;
};

const handleSummarizeNow = async () => {
	if (!result.link) return;

	isSummarizing = true;
	try {
		const summaryResponse = await getArticleSummaryClient(result.link);
		summary = summaryResponse;
		isExpanded = true;
	} catch (error) {
		console.error("Error generating summary:", error);
		summaryError = "Failed to fetch summary";
	} finally {
		isSummarizing = false;
	}
};

const authorName = $derived(
	result.author?.name || result.authors?.[0]?.name || "Unknown",
);
const publishedDate = $derived(
	result.published ? new Date(result.published).toLocaleDateString() : null,
);
</script>

<div
	class="glass p-5 rounded-[24px] transition-all duration-300 hover:-translate-y-[2px] hover:shadow-lg"
	style="
		background: var(--alt-glass);
		border: 1px solid var(--alt-glass-border);
		box-shadow: var(--alt-glass-shadow);
	"
	data-testid="search-result-item"
>
	<div class="flex flex-col gap-3">
		<!-- Title as link -->
		<div class="flex flex-row items-center gap-2">
			<div
				class="flex items-center justify-center w-6 h-6 flex-shrink-0"
				style="color: var(--alt-primary);"
			>
				<SquareArrowOutUpRight size={16} />
			</div>
			<a
				href={result.link || "#"}
				target="_blank"
				rel="noopener noreferrer"
				class="text-base font-semibold hover:underline leading-tight break-words transition-colors duration-200"
				style="color: var(--text-primary);"
			>
				{result.title}
			</a>
		</div>

		<!-- Author and published date -->
		{#if result.published}
			<div class="flex justify-between items-center text-xs" style="color: var(--text-secondary);">
				<span>{authorName}</span>
				<span>{publishedDate}</span>
			</div>
		{/if}

		<!-- Description -->
		{#if hasDescription}
			<div>
				<p
					class="leading-relaxed break-words"
					style="color: var(--text-secondary);"
				>
					{displayDescription}
				</p>
				{#if shouldTruncateDescription}
					<Button
						variant="ghost"
						size="sm"
						onclick={() => {
							isDescriptionExpanded = !isDescriptionExpanded;
						}}
						class="mt-2 w-full text-xs"
					>
						{isDescriptionExpanded ? "Show less" : "Read more"}
					</Button>
				{/if}
			</div>
		{/if}

		<!-- Summary section -->
		{#if isExpanded}
			<div class="mt-3">
				{#if isLoadingSummary}
					<div class="flex justify-center items-center gap-2 py-4">
						<Loader2 class="h-4 w-4 animate-spin" style="color: var(--alt-primary);" />
						<span class="text-sm" style="color: var(--text-secondary);">
							Loading summary...
						</span>
					</div>
				{:else if isSummarizing}
					<div class="flex flex-col gap-3 py-4">
						<div class="flex justify-center items-center gap-2">
							<Loader2 class="h-4 w-4 animate-spin" style="color: var(--alt-primary);" />
							<span class="text-sm" style="color: var(--text-secondary);">
								Generating summary...
							</span>
						</div>
						<p
							class="text-xs text-center"
							style="color: var(--text-muted);"
						>
							This may take a few seconds
						</p>
					</div>
				{:else if summaryError}
					<div class="flex flex-col gap-3 w-full">
						<p
							class="text-sm text-center"
							style="color: var(--text-secondary);"
						>
							{summaryError}
						</p>
						{#if summaryError === "Failed to fetch summary"}
							<Button
								size="sm"
								onclick={handleSummarizeNow}
								class="w-full"
								style="background: var(--alt-primary); color: white;"
							>
								✨ Summarize Immediately
							</Button>
						{/if}
					</div>
				{:else if summary?.matched_articles && summary.matched_articles.length > 0}
					<div class="flex flex-col gap-2 w-full">
						<h3
							class="text-sm font-bold break-words"
							style="color: var(--alt-primary);"
						>
							{summary.matched_articles[0].title}
						</h3>
						<p
							class="text-sm leading-relaxed break-words whitespace-pre-wrap"
							style="color: var(--text-primary);"
						>
							{summary.matched_articles[0].content}
						</p>
					</div>
				{:else}
					<div class="flex flex-col gap-3 w-full">
						<p
							class="text-sm text-center"
							style="color: var(--text-secondary);"
						>
							No summary available for this article
						</p>
						<Button
							size="sm"
							onclick={handleSummarizeNow}
							class="w-full"
							style="background: var(--alt-primary); color: white;"
						>
							✨ Summarize Immediately
						</Button>
					</div>
				{/if}
			</div>
		{/if}

		<!-- Toggle summary button -->
		<div class="flex gap-2 mt-3">
			<Button
				variant="outline"
				size="sm"
				onclick={handleToggleSummary}
				class="w-full"
			>
				{isExpanded ? "Hide summary" : "Show summary"}
			</Button>
		</div>
	</div>
</div>

