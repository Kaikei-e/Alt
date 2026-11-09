<script lang="ts">
import PulseHeader from "#lib/components/pulse/PulseHeader.svelte";
import PulseWeeklyHighlight from "#lib/components/pulse/PulseWeeklyHighlight.svelte";
import { Button } from "#lib/components/ui/button/index.js";
import type { QuietDayInfo } from "#lib/schema/evening_pulse.js";

interface Props {
	date: string;
	quietDay?: QuietDayInfo;
	onNavigateToRecap?: () => void;
	onHighlightClick?: (id: string) => void;
}

const { date, quietDay, onNavigateToRecap, onHighlightClick }: Props = $props();
</script>

<div class="p-6 max-w-7xl mx-auto">
	<PulseHeader {date} />

	<div
		class="p-8 border text-center max-w-2xl mx-auto mb-8"
		style="background: var(--surface-bg); border-color: var(--surface-border);"
	>
		<h2
			class="text-2xl font-bold mb-3"
			style="color: var(--text-primary); font-family: var(--font-display);"
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
				class="px-6 py-2 rounded-none font-mono uppercase tracking-wider text-xs"
				style="background: var(--alt-charcoal); color: var(--surface-bg); border: 1px solid var(--alt-charcoal);"
			>
				View 3-Day Recap
			</Button>
		{/if}
	</div>

	{#if quietDay?.weeklyHighlights && quietDay.weeklyHighlights.length > 0}
		<section class="mt-8">
			<h3
				class="text-lg font-bold mb-4"
				style="color: var(--text-primary); font-family: var(--font-display);"
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
