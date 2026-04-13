<script lang="ts">
import { onMount } from "svelte";
import { goto } from "$app/navigation";
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
import DesktopChat from "$lib/components/desktop/morning-letter/MorningLetterChat.svelte";

// Mobile
import MobileChat from "$lib/components/mobile/morning-letter/MorningLetterChat.svelte";

const { isDesktop } = useViewport();

const ml = useMorningLetter(page.data.letter ?? null);

const targetDate = $derived(
	ml.letter?.targetDate ?? page.data.requestedDate ?? undefined,
);
const withinHours = $derived(deriveWithinHours(targetDate));
const dateDisplay = $derived(
	targetDate
		? formatLetterDate(targetDate, ml.letter?.editionTimezone)
		: "Today's Edition",
);

let revealed = $state(false);

onMount(() => {
	requestAnimationFrame(() => {
		revealed = true;
	});
	if (page.data.error && !ml.letter) {
		void ml.fetchLetter();
	}
});

async function handleRegenerate() {
	const result = await ml.regenerate();
	if (result.regenerated && ml.letter?.targetDate) {
		await goto(
			`/recap/morning-letter?date=${encodeURIComponent(ml.letter.targetDate)}`,
			{ replaceState: true, noScroll: true, invalidateAll: false },
		);
	}
}

function handlePreviousLetterSelected(targetDate: string) {
	void goto(`/recap/morning-letter?date=${encodeURIComponent(targetDate)}`, {
		noScroll: false,
	});
}
</script>

<svelte:head>
	<title>Morning Letter - Alt</title>
</svelte:head>

<div class="morning-letter-page" class:revealed data-role="morning-letter-page">
	{#if isDesktop}
		<!-- Desktop: Edition header + document/chat layout -->
		<header class="edition-header">
			<span class="edition-label">Morning Letter</span>
			<h1 class="edition-dateline">{dateDisplay}</h1>
			<div class="edition-rule"></div>
		</header>

		<div class="flex gap-6 max-w-7xl mx-auto px-4 min-h-[60vh]">
			<!-- Document (2/3) -->
			<div class="flex-[2] min-w-0">
				{#if ml.letterLoading}
					<MorningLetterSkeleton minHeight="60vh" />
				{:else if !ml.letter}
					<MorningLetterEmpty
						requestedDate={page.data.requestedDate}
						onRegenerate={handleRegenerate}
						regenerating={ml.regenerating}
						regenerateDisabledReason={ml.regenerateCooldownMsg}
					/>
				{:else}
					<MorningLetterDocumentCore
						letter={ml.letter}
						sources={ml.sources}
						sourcesLoading={ml.sourcesLoading}
						enrichments={ml.enrichments}
						enrichmentsLoading={ml.enrichmentsLoading}
						onPreviousLetterSelected={handlePreviousLetterSelected}
					/>
				{/if}
			</div>

			<!-- Follow-up Chat (1/3) -->
			<aside class="flex-1 min-h-[40vh] max-w-md">
				<DesktopChat {withinHours} {targetDate} />
			</aside>
		</div>
	{:else}
		<!-- Mobile: header + document + chat disclosure -->
		<div class="min-h-[100dvh] flex flex-col" style="background: var(--app-bg);">
			<!-- Mobile header -->
			<header class="mobile-edition-header">
				<span class="edition-label">Morning Letter</span>
				<h1 class="mobile-edition-dateline">{dateDisplay}</h1>
			</header>

			<!-- Document -->
			<div class="flex-1 p-4">
				{#if ml.letterLoading}
					<MorningLetterSkeleton minHeight="40vh" />
				{:else if !ml.letter}
					<MorningLetterEmpty
						requestedDate={page.data.requestedDate}
						onRegenerate={handleRegenerate}
						regenerating={ml.regenerating}
						regenerateDisabledReason={ml.regenerateCooldownMsg}
					/>
				{:else}
					<MorningLetterDocumentCore
						letter={ml.letter}
						sources={ml.sources}
						sourcesLoading={ml.sourcesLoading}
						enrichments={ml.enrichments}
						enrichmentsLoading={ml.enrichmentsLoading}
						onPreviousLetterSelected={handlePreviousLetterSelected}
					/>
				{/if}
			</div>

			<!-- Follow-up Chat (disclosure) -->
			<MobileChat {withinHours} {targetDate} />
		</div>
	{/if}
</div>

<style>
	/* ===== Page Reveal ===== */
	.morning-letter-page {
		opacity: 0;
		transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}
	.morning-letter-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	/* ===== Desktop Edition Header ===== */
	.edition-header {
		max-width: 80rem;
		margin: 0 auto;
		padding: 1rem 1rem 0;
		margin-bottom: 1.5rem;
	}

	.edition-label {
		display: block;
		font-family: var(--font-body, "Source Sans 3", sans-serif);
		font-size: 0.65rem;
		font-weight: 600;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		color: var(--alt-ash, #999);
		margin-bottom: 0.2rem;
	}

	.edition-dateline {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.5rem;
		font-weight: 700;
		line-height: 1.2;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0 0 0.75rem;
	}

	.edition-rule {
		height: 1px;
		background: var(--surface-border, #c8c8c8);
	}

	/* ===== Mobile Edition Header ===== */
	.mobile-edition-header {
		position: sticky;
		top: 0;
		z-index: 10;
		padding: 0.75rem 1rem;
		padding-top: calc(0.75rem + env(safe-area-inset-top, 0px));
		background: var(--surface-bg, #faf9f7);
		border-bottom: 1px solid var(--surface-border, #c8c8c8);
	}

	.mobile-edition-dateline {
		font-family: var(--font-display, "Playfair Display", serif);
		font-size: 1.2rem;
		font-weight: 700;
		color: var(--alt-charcoal, #1a1a1a);
		margin: 0.15rem 0 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.morning-letter-page {
			transition: none;
			opacity: 1;
			transform: none;
		}
	}
</style>
