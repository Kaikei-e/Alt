<script lang="ts">
	import { FileText, Tag, FileStack } from "@lucide/svelte";
	import type { RecapGenre } from "$lib/schema/recap";
	import EvidenceArticles from "./EvidenceArticles.svelte";

	interface Props {
		genre: RecapGenre | null;
	}

	let { genre }: Props = $props();
</script>

<div class="border border-[var(--surface-border)] bg-white">
	{#if !genre}
		<!-- Placeholder state -->
		<div class="flex items-center justify-center h-full p-12">
			<div class="text-center">
				<FileText class="h-12 w-12 text-[var(--text-muted)] mx-auto mb-4" />
				<p class="text-sm text-[var(--text-secondary)]">Select a genre to view details</p>
			</div>
		</div>
	{:else}
		<!-- Genre detail content -->
		<div class="p-6">
			<!-- Header -->
			<div class="mb-6">
				<h2 class="text-2xl font-bold text-[var(--text-primary)] mb-2">
					{genre.genre}
				</h2>
				<div class="flex items-center gap-4 text-xs text-[var(--text-secondary)]">
					<div class="flex items-center gap-1">
						<FileStack class="h-3.5 w-3.5" />
						<span>{genre.articleCount} articles</span>
					</div>
					<div class="flex items-center gap-1">
						<FileText class="h-3.5 w-3.5" />
						<span>{genre.clusterCount} clusters</span>
					</div>
				</div>
			</div>

			<!-- Summary -->
			<div class="mb-6">
				<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2">Summary</h3>
				<p class="text-sm text-[var(--text-primary)] leading-relaxed whitespace-pre-wrap">
					{genre.summary}
				</p>
			</div>

			<!-- Bullet points -->
			{#if genre.bullets && genre.bullets.length > 0}
				<div class="mb-6">
					<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2">Key Points</h3>
					<ul class="space-y-2">
						{#each genre.bullets as bullet}
							<li class="flex items-start gap-2">
								<span class="text-[var(--accent-primary)] mt-1">•</span>
								<span class="text-sm text-[var(--text-primary)]">{bullet}</span>
							</li>
						{/each}
					</ul>
				</div>
			{/if}

			<!-- Top terms/keywords -->
			{#if genre.topTerms && genre.topTerms.length > 0}
				<div class="mb-6">
					<h3 class="text-sm font-semibold text-[var(--text-primary)] mb-2 flex items-center gap-2">
						<Tag class="h-4 w-4" />
						Top Keywords
					</h3>
					<div class="flex flex-wrap gap-2">
						{#each genre.topTerms as term}
							<span class="text-xs px-3 py-1 bg-[var(--surface-hover)] text-[var(--text-primary)]">
								{term}
							</span>
						{/each}
					</div>
				</div>
			{/if}

			<!-- Evidence articles -->
			{#if genre.evidenceLinks && genre.evidenceLinks.length > 0}
				<EvidenceArticles evidenceLinks={genre.evidenceLinks} />
			{/if}
		</div>
	{/if}
</div>
