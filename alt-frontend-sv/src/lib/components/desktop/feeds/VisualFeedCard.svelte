<script lang="ts">
import { Eye } from "@lucide/svelte";
import type { RenderFeed } from "$lib/schema/feed";
import { cn } from "$lib/utils";

interface Props {
	feed: RenderFeed;
	onSelect: (feed: RenderFeed) => void;
	isRead?: boolean;
}

let { feed, onSelect, isRead = false }: Props = $props();

let imageLoaded = $state(false);
let imageError = $state(false);

// Reset error/loaded state when ogImageProxyUrl changes (e.g. raw URL → proxy URL)
$effect(() => {
	void feed.ogImageProxyUrl;
	imageError = false;
	imageLoaded = false;
});

function handleClick() {
	onSelect(feed);
}

function handleImageLoad() {
	imageLoaded = true;
}

function handleImageError() {
	imageError = true;
}

const showImage = $derived(feed.ogImageProxyUrl && !imageError);
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
	<div class="relative aspect-video overflow-hidden bg-[var(--surface-hover)]">
		{#if showImage}
			{#if !imageLoaded}
				<!-- Shimmer placeholder -->
				<div class="absolute inset-0 shimmer"></div>
			{/if}
			<img
				data-testid="card-image"
				src={feed.ogImageProxyUrl}
				alt=""
				loading="lazy"
				class={cn(
					"w-full h-full object-cover transition-opacity duration-300",
					imageLoaded ? "opacity-100" : "opacity-0",
				)}
				onload={handleImageLoad}
				onerror={handleImageError}
			/>
		{:else}
			<!-- Fallback gradient (matches swipe VisualPreviewCard) -->
			<div
				data-testid="image-fallback"
				class="absolute inset-0 fallback-gradient"
			></div>
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
