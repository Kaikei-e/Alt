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
	{ id: "observability", label: "Observability" },
];
</script>

<nav
	class="tab-bar"
	data-role="admin-tab-nav"
	aria-label="Operations sections"
>
	{#each tabs as tab (tab.id)}
		<button
			type="button"
			class="tab-item"
			class:active={activeTab === tab.id}
			aria-current={activeTab === tab.id ? "page" : undefined}
			onclick={() => onTabChange(tab.id)}
		>
			{tab.label}
		</button>
	{/each}
</nav>

<style>
	.tab-bar {
		display: flex;
		flex-wrap: nowrap;
		gap: 0;
		max-width: 100%;
		min-width: 0;
		overflow-x: auto;
		-webkit-overflow-scrolling: touch;
		overscroll-behavior-x: contain;
		scrollbar-width: none;
		border-bottom: 1px solid var(--surface-border);
	}

	.tab-bar::-webkit-scrollbar {
		display: none;
	}

	.tab-item {
		display: inline-flex;
		align-items: center;
		flex-shrink: 0;
		white-space: nowrap;
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

	@media (max-width: 48rem) {
		.tab-item {
			min-height: 2.75rem;
			padding: 0.65rem 0.8rem;
		}
	}
</style>
