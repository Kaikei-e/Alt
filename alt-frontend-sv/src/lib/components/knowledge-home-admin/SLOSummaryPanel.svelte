<script lang="ts">
import type { SLOStatusData } from "$lib/connect/knowledge_home_admin";
import AdminMetricCard from "./AdminMetricCard.svelte";
import ErrorBudgetGauge from "./ErrorBudgetGauge.svelte";

let { sloStatus }: { sloStatus: SLOStatusData | null } = $props();

const overallHealthColor = (health: string) => {
	switch (health) {
		case "healthy":
			return "ok" as const;
		case "at_risk":
			return "warning" as const;
		case "breaching":
			return "error" as const;
		default:
			return "neutral" as const;
	}
};

const sliStatusLabel = (status: string): "ok" | "warning" | "error" | "neutral" => {
	switch (status) {
		case "meeting":
			return "ok";
		case "burning":
			return "warning";
		case "breached":
			return "error";
		default:
			return "neutral";
	}
};

const formatSliName = (name: string) =>
	name
		.split("_")
		.map((w) => w.charAt(0).toUpperCase() + w.slice(1))
		.join(" ");
</script>

<div class="panel" data-role="slo-summary">
	<div class="panel-header">
		<h3 class="section-heading">SLO Status</h3>
		{#if sloStatus}
			<span class="header-meta">
				Window: {sloStatus.errorBudgetWindowDays}d | Computed: {sloStatus.computedAt
					? new Date(sloStatus.computedAt).toLocaleTimeString()
					: "--"}
			</span>
		{/if}
	</div>
	<div class="heading-rule"></div>

	{#if !sloStatus}
		<div class="loading-state">
			<span class="loading-pulse"></span>
			<span class="loading-text">Loading SLO data</span>
		</div>
	{:else}
		<AdminMetricCard
			label="Overall Health"
			value={sloStatus.overallHealth}
			status={overallHealthColor(sloStatus.overallHealth)}
		/>

		<div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
			{#each sloStatus.slis as sli (sli.name)}
				<div class="sli-card" data-status={sliStatusLabel(sli.status)}>
					<div class="sli-stripe"></div>
					<div class="sli-body">
						<div class="sli-header">
							<span class="sli-name">{formatSliName(sli.name)}</span>
							<span class="sli-status">{sli.status}</span>
						</div>
						<div class="sli-value-row">
							<span class="sli-current">{sli.currentValue.toFixed(2)}</span>
							<span class="sli-target">/ {sli.targetValue.toFixed(2)} {sli.unit}</span>
						</div>
						<ErrorBudgetGauge
							consumedPct={sli.errorBudgetConsumedPct}
							label="Error Budget"
						/>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<style>
	.panel {
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}

	.panel-header {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
	}

	.section-heading {
		font-family: var(--font-display);
		font-size: 1.05rem;
		font-weight: 700;
		line-height: 1.3;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.header-meta {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
	}

	.heading-rule {
		height: 1px;
		background: var(--surface-border);
		margin-bottom: 0.25rem;
	}

	.sli-card {
		display: flex;
		flex-direction: column;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.sli-stripe {
		height: 3px;
		flex-shrink: 0;
	}

	[data-status="ok"] .sli-stripe { background: var(--alt-sage); }
	[data-status="warning"] .sli-stripe { background: var(--alt-sand); }
	[data-status="error"] .sli-stripe { background: var(--alt-terracotta); }
	[data-status="neutral"] .sli-stripe { background: var(--surface-border); }

	.sli-body {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		padding: 0.75rem;
	}

	.sli-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.sli-name {
		font-family: var(--font-body);
		font-size: 0.7rem;
		font-weight: 600;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--alt-charcoal);
	}

	.sli-status {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.sli-value-row {
		display: flex;
		align-items: baseline;
		gap: 0.25rem;
	}

	.sli-current {
		font-family: var(--font-mono);
		font-size: 1.15rem;
		font-weight: 700;
		color: var(--alt-charcoal);
	}

	.sli-target {
		font-family: var(--font-mono);
		font-size: 0.65rem;
		color: var(--alt-ash);
	}

	.loading-state {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.loading-pulse {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-ash);
		animation: pulse 1.2s ease-in-out infinite;
	}

	.loading-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	@keyframes pulse {
		0%, 100% { opacity: 0.3; }
		50% { opacity: 1; }
	}
</style>
