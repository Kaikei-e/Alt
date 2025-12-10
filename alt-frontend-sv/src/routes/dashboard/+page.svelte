<script lang="ts">
	import OverviewTab from "$lib/components/dashboard/OverviewTab.svelte";
	import ClassificationTab from "$lib/components/dashboard/ClassificationTab.svelte";
	import ClusteringTab from "$lib/components/dashboard/ClusteringTab.svelte";
	import SummarizationTab from "$lib/components/dashboard/SummarizationTab.svelte";
	import LogAnalysisTab from "$lib/components/dashboard/LogAnalysisTab.svelte";
	import AdminJobsTab from "$lib/components/dashboard/AdminJobsTab.svelte";
	import SystemMonitorTab from "$lib/components/dashboard/SystemMonitorTab.svelte";
	import type { TimeWindow } from "$lib/schema/dashboard";
	import { TIME_WINDOWS } from "$lib/schema/dashboard";

	let selectedTab = $state(0);
	let timeWindow = $state<TimeWindow>("4h");

	const windowSeconds = $derived(TIME_WINDOWS[timeWindow]);

	const timeWindowOptions: TimeWindow[] = ["4h", "24h", "3d"];

	const tabs = [
		{ name: "Overview", component: OverviewTab },
		{ name: "Classification", component: ClassificationTab },
		{ name: "Clustering", component: ClusteringTab },
		{ name: "Summarization", component: SummarizationTab },
		{ name: "Log Analysis", component: LogAnalysisTab },
		{ name: "System Monitor", component: SystemMonitorTab },
		{ name: "Admin Jobs", component: AdminJobsTab },
	];
</script>

<div class="p-8 max-w-7xl mx-auto" data-style="alt-paper">
	<h1 class="text-3xl font-bold mb-6" style="color: var(--text-primary);">
		Recap System Evaluation Dashboard
	</h1>

	<!-- Time Range Selection -->
	<div class="mb-6">
		<label
			class="block text-sm font-medium mb-2"
			style="color: var(--text-primary);"
		>
			Time Range
		</label>
		<div class="flex gap-4">
			{#each timeWindowOptions as window}
				<label
					class="flex items-center gap-2 cursor-pointer"
					style="color: var(--text-primary);"
				>
					<input
						type="radio"
						name="timeWindow"
						value={window}
						bind:group={timeWindow}
						class="cursor-pointer"
					/>
					<span>{window}</span>
				</label>
			{/each}
		</div>
	</div>

	<!-- Tabs -->
	<div class="mb-6">
		<div
			class="flex gap-1 border-b"
			style="border-color: var(--surface-border);"
		>
			{#each tabs as tab, index}
				<button
					type="button"
					onclick={() => {
						selectedTab = index;
					}}
					class="px-6 py-3 text-sm font-medium transition-colors"
					style="
						color: {selectedTab === index
							? 'var(--text-primary)'
							: 'var(--text-muted)'};
						border-bottom: 2px solid {selectedTab === index
							? 'var(--alt-primary)'
							: 'transparent'};
						background: {selectedTab === index
							? 'var(--surface-hover)'
							: 'transparent'};
					"
				>
					{tab.name}
				</button>
			{/each}
		</div>
	</div>

	<!-- Tab Content -->
	<div
		class="p-6 border"
		style="
			background: var(--surface-bg);
			border-color: var(--surface-border);
			box-shadow: var(--shadow-sm);
		"
	>
		{#if selectedTab === 0}
			<OverviewTab {windowSeconds} />
		{:else if selectedTab === 1}
			<ClassificationTab {windowSeconds} />
		{:else if selectedTab === 2}
			<ClusteringTab {windowSeconds} />
		{:else if selectedTab === 3}
			<SummarizationTab {windowSeconds} />
		{:else if selectedTab === 4}
			<LogAnalysisTab {windowSeconds} />
		{:else if selectedTab === 5}
			<SystemMonitorTab />
		{:else if selectedTab === 6}
			<AdminJobsTab {windowSeconds} />
		{/if}
	</div>
</div>
