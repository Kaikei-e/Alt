<script lang="ts">
import { onDestroy, onMount } from "svelte";
import { browser } from "$app/environment";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import { Button } from "$lib/components/ui/button";
import ProjectionStatusPanel from "$lib/components/knowledge-home-admin/ProjectionStatusPanel.svelte";
import BackfillJobsTable from "$lib/components/knowledge-home-admin/BackfillJobsTable.svelte";
import FeatureFlagPanel from "$lib/components/knowledge-home-admin/FeatureFlagPanel.svelte";
import AdminTabNavigation from "$lib/components/knowledge-home-admin/AdminTabNavigation.svelte";
import SLOSummaryPanel from "$lib/components/knowledge-home-admin/SLOSummaryPanel.svelte";
import AlertStatusPanel from "$lib/components/knowledge-home-admin/AlertStatusPanel.svelte";
import ReprojectActions from "$lib/components/knowledge-home-admin/ReprojectActions.svelte";
import ReprojectRunsTable from "$lib/components/knowledge-home-admin/ReprojectRunsTable.svelte";
import DiffSummaryPanel from "$lib/components/knowledge-home-admin/DiffSummaryPanel.svelte";
import StorageStatsPanel from "$lib/components/knowledge-home-admin/StorageStatsPanel.svelte";
import SnapshotListPanel from "$lib/components/knowledge-home-admin/SnapshotListPanel.svelte";
import RetentionStatusPanel from "$lib/components/knowledge-home-admin/RetentionStatusPanel.svelte";
import RetentionRunResultPanel from "$lib/components/knowledge-home-admin/RetentionRunResultPanel.svelte";
import AuditActions from "$lib/components/knowledge-home-admin/AuditActions.svelte";
import AuditResultPanel from "$lib/components/knowledge-home-admin/AuditResultPanel.svelte";
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
		};
		error: string | null;
	};
}>();

let activeTab = $state("overview");

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

const runSovereignAction = async (action: { action: string; dry_run?: boolean }) => {
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

$effect(() => {
	admin.seed(data.adminData, data.error ? new Error(data.error) : null);
});

onMount(() => {
	if (browser) {
		admin.startPolling(10000);
		sovereign.startPolling(30000);
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

<PageHeader
	title="Knowledge Home Operations"
	description="Projection health, backfill status, and rollout configuration"
>
	{#snippet actions()}
		<div class="flex items-center gap-3 text-right">
			<Button
				variant="default"
				size="sm"
				disabled={admin.acting || !admin.health}
				onclick={() => void admin.triggerBackfill(admin.health?.activeVersion ?? 1)}
			>
				{admin.acting && admin.activeJobId === null ? "Triggering..." : "Trigger Backfill"}
			</Button>
			<p class="text-xs text-[var(--text-secondary)]">
				{admin.refreshing ? "Updating..." : "Up to date"}
			</p>
			<p class="text-xs text-[var(--text-secondary)]">
				Last updated: {admin.lastUpdatedLabel}
			</p>
		</div>
	{/snippet}
</PageHeader>

{#if admin.error}
	<div
		class="mb-4 rounded-lg border px-4 py-2 text-sm"
		style="background: var(--error-bg, #fee2e2); border-color: var(--error-border, #ef4444); color: var(--error-text, #991b1b);"
	>
		{admin.error.message}
	</div>
{/if}

<div class="mb-4">
	<AdminTabNavigation {activeTab} onTabChange={(tab: string) => (activeTab = tab)} />
</div>

{#if activeTab === "overview"}
	<div class="mt-4 grid gap-6 lg:grid-cols-2">
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
	<div class="mt-4 flex flex-col gap-6">
		<SLOSummaryPanel sloStatus={admin.sloStatus} />
		<AlertStatusPanel alerts={admin.sloStatus?.activeAlerts ?? []} />
	</div>
{:else if activeTab === "reproject"}
	<div class="mt-4 flex flex-col gap-6">
		<ReprojectActions
			onStart={(mode: string, fromVersion: string, toVersion: string, rangeStart?: string, rangeEnd?: string) =>
				void admin.startReproject(mode, fromVersion, toVersion, rangeStart, rangeEnd)}
			inFlight={admin.acting}
		/>
		<ReprojectRunsTable
			runs={admin.reprojectRuns}
			disableActions={admin.acting}
			onCompare={(run: ReprojectRunData) => admin.compareReproject(run.reprojectRunId)}
			onSwap={(run: ReprojectRunData) => admin.swapReproject(run.reprojectRunId)}
			onRollback={(run: ReprojectRunData) => admin.rollbackReproject(run.reprojectRunId)}
		/>
		<DiffSummaryPanel diff={admin.reprojectDiff} />
	</div>
{:else if activeTab === "storage"}
	<div class="mt-4 flex flex-col gap-6">
		<StorageStatsPanel stats={sovereign.storageStats} />
		<SnapshotListPanel
			snapshots={sovereign.snapshots}
			latestSnapshot={sovereign.latestSnapshot}
			disabled={sovereign.acting}
			onCreateSnapshot={() => sovereign.createSnapshot()}
		/>
		<RetentionStatusPanel
			retentionLogs={sovereign.retentionLogs}
			eligiblePartitions={sovereign.eligiblePartitions}
			disabled={sovereign.acting}
			onRunRetention={(dryRun) => sovereign.runRetention(dryRun)}
		/>
		<RetentionRunResultPanel result={sovereign.retentionResult} />
	</div>
{:else if activeTab === "audit"}
	<div class="mt-4 flex flex-col gap-6">
		<AuditActions
			onRunAudit={(name, version, size) => void admin.runAudit(name, version, size)}
			inFlight={admin.acting}
		/>
		<AuditResultPanel audit={admin.auditResult} />
	</div>
{/if}
