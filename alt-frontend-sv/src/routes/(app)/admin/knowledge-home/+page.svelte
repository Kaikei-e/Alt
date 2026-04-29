<script lang="ts">
import { onDestroy, onMount } from "svelte";
import { browser } from "$app/environment";
import ProjectionStatusPanel from "$lib/components/knowledge-home-admin/ProjectionStatusPanel.svelte";
import BackfillJobsTable from "$lib/components/knowledge-home-admin/BackfillJobsTable.svelte";
import FeatureFlagPanel from "$lib/components/knowledge-home-admin/FeatureFlagPanel.svelte";
import AdminTabNavigation from "$lib/components/knowledge-home-admin/AdminTabNavigation.svelte";
import SLOSummaryPanel from "$lib/components/knowledge-home-admin/SLOSummaryPanel.svelte";
import AlertStatusPanel from "$lib/components/knowledge-home-admin/AlertStatusPanel.svelte";
import ReprojectActions from "$lib/components/knowledge-home-admin/ReprojectActions.svelte";
import ReprojectRunsTable from "$lib/components/knowledge-home-admin/ReprojectRunsTable.svelte";
import KnowledgeLoopReprojectPanel from "$lib/components/knowledge-home-admin/KnowledgeLoopReprojectPanel.svelte";
import type {
	KnowledgeLoopReprojectResult,
	KnowledgeLoopReprojectStatus,
} from "$lib/server/sovereign-admin";
import DiffSummaryPanel from "$lib/components/knowledge-home-admin/DiffSummaryPanel.svelte";
import StorageStatsPanel from "$lib/components/knowledge-home-admin/StorageStatsPanel.svelte";
import SnapshotListPanel from "$lib/components/knowledge-home-admin/SnapshotListPanel.svelte";
import RetentionStatusPanel from "$lib/components/knowledge-home-admin/RetentionStatusPanel.svelte";
import RetentionRunResultPanel from "$lib/components/knowledge-home-admin/RetentionRunResultPanel.svelte";
import AuditActions from "$lib/components/knowledge-home-admin/AuditActions.svelte";
import AuditResultPanel from "$lib/components/knowledge-home-admin/AuditResultPanel.svelte";
import SystemAtAGlancePanel from "$lib/components/knowledge-home-admin/SystemAtAGlancePanel.svelte";
import ServiceHealthGrid from "$lib/components/knowledge-home-admin/ServiceHealthGrid.svelte";
import ProjectorPipelinePanel from "$lib/components/knowledge-home-admin/ProjectorPipelinePanel.svelte";
import StreamHealthPanel from "$lib/components/knowledge-home-admin/StreamHealthPanel.svelte";
import RecallPipelinePanel from "$lib/components/knowledge-home-admin/RecallPipelinePanel.svelte";
import SovereignMutationPanel from "$lib/components/knowledge-home-admin/SovereignMutationPanel.svelte";
import ErrorBudgetBurnRatePanel from "$lib/components/knowledge-home-admin/ErrorBudgetBurnRatePanel.svelte";
import InteractionFunnelPanel from "$lib/components/knowledge-home-admin/InteractionFunnelPanel.svelte";
import ReasonDistributionChart from "$lib/components/knowledge-home-admin/ReasonDistributionChart.svelte";
import ObservabilityPanel from "$lib/components/knowledge-home-admin/observability/ObservabilityPanel.svelte";
import {
	useKnowledgeHomeAdmin,
	type KnowledgeHomeAdminActionRequest,
} from "$lib/hooks/useKnowledgeHomeAdmin.svelte";
import { useSovereignAdmin } from "$lib/hooks/useSovereignAdmin.svelte";
import type {
	BackfillJobData,
	ReprojectRunData,
	SLOStatusData,
} from "$lib/connect/knowledge_home_admin";

let { data } = $props<{
	data: {
		adminData: {
			health:
				| import("$lib/connect/knowledge_home_admin").ProjectionHealthData
				| null;
			flags:
				| import("$lib/connect/knowledge_home_admin").FeatureFlagsConfigData
				| null;
			sloStatus:
				| import("$lib/connect/knowledge_home_admin").SLOStatusData
				| null;
			reprojectRuns: import("$lib/connect/knowledge_home_admin").ReprojectRunData[];
			systemMetrics:
				| import("$lib/connect/knowledge_home_admin").SystemMetricsData
				| null;
		};
		error: string | null;
	};
}>();

let activeTab = $state("overview");
let revealed = $state(false);

