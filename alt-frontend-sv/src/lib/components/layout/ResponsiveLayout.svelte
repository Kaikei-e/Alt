<script lang="ts">
import type { Snippet } from "svelte";
import { afterNavigate } from "$app/navigation";
import { page } from "$app/state";
import Sidebar from "$lib/components/desktop/layout/Sidebar.svelte";
import { isImmersiveRoute } from "$lib/components/mobile/bottom-nav";
import MobileBottomNav from "$lib/components/mobile/MobileBottomNav.svelte";
import { isDesktop, isMobile } from "$lib/stores/viewport.svelte";
import { cn } from "$lib/utils";

let { children, class: className = "" }: { children: Snippet; class?: string } =
	$props();

const FULL_BLEED_PATHS = ["/feeds/tag-verse"];

const isFullBleed = $derived(FULL_BLEED_PATHS.includes(page.url.pathname));
const isImmersive = $derived(isImmersiveRoute(page.url.pathname));

let mainEl = $state<HTMLElement | undefined>(undefined);

afterNavigate(() => {
	mainEl?.focus({ preventScroll: false });
});
</script>

<a
	href="#main"
	class="skip-link"
>
	Skip to main content
</a>

<!--
	One `<main>`, one `{@render children()}`.

	This used to be two `{#if isDesktop()}` branches, each with its own `<main>`
	wrapping its own `{@render children()}`. Once the viewport check became
	genuinely reactive, rotating a phone flipped that branch — and a flipped
	`{#if}` destroys everything inside it. Every page in the app was rebuilt from
	scratch on rotation, including the ones that never read the viewport: the
	visual-preview grid fell from 40 cards back to 20 and re-fetched, scrollY
	reset to 0, articles marked read reappeared, an in-flight Augur answer was
	re-asked, and half-typed forms were wiped.

	The layout difference between the two viewports is presentation only, so it
	belongs in Tailwind's responsive modifiers rather than in a structural
	branch. `{#if}` is kept for `Sidebar` and `MobileBottomNav` because they hold
	no page state — and because putting both in the DOM behind CSS would leave a
	screen reader announcing two navigations.
-->
<div class="block min-h-screen bg-[var(--surface-bg)] md:flex">
	{#if isDesktop()}
		<Sidebar />
	{/if}
	<main
		id="main"
		tabindex="-1"
		bind:this={mainEl}
		class={cn(
			"outline-none md:flex-1",
			// Desktop padding, previously `className || (isFullBleed ? "p-0" : "p-6")`:
			// an explicit `class` prop replaced the default padding outright.
			!className && (isFullBleed ? "md:p-0" : "md:p-6"),
			// Mobile clearance for the fixed bottom nav; immersive routes drop
			// the bar, so they drop the reservation with it.
			!isImmersive &&
				"max-md:pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))]",
			className,
		)}
	>
		{@render children()}
	</main>
</div>

{#if isMobile() && !isImmersive}
	<MobileBottomNav pathname={page.url.pathname} />
{/if}

<style>
	.skip-link {
		position: absolute;
		left: 0.5rem;
		top: 0.5rem;
		z-index: 9999;
		padding: 0.5rem 0.75rem;
		background: var(--alt-charcoal, #1a1a1a);
		color: var(--surface-bg, #faf9f7);
		font-family: var(--font-body);
		font-size: 0.85rem;
		text-decoration: none;
		transform: translateY(-200%);
		transition: transform 0.15s ease-out;
	}
	.skip-link:focus {
		transform: translateY(0);
		outline: 2px solid var(--alt-primary, #2f4f4f);
		outline-offset: 2px;
	}
</style>
