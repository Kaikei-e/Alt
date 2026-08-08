<script lang="ts">
/**
 * Switch primitive.
 *
 * A new primitive rather than a copy of the toggle in `settings/feeds`: that
 * one is a 24px-tall `rounded-full` pill, which is below the 44px touch-target
 * floor in DESIGN_LANGUAGE.md and contradicts its "no border-radius" rule.
 * Reproducing it in a surface whose whole purpose is mobile would be the wrong
 * trade — so this follows the document instead, and `settings/feeds` can adopt
 * it later.
 *
 * Shape follows the Button spec: sharp edges, 1.5px charcoal border, and the
 * on-state reads as a filled block rather than a colour change, so the state is
 * legible without relying on hue.
 */
import type { HTMLButtonAttributes } from "svelte/elements";

type Props = Omit<HTMLButtonAttributes, "onchange"> & {
	checked: boolean;
	label: string;
	busy?: boolean;
	onchange?: (next: boolean) => void;
};

let {
	checked,
	label,
	busy = false,
	disabled = false,
	onchange,
	...restProps
}: Props = $props();

function toggle() {
	if (disabled || busy) return;
	onchange?.(!checked);
}
</script>

<button
	type="button"
	role="switch"
	aria-checked={checked}
	aria-label={label}
	aria-busy={busy}
	{disabled}
	onclick={toggle}
	class="alt-switch"
	class:is-on={checked}
	class:is-busy={busy}
	{...restProps}
>
	<span class="knob" aria-hidden="true"></span>
</button>

<style>
	.alt-switch {
		/* 44px is the floor, not a target: this control is the whole interaction
		   on the notification settings page. */
		min-width: 72px;
		min-height: 44px;
		display: inline-flex;
		align-items: center;
		padding: 0 4px;
		border: 1.5px solid var(--alt-charcoal);
		border-radius: 0;
		background: transparent;
		cursor: pointer;
		touch-action: manipulation;
		transition: background-color 160ms ease;
	}

	.alt-switch:focus-visible {
		outline: 2px solid var(--alt-charcoal);
		outline-offset: 2px;
	}

	.alt-switch:disabled {
		cursor: not-allowed;
		opacity: 0.45;
	}

	.knob {
		display: block;
		width: 28px;
		height: 28px;
		background: var(--alt-charcoal);
		transform: translateX(0);
		transition: transform 160ms ease, background-color 160ms ease;
	}

	.is-on {
		background: var(--alt-charcoal);
	}

	.is-on .knob {
		background: var(--surface-bg);
		transform: translateX(32px);
	}

	/* In flight the knob dims rather than spinning — DESIGN_LANGUAGE.md rules
	   out spinners, and a control that keeps its position is easier to read
	   mid-request than one that animates. */
	.is-busy .knob {
		opacity: 0.5;
	}

	@media (prefers-reduced-motion: reduce) {
		.alt-switch,
		.knob {
			transition: none;
		}
	}
</style>
