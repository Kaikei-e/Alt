<script lang="ts">
	import { Eye, ExternalLink } from "@lucide/svelte";
	import type { RenderFeed } from "$lib/schema/feed";
	import { cn } from "$lib/utils";

	interface Props {
		feed: RenderFeed;
		onSelect: (feed: RenderFeed) => void;
		isRead?: boolean;
	}

	let { feed, onSelect, isRead = false }: Props = $props();

	function handleClick() {
		onSelect(feed);
	}
</script>

<button
	type="button"
	onclick={handleClick}
	class={cn(
		"w-full text-left border border-[var(--surface-border)] bg-white p-4 transition-all duration-200 hover:shadow-md hover:-translate-y-1 cursor-pointer group",
		isRead && "opacity-60"
	)}
	aria-label="Open {feed.title}"
>
	<div class="flex flex-col h-full gap-2">
		<!-- Title -->
		<h3
			class="text-sm font-semibold text-[var(--text-primary)] line-clamp-2 group-hover:text-[var(--accent-primary)] transition-colors"
		>
			{feed.title}
		</h3>

		<!-- Excerpt -->
		{#if feed.excerpt}
			<p class="text-xs text-[var(--text-secondary)] line-clamp-3 flex-1">
				{feed.excerpt}
			</p>
		{/if}

		<!-- Footer -->
		<div class="flex items-center justify-between mt-auto pt-2 border-t border-[var(--surface-border)]">
			<div class="flex flex-col gap-0.5">
				{#if feed.author}
					<p class="text-xs text-[var(--text-secondary)] truncate">
						{feed.author}
					</p>
				{/if}
				{#if feed.publishedAtFormatted}
					<p class="text-xs text-[var(--text-muted)]">
						{feed.publishedAtFormatted}
					</p>
				{/if}
			</div>

			<!-- Read status badge -->
			{#if isRead}
				<div class="flex items-center gap-1 text-xs text-[var(--text-muted)]">
					<Eye class="h-3 w-3" />
					<span>Read</span>
				</div>
			{/if}
		</div>

		<!-- Tags (if available) -->
		{#if feed.mergedTagsLabel}
			<div class="flex flex-wrap gap-1 mt-1">
				{#each feed.mergedTagsLabel.split(" / ").slice(0, 2) as tag}
					<span
						class="text-xs px-2 py-0.5 bg-[var(--surface-hover)] text-[var(--text-secondary)] truncate max-w-[120px]"
					>
						{tag}
					</span>
				{/each}
			</div>
		{/if}
	</div>
</button>
