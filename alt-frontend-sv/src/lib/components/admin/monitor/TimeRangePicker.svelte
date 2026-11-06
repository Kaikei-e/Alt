<script lang="ts">
import {
	RangeWindow,
	Step,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import {
	clampStep,
	optToStepEnum,
	type StepOpt,
	stepEnumToOpt,
	validSteps,
	type WindowOpt,
	windowEnumToOpt,
} from "./model";

let {
	window = $bindable(RangeWindow.RANGE_WINDOW_1H),
	step = $bindable(Step.STEP_30S),
}: {
	window?: RangeWindow;
	step?: Step;
} = $props();

const windows: Array<{ value: RangeWindow; label: WindowOpt }> = [
	{ value: RangeWindow.RANGE_WINDOW_5M, label: "5m" },
	{ value: RangeWindow.RANGE_WINDOW_15M, label: "15m" },
	{ value: RangeWindow.RANGE_WINDOW_1H, label: "1h" },
	{ value: RangeWindow.RANGE_WINDOW_6H, label: "6h" },
	{ value: RangeWindow.RANGE_WINDOW_24H, label: "24h" },
];

const steps: Array<{ value: Step; label: StepOpt }> = [
	{ value: Step.STEP_15S, label: "15s" },
	{ value: Step.STEP_30S, label: "30s" },
	{ value: Step.STEP_1M, label: "1m" },
	{ value: Step.STEP_5M, label: "5m" },
];

const allowedSteps = $derived(validSteps(windowEnumToOpt(window)));

function stepDisabled(s: StepOpt): boolean {
	return !allowedSteps.includes(s);
}

function selectWindow(nextWindow: RangeWindow, label: WindowOpt) {
	const nextStepOpt = clampStep(label, stepEnumToOpt(step));
	step = optToStepEnum(nextStepOpt);
	window = nextWindow;
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
					onchange={() => selectWindow(w.value, w.label)}
				/>
				<span>{w.label}</span>
			</label>
		{/each}
	</fieldset>
	<fieldset>
		<legend>Step</legend>
		{#each steps as s (s.value)}
			{@const disabled = stepDisabled(s.label)}
			<label
				class="seg"
				class:active={step === s.value}
				class:disabled={disabled}
			>
				<input
					type="radio"
					name="monitor-step"
					value={s.value}
					checked={step === s.value}
					{disabled}
					onchange={() => {
						if (!disabled) step = s.value;
					}}
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
		flex-wrap: wrap;
	}

	fieldset {
		display: inline-flex;
		gap: 0;
		border: 0.5px solid var(--obs-rule, var(--surface-border));
		padding: 0;
		margin: 0;
		background: var(--surface-bg);
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
		color: var(--surface-bg);
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
