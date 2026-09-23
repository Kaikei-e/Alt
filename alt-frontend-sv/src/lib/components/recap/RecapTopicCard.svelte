<script lang="ts">
import type { RecapCard } from "$lib/connect/recap";
import { safeArticleHref } from "$lib/utils/safeHref";

interface Props {
	card: RecapCard;
}

const { card }: Props = $props();

const sortedSources = $derived(
	[...(card.sources ?? [])].sort((a, b) => a.n - b.n),
);
</script>

<article
	data-testid="recap-topic-card"
	data-rank={card.rank}
	class="rounded-xl border border-[var(--surface-border)] bg-[var(--surface-bg)] p-5 shadow-xs transition-shadow hover:shadow-md space-y-4"
>
	<!-- Header metadata: Rank, Genre, Continues marker -->
	<header class="flex items-center justify-between gap-2 flex-wrap">
		<div class="flex items-center gap-2">
			<span
				class="inline-flex items-center justify-center font-mono font-bold text-xs px-2 py-0.5 rounded bg-[var(--surface-hover)] text-[var(--text-primary)] border border-[var(--surface-border)]"
			>
				#{card.rank}
			</span>
			{#if card.genre}
				<span
					data-testid="card-genre"
					class="inline-block rounded-full bg-[var(--surface-hover)] px-2.5 py-0.5 text-xs font-medium text-[var(--text-secondary)] border border-[var(--surface-border)]"
				>
					{card.genre}
				</span>
			{/if}
		</div>

		{#if card.continuesCardId}
			<span
				data-testid="card-continues"
				class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20"
			>
				Continues
			</span>
		{/if}
	</header>

	<!-- Headline -->
	<h2
		data-testid="card-headline"
		class="text-base sm:text-lg font-bold text-[var(--text-primary)] leading-snug tracking-tight"
	>
		{card.headlineJa}
	</h2>

	<!-- Event summary (what_ja) -->
	<p
		data-testid="card-what"
		class="text-sm text-[var(--text-secondary)] leading-relaxed"
	>
		{card.whatJa}
	</p>

	<!-- Significance explanation (why_ja, optional) -->
	{#if card.whyJa && card.whyJa.trim() !== ""}
		<div
			data-testid="card-why"
			class="rounded-lg bg-[var(--surface-hover)] border-l-3 border-[var(--interactive-text)] p-3 text-xs sm:text-sm text-[var(--text-secondary)] leading-relaxed"
		>
			<span class="font-semibold text-[var(--text-primary)] mr-1">Why it matters:</span>
			{card.whyJa}
		</div>
	{/if}

	<!-- Sources (cited articles) -->
	{#if sortedSources.length > 0}
		<div class="pt-2 border-t border-[var(--surface-border)]">
			<h3 class="text-xs font-semibold text-[var(--text-muted)] uppercase tracking-wider mb-2">
				Sources
			</h3>
			<ol class="space-y-1.5 list-none p-0 m-0" aria-label="Sources">
				{#each sortedSources as source (source.n)}
					{@const href = safeArticleHref(source.url)}
					<li class="text-xs leading-normal">
						{#if href}
							<a
								data-testid="card-source"
								{href}
								target="_blank"
								rel="noopener noreferrer"
								class="group inline-flex items-baseline gap-1.5 text-[var(--interactive-text)] hover:text-[var(--interactive-text-hover)] transition-colors"
							>
								<span class="font-mono text-[var(--text-muted)] shrink-0">[{source.n}]</span>
								<span class="font-medium group-hover:underline">{source.title}</span>
								<span class="text-[var(--text-muted)] text-[11px] shrink-0">({source.host})</span>
							</a>
						{:else}
							<span
								data-testid="card-source"
								class="inline-flex items-baseline gap-1.5 text-[var(--text-secondary)]"
							>
								<span class="font-mono text-[var(--text-muted)] shrink-0">[{source.n}]</span>
								<span class="font-medium">{source.title}</span>
								<span class="text-[var(--text-muted)] text-[11px] shrink-0">({source.host})</span>
							</span>
						{/if}
					</li>
				{/each}
			</ol>
		</div>
	{/if}
</article>
