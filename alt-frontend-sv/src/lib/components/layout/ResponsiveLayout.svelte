<script lang="ts">
import type { Snippet } from "svelte";
import { page } from "$app/state";
import { useViewport } from "$lib/stores/viewport.svelte";
import Sidebar from "$lib/components/desktop/layout/Sidebar.svelte";
import MobileBottomNav from "$lib/components/mobile/MobileBottomNav.svelte";
import { shouldShowBottomNav } from "$lib/components/mobile/bottom-nav";
import { cn } from "$lib/utils";

let { children, class: className = "" }: { children: Snippet; class?: string } =
	$props();

const { isDesktop } = useViewport();

/** Routes that need full-bleed layout (no padding) */
const FULL_BLEED_PATHS = ["/feeds/tag-verse"];

const showBottomNav = $derived(shouldShowBottomNav(page.url.pathname));
const isFullBleed = $derived(FULL_BLEED_PATHS.includes(page.url.pathname));
</script>

{#if isDesktop}
	<!-- Desktop: Sidebar + main content area (matches DesktopLayout pattern) -->
	<div class="flex min-h-screen bg-[var(--surface-bg)]">
		<Sidebar />
		<main class={cn("flex-1", className || (isFullBleed ? "p-0" : "p-6"))}>
			{@render children()}
		</main>
	</div>
{:else}
	<!-- Mobile: Full-screen content + persistent BottomNav -->
	<div class="min-h-screen bg-[var(--surface-bg)]">
		<main class={cn(className, showBottomNav ? "pb-[calc(2.75rem+env(safe-area-inset-bottom,0px))]" : "")}>
			{@render children()}
		</main>
		{#if showBottomNav}
			<MobileBottomNav pathname={page.url.pathname} />
		{/if}
	</div>
{/if}
