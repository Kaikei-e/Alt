<script lang="ts">
import type { LoopStageName } from "$lib/connect/knowledge_loop";

/**
 * OODA pipeline kicker — replaces the flat horizontal stage labels in the
 * Knowledge Loop masthead with a perspective-aware "ribbon" that conveys the
 * cycle's direction and the user's current position. The active stage rises
 * to the foreground (Z=0); upcoming stages recede further into the page.
 *
 * Alt-Paper composition (canonical contract §12, ADR-000831):
 *   - Depth lives on the planes, not on the glyphs. Letters stay flat —
 *     only the kicker tile's transform changes.
 *   - No drop shadows. Saturation + brightness + a 1px arc rule below the
 *     active label do the spatial work.
 *   - Reduced-motion users get a flat ribbon with the active stage in
 *     `var(--alt-charcoal)` and the others in `var(--alt-ash)`. The
 *     translateZ disappears, the arc rule remains horizontal.
 *
 * Layout choices:
 *   - The ribbon is laid out in a 2-row grid: kickers on top, an arc rule
 *     under the active one. A `→` between kickers signals direction, with
 *     a wrap-around `↻` after Act to close the loop visually.
 *   - The component is presentational only — it does not call any hooks
 *     or load data; it receives `currentStage` from `+page.svelte`.
 */

let { currentStage }: { currentStage: LoopStageName } = $props();

type Stage = { name: LoopStageName; label: string };
const stages: Stage[] = [
	{ name: "observe", label: "Observe" },
	{ name: "orient", label: "Orient" },
	{ name: "decide", label: "Decide" },
	{ name: "act", label: "Act" },
];

const order: Record<LoopStageName, number> = {
	observe: 0,
	orient: 1,
	decide: 2,
	act: 3,
};

// Each kicker's depth band is computed from its distance to the active stage
// along the Observe→Orient→Decide→Act→Observe cycle. The active stage sits
// at Z=0, the next at Z=-14, and so on. The cycle wraps so that after Act
// the next foreground candidate is Observe — visually "pulling" the loop
// closed without any animation cue.
function depthFor(stage: LoopStageName): number {
	const distance = (order[stage] - order[currentStage] + 4) % 4;
	return distance; // 0..3
}
</script>

<nav
	class="ooda"
	aria-label="OODA loop progression"
	data-testid="ooda-pipeline"
	data-current-stage={currentStage}
>
	<ol class="row">
		{#each stages as stage, i (stage.name)}
			{@const depth = depthFor(stage.name)}
			<li
				class="kicker"
				class:kicker--active={depth === 0}
				data-depth={depth}
				data-stage={stage.name}
			>
				<span class="kicker-label">{stage.label}</span>
				{#if depth === 0}
					<span class="kicker-rule" aria-hidden="true"></span>
				{/if}
			</li>
			{#if i < stages.length - 1}
				<li class="arrow" aria-hidden="true">→</li>
			{:else}
				<li class="arrow arrow--wrap" aria-hidden="true">↻</li>
			{/if}
		{/each}
	</ol>
</nav>

<style>
	.ooda {
		/* Local 3D context so kicker depths render against a shared vanishing
		 * point. Without preserve-3d here, each li would flatten to 2D. */
		perspective: 700px;
		perspective-origin: 50% 60%;
		transform-style: preserve-3d;
	}

	.row {
		list-style: none;
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.4rem 0.5rem;
		margin: 0;
		padding: 0;
		transform-style: preserve-3d;
	}

	.kicker {
		position: relative;
		display: inline-flex;
		flex-direction: column;
		align-items: center;
		gap: 0.18rem;
		font-family: var(--font-body, "Source Sans 3", system-ui, sans-serif);
		font-size: 0.62rem;
		font-weight: 700;
		letter-spacing: 0.18em;
		text-transform: uppercase;
		color: var(--alt-ash, #999);
		transition:
			transform 320ms cubic-bezier(0.2, 0, 0.1, 1),
			color 220ms ease,
			filter 220ms ease;
	}

	/* Stage Z bands. Active = Z 0 (front, full saturation). Subsequent stages
	   recede along Z and desaturate. The label glyphs stay flat — depth is
	   carried by the kicker tile's transform plus the saturation step. */
	.kicker[data-depth="0"] {
		transform: translateZ(0);
		color: var(--alt-charcoal, #1a1a1a);
		filter: saturate(1.05);
	}
	.kicker[data-depth="1"] {
		transform: translateZ(-14px);
		filter: saturate(0.95);
	}
	.kicker[data-depth="2"] {
		transform: translateZ(-26px);
		filter: saturate(0.88);
	}
	.kicker[data-depth="3"] {
		transform: translateZ(-38px);
		filter: saturate(0.8);
	}

	.kicker-label {
		display: inline-block;
	}

	/* The arc rule appears only under the active kicker. A subtle inverse
	 * curve plus a 2px stroke evokes the OODA cycle's continuous loop without
	 * leaving the Alt-Paper "thin rule" vocabulary. */
	.kicker-rule {
		display: block;
		width: 100%;
		height: 2px;
		background: var(--alt-charcoal, #1a1a1a);
		border-radius: 0;
		/* Slight skew anchors the rule to the perspective ribbon — only on
		 * the active kicker, so the eye reads it as the user's current
		 * "page line" in the loop. */
		transform: translateY(2px);
	}

	.arrow {
		display: inline-flex;
		align-items: baseline;
		font-family: var(--font-mono, "IBM Plex Mono", ui-monospace, monospace);
		font-size: 0.7rem;
		color: var(--surface-border, #c8c8c8);
		transform: translateZ(-30px);
	}
	.arrow--wrap {
		color: var(--alt-sand, #d4a574);
		transform: translateZ(-46px);
	}

	@media (prefers-reduced-motion: reduce) {
		.ooda {
			perspective: none;
			transform-style: flat;
		}
		.row,
		.kicker[data-depth],
		.arrow,
		.arrow--wrap {
			transform: none;
		}
		.kicker {
			transition: color 200ms ease;
			filter: none !important;
		}
		.kicker[data-depth="0"] {
			color: var(--alt-charcoal, #1a1a1a);
		}
	}
</style>
