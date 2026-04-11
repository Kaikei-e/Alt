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
	{ id: "system", label: "System" },
	{ id: "reproject", label: "Reproject" },
	{ id: "storage", label: "Storage" },
	{ id: "audit", label: "Audit" },
];
</script>

<nav class="tab-bar" data-role="admin-tab-nav">
	{#each tabs as tab (tab.id)}
		<button
			type="button"
			class="tab-item"
			class:active={activeTab === tab.id}
			onclick={() => onTabChange(tab.id)}
		>
			{tab.label}
		</button>
	{/each}
</nav>

<style>
	.tab-bar {
		display: flex;
		gap: 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.tab-item {
		padding: 0.5rem 1rem;
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		background: transparent;
		border: none;
		border-bottom: 2px solid transparent;
		cursor: pointer;
		transition: color 0.15s, border-color 0.15s;
	}

	.tab-item:hover {
		color: var(--alt-slate);
	}

	.tab-item.active {
		color: var(--alt-charcoal);
		border-bottom-color: var(--alt-charcoal);
	}
</style>
