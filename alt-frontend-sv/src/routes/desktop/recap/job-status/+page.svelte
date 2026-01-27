<script lang="ts">
	import { onMount } from "svelte";
	import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
	import {
		MetricCard,
		ActiveJobCard,
		JobHistoryTable,
	} from "$lib/components/desktop/recap/job-status";
	import { useJobProgress } from "$lib/hooks/useJobProgress.svelte";
	import { triggerRecapJob } from "$lib/api/client/dashboard";
	import type { TimeWindow } from "$lib/schema/dashboard";
	import { TIME_WINDOWS } from "$lib/schema/dashboard";
	import { loadingStore } from "$lib/stores/loading.svelte";
	import {
		Activity,
		CheckCircle,
		XCircle,
		Clock,
		RefreshCw,
		Pause,
		Play,
		Rocket,
	} from "@lucide/svelte";

	// Time window options
	const timeWindows: { label: string; value: TimeWindow }[] = [
		{ label: "4h", value: "4h" },
		{ label: "24h", value: "24h" },
		{ label: "3d", value: "3d" },
		{ label: "7d", value: "7d" },
	];

	const jobProgress = useJobProgress({ initialWindow: "24h", pollInterval: 5000 });

	let triggering = $state(false);
	let triggerError = $state<string | null>(null);
	let triggerSuccess = $state<string | null>(null);
	let justStartedJobId = $state<string | null>(null);

	onMount(async () => {
		loadingStore.startLoading();
		await jobProgress.fetchData();
		loadingStore.stopLoading();
		jobProgress.startPolling();
	});

	function handleWindowChange(window: TimeWindow) {
		jobProgress.setWindow(window);
	}

	function togglePolling() {
		if (jobProgress.isPolling) {
			jobProgress.stopPolling();
		} else {
			jobProgress.startPolling();
		}
	}

	async function handleTriggerJob() {
		if (triggering || justStartedJobId) return;

		triggering = true;
		triggerError = null;
		triggerSuccess = null;

		try {
			const result = await triggerRecapJob(fetch);
			justStartedJobId = result.job_id;
			triggerSuccess = `Job ${result.job_id.slice(0, 8)}... started with ${result.genres.length} genres`;

			// Refresh data after triggering
			setTimeout(async () => {
				await jobProgress.refresh();
				// Clear optimistic lock if active_job is now present
				if (jobProgress.data?.active_job) {
					justStartedJobId = null;
				}
			}, 1000);

			// Fallback: force clear optimistic lock after 10 seconds
			setTimeout(() => {
				justStartedJobId = null;
			}, 10000);

			// Clear success message after 5 seconds
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

	// Computed values for metrics
	const successRate = $derived(
		jobProgress.data?.stats.success_rate_24h
			? `${(jobProgress.data.stats.success_rate_24h * 100).toFixed(1)}%`
			: "-"
	);

	const avgDuration = $derived.by(() => {
		const secs = jobProgress.data?.stats.avg_duration_secs;
		if (!secs) return "-";
		if (secs < 60) return `${secs}s`;
		const mins = Math.floor(secs / 60);
		return `${mins}m`;
	});

	// Check if there's already a running job
	// Use $derived.by() for explicit tracking through getter
	const hasRunningJob = $derived.by(() => {
		const d = jobProgress.data;
		return d?.active_job != null;
	});
	const runningJobTooltip = $derived.by(() => {
		if (justStartedJobId) return "Job is starting...";
		const activeJob = jobProgress.data?.active_job;
		if (!activeJob) return "Start a new recap job";
		const source = activeJob.trigger_source === "user" ? "user" : "system";
		return `A ${source} job is already running`;
	});
</script>

<PageHeader
	title="Recap Job Status"
	description="Monitor real-time job progress and pipeline status"
/>

<!-- Controls bar -->
<div
	class="flex items-center justify-between mb-6 pb-4 border-b"
	style="border-color: var(--surface-border);"
>
	<!-- Time window selector -->
	<div class="flex items-center gap-2">
		<span class="text-sm font-medium" style="color: var(--text-muted);">
			Time Window:
		</span>
		<div class="flex rounded-lg overflow-hidden border" style="border-color: var(--surface-border, #e5e7eb);">
			{#each timeWindows as tw}
				<button
					data-testid="time-window-{tw.value}"
					class="px-3 py-1.5 text-sm font-medium transition-colors"
					style={jobProgress.currentWindow === tw.value
						? "background: var(--alt-primary, #2f4f4f); color: #ffffff;"
						: "background: var(--surface-bg, #f9fafb); color: var(--text-primary, #1a1a1a);"}
					aria-pressed={jobProgress.currentWindow === tw.value}
					onmouseenter={(e) => {
						if (jobProgress.currentWindow !== tw.value) {
							e.currentTarget.style.background = 'var(--surface-hover, #f3f4f6)';
						}
					}}
					onmouseleave={(e) => {
						if (jobProgress.currentWindow !== tw.value) {
							e.currentTarget.style.background = 'var(--surface-bg, #f9fafb)';
						}
					}}
					onclick={() => handleWindowChange(tw.value)}
				>
					{tw.label}
				</button>
			{/each}
		</div>
	</div>

	<!-- Polling controls -->
	<div class="flex items-center gap-3">
		<button
			class="flex items-center gap-2 px-3 py-1.5 rounded-lg border transition-colors hover:bg-gray-100"
			style="border-color: var(--surface-border); color: var(--text-primary);"
			onclick={() => jobProgress.refresh()}
			disabled={jobProgress.loading}
		>
			<RefreshCw class="w-4 h-4 {jobProgress.loading ? 'animate-spin' : ''}" />
			Refresh
		</button>
		<button
			class="flex items-center gap-2 px-3 py-1.5 rounded-lg border transition-colors
				{jobProgress.isPolling ? 'bg-green-50 border-green-200' : 'hover:bg-gray-100'}"
			style={!jobProgress.isPolling ? "border-color: var(--surface-border); color: var(--text-primary);" : "color: #16a34a;"}
			onclick={togglePolling}
		>
			{#if jobProgress.isPolling}
				<Pause class="w-4 h-4" />
				<span>Auto-refresh ON</span>
			{:else}
				<Play class="w-4 h-4" />
				<span>Auto-refresh OFF</span>
			{/if}
		</button>
		<button
			class="flex items-center gap-2 px-4 py-1.5 rounded-lg transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
			style="background: var(--alt-primary, #2f4f4f); color: #ffffff;"
			onclick={handleTriggerJob}
			disabled={triggering || hasRunningJob || justStartedJobId !== null}
			title={runningJobTooltip}
		>
			<Rocket class="w-4 h-4 {triggering ? 'animate-pulse' : ''}" />
			{#if triggering}
				<span>Starting...</span>
			{:else}
				<span>Start Job</span>
			{/if}
		</button>
	</div>
</div>

{#if triggerSuccess}
	<div class="mb-4 p-3 rounded-lg bg-green-50 border border-green-200">
		<p class="text-sm text-green-700">{triggerSuccess}</p>
	</div>
{/if}

{#if triggerError}
	<div class="mb-4 p-3 rounded-lg bg-red-50 border border-red-200">
		<p class="text-sm text-red-700">{triggerError}</p>
	</div>
{/if}

{#if jobProgress.error}
	<div class="mb-6 p-4 rounded-lg bg-red-50 border border-red-200">
		<p class="text-sm text-red-700">
			Error loading job data: {jobProgress.error}
		</p>
	</div>
{/if}

{#if jobProgress.data}
	<!-- Stats cards -->
	<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-6">
		<MetricCard
			title="Success Rate"
			value={successRate}
			subtitle="Last 24 hours"
			icon={CheckCircle}
		/>
		<MetricCard
			title="Avg Duration"
			value={avgDuration}
			subtitle="Per job"
			icon={Clock}
		/>
		<MetricCard
			title="Jobs Today"
			value={jobProgress.data.stats.total_jobs_24h}
			subtitle={`${jobProgress.data.stats.running_jobs} running`}
			icon={Activity}
		/>
		<MetricCard
			title="Failed Jobs"
			value={jobProgress.data.stats.failed_jobs_24h}
			subtitle="Last 24 hours"
			icon={XCircle}
		/>
	</div>

	<!-- Active job section -->
	{#if jobProgress.data.active_job}
		<div class="mb-6">
			<h2
				class="text-lg font-semibold mb-4"
				style="color: var(--text-primary);"
			>
				Currently Running
			</h2>
			<ActiveJobCard job={jobProgress.data.active_job} />
		</div>
	{:else}
		<div
			class="mb-6 p-6 rounded-lg border text-center"
			style="background: var(--surface-bg); border-color: var(--surface-border);"
		>
			<Activity class="w-8 h-8 mx-auto mb-2" style="color: var(--text-muted);" />
			<p style="color: var(--text-muted);">No job currently running</p>
		</div>
	{/if}

	<!-- Job history section -->
	<div>
		<h2
			class="text-lg font-semibold mb-4"
			style="color: var(--text-primary);"
		>
			Recent Jobs
		</h2>
		<JobHistoryTable jobs={jobProgress.data.recent_jobs} stats={jobProgress.data.stats} />
	</div>
{:else if jobProgress.loading}
	<!-- Loading state handled by SystemLoader -->
{:else}
	<div class="text-center py-12">
		<p style="color: var(--text-muted);">No job data available</p>
	</div>
{/if}
