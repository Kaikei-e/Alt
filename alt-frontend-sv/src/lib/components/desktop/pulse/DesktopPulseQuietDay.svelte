<script lang="ts">
import type { QuietDayInfo } from "$lib/schema/evening_pulse";
import { Button } from "$lib/components/ui/button";
import PulseWeeklyHighlight from "$lib/components/pulse/PulseWeeklyHighlight.svelte";

interface Props {
	date: string;
	quietDay?: QuietDayInfo;
	onNavigateToRecap?: () => void;
	onHighlightClick?: (id: string) => void;
}

const { date, quietDay, onNavigateToRecap, onHighlightClick }: Props = $props();

const formattedDate = $derived.by(() => {
	try {
		const d = new Date(date);
		return d.toLocaleDateString("en-US", {
			month: "long",
			day: "numeric",
			weekday: "long",
		});
	} catch {
		return date;
	}
});
</script>

<div class="p-6 max-w-7xl mx-auto">
	<header class="mb-8">
		<h1 class="text-3xl font-bold" style="color: var(--text-primary);">
			Evening Pulse
		</h1>
		<p class="text-sm mt-1" style="color: var(--text-secondary);">
			{formattedDate}
		</p>
	</header>

	<div
		class="p-8 border-2 border-[var(--surface-border)] text-center max-w-2xl mx-auto mb-8"
		style="background: white;"
	>
		<div class="text-5xl mb-4" aria-hidden="true">&#9790;</div>
		<h2
			class="text-2xl font-semibold mb-3"
			style="color: var(--text-primary);"
		>
			Quiet Day
		</h2>
		<p
			class="text-sm mb-6 max-w-md mx-auto"
			style="color: var(--text-secondary);"
		>
			{quietDay?.message ?? "Today was a quiet day. No notable news was found."}
		</p>

		{#if onNavigateToRecap}
			<Button
				onclick={onNavigateToRecap}
				class="px-6 py-2"
				style="background: var(--alt-primary); color: white;"
			>
				View 7-Day Recap
			</Button>
		{/if}
	</div>

	{#if quietDay?.weeklyHighlights && quietDay.weeklyHighlights.length > 0}
		<section class="mt-8">
			<h3
				class="text-lg font-semibold mb-4"
				style="color: var(--text-primary);"
			>
				This Week's Highlights
			</h3>
			<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
				{#each quietDay.weeklyHighlights as highlight (highlight.id)}
					<PulseWeeklyHighlight
						{highlight}
						onclick={() => onHighlightClick?.(highlight.id)}
					/>
				{/each}
			</div>
		</section>
	{/if}
</div>
