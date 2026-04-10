<script lang="ts">
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import {
	getSectionDisplayTitle,
	getSourcesForSection,
} from "./morning-letter-document";
import type {
	MorningLetterSection,
	MorningLetterSourceProto,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";

type Props = {
	section: MorningLetterSection;
	sources: MorningLetterSourceProto[];
	sourcesLoading: boolean;
};

let { section, sources, sourcesLoading }: Props = $props();

const title = $derived(getSectionDisplayTitle(section));
const sectionSources = $derived(getSourcesForSection(sources, section.key));
</script>

<section class="letter-section">
	<div class="section-rule"></div>

	<h2 class="section-heading">{title}</h2>

	<ul class="section-bullets">
		{#each section.bullets as bullet}
			<li class="section-bullet">
				{@html parseMarkdown(bullet)}
			</li>
		{/each}
	</ul>

	{#if sourcesLoading}
		<div class="sources-loading">
			<div class="loading-pulse"></div>
			<span class="loading-text">Loading sources&hellip;</span>
		</div>
	{:else if sectionSources.length > 0}
		<div class="entry-sources">
			<span class="sources-heading">Sources</span>
			<ul class="sources-list">
				{#each sectionSources as src, i}
					<li class="source-item">
						<span class="source-id">[{i + 1}]</span>
						<span class="source-meta">{src.sourceType === 1 ? "recap" : "overnight"}</span>
					</li>
				{/each}
			</ul>
		</div>
	{/if}
</section>

<style>
	.letter-section {
		margin-bottom: 1.5rem;
	}

	.section-rule {
		height: 1px;
		background: var(--surface-border, #c8c8c8);
		margin-bottom: 1rem;
	}

	.section-heading {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.15rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.75rem;
	}

	.section-bullets {
		list-style: none;
		padding: 0;
		margin: 0 0 0.75rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.section-bullet {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.9rem;
		line-height: 1.6;
		color: var(--alt-charcoal, #1a1a1a);
		padding-left: 0.75rem;
		border-left: 1px solid var(--surface-border, #c8c8c8);
	}

	.section-bullet :global(strong) {
		font-weight: 600;
	}

	.section-bullet :global(a) {
		color: var(--alt-primary, #2f4f4f);
		text-decoration: underline;
		text-underline-offset: 2px;
	}

	/* ===== Sources (citation footnotes) ===== */
	.entry-sources {
		margin-top: 0.5rem;
		padding-top: 0.5rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);
	}

	.sources-heading {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.6rem;
		font-weight: 700;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		margin-bottom: 0.3rem;
	}

	.sources-list {
		list-style: none;
		padding: 0;
		margin: 0;
		display: flex;
		flex-wrap: wrap;
		gap: 0.5rem;
	}

	.source-item {
		display: inline-flex;
		align-items: center;
		gap: 0.2rem;
	}

	.source-id {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		font-weight: 600;
		color: var(--alt-charcoal, #1a1a1a);
	}

	.source-meta {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.6rem;
		color: var(--alt-ash, #999);
	}

	/* ===== Loading ===== */
	.sources-loading {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		margin-top: 0.5rem;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash, #999);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.75rem;
		font-style: italic;
		color: var(--alt-ash, #999);
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}

	@media (prefers-reduced-motion: reduce) {
		.loading-pulse { animation: none; opacity: 0.6; }
	}
</style>
