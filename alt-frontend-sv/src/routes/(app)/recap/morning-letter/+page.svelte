<script lang="ts">
import { onMount } from "svelte";
import { page } from "$app/state";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useMorningLetter } from "$lib/hooks/useMorningLetter.svelte";
import {
	deriveWithinHours,
	formatLetterDate,
} from "$lib/components/morning-letter/morning-letter-document";

// Shared components
import MorningLetterDocumentCore from "$lib/components/morning-letter/MorningLetterDocumentCore.svelte";
import MorningLetterSkeleton from "$lib/components/morning-letter/MorningLetterSkeleton.svelte";
import MorningLetterEmpty from "$lib/components/morning-letter/MorningLetterEmpty.svelte";

// Desktop
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import DesktopChat from "$lib/components/desktop/morning-letter/MorningLetterChat.svelte";

// Mobile
import MobileChat from "$lib/components/mobile/morning-letter/MorningLetterChat.svelte";

const { isDesktop } = useViewport();

// useMorningLetter receives preloaded letter from +page.ts load
// page.data is populated by SvelteKit before rendering (ssr=false + load)
const ml = useMorningLetter(page.data.letter ?? null);

const targetDate = $derived(
	ml.letter?.targetDate ?? page.data.requestedDate ?? undefined,
);
const withinHours = $derived(deriveWithinHours(targetDate));
const dateDisplay = $derived(
	targetDate
		? formatLetterDate(targetDate, ml.letter?.editionTimezone)
		: "Morning Letter",
);

// If load returned an error, attempt client-side retry
onMount(() => {
	if (page.data.error && !ml.letter) {
		void ml.fetchLetter();
	}
});
</script>

<svelte:head>
	<title>Morning Letter - Alt</title>
</svelte:head>

{#if isDesktop}
	<PageHeader title="Morning Letter" description={dateDisplay} />
	<div class="flex gap-6 max-w-7xl mx-auto px-4 min-h-[60vh]">
		<!-- Document (2/3) -->
		<div class="flex-[2] min-w-0">
			{#if ml.letterLoading}
				<MorningLetterSkeleton minHeight="60vh" />
			{:else if !ml.letter}
				<MorningLetterEmpty requestedDate={page.data.requestedDate} />
			{:else}
				<MorningLetterDocumentCore
					letter={ml.letter}
					sources={ml.sources}
					sourcesLoading={ml.sourcesLoading}
				/>
			{/if}
		</div>

		<!-- Follow-up Chat (1/3) -->
		<aside class="flex-1 min-h-[40vh] max-w-md">
			<DesktopChat {withinHours} {targetDate} />
		</aside>
	</div>
{:else}
	<!-- Mobile: document above, chat disclosure below -->
	<div class="min-h-[100dvh] flex flex-col" style="background: var(--app-bg);">
		<!-- Mobile header -->
		<div class="sticky top-0 z-10 p-4 border-b" style="background: var(--surface-bg); border-color: var(--border-color);">
			<h1 class="font-['Playfair_Display'] text-lg font-bold" style="color: var(--text-primary);">
				Morning Letter
			</h1>
			{#if targetDate}
				<p class="text-xs mt-1" style="color: var(--text-secondary);">{dateDisplay}</p>
			{/if}
		</div>

		<!-- Document -->
		<div class="flex-1 p-4">
			{#if ml.letterLoading}
				<MorningLetterSkeleton minHeight="40vh" />
			{:else if !ml.letter}
				<MorningLetterEmpty requestedDate={page.data.requestedDate} />
			{:else}
				<MorningLetterDocumentCore
					letter={ml.letter}
					sources={ml.sources}
					sourcesLoading={ml.sourcesLoading}
				/>
			{/if}
		</div>

		<!-- Follow-up Chat (disclosure) -->
		<MobileChat {withinHours} {targetDate} />
	</div>
{/if}
