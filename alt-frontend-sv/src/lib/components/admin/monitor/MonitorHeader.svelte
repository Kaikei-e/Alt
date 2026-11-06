<script lang="ts">
import {
	RangeWindow,
	Step,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import type { StreamState } from "#lib/hooks/useConnectAdminMetrics.svelte.js";
import { formatLocalTime, localTimeZone } from "./format";
import { badge, freshness } from "./model";
import TimeRangePicker from "./TimeRangePicker.svelte";

let {
	window = $bindable(RangeWindow.RANGE_WINDOW_1H),
	step = $bindable(Step.STEP_30S),
	streamState,
	snapshotTime,
	now = Date.now(),
	lastError = null,
	onTogglePause,
	paused = false,
}: {
	window?: RangeWindow;
	step?: Step;
	streamState: StreamState;
	snapshotTime: string;
	now?: number;
	lastError?: string | null;
	onTogglePause?: () => void;
	paused?: boolean;
} = $props();

const tz = localTimeZone();

const indicator = $derived.by(() => {
	if (paused) return { glyph: "▮▮", text: "paused" };
	switch (streamState) {
		case "live":
			return { glyph: "●", text: "live" };
		case "connecting":
			return { glyph: "◌", text: "connecting" };
		case "degraded":
			return { glyph: "▲", text: "degraded" };
		case "closed":
			return { glyph: "○", text: "closed" };
		default:
			return { glyph: "○", text: "idle" };
	}
});

const snapFreshness = $derived(freshness(snapshotTime || null, now));
const staleBadge = $derived(badge("stale"));

const freshnessText = $derived.by(() => {
	if (!snapshotTime || snapFreshness.ageSeconds === null) {
		return "Waiting for first snapshot";
	}
	return `Updated ${formatLocalTime(snapshotTime)} · ${snapFreshness.ageSeconds}s ago`;
});
</script>

<header class="header">
	<div class="title-row">
		<h1>System Monitor</h1>
		<p class="sub">
			Operator audit view across alerts, edge SLOs, service RED signals, host
			resources, certificates, and queues.
			<span class="tz-note" data-testid="header-timezone">Times in {tz}</span>
		</p>
		{#if lastError}
			<p class="transport-error" data-testid="header-transport-error">
				Transport error: {lastError}
			</p>
		{/if}
	</div>

	<div class="controls">
		<TimeRangePicker bind:window bind:step />
		<div class="status" data-state={indicator.text} aria-live="polite">
			<span class="glyph" aria-hidden="true">{indicator.glyph}</span>
			<span class="state-text">{indicator.text}</span>
			<span class="snapshot" title="last snapshot freshness">
				{freshnessText}
			</span>
			{#if snapFreshness.stale}
				<span
					class="stale-badge"
					data-testid="stale-badge"
					aria-label="Snapshot stale"
				>
					<span aria-hidden="true">{staleBadge.glyph}</span>
					<span>{staleBadge.label}</span>
				</span>
			{/if}
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
		border-bottom: 1px solid var(--obs-rule-strong, var(--surface-border));
		min-width: 0;
	}

	.title-row {
		min-width: 0;
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
		max-width: 68ch;
	}

	.tz-note {
		display: inline-block;
		margin-left: 0.35rem;
		font-family: var(--font-mono);
		font-size: 0.72rem;
		color: var(--alt-ash);
	}

	.transport-error {
		margin: 0.35rem 0 0;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--obs-critical, var(--alt-error));
		overflow-wrap: anywhere;
	}

	.controls {
		display: flex;
		align-items: center;
		gap: 0.6rem;
		justify-content: flex-end;
		flex-wrap: wrap;
		min-width: 0;
	}

	.status {
		display: inline-flex;
		align-items: baseline;
		flex-wrap: wrap;
		gap: 0.4rem;
		padding: 0.25rem 0.55rem;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		background: var(--surface-bg);
		font-family: var(--font-mono);
		font-size: 0.72rem;
	}

	.status[data-state="live"] .glyph,
	.status[data-state="live"] .state-text {
		color: var(--obs-good, var(--alt-success));
	}

	.status[data-state="paused"] .glyph,
	.status[data-state="paused"] .state-text,
	.status[data-state="degraded"] .glyph,
	.status[data-state="degraded"] .state-text,
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
		color: var(--alt-slate);
		font-size: 0.7rem;
		font-variant-numeric: tabular-nums;
	}

	.stale-badge {
		display: inline-flex;
		align-items: baseline;
		gap: 0.25rem;
		padding: 0.05rem 0.35rem;
		border: 0.5px solid var(--obs-warn, var(--alt-warning));
		color: var(--obs-warn, var(--alt-warning));
		font-weight: 600;
		font-size: 0.68rem;
		letter-spacing: 0.08em;
	}

	.pause {
		padding: 0.3rem 0.7rem;
		border: 0.5px solid var(--alt-charcoal);
		background: var(--surface-bg);
		font-family: var(--font-mono);
		font-size: 0.74rem;
		cursor: pointer;
	}

	.pause:hover {
		background: var(--alt-charcoal);
		color: var(--surface-bg);
	}

	@media (max-width: 900px) {
		.header {
			grid-template-columns: 1fr;
		}

		.controls {
			justify-content: flex-start;
		}
	}
</style>
