<script lang="ts">
import { AlertTriangle } from "@lucide/svelte";
import type {
	MorningLetterDocument,
	MorningLetterSourceProto,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";
import { orderSections, isLetterStale } from "./morning-letter-document";
import MorningLetterSectionCore from "./MorningLetterSectionCore.svelte";

type Props = {
	letter: MorningLetterDocument;
	sources: MorningLetterSourceProto[];
	sourcesLoading: boolean;
};

let { letter, sources, sourcesLoading }: Props = $props();

const orderedSections = $derived(orderSections(letter.body?.sections ?? []));
const stale = $derived(isLetterStale(letter.createdAt, 12));
</script>

<article class="letter-document">
	<!-- Badges (degraded / stale) -->
	{#if letter.isDegraded || stale}
		<div class="flex items-center gap-2 mb-3">
			{#if letter.isDegraded}
				<span class="edition-badge edition-badge--degraded">
					<AlertTriangle class="h-3 w-3" />
					Degraded
				</span>
			{/if}
			{#if stale}
				<span class="edition-badge edition-badge--stale">Stale</span>
			{/if}
		</div>
	{/if}

	<!-- Recap window -->
	{#if letter.body?.sourceRecapWindowDays}
		<p class="recap-window">
			Based on {letter.body.sourceRecapWindowDays}-day recap
		</p>
	{/if}

	<!-- Lead paragraph -->
	{#if letter.body?.lead}
		<p class="edition-lede">{letter.body.lead}</p>
	{/if}

	<!-- Sections -->
	{#each orderedSections as section, index (section.key)}
		<div class="section-enter" style="--stagger: {index};">
			<MorningLetterSectionCore {section} {sources} {sourcesLoading} />
		</div>
	{/each}

	<!-- Footer -->
	<footer class="edition-footer">
		<span>Model: {letter.model}</span>
		{#if letter.generationRevision > 1}
			<span class="footer-sep">&middot;</span>
			<span>Rev. {letter.generationRevision}</span>
		{/if}
	</footer>
</article>

<style>
	.letter-document {
		max-width: 65ch;
	}

	.edition-badge {
		display: inline-flex;
		align-items: center;
		gap: 0.25rem;
		padding: 0.15rem 0.5rem;

		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.edition-badge--degraded {
		color: var(--alt-terracotta, #b85450);
		border: 1px solid var(--alt-terracotta, #b85450);
	}

	.edition-badge--stale {
		color: var(--alt-ash, #999);
		border: 1px solid var(--surface-border, #c8c8c8);
	}

	.recap-window {
		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
		margin: 0 0 1rem;
	}

	.edition-lede {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.1rem;
		line-height: 1.65;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 2rem;
	}

	.section-enter {
		opacity: 0;
		animation: section-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger) * 60ms);
	}
	@keyframes section-in {
		to { opacity: 1; }
	}

	.edition-footer {
		margin-top: 2rem;
		padding-top: 0.75rem;
		border-top: 1px solid var(--surface-border, #c8c8c8);

		font-family: var(--font-mono, "IBM Plex Mono", monospace);
		font-size: 0.65rem;
		color: var(--alt-ash, #999);
	}

	.footer-sep {
		margin: 0 0.35rem;
		color: var(--surface-border, #c8c8c8);
	}

	@media (prefers-reduced-motion: reduce) {
		.section-enter {
			animation: none;
			opacity: 1;
		}
	}
</style>
