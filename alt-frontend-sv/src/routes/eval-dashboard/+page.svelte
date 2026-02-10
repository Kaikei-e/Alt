<script lang="ts">
import AdminJobsTab from "$lib/components/dashboard/AdminJobsTab.svelte";
import ClassificationTab from "$lib/components/dashboard/ClassificationTab.svelte";
import ClusteringTab from "$lib/components/dashboard/ClusteringTab.svelte";
import LogAnalysisTab from "$lib/components/dashboard/LogAnalysisTab.svelte";
import OverviewTab from "$lib/components/dashboard/OverviewTab.svelte";
import RecapJobsTab from "$lib/components/dashboard/RecapJobsTab.svelte";
import SummarizationTab from "$lib/components/dashboard/SummarizationTab.svelte";
import SystemMonitorTab from "$lib/components/dashboard/SystemMonitorTab.svelte";
import { buttonVariants } from "$lib/components/ui/button";
import type { TimeWindow } from "$lib/schema/dashboard";
import { TIME_WINDOWS } from "$lib/schema/dashboard";
import { cn } from "$lib/utils.js";

let selectedTab = $state(0);
let timeWindow = $state<TimeWindow>("4h");

const windowSeconds = $derived(TIME_WINDOWS[timeWindow]);

const timeWindowOptions: TimeWindow[] = ["4h", "24h", "3d", "7d"];

// Tab organization:
// 1. Overview - Overall summary
// 2-4. Pipeline - Processing stages
// 5-6. Monitoring - System monitoring and analysis
// 7-8. Jobs - Job management
const tabs = [
	// Overview
	{ name: "Overview", component: OverviewTab },
	// Pipeline
	{ name: "Classification", component: ClassificationTab },
	{ name: "Clustering", component: ClusteringTab },
	{ name: "Summarization", component: SummarizationTab },
	// Monitoring
	{ name: "System Monitor", component: SystemMonitorTab },
	{ name: "Log Analysis", component: LogAnalysisTab },
	// Jobs
	{ name: "Admin Jobs", component: AdminJobsTab },
	{ name: "Recap Jobs", component: RecapJobsTab },
];
</script>

<div class="p-8 max-w-7xl mx-auto" data-style="alt-paper">
	<h1 class="text-3xl font-bold mb-6" style="color: var(--text-primary);">
		Recap System Evaluation Dashboard
	</h1>

	<!-- Time Range Selection -->
	<div class="mb-6">
		<div
			class="block text-sm font-medium mb-2"
			style="color: var(--text-primary);"
		>
			Time Range
		</div>
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
					class={cn(
						buttonVariants({
							variant: selectedTab === index ? "outline" : "ghost",
							size: "sm",
						}),
						"rounded-none border-b-2 border-t-0 border-l-0 border-r-0",
						selectedTab === index
							? "border-b-[var(--text-primary)] bg-[var(--surface-hover)]"
							: "border-b-transparent",
					)}
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
			<SystemMonitorTab />
		{:else if selectedTab === 5}
			<LogAnalysisTab {windowSeconds} />
		{:else if selectedTab === 6}
			<AdminJobsTab {windowSeconds} />
		{:else if selectedTab === 7}
			<RecapJobsTab {windowSeconds} />
		{/if}
	</div>
</div>
