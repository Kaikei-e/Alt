<script lang="ts">
import type { Snippet } from "svelte";

let {
	label,
	value,
	status = "neutral",
	icon,
}: {
	label: string;
	value: string | number;
	status?: "ok" | "warning" | "error" | "neutral";
	icon?: Snippet;
} = $props();
</script>

<div class="metric-card" data-role="metric-card" data-status={status}>
	<div class="metric-stripe"></div>
	<div class="metric-body">
		<div class="metric-label">
			{#if icon}
				{@render icon()}
			{/if}
			<span>{label}</span>
		</div>
		<div class="metric-value">
			{value}
		</div>
	</div>
</div>

<style>
	.metric-card {
		display: flex;
		flex-direction: column;
		border: 1px solid var(--surface-border);
		background: var(--surface-bg);
	}

	.metric-stripe {
		height: 3px;
		flex-shrink: 0;
	}

	[data-status="ok"] .metric-stripe {
		background: var(--alt-sage);
	}
	[data-status="warning"] .metric-stripe {
		background: var(--alt-sand);
	}
	[data-status="error"] .metric-stripe {
		background: var(--alt-terracotta);
	}
	[data-status="neutral"] .metric-stripe {
		background: var(--surface-border);
	}

	.metric-body {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		padding: 0.75rem 1rem;
	}

	.metric-label {
		display: flex;
		align-items: center;
		gap: 0.35rem;
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
		color: var(--alt-ash);
	}

	.metric-value {
		font-family: var(--font-mono);
		font-size: 1.35rem;
		font-weight: 700;
		line-height: 1.2;
		color: var(--alt-charcoal);
	}
</style>
