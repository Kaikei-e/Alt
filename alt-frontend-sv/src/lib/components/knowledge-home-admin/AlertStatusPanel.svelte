<script lang="ts">
import type { AlertSummaryData } from "$lib/connect/knowledge_home_admin";
import { AlertTriangle, Bell, ShieldAlert } from "@lucide/svelte";

let { alerts }: { alerts: AlertSummaryData[] } = $props();

const severityColor = (severity: string) => {
	switch (severity) {
		case "critical":
			return "var(--accent-red, #ef4444)";
		case "warning":
			return "var(--accent-amber, #f59e0b)";
		case "info":
			return "var(--accent-blue, #3b82f6)";
		default:
			return "var(--text-secondary)";
	}
};

const severityIcon = (severity: string) => {
	switch (severity) {
		case "critical":
			return ShieldAlert;
		case "warning":
			return AlertTriangle;
		default:
			return Bell;
	}
};
</script>

<div class="flex flex-col gap-3">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Active Alerts
	</h3>

	{#if alerts.length === 0}
		<div
			class="flex items-center gap-2 rounded-lg border-2 px-4 py-3 text-xs"
			style="background: var(--surface-bg); border-color: var(--border-primary); color: var(--text-secondary);"
		>
			<Bell size={14} />
			<span>No active alerts.</span>
		</div>
	{:else}
		<div class="flex flex-col gap-2">
			{#each alerts as alert (alert.alertName + alert.firedAt)}
				{@const Icon = severityIcon(alert.severity)}
				<div
					class="flex items-start gap-3 rounded-lg border-2 px-4 py-3"
					style="background: var(--surface-bg); border-color: var(--border-primary);"
				>
					<div class="mt-0.5 shrink-0" style="color: {severityColor(alert.severity)};">
						<Icon size={16} />
					</div>
					<div class="flex flex-1 flex-col gap-1">
						<div class="flex items-center justify-between">
							<span class="text-sm font-medium" style="color: var(--text-primary);">
								{alert.alertName}
							</span>
							<span
								class="inline-block rounded px-2 py-0.5 text-xs font-medium text-white"
								style="background: {severityColor(alert.severity)};"
							>
								{alert.severity}
							</span>
						</div>
						<p class="text-xs" style="color: var(--text-secondary);">
							{alert.description}
						</p>
						<p class="text-xs" style="color: var(--text-secondary);">
							Fired: {alert.firedAt
								? new Date(alert.firedAt).toLocaleString("ja-JP")
								: "--"}
							| Status: {alert.status}
						</p>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
