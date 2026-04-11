<script lang="ts">
import type { AlertSummaryData } from "$lib/connect/knowledge_home_admin";

let { alerts }: { alerts: AlertSummaryData[] } = $props();

const severityStatus = (severity: string): "error" | "warning" | "neutral" => {
	switch (severity) {
		case "critical":
		case "page":
			return "error";
		case "warning":
			return "warning";
		default:
			return "neutral";
	}
};

const severityColor = (severity: string) => {
	switch (severity) {
		case "critical":
		case "page":
			return "var(--alt-terracotta)";
		case "warning":
			return "var(--alt-sand)";
		default:
			return "var(--alt-ash)";
	}
};
</script>

<div class="panel" data-role="alert-status">
	<h3 class="section-heading">Active Alerts</h3>
	<div class="heading-rule"></div>

	{#if alerts.length === 0}
		<div class="no-alerts">
			<span class="no-alerts-dot"></span>
			<span class="no-alerts-text">No active alerts</span>
		</div>
	{:else}
		<div class="alert-list">
			{#each alerts as alert (alert.alertName + alert.firedAt)}
				<div class="alert-item" data-severity={severityStatus(alert.severity)}>
					<div class="alert-stripe" style="background: {severityColor(alert.severity)};"></div>
					<div class="alert-body">
						<div class="alert-header">
							<span class="alert-name">{alert.alertName}</span>
							<span class="alert-severity">{alert.severity}</span>
						</div>
						<p class="alert-desc">{alert.description}</p>
						<p class="alert-meta">
							Fired: {alert.firedAt
								? new Date(alert.firedAt).toLocaleString()
								: "--"}
							| Status: {alert.status}
						</p>
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

	.no-alerts {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.5rem 0;
	}

	.no-alerts-dot {
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--alt-sage);
	}

	.no-alerts-text {
		font-family: var(--font-display);
		font-size: 0.8rem;
		font-style: italic;
		color: var(--alt-ash);
	}

	.alert-list {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.alert-item {
		display: flex;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.alert-stripe {
		width: 3px;
		flex-shrink: 0;
	}

	.alert-body {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.6rem 0.75rem;
		flex: 1;
	}

	.alert-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.alert-name {
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		color: var(--alt-charcoal);
	}

	.alert-severity {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.alert-desc {
		font-family: var(--font-body);
		font-size: 0.75rem;
		color: var(--alt-slate);
		margin: 0;
	}

	.alert-meta {
		font-family: var(--font-mono);
		font-size: 0.6rem;
		color: var(--alt-ash);
		margin: 0;
	}
</style>
