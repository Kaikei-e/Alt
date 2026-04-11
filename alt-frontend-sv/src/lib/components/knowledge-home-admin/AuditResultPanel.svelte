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

const mismatchStatus = $derived.by(
	(): "ok" | "warning" | "error" | "neutral" => {
		if (!audit) return "neutral";
		const rate = audit.mismatchCount / audit.sampleSize;
		if (rate === 0) return "ok";
		if (rate <= 0.05) return "warning";
		return "error";
	},
);

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
	<div class="panel" data-role="audit-result" data-testid="audit-result-panel">
		<h3 class="section-heading">Audit Result</h3>
		<div class="heading-rule"></div>

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

		<div class="audit-meta">
			<span>Audit ID: <code class="audit-id">{audit.auditId}</code></span>
			<span>Checked: {new Date(audit.checkedAt).toLocaleString()}</span>
		</div>

		{#if parsedDetails}
			<div class="details-section">
				<h4 class="details-label">Details</h4>
				<pre class="details-pre">{JSON.stringify(parsedDetails, null, 2)}</pre>
			</div>
		{/if}
	</div>
{/if}

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

	.audit-meta {
		display: flex;
		gap: 1rem;
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
	}

	.audit-id {
		font-family: var(--font-mono);
	}

	.details-section {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.details-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.details-pre {
		max-height: 12rem;
		overflow: auto;
		padding: 0.75rem;
		border: 1px solid var(--surface-border);
		background: var(--surface-2);
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-charcoal);
		margin: 0;
	}
</style>
