import { MediaQuery } from "svelte/reactivity";

// Written to match the CSS `@media (prefers-reduced-motion: reduce)` blocks
// character for character, so a component that asks prefersReducedMotion()
// and a stylesheet that says `prefers-reduced-motion` are answering the same
// question the reader's OS settings posed.
const reducedMotionQuery = new MediaQuery(
	"prefers-reduced-motion: reduce",
	false,
);

/**
 * True while the reader has asked their OS to reduce motion.
 *
 * **Call it, never alias it.** Reading it inside a template, a `$derived`, or
 * an `$effect` subscribes that spot to the media query, so the answer follows
 * the OS setting being toggled mid-session. Parking the result in a plain
 * `const` takes a one-time snapshot and stops following — see
 * `viewport.svelte.ts` for the full account of why this module hands out a
 * function rather than a value.
 *
 * Reading it outside a reactive context — an event handler, the start of a
 * streamed turn — is fine and deliberate: it answers for the setting as it is
 * at that instant. The typewriter reveal reads it once per turn, which is the
 * finest granularity a mid-answer toggle could reasonably want.
 *
 * SSR / no `window`: motion is the default, reduction the explicit request —
 * returns false.
 */
export function prefersReducedMotion(): boolean {
	return reducedMotionQuery.current;
}
