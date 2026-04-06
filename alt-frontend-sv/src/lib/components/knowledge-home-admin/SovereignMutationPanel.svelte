<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	sovereign,
}: {
	sovereign: {
		mutationsApplied: number;
		mutationsErrors: number;
		mutationDurationMsP50: number;
		mutationDurationMsP95: number;
		errorRatePct: number;
	} | null;
} = $props();

const errorsStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!sovereign ? "neutral" : sovereign.mutationsErrors === 0 ? "ok" : "error",
);

const errorRateStatus = $derived<"ok" | "warning" | "error" | "neutral">(
	!sovereign
		? "neutral"
		: sovereign.errorRatePct === 0
			? "ok"
			: sovereign.errorRatePct < 5
				? "warning"
				: "error",
);
</script>

<div class="flex flex-col gap-4">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Sovereign Mutations
	</h3>

	{#if !sovereign}
		<p class="text-xs" style="color: var(--text-secondary);">Loading sovereign data...</p>
	{:else}
		<div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
			<AdminMetricCard
				label="Applied"
				value={sovereign.mutationsApplied.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Errors"
				value={sovereign.mutationsErrors}
				status={errorsStatus}
			/>
			<AdminMetricCard
				label="Error Rate"
				value="{sovereign.errorRatePct.toFixed(1)}%"
				status={errorRateStatus}
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
					P50: <span class="font-mono font-bold" style="color: var(--text-primary);">{sovereign.mutationDurationMsP50}ms</span>
				</span>
				<span class="text-xs" style="color: var(--text-secondary);">
					P95: <span class="font-mono font-bold" style="color: var(--text-primary);">{sovereign.mutationDurationMsP95}ms</span>
				</span>
			</div>
		</div>
	{/if}
</div>
