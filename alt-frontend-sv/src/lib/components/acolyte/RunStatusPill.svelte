<script lang="ts">
import {
	RUN_STATUS_LABELS,
	type RunStatusKind,
} from "./runStatusPill";

interface Props {
	status: RunStatusKind;
}

const { status }: Props = $props();
const label = $derived(RUN_STATUS_LABELS[status]);
const live = $derived(status === "failed" ? "assertive" : "polite");
</script>

<span
	class="pill kind-{status}"
	role="status"
	aria-live={live}
	aria-atomic="true"
>
	<span class="dot" aria-hidden="true"></span>
	<span class="label">{label}</span>
</span>

<style>
	.pill {
		display: inline-flex;
		align-items: center;
		gap: 0.4rem;
		padding: 0.2rem 0.55rem 0.22rem;
		font-family: var(--font-mono, "IBM Plex Mono", "JetBrains Mono", ui-monospace, monospace);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.12em;
		text-transform: uppercase;
		color: var(--alt-slate, #666);
		background: var(--surface-bg, #faf9f7);
		border: 1px solid var(--surface-border, #c8c8c8);
		white-space: nowrap;
	}

	.dot {
		flex: 0 0 auto;
		width: 6px;
		height: 6px;
		border-radius: 50%;
		background: currentColor;
	}

	.label {
		line-height: 1;
	}

	.pill.kind-idle,
	.pill.kind-cancelled {
		color: var(--alt-ash, #999);
	}

	.pill.kind-ready {
		color: var(--alt-charcoal, #1a1a1a);
	}

	.pill.kind-generating {
		color: var(--alt-charcoal, #1a1a1a);
	}

	.pill.kind-completed {
		color: var(--alt-success, #2f6b3a);
		border-color: color-mix(in srgb, var(--alt-success, #2f6b3a) 35%, var(--surface-border, #c8c8c8));
	}

	.pill.kind-failed {
		color: var(--alt-terracotta, #b85450);
		border-color: color-mix(in srgb, var(--alt-terracotta, #b85450) 35%, var(--surface-border, #c8c8c8));
	}

	.pill.kind-generating .dot {
		animation: run-status-pulse 1.2s ease-in-out infinite;
	}

	@keyframes run-status-pulse {
		0%, 100% { opacity: 1; transform: scale(1); }
		50% { opacity: 0.45; transform: scale(0.85); }
	}

	@media (prefers-reduced-motion: reduce) {
		.pill.kind-generating .dot { animation: none; }
	}
</style>
