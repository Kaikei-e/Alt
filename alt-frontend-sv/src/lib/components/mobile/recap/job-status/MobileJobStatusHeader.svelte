<script lang="ts">
	import type { TimeWindow } from "$lib/schema/dashboard";

	interface Props {
		currentWindow: TimeWindow;
		onWindowChange: (window: TimeWindow) => void;
	}

	let { currentWindow, onWindowChange }: Props = $props();

	const timeWindows: { label: string; value: TimeWindow }[] = [
		{ label: "4h", value: "4h" },
		{ label: "24h", value: "24h" },
		{ label: "3d", value: "3d" },
		{ label: "7d", value: "7d" },
	];
</script>

<div class="sticky top-0 z-10 px-4 py-3" style="background: var(--app-bg);">
	<h1
		class="text-xl font-bold mb-3"
		style="color: var(--text-primary);"
	>
		Job Status
	</h1>

	<!-- Time window selector (horizontal scroll pills) -->
	<div class="flex gap-2 overflow-x-auto pb-2 -mx-1 px-1 scrollbar-hide">
		{#each timeWindows as tw}
			<button
				class="flex-shrink-0 px-4 py-2 rounded-full text-sm font-medium transition-colors min-h-[44px]"
				style={currentWindow === tw.value
					? "background: var(--alt-primary, #2f4f4f); color: #ffffff;"
					: "background: var(--surface-bg, #f9fafb); color: var(--text-primary, #1a1a1a); border: 1px solid var(--surface-border, #e5e7eb);"}
				aria-pressed={currentWindow === tw.value}
				onclick={() => onWindowChange(tw.value)}
			>
				{tw.label}
			</button>
		{/each}
	</div>
</div>

<style>
	.scrollbar-hide {
		-ms-overflow-style: none;
		scrollbar-width: none;
	}
	.scrollbar-hide::-webkit-scrollbar {
		display: none;
	}
</style>
