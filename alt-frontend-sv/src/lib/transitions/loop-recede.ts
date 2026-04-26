/**
 * loop-recede — Knowledge Loop foreground exit transition.
 *
 * ADR-000831 §11-13:
 *   - Depth lives on tiles + planes, not glyphs (text stays flat).
 *   - Alt-Paper forbids drop shadows; depth is expressed via Z-translation,
 *     scale, and saturate/brightness shifts.
 *   - Reduced-motion replaces depth simulation with dissolve + highlight
 *     fade + color shift (§12.5). For `out:` transitions specifically that
 *     means: drop translateZ + scale, keep opacity + filter.
 *
 * The transition is only applied as `out:` — entries arrive with the existing
 * `entry-in` keyframe inside `LoopEntryTile.svelte`, and survivors slide up
 * via `animate:flip` on the parent `#each`. The tile that is leaving recedes
 * along Z (~72 px back) while desaturating and dissolving, evoking a printed
 * ledger row "filing itself back" rather than dropping below the page.
 */

import { cubicOut } from "svelte/easing";
import type { TransitionConfig } from "svelte/transition";

export interface LoopRecedeOptions {
	/** Total duration in milliseconds. Default: 280. */
	duration?: number;
	/** Maximum Z offset at end-of-transition (px). Default: 72. */
	maxTranslateZ?: number;
	/** Scale at end-of-transition. Default: 0.965. */
	endScale?: number;
}

const DEFAULT_DURATION = 280;
const DEFAULT_MAX_Z = 72;
const DEFAULT_END_SCALE = 0.965;

/**
 * Detect prefers-reduced-motion at transition-construction time. Svelte calls
 * the transition fn each time an exit fires, so this matches the user's
 * current setting.
 */
function prefersReducedMotion(): boolean {
	if (typeof window === "undefined" || !window.matchMedia) return false;
	try {
		return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
	} catch {
		return false;
	}
}

export function loopRecede(
	_node: Element,
	options: LoopRecedeOptions = {},
): TransitionConfig {
	const duration = options.duration ?? DEFAULT_DURATION;
	const maxZ = options.maxTranslateZ ?? DEFAULT_MAX_Z;
	const endScale = options.endScale ?? DEFAULT_END_SCALE;
	const reduced = prefersReducedMotion();

	if (reduced) {
		// §12.5 mapping: dissolve + slight saturate falloff, no Z, no scale.
		return {
			duration: Math.min(duration, 200),
			easing: cubicOut,
			css: (t) => {
				const opacity = t;
				const sat = 0.85 + 0.15 * t;
				return `opacity: ${opacity}; filter: saturate(${sat});`;
			},
		};
	}

	// `t` runs from 1 → 0 during an `out:` transition, so the at-rest state is
	// `t = 1` (no recede) and `t = 0` is "fully receded + invisible".
	return {
		duration,
		easing: cubicOut,
		css: (t) => {
			const z = -maxZ * (1 - t);
			const scale = endScale + (1 - endScale) * t;
			const opacity = t;
			const sat = 0.85 + 0.15 * t;
			const bright = 0.99 + 0.01 * t;
			return [
				`transform: translateZ(${z}px) scale(${scale});`,
				`opacity: ${opacity};`,
				`filter: saturate(${sat}) brightness(${bright});`,
			].join(" ");
		},
	};
}
