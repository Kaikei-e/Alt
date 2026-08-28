<script lang="ts">
import { FileText } from "@lucide/svelte";
import { articleThumbnailResolver } from "$lib/utils/articleThumbnail";
import { createProxyImage } from "$lib/utils/proxyImage.svelte";

interface Props {
	/** The article's own title, never the message envelope it arrived in. */
	title: string;
	articleId?: string | null;
	/** The source URL, when the caller holds one. Saves a lookup. */
	sourceUrl?: string | null;
	tags?: string[];
}

const {
	title,
	articleId = null,
	sourceUrl = null,
	tags = [],
}: Props = $props();

let thumbnailContainer = $state<HTMLElement | null>(null);

// The thumbnail is fetched because this card is on screen, so an article whose
// stored image has aged out still shows one. Without an id there is nothing to
// resolve and the card settles on its icon.
const thumbnail = createProxyImage({
	url: () => null,
	container: () => thumbnailContainer,
	// `articleThumbnailResolver` still answers `string | null`, so a transport
	// failure and "this article has no thumbnail" both arrive as null and both
	// settle the card on its icon — the same collapse `ogImageResolver` was
	// given three outcomes to end. Mapped, not fixed, here: this card is not
	// the surface under change, and widening the retry policy of the augur
	// thumbnails on the way past is not this diff's to decide.
	resolve: async () => {
		const url = articleId
			? await articleThumbnailResolver().resolve(articleId, sourceUrl)
			: null;
		return url
			? { status: "resolved" as const, url }
			: { status: "absent" as const };
	},
});
</script>

<div
	data-testid="article-scope-card"
	class="flex items-start gap-2.5 rounded-lg border border-[var(--surface-border)] bg-[var(--surface-hover)] p-2.5"
>
	<div
		bind:this={thumbnailContainer}
		class="relative h-12 w-16 flex-shrink-0 overflow-hidden rounded bg-[var(--surface-bg)]"
	>
		{#if thumbnail.state === "loaded" && thumbnail.src}
			<img
				data-testid="article-scope-thumbnail"
				src={thumbnail.src}
				alt=""
				class="h-full w-full object-cover"
			/>
		{:else if thumbnail.state === "loading"}
			<div data-testid="article-scope-thumbnail-loading" class="absolute inset-0 shimmer"></div>
		{:else}
			<div class="flex h-full w-full items-center justify-center">
				<FileText class="h-4 w-4 text-[var(--interactive-text)]" />
			</div>
		{/if}
	</div>

	<div class="min-w-0 flex-1">
		<p class="line-clamp-2 text-sm font-medium text-[var(--text-primary)]">{title}</p>
		{#if tags.length > 0}
			<div class="mt-1.5 flex flex-wrap gap-1">
				{#each tags as tag}
					<span class="rounded-full bg-[var(--surface-bg)] px-2 py-0.5 text-[10px] text-[var(--text-secondary)]">
						{tag}
					</span>
				{/each}
			</div>
		{/if}
	</div>
</div>

<style>
	.shimmer {
		background: linear-gradient(
			90deg,
			var(--surface-bg, #f5f4f1) 25%,
			var(--surface-hover, #eceae5) 50%,
			var(--surface-bg, #f5f4f1) 75%
		);
		background-size: 200% 100%;
		animation: scope-shimmer 1.4s ease-in-out infinite;
	}
	@keyframes scope-shimmer {
		to { background-position: -200% 0; }
	}
	@media (prefers-reduced-motion: reduce) {
		.shimmer { animation: none; }
	}
</style>
