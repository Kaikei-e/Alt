<script lang="ts">
import {
	RangeWindow,
	Step,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";

let {
	window = $bindable(RangeWindow.RANGE_WINDOW_1H),
	step = $bindable(Step.STEP_30S),
}: {
	window?: RangeWindow;
	step?: Step;
} = $props();

const windows: Array<{ value: RangeWindow; label: string }> = [
	{ value: RangeWindow.RANGE_WINDOW_5M, label: "5m" },
	{ value: RangeWindow.RANGE_WINDOW_15M, label: "15m" },
	{ value: RangeWindow.RANGE_WINDOW_1H, label: "1h" },
	{ value: RangeWindow.RANGE_WINDOW_6H, label: "6h" },
	{ value: RangeWindow.RANGE_WINDOW_24H, label: "24h" },
];

const steps: Array<{ value: Step; label: string }> = [
	{ value: Step.STEP_15S, label: "15s" },
	{ value: Step.STEP_30S, label: "30s" },
	{ value: Step.STEP_1M, label: "1m" },
	{ value: Step.STEP_5M, label: "5m" },
];

// Server-side guard: window / step ≤ 720 points. Disable any step that would
// exceed that for the chosen window so the picker can never send a request the
// gateway rejects with InvalidArgument.
function windowSeconds(w: RangeWindow): number {
	switch (w) {
		case RangeWindow.RANGE_WINDOW_5M:
			return 300;
		case RangeWindow.RANGE_WINDOW_15M:
			return 900;
		case RangeWindow.RANGE_WINDOW_1H:
			return 3_600;
		case RangeWindow.RANGE_WINDOW_6H:
			return 21_600;
		case RangeWindow.RANGE_WINDOW_24H:
			return 86_400;
		default:
			return 0;
	}
}

function stepSeconds(s: Step): number {
	switch (s) {
		case Step.STEP_15S:
			return 15;
		case Step.STEP_30S:
			return 30;
		case Step.STEP_1M:
			return 60;
		case Step.STEP_5M:
			return 300;
		default:
			return 0;
	}
}

function stepDisabled(s: Step): boolean {
	const ws = windowSeconds(window);
	const ss = stepSeconds(s);
	if (ws === 0 || ss === 0) return false;
	return ws / ss > 720;
}
</script>

<div class="picker" role="group" aria-label="Time range">
	<fieldset>
		<legend>Window</legend>
		{#each windows as w (w.value)}
			<label class="seg" class:active={window === w.value}>
				<input
					type="radio"
					name="monitor-window"
					value={w.value}
					checked={window === w.value}
					onchange={() => (window = w.value)}
				/>
				<span>{w.label}</span>
			</label>
		{/each}
	</fieldset>
	<fieldset>
		<legend>Step</legend>
		{#each steps as s (s.value)}
			<label
				class="seg"
				class:active={step === s.value}
				class:disabled={stepDisabled(s.value)}
			>
				<input
					type="radio"
					name="monitor-step"
					value={s.value}
					checked={step === s.value}
					disabled={stepDisabled(s.value)}
					onchange={() => (step = s.value)}
				/>
				<span>{s.label}</span>
			</label>
		{/each}
	</fieldset>
</div>

<style>
	.picker {
		display: flex;
		gap: 0.6rem;
		align-items: center;
	}

	fieldset {
		display: inline-flex;
		gap: 0;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		padding: 0;
		margin: 0;
		background: var(--surface);
	}

	legend {
		display: none;
	}

	.seg {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 2.4rem;
		padding: 0.25rem 0.55rem;
		font-family: var(--font-mono);
		font-size: 0.74rem;
		color: var(--alt-slate);
		cursor: pointer;
		border-right: 0.5px solid var(--obs-rule, var(--surface-border));
	}

	.seg:last-child {
		border-right: none;
	}

	.seg.active {
		background: var(--alt-charcoal);
		color: var(--surface);
	}

	.seg.disabled {
		opacity: 0.35;
		cursor: not-allowed;
	}

	.seg input {
		position: absolute;
		opacity: 0;
		pointer-events: none;
	}
</style>
