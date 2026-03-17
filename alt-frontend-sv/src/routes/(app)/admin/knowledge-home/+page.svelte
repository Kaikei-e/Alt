<script lang="ts">
import { onDestroy, onMount } from "svelte";
import { browser } from "$app/environment";
import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import ProjectionStatusPanel from "$lib/components/knowledge-home-admin/ProjectionStatusPanel.svelte";
import BackfillJobsTable from "$lib/components/knowledge-home-admin/BackfillJobsTable.svelte";
import FeatureFlagPanel from "$lib/components/knowledge-home-admin/FeatureFlagPanel.svelte";
import { useKnowledgeHomeAdmin } from "$lib/hooks/useKnowledgeHomeAdmin.svelte";

let { data } = $props<{
	data: {
		adminData: {
			health: import("$lib/connect/knowledge_home_admin").ProjectionHealthData | null;
			flags: import("$lib/connect/knowledge_home_admin").FeatureFlagsConfigData | null;
		};
		error: string | null;
	};
}>();

const admin = useKnowledgeHomeAdmin(async () => {
	const response = await fetch("/api/admin/knowledge-home", {
		credentials: "include",
	});
	if (!response.ok) {
		const body = await response.json().catch(() => null);
		throw new Error(body?.error ?? "Failed to load admin data.");
	}
	return await response.json();
});

admin.seed(data.adminData, data.error ? new Error(data.error) : null);

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
		<div class="text-right">
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
		<BackfillJobsTable jobs={admin.health?.backfillJobs ?? []} />
	</div>
</div>
