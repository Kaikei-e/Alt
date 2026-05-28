<script lang="ts">
import {
	RangeWindow,
	Step,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import type { StreamState } from "$lib/hooks/useConnectAdminMetrics.svelte";
import TimeRangePicker from "./TimeRangePicker.svelte";

let {
	window = $bindable(RangeWindow.RANGE_WINDOW_1H),
	step = $bindable(Step.STEP_30S),
	streamState,
	snapshotTime,
	onTogglePause,
	paused = false,
}: {
	window?: RangeWindow;
	step?: Step;
	streamState: StreamState;
	snapshotTime: string;
	onTogglePause?: () => void;
	paused?: boolean;
} = $props();

const indicator = $derived(() => {
	if (paused) return { glyph: "▮▮", text: "paused" };
	switch (streamState) {
		case "live":
			return { glyph: "▲", text: "live" };
		case "connecting":
			return { glyph: "●", text: "connecting" };
		case "degraded":
			return { glyph: "●", text: "degraded" };
		case "closed":
			return { glyph: "○", text: "closed" };
		default:
			return { glyph: "○", text: "idle" };
	}
});

const snapshotLabel = $derived(() => {
	if (!snapshotTime) return "—";
	try {
		return new Date(snapshotTime).toLocaleTimeString();
	} catch {
		return snapshotTime;
	}
});
</script>

<header class="header">
	<div class="title-row">
		<h1>System Monitor</h1>
		<p class="sub">
			Live Prometheus snapshot for the Alt stack. Refresh interval 5 s, stream
			rotates every 15 min to avoid intermediate proxy idle kills.
		</p>
	</div>

	<div class="controls">
		<TimeRangePicker bind:window bind:step />
		<div class="status" data-state={indicator().text} aria-live="polite">
			<span class="glyph" aria-hidden="true">{indicator().glyph}</span>
			<span class="state-text">{indicator().text}</span>
			<span class="snapshot" title="last snapshot timestamp"
				>{snapshotLabel()}</span
			>
		</div>
		{#if onTogglePause}
			<button type="button" class="pause" onclick={onTogglePause}>
				{paused ? "Resume" : "Pause"}
			</button>
		{/if}
	</div>
</header>

<style>
	.header {
		display: grid;
		grid-template-columns: 1fr auto;
		gap: 1rem;
		align-items: end;
		padding: 0.4rem 0 1rem;
		border-bottom: 0.5px solid var(--obs-rule, var(--surface-border));
	}

	.title-row h1 {
		font-family: var(--font-display, var(--font-serif));
		font-size: 1.85rem;
		font-weight: 600;
		margin: 0;
		letter-spacing: -0.005em;
		color: var(--alt-charcoal);
	}

	.sub {
		font-family: var(--font-body);
		font-size: 0.84rem;
		color: var(--alt-slate);
		margin: 0.25rem 0 0;
		max-width: 52ch;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		justify-content: flex-end;
	}

	.status {
		display: inline-flex;
		align-items: baseline;
		gap: 0.4rem;
		padding: 0.25rem 0.55rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		background: var(--surface);
		font-family: var(--font-mono);
		font-size: 0.72rem;
	}

	.status[data-state="live"] .glyph,
	.status[data-state="live"] .state-text {
		color: var(--obs-good, var(--alt-success));
	}

	.status[data-state="degraded"] .glyph,
	.status[data-state="connecting"] .glyph {
		color: var(--obs-warn, var(--alt-warning));
	}

	.status[data-state="closed"] .glyph {
		color: var(--alt-ash);
	}

	.state-text {
		text-transform: uppercase;
		letter-spacing: 0.12em;
	}

	.snapshot {
		color: var(--alt-ash);
		font-size: 0.7rem;
	}

	.pause {
		padding: 0.3rem 0.7rem;
		border: 0.5px solid var(--alt-charcoal);
		background: var(--surface);
		font-family: var(--font-mono);
		font-size: 0.74rem;
		cursor: pointer;
	}

	.pause:hover {
		background: var(--alt-charcoal);
		color: var(--surface);
	}

	@media (max-width: 900px) {
		.header {
			grid-template-columns: 1fr;
		}

		.controls {
			justify-content: flex-start;
			flex-wrap: wrap;
		}
	}
</style>
