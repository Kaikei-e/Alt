<script lang="ts">
import type { TimeWindow } from "$lib/schema/dashboard";

interface Props {
	currentWindow: TimeWindow;
	onWindowChange: (window: TimeWindow) => void;
}

let { currentWindow, onWindowChange }: Props = $props();

const timeWindows: { label: string; value: TimeWindow }[] = [
	{ label: "4h", value: "4h" },
	{ label: "24h", value: "24h" },
	{ label: "3d", value: "3d" },
	{ label: "7d", value: "7d" },
];

const windowLabel = $derived(currentWindow.toUpperCase());
</script>

<header class="mobile-header" data-role="page-kicker">
	<p class="kicker">JOB STATUS · {windowLabel} WINDOW</p>
	<h1 class="title">Job Status</h1>
	<div class="window-row" role="group" aria-label="Time window">
		{#each timeWindows as tw}
			<button
				type="button"
				class="window-pill"
				aria-pressed={currentWindow === tw.value}
				onclick={() => onWindowChange(tw.value)}
				data-testid="time-window-{tw.value}"
			>
				{tw.label}
			</button>
		{/each}
	</div>
	<div class="rule" aria-hidden="true"></div>
</header>

<style>
	.mobile-header {
		position: sticky;
		top: 0;
		z-index: 10;
		display: flex;
		flex-direction: column;
		gap: 0.55rem;
		padding: calc(0.85rem + env(safe-area-inset-top, 0px)) 1rem 0.65rem;
		background: var(--surface-bg);
	}

	.kicker {
		font-family: var(--font-body);
		font-size: 0.6rem;
		font-weight: 600;
		letter-spacing: 0.1em;
		text-transform: uppercase;
		color: var(--alt-ash);
		margin: 0;
	}

	.title {
		font-family: var(--font-display);
		font-size: 1.55rem;
		font-weight: 700;
		line-height: 1.1;
		color: var(--alt-charcoal);
		margin: 0;
	}

	.window-row {
		display: flex;
		gap: 0.4rem;
		overflow-x: auto;
		scrollbar-width: none;
		-ms-overflow-style: none;
		padding: 0.25rem 0;
	}

	.window-row::-webkit-scrollbar {
		display: none;
	}

	.window-pill {
		all: unset;
		flex-shrink: 0;
		font-family: var(--font-body);
		font-size: 0.8rem;
		font-weight: 600;
		letter-spacing: 0.04em;
		text-transform: uppercase;
		padding: 0.5rem 0.85rem;
		min-height: 44px;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border: 1px solid var(--surface-border);
		color: var(--alt-charcoal);
		background: transparent;
		cursor: pointer;
	}

	.window-pill[aria-pressed="true"] {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
		border-color: var(--alt-charcoal);
	}

	.rule {
		height: 1px;
		background: var(--surface-border);
	}
</style>
