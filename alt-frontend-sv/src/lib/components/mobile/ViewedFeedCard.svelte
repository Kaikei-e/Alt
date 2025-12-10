<script lang="ts">
import { Archive, ExternalLink, BookOpen } from "@lucide/svelte";
import { Button } from "$lib/components/ui/button";
import type { RenderFeed } from "$lib/schema/feed";
import { archiveContentClient } from "$lib/api/client";
import FeedDetails from "./FeedDetails.svelte";

interface Props {
	feed: RenderFeed;
}

const { feed }: Props = $props();

let isArchiving = $state(false);
let isArchived = $state(false);
let isDetailsOpen = $state(false);

const handleArchive = async (e: MouseEvent) => {
	e.preventDefault();
	e.stopPropagation();
	if (!feed.link || isArchiving || isArchived) return;

	try {
		isArchiving = true;
		await archiveContentClient(feed.link, feed.title);
		isArchived = true;
	} catch (error) {
		console.error("Failed to archive:", error);
	} finally {
		isArchiving = false;
	}
};

const handleDetailsClick = async (e: MouseEvent) => {
	e.stopPropagation();
	isDetailsOpen = true;
	// Trigger data loading in FeedDetails
	// FeedDetails will handle the loading when isOpen becomes true
};
</script>

<div
	class="p-4 rounded-2xl border-2 transition-all duration-300 ease-in-out relative overflow-hidden w-full"
	role="article"
	aria-label="Viewed feed: {feed.title}"
	style="
		background: var(--alt-glass);
		border-color: var(--alt-glass-border);
		box-shadow: 0 12px 40px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1);
		backdrop-filter: blur(20px);
	"
	onmouseenter={(e) => {
		e.currentTarget.style.borderColor = "var(--alt-glass-border)";
		e.currentTarget.style.boxShadow =
			"0 12px 40px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.15)";
		e.currentTarget.style.transform = "translateY(-2px)";
	}}
	onmouseleave={(e) => {
		e.currentTarget.style.borderColor = "var(--alt-glass-border)";
		e.currentTarget.style.boxShadow =
			"0 12px 40px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.1)";
		e.currentTarget.style.transform = "translateY(0)";
	}}
	data-testid="viewed-feed-card"
>
	<div class="flex flex-col gap-3">
		<!-- Header: Title and Badge -->
		<div class="flex justify-between items-start gap-3">
			<a
				href={feed.normalizedUrl}
				target="_blank"
				rel="noopener noreferrer"
				class="flex-1"
			>
				<h3
					class="text-base font-bold leading-tight line-clamp-2"
					style="
						color: var(--alt-text-primary);
						font-family: var(--font-outfit, sans-serif);
					"
				>
					{feed.title}
				</h3>
			</a>
			<div
				class="px-2 py-0.5 rounded-full text-xs whitespace-nowrap border"
				style="
					background: rgba(255, 255, 255, 0.1);
					color: var(--alt-text-secondary);
					border-color: var(--alt-glass-border);
				"
			>
				Read
			</div>
		</div>

		<!-- Description - Use server-generated excerpt if available -->
		<p
			class="text-sm leading-normal line-clamp-3"
			style="color: var(--alt-text-secondary);"
		>
			{feed.excerpt}
		</p>

		<!-- Footer: Actions and Meta -->
		<div class="flex justify-between items-center mt-1 gap-2 flex-wrap">
			<Button
				size="sm"
				variant="ghost"
				class="text-xs"
				style="
					color: var(--alt-text-secondary);
				"
				onclick={handleDetailsClick}
			>
				<div class="flex items-center gap-1">
					<BookOpen size={14} />
					<span>Details</span>
				</div>
			</Button>

			<div class="flex gap-2">
				<Button
					size="sm"
					variant="ghost"
					class="text-xs"
					style="
						color: var(--alt-text-secondary);
					"
					onclick={handleArchive}
					disabled={isArchived}
				>
					<div class="flex items-center gap-1">
						{#if !isArchived}
							<Archive size={14} />
						{/if}
						<span>
							{isArchiving ? "..." : isArchived ? "Archived" : "Archive"}
						</span>
					</div>
				</Button>

				<a
					href={feed.normalizedUrl}
					target="_blank"
					rel="noopener noreferrer"
				>
					<Button
						size="sm"
						variant="ghost"
						class="text-xs"
						style="
							color: var(--alt-text-secondary);
						"
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

<!-- Feed Details Modal -->
<FeedDetails
	feedURL={feed.link}
	feedTitle={feed.title}
	open={isDetailsOpen}
	onOpenChange={(open) => {
		isDetailsOpen = open;
	}}
	showButton={false}
/>

