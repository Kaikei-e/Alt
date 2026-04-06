<script lang="ts">
import { goto } from "$app/navigation";
import type { ArticleSectionData } from "$lib/connect/global_search";
import { FileText, ChevronRight } from "@lucide/svelte";

interface Props {
	section: ArticleSectionData;
	query: string;
}

const { section, query }: Props = $props();

function navigateToArticle(id: string, link: string, title: string) {
	const params = new URLSearchParams({ url: link });
	if (title) params.set("title", title);
	goto(`/articles/${id}?${params.toString()}`);
}

function seeAll() {
	goto(`/feeds/search?q=${encodeURIComponent(query)}`);
}
</script>

<section class="space-y-3">
	<div class="flex items-center justify-between">
		<h2
			class="text-xs font-semibold uppercase tracking-wider text-[var(--text-secondary)]"
		>
			Articles
			{#if section.estimatedTotal > 0}
				<span class="ml-1 font-normal text-[var(--text-secondary)]"
					>({section.estimatedTotal})</span
				>
			{/if}
		</h2>
		{#if section.hasMore}
			<button
				type="button"
				onclick={seeAll}
				class="inline-flex items-center gap-1 text-xs text-[var(--interactive-text)] hover:text-[var(--interactive-text-hover)] transition-colors"
			>
				See all <ChevronRight class="h-3 w-3" />
			</button>
		{/if}
	</div>

	{#if section.hits.length === 0}
		<p class="text-sm text-[var(--text-secondary)] italic">
			No matching articles found.
		</p>
	{:else}
		<div class="space-y-2">
			{#each section.hits as hit (hit.id)}
				<button
					type="button"
					onclick={() => navigateToArticle(hit.id, hit.link, hit.title)}
					class="w-full text-left rounded-lg border border-[var(--surface-border)] bg-[var(--surface-bg)] p-4 hover:bg-[var(--surface-hover)] hover:border-[var(--interactive-text)] transition-colors cursor-pointer"
				>
					<div class="flex items-start gap-3">
						<FileText
							class="mt-0.5 h-4 w-4 flex-shrink-0 text-[var(--text-secondary)]"
						/>
						<div class="min-w-0 flex-1 space-y-1.5">
							<h3
								class="text-sm font-medium text-[var(--text-primary)] leading-tight line-clamp-2"
							>
								{hit.title}
							</h3>
							{#if hit.snippet}
								<p
									class="text-xs text-[var(--text-secondary)] leading-relaxed line-clamp-2"
								>
									{@html hit.snippet}
								</p>
							{/if}
							<div class="flex flex-wrap items-center gap-1.5">
								{#each hit.matchedFields as field}
									<span
										class="inline-block rounded border border-[var(--surface-border)] px-1.5 py-0.5 text-[10px] uppercase tracking-wider text-[var(--text-secondary)]"
									>
										{field}
									</span>
								{/each}
								{#each hit.tags.slice(0, 3) as tag}
									<span
										class="inline-block rounded-full bg-[var(--surface-hover)] px-2 py-0.5 text-[10px] text-[var(--text-secondary)]"
									>
										{tag}
									</span>
								{/each}
							</div>
						</div>
					</div>
				</button>
			{/each}
		</div>
	{/if}
</section>
