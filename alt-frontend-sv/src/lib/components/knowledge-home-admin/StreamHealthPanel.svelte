<script lang="ts">
import AdminMetricCard from "./AdminMetricCard.svelte";

let {
	stream,
}: {
	stream: {
		connectionsTotal: number;
		disconnectsTotal: number;
		reconnectsTotal: number;
		deliveriesTotal: number;
		disconnectRatePct: number;
	} | null;
} = $props();

const clampedPct = $derived(
	stream ? Math.max(0, Math.min(100, stream.disconnectRatePct)) : 0,
);

const barColor = $derived(
	clampedPct >= 20
		? "var(--accent-red, #ef4444)"
		: clampedPct >= 5
			? "var(--accent-amber, #f59e0b)"
			: "var(--accent-green, #22c55e)",
);
</script>

<div class="flex flex-col gap-4">
	<h3 class="text-sm font-semibold" style="color: var(--text-primary);">
		Stream Health
	</h3>

	{#if !stream}
		<p class="text-xs" style="color: var(--text-secondary);">Loading stream data...</p>
	{:else}
		<div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
			<AdminMetricCard
				label="Connections"
				value={stream.connectionsTotal.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Disconnects"
				value={stream.disconnectsTotal.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Reconnects"
				value={stream.reconnectsTotal.toLocaleString()}
				status="neutral"
			/>
			<AdminMetricCard
				label="Deliveries"
				value={stream.deliveriesTotal.toLocaleString()}
				status="neutral"
			/>
		</div>

		<div
			class="flex flex-col gap-1.5 rounded-lg border-2 px-4 py-3"
			style="background: var(--surface-bg); border-color: var(--border-primary);"
		>
			<div class="flex items-center justify-between text-xs">
				<span style="color: var(--text-secondary);">Disconnect Rate</span>
				<span class="font-mono font-bold" style="color: {barColor};">
					{clampedPct.toFixed(1)}%
				</span>
			</div>
			<div
				class="h-2 w-full overflow-hidden rounded-full"
				style="background: var(--surface-border, #e5e7eb);"
			>
				<div
					class="h-full rounded-full transition-all"
					style="width: {clampedPct}%; background: {barColor};"
				></div>
			</div>
		</div>
	{/if}
</div>
