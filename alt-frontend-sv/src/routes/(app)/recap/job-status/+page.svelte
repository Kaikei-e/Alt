<script lang="ts">
import { onMount } from "svelte";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useJobProgress } from "$lib/hooks/useJobProgress.svelte";
import { triggerRecapJob } from "$lib/api/client/dashboard";
import { getLoadingStore } from "$lib/stores/loading.svelte";
import type { TimeWindow, RecentJobSummary } from "$lib/schema/dashboard";

import {
	PageKicker,
	LedgerFigure,
} from "$lib/components/recap/job-status";
import {
	ActiveJobCard,
	JobHistoryTable,
} from "$lib/components/desktop/recap/job-status";

import {
	MobileJobStatusHeader,
	MobileStatsRow,
	MobileActiveJobPanel,
	MobileJobHistoryList,
	MobileControlBar,
	MobileJobDetailSheet,
} from "$lib/components/mobile/recap/job-status";

const { isDesktop } = useViewport();
const loadingStore = getLoadingStore();

const timeWindows: { label: string; value: TimeWindow }[] = [
	{ label: "4h", value: "4h" },
	{ label: "24h", value: "24h" },
	{ label: "3d", value: "3d" },
	{ label: "7d", value: "7d" },
];

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

function togglePolling() {
	if (jobProgress.isPolling) {
		jobProgress.stopPolling();
	} else {
		jobProgress.startPolling();
	}
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
		triggerSuccess = isDesktop
			? `Job ${result.job_id.slice(0, 8)}... started with ${result.genres.length} genres`
			: `Job ${result.job_id.slice(0, 8)}... started`;

		setTimeout(async () => {
			await jobProgress.refresh();
			if (jobProgress.data?.active_job) {
				justStartedJobId = null;
			}
		}, 1000);

		setTimeout(() => {
			justStartedJobId = null;
		}, 10000);

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

const successRate = $derived(
	jobProgress.data?.stats.success_rate_24h
		? `${(jobProgress.data.stats.success_rate_24h * 100).toFixed(1)}%`
		: "—",
);

const avgDuration = $derived.by(() => {
	const secs = jobProgress.data?.stats.avg_duration_secs;
	if (!secs) return "—";
	if (secs < 60) return `${secs}s`;
	const mins = Math.floor(secs / 60);
	return `${mins}m`;
});

const hasRunningJob = $derived.by(() => jobProgress.data?.active_job != null);

const runningJobTooltip = $derived.by(() => {
	if (justStartedJobId) return "Job is starting…";
	const activeJob = jobProgress.data?.active_job;
	if (!activeJob) return "Start a new recap job";
	const source = activeJob.trigger_source === "user" ? "user" : "system";
	return `A ${source} job is already running`;
});

const windowLabel = $derived(jobProgress.currentWindow.toUpperCase());
</script>

<svelte:head>
	<title>Job Status — Alt</title>
</svelte:head>

{#if isDesktop}
	<div class="page" data-role="job-status-page">
		<PageKicker
			kicker={`JOB STATUS · ${windowLabel} WINDOW`}
			title="Job Status"
			lede="Monitor recap pipeline runs and recent job history."
		/>

		<section class="controls" data-role="controls">
			<div class="window-group" role="group" aria-label="Time window">
				<span class="control-label">Window</span>
				{#each timeWindows as tw}
					<button
						type="button"
						data-testid="time-window-{tw.value}"
						class="window-pill"
						aria-pressed={jobProgress.currentWindow === tw.value}
						onclick={() => handleWindowChange(tw.value)}
					>
						{tw.label}
					</button>
				{/each}
			</div>

			<div class="action-group">
				<button
					type="button"
					class="action-button"
					onclick={() => jobProgress.refresh()}
					disabled={jobProgress.loading}
					data-role="refresh"
				>
					{jobProgress.loading ? "Refreshing…" : "Refresh"}
				</button>
				<button
					type="button"
					class="action-button"
					data-active={jobProgress.isPolling}
					onclick={togglePolling}
					data-role="auto-refresh"
				>
					Auto-refresh {jobProgress.isPolling ? "on" : "off"}
				</button>
				<button
					type="button"
					class="action-button action-button--primary"
					onclick={handleTriggerJob}
					disabled={triggering || hasRunningJob || justStartedJobId !== null}
					title={runningJobTooltip}
					data-role="start-job"
				>
					{triggering ? "Starting…" : "Start job"}
				</button>
			</div>
		</section>

		{#if triggerSuccess}
			<p class="banner banner--success" data-role="trigger-success">
				<span class="glyph" aria-hidden="true">✓</span>
				{triggerSuccess}
			</p>
		{/if}

		{#if triggerError}
			<p class="banner banner--error" data-role="trigger-error">
				<span class="glyph" aria-hidden="true">✗</span>
				{triggerError}
			</p>
		{/if}

		{#if jobProgress.error}
			<p class="banner banner--error" data-role="load-error">
				<span class="glyph" aria-hidden="true">✗</span>
				Error loading job data: {jobProgress.error}
			</p>
		{/if}

		{#if jobProgress.data}
			<dl class="ledger" data-role="ledger">
				<LedgerFigure
					label="Success rate"
					value={successRate}
					subtitle="Last 24 hours"
				/>
				<LedgerFigure
					label="Avg duration"
					value={avgDuration}
					subtitle="Per job"
				/>
				<LedgerFigure
					label="Jobs today"
					value={jobProgress.data.stats.total_jobs_24h}
					subtitle={`${jobProgress.data.stats.running_jobs} running`}
				/>
				<LedgerFigure
					label="Failed jobs"
					value={jobProgress.data.stats.failed_jobs_24h}
					subtitle="Last 24 hours"
				/>
			</dl>

			{#if jobProgress.data.active_job}
				<section class="section" data-role="active-section">
					<ActiveJobCard job={jobProgress.data.active_job} />
				</section>
			{:else}
				<section class="empty-active" data-role="active-empty">
					<p>No active job.</p>
				</section>
			{/if}

			<section class="section" data-role="recent-section">
				<header class="section-head">
					<h2 class="section-title">Recent jobs</h2>
					<span class="section-meta">Last {jobProgress.currentWindow}</span>
				</header>
				<JobHistoryTable
					jobs={jobProgress.data.recent_jobs}
					stats={jobProgress.data.stats}
				/>
			</section>
		{:else if !jobProgress.loading}
			<p class="empty-state">No job data available.</p>
		{/if}
	</div>
{:else}
	<div class="mobile-shell" data-role="job-status-page">
		{#if jobProgress.loading && !jobProgress.data}
			<div class="mobile-skeleton" data-testid="job-status-skeleton">
				<div class="skel skel--head"></div>
				<div class="skel-row">
					{#each Array(4) as _}
						<div class="skel skel--stat"></div>
					{/each}
				</div>
				<div class="skel-list">
					{#each Array(3) as _}
						<div class="skel skel--card"></div>
					{/each}
				</div>
			</div>
		{:else if jobProgress.error}
			<div class="mobile-error">
				<p class="mobile-error-title">Error loading job data</p>
				<p class="mobile-error-detail">{jobProgress.error}</p>
				<button
					type="button"
					class="mobile-retry"
					onclick={() => jobProgress.refresh()}
				>
					Retry
				</button>
			</div>
		{:else if jobProgress.data}
			<MobileJobStatusHeader
				currentWindow={jobProgress.currentWindow}
				onWindowChange={handleWindowChange}
			/>

			<MobileStatsRow
				successRate={successRate}
				avgDuration={avgDuration}
				totalJobs={jobProgress.data.stats.total_jobs_24h}
				runningJobs={jobProgress.data.stats.running_jobs}
				failedJobs={jobProgress.data.stats.failed_jobs_24h}
			/>

			{#if triggerSuccess}
				<p class="banner banner--success mobile-banner">
					<span class="glyph" aria-hidden="true">✓</span>
					{triggerSuccess}
				</p>
			{/if}

			{#if triggerError}
				<p class="banner banner--error mobile-banner">
					<span class="glyph" aria-hidden="true">✗</span>
					{triggerError}
				</p>
			{/if}

			<MobileActiveJobPanel job={jobProgress.data.active_job} />

			<MobileJobHistoryList
				jobs={jobProgress.data.recent_jobs}
				onJobSelect={handleJobSelect}
			/>

			<MobileJobDetailSheet
				job={selectedJob}
				open={detailSheetOpen}
				onClose={handleCloseDetailSheet}
			/>
		{:else}
			<p class="empty-state mobile-empty">No job data available.</p>
		{/if}

		<MobileControlBar
			onRefresh={handleRefresh}
			onTriggerJob={handleTriggerJob}
			loading={jobProgress.loading}
			triggering={triggering}
			hasRunningJob={hasRunningJob}
			justStartedJobId={justStartedJobId}
		/>
	</div>
{/if}

<style>
	.page {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
		max-width: 1080px;
		padding: 1.5rem 1.25rem 3rem;
	}

	.controls {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		padding: 0.75rem 0;
		border-bottom: 1px solid var(--surface-border);
	}

	.window-group {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}

	.control-label {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.window-pill {
		all: unset;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		padding: 0.4rem 0.75rem;
		border: 1px solid var(--surface-border);
		color: var(--alt-charcoal);
		background: transparent;
		cursor: pointer;
		min-height: 32px;
		display: inline-flex;
		align-items: center;
	}

	.window-pill[aria-pressed="true"] {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
		border-color: var(--alt-charcoal);
	}

	.window-pill:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: 2px;
	}

	.action-group {
		display: flex;
		gap: 0.5rem;
	}

	.action-button {
		all: unset;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		padding: 0.5rem 0.85rem;
		border: 1.5px solid var(--alt-charcoal);
		color: var(--alt-charcoal);
		background: transparent;
		cursor: pointer;
		min-height: 36px;
		display: inline-flex;
		align-items: center;
		transition:
			background 0.15s ease,
			color 0.15s ease;
	}

	.action-button:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-button:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: 2px;
	}

	.action-button[data-active="true"] {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.action-button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}

	.action-button--primary {
		border-width: 2px;
	}

	.banner {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
		padding: 0.65rem 0.85rem;
		font-family: var(--font-body);
		font-size: 0.85rem;
		border: 1px solid var(--surface-border);
		margin: 0;
	}

	.banner .glyph {
		font-family: var(--font-mono);
	}

	.banner--success {
		color: var(--alt-success);
		border-color: color-mix(in srgb, var(--alt-success) 35%, transparent);
		background: color-mix(in srgb, var(--alt-success) 6%, transparent);
	}

	.banner--error {
		color: var(--alt-error);
		border-color: color-mix(in srgb, var(--alt-error) 35%, transparent);
		background: color-mix(in srgb, var(--alt-error) 6%, transparent);
	}

	.ledger {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
		gap: 0;
		margin: 0;
		border-top: 1px solid var(--alt-charcoal);
		border-bottom: 1px solid var(--alt-charcoal);
	}

	.ledger > :global(*) {
		padding: 0.85rem 1rem;
		border-right: 1px solid var(--surface-border);
	}

	.ledger > :global(*:last-child) {
		border-right: none;
	}

	.section {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.section-head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 1rem;
		padding-bottom: 0.4rem;
		border-bottom: 1px solid var(--surface-border);
	}

	.section-title {
		font-family: var(--font-display);
		font-size: 1.25rem;
		font-weight: 700;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.section-meta {
		font-family: var(--font-body);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.empty-active {
		padding: 1.25rem 1rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.empty-active p {
		font-family: var(--font-body);
		font-size: 0.95rem;
		font-style: italic;
		color: var(--alt-slate);
		margin: 0;
		text-align: center;
	}

	.empty-state {
		font-family: var(--font-body);
		font-size: 0.95rem;
		font-style: italic;
		color: var(--alt-slate);
		text-align: center;
		padding: 3rem 1rem;
	}

	/* Mobile shell */
	.mobile-shell {
		min-height: 100dvh;
		padding-bottom: calc(5rem + env(safe-area-inset-bottom, 0px));
		background: var(--surface-bg);
		position: relative;
	}

	.mobile-skeleton {
		padding: 1rem;
		display: flex;
		flex-direction: column;
		gap: 1rem;
	}

	.skel {
		background: var(--surface-2);
		animation: skel-pulse 1.4s ease-in-out infinite;
	}

	.skel--head {
		height: 2rem;
		width: 60%;
	}

	.skel-row {
		display: flex;
		gap: 0.75rem;
		overflow-x: auto;
		padding-bottom: 0.5rem;
	}

	.skel--stat {
		min-width: 120px;
		height: 5rem;
	}

	.skel-list {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.skel--card {
		height: 6rem;
	}

	@keyframes skel-pulse {
		0%,
		100% {
			opacity: 0.55;
		}
		50% {
			opacity: 1;
		}
	}

	.mobile-error {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 0.75rem;
		padding: 2rem 1.5rem;
		text-align: center;
		border: 1px solid color-mix(in srgb, var(--alt-error) 30%, transparent);
		background: color-mix(in srgb, var(--alt-error) 5%, transparent);
		margin: 1rem;
	}

	.mobile-error-title {
		font-family: var(--font-display);
		font-size: 1.1rem;
		font-weight: 700;
		color: var(--alt-error);
		margin: 0;
	}

	.mobile-error-detail {
		font-family: var(--font-body);
		font-size: 0.85rem;
		color: var(--alt-slate);
		margin: 0;
	}

	.mobile-retry {
		all: unset;
		padding: 0.6rem 1.25rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		border: 1.5px solid var(--alt-charcoal);
		color: var(--alt-charcoal);
		cursor: pointer;
		min-height: 44px;
		display: inline-flex;
		align-items: center;
	}

	.mobile-retry:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.mobile-banner {
		margin: 0 1rem 0.75rem;
	}

	.mobile-empty {
		padding: 4rem 1.5rem;
	}

	@media (prefers-reduced-motion: reduce) {
		.skel {
			animation: none;
		}
	}
</style>
