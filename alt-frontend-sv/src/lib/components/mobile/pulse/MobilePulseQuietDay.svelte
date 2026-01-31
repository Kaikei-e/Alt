<script lang="ts">
import type { QuietDayInfo } from "$lib/schema/evening_pulse";
import { Button } from "$lib/components/ui/button";

interface Props {
	date: string;
	quietDay?: QuietDayInfo;
	onNavigateToRecap?: () => void;
}

const { date, quietDay, onNavigateToRecap }: Props = $props();

const formattedDate = $derived.by(() => {
	const d = new Date(date);
	return d.toLocaleDateString("en-US", {
		month: "short",
		day: "numeric",
		weekday: "short",
	});
});
</script>

<div class="p-4 max-w-2xl mx-auto pb-24">
	<header class="mb-6">
		<h1
			class="text-2xl font-bold mb-1"
			style="color: var(--text-primary);"
		>
			Evening Pulse
		</h1>
		<p class="text-sm" style="color: var(--text-secondary);">
			{formattedDate}
		</p>
	</header>

	<div
		class="p-6 rounded-2xl text-center"
		style="background: var(--surface-bg);"
	>
		<div class="text-4xl mb-4" aria-hidden="true">
			\u263E
		</div>
		<h2
			class="text-xl font-semibold mb-2"
			style="color: var(--text-primary);"
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
				class="px-6 py-2 rounded-full"
				style="background: var(--alt-primary); color: var(--text-primary);"
			>
				View 7-Day Recap
			</Button>
		{/if}
	</div>

	{#if quietDay?.weeklyHighlights && quietDay.weeklyHighlights.length > 0}
		<div class="mt-6">
			<h3
				class="text-sm font-medium mb-3"
				style="color: var(--text-secondary);"
			>
				This Week's Highlights
			</h3>
			<div class="flex flex-col gap-2">
				{#each quietDay.weeklyHighlights as highlight (highlight.id)}
					<div
						class="p-3 rounded-lg"
						style="background: var(--surface-bg);"
					>
						<p
							class="text-sm font-medium"
							style="color: var(--text-primary);"
						>
							{highlight.title}
						</p>
						<p class="text-xs" style="color: var(--text-tertiary);">
							{highlight.date}
						</p>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
