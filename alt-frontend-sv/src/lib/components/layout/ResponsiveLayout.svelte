<script lang="ts">
import type { Snippet } from "svelte";
import { page } from "$app/state";
import { useViewport } from "$lib/stores/viewport.svelte";
import Sidebar from "$lib/components/desktop/layout/Sidebar.svelte";
import FloatingMenu from "$lib/components/mobile/feeds/swipe/FloatingMenu.svelte";
import { cn } from "$lib/utils";

let { children, class: className = "" }: { children: Snippet; class?: string } =
	$props();

const { isDesktop } = useViewport();

/** Routes where FloatingMenu should be hidden (full-screen interactive pages) */
const HIDE_FLOATING_MENU_PATHS = ["/sv/augur", "/sv/feeds/swipe", "/sv/feeds/search"];

const showFloatingMenu = $derived(
	!HIDE_FLOATING_MENU_PATHS.includes(page.url.pathname),
);
</script>

{#if isDesktop}
	<!-- Desktop: Sidebar + main content area (matches DesktopLayout pattern) -->
	<div class="flex min-h-screen bg-[var(--surface-bg)]">
		<Sidebar />
		<main class={cn("flex-1", className || "p-6")}>
			{@render children()}
		</main>
	</div>
{:else}
	<!-- Mobile: Full-screen content + FloatingMenu FAB -->
	<div class="min-h-screen bg-[var(--surface-bg)]">
		<main class={className}>
			{@render children()}
		</main>
		{#if showFloatingMenu}
			<FloatingMenu />
		{/if}
	</div>
{/if}
