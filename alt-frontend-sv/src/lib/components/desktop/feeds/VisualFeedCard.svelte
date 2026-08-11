<script lang="ts">
import { Eye } from "@lucide/svelte";
import type { RenderFeed } from "$lib/schema/feed";
import { cn } from "$lib/utils";
import { ogImageResolver } from "$lib/utils/ogImageResolver";
import { createProxyImage } from "$lib/utils/proxyImage.svelte";

interface Props {
	feed: RenderFeed;
	onSelect: (feed: RenderFeed) => void;
	isRead?: boolean;
}

let { feed, onSelect, isRead = false }: Props = $props();

// idle/loading -> shimmer; loaded -> <img>; absent -> fallback gradient.
// A transient rate-limit (429) is retried inside the loader and stays "loading",
// so it never collapses to the fallback the way a sticky onerror used to.
let imageContainer = $state<HTMLElement | null>(null);

const image = createProxyImage({
	url: () => feed.ogImageProxyUrl,
	container: () => imageContainer,
	// Feeds whose RSS carried no image resolve when the reader reaches them,
	// keyed on the feed: most of these have no article row to key on.
	resolve: () => ogImageResolver().resolve(feed.id),
});

function handleClick() {
	onSelect(feed);
}

const tags = $derived(
	feed.mergedTagsLabel ? feed.mergedTagsLabel.split(" / ").slice(0, 2) : [],
);
</script>

<button
	type="button"
	onclick={handleClick}
	class={cn(
		"w-full text-left border border-[var(--surface-border)] transition-colors duration-200 cursor-pointer group overflow-hidden",
		isRead
			? "bg-[var(--surface-hover)]"
			: "border-l-[3px] border-l-[var(--alt-primary)]",
	)}
	style="background: var(--surface-bg);"
	aria-label="Open {feed.title}"
>
	<!-- Image area -->
	<div
		class="relative aspect-video overflow-hidden bg-[var(--surface-hover)]"
		bind:this={imageContainer}
	>
		{#if image.state === "absent"}
			<!-- Fallback gradient: resolution came back empty, the origin refused,
			     or every retry was exhausted. A card with no URL yet is *not*
			     here — it shows the shimmer while it resolves. -->
			<div
				data-testid="image-fallback"
				class="absolute inset-0 fallback-gradient"
			></div>
		{:else if image.state === "loaded" && image.objectUrl}
			<img
				data-testid="card-image"
				src={image.objectUrl}
				alt=""
				class="w-full h-full object-cover"
			/>
		{:else}
			<!-- idle / loading (incl. in-flight retries): image exists, not resolved yet -->
			<div data-testid="image-loading" class="absolute inset-0 shimmer"></div>
		{/if}
	</div>

	<!-- Content area -->
	<div class="p-4 flex flex-col gap-2">
		<!-- Title -->
		<h3
			class={cn(
				"text-sm line-clamp-2 group-hover:text-[var(--accent-primary)] transition-colors",
				isRead
					? "font-normal text-[var(--text-muted)]"
					: "font-semibold text-[var(--text-primary)]",
			)}
		>
			{feed.title}
		</h3>

		<!-- Excerpt -->
		{#if feed.excerpt}
			<p class="text-xs text-[var(--text-secondary)] line-clamp-2">
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

			{#if isRead}
				<div class="flex items-center gap-1 text-xs text-[var(--text-muted)]">
					<Eye class="h-3 w-3" />
					<span>Read</span>
				</div>
			{/if}
		</div>

		<!-- Tags -->
		{#if tags.length > 0}
			<div data-testid="tags-container" class="flex flex-wrap gap-1">
				{#each tags as tag}
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

<style>
	.shimmer {
		background: linear-gradient(
			90deg,
			rgba(0, 0, 0, 0.03) 25%,
			rgba(0, 0, 0, 0.08) 50%,
			rgba(0, 0, 0, 0.03) 75%
		);
		background-size: 200% 100%;
		animation: shimmer 1.5s infinite;
	}

	@keyframes shimmer {
		0% {
			background-position: 200% 0;
		}
		100% {
			background-position: -200% 0;
		}
	}

	.fallback-gradient {
		background: linear-gradient(
			135deg,
			rgba(var(--alt-primary-rgb, 99, 102, 241), 0.15) 0%,
			rgba(var(--alt-secondary-rgb, 168, 85, 247), 0.10) 50%,
			rgba(var(--alt-primary-rgb, 99, 102, 241), 0.05) 100%
		);
	}
</style>
