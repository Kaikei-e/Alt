<script lang="ts">
import type { TagTrailArticle } from "$lib/connect";
import { SquareArrowOutUpRight } from "@lucide/svelte";

interface Props {
	article: TagTrailArticle;
	selected: boolean;
	onclick: () => void;
}

const { article, selected, onclick }: Props = $props();

function formatDate(dateStr: string): string {
	const date = new Date(dateStr);
	return date.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}
</script>

<button
	type="button"
	class="w-full text-left rounded-lg border px-4 py-3 transition-all duration-200 cursor-pointer
		{selected
			? 'border-l-[3px] border-l-[var(--interactive-text)] border-[var(--interactive-text)] bg-[var(--surface-hover)] shadow-[var(--shadow-sm)]'
			: 'border-[var(--surface-border)] bg-[var(--surface-bg)] hover:border-[var(--interactive-text)]/50 hover:shadow-[var(--shadow-sm)] hover:-translate-y-0.5'}"
	data-testid="tag-article-{article.id}"
	{onclick}
>
	<div class="flex items-start gap-2">
		<SquareArrowOutUpRight class="h-3.5 w-3.5 mt-1 text-[var(--interactive-text)] flex-shrink-0 opacity-60" />
		<div class="flex-1 min-w-0">
			<h3 class="text-sm font-semibold leading-snug text-[var(--text-primary)] line-clamp-2">
				{article.title}
			</h3>
			<div class="flex items-center gap-1.5 mt-1.5">
				{#if article.feedTitle}
					<span class="text-xs text-[var(--text-secondary)] truncate max-w-[120px]">{article.feedTitle}</span>
					<span class="text-[var(--text-muted)]">·</span>
				{/if}
				<span class="text-xs text-[var(--text-muted)]">{formatDate(article.publishedAt)}</span>
			</div>
		</div>
	</div>
</button>
