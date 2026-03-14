<script lang="ts">
import {
	BookOpen,
	ExternalLink,
	StarOff,
	SquareArrowOutUpRight,
} from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import type { RenderFeed } from "$lib/schema/feed";
import FeedDetails from "./FeedDetails.svelte";

interface Props {
	feed: RenderFeed;
	onRemove?: (feedUrl: string) => void;
}

const { feed, onRemove }: Props = $props();

let isRemoving = $state(false);
let isDetailsOpen = $state(false);

const isRead = $derived(feed.isRead ?? false);

const handleRemove = async () => {
	if (!onRemove || isRemoving) return;
	isRemoving = true;
	try {
		onRemove(feed.normalizedUrl);
	} finally {
		isRemoving = false;
	}
};

const handleDetailsClick = () => {
	isDetailsOpen = true;
};
</script>

<div
	class="w-full max-w-[calc(100vw-2.5rem)] p-[2px] rounded-[18px] border-2 transition-transform duration-300 ease-in-out cursor-pointer hover:-translate-y-[2px] hover:shadow-lg"
	style="border-color: var(--surface-border);"
	data-testid="favorite-card"
>
	<div
		class="w-full p-4 rounded-2xl"
		role="article"
		aria-label="Favorite: {feed.title}"
		style="background: var(--surface-bg);"
	>
		<div class="flex flex-col gap-2">
			<!-- Header: Title + Read badge -->
			<div class="flex items-start justify-between gap-2">
				<div class="flex flex-row items-center gap-2 flex-1 min-w-0">
					<div class="flex items-center justify-center w-6 h-6 flex-shrink-0">
						<SquareArrowOutUpRight
							size={16}
							style="color: var(--alt-primary);"
						/>
					</div>
					<a
						href={feed.normalizedUrl}
						target="_blank"
						rel="noopener noreferrer"
						aria-label="Open {feed.title} in external link"
						class="text-sm hover:underline leading-tight break-words flex-1 min-w-0 {isRead
							? 'font-normal'
							: 'font-semibold'}"
						style="color: {isRead
							? 'var(--text-muted)'
							: 'var(--accent-primary)'};"
					>
						{feed.title}
					</a>
				</div>

				{#if isRead}
					<div
						class="px-2 py-0.5 rounded-full text-xs whitespace-nowrap border flex-shrink-0"
						style="
							background: rgba(0, 0, 0, 0.05);
							color: var(--text-muted);
							border-color: var(--surface-border);
						"
					>
						Read
					</div>
				{/if}
			</div>

			<!-- Excerpt -->
			<p
				class="text-xs leading-normal break-words line-clamp-3"
				style="color: {isRead ? 'var(--text-muted)' : 'var(--text-primary)'};"
			>
				{feed.excerpt}
			</p>

			<!-- Author -->
			{#if feed.author}
				<p class="text-xs italic" style="color: var(--text-secondary);">
					by {feed.author}
				</p>
			{/if}

			<!-- Actions -->
			<div class="flex justify-between items-center mt-1 gap-2 flex-wrap">
				<Button
					size="sm"
					variant="ghost"
					class="text-xs min-h-[44px]"
					style="color: var(--text-secondary);"
					onclick={handleDetailsClick}
					aria-label="Show details for {feed.title}"
				>
					<div class="flex items-center gap-1">
						<BookOpen size={14} />
						<span>Details</span>
					</div>
				</Button>

				<div class="flex gap-2">
					{#if onRemove}
						<Button
							size="sm"
							variant="ghost"
							class="text-xs min-h-[44px]"
							style="color: var(--text-secondary);"
							onclick={handleRemove}
							disabled={isRemoving}
							aria-label="Remove from favorites"
						>
							<div class="flex items-center gap-1">
								<StarOff size={14} />
								<span>{isRemoving ? "..." : "Remove"}</span>
							</div>
						</Button>
					{/if}

					<a
						href={feed.normalizedUrl}
						target="_blank"
						rel="noopener noreferrer"
						aria-label="Open article"
					>
						<Button
							size="sm"
							variant="ghost"
							class="text-xs min-h-[44px]"
							style="color: var(--text-secondary);"
						>
							<div class="flex items-center gap-1">
								<span>Open</span>
								<ExternalLink size={14} />
							</div>
						</Button>
					</a>
				</div>
			</div>
		</div>
	</div>
</div>

<!-- Feed Details Sheet -->
<FeedDetails
	feedURL={feed.link}
	feedTitle={feed.title}
	open={isDetailsOpen}
	onOpenChange={(open) => {
		isDetailsOpen = open;
	}}
	showButton={false}
/>
