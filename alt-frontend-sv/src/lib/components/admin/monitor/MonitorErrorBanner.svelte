<script lang="ts">
import type { MetricResult } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import type { StreamState } from "$lib/hooks/useConnectAdminMetrics.svelte";

let {
	metrics,
	streamState,
}: {
	metrics: MetricResult[];
	streamState: StreamState;
} = $props();

const anyDegraded = $derived(metrics.some((m) => m.degraded));

const reasons = $derived([
	...new Set(
		metrics.filter((m) => m.degraded && m.reason).map((m) => m.reason),
	),
]);

const visible = $derived(
	anyDegraded || streamState === "degraded" || streamState === "connecting",
);

const headline = $derived.by(() => {
	if (streamState === "connecting") return "Connecting to observability stream";
	if (streamState === "degraded") return "Stream degraded, reconnecting";
	if (anyDegraded) return "One or more metric sources are degraded";
	return "";
});
</script>

{#if visible}
	<aside
		class="banner"
		role="status"
		aria-live="polite"
		data-testid="monitor-error-banner"
	>
		<span class="rule" aria-hidden="true"></span>
		<div class="body">
			<div class="head">
				<span class="glyph" aria-hidden="true">●</span>
				<span class="state-text">degraded</span>
				<span class="msg">{headline}</span>
			</div>
			{#if reasons.length > 0}
				<ul class="reasons">
					{#each reasons as r (r)}
						<li>{r}</li>
					{/each}
				</ul>
			{/if}
		</div>
	</aside>
{/if}

<style>
	.banner {
		display: grid;
		grid-template-columns: 3px 1fr;
		gap: 0.85rem;
		padding: 0.7rem 0.95rem;
		border-top: 0.5px solid var(--obs-rule, var(--surface-border));
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
		background: color-mix(in oklch, var(--surface) 92%, var(--alt-warning) 8%);
	}

	.rule {
		background: var(--obs-warn, var(--alt-warning));
		border-radius: 1px;
	}

	.body {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		min-width: 0;
	}

	.head {
		display: flex;
		align-items: baseline;
		gap: 0.45rem;
		font-size: 0.82rem;
		color: var(--alt-charcoal);
	}

	.glyph,
	.state-text {
		color: var(--obs-warn, var(--alt-warning));
	}

	.state-text {
		font-family: var(--font-body);
		font-size: 0.7rem;
		letter-spacing: 0.16em;
		text-transform: uppercase;
		font-weight: 600;
	}

	.msg {
		font-family: var(--font-body);
	}

	.reasons {
		list-style: disc;
		margin: 0;
		padding-left: 1.2rem;
		color: var(--obs-muted, var(--alt-ash));
		font-size: 0.74rem;
	}
</style>
