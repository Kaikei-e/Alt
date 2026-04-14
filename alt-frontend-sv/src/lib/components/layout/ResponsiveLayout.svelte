<script lang="ts">
import type { Snippet } from "svelte";
import { page } from "$app/state";
import { afterNavigate } from "$app/navigation";
import { useViewport } from "$lib/stores/viewport.svelte";
import Sidebar from "$lib/components/desktop/layout/Sidebar.svelte";
import MobileBottomNav from "$lib/components/mobile/MobileBottomNav.svelte";
import { cn } from "$lib/utils";

let { children, class: className = "" }: { children: Snippet; class?: string } =
	$props();

const { isDesktop } = useViewport();

const FULL_BLEED_PATHS = ["/feeds/tag-verse"];

const isFullBleed = $derived(FULL_BLEED_PATHS.includes(page.url.pathname));

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

{#if isDesktop}
	<div class="flex min-h-screen bg-[var(--surface-bg)]">
		<Sidebar />
		<main
			id="main"
			tabindex="-1"
			bind:this={mainEl}
			class={cn("flex-1 outline-none", className || (isFullBleed ? "p-0" : "p-6"))}
		>
			{@render children()}
		</main>
	</div>
{:else}
	<div class="min-h-screen bg-[var(--surface-bg)]">
		<main
			id="main"
			tabindex="-1"
			bind:this={mainEl}
			class={cn(
				"outline-none pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))]",
				className,
			)}
		>
			{@render children()}
		</main>
		<MobileBottomNav pathname={page.url.pathname} />
	</div>
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
