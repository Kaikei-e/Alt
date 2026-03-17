<script lang="ts">
import { onMount, onDestroy } from "svelte";
import { browser } from "$app/environment";
import { useViewport } from "$lib/stores/viewport.svelte";
import { useKnowledgeHomeAdmin } from "$lib/hooks/useKnowledgeHomeAdmin.svelte";

import PageHeader from "$lib/components/desktop/layout/PageHeader.svelte";
import ProjectionStatusPanel from "$lib/components/knowledge-home-admin/ProjectionStatusPanel.svelte";
import BackfillJobsTable from "$lib/components/knowledge-home-admin/BackfillJobsTable.svelte";
import FeatureFlagPanel from "$lib/components/knowledge-home-admin/FeatureFlagPanel.svelte";
import ReasonDistributionChart from "$lib/components/knowledge-home-admin/ReasonDistributionChart.svelte";
import InteractionFunnelPanel from "$lib/components/knowledge-home-admin/InteractionFunnelPanel.svelte";

const { isDesktop } = useViewport();
const admin = useKnowledgeHomeAdmin();

// Placeholder data for reason distribution and funnel
// These would come from a dedicated metrics endpoint in a future iteration
const reasonDistribution = $state([
	{ code: "new_unread", count: 142 },
	{ code: "summary_completed", count: 98 },
	{ code: "tag_hotspot", count: 45 },
	{ code: "in_weekly_recap", count: 23 },
	{ code: "pulse_need_to_know", count: 12 },
]);

const interactionFunnel = $state([
	{ label: "Generated", value: 320 },
	{ label: "Seen", value: 210 },
	{ label: "Opened", value: 78 },
	{ label: "Dismissed", value: 15 },
]);

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
	<title>KH Admin - Alt</title>
</svelte:head>

<PageHeader
	title="Knowledge Home Admin"
	description="Projection operations and health monitoring"
/>

{#if admin.error}
	<div
		class="rounded-lg border px-4 py-2 text-sm mb-4"
		style="background: var(--error-bg, #fee2e2); border-color: var(--error-border, #ef4444); color: var(--error-text, #991b1b);"
	>
		Error loading admin data: {admin.error.message}
	</div>
{/if}

<div class="flex flex-col gap-6 mt-4" class:lg:grid={isDesktop} class:lg:grid-cols-2={isDesktop}>
	<ProjectionStatusPanel health={admin.health} />
	<FeatureFlagPanel flags={admin.flags} />
	<BackfillJobsTable jobs={admin.health?.backfillJobs ?? []} />
	<ReasonDistributionChart distribution={reasonDistribution} />
	<InteractionFunnelPanel funnel={interactionFunnel} />
</div>
