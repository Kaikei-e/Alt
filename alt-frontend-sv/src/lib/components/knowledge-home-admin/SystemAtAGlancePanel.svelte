<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	overallHealth,
	lagSeconds,
	healthyCount,
	totalServiceCount,
	activeAlertCount,
}: {
	overallHealth: string | null;
	lagSeconds: number | null;
	healthyCount: number;
	totalServiceCount: number;
	activeAlertCount: number;
} = $props();

const healthStatus = $derived.by((): "ok" | "warning" | "error" | "neutral" => {
	if (!overallHealth) return "neutral";
	switch (overallHealth) {
		case "healthy":
			return "ok";
		case "at_risk":
			return "warning";
		case "breaching":
			return "error";
		default:
			return "neutral";
	}
});

const lagStatus = $derived.by((): "ok" | "warning" | "error" | "neutral" => {
	if (lagSeconds === null) return "neutral";
	if (lagSeconds < 60) return "ok";
	if (lagSeconds < 300) return "warning";
	return "error";
});

const servicesStatus = $derived.by(
	(): "ok" | "warning" | "error" | "neutral" => {
		if (totalServiceCount === 0) return "neutral";
		if (healthyCount === totalServiceCount) return "ok";
		if (healthyCount === 0) return "error";
		return "warning";
	},
);

const alertsStatus = $derived.by((): "ok" | "warning" | "error" | "neutral" => {
	if (activeAlertCount === 0) return "ok";
	if (activeAlertCount <= 2) return "warning";
	return "error";
});
</script>

<div class="grid grid-cols-2 gap-4 lg:grid-cols-4" data-role="system-glance">
	<AdminMetricCard
		label="SLO Health"
		value={overallHealth ?? "--"}
		status={healthStatus}
	/>
	<AdminMetricCard
		label="Projection Lag"
		value={lagSeconds !== null ? `${lagSeconds.toFixed(0)}s` : "--"}
		status={lagStatus}
	/>
	<AdminMetricCard
		label="Services"
		value="{healthyCount}/{totalServiceCount}"
		status={servicesStatus}
	/>
	<AdminMetricCard
		label="Active Alerts"
		value={activeAlertCount}
		status={alertsStatus}
	/>
</div>
