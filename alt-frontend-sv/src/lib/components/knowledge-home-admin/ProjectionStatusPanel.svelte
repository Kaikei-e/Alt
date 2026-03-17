<script lang="ts">
import type { ProjectionHealthData } from "$lib/connect/knowledge_home_admin";
import AdminMetricCard from "./AdminMetricCard.svelte";

let { health }: { health: ProjectionHealthData | null } = $props();

const lastUpdatedFormatted = $derived(
	health?.lastUpdated
		? new Date(health.lastUpdated).toLocaleTimeString()
		: "—",
);
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Projection Status
	</h3>
	<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
		<AdminMetricCard
			label="Active Version"
			value={health?.activeVersion ?? "—"}
			status={health ? "ok" : "neutral"}
		/>
		<AdminMetricCard
			label="Checkpoint Seq"
			value={health?.checkpointSeq ?? "—"}
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
