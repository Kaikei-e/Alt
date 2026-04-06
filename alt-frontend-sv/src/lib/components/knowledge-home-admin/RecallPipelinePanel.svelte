<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	recall,
}: {
	recall: {
		signalsAppended: number;
		signalErrors: number;
		candidatesGenerated: number;
		candidatesEmpty: number;
		usersProcessed: number;
		projectorDurationMsP50: number;
		projectorDurationMsP95: number;
	} | null;
} = $props();

const signalErrorsStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!recall ? "neutral" : recall.signalErrors === 0 ? "ok" : "error",
);
</script>

<div class="flex flex-col gap-4">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Recall Pipeline
	</h3>

	{#if !recall}
		<p class="text-xs" style="color: var(--text-secondary);">Loading recall data...</p>
	{:else}
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-5">
			<AdminMetricCard
				label="Signals"
				value={recall.signalsAppended.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Signal Errors"
				value={recall.signalErrors}
				status={signalErrorsStatus}
			/>
			<AdminMetricCard
				label="Candidates Generated"
				value={recall.candidatesGenerated.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Candidates Empty"
				value={recall.candidatesEmpty.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Users Processed"
				value={recall.usersProcessed.toLocaleString()}
				status="neutral"
			/>
		</div>

		<div
			class="flex items-center gap-6 rounded-lg border-2 px-4 py-3"
			style="background: var(--surface-bg); border-color: var(--border-primary);"
		>
			<span class="text-xs font-medium" style="color: var(--text-secondary);">
				Duration
			</span>
			<div class="flex items-center gap-4">
				<span class="text-xs" style="color: var(--text-secondary);">
					P50: <span class="font-mono font-bold" style="color: var(--text-primary);">{recall.projectorDurationMsP50}ms</span>
				</span>
				<span class="text-xs" style="color: var(--text-secondary);">
					P95: <span class="font-mono font-bold" style="color: var(--text-primary);">{recall.projectorDurationMsP95}ms</span>
				</span>
			</div>
		</div>
	{/if}
</div>
