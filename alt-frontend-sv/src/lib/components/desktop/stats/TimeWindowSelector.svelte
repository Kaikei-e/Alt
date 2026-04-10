<script lang="ts">
import type { TimeWindow } from "$lib/schema/stats";

interface Props {
	selected: TimeWindow;
	onchange: (window: TimeWindow) => void;
}

let { selected, onchange }: Props = $props();

const windows: { value: TimeWindow; label: string }[] = [
	{ value: "4h", label: "4H" },
	{ value: "24h", label: "24H" },
	{ value: "3d", label: "3D" },
	{ value: "7d", label: "7D" },
];

function handleClick(window: TimeWindow) {
	if (window !== selected) {
		onchange(window);
	}
}
</script>

<div class="window-bar">
	{#each windows as { value, label }}
		<button
			type="button"
			class="window-btn"
			class:window-btn--active={selected === value}
			onclick={() => handleClick(value)}
		>
			{label}
		</button>
	{/each}
</div>

<style>
	.window-bar {
		display: flex;
	}

	.window-btn {
		border: 1px solid var(--surface-border);
		background: transparent;
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--alt-ash);
		padding: 0.3rem 0.6rem;
		cursor: pointer;
		transition:
			background 0.15s,
			color 0.15s;
	}

	.window-btn + .window-btn {
		border-left: 0;
	}

	.window-btn:hover:not(.window-btn--active) {
		background: var(--surface-hover);
	}

	.window-btn--active {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
		border-color: var(--alt-charcoal);
	}
</style>