const fetchSnapshot = async () => {
	const response = await fetch("/api/admin/knowledge-home", {
		credentials: "include",
	});
	if (!response.ok) {
		const body = await response.json().catch(() => null);
		throw new Error(body?.error ?? "Failed to load admin data.");
	}
	return await response.json();
};

const runAdminAction = async (action: KnowledgeHomeAdminActionRequest) => {
	const response = await fetch("/api/admin/knowledge-home", {
		method: "POST",
		credentials: "include",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(action),
	});
	if (!response.ok) {
		const body = await response.json().catch(() => null);
		throw new Error(body?.error ?? "Failed to run admin action.");
	}
};

const admin = useKnowledgeHomeAdmin(fetchSnapshot, runAdminAction);

const fetchSovereignSnapshot = async () => {
	const response = await fetch("/api/admin/knowledge-home/sovereign", {
		credentials: "include",
	});
	if (!response.ok) {
		const body = await response.json().catch(() => null);
		throw new Error(body?.error ?? "Failed to load sovereign data.");
	}
	return await response.json();
};

const runSovereignAction = async (action: {
	action: string;
	dry_run?: boolean;
}) => {
	const response = await fetch("/api/admin/knowledge-home/sovereign", {
		method: "POST",
		credentials: "include",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify(action),
	});
	if (!response.ok) {
		const body = await response.json().catch(() => null);
		throw new Error(body?.error ?? "Failed to run sovereign action.");
	}
};

const sovereign = useSovereignAdmin(fetchSovereignSnapshot, runSovereignAction);

// Knowledge Loop full reproject. Distinct from the Knowledge Home shadow/swap
// reproject above: Loop is a disposable projection with TRUNCATE-and-rerun
// semantics, so it gets its own one-shot panel rather than the compare /
// swap / rollback flow.
async function triggerKnowledgeLoopReproject(): Promise<KnowledgeLoopReprojectResult> {
	const response = await fetch("/admin/knowledge-home/reproject-loop", {
		method: "POST",
		credentials: "include",
	});
	if (!response.ok) {
		const body = (await response.json().catch(() => null)) as {
			error?: string;
			message?: string;
		} | null;
		throw new Error(
			body?.error ?? body?.message ?? `Reproject failed (${response.status})`,
		);
	}
	return (await response.json()) as KnowledgeLoopReprojectResult;
}

async function fetchKnowledgeLoopReprojectStatus(): Promise<KnowledgeLoopReprojectStatus> {
	const response = await fetch("/admin/knowledge-home/reproject-loop", {
		method: "GET",
		credentials: "include",
	});
	if (!response.ok) {
		throw new Error(`Reproject status failed (${response.status})`);
	}
	return (await response.json()) as KnowledgeLoopReprojectStatus;
}

$effect(() => {
	admin.seed(data.adminData, data.error ? new Error(data.error) : null);
});

onMount(() => {
	if (browser) {
		admin.startPolling(10000);
		sovereign.startPolling(30000);
		requestAnimationFrame(() => {
			revealed = true;
		});
	}
});

onDestroy(() => {
	admin.stopPolling();
	sovereign.stopPolling();
});
</script>

<svelte:head>
	<title>Knowledge Home Operations - Alt</title>
</svelte:head>

