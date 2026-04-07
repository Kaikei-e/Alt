<script lang="ts">
import { AlertTriangle } from "@lucide/svelte";
import type {
	MorningLetterDocument,
	MorningLetterSourceProto,
} from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";
import {
	orderSections,
	formatLetterDate,
	isLetterStale,
} from "./morning-letter-document";
import MorningLetterSectionCore from "./MorningLetterSectionCore.svelte";

type Props = {
	letter: MorningLetterDocument;
	sources: MorningLetterSourceProto[];
	sourcesLoading: boolean;
};

let { letter, sources, sourcesLoading }: Props = $props();

const orderedSections = $derived(orderSections(letter.body?.sections ?? []));
const dateDisplay = $derived(
	formatLetterDate(letter.targetDate, letter.editionTimezone),
);
const stale = $derived(isLetterStale(letter.createdAt, 12));
</script>

<article class="morning-letter-document">
	<!-- Header -->
	<header class="mb-6">
		<div class="flex items-center gap-3 mb-2">
			<time class="font-['Source_Sans_3'] text-sm" style="color: var(--text-secondary);" datetime={letter.targetDate}>
				{dateDisplay}
			</time>

			{#if letter.isDegraded}
				<span class="inline-flex items-center gap-1 px-2 py-0.5 rounded text-xs font-medium bg-amber-100 text-amber-800">
					<AlertTriangle class="h-3 w-3" />
					Degraded
				</span>
			{/if}

			{#if stale}
				<span class="text-xs px-2 py-0.5 rounded" style="background: var(--skeleton-base, #e8e4de); color: var(--text-secondary);">
					Stale
				</span>
			{/if}
		</div>

		{#if letter.body?.sourceRecapWindowDays}
			<p class="text-xs" style="color: var(--text-secondary);">
				Based on {letter.body.sourceRecapWindowDays}-day recap
			</p>
		{/if}
	</header>

	<!-- Lead paragraph -->
	{#if letter.body?.lead}
		<p class="font-['Playfair_Display'] text-lg leading-relaxed mb-8" style="color: var(--text-primary);">
			{letter.body.lead}
		</p>
	{/if}

	<!-- Sections -->
	{#each orderedSections as section (section.key)}
		<MorningLetterSectionCore {section} {sources} {sourcesLoading} />
	{/each}

	<!-- Footer -->
	<footer class="mt-8 pt-4 border-t text-xs" style="border-color: var(--border-color); color: var(--text-secondary);">
		<span>Model: {letter.model}</span>
		{#if letter.generationRevision > 1}
			<span class="ml-3">Rev. {letter.generationRevision}</span>
		{/if}
	</footer>
</article>
