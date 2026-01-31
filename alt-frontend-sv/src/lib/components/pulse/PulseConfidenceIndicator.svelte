<script lang="ts">
import type { Confidence } from "$lib/schema/evening_pulse";

interface Props {
	confidence: Confidence;
}

const { confidence }: Props = $props();

const filledCount = $derived.by(() => {
	switch (confidence) {
		case "high":
			return 3;
		case "medium":
			return 2;
		case "low":
			return 1;
		default:
			return 2;
	}
});

const dots = $derived(
	Array.from({ length: 3 }, (_, i) => ({
		filled: i < filledCount,
		index: i,
	})),
);
</script>

<div
	class="inline-flex items-center gap-0.5"
	data-testid="confidence-indicator"
	aria-label="Confidence: {confidence}"
	role="img"
>
	{#each dots as dot (dot.index)}
		{#if dot.filled}
			<span
				class="w-1.5 h-1.5 rounded-full"
				style="background-color: var(--text-secondary);"
				data-testid="confidence-dot-filled"
				aria-hidden="true"
			></span>
		{:else}
			<span
				class="w-1.5 h-1.5 rounded-full"
				style="background-color: var(--surface-border);"
				data-testid="confidence-dot-empty"
				aria-hidden="true"
			></span>
		{/if}
	{/each}
</div>
