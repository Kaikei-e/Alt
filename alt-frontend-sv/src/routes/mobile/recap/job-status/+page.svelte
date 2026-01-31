<script lang="ts">
import { onMount } from "svelte";
import { useJobProgress } from "$lib/hooks/useJobProgress.svelte";
import { triggerRecapJob } from "$lib/api/client/dashboard";
import { loadingStore } from "$lib/stores/loading.svelte";
import type { TimeWindow } from "$lib/schema/dashboard";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import {
	MobileJobStatusHeader,
	MobileStatsRow,
	MobileActiveJobPanel,
	MobileJobHistoryList,
	MobileControlBar,
	MobileJobDetailSheet,
} from "$lib/components/mobile/recap/job-status";
import type { RecentJobSummary } from "$lib/schema/dashboard";

const jobProgress = useJobProgress({
	initialWindow: "24h",
	pollInterval: 5000,
});

let triggering = $state(false);
let triggerError = $state<string | null>(null);
let triggerSuccess = $state<string | null>(null);
let justStartedJobId = $state<string | null>(null);
let selectedJob = $state<RecentJobSummary | null>(null);
let detailSheetOpen = $state(false);

onMount(async () => {
	loadingStore.startLoading();
	await jobProgress.fetchData();
	loadingStore.stopLoading();
	jobProgress.startPolling();
});

function handleWindowChange(window: TimeWindow) {
	jobProgress.setWindow(window);
}

async function handleRefresh() {
	await jobProgress.refresh();
}

async function handleTriggerJob() {
	if (triggering || justStartedJobId) return;

	triggering = true;
	triggerError = null;
	triggerSuccess = null;

	try {
		const result = await triggerRecapJob(fetch);
		justStartedJobId = result.job_id;
		triggerSuccess = `Job ${result.job_id.slice(0, 8)}... started`;

		// Refresh data after triggering
		setTimeout(async () => {
			await jobProgress.refresh();
			if (jobProgress.data?.active_job) {
				justStartedJobId = null;
			}
		}, 1000);

		// Fallback: force clear optimistic lock
		setTimeout(() => {
			justStartedJobId = null;
		}, 10000);

		// Clear success message
		setTimeout(() => {
			triggerSuccess = null;
		}, 5000);
	} catch (e) {
		triggerError = e instanceof Error ? e.message : "Failed to trigger job";
		justStartedJobId = null;
	} finally {
		triggering = false;
	}
}

function handleJobSelect(job: RecentJobSummary) {
	selectedJob = job;
	detailSheetOpen = true;
}

function handleCloseDetailSheet() {
	detailSheetOpen = false;
	selectedJob = null;
}

// Computed values
const successRate = $derived(
	jobProgress.data?.stats.success_rate_24h
		? `${(jobProgress.data.stats.success_rate_24h * 100).toFixed(1)}%`
		: "-",
);

const avgDuration = $derived.by(() => {
	const secs = jobProgress.data?.stats.avg_duration_secs;
	if (!secs) return "-";
	if (secs < 60) return `${secs}s`;
	const mins = Math.floor(secs / 60);
	return `${mins}m`;
});

const hasRunningJob = $derived.by(() => {
	const d = jobProgress.data;
	return d?.active_job != null;
});
</script>

<svelte:head>
	<title>Job Status - Alt</title>
</svelte:head>

<div class="min-h-[100dvh] pb-20 relative" style="background: var(--app-bg);">
	{#if jobProgress.loading && !jobProgress.data}
		<!-- Loading skeleton -->
		<div class="p-4" data-testid="job-status-skeleton">
			<div class="h-8 bg-muted rounded w-1/2 mb-4 animate-pulse"></div>
			<div class="flex gap-3 overflow-x-auto pb-4">
				{#each Array(4) as _}
					<div class="min-w-[120px] h-20 bg-muted rounded-lg animate-pulse"></div>
				{/each}
			</div>
			<div class="space-y-3 mt-4">
				{#each Array(3) as _}
					<div class="h-24 bg-muted rounded-lg animate-pulse"></div>
				{/each}
			</div>
		</div>
	{:else if jobProgress.error}
		<!-- Error state -->
		<div class="flex flex-col items-center justify-center min-h-[50vh] p-6">
			<div
				class="p-6 rounded-lg border text-center"
				style="background: var(--surface-bg); border-color: hsl(var(--destructive));"
			>
				<p class="font-semibold mb-2" style="color: hsl(var(--destructive));">
					Error loading job data
				</p>
				<p class="text-sm mb-4" style="color: var(--text-secondary);">
					{jobProgress.error}
				</p>
				<button
					onclick={() => jobProgress.refresh()}
					class="px-4 py-2 rounded disabled:opacity-50"
					style="background: var(--alt-primary); color: white;"
				>
					Retry
				</button>
			</div>
		</div>
	{:else if jobProgress.data}
		<!-- Header with time window selector -->
		<MobileJobStatusHeader
			currentWindow={jobProgress.currentWindow}
			onWindowChange={handleWindowChange}
		/>

		<!-- Stats row (horizontally scrollable) -->
		<MobileStatsRow
			successRate={successRate}
			avgDuration={avgDuration}
			totalJobs={jobProgress.data.stats.total_jobs_24h}
			runningJobs={jobProgress.data.stats.running_jobs}
			failedJobs={jobProgress.data.stats.failed_jobs_24h}
		/>

		<!-- Success/Error feedback -->
		{#if triggerSuccess}
			<div class="mx-4 mb-4 p-3 rounded-lg bg-green-50 border border-green-200">
				<p class="text-sm text-green-700">{triggerSuccess}</p>
			</div>
		{/if}

		{#if triggerError}
			<div class="mx-4 mb-4 p-3 rounded-lg bg-red-50 border border-red-200">
				<p class="text-sm text-red-700">{triggerError}</p>
			</div>
		{/if}

		<!-- Active job panel (collapsible) -->
		<MobileActiveJobPanel job={jobProgress.data.active_job} />

		<!-- Job history list -->
		<MobileJobHistoryList
			jobs={jobProgress.data.recent_jobs}
			onJobSelect={handleJobSelect}
		/>

		<!-- Job detail bottom sheet -->
		<MobileJobDetailSheet
			job={selectedJob}
			open={detailSheetOpen}
			onClose={handleCloseDetailSheet}
		/>
	{:else}
		<div class="text-center py-12">
			<p style="color: var(--text-muted);">No job data available</p>
		</div>
	{/if}

	<!-- Fixed bottom control bar -->
	<MobileControlBar
		onRefresh={handleRefresh}
		onTriggerJob={handleTriggerJob}
		loading={jobProgress.loading}
		triggering={triggering}
		hasRunningJob={hasRunningJob}
		justStartedJobId={justStartedJobId}
	/>

	<FloatingMenu />
</div>
