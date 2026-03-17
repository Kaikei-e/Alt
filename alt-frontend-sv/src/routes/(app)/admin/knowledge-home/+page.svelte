<script lang="ts">
import { onDestroy, onMount } from "svelte";
import { browser } from "$app/environment";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import { Button } from "$lib/components/ui/button";
import ProjectionStatusPanel from "$lib/components/knowledge-home-admin/ProjectionStatusPanel.svelte";
import BackfillJobsTable from "$lib/components/knowledge-home-admin/BackfillJobsTable.svelte";
import FeatureFlagPanel from "$lib/components/knowledge-home-admin/FeatureFlagPanel.svelte";
import {
	useKnowledgeHomeAdmin,
	type KnowledgeHomeAdminActionRequest,
} from "$lib/hooks/useKnowledgeHomeAdmin.svelte";
import type { BackfillJobData } from "$lib/connect/knowledge_home_admin";

let { data } = $props<{
	data: {
		adminData: {
			health: import("$lib/connect/knowledge_home_admin").ProjectionHealthData | null;
			flags: import("$lib/connect/knowledge_home_admin").FeatureFlagsConfigData | null;
		};
		error: string | null;
	};
}>();

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

$effect(() => {
	admin.seed(data.adminData, data.error ? new Error(data.error) : null);
});

onMount(() => {
	if (browser) {
		admin.startPolling(10000);
	}
});

onDestroy(() => {
	admin.stopPolling();
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
