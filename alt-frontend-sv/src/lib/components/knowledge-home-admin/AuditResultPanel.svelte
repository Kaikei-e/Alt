<script lang="ts">
import type { ProjectionAuditData } from "$lib/connect/knowledge_home_admin";
import AdminMetricCard from "./AdminMetricCard.svelte";

interface Props {
	audit: ProjectionAuditData | null;
}

let { audit }: Props = $props();

const mismatchRate = $derived(
	audit && audit.sampleSize > 0
		? ((audit.mismatchCount / audit.sampleSize) * 100).toFixed(1)
		: "0.0",
);

const mismatchStatus = $derived.by((): "ok" | "warning" | "error" | "neutral" => {
	if (!audit) return "neutral";
	const rate = audit.mismatchCount / audit.sampleSize;
	if (rate === 0) return "ok";
	if (rate <= 0.05) return "warning";
	return "error";
});

let parsedDetails = $derived.by(() => {
	if (!audit?.detailsJson) return null;
	try {
		return JSON.parse(audit.detailsJson);
	} catch {
		return null;
	}
});
</script>

{#if audit}
	<div class="flex flex-col gap-3" data-testid="audit-result-panel">
		<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
			Audit Result
		</h3>
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			<AdminMetricCard
				label="Projection"
				value="{audit.projectionName} v{audit.projectionVersion}"
				status="neutral"
			/>
			<AdminMetricCard
				label="Sample Size"
				value={audit.sampleSize}
				status="neutral"
			/>
			<AdminMetricCard
				label="Mismatches"
				value={audit.mismatchCount}
				status={mismatchStatus}
			/>
			<AdminMetricCard
				label="Mismatch Rate"
				value="{mismatchRate}%"
				status={mismatchStatus}
			/>
		</div>

		<div class="flex gap-4 text-xs" style="color: var(--text-secondary);">
			<span>Audit ID: <code class="font-mono">{audit.auditId}</code></span>
			<span>Checked: {new Date(audit.checkedAt).toLocaleString()}</span>
		</div>

		{#if parsedDetails}
			<div class="flex flex-col gap-1">
				<h4 class="text-xs font-medium" style="color: var(--text-secondary);">
					Details
				</h4>
				<pre
					class="max-h-48 overflow-auto rounded-lg border-2 p-3 text-xs font-mono"
					style="background: var(--surface-bg); border-color: var(--border-primary); color: var(--text-primary);"
				>{JSON.stringify(parsedDetails, null, 2)}</pre>
			</div>
		{/if}
	</div>
{/if}
