<script lang="ts">
import type { ProjectionHealthData } from "$lib/connect/knowledge_home_admin";
import AdminMetricCard from "./AdminMetricCard.svelte";

let { health }: { health: ProjectionHealthData | null } = $props();

const lastUpdatedFormatted = $derived(
	health?.lastUpdated ? new Date(health.lastUpdated).toLocaleTimeString() : "--",
);
</script>

<div class="panel" data-role="projection-status">
	<h3 class="section-heading">Projection Status</h3>
	<div class="heading-rule"></div>
	<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
		<AdminMetricCard
			label="Active Version"
			value={health?.activeVersion ?? "--"}
			status={health ? "ok" : "neutral"}
		/>
		<AdminMetricCard
			label="Checkpoint Seq"
			value={health?.checkpointSeq ?? "--"}
			status="neutral"
		/>
		<AdminMetricCard
			label="Last Updated"
			value={lastUpdatedFormatted}
			status="neutral"
		/>
		<AdminMetricCard
			label="Backfill Jobs"
			value={health?.backfillJobs.length ?? 0}
			status="neutral"
		/>
	</div>
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.heading-rule {
		height: 1px;
		background: var(--surface-border);
		margin-bottom: 0.25rem;
	}
</style>
