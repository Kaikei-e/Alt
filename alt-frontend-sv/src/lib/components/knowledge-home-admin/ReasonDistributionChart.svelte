<script lang="ts">
let { distribution }: { distribution: { code: string; count: number }[] } = $props();

const maxCount = $derived(
	distribution.length > 0 ? Math.max(...distribution.map((d) => d.count)) : 1,
);

const reasonColors: Record<string, string> = {
	new_unread: "var(--accent-blue, #3b82f6)",
	summary_completed: "var(--accent-teal, #14b8a6)",
	tag_hotspot: "var(--accent-green, #22c55e)",
	in_weekly_recap: "var(--accent-purple, #a855f7)",
	pulse_need_to_know: "var(--accent-orange, #f97316)",
};

const getColor = (code: string) => reasonColors[code] ?? "var(--text-secondary)";
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Why Reason Distribution
	</h3>

	{#if distribution.length === 0}
		<p class="text-xs" style="color: var(--text-secondary);">No data available.</p>
	{:else}
		<div class="flex flex-col gap-2">
			{#each distribution as item}
				<div class="flex items-center gap-2 text-xs">
					<span
						class="w-32 truncate text-right"
						style="color: var(--text-secondary);"
					>
						{item.code}
					</span>
					<div
						class="h-4 rounded"
						style="width: {(item.count / maxCount) * 100}%; min-width: 4px; background: {getColor(item.code)};"
					></div>
					<span style="color: var(--text-primary);">{item.count}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
