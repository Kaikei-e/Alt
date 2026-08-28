import { MediaQuery } from "svelte/reactivity";

/**
 * The `md` breakpoint, in `rem`, which is the unit TailwindCSS v4 states its own
 * default in.
 *
 * `rem`, not `px`, and that is the whole point. This app sets no root
 * font-size, so it inherits whatever default the reader has chosen in their
 * browser — and in a media query `rem` resolves against that initial value
 * rather than against anything the page applies. A reader who raises their
 * default font for legibility therefore keeps the one-column layout up to a
 * wider pixel width, which is what they need: larger text wants more room per
 * column, not the same room with more columns in it.
 *
 * Stating this in `px` would have been the tempting fix, because it makes the
 * number here and the number in `md:` visibly the same. It also quietly throws
 * that adaptation away for every `md:` utility in the app, leaving the reader
 * with text that scales and a layout that does not.
 */
export const BREAKPOINT_REM = 48;

// Written to match Tailwind's own `md` query character for character, so the
// two cannot drift: a component that asks `isDesktop()` and a class that says
// `md:` are answering the same question.
const desktopQuery = new MediaQuery(`min-width: ${BREAKPOINT_REM}rem`, false);

/**
 * True while the viewport is at least `BREAKPOINT_REM` wide.
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
 * True while the viewport is narrower than `BREAKPOINT_REM` — the complement of
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
