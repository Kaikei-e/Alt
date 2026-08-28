import { MediaQuery } from "svelte/reactivity";

/** Breakpoint matching TailwindCSS v4 `md` (768px) */
export const BREAKPOINT = 768;

const desktopQuery = new MediaQuery(`min-width: ${BREAKPOINT}px`, false);

/**
 * True while the viewport is at least `BREAKPOINT` wide.
 *
 * **Call it, never alias it.** Reading it inside a template, a `$derived`, or
 * an `$effect` subscribes that spot to the media query, so the answer follows
 * a window resize or a phone being rotated:
 *
 * ```svelte
 * {#if isDesktop()}<Sidebar />{:else}<MobileBottomNav />{/if}
 * const sheetSide = $derived(isDesktop() ? "right" : "bottom");
 * ```
 *
 * Parking the result in a plain `const` takes a one-time snapshot and stops
 * following the viewport. That is precisely what this module used to hand out:
 * `const { isDesktop } = useViewport()` evaluated a getter once and bound a
 * dead boolean, so a phone opened in landscape (Pixel 5 = 851 CSS px, iPhone 13
 * = 844) got the desktop layout and never came back to mobile without a reload.
 *
 * Functions are deliberate rather than an object of getters: the shape that
 * freezes, `{#if isDesktop}`, is a `svelte-check` error (ts2774, "did you mean
 * to call it instead?"), so `bun run check` catches it. A frozen destructure
 * has no such tell — nothing in the toolchain can see it.
 *
 * Reading it outside a reactive context — an event handler, `onMount` — is
 * fine and deliberate: it answers for the viewport as it is at that instant.
 *
 * SSR / no `window`: mobile-first, returns false.
 */
export function isDesktop(): boolean {
	return desktopQuery.current;
}

/**
 * True while the viewport is narrower than `BREAKPOINT` — the complement of
 * {@link isDesktop}, with the same call-don't-alias rule.
 *
 * Reach for this instead of `!isDesktop()`. TypeScript flags a bare
 * `{#if isDesktop}` but says nothing about `{#if !isDesktop}`, which is
 * silently always false — so the negated spelling is the one hole in the
 * compile-time net, and there is never a need to write it.
 */
export function isMobile(): boolean {
	return !desktopQuery.current;
}
