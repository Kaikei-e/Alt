<script lang="ts">
import {
	statusToGlyph,
	statusToInk,
	statusToLabel,
	type StatusInput,
} from "$lib/utils/jobStatusInk";

interface Props {
	status: StatusInput;
	pulse?: boolean;
	includeLabel?: boolean;
}

let { status, pulse = false, includeLabel = false }: Props = $props();

const glyph = $derived(statusToGlyph(status));
const ink = $derived(statusToInk(status));
const label = $derived(statusToLabel(status));
const shouldPulse = $derived(pulse && status === "running");
</script>

<span class="status-glyph" data-status={status} data-ink={ink}>
	<span
		class="glyph"
		class:glyph--pulse={shouldPulse}
		aria-hidden="true"
	>{glyph}</span>
	{#if includeLabel}
		<span class="label">{label}</span>
	{:else}
		<span class="visually-hidden">{label}</span>
	{/if}
</span>

<style>
	.status-glyph {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
		font-family: var(--font-body);
		font-size: 0.75rem;
		line-height: 1;
	}

	.glyph {
		font-family: var(--font-mono);
		font-size: 0.85rem;
		line-height: 1;
	}

	.label {
		font-size: 0.65rem;
		font-weight: 600;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	[data-ink="success"] {
		color: var(--alt-success);
	}

	[data-ink="error"] {
		color: var(--alt-error);
	}

	[data-ink="warning"] {
		color: var(--alt-warning);
	}

	[data-ink="neutral"] {
		color: var(--alt-charcoal);
	}

	[data-ink="muted"] {
		color: var(--alt-ash);
	}

	.glyph--pulse {
		animation: status-pulse 1.2s ease-in-out infinite;
	}

	@keyframes status-pulse {
		0%,
		100% {
			opacity: 0.45;
		}
		50% {
			opacity: 1;
		}
	}

	.visually-hidden {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		margin: -1px;
		overflow: hidden;
		clip: rect(0, 0, 0, 0);
		white-space: nowrap;
		border: 0;
	}

	@media (prefers-reduced-motion: reduce) {
		.glyph--pulse {
			animation: none;
		}
	}
</style>