<div class="ops-page" class:revealed data-role="operations-page">
	<!-- Page Header -->
	<header class="ops-header">
		<div class="ops-header-top">
			<div>
				<h1 class="ops-title">Knowledge Home Operations</h1>
				<p class="ops-subtitle">Projection health, backfill status, and rollout configuration</p>
			</div>
			<div class="ops-actions">
				<button
					class="ops-action-btn"
					disabled={admin.acting || !admin.health}
					onclick={() => void admin.triggerBackfill(admin.health?.activeVersion ?? 1)}
				>
					{admin.acting && admin.activeJobId === null ? "Triggering..." : "Trigger Backfill"}
				</button>
				<button
					class="ops-action-btn"
					disabled={admin.acting}
					data-testid="emit-article-url-backfill-button"
					title="Emit ArticleUrlBackfilled corrective events for articles whose Knowledge Home projection has an empty URL. Run a Full Reproject + Swap afterwards to land the URLs."
					onclick={() => void admin.emitArticleUrlBackfill(0, false)}
				>
					{admin.acting ? "Emitting..." : "Emit URL Backfill"}
				</button>
				{#if admin.urlBackfillResult}
					<span class="ops-status-text" data-testid="url-backfill-result-summary">
						URL backfill: {admin.urlBackfillResult.eventsAppended} appended /
						{admin.urlBackfillResult.articlesScanned} scanned
						{admin.urlBackfillResult.skippedBlockedScheme > 0
							? ` · ${admin.urlBackfillResult.skippedBlockedScheme} blocked-scheme`
							: ""}
						{admin.urlBackfillResult.moreRemaining ? " · more remaining" : ""}
					</span>
				{/if}
				<span class="ops-status-text">
					{admin.refreshing ? "Updating..." : "Up to date"}
				</span>
				<span class="ops-status-text">
					Last updated: {admin.lastUpdatedLabel}
				</span>
			</div>
		</div>
		<div class="header-rule"></div>
	</header>

	{#if admin.error}
		<div class="ops-error" data-role="error-banner">
			{admin.error.message}
		</div>
	{/if}

	<div class="ops-tab-section">
		<AdminTabNavigation {activeTab} onTabChange={(tab: string) => (activeTab = tab)} />
	</div>

	<div class="ops-content" style="--stagger: 0">
		{#if activeTab === "overview"}
			<div class="ops-section" style="--stagger: 1">
				<SystemAtAGlancePanel
					overallHealth={admin.sloStatus?.overallHealth ?? null}
					lagSeconds={admin.systemMetrics?.projector?.lagSeconds ?? null}
					healthyCount={admin.systemMetrics?.serviceHealth?.filter(s => s.status === "healthy").length ?? 0}
					totalServiceCount={admin.systemMetrics?.serviceHealth?.length ?? 0}
					activeAlertCount={admin.sloStatus?.activeAlerts?.length ?? 0}
				/>
			</div>
			<div class="grid gap-6 lg:grid-cols-2 ops-section" style="--stagger: 2">
				<ProjectionStatusPanel health={admin.health} />
				<FeatureFlagPanel flags={admin.flags} />
				<div class="lg:col-span-2">
					<BackfillJobsTable
						jobs={admin.health?.backfillJobs ?? []}
						disableActions={admin.acting}
						activeJobId={admin.activeJobId}
						onPause={(job: BackfillJobData) => admin.pauseBackfill(job.jobId)}
						onResume={(job: BackfillJobData) => admin.resumeBackfill(job.jobId)}
					/>
				</div>
			</div>
		{:else if activeTab === "slo"}
			<div class="ops-section" style="--stagger: 1">
				<SLOSummaryPanel sloStatus={admin.sloStatus} />
			</div>
			<div class="ops-section" style="--stagger: 2">
				<ErrorBudgetBurnRatePanel slis={admin.sloStatus?.slis ?? []} />
			</div>
			<div class="ops-section" style="--stagger: 3">
				<AlertStatusPanel alerts={admin.sloStatus?.activeAlerts ?? []} />
			</div>
			<div class="grid gap-6 lg:grid-cols-2 ops-section" style="--stagger: 4">
				<InteractionFunnelPanel funnel={admin.systemMetrics?.tracking
					? [
						{ label: "Exposed", value: admin.systemMetrics.tracking.itemsExposed },
						{ label: "Opened", value: admin.systemMetrics.tracking.itemsOpened },
						{ label: "Dismissed", value: admin.systemMetrics.tracking.itemsDismissed },
					]
					: []} />
				<ReasonDistributionChart distribution={(() => {
					try {
						if (!admin.auditResult?.detailsJson) return [];
						const details = JSON.parse(admin.auditResult.detailsJson);
						if (details.why_distribution && typeof details.why_distribution === "object") {
							return Object.entries(details.why_distribution).map(([code, count]) => ({
								code,
								count: count as number,
							}));
						}
						return [];
					} catch {
						return [];
					}
				})()} />
			</div>
		{:else if activeTab === "system"}
			<div class="ops-section" style="--stagger: 1">
				<ServiceHealthGrid services={admin.systemMetrics?.serviceHealth ?? []} />
			</div>
			<div class="ops-section" style="--stagger: 2">
				<ProjectorPipelinePanel projector={admin.systemMetrics?.projector ?? null} />
			</div>
			<div class="ops-section" style="--stagger: 3">
				<StreamHealthPanel stream={admin.systemMetrics?.stream ?? null} />
			</div>
			<div class="grid gap-6 lg:grid-cols-2 ops-section" style="--stagger: 4">
				<RecallPipelinePanel recall={admin.systemMetrics?.recall ?? null} />
				<SovereignMutationPanel sovereign={admin.systemMetrics?.sovereign ?? null} />
			</div>
		{:else if activeTab === "reproject"}
			<div class="ops-section" style="--stagger: 1">
				<ReprojectActions
					onStart={(mode: string, fromVersion: string, toVersion: string, rangeStart?: string, rangeEnd?: string) =>
						void admin.startReproject(mode, fromVersion, toVersion, rangeStart, rangeEnd)}
					inFlight={admin.acting}
				/>
			</div>
			<div class="ops-section" style="--stagger: 2">
				<ReprojectRunsTable
					runs={admin.reprojectRuns}
					disableActions={admin.acting}
					onCompare={(run: ReprojectRunData) => admin.compareReproject(run.reprojectRunId)}
					onSwap={(run: ReprojectRunData) => admin.swapReproject(run.reprojectRunId)}
					onRollback={(run: ReprojectRunData) => admin.rollbackReproject(run.reprojectRunId)}
				/>
			</div>
			<div class="ops-section" style="--stagger: 3">
				<DiffSummaryPanel diff={admin.reprojectDiff} />
			</div>
			<div class="ops-section" style="--stagger: 4">
				<KnowledgeLoopReprojectPanel
					onTrigger={triggerKnowledgeLoopReproject}
					onFetchStatus={fetchKnowledgeLoopReprojectStatus}
				/>
			</div>
		{:else if activeTab === "storage"}
			<div class="ops-section" style="--stagger: 1">
				<StorageStatsPanel stats={sovereign.storageStats} />
			</div>
			<div class="ops-section" style="--stagger: 2">
				<SnapshotListPanel
					snapshots={sovereign.snapshots}
					latestSnapshot={sovereign.latestSnapshot}
					disabled={sovereign.acting}
					onCreateSnapshot={() => sovereign.createSnapshot()}
				/>
			</div>
			<div class="ops-section" style="--stagger: 3">
				<RetentionStatusPanel
					retentionLogs={sovereign.retentionLogs}
					eligiblePartitions={sovereign.eligiblePartitions}
					disabled={sovereign.acting}
					onRunRetention={(dryRun) => sovereign.runRetention(dryRun)}
				/>
			</div>
			<div class="ops-section" style="--stagger: 4">
				<RetentionRunResultPanel result={sovereign.retentionResult} />
			</div>
		{:else if activeTab === "audit"}
			<div class="ops-section" style="--stagger: 1">
				<AuditActions
					onRunAudit={(name, version, size) => void admin.runAudit(name, version, size)}
					inFlight={admin.acting}
				/>
			</div>
			<div class="ops-section" style="--stagger: 2">
				<AuditResultPanel audit={admin.auditResult} />
			</div>
		{:else if activeTab === "observability"}
			<div class="ops-section" style="--stagger: 1">
				<ObservabilityPanel />
			</div>
		{/if}
	</div>
</div>

<style>
	.ops-page {
		max-width: 1400px;
		margin: 0 auto;
		padding: 1.5rem 2rem;
		opacity: 0;
		transform: translateY(6px);
		transition: opacity 0.4s ease, transform 0.4s ease;
	}

	.ops-page.revealed {
		opacity: 1;
		transform: translateY(0);
	}

	.ops-header {
		margin-bottom: 1.5rem;
	}

	.ops-header-top {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 1rem;
		margin-bottom: 0.75rem;
	}

	.ops-title {
		font-family: var(--font-display);
		font-size: 1.5rem;
		font-weight: 700;
		line-height: 1.2;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.ops-subtitle {
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-slate);
		margin: 0.25rem 0 0;
	}

	.ops-actions {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-shrink: 0;
	}

	.ops-action-btn {
		border: 1.5px solid var(--alt-charcoal);
		background: transparent;
		color: var(--alt-charcoal);
		font-family: var(--font-body);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		padding: 0.4rem 0.75rem;
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
	}

	.ops-action-btn:hover:not(:disabled) {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	.ops-action-btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}

	.ops-status-text {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
	}

	.header-rule {
		height: 1px;
		background: var(--surface-border);
	}

	.ops-error {
		border-left: 3px solid var(--alt-terracotta);
		background: var(--surface-bg);
		padding: 0.5rem 1rem;
		margin-bottom: 1rem;
		font-family: var(--font-body);
		font-size: 0.8rem;
		color: var(--alt-terracotta);
	}

	.ops-tab-section {
		margin-bottom: 1.5rem;
	}

	.ops-content {
		display: flex;
		flex-direction: column;
		gap: 1.5rem;
	}

	.ops-section {
		opacity: 0;
		animation: section-in 0.3s ease forwards;
		animation-delay: calc(var(--stagger, 0) * 60ms);
	}

	@keyframes section-in {
		to {
			opacity: 1;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.ops-page {
			opacity: 1;
			transform: none;
			transition: none;
		}

		.ops-section {
			opacity: 1;
			animation: none;
		}
	}
</style>
