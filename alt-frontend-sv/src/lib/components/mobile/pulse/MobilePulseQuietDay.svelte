<script lang="ts">
import PulseHeader from "#lib/components/pulse/PulseHeader.svelte";
import { SurfaceStatusBanner } from "#lib/components/recap/index.js";
import { Button } from "#lib/components/ui/button/index.js";
import type { QuietDayInfo, SurfaceStatus } from "#lib/schema/evening_pulse.js";
import { evaluateSurfaceStatus } from "#lib/utils/surface-status.js";

interface Props {
	date: string;
	quietDay?: QuietDayInfo;
	surfaceStatus?: SurfaceStatus | null;
	onNavigateToRecap?: () => void;
}

const {
	date,
	quietDay,
	surfaceStatus = null,
	onNavigateToRecap,
}: Props = $props();

const isCurrent = $derived(evaluateSurfaceStatus(surfaceStatus).isCurrent);
</script>

<div class="p-4 max-w-2xl mx-auto pb-24">
	<PulseHeader {date} {isCurrent} />

	<SurfaceStatusBanner {surfaceStatus} />

	<div
		class="p-6 border text-center"
		style="background: var(--surface-bg); border-color: var(--surface-border);"
	>
		<h2
			class="text-xl font-bold mb-2"
			style="color: var(--text-primary); font-family: var(--font-display);"
		>
			Quiet Day
		</h2>
		<p
			class="text-sm mb-6"
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
		<div class="mt-6">
			<h3
				class="text-sm font-bold mb-3"
				style="color: var(--text-primary); font-family: var(--font-display);"
			>
				This Week's Highlights
			</h3>
			<div class="flex flex-col gap-2">
				{#each quietDay.weeklyHighlights as highlight (highlight.id)}
					<div
						class="p-3 border"
						style="background: var(--surface-bg); border-color: var(--surface-border);"
					>
						<p
							class="text-sm font-medium"
							style="color: var(--text-primary); font-family: var(--font-display);"
						>
							{highlight.title}
						</p>
						<p class="text-xs font-mono mt-0.5" style="color: var(--text-muted);">
							{highlight.date}
						</p>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
