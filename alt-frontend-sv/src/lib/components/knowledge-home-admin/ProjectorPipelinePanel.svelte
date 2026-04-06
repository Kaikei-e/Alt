<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	projector,
}: {
	projector: {
		eventsProcessed: number;
		lagSeconds: number;
		batchDurationMsP50: number;
		batchDurationMsP95: number;
		batchDurationMsP99: number;
		errors: number;
	} | null;
} = $props();

const lagStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!projector
		? "neutral"
		: projector.lagSeconds < 60
			? "ok"
			: projector.lagSeconds < 300
				? "warning"
				: "error",
);

const errorsStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!projector ? "neutral" : projector.errors === 0 ? "ok" : "error",
);
</script>

<div class="flex flex-col gap-4">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Projector Pipeline
	</h3>

	{#if !projector}
		<p class="text-xs" style="color: var(--text-secondary);">Loading projector data...</p>
	{:else}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
			<AdminMetricCard
				label="Events Processed"
				value={projector.eventsProcessed.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Lag"
				value="{projector.lagSeconds}s"
				status={lagStatus}
			/>
			<AdminMetricCard
				label="Errors"
				value={projector.errors}
				status={errorsStatus}
			/>
		</div>

		<div
			class="flex items-center gap-6 rounded-lg border-2 px-4 py-3"
			style="background: var(--surface-bg); border-color: var(--border-primary);"
		>
			<span class="text-xs font-medium" style="color: var(--text-secondary);">
				Batch Duration
			</span>
			<div class="flex items-center gap-4">
				<span class="text-xs" style="color: var(--text-secondary);">
					P50: <span class="font-mono font-bold" style="color: var(--text-primary);">{projector.batchDurationMsP50}ms</span>
				</span>
				<span class="text-xs" style="color: var(--text-secondary);">
					P95: <span class="font-mono font-bold" style="color: var(--text-primary);">{projector.batchDurationMsP95}ms</span>
				</span>
				<span class="text-xs" style="color: var(--text-secondary);">
					P99: <span class="font-mono font-bold" style="color: var(--text-primary);">{projector.batchDurationMsP99}ms</span>
				</span>
			</div>
		</div>
	{/if}
</div>
