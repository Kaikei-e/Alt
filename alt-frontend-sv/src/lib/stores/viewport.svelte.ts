import { MediaQuery } from "svelte/reactivity";

/** Breakpoint matching TailwindCSS v4 `md` (768px) */
export const BREAKPOINT = 768;

const desktopQuery = new MediaQuery(`min-width: ${BREAKPOINT}px`, false);

/**
 * Creates a viewport state object with reactive isDesktop / isMobile getters.
 * SSR fallback: mobile-first (isDesktop = false).
 */
export function createViewportState() {
	return {
		get isDesktop() {
			return desktopQuery.current;
		},
		get isMobile() {
			return !desktopQuery.current;
		},
	};
}

/**
 * Convenience alias used in Svelte components.
 * Usage: `const { isDesktop } = useViewport();`
 */
export function useViewport() {
	return createViewportState();
}
