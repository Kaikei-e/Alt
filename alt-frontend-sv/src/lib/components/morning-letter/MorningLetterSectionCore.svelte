<script lang="ts">
import { Loader2 } from "@lucide/svelte";
import { parseMarkdown } from "$lib/utils/simpleMarkdown";
import { getSectionDisplayTitle, getSourcesForSection } from "./morning-letter-document";
import type { MorningLetterSection, MorningLetterSourceProto } from "$lib/gen/alt/morning_letter/v2/morning_letter_pb";

type Props = {
	section: MorningLetterSection;
	sources: MorningLetterSourceProto[];
	sourcesLoading: boolean;
};

let { section, sources, sourcesLoading }: Props = $props();

const title = $derived(getSectionDisplayTitle(section));
const sectionSources = $derived(getSourcesForSection(sources, section.key));
</script>

<section class="morning-letter-section border-t-2 pt-4 mb-6" style="border-color: var(--text-primary);">
	<h2 class="font-['Playfair_Display'] text-xl font-bold mb-3" style="color: var(--text-primary);">
		{title}
	</h2>

	<ul class="space-y-2 mb-3">
		{#each section.bullets as bullet}
			<li class="font-['Source_Sans_3'] text-sm leading-relaxed pl-4 border-l-2" style="color: var(--text-primary); border-color: var(--border-color);">
				{@html parseMarkdown(bullet)}
			</li>
		{/each}
	</ul>

	{#if sourcesLoading}
		<div class="flex items-center gap-2 text-xs" style="color: var(--text-secondary);">
			<Loader2 class="h-3 w-3 animate-spin" />
			<span>Loading sources...</span>
		</div>
	{:else if sectionSources.length > 0}
		<div class="text-xs" style="color: var(--text-secondary);">
			<span class="font-semibold">{sectionSources.length} source{sectionSources.length > 1 ? 's' : ''}</span>
		</div>
	{/if}
</section>
