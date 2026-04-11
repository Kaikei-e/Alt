<script lang="ts">
let {
	consumedPct,
	label,
}: {
	consumedPct: number;
	label: string;
} = $props();

const clampedPct = $derived(Math.max(0, Math.min(100, consumedPct)));

const barColor = $derived(
	clampedPct > 80
		? "var(--alt-terracotta)"
		: clampedPct > 50
			? "var(--alt-sand)"
			: "var(--alt-sage)",
);
</script>

<div class="gauge" data-role="error-budget-gauge">
	<div class="gauge-header">
		<span class="gauge-label">{label}</span>
		<span class="gauge-value" style="color: {barColor};">
			{clampedPct.toFixed(1)}%
		</span>
	</div>
	<div class="gauge-track">
		<div
			class="gauge-fill"
			style="width: {clampedPct}%; background: {barColor};"
		></div>
	</div>
</div>

<style>
	.gauge {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.gauge-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}

	.gauge-label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.gauge-value {
		font-family: var(--font-mono);
		font-size: 0.7rem;
		font-weight: 700;
	}

	.gauge-track {
		height: 2px;
		width: 100%;
		overflow: hidden;
		background: var(--surface-border);
	}

	.gauge-fill {
		height: 100%;
		transition: width 0.3s ease;
	}
</style>
