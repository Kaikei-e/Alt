<script lang="ts">
	import { X, ExternalLink, Eye } from "@lucide/svelte";
	import type { RenderFeed } from "$lib/schema/feed";
	import { Button } from "$lib/components/ui/button";
	import {
		Dialog,
		DialogContent,
		DialogHeader,
		DialogTitle,
		DialogDescription,
	} from "$lib/components/ui/dialog";
	import { updateFeedReadStatusClient } from "$lib/api/client/feeds";

	interface Props {
		open: boolean;
		feed: RenderFeed | null;
		onOpenChange: (open: boolean) => void;
		onMarkAsRead?: (feedUrl: string) => void;
	}

	let { open = $bindable(), feed, onOpenChange, onMarkAsRead }: Props = $props();

	// Simple state for mark as read
	let isMarkingAsRead = $state(false);

	async function handleMarkAsRead() {
		if (!feed || isMarkingAsRead) return;

		try {
			isMarkingAsRead = true;
			await updateFeedReadStatusClient(feed.normalizedUrl);
			onMarkAsRead?.(feed.normalizedUrl);
		} catch (error) {
			console.error("Failed to mark feed as read:", error);
		} finally {
			isMarkingAsRead = false;
		}
	}

	function handleOpenExternal() {
		if (!feed) return;
		window.open(feed.link, "_blank", "noopener,noreferrer");
	}
</script>

<Dialog {open} {onOpenChange}>
	<DialogContent class="max-w-2xl max-h-[90vh] overflow-y-auto">
		{#if feed}
			<DialogHeader>
				<DialogTitle class="text-xl font-bold text-[var(--text-primary)] pr-8">
					{feed.title}
				</DialogTitle>
				<DialogDescription class="flex items-center gap-3 text-sm text-[var(--text-secondary)]">
					{#if feed.author}
						<span>by {feed.author}</span>
						<span>•</span>
					{/if}
					{#if feed.publishedAtFormatted}
						<span>{feed.publishedAtFormatted}</span>
					{/if}
				</DialogDescription>
			</DialogHeader>

			<!-- Content -->
			<div class="mt-4">
				{#if feed.excerpt}
					<p class="text-sm text-[var(--text-primary)] leading-relaxed whitespace-pre-wrap">
						{feed.excerpt}
					</p>
				{/if}

				<!-- Tags -->
				{#if feed.mergedTagsLabel}
					<div class="flex flex-wrap gap-2 mt-4">
						{#each feed.mergedTagsLabel.split(" / ") as tag}
							<span
								class="text-xs px-2 py-1 bg-[var(--surface-hover)] text-[var(--text-secondary)]"
							>
								{tag}
							</span>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Actions -->
			<div class="flex items-center gap-3 mt-6 pt-4 border-t border-[var(--surface-border)]">
				<Button
					variant="default"
					class="flex items-center gap-2"
					onclick={handleMarkAsRead}
					disabled={isMarkingAsRead}
				>
					<Eye class="h-4 w-4" />
					{isMarkingAsRead ? "Marking..." : "Mark as Read"}
				</Button>

				<Button variant="outline" class="flex items-center gap-2" onclick={handleOpenExternal}>
					<ExternalLink class="h-4 w-4" />
					Open External
				</Button>
			</div>
		{/if}
	</DialogContent>
</Dialog>
