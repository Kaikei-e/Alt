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

<div class="flex gap-1">
	{#each windows as { value, label }}
		<button
			type="button"
			class="px-3 py-1 text-sm font-medium transition-colors
                   {selected === value
				? 'bg-[var(--alt-primary)] text-white'
				: 'bg-[var(--surface-bg)] text-[var(--text-secondary)] hover:bg-[var(--surface-hover)]'}
                   border border-[var(--surface-border)]"
			onclick={() => handleClick(value)}
		>
			{label}
		</button>
	{/each}
</div>
