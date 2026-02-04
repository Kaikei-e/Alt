<script lang="ts">
import type { TagTrailArticle, TagTrailTag } from "$lib/schema/tagTrail";
import { SquareArrowOutUpRight, Loader2 } from "@lucide/svelte";
import TagChipList from "./TagChipList.svelte";

interface Props {
	articles: TagTrailArticle[];
	isLoading: boolean;
	hasMore: boolean;
	selectedTagName: string;
	onTagClick: (tag: TagTrailTag) => void;
	onLoadMore: () => void;
	getArticleTags: (articleId: string) => TagTrailTag[];
	loadingArticleTags: Set<string>;
}

const {
	articles,
	isLoading,
	hasMore,
	selectedTagName,
	onTagClick,
	onLoadMore,
	getArticleTags,
	loadingArticleTags,
}: Props = $props();

const formatDate = (dateStr: string) => {
	const date = new Date(dateStr);
	return date.toLocaleDateString(undefined, {
		month: "short",
		day: "numeric",
	});
};
</script>

<div class="flex-1 overflow-y-auto px-4 pb-4" data-testid="tag-article-list">
	<p class="text-sm font-medium mb-3" style="color: var(--text-secondary);">
		Articles tagged with "{selectedTagName}"
	</p>

	<div class="flex flex-col gap-3">
		{#each articles as article (article.id)}
			<div
				class="p-[2px] rounded-[14px] border"
				style="border-color: var(--surface-border);"
				data-testid="tag-article-{article.id}"
			>
				<div
					class="p-3 rounded-xl"
					style="background: var(--surface-bg);"
				>
					<div class="flex items-start gap-2">
						<div class="flex items-center justify-center w-5 h-5 flex-shrink-0 mt-0.5">
							<SquareArrowOutUpRight size={14} style="color: var(--alt-primary);" />
						</div>
						<div class="flex-1 min-w-0">
							<a
								href={article.link}
								target="_blank"
								rel="noopener noreferrer"
								class="text-base font-semibold hover:underline leading-snug line-clamp-2"
								style="color: var(--text-primary);"
							>
								{article.title}
							</a>
							<div class="flex items-center gap-2 mt-1">
								{#if article.feedTitle}
									<span class="text-xs" style="color: var(--text-secondary);">
										{article.feedTitle}
									</span>
									<span style="color: var(--text-secondary);">·</span>
								{/if}
								<span class="text-xs" style="color: var(--text-secondary);">
									{formatDate(article.publishedAt)}
								</span>
							</div>
							<!-- Article tags for hopping -->
							{#if loadingArticleTags.has(article.id)}
								<div class="flex gap-1 mt-2">
									{#each [1, 2] as i}
										<div
											class="h-[28px] w-[60px] rounded-full animate-pulse"
											style="background: var(--muted);"
										></div>
									{/each}
								</div>
							{:else if getArticleTags(article.id).length > 0}
								<div class="mt-1.5 -mx-3">
									<TagChipList
										tags={getArticleTags(article.id)}
										onTagClick={onTagClick}
									/>
								</div>
							{/if}
						</div>
					</div>
				</div>
			</div>
		{/each}

		{#if isLoading}
			<div class="flex justify-center py-4">
				<Loader2 size={24} class="animate-spin" style="color: var(--alt-primary);" />
			</div>
		{/if}

		{#if !isLoading && hasMore}
			<button
				type="button"
				class="w-full py-3 text-sm font-medium rounded-xl min-h-[44px] transition-all active:scale-95"
				style="background: var(--muted); color: var(--text-primary);"
				onclick={onLoadMore}
			>
				Load more
			</button>
		{/if}

		{#if !isLoading && articles.length === 0}
			<div class="text-center py-8">
				<p class="text-sm" style="color: var(--text-secondary);">
					No articles found with this tag
				</p>
			</div>
		{/if}
	</div>
</div>
