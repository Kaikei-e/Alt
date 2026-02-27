<script lang="ts">
import {
	FileText,
	Tag,
	FileStack,
	Volume2,
	Square,
	Loader2,
} from "@lucide/svelte";
import type { RecapGenre } from "$lib/schema/recap";
import EvidenceArticles from "./EvidenceArticles.svelte";
import { useTtsPlayback } from "$lib/hooks/useTtsPlayback.svelte";

interface Props {
	genre: RecapGenre | null;
}

let { genre }: Props = $props();

const tts = useTtsPlayback();

const ttsText = $derived(
	genre ? `${genre.summary}\n${(genre.bullets ?? []).join("\n")}` : "",
);

// Stop playback when genre changes
$effect(() => {
	// Access genre to track it
	genre;
	return () => {
		tts.stop();
	};
});

function handleTtsClick() {
	if (tts.isPlaying || tts.isLoading) {
		tts.stop();
	} else {
		tts.play(ttsText, { speed: 1.25 });
	}
}
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
				<div class="flex items-center justify-between mb-2">
					<h2 class="text-2xl font-bold text-[var(--text-primary)]">
						{genre.genre}
					</h2>
					<button
						onclick={handleTtsClick}
						class="p-2 rounded-md hover:bg-[var(--surface-hover)] transition-colors text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
						aria-label={tts.isPlaying ? "Stop reading" : tts.isLoading ? "Cancel loading" : "Read aloud"}
						title={tts.isPlaying ? "Stop reading" : tts.isLoading ? "Cancel loading" : "Read aloud"}
					>
						{#if tts.isLoading}
							<Loader2 class="h-5 w-5 animate-spin" />
						{:else if tts.isPlaying}
							<Square class="h-5 w-5" />
						{:else}
							<Volume2 class="h-5 w-5" />
						{/if}
					</button>
				</div>
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
				{#if tts.error}
					<p class="text-xs text-red-500 mt-1">{tts.error}</p>
				{/if}
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
