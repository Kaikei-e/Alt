<script lang="ts">
let {
	activeTab,
	onTabChange,
}: {
	activeTab: string;
	onTabChange: (tab: string) => void;
} = $props();

const tabs = [
	{ id: "overview", label: "Overview" },
	{ id: "slo", label: "SLO" },
	{ id: "reproject", label: "Reproject" },
];
</script>

<nav class="flex gap-1 rounded-lg p-1" style="background: var(--surface-bg);">
	{#each tabs as tab (tab.id)}
		<button
			type="button"
			class="rounded-md px-4 py-1.5 text-sm font-medium transition-colors"
			style={activeTab === tab.id
				? "background: var(--accent-blue, #3b82f6); color: #fff;"
				: "background: transparent; color: var(--text-secondary);"}
			onclick={() => onTabChange(tab.id)}
		>
			{tab.label}
		</button>
	{/each}
</nav>
